// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"

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

// Resources is the object that unmarshalls list of resources.
type Resources struct {
	Resources map[string][]Resource `yaml:"resources"`
}

// GraphConfigData contains the graph data for GraphConfig.
type GraphConfigData struct {
	Graph     string               `yaml:"graph"`
	Collector []collectorResConfig `yaml:"collect"`
	Edges     []Edge               `yaml:"edges"`
	Comment   string               `yaml:"comment"`
}

// ResourceData are the parameters for resource format.
type ResourceData struct {
	Name string `yaml:"name"`
}

// Resource is the object that unmarshalls resources.
type Resource struct {
	ResourceData

	resource  engine.Res
	unmarshal func(interface{}) error
}

// UnmarshalYAML is the first stage for unmarshaling of resources.
func (r *Resource) UnmarshalYAML(unmarshal func(interface{}) error) error {
	r.unmarshal = unmarshal
	return unmarshal(&r.ResourceData)
}

// Decode is the second stage for unmarshaling of resources (knowing their
// kind).
func (r *Resource) Decode(kind string) (err error) {
	if kind == "" {
		return fmt.Errorf("can't set empty kind") // bug?
	}
	r.resource, err = engine.NewResource(kind)
	if err != nil {
		return err
	}

	// i think this erases the `SetKind` that happens with the NewResource
	// so as a result, we need to do it again below... this is a hack...
	err = r.unmarshal(r.resource)
	if err != nil {
		return err
	}

	// set resource name and kind
	r.resource.SetName(r.Name)
	r.resource.SetKind(kind)
	// TODO: I don't think meta is getting unmarshalled properly anymore
	return
}

// GraphConfig is the data structure that describes a single graph to run.
type GraphConfig struct {
	GraphConfigData
	ResList []engine.Res

	Debug bool
	Logf  func(format string, v ...interface{})
}

// NewGraphConfigFromFile takes data and returns the graph config structure.
func NewGraphConfigFromFile(data []byte, debug bool, logf func(format string, v ...interface{})) (*GraphConfig, error) {
	var config GraphConfig
	config.Debug = debug
	config.Logf = logf
	if err := config.Parse(data); err != nil {
		return nil, errwrap.Wrapf(err, "parse error")
	}

	return &config, nil
}

// UnmarshalYAML unmarshalls the complete graph.
func (obj *GraphConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal the graph data, except the resources
	if err := unmarshal(&obj.GraphConfigData); err != nil {
		return err
	}

	// unmarshal resources
	var list Resources
	list.Resources = map[string][]Resource{}
	if err := unmarshal(&list); err != nil {
		return err
	}

	// finish unmarshalling by giving to each resource its kind
	// and store each resource in the graph
	for kind, resList := range list.Resources {
		for _, res := range resList {
			err := res.Decode(kind)
			if err != nil {
				return err
			}
			obj.ResList = append(obj.ResList, res.resource)
		}
	}

	return nil
}

// Parse parses a data stream into the graph structure.
func (obj *GraphConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, obj); err != nil {
		return err
	}
	if obj.Graph == "" {
		return fmt.Errorf("invalid graph")
	}
	return nil
}

