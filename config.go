// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"reflect"
	"strings"
)

type collectorResConfig struct {
	Kind    string `yaml:"kind"`
	Pattern string `yaml:"pattern"` // XXX: Not Implemented
}

type vertexConfig struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

type edgeConfig struct {
	Name string       `yaml:"name"`
	From vertexConfig `yaml:"from"`
	To   vertexConfig `yaml:"to"`
}

type GraphConfig struct {
	Graph     string `yaml:"graph"`
	Resources struct {
		Noop  []*NoopRes  `yaml:"noop"`
		Pkg   []*PkgRes   `yaml:"pkg"`
		File  []*FileRes  `yaml:"file"`
		Svc   []*SvcRes   `yaml:"svc"`
		Exec  []*ExecRes  `yaml:"exec"`
		Timer []*TimerRes `yaml:"timer"`
	} `yaml:"resources"`
	Collector []collectorResConfig `yaml:"collect"`
	Edges     []edgeConfig         `yaml:"edges"`
	Comment   string               `yaml:"comment"`
}

func (c *GraphConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if c.Graph == "" {
		return errors.New("Graph config: invalid `graph`")
	}
	return nil
}

func ParseConfigFromFile(filename string) *GraphConfig {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("Error: Config: ParseConfigFromFile: File: %v", err)
		return nil
	}

	var config GraphConfig
	if err := config.Parse(data); err != nil {
		log.Printf("Error: Config: ParseConfigFromFile: Parse: %v", err)
		return nil
	}

	return &config
}

// NewGraphFromConfig returns a new graph from existing input, such as from the
// existing graph, and a GraphConfig struct.
func (g *Graph) NewGraphFromConfig(config *GraphConfig, embdEtcd *EmbdEtcd, hostname string, noop bool) (*Graph, error) {

	var graph *Graph // new graph to return
	if g == nil {    // FIXME: how can we check for an empty graph?
		graph = NewGraph("Graph") // give graph a default name
	} else {
		graph = g.Copy() // same vertices, since they're pointers!
	}

	var lookup = make(map[string]map[string]*Vertex)

	//log.Printf("%+v", config) // debug

	// TODO: if defined (somehow)...
	graph.SetName(config.Graph) // set graph name

	var keep []*Vertex  // list of vertex which are the same in new graph
	var resources []Res // list of resources to export
	// use reflection to avoid duplicating code... better options welcome!
	value := reflect.Indirect(reflect.ValueOf(config.Resources))
	vtype := value.Type()
	for i := 0; i < vtype.NumField(); i++ { // number of fields in struct
		name := vtype.Field(i).Name // string of field name
		field := value.FieldByName(name)
		iface := field.Interface() // interface type of value
		slice := reflect.ValueOf(iface)
		// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
		kind := FirstToUpper(name)
		if DEBUG {
			log.Printf("Config: Processing: %v...", kind)
		}
		for j := 0; j < slice.Len(); j++ { // loop through resources of same kind
			x := slice.Index(j).Interface()
			res, ok := x.(Res) // convert to Res type
			if !ok {
				return nil, fmt.Errorf("Error: Config: Can't convert: %v of type: %T to Res.", x, x)
			}
			if noop {
				res.Meta().Noop = noop
			}
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]*Vertex)
			}
			// XXX: should we export based on a @@ prefix, or a metaparam
			// like exported => true || exported => (host pattern)||(other pattern?)
			if !strings.HasPrefix(res.GetName(), "@@") { // not exported resource
				// XXX: we don't have a way of knowing if any of the
				// metaparams are undefined, and as a result to set the
				// defaults that we want! I hate the go yaml parser!!!
				v := graph.GetVertexMatch(res)
				if v == nil { // no match found
					res.Init()
					v = NewVertex(res)
					graph.AddVertex(v) // call standalone in case not part of an edge
				}
				lookup[kind][res.GetName()] = v // used for constructing edges
				keep = append(keep, v)          // append

			} else if !noop { // do not export any resources if noop
				// store for addition to etcd storage...
				res.SetName(res.GetName()[2:]) //slice off @@
				res.setKind(kind)              // cheap init
				resources = append(resources, res)
			}
		}
	}
	// store in etcd
	if err := EtcdSetResources(embdEtcd, hostname, resources); err != nil {
		return nil, fmt.Errorf("Config: Could not export resources: %v", err)
	}

	// lookup from etcd
	var hostnameFilter []string // empty to get from everyone
	kindFilter := []string{}
	for _, t := range config.Collector {
		// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
		kind := FirstToUpper(t.Kind)
		kindFilter = append(kindFilter, kind)
	}
	// do all the graph look ups in one single step, so that if the etcd
	// database changes, we don't have a partial state of affairs...
	if len(kindFilter) > 0 { // if kindFilter is empty, don't need to do lookups!
		var err error
		resources, err = EtcdGetResources(embdEtcd, hostnameFilter, kindFilter)
		if err != nil {
			return nil, fmt.Errorf("Config: Could not collect resources: %v", err)
		}
	}
	for _, res := range resources {
		matched := false
		// see if we find a collect pattern that matches
		for _, t := range config.Collector {
			// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
			kind := FirstToUpper(t.Kind)
			// use t.Kind and optionally t.Pattern to collect from etcd storage
			log.Printf("Collect: %v; Pattern: %v", kind, t.Pattern)

			// XXX: expand to more complex pattern matching here...
			if res.Kind() != kind {
				continue
			}

			if matched {
				// we've already matched this resource, should we match again?
				log.Printf("Config: Warning: Matching %v[%v] again!", kind, res.GetName())
			}
			matched = true

			// collect resources but add the noop metaparam
			if noop {
				res.Meta().Noop = noop
			}

			if t.Pattern != "" { // XXX: simplistic for now
				res.CollectPattern(t.Pattern) // res.Dirname = t.Pattern
			}

			log.Printf("Collect: %v[%v]: collected!", kind, res.GetName())

			// XXX: similar to other resource add code:
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]*Vertex)
			}
			v := graph.GetVertexMatch(res)
			if v == nil { // no match found
				res.Init() // initialize go channels or things won't work!!!
				v = NewVertex(res)
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[kind][res.GetName()] = v // used for constructing edges
			keep = append(keep, v)          // append

			//break // let's see if another resource even matches
		}
	}

	// get rid of any vertices we shouldn't "keep" (that aren't in new graph)
	for _, v := range graph.GetVertices() {
		if !VertexContains(v, keep) {
			// wait for exit before starting new graph!
			v.SendEvent(eventExit, true, false)
			graph.DeleteVertex(v)
		}
	}

	for _, e := range config.Edges {
		if _, ok := lookup[FirstToUpper(e.From.Kind)]; !ok {
			return nil, fmt.Errorf("Can't find 'from' resource!")
		}
		if _, ok := lookup[FirstToUpper(e.To.Kind)]; !ok {
			return nil, fmt.Errorf("Can't find 'to' resource!")
		}
		if _, ok := lookup[FirstToUpper(e.From.Kind)][e.From.Name]; !ok {
			return nil, fmt.Errorf("Can't find 'from' name!")
		}
		if _, ok := lookup[FirstToUpper(e.To.Kind)][e.To.Name]; !ok {
			return nil, fmt.Errorf("Can't find 'to' name!")
		}
		graph.AddEdge(lookup[FirstToUpper(e.From.Kind)][e.From.Name], lookup[FirstToUpper(e.To.Kind)][e.To.Name], NewEdge(e.Name))
	}

	return graph, nil
}

