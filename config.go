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
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"strings"
)

type collectorResConfig struct {
	Res     string `yaml:"res"`
	Pattern string `yaml:"pattern"` // XXX: Not Implemented
}

type vertexConfig struct {
	Res  string `yaml:"res"`
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
		Noop []NoopRes `yaml:"noop"`
		File []FileRes `yaml:"file"`
		Svc  []SvcRes  `yaml:"svc"`
		Exec []ExecRes `yaml:"exec"`
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

// XXX: we need to fix this function so that it either fails without modifying
// the graph, passes successfully and modifies it, or basically panics i guess
// this way an invalid compilation can leave the old graph running, and we we
// don't modify a partial graph. so we really need to validate, and then perform
// whatever actions are necessary
// finding some way to do this on a copy of the graph, and then do a graph diff
// and merge the new data into the old graph would be more appropriate, in
// particular if we can ensure the graph merge can't fail. As for the putting
// of stuff into etcd, we should probably store the operations to complete in
// the new graph, and keep retrying until it succeeds, thus blocking any new
// etcd operations until that time.
func UpdateGraphFromConfig(config *GraphConfig, hostname string, g *Graph, etcdO *EtcdWObject) bool {

	var NoopMap = make(map[string]*Vertex)
	var FileMap = make(map[string]*Vertex)
	var SvcMap = make(map[string]*Vertex)
	var ExecMap = make(map[string]*Vertex)

	var lookup = make(map[string]map[string]*Vertex)
	lookup["noop"] = NoopMap
	lookup["file"] = FileMap
	lookup["svc"] = SvcMap
	lookup["exec"] = ExecMap

	//log.Printf("%+v", config) // debug

	g.SetName(config.Graph) // set graph name

	var keep []*Vertex // list of vertex which are the same in new graph

	for _, t := range config.Resources.Noop {
		obj := NewNoopRes(t.Name)
		v := g.GetVertexMatch(obj)
		if v == nil { // no match found
			v = NewVertex(obj)
			g.AddVertex(v) // call standalone in case not part of an edge
		}
		NoopMap[obj.Name] = v  // used for constructing edges
		keep = append(keep, v) // append
	}

	for _, t := range config.Resources.File {
		// XXX: should we export based on a @@ prefix, or a metaparam
		// like exported => true || exported => (host pattern)||(other pattern?)
		if strings.HasPrefix(t.Name, "@@") { // exported resource
			// add to etcd storage...
			t.Name = t.Name[2:] //slice off @@
			if !etcdO.EtcdPut(hostname, t.Name, "file", t) {
				log.Printf("Problem exporting file resource %v.", t.Name)
				continue
			}
		} else {
			obj := NewFileRes(t.Name, t.Path, t.Dirname, t.Basename, t.Content, t.State)
			v := g.GetVertexMatch(obj)
			if v == nil { // no match found
				v = NewVertex(obj)
				g.AddVertex(v) // call standalone in case not part of an edge
			}
			FileMap[obj.Name] = v  // used for constructing edges
			keep = append(keep, v) // append
		}
	}

	for _, t := range config.Resources.Svc {
		obj := NewSvcRes(t.Name, t.State, t.Startup)
		v := g.GetVertexMatch(obj)
		if v == nil { // no match found
			v = NewVertex(obj)
			g.AddVertex(v) // call standalone in case not part of an edge
		}
		SvcMap[obj.Name] = v   // used for constructing edges
		keep = append(keep, v) // append
	}

	for _, t := range config.Resources.Exec {
		obj := NewExecRes(t.Name, t.Cmd, t.Shell, t.Timeout, t.WatchCmd, t.WatchShell, t.IfCmd, t.IfShell, t.PollInt, t.State)
		v := g.GetVertexMatch(obj)
		if v == nil { // no match found
			v = NewVertex(obj)
			g.AddVertex(v) // call standalone in case not part of an edge
		}
		ExecMap[obj.Name] = v  // used for constructing edges
		keep = append(keep, v) // append
	}

	// lookup from etcd graph
	// do all the graph look ups in one single step, so that if the etcd
	// database changes, we don't have a partial state of affairs...
	nodes, ok := etcdO.EtcdGet()
	if ok {
		for _, t := range config.Collector {
			// XXX: use t.Res and optionally t.Pattern to collect from etcd storage
			log.Printf("Collect: %v; Pattern: %v", t.Res, t.Pattern)

			for _, x := range etcdO.EtcdGetProcess(nodes, "file") {
				var obj *FileRes
				if B64ToObj(x, &obj) != true {
					log.Printf("Collect: File: %v not collected!", x)
					continue
				}
				if t.Pattern != "" { // XXX: currently the pattern for files can only override the Dirname variable :P
					obj.Dirname = t.Pattern
				}

				log.Printf("Collect: File: %v collected!", obj.GetName())

				// XXX: similar to file add code:
				v := g.GetVertexMatch(obj)
				if v == nil { // no match found
					obj.Init() // initialize go channels or things won't work!!!
					v = NewVertex(obj)
					g.AddVertex(v) // call standalone in case not part of an edge
				}
				FileMap[obj.GetName()] = v // used for constructing edges
				keep = append(keep, v)     // append

			}

		}
	}

	// get rid of any vertices we shouldn't "keep" (that aren't in new graph)
	for _, v := range g.GetVertices() {
		if !HasVertex(v, keep) {
			// wait for exit before starting new graph!
			v.Res.SendEvent(eventExit, true, false)
			g.DeleteVertex(v)
		}
	}

	for _, e := range config.Edges {
		if _, ok := lookup[e.From.Res]; !ok {
			return false
		}
		if _, ok := lookup[e.To.Res]; !ok {
			return false
		}
		if _, ok := lookup[e.From.Res][e.From.Name]; !ok {
			return false
		}
		if _, ok := lookup[e.To.Res][e.To.Name]; !ok {
			return false
		}
		g.AddEdge(lookup[e.From.Res][e.From.Name], lookup[e.To.Res][e.To.Name], NewEdge(e.Name))
	}
	return true
}