// NewGraphFromConfig transforms a GraphConfig struct into a new graph.
// FIXME: remove any possibly left over, now obsolete graph diff code from here!
// TODO: add a timeout to replace context.TODO()
func (obj *GraphConfig) NewGraphFromConfig(hostname string, world engine.World, noop bool) (*pgraph.Graph, error) {
	// hostname is the uuid for the host

	var graph *pgraph.Graph // new graph to return
	var err error
	graph, err = pgraph.NewGraph("Graph") // give graph a default name
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not run NewGraphFromConfig() properly")
	}

	var lookup = make(map[string]map[string]pgraph.Vertex)

	// TODO: if defined (somehow)...
	graph.SetName(obj.Graph) // set graph name

	var keep []pgraph.Vertex      // list of vertex which are the same in new graph
	var resourceList []engine.Res // list of resources to export

	// Resources
	for _, res := range obj.ResList {
		kind := res.Kind()
		if kind == "" {
			return nil, fmt.Errorf("resource has an empty kind") // bug?
		}
		if _, exists := lookup[kind]; !exists {
			lookup[kind] = make(map[string]pgraph.Vertex)
		}
		// XXX: should we export based on a @@ prefix, or a metaparam
		// like exported => true || exported => (host pattern)||(other pattern?)
		if !strings.HasPrefix(res.Name(), "@@") { // not exported resource
			fn := func(v pgraph.Vertex) (bool, error) {
				r, ok := v.(engine.Res)
				if !ok {
					return false, fmt.Errorf("not a Res")
				}
				return engine.ResCmp(r, res) == nil, nil
			}
			v, err := graph.VertexMatchFn(fn)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not VertexMatchFn() resource")
			}
			if v == nil { // no match found
				v = res            // a standalone res can be a vertex
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[kind][res.Name()] = v // used for constructing edges
			keep = append(keep, v)       // append

		} else if !noop { // do not export any resources if noop
			// store for addition to backend storage...
			res.SetName(res.Name()[2:]) // slice off @@
			resourceList = append(resourceList, res)
		}
	}

	// store in backend (usually etcd)
	if err := world.ResExport(context.TODO(), resourceList); err != nil {
		return nil, fmt.Errorf("config: could not export resources: %v", err)
	}

	// lookup from backend (usually etcd)
	var hostnameFilter []string // empty to get from everyone
	kindFilter := []string{}
	for _, t := range obj.Collector {
		kind := strings.ToLower(t.Kind)
		kindFilter = append(kindFilter, kind)
	}
	// do all the graph look ups in one single step, so that if the backend
	// database changes, we don't have a partial state of affairs...
	if len(kindFilter) > 0 { // if kindFilter is empty, don't need to do lookups!
		var err error
		resourceList, err = world.ResCollect(context.TODO(), hostnameFilter, kindFilter)
		if err != nil {
			return nil, fmt.Errorf("config: could not collect resources: %v", err)
		}
	}
	for _, res := range resourceList {
		matched := false
		// see if we find a collect pattern that matches
		for _, t := range obj.Collector {
			kind := strings.ToLower(t.Kind)
			// use t.Kind and optionally t.Pattern to collect from storage
			obj.Logf("collect: %s; pattern: %v", kind, t.Pattern)

			// XXX: expand to more complex pattern matching here...
			if res.Kind() != kind {
				continue
			}

			if matched {
				// we've already matched this resource, should we match again?
				obj.Logf("warning: matching %s again!", res)
			}
			matched = true

			// collect resources but add the noop metaparam
			//if noop { // now done in main lib
			//	res.MetaParams().Noop = noop
			//}

			if t.Pattern != "" { // XXX: simplistic for now
				if xres, ok := res.(engine.CollectableRes); ok {
					xres.CollectPattern(t.Pattern) // res.Dirname = t.Pattern
				}
			}

			obj.Logf("collected: %s", res)

			// XXX: similar to other resource add code:
			if _, exists := lookup[kind]; !exists {
				lookup[kind] = make(map[string]pgraph.Vertex)
			}

			// FIXME: do we need to expand this match function with
			// the additional Cmp properties found in Meta, etc...?
			fn := func(v pgraph.Vertex) (bool, error) {
				r, ok := v.(engine.Res)
				if !ok {
					return false, fmt.Errorf("not a Res")
				}
				return engine.ResCmp(r, res) == nil, nil
			}
			v, err := graph.VertexMatchFn(fn)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not VertexMatchFn() resource")
			}
			if v == nil { // no match found
				v = res            // a standalone res can be a vertex
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[kind][res.Name()] = v // used for constructing edges
			keep = append(keep, v)       // append

			//break // let's see if another resource even matches
		}
	}

	for _, e := range obj.Edges {
		if _, ok := lookup[strings.ToLower(e.From.Kind)]; !ok {
			return nil, fmt.Errorf("can't find 'from' resource")
		}
		if _, ok := lookup[strings.ToLower(e.To.Kind)]; !ok {
			return nil, fmt.Errorf("can't find 'to' resource")
		}
		if _, ok := lookup[strings.ToLower(e.From.Kind)][e.From.Name]; !ok {
			return nil, fmt.Errorf("can't find 'from' name")
		}
		if _, ok := lookup[strings.ToLower(e.To.Kind)][e.To.Name]; !ok {
			return nil, fmt.Errorf("can't find 'to' name")
		}
		from := lookup[strings.ToLower(e.From.Kind)][e.From.Name]
		to := lookup[strings.ToLower(e.To.Kind)][e.To.Name]
		edge := &engine.Edge{
			Name:   e.Name,
			Notify: e.Notify,
		}
		graph.AddEdge(from, to, edge)
	}

	return graph, nil
}
