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

// Package yamlgraph provides the facilities for loading a graph from a yaml file.
package yamlgraph

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strings"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"
	"github.com/purpleidea/mgmt/util"

	"gopkg.in/yaml.v2"
)

type collectorResConfig struct {
	Kind    string `yaml:"kind"`
	Pattern string `yaml:"pattern"` // XXX: Not Implemented
}

// Vertex is the data structure of a vertex.
type Vertex struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

// Edge is the data structure of an edge.
type Edge struct {
	Name   string `yaml:"name"`
	From   Vertex `yaml:"from"`
	To     Vertex `yaml:"to"`
	Notify bool   `yaml:"notify"`
}

// Resources is the data structure of the set of resources.
type Resources struct {
	// in alphabetical order
	Augeas   []*resources.AugeasRes   `yaml:"augeas"`
	Exec     []*resources.ExecRes     `yaml:"exec"`
	File     []*resources.FileRes     `yaml:"file"`
	Hostname []*resources.HostnameRes `yaml:"hostname"`
	Msg      []*resources.MsgRes      `yaml:"msg"`
	Noop     []*resources.NoopRes     `yaml:"noop"`
	Nspawn   []*resources.NspawnRes   `yaml:"nspawn"`
	Password []*resources.PasswordRes `yaml:"password"`
	Pkg      []*resources.PkgRes      `yaml:"pkg"`
	Svc      []*resources.SvcRes      `yaml:"svc"`
	Timer    []*resources.TimerRes    `yaml:"timer"`
	Virt     []*resources.VirtRes     `yaml:"virt"`
}

// GraphConfig is the data structure that describes a single graph to run.
type GraphConfig struct {
	Graph     string               `yaml:"graph"`
	Resources Resources            `yaml:"resources"`
	Collector []collectorResConfig `yaml:"collect"`
	Edges     []Edge               `yaml:"edges"`
	Comment   string               `yaml:"comment"`
	Remote    string               `yaml:"remote"`
}

// Parse parses a data stream into the graph structure.
func (c *GraphConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if c.Graph == "" {
		return errors.New("Graph config: invalid `graph`")
	}
	return nil
}