// add edges to the vertex in a graph based on if it matches a uuid list
func (g *Graph) addEdgesByMatchingUUIDS(v *Vertex, uuids []ResUUID) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uuid, and see if it matches any vertex
	for _, uuid := range uuids {
		var found = false
		// uuid is a ResUUID object
		for _, vv := range g.GetVertices() { // search
			if v == vv { // skip self
				continue
			}
			if DEBUG {
				log.Printf("Compile: AutoEdge: Match: %v[%v] with UUID: %v[%v]", vv.Kind(), vv.GetName(), uuid.Kind(), uuid.GetName())
			}
			// we must match to an effective UUID for the resource,
			// that is to say, the name value of a res is a helpful
			// handle, but it is not necessarily a unique identity!
			// remember, resources can return multiple UUID's each!
			if UUIDExistsInUUIDs(uuid, vv.GetUUIDs()) {
				// add edge from: vv -> v
				if uuid.Reversed() {
					txt := fmt.Sprintf("AutoEdge: %v[%v] -> %v[%v]", vv.Kind(), vv.GetName(), v.Kind(), v.GetName())
					log.Printf("Compile: Adding %v", txt)
					g.AddEdge(vv, v, NewEdge(txt))
				} else { // edges go the "normal" way, eg: pkg resource
					txt := fmt.Sprintf("AutoEdge: %v[%v] -> %v[%v]", v.Kind(), v.GetName(), vv.Kind(), vv.GetName())
					log.Printf("Compile: Adding %v", txt)
					g.AddEdge(v, vv, NewEdge(txt))
				}
				found = true
				break
			}
		}
		result = append(result, found)
	}
	return result
}

