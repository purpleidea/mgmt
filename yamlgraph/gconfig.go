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

// ResourceV1Data are the parameters for resource V1 format
type ResourceV1Data struct {
	Name string               `yaml:"name"`
	Meta resources.MetaParams `yaml:"meta"`
}

// ResourceV1 is the object that unmarshalls V1 resources
type ResourceV1 struct {
	ResourceV1Data
	unmarshal func(interface{}) error
	resource  resources.Res
}

// ResourcesV1 is the object that unmarshalls list of V1 resources
type ResourcesV1 struct {
	Resources map[string][]ResourceV1 `yaml:"resources"`
}

// ResourceV2Data are the parameters for resource V2 format
type ResourceV2Data struct {
	Name   string           `yaml:"name"`
	Kind   string           `yaml:"kind"`
	Params ResourceV2Params `yaml:"params"`
	Before []string         `yaml:"before"`
	After  []string         `yaml:"after"`
}

// ResourceV2 is the object that unmarshalls V2 resources
type ResourceV2 struct {
	ResourceV2Data
	resource resources.Res
}

// ResourceV2Params is the object that unmarshalls V2 resource params
type ResourceV2Params struct {
	res *ResourceV2
}

// ResourcesV2 is the object that unmarshalls list of V2 resources
type ResourcesV2 struct {
	Resources []ResourceV2 `yaml:"resources"`
}

// GraphConfigData contains the graph data for GraphConfig
type GraphConfigData struct {
	Version   int                  `yaml:"version"`
	Graph     string               `yaml:"graph"`
	Collector []collectorResConfig `yaml:"collect"`
	Edges     []Edge               `yaml:"edges"`
	Comment   string               `yaml:"comment"`
	Remote    string               `yaml:"remote"`
}

// GraphConfig is the data structure that describes a single graph to run.
type GraphConfig struct {
	GraphConfigData
	ResList []resources.Res
}

// UnmarshalYAML unmarshalls the complete graph.
func (c *GraphConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal the graph data, except the resources
	err := unmarshal(&c.GraphConfigData)
	if err != nil {
		return err
	}

	if c.Version <= 1 {
		// Unmarshal resources version 1
		var list ResourcesV1
		list.Resources = map[string][]ResourceV1{}
		err = unmarshal(&list)
		if err != nil {
			return err
		}

		// Finish unmarshalling by giving to each resource its kind
		// and store each resource in the graph
		for kind, resList := range list.Resources {
			for _, res := range resList {
				err := res.Decode(kind)
				if err != nil {
					return err
				}
				c.ResList = append(c.ResList, res.resource)
			}
		}

		return nil
	} else if c.Version <= 2 {
		// Unmarshal resources version 2
		var list ResourcesV2
		err = unmarshal(&list)
		if err != nil {
			return err
		}

		// Save resources to graph
		for _, res := range list.Resources {
			c.ResList = append(c.ResList, res.resource)
		}

		// Collect edges specified in resources
		var edgeList []Edge
		for _, res := range list.Resources {
			edges, err := res.Edges()
			if err != nil {
				return fmt.Errorf("Edge error in resource %s \"%s\": %s", res.Kind, res.Name, err.Error())
			}
			edgeList = append(edgeList, edges...)
		}

		// Collect edge names specified outside the resources
		// used to name edges while avoiding edge name collisions
		edgeNames := map[string]bool{}
		for _, e := range c.Edges {
			edgeNames[e.Name] = true
		}

		// Generate a name for each edge embedded in a resource
		// and save them in the graph
		var n int
		for _, e := range edgeList {
			for e.Name == "" && edgeNames[e.Name] {
				e.Name = fmt.Sprintf("edge-%d", n)
				n = n + 1
			}
			c.Edges = append(c.Edges, e)
		}

		return nil
	} else {
		// Later version
		return fmt.Errorf("Graph version %d not supported", c.Version)
	}
}

// UnmarshalYAML is the first stage for unmarshaling of V1 resources.
func (r *ResourceV1) UnmarshalYAML(unmarshal func(interface{}) error) error {
	r.unmarshal = unmarshal
	return unmarshal(&r.ResourceV1Data)
}

