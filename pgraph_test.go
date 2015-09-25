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

// NOTE: this is pgraph, a pointer graph

package main

import (
	"testing"
)

func TestPgraphT1(t *testing.T) {

	G := NewGraph("g1")

	if i := G.NumVertices(); i != 0 {
		t.Errorf("Should have 0 vertices instead of: %d.", i)
	}

	v1 := NewVertex("v1", "type")
	v2 := NewVertex("v2", "type")
	e1 := NewEdge("e1")
	G.AddEdge(v1, v2, e1)

	if i := G.NumVertices(); i != 2 {
		t.Errorf("Should have 2 vertices instead of: %d.", i)
	}
}

func TestPgraphT2(t *testing.T) {

	G := NewGraph("g2")
	v1 := NewVertex("v1", "type")
	v2 := NewVertex("v2", "type")
	v3 := NewVertex("v3", "type")
	v4 := NewVertex("v4", "type")
	v5 := NewVertex("v5", "type")
	v6 := NewVertex("v6", "type")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	//e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)

	if i := G.NumVertices(); i != 6 {
		t.Errorf("Should have 6 vertices instead of: %d.", i)
	}
}

func TestPgraphT3(t *testing.T) {

	G := NewGraph("g3")
	v1 := NewVertex("v1", "type")
	v2 := NewVertex("v2", "type")
	v3 := NewVertex("v3", "type")
	v4 := NewVertex("v4", "type")
	v5 := NewVertex("v5", "type")
	v6 := NewVertex("v6", "type")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	//e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	//G.AddEdge(v6, v4, e6)
	out1 := G.DFS(v1)
	if i := len(out1); i != 3 {
		t.Errorf("Should have 3 vertices instead of: %d.", i)
		t.Errorf("Found: %v", out1)
		for _, v := range out1 {
			t.Errorf("Value: %v", v.Name)
		}
	}

	out2 := G.DFS(v4)
	if i := len(out2); i != 3 {
		t.Errorf("Should have 3 vertices instead of: %d.", i)
		t.Errorf("Found: %v", out1)
		for _, v := range out1 {
			t.Errorf("Value: %v", v.Name)
		}
	}
}

func TestPgraphT4(t *testing.T) {

	G := NewGraph("g4")
	v1 := NewVertex("v1", "type")
	v2 := NewVertex("v2", "type")
	v3 := NewVertex("v3", "type")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	out := G.DFS(v1)
	if i := len(out); i != 3 {
		t.Errorf("Should have 3 vertices instead of: %d.", i)
		t.Errorf("Found: %v", out)
		for _, v := range out {
			t.Errorf("Value: %v", v.Name)
		}
	}
}

func TestPgraphT5(t *testing.T) {
	G := NewGraph("g5")
	v1 := NewVertex("v1", "type")
	v2 := NewVertex("v2", "type")
	v3 := NewVertex("v3", "type")
	v4 := NewVertex("v4", "type")
	v5 := NewVertex("v5", "type")
	v6 := NewVertex("v6", "type")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	//e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	//G.AddEdge(v6, v4, e6)

	save := []*Vertex{v1, v2, v3}
	out := G.FilterGraph("new g5", save)
	if i := out.NumVertices(); i != 3 {
		t.Errorf("Should have 3 vertices instead of: %d.", i)
	}

}

func TestPgraphT6(t *testing.T) {
	G := NewGraph("g6")
	v1 := NewVertex("v1", "type")
	v2 := NewVertex("v2", "type")
	v3 := NewVertex("v3", "type")
	v4 := NewVertex("v4", "type")
	v5 := NewVertex("v5", "type")
	v6 := NewVertex("v6", "type")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	//e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	//G.AddEdge(v6, v4, e6)

	graphs := G.GetDisconnectedGraphs()
	HeisenbergGraphCount := func(ch chan *Graph) int {
		c := 0
		for x := range ch {
			_ = x
			c++
		}
		return c
	}

	if i := HeisenbergGraphCount(graphs); i != 2 {
		t.Errorf("Should have 2 graphs instead of: %d.", i)
	}

}