// add auto edges to graph
func (g *Graph) AutoEdges() {
	log.Println("Compile: Adding AutoEdges...")
	for _, v := range g.GetVertices() { // for each vertexes autoedges
		if !v.Meta().AutoEdge { // is the metaparam true?
			continue
		}
		autoEdgeObj := v.AutoEdges()
		if autoEdgeObj == nil {
			log.Printf("%v[%v]: Config: No auto edges were found!", v.Kind(), v.GetName())
			continue // next vertex
		}

		for { // while the autoEdgeObj has more uuids to add...
			uuids := autoEdgeObj.Next() // get some!
			if uuids == nil {
				log.Printf("%v[%v]: Config: The auto edge list is empty!", v.Kind(), v.GetName())
				break // inner loop
			}
			if DEBUG {
				log.Println("Compile: AutoEdge: UUIDS:")
				for i, u := range uuids {
					log.Printf("Compile: AutoEdge: UUID%d: %v", i, u)
				}
			}

			// match and add edges
			result := g.addEdgesByMatchingUUIDS(v, uuids)

			// report back, and find out if we should continue
			if !autoEdgeObj.Test(result) {
				break
			}
		}
	}
}

// AutoGrouper is the required interface to implement for an autogroup algorithm
type AutoGrouper interface {
	// listed in the order these are typically called in...
	name() string                                  // friendly identifier
	init(*Graph) error                             // only call once
	vertexNext() (*Vertex, *Vertex, error)         // mostly algorithmic
	vertexCmp(*Vertex, *Vertex) error              // can we merge these ?
	vertexMerge(*Vertex, *Vertex) (*Vertex, error) // vertex merge fn to use
	edgeMerge(*Edge, *Edge) *Edge                  // edge merge fn to use
	vertexTest(bool) (bool, error)                 // call until false
}

// baseGrouper is the base type for implementing the AutoGrouper interface
type baseGrouper struct {
	graph    *Graph    // store a pointer to the graph
	vertices []*Vertex // cached list of vertices
	i        int
	j        int
	done     bool
}

// name provides a friendly name for the logs to see
func (ag *baseGrouper) name() string {
	return "baseGrouper"
}

// init is called only once and before using other AutoGrouper interface methods
// the name method is the only exception: call it any time without side effects!
func (ag *baseGrouper) init(g *Graph) error {
	if ag.graph != nil {
		return fmt.Errorf("The init method has already been called!")
	}
	ag.graph = g                               // pointer
	ag.vertices = ag.graph.GetVerticesSorted() // cache in deterministic order!
	ag.i = 0
	ag.j = 0
	if len(ag.vertices) == 0 { // empty graph
		ag.done = true
		return nil
	}
	return nil
}

// vertexNext is a simple iterator that loops through vertex (pair) combinations
// an intelligent algorithm would selectively offer only valid pairs of vertices
// these should satisfy logical grouping requirements for the autogroup designs!
// the desired algorithms can override, but keep this method as a base iterator!
func (ag *baseGrouper) vertexNext() (v1, v2 *Vertex, err error) {
	// this does a for v... { for w... { return v, w }} but stepwise!
	l := len(ag.vertices)
	if ag.i < l {
		v1 = ag.vertices[ag.i]
	}
	if ag.j < l {
		v2 = ag.vertices[ag.j]
	}

	// in case the vertex was deleted
	if !ag.graph.HasVertex(v1) {
		v1 = nil
	}
	if !ag.graph.HasVertex(v2) {
		v2 = nil
	}

	// two nested loops...
	if ag.j < l {
		ag.j++
	}
	if ag.j == l {
		ag.j = 0
		if ag.i < l {
			ag.i++
		}
		if ag.i == l {
			ag.done = true
		}
	}

	return
}

func (ag *baseGrouper) vertexCmp(v1, v2 *Vertex) error {
	if v1 == nil || v2 == nil {
		return fmt.Errorf("Vertex is nil!")
	}
	if v1 == v2 { // skip yourself
		return fmt.Errorf("Vertices are the same!")
	}
	if v1.Kind() != v2.Kind() { // we must group similar kinds
		// TODO: maybe future resources won't need this limitation?
		return fmt.Errorf("The two resources aren't the same kind!")
	}
	// someone doesn't want to group!
	if !v1.Meta().AutoGroup || !v2.Meta().AutoGroup {
		return fmt.Errorf("One of the autogroup flags is false!")
	}
	if v1.Res.IsGrouped() { // already grouped!
		return fmt.Errorf("Already grouped!")
	}
	if len(v2.Res.GetGroup()) > 0 { // already has children grouped!
		return fmt.Errorf("Already has groups!")
	}
	if !v1.Res.GroupCmp(v2.Res) { // resource groupcmp failed!
		return fmt.Errorf("The GroupCmp failed!")
	}
	return nil // success
}