// Decode is the second stage for unmarshaling of V1 resources (knowing their
// kind).
func (r *ResourceV1) Decode(kind string) (err error) {
	r.resource, err = resources.NewEmptyNamedResource(kind)
	if err != nil {
		return err
	}

	err = r.unmarshal(r.resource)
	if err != nil {
		return err
	}

	// Set resource name, meta and kind
	r.resource.SetName(r.Name)
	// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
	r.resource.SetKind(util.FirstToUpper(kind))
	meta := r.resource.Meta()
	*meta = r.Meta
	return
}

// decodeEdge decodes an edge embedded within a resource.
// notify is true if s starts with '*'
// vertex contains the other resource kind and name (separated by spaces)
// ok is false on error
func decodeEdge(s string) (notify bool, vertex Vertex, ok bool) {
	if len(s) > 0 && s[0] == '*' {
		notify = true
		s = s[1:]
	}
	sub := strings.SplitN(s, " ", 2)
	if len(sub) != 2 {
		return
	}
	vertex = Vertex{Kind: sub[0], Name: sub[1]}
	ok = true
	return
}

// Edges decodes edges embedded within a resource in the before and after
// lists.
func (r *ResourceV2) Edges() ([]Edge, error) {
	var res []Edge
	for _, s := range r.Before {
		notify, vertex, ok := decodeEdge(s)
		if !ok {
			return res, fmt.Errorf("Invalid before edge \"%s\", expected \"[*]<kind> <name>\"", s)
		}
		res = append(res, Edge{
			Notify: notify,
			From:   Vertex{Kind: r.Kind, Name: r.Name},
			To:     vertex,
		})
	}
	for _, s := range r.After {
		notify, vertex, ok := decodeEdge(s)
		if !ok {
			return res, fmt.Errorf("Invalid after edge \"%s\", expected \"[*]<kind> <name>\"", s)
		}
		res = append(res, Edge{
			Notify: notify,
			From:   vertex,
			To:     Vertex{Kind: r.Kind, Name: r.Name},
		})
	}
	return res, nil
}

// UnmarshalYAML unmarshalls the V2 resource.
func (r *ResourceV2) UnmarshalYAML(unmarshal func(interface{}) error) error {
	r.Params.res = r
	r.resource = nil

	err := unmarshal(&r.ResourceV2Data)
	if err != nil {
		return err
	}

	// Resource kind specified but no params are included
	// Construct an empty resource
	if r.Kind != "" && r.resource == nil {
		r.resource, err = resources.NewEmptyNamedResource(r.Kind)
		if err != nil {
			return err
		}

		err = unmarshal(r.resource)
		if err != nil {
			return err
		}
	}

	if r.Kind == "" {
		return fmt.Errorf("Resource kind not specified")
	}

	// Set resource name and meta and kind
	r.resource.SetName(r.Name)
	// XXX: should we just drop these everywhere and have the kind strings be all lowercase?
	r.resource.SetKind(util.FirstToUpper(r.Kind))
	return unmarshal(r.resource.Meta())
}

// UnmarshalYAML unmarshalls the params attributes.
func (p *ResourceV2Params) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var err error
	p.res.resource, err = resources.NewEmptyNamedResource(p.res.Kind)
	if err != nil {
		return err
	}

	err = unmarshal(p.res.resource)
	return err
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

	// Resources V2
	for _, res := range c.ResList {
		kind := res.Kind()
		if _, exists := lookup[kind]; !exists {
			lookup[kind] = make(map[string]*pgraph.Vertex)
		}
		// XXX: should we export based on a @@ prefix, or a metaparam
		// like exported => true || exported => (host pattern)||(other pattern?)
		if !strings.HasPrefix(res.GetName(), "@@") { // not exported resource
			v := graph.GetVertexMatch(res)
			if v == nil { // no match found
				res.Init()
				v = pgraph.NewVertex(res)
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[kind][res.GetName()] = v // used for constructing edges
			keep = append(keep, v)          // append

		} else if !noop { // do not export any resources if noop
			// store for addition to backend storage...
			res.SetName(res.GetName()[2:]) // slice off @@
			res.SetKind(kind)              // cheap init
			resourceList = append(resourceList, res)
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
			v := graph.GetVertexMatch(res)
			if v == nil { // no match found
				res.Init() // initialize go channels or things won't work!!!
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
