// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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
)

type noopTypeConfig struct {
	Name string `yaml:"name"`
}

type fileTypeConfig struct {
	Name    string `yaml:"name"`
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
	State   string `yaml:"state"`
}

type serviceTypeConfig struct {
	Name    string `yaml:"name"`
	State   string `yaml:"state"`
	Startup string `yaml:"startup"`
}

type vertexConfig struct {
	Type string `yaml:"type"`
	Name string `yaml:"name"`
}

type edgeConfig struct {
	Name string       `yaml:"name"`
	From vertexConfig `yaml:"from"`
	To   vertexConfig `yaml:"to"`
}

type graphConfig struct {
	Graph string `yaml:"graph"`
	Types struct {
		Noop    []noopTypeConfig    `yaml:"noop"`
		File    []fileTypeConfig    `yaml:"file"`
		Service []serviceTypeConfig `yaml:"service"`
	} `yaml:"types"`
	Edges   []edgeConfig `yaml:"edges"`
	Comment string       `yaml:"comment"`
}

func (c *graphConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if c.Graph == "" {
		return errors.New("Graph config: invalid `graph`")
	}
	return nil
}

func GraphFromConfig(filename string) *Graph {

	var NoopMap map[string]*Vertex = make(map[string]*Vertex)
	var FileMap map[string]*Vertex = make(map[string]*Vertex)
	var ServiceMap map[string]*Vertex = make(map[string]*Vertex)

	var lookup map[string]map[string]*Vertex = make(map[string]map[string]*Vertex)
	lookup["noop"] = NoopMap
	lookup["file"] = FileMap
	lookup["service"] = ServiceMap

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	var config graphConfig
	if err := config.Parse(data); err != nil {
		log.Fatal(err)
	}
	//fmt.Printf("%+v\n", config)	// debug

	g := NewGraph(config.Graph)

	for _, t := range config.Types.Noop {
		NoopMap[t.Name] = NewVertex(t.Name, "noop")
		// FIXME: duplicate of name stored twice... where should it go?
		NoopMap[t.Name].Associate(NewNoopType(t.Name))
		g.AddVertex(NoopMap[t.Name]) // call standalone in case not part of an edge
	}

	for _, t := range config.Types.File {
		FileMap[t.Name] = NewVertex(t.Name, "file")
		// FIXME: duplicate of name stored twice... where should it go?
		FileMap[t.Name].Associate(NewFileType(t.Name, t.Path, t.Content, t.State))
		g.AddVertex(FileMap[t.Name]) // call standalone in case not part of an edge
	}

	for _, t := range config.Types.Service {
		ServiceMap[t.Name] = NewVertex(t.Name, "service")
		// FIXME: duplicate of name stored twice... where should it go?
		ServiceMap[t.Name].Associate(NewServiceType(t.Name, t.State, t.Startup))
		g.AddVertex(ServiceMap[t.Name]) // call standalone in case not part of an edge
	}

	for _, e := range config.Edges {
		g.AddEdge(lookup[e.From.Type][e.From.Name], lookup[e.To.Type][e.To.Name], NewEdge(e.Name))
	}

	return g
}