// NewGraphFromConfig transforms a GraphConfig struct into a new graph.
// FIXME: remove any possibly left over, now obsolete graph diff code from here!
func (c *GraphConfig) NewGraphFromConfig(hostname string, world gapi.World, noop bool) (*pgraph.Graph, error) {
	// hostname is the uuid for the host

	var graph *pgraph.Graph          // new graph to return
	graph = pgraph.NewGraph("Graph") // give graph a default name

	var lookup = make(map[string]map[string]*pgraph.Vertex)

	//log.Printf("%+v", config) // debug

	// TODO: if defined (somehow)...
	graph.SetName(c.Graph) // set graph name

	var keep []*pgraph.Vertex        // list of vertex which are the same in new graph
	var resourceList []resources.Res // list of resources to export
	// use reflection to avoid duplicating code... better options welcome!
	value := reflect.Indirect(reflect.ValueOf(c.Resources))
	vtype := value.Type()
	for i := 0; i < vtype.NumField(); i++ { // number of fields in struct
		name := vtype.Field(i).Name // string of field name
		field := value.FieldByName(name)
		iface := field.Interface() // interface type of value
		slice := reflect.ValueOf(iface)
		// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
		kind := util.FirstToUpper(name)
		for j := 0; j < slice.Len(); j++ { // loop through resources of same kind
			x := slice.Index(j).Interface()
			res, ok := x.(resources.Res) // convert to Res type
			if !ok {
				return nil, fmt.Errorf("Config: Error: Can't convert: %v of type: %T to Res.", x, x)
			}
			//if noop { // now done in mgmtmain
			//	res.Meta().Noop = noop
			//}
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]*pgraph.Vertex)
			}
			// XXX: should we export based on a @@ prefix, or a metaparam
			// like exported => true || exported => (host pattern)||(other pattern?)
			if !strings.HasPrefix(res.GetName(), "@@") { // not exported resource
				v := graph.CompareMatch(res)
				if v == nil { // no match found
					v = pgraph.NewVertex(res)
					graph.AddVertex(v) // call standalone in case not part of an edge
				}
				lookup[kind][res.GetName()] = v // used for constructing edges
				keep = append(keep, v)          // append

			} else if !noop { // do not export any resources if noop
				// store for addition to backend storage...
				res.SetName(res.GetName()[2:]) //slice off @@
				res.SetKind(kind)              // cheap init
				resourceList = append(resourceList, res)
			}
		}
	}
	// store in backend (usually etcd)
	if err := world.ResExport(resourceList); err != nil {
		return nil, fmt.Errorf("Config: Could not export resources: %v", err)
	}

	// lookup from backend (usually etcd)
	var hostnameFilter []string // empty to get from everyone
	kindFilter := []string{}
	for _, t := range c.Collector {
		// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
		kind := util.FirstToUpper(t.Kind)
		kindFilter = append(kindFilter, kind)
	}
	// do all the graph look ups in one single step, so that if the backend
	// database changes, we don't have a partial state of affairs...
	if len(kindFilter) > 0 { // if kindFilter is empty, don't need to do lookups!
		var err error
		resourceList, err = world.ResCollect(hostnameFilter, kindFilter)
		if err != nil {
			return nil, fmt.Errorf("Config: Could not collect resources: %v", err)
		}
	}
	for _, res := range resourceList {
		matched := false
		// see if we find a collect pattern that matches
		for _, t := range c.Collector {
			// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
			kind := util.FirstToUpper(t.Kind)
			// use t.Kind and optionally t.Pattern to collect from storage
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
			//if noop { // now done in mgmtmain
			//	res.Meta().Noop = noop
			//}

			if t.Pattern != "" { // XXX: simplistic for now
				res.CollectPattern(t.Pattern) // res.Dirname = t.Pattern
			}

			log.Printf("Collect: %v[%v]: collected!", kind, res.GetName())

			// XXX: similar to other resource add code:
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]*pgraph.Vertex)
			}
			v := graph.CompareMatch(res)
			if v == nil { // no match found
				v = pgraph.NewVertex(res)
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[kind][res.GetName()] = v // used for constructing edges
			keep = append(keep, v)          // append

			//break // let's see if another resource even matches
		}
	}

	for _, e := range c.Edges {
		if _, ok := lookup[util.FirstToUpper(e.From.Kind)]; !ok {
			return nil, fmt.Errorf("Can't find 'from' resource!")
		}
		if _, ok := lookup[util.FirstToUpper(e.To.Kind)]; !ok {
			return nil, fmt.Errorf("Can't find 'to' resource!")
		}
		if _, ok := lookup[util.FirstToUpper(e.From.Kind)][e.From.Name]; !ok {
			return nil, fmt.Errorf("Can't find 'from' name!")
		}
		if _, ok := lookup[util.FirstToUpper(e.To.Kind)][e.To.Name]; !ok {
			return nil, fmt.Errorf("Can't find 'to' name!")
		}
		from := lookup[util.FirstToUpper(e.From.Kind)][e.From.Name]
		to := lookup[util.FirstToUpper(e.To.Kind)][e.To.Name]
		edge := pgraph.NewEdge(e.Name)
		edge.Notify = e.Notify
		graph.AddEdge(from, to, edge)
	}

	return graph, nil
}

// ParseConfigFromFile takes a filename and returns the graph config structure.
func ParseConfigFromFile(filename string) *GraphConfig {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("Config: Error: ParseConfigFromFile: File: %v", err)
		return nil
	}

	var config GraphConfig
	if err := config.Parse(data); err != nil {
		log.Printf("Config: Error: ParseConfigFromFile: Parse: %v", err)
		return nil
	}

	return &config
}
