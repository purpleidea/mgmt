// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

package hcl

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/hashicorp/hil"
	"github.com/purpleidea/mgmt/gapi"
	hv "github.com/purpleidea/mgmt/hil"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"
)

type collectorResConfig struct {
	Kind    string
	Pattern string
}

// Config defines the structure of the hcl config.
type Config struct {
	Resources []*Resource
	Edges     []*Edge
	Collector []collectorResConfig
}

// vertex is the data structure of a vertex.
type vertex struct {
	Kind string `hcl:"kind"`
	Name string `hcl:"name"`
}

// Edge defines an edge in hcl.
type Edge struct {
	Name   string
	From   vertex
	To     vertex
	Notify bool
}

// Resources define the state for resources.
type Resources struct {
	Resources []resources.Res
}

// Resource ...
type Resource struct {
	Name     string
	Kind     string
	resource resources.Res
	Meta     resources.MetaParams
	deps     []*Edge
	rcv      map[string]*hv.ResourceVariable
}

type key struct {
	kind, name string
}

func graphFromConfig(c *Config, data gapi.Data) (*pgraph.Graph, error) {
	var graph *pgraph.Graph
	var err error

	graph, err = pgraph.NewGraph("Graph")
	if err != nil {
		return nil, fmt.Errorf("unable to create graph from config: %s", err)
	}

	lookup := make(map[key]pgraph.Vertex)

	var keep []pgraph.Vertex
	var resourceList []resources.Res

	log.Printf("HCL: parsing %d resources", len(c.Resources))
	for _, r := range c.Resources {
		res := r.resource
		kind := r.resource.GetKind()

		log.Printf("HCL: resource \"%s\" \"%s\"", kind, r.Name)
		if !strings.HasPrefix(res.GetName(), "@@") {
			fn := func(v pgraph.Vertex) (bool, error) {
				return resources.VtoR(v).Compare(res), nil
			}
			v, err := graph.VertexMatchFn(fn)
			if err != nil {
				return nil, fmt.Errorf("could not match vertex: %s", err)
			}
			if v == nil {
				v = res
				graph.AddVertex(v)
			}
			lookup[key{kind, res.GetName()}] = v
			keep = append(keep, v)
		} else if !data.Noop {
			res.SetName(res.GetName()[2:])
			res.SetKind(kind)
			resourceList = append(resourceList, res)
		}
	}

	// store in backend (usually etcd)
	if err := data.World.ResExport(resourceList); err != nil {
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
		resourceList, err = data.World.ResCollect(hostnameFilter, kindFilter)
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
			// if _, exists := lookup[kind]; !exists {
			// 	lookup[kind] = make(map[string]pgraph.Vertex)
			// }

			fn := func(v pgraph.Vertex) (bool, error) {
				return resources.VtoR(v).Compare(res), nil
			}
			v, err := graph.VertexMatchFn(fn)
			if err != nil {
				return nil, fmt.Errorf("could not VertexMatchFn() resource: %s", err)
			}
			if v == nil { // no match found
				v = res            // a standalone res can be a vertex
				graph.AddVertex(v) // call standalone in case not part of an edge
			}
			lookup[key{kind, res.GetName()}] = v // used for constructing edges
			keep = append(keep, v)               // append

			//break // let's see if another resource even matches
		}
	}

	for _, r := range c.Resources {
		for _, e := range r.deps {
			if _, ok := lookup[key{strings.ToLower(e.From.Kind), e.From.Name}]; !ok {
				return nil, fmt.Errorf("can't find 'from' name")
			}
			if _, ok := lookup[key{strings.ToLower(e.To.Kind), e.To.Name}]; !ok {
				return nil, fmt.Errorf("can't find 'to' name")
			}
			from := lookup[key{strings.ToLower(e.From.Kind), e.From.Name}]
			to := lookup[key{strings.ToLower(e.To.Kind), e.To.Name}]
			edge := &resources.Edge{
				Name:   e.Name,
				Notify: e.Notify,
			}
			graph.AddEdge(from, to, edge)
		}

		recv := make(map[string]*resources.Send)
		// build Rcv's from resource variables
		for k, v := range r.rcv {
			send, ok := lookup[key{strings.ToLower(v.Kind), v.Name}]
			if !ok {
				return nil, fmt.Errorf("resource not found")
			}

			recv[strings.ToUpper(string(k[0]))+k[1:]] = &resources.Send{
				Res: resources.VtoR(send),
				Key: v.Field,
			}

			to := lookup[key{strings.ToLower(r.Kind), r.Name}]
			edge := &resources.Edge{
				Name:   v.Name,
				Notify: true,
			}
			graph.AddEdge(send, to, edge)
		}

		r.resource.SetRecv(recv)
	}

	return graph, nil
}