func (ag *baseGrouper) vertexMerge(v1, v2 *Vertex) (v *Vertex, err error) {
	// NOTE: it's important to use w.Res instead of w, b/c
	// the w by itself is the *Vertex obj, not the *Res obj
	// which is contained within it! They both satisfy the
	// Res interface, which is why both will compile! :(
	err = v1.Res.GroupRes(v2.Res) // GroupRes skips stupid groupings
	return                        // success or fail, and no need to merge the actual vertices!
}

func (ag *baseGrouper) edgeMerge(e1, e2 *Edge) *Edge {
	return e1 // noop
}

// vertexTest processes the results of the grouping for the algorithm to know
// return an error if something went horribly wrong, and bool false to stop
func (ag *baseGrouper) vertexTest(b bool) (bool, error) {
	// NOTE: this particular baseGrouper version doesn't track what happens
	// because since we iterate over every pair, we don't care which merge!
	if ag.done {
		return false, nil
	}
	return true, nil
}

// TODO: this algorithm may not be correct in all cases. replace if needed!
type nonReachabilityGrouper struct {
	baseGrouper // "inherit" what we want, and reimplement the rest
}

func (ag *nonReachabilityGrouper) name() string {
	return "nonReachabilityGrouper"
}

// this algorithm relies on the observation that if there's a path from a to b,
// then they *can't* be merged (b/c of the existing dependency) so therefore we
// merge anything that *doesn't* satisfy this condition or that of the reverse!
func (ag *nonReachabilityGrouper) vertexNext() (v1, v2 *Vertex, err error) {
	for {
		v1, v2, err = ag.baseGrouper.vertexNext() // get all iterable pairs
		if err != nil {
			log.Fatalf("Error running autoGroup(vertexNext): %v", err)
		}

		if v1 != v2 { // ignore self cmp early (perf optimization)
			// if NOT reachable, they're viable...
			out1 := ag.graph.Reachability(v1, v2)
			out2 := ag.graph.Reachability(v2, v1)
			if len(out1) == 0 && len(out2) == 0 {
				return // return v1 and v2, they're viable
			}
		}

		// if we got here, it means we're skipping over this candidate!
		if ok, err := ag.baseGrouper.vertexTest(false); err != nil {
			log.Fatalf("Error running autoGroup(vertexTest): %v", err)
		} else if !ok {
			return nil, nil, nil // done!
		}

		// the vertexTest passed, so loop and try with a new pair...
	}
}

// autoGroup is the mechanical auto group "runner" that runs the interface spec
func (g *Graph) autoGroup(ag AutoGrouper) chan string {
	strch := make(chan string) // output log messages here
	go func(strch chan string) {
		strch <- fmt.Sprintf("Compile: Grouping: Algorithm: %v...", ag.name())
		if err := ag.init(g); err != nil {
			log.Fatalf("Error running autoGroup(init): %v", err)
		}

		for {
			var v, w *Vertex
			v, w, err := ag.vertexNext() // get pair to compare
			if err != nil {
				log.Fatalf("Error running autoGroup(vertexNext): %v", err)
			}
			merged := false
			// save names since they change during the runs
			vStr := fmt.Sprintf("%s", v) // valid even if it is nil
			wStr := fmt.Sprintf("%s", w)

			if err := ag.vertexCmp(v, w); err != nil { // cmp ?
				if DEBUG {
					strch <- fmt.Sprintf("Compile: Grouping: !GroupCmp for: %s into %s", wStr, vStr)
				}

				// remove grouped vertex and merge edges (res is safe)
			} else if err := g.VertexMerge(v, w, ag.vertexMerge, ag.edgeMerge); err != nil { // merge...
				strch <- fmt.Sprintf("Compile: Grouping: !VertexMerge for: %s into %s", wStr, vStr)

			} else { // success!
				strch <- fmt.Sprintf("Compile: Grouping: Success for: %s into %s", wStr, vStr)
				merged = true // woo
			}

			// did these get used?
			if ok, err := ag.vertexTest(merged); err != nil {
				log.Fatalf("Error running autoGroup(vertexTest): %v", err)
			} else if !ok {
				break // done!
			}
		}

		close(strch)
		return
	}(strch) // call function
	return strch
}

// AutoGroup runs the auto grouping on the graph and prints out log messages
func (g *Graph) AutoGroup() {
	// receive log messages from channel...
	// this allows test cases to avoid printing them when they're unwanted!
	// TODO: this algorithm may not be correct in all cases. replace if needed!
	for str := range g.autoGroup(&nonReachabilityGrouper{}) {
		log.Println(str)
	}
}
