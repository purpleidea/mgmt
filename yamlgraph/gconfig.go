// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package yamlgraph provides the facilities for loading a graph from a yaml file.
package yamlgraph

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"

	errwrap "github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type collectorResConfig struct {
	Kind    string `yaml:"kind"`
	Pattern string `yaml:"pattern"` // XXX: not implemented
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
	AwsEc2   []*resources.AwsEc2Res   `yaml:"aws:ec2"`
	Exec     []*resources.ExecRes     `yaml:"exec"`
	File     []*resources.FileRes     `yaml:"file"`
	Graph    []*resources.GraphRes    `yaml:"graph"`
	Group    []*resources.GroupRes    `yaml:"group"`
	Hostname []*resources.HostnameRes `yaml:"hostname"`
	KV       []*resources.KVRes       `yaml:"kv"`
	Msg      []*resources.MsgRes      `yaml:"msg"`
	Net      []*resources.NetRes      `yaml:"net"`
	Noop     []*resources.NoopRes     `yaml:"noop"`
	Nspawn   []*resources.NspawnRes   `yaml:"nspawn"`
	Password []*resources.PasswordRes `yaml:"password"`
	Pkg      []*resources.PkgRes      `yaml:"pkg"`
	Print    []*resources.PrintRes    `yaml:"print"`
	Svc      []*resources.SvcRes      `yaml:"svc"`
	Test     []*resources.TestRes     `yaml:"test"`
	Timer    []*resources.TimerRes    `yaml:"timer"`
	User     []*resources.UserRes     `yaml:"user"`
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
		return errors.New("graph config: invalid graph")
	}
	return nil
}

// NewGraphFromConfig transforms a GraphConfig struct into a new graph.
// FIXME: remove any possibly left over, now obsolete graph diff code from here!
func (c *GraphConfig) NewGraphFromConfig(hostname string, world resources.World, noop bool) (*pgraph.Graph, error) {
	// hostname is the uuid for the host

	var graph *pgraph.Graph // new graph to return
	var err error
	graph, err = pgraph.NewGraph("Graph") // give graph a default name
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not run NewGraphFromConfig() properly")
	}

	var lookup = make(map[string]map[string]pgraph.Vertex)

	//log.Printf("%+v", config) // debug

	// TODO: if defined (somehow)...
	graph.SetName(c.Graph) // set graph name

	var keep []pgraph.Vertex         // list of vertex which are the same in new graph
	var resourceList []resources.Res // list of resources to export
	// use reflection to avoid duplicating code... better options welcome!
	value := reflect.Indirect(reflect.ValueOf(c.Resources))
	vtype := value.Type()
	for i := 0; i < vtype.NumField(); i++ { // number of fields in struct
		name := vtype.Field(i).Name // string of field name
		field := value.FieldByName(name)
		iface := field.Interface() // interface type of value
		slice := reflect.ValueOf(iface)
		kind := strings.ToLower(name)
		for j := 0; j < slice.Len(); j++ { // loop through resources of same kind
			x := slice.Index(j).Interface()
			res, ok := x.(resources.Res) // convert to Res type
			if !ok {
				return nil, fmt.Errorf("Config: Error: Can't convert: %v of type: %T to Res", x, x)
			}
			res.SetKind(kind) // cheap init
			//if noop { // now done in mgmtmain
			//	res.Meta().Noop = noop
			//}
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]pgraph.Vertex)
			}
			// XXX: should we export based on a @@ prefix, or a metaparam
			// like exported => true || exported => (host pattern)||(other pattern?)
			if !strings.HasPrefix(res.GetName(), "@@") { // not exported resource
				fn := func(v pgraph.Vertex) (bool, error) {
					return resources.VtoR(v).Compare(res), nil
				}
				v, err := graph.VertexMatchFn(fn)
				if err != nil {
					return nil, errwrap.Wrapf(err, "could not VertexMatchFn() resource")
				}
				if v == nil { // no match found
					v = res            // a standalone res can be a vertex
					graph.AddVertex(v) // call standalone in case not part of an edge
				}
				lookup[kind][res.GetName()] = v // used for constructing edges
				keep = append(keep, v)          // append

			} else if !noop { // do not export any resources if noop
				// store for addition to backend storage...
				res.SetName(res.GetName()[2:]) //slice off @@
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
		kind := strings.ToLower(t.Kind)
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
			kind := strings.ToLower(t.Kind)
			// use t.Kind and optionally t.Pattern to collect from storage
			log.Printf("Collect: %v; Pattern: %v", kind, t.Pattern)

			// XXX: expand to more complex pattern matching here...
			if res.GetKind() != kind {
				continue
			}

			if matched {
				// we've already matched this resource, should we match again?
				log.Printf("Config: Warning: Matching %s again!", res)
			}
			matched = true

			// collect resources but add the noop metaparam
			//if noop { // now done in mgmtmain
			//	res.Meta().Noop = noop
			//}

			if t.Pattern != "" { // XXX: simplistic for now
				res.CollectPattern(t.Pattern) // res.Dirname = t.Pattern
			}

			log.Printf("Collect: %s: collected!", res)

			// XXX: similar to other resource add code:
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]pgraph.Vertex)
			}

			fn := func(v pgraph.Vertex) (bool, error) {
				return resources.VtoR(v).Compare(res), nil
			}
			v, err := graph.VertexMatchFn(fn)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not VertexMatchFn() resource")
			}
			if v == nil { // no match found
				v = res            // a standalone res can be a vertex
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[kind][res.GetName()] = v // used for constructing edges
			keep = append(keep, v)          // append

			//break // let's see if another resource even matches
		}
	}

	for _, e := range c.Edges {
		if _, ok := lookup[strings.ToLower(e.From.Kind)]; !ok {
			return nil, fmt.Errorf("can't find 'from' kind: %s", e.From.Kind)
		}
		if _, ok := lookup[strings.ToLower(e.To.Kind)]; !ok {
			return nil, fmt.Errorf("can't find 'to' kind: %s", e.To.Kind)
		}
		if _, ok := lookup[strings.ToLower(e.From.Kind)][e.From.Name]; !ok {
			return nil, fmt.Errorf("can't find 'from' name: %s", e.From.Name)
		}
		if _, ok := lookup[strings.ToLower(e.To.Kind)][e.To.Name]; !ok {
			return nil, fmt.Errorf("can't find 'to' name: %s", e.To.Name)
		}
		from := lookup[strings.ToLower(e.From.Kind)][e.From.Name]
		to := lookup[strings.ToLower(e.To.Kind)][e.To.Name]
		edge := &resources.Edge{
			Name:   e.Name,
			Notify: e.Notify,
		}
		graph.AddEdge(from, to, edge)
	}

	return graph, nil
}

// ParseConfigFromFile takes a filename and returns the graph config structure.
func ParseConfigFromFile(data []byte) *GraphConfig {
	var config GraphConfig
	if err := config.Parse(data); err != nil {
		log.Printf("Config: Error: ParseConfigFromFile: Parse: %v", err)
		return nil
	}

	return &config
}