func loadHcl(f *string) (*Config, error) {
	if f == nil {
		return nil, fmt.Errorf("empty file given")
	}

	data, err := ioutil.ReadFile(*f)
	if err != nil {
		return nil, fmt.Errorf("unable to read file: %v", err)
	}

	file, err := hcl.ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("unable to parse file: %s", err)
	}

	config := new(Config)

	list, ok := file.Node.(*ast.ObjectList)
	if !ok {
		return nil, fmt.Errorf("unable to parse file: file does not contain root node object")
	}

	if resources := list.Filter("resource"); len(resources.Items) > 0 {
		var err error
		config.Resources, err = loadResourcesHcl(resources)
		if err != nil {
			return nil, fmt.Errorf("unable to parse: %s", err)
		}
	}

	return config, nil
}

func loadResourcesHcl(list *ast.ObjectList) ([]*Resource, error) {
	list = list.Children()
	if len(list.Items) == 0 {
		return nil, nil
	}

	var result []*Resource

	for _, item := range list.Items {
		kind := item.Keys[0].Token.Value().(string)
		name := item.Keys[1].Token.Value().(string)

		var listVal *ast.ObjectList
		if ot, ok := item.Val.(*ast.ObjectType); ok {
			listVal = ot.List
		} else {
			return nil, fmt.Errorf("module '%s': should be an object", name)
		}

		var params = resources.DefaultMetaParams
		if o := listVal.Filter("meta"); len(o.Items) > 0 {
			err := hcl.DecodeObject(&params, o)
			if err != nil {
				return nil, fmt.Errorf(
					"Error parsing meta for %s: %s",
					name,
					err)
			}
		}

		var deps []string
		if edges := listVal.Filter("depends_on"); len(edges.Items) > 0 {
			err := hcl.DecodeObject(&deps, edges.Items[0].Val)
			if err != nil {
				return nil, fmt.Errorf("unable to parse: %s", err)
			}
		}

		var edges []*Edge
		for _, dep := range deps {
			vertices := strings.Split(dep, ".")
			edges = append(edges, &Edge{
				To: vertex{
					Kind: kind,
					Name: name,
				},
				From: vertex{
					Kind: vertices[0],
					Name: vertices[1],
				},
			})
		}

		var config map[string]interface{}
		if err := hcl.DecodeObject(&config, item.Val); err != nil {
			log.Printf("HCL: unable to decode body: %v", err)
			return nil, fmt.Errorf(
				"Error reading config for %s: %s",
				name,
				err)
		}

		delete(config, "meta")
		delete(config, "depends_on")

		rcv := make(map[string]*hv.ResourceVariable)
		// parse strings for hil
		for k, v := range config {
			n, err := hil.Parse(v.(string))
			if err != nil {
				return nil, fmt.Errorf("unable to parse fields: %v", err)
			}

			variables, err := hv.ParseVariables(n)
			if err != nil {
				return nil, fmt.Errorf("unable to parse variables: %v", err)
			}

			for _, v := range variables {
				val, ok := v.(*hv.ResourceVariable)
				if !ok {
					continue
				}

				rcv[k] = val
			}
		}

		res, err := resources.NewResource(kind)
		if err != nil {
			log.Printf("HCLParse: unable to parse resource: %v", err)
			return nil, err
		}

		res.SetName(name)

		if err := hcl.DecodeObject(res, item.Val); err != nil {
			log.Printf("HCLParse: unable to decode body: %v", err)
			return nil, fmt.Errorf(
				"Error reading config for %s: %s",
				name,
				err)
		}

		meta := res.Meta()
		*meta = params

		result = append(result, &Resource{
			Name:     name,
			Kind:     kind,
			resource: res,
			deps:     edges,
			rcv:      rcv,
		})
	}

	return result, nil
}
