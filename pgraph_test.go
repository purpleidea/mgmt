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

// NOTE: this is pgraph, a pointer graph

package main

import (
	"reflect"
	"testing"
)

func TestPgraphT1(t *testing.T) {

	G := NewGraph("g1")

	if i := G.NumVertices(); i != 0 {
		t.Errorf("Should have 0 vertices instead of: %d.", i)
	}

	if i := G.NumEdges(); i != 0 {
		t.Errorf("Should have 0 edges instead of: %d.", i)
	}

	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	e1 := NewEdge("e1")
	G.AddEdge(v1, v2, e1)

	if i := G.NumVertices(); i != 2 {
		t.Errorf("Should have 2 vertices instead of: %d.", i)
	}

	if i := G.NumEdges(); i != 1 {
		t.Errorf("Should have 1 edges instead of: %d.", i)
	}
}

func TestPgraphT2(t *testing.T) {

	G := NewGraph("g2")
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
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
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
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
			t.Errorf("Value: %v", v.GetName())
		}
	}

	out2 := G.DFS(v4)
	if i := len(out2); i != 3 {
		t.Errorf("Should have 3 vertices instead of: %d.", i)
		t.Errorf("Found: %v", out1)
		for _, v := range out1 {
			t.Errorf("Value: %v", v.GetName())
		}
	}
}

func TestPgraphT4(t *testing.T) {

	G := NewGraph("g4")
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
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
			t.Errorf("Value: %v", v.GetName())
		}
	}
}

func TestPgraphT5(t *testing.T) {
	G := NewGraph("g5")
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
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
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
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

func TestPgraphT7(t *testing.T) {

	G := NewGraph("g7")
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	if i := G.NumVertices(); i != 3 {
		t.Errorf("Should have 3 vertices instead of: %d.", i)
	}

	G.DeleteVertex(v2)

	if i := G.NumVertices(); i != 2 {
		t.Errorf("Should have 2 vertices instead of: %d.", i)
	}

	G.DeleteVertex(v1)

	if i := G.NumVertices(); i != 1 {
		t.Errorf("Should have 1 vertices instead of: %d.", i)
	}

	G.DeleteVertex(v3)

	if i := G.NumVertices(); i != 0 {
		t.Errorf("Should have 0 vertices instead of: %d.", i)
	}

	G.DeleteVertex(v2) // duplicate deletes don't error...

	if i := G.NumVertices(); i != 0 {
		t.Errorf("Should have 0 vertices instead of: %d.", i)
	}
}

func TestPgraphT8(t *testing.T) {

	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	if HasVertex(v1, []*Vertex{v1, v2, v3}) != true {
		t.Errorf("Should be true instead of false.")
	}

	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
	if HasVertex(v4, []*Vertex{v5, v6}) != false {
		t.Errorf("Should be false instead of true.")
	}

	v7 := NewVertex(NewNoopType("v7"))
	v8 := NewVertex(NewNoopType("v8"))
	v9 := NewVertex(NewNoopType("v9"))
	if HasVertex(v8, []*Vertex{v7, v8, v9}) != true {
		t.Errorf("Should be true instead of false.")
	}

	v_1 := NewVertex(NewNoopType("v1")) // same value, different objects
	if HasVertex(v_1, []*Vertex{v1, v2, v3}) != false {
		t.Errorf("Should be false instead of true.")
	}
}

func TestPgraphT9(t *testing.T) {

	G := NewGraph("g9")
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v1, v3, e2)
	G.AddEdge(v2, v4, e3)
	G.AddEdge(v3, v4, e4)

	G.AddEdge(v4, v5, e5)
	G.AddEdge(v5, v6, e6)

	indegree := G.InDegree() // map[*Vertex]int
	if i := indegree[v1]; i != 0 {
		t.Errorf("Indegree of v1 should be 0 instead of: %d.", i)
	}
	if i := indegree[v2]; i != 1 {
		t.Errorf("Indegree of v2 should be 1 instead of: %d.", i)
	}
	if i := indegree[v3]; i != 1 {
		t.Errorf("Indegree of v3 should be 1 instead of: %d.", i)
	}
	if i := indegree[v4]; i != 2 {
		t.Errorf("Indegree of v4 should be 2 instead of: %d.", i)
	}
	if i := indegree[v5]; i != 1 {
		t.Errorf("Indegree of v5 should be 1 instead of: %d.", i)
	}
	if i := indegree[v6]; i != 1 {
		t.Errorf("Indegree of v6 should be 1 instead of: %d.", i)
	}

	outdegree := G.OutDegree() // map[*Vertex]int
	if i := outdegree[v1]; i != 2 {
		t.Errorf("Outdegree of v1 should be 2 instead of: %d.", i)
	}
	if i := outdegree[v2]; i != 1 {
		t.Errorf("Outdegree of v2 should be 1 instead of: %d.", i)
	}
	if i := outdegree[v3]; i != 1 {
		t.Errorf("Outdegree of v3 should be 1 instead of: %d.", i)
	}
	if i := outdegree[v4]; i != 1 {
		t.Errorf("Outdegree of v4 should be 1 instead of: %d.", i)
	}
	if i := outdegree[v5]; i != 1 {
		t.Errorf("Outdegree of v5 should be 1 instead of: %d.", i)
	}
	if i := outdegree[v6]; i != 0 {
		t.Errorf("Outdegree of v6 should be 0 instead of: %d.", i)
	}

	s, ok := G.TopologicalSort()
	// either possibility is a valid toposort
	match := reflect.DeepEqual(s, []*Vertex{v1, v2, v3, v4, v5, v6}) || reflect.DeepEqual(s, []*Vertex{v1, v3, v2, v4, v5, v6})
	if !ok || !match {
		t.Errorf("Topological sort failed, status: %v.", ok)
		str := "Found:"
		for _, v := range s {
			str += " " + v.Type.GetName()
		}
		t.Errorf(str)
	}
}

func TestPgraphT10(t *testing.T) {

	G := NewGraph("g10")
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v4, e3)
	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	G.AddEdge(v4, v2, e6) // cycle

	if _, ok := G.TopologicalSort(); ok {
		t.Errorf("Topological sort passed, but graph is cyclic.")
	}
}

func TestPgraphT11(t *testing.T) {
	v1 := NewVertex(NewNoopType("v1"))
	v2 := NewVertex(NewNoopType("v2"))
	v3 := NewVertex(NewNoopType("v3"))
	v4 := NewVertex(NewNoopType("v4"))
	v5 := NewVertex(NewNoopType("v5"))
	v6 := NewVertex(NewNoopType("v6"))

	if rev := Reverse([]*Vertex{}); !reflect.DeepEqual(rev, []*Vertex{}) {
		t.Errorf("Reverse of vertex slice failed.")
	}

	if rev := Reverse([]*Vertex{v1}); !reflect.DeepEqual(rev, []*Vertex{v1}) {
		t.Errorf("Reverse of vertex slice failed.")
	}

	if rev := Reverse([]*Vertex{v1, v2, v3, v4, v5, v6}); !reflect.DeepEqual(rev, []*Vertex{v6, v5, v4, v3, v2, v1}) {
		t.Errorf("Reverse of vertex slice failed.")
	}

	if rev := Reverse([]*Vertex{v6, v5, v4, v3, v2, v1}); !reflect.DeepEqual(rev, []*Vertex{v1, v2, v3, v4, v5, v6}) {
		t.Errorf("Reverse of vertex slice failed.")
	}

}
