// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

// +build !root

package pgraph

import (
	"reflect"
	"testing"
)

func TestCount1(t *testing.T) {
	G := &Graph{}

	if i := G.NumVertices(); i != 0 {
		t.Errorf("should have 0 vertices instead of: %d", i)
	}

	if i := G.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
	}

	v1 := NV("v1")
	v2 := NV("v2")
	e1 := NE("e1")
	G.AddEdge(v1, v2, e1)

	if i := G.NumVertices(); i != 2 {
		t.Errorf("should have 2 vertices instead of: %d", i)
	}

	if i := G.NumEdges(); i != 1 {
		t.Errorf("should have 1 edges instead of: %d", i)
	}
}

func TestAddVertex1(t *testing.T) {
	G := &Graph{Name: "g2"}
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	//e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)

	if i := G.NumVertices(); i != 6 {
		t.Errorf("should have 6 vertices instead of: %d", i)
	}
}

func TestDFS1(t *testing.T) {
	G, _ := NewGraph("g3")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	//e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	//G.AddEdge(v6, v4, e6)
	out1 := G.DFS(v1)
	if i := len(out1); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
		t.Errorf("found: %v", out1)
		for _, v := range out1 {
			t.Errorf("value: %s", v)
		}
	}

	out2 := G.DFS(v4)
	if i := len(out2); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
		t.Errorf("found: %v", out1)
		for _, v := range out1 {
			t.Errorf("value: %s", v)
		}
	}
}

func TestDFS2(t *testing.T) {
	G, _ := NewGraph("g4")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	out := G.DFS(v1)
	if i := len(out); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
		t.Errorf("found: %v", out)
		for _, v := range out {
			t.Errorf("value: %s", v)
		}
	}
}

func TestFilterGraph1(t *testing.T) {
	G, _ := NewGraph("g5")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	//e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	//G.AddEdge(v6, v4, e6)

	save := []Vertex{v1, v2, v3}
	out, err := G.FilterGraph("new g5", save)
	if err != nil {
		t.Errorf("failed with: %v", err)
	}

	if i := out.NumVertices(); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
	}
}

func TestDisconnectedGraphs1(t *testing.T) {
	G, _ := NewGraph("g6")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	//e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	//G.AddEdge(v6, v4, e6)

	graphs, err := G.DisconnectedGraphs()
	if err != nil {
		t.Errorf("failed with: %v", err)
	}

	if i := len(graphs); i != 2 {
		t.Errorf("should have 2 graphs instead of: %d", i)
	}
}

func TestDeleteVertex1(t *testing.T) {
	G, _ := NewGraph("g7")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	if i := G.NumVertices(); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
	}

	G.DeleteVertex(v2)

	if i := G.NumVertices(); i != 2 {
		t.Errorf("should have 2 vertices instead of: %d", i)
	}

	G.DeleteVertex(v1)

	if i := G.NumVertices(); i != 1 {
		t.Errorf("should have 1 vertices instead of: %d", i)
	}

	G.DeleteVertex(v3)

	if i := G.NumVertices(); i != 0 {
		t.Errorf("should have 0 vertices instead of: %d", i)
	}

	G.DeleteVertex(v2) // duplicate deletes don't error...

	if i := G.NumVertices(); i != 0 {
		t.Errorf("should have 0 vertices instead of: %d", i)
	}
}

func TestDeleteVertex2(t *testing.T) {
	G := &Graph{}
	v1 := NV("v1")
	G.DeleteVertex(v1) // check this doesn't panic

	if i := G.NumVertices(); i != 0 {
		t.Errorf("should have 0 vertices instead of: %d", i)
	}
}

func TestVertexContains1(t *testing.T) {
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	if VertexContains(v1, []Vertex{v1, v2, v3}) != true {
		t.Errorf("should be true instead of false.")
	}

	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	if VertexContains(v4, []Vertex{v5, v6}) != false {
		t.Errorf("should be false instead of true.")
	}

	v7 := NV("v7")
	v8 := NV("v8")
	v9 := NV("v9")
	if VertexContains(v8, []Vertex{v7, v8, v9}) != true {
		t.Errorf("should be true instead of false.")
	}

	v1b := NV("v1") // same value, different objects
	if VertexContains(v1b, []Vertex{v1, v2, v3}) != false {
		t.Errorf("should be false instead of true.")
	}
}

func TestTopoSort1(t *testing.T) {
	G, _ := NewGraph("g9")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v1, v3, e2)
	G.AddEdge(v2, v4, e3)
	G.AddEdge(v3, v4, e4)

	G.AddEdge(v4, v5, e5)
	G.AddEdge(v5, v6, e6)

	indegree := G.InDegree() // map[Vertex]int
	if i := indegree[v1]; i != 0 {
		t.Errorf("indegree of v1 should be 0 instead of: %d", i)
	}
	if i := indegree[v2]; i != 1 {
		t.Errorf("indegree of v2 should be 1 instead of: %d", i)
	}
	if i := indegree[v3]; i != 1 {
		t.Errorf("indegree of v3 should be 1 instead of: %d", i)
	}
	if i := indegree[v4]; i != 2 {
		t.Errorf("indegree of v4 should be 2 instead of: %d", i)
	}
	if i := indegree[v5]; i != 1 {
		t.Errorf("indegree of v5 should be 1 instead of: %d", i)
	}
	if i := indegree[v6]; i != 1 {
		t.Errorf("indegree of v6 should be 1 instead of: %d", i)
	}

	outdegree := G.OutDegree() // map[Vertex]int
	if i := outdegree[v1]; i != 2 {
		t.Errorf("outdegree of v1 should be 2 instead of: %d", i)
	}
	if i := outdegree[v2]; i != 1 {
		t.Errorf("outdegree of v2 should be 1 instead of: %d", i)
	}
	if i := outdegree[v3]; i != 1 {
		t.Errorf("outdegree of v3 should be 1 instead of: %d", i)
	}
	if i := outdegree[v4]; i != 1 {
		t.Errorf("outdegree of v4 should be 1 instead of: %d", i)
	}
	if i := outdegree[v5]; i != 1 {
		t.Errorf("outdegree of v5 should be 1 instead of: %d", i)
	}
	if i := outdegree[v6]; i != 0 {
		t.Errorf("outdegree of v6 should be 0 instead of: %d", i)
	}

	s, err := G.TopologicalSort()
	// either possibility is a valid toposort
	match := reflect.DeepEqual(s, []Vertex{v1, v2, v3, v4, v5, v6}) || reflect.DeepEqual(s, []Vertex{v1, v3, v2, v4, v5, v6})
	if err != nil || !match {
		t.Errorf("topological sort failed, error: %v", err)
		str := "Found:"
		for _, v := range s {
			str += " " + v.String()
		}
		t.Errorf(str)
	}
}

func TestTopoSort2(t *testing.T) {
	G, _ := NewGraph("g10")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v4, e3)
	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	G.AddEdge(v4, v2, e6) // cycle

	if _, err := G.TopologicalSort(); err == nil {
		t.Errorf("topological sort passed, but graph is cyclic")
	}
}

// empty
func TestReachability0(t *testing.T) {
	{
		G, _ := NewGraph("g")
		result, err := G.Reachability(nil, nil)
		if err != nil {
			t.Logf("reachability failed: %+v", err)
			if result != nil {
				str := "Got:"
				for _, v := range result {
					str += " " + v.String()
				}
				t.Errorf(str)
			}
		}
	}
	{
		G, _ := NewGraph("g")
		v1 := NV("v1")
		v6 := NV("v6")

		result, err := G.Reachability(v1, v6)
		if err != nil {
			t.Logf("reachability failed: %+v", err)
			return
		}
		expected := []Vertex{}

		if !reflect.DeepEqual(result, expected) {
			t.Logf("reachability failed")
			str := "Got:"
			for _, v := range result {
				str += " " + v.String()
			}
			t.Errorf(str)
		}
	}
	{
		G, _ := NewGraph("g")
		v1 := NV("v1")
		v2 := NV("v2")
		v3 := NV("v3")
		v4 := NV("v4")
		v5 := NV("v5")
		v6 := NV("v6")
		e1 := NE("e1")
		e2 := NE("e2")
		e3 := NE("e3")
		e4 := NE("e4")
		e5 := NE("e5")
		G.AddEdge(v1, v2, e1)
		G.AddEdge(v2, v3, e2)
		G.AddEdge(v1, v4, e3)
		G.AddEdge(v3, v4, e4)
		G.AddEdge(v3, v5, e5)

		result, err := G.Reachability(v1, v6)
		if err != nil {
			t.Logf("reachability failed: %+v", err)
			return
		}
		expected := []Vertex{}

		if !reflect.DeepEqual(result, expected) {
			t.Logf("reachability failed")
			str := "Got:"
			for _, v := range result {
				str += " " + v.String()
			}
			t.Errorf(str)
		}
	}
}

// simple linear path
func TestReachability1(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	//e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v4, e3)
	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)

	result, err := G.Reachability(v1, v6)
	if err != nil {
		t.Logf("reachability failed: %+v", err)
		return
	}
	expected := []Vertex{v1, v2, v3, v4, v5, v6}

	if !reflect.DeepEqual(result, expected) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.String()
		}
		t.Errorf(str)
	}
}

// pick one of two correct paths
func TestReachability2(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v1, v3, e2)
	G.AddEdge(v2, v4, e3)
	G.AddEdge(v3, v4, e4)
	G.AddEdge(v4, v5, e5)
	G.AddEdge(v5, v6, e6)

	result, err := G.Reachability(v1, v6)
	if err != nil {
		t.Logf("reachability failed: %+v", err)
		return
	}
	expected1 := []Vertex{v1, v2, v4, v5, v6}
	expected2 := []Vertex{v1, v3, v4, v5, v6}

	// !xor test
	if reflect.DeepEqual(result, expected1) == reflect.DeepEqual(result, expected2) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.String()
		}
		t.Errorf(str)
	}
}

// pick shortest path
func TestReachability3(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v4, e3)
	G.AddEdge(v4, v5, e4)
	G.AddEdge(v1, v5, e5)
	G.AddEdge(v5, v6, e6)

	result, err := G.Reachability(v1, v6)
	if err != nil {
		t.Logf("reachability failed: %+v", err)
		return
	}
	expected := []Vertex{v1, v5, v6}

	if !reflect.DeepEqual(result, expected) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.String()
		}
		t.Errorf(str)
	}
}

// direct path
func TestReachability4(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v4, e3)
	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)
	G.AddEdge(v1, v6, e6)

	result, err := G.Reachability(v1, v6)
	if err != nil {
		t.Logf("reachability failed: %+v", err)
		return
	}
	expected := []Vertex{v1, v6}

	if !reflect.DeepEqual(result, expected) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.String()
		}
		t.Errorf(str)
	}
}

func TestReverse1(t *testing.T) {
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")

	if rev := Reverse([]Vertex{}); !reflect.DeepEqual(rev, []Vertex{}) {
		t.Errorf("reverse of vertex slice failed (empty)")
	}

	if rev := Reverse([]Vertex{v1}); !reflect.DeepEqual(rev, []Vertex{v1}) {
		t.Errorf("reverse of vertex slice failed (single)")
	}

	if rev := Reverse([]Vertex{v1, v2, v3, v4, v5, v6}); !reflect.DeepEqual(rev, []Vertex{v6, v5, v4, v3, v2, v1}) {
		t.Errorf("reverse of vertex slice failed (1..6)")
	}

	if rev := Reverse([]Vertex{v6, v5, v4, v3, v2, v1}); !reflect.DeepEqual(rev, []Vertex{v1, v2, v3, v4, v5, v6}) {
		t.Errorf("reverse of vertex slice failed (6..1)")
	}
}

func TestCopy1(t *testing.T) {
	g1 := &Graph{}
	g2 := g1.Copy() // check this doesn't panic
	if !reflect.DeepEqual(g1.String(), g2.String()) {
		t.Errorf("graph copy failed")
	}
}

func TestGraphCmp1(t *testing.T) {
	g1 := &Graph{}
	g2 := &Graph{}
	g3 := &Graph{}
	g3.AddVertex(NV("v1"))
	g4 := &Graph{}
	g4.AddVertex(NV("v2"))

	if err := g1.GraphCmp(g2, strVertexCmpFn, strEdgeCmpFn); err != nil {
		t.Errorf("should have no error during GraphCmp, but got: %v", err)
	}

	if err := g1.GraphCmp(g3, strVertexCmpFn, strEdgeCmpFn); err == nil {
		t.Errorf("should have error during GraphCmp, but got nil")
	}

	if err := g3.GraphCmp(g4, strVertexCmpFn, strEdgeCmpFn); err == nil {
		t.Errorf("should have error during GraphCmp, but got nil")
	}
}

// FIXME: i think we should allow equivalent elements in the graph to compare...
// FIXME: currently this fails :(
//func TestGraphCmp2(t *testing.T) {
//	g1 := &Graph{}
//	g2 := &Graph{}
//	g1.AddVertex(NV("v1"), NV("v1"))
//	g2.AddVertex(NV("v1"), NV("v1"))
//
//	if err := g1.GraphCmp(g2, strVertexCmpFn, strEdgeCmpFn); err != nil {
//		t.Errorf("should have no error during GraphCmp, but got: %v", err)
//	}
//}

func TestSort0(t *testing.T) {
	vs := []Vertex{}
	s := Sort(vs)

	if !reflect.DeepEqual(s, []Vertex{}) {
		t.Errorf("sort failed!")
		if s == nil {
			t.Logf("output is nil!")
		} else {
			str := "Got:"
			for _, v := range s {
				str += " " + v.String()
			}
			t.Errorf(str)
		}
	}
}

func TestSort1(t *testing.T) {
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")

	vs := []Vertex{v3, v2, v6, v1, v5, v4}
	s := Sort(vs)

	if !reflect.DeepEqual(s, []Vertex{v1, v2, v3, v4, v5, v6}) {
		t.Errorf("sort failed!")
		str := "Got:"
		for _, v := range s {
			str += " " + v.String()
		}
		t.Errorf(str)
	}

	if !reflect.DeepEqual(vs, []Vertex{v3, v2, v6, v1, v5, v4}) {
		t.Errorf("sort modified input!")
		str := "Got:"
		for _, v := range vs {
			str += " " + v.String()
		}
		t.Errorf(str)
	}
}

func TestSprint1(t *testing.T) {
	g, _ := NewGraph("graph1")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	g.AddEdge(v1, v2, e1)
	g.AddEdge(v2, v3, e2)
	g.AddEdge(v3, v4, e3)
	g.AddEdge(v4, v5, e4)
	g.AddEdge(v5, v6, e5)

	str := g.Sprint()
	t.Logf("graph is:\n%s", str)
	count := 0
	for count < 100000 { // about one second
		x := g.Sprint()
		if str != x {
			t.Errorf("graph sprint is not consistent")
			return
		}
		count++
	}
}

func TestDeleteEdge1(t *testing.T) {
	g, _ := NewGraph("g")

	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")

	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")

	g.AddEdge(v1, v2, e1)
	g.AddEdge(v2, v3, e2)
	g.AddEdge(v1, v3, e3)
	g.AddEdge(v2, v1, e4)
	g.AddEdge(v3, v2, e5)
	g.AddEdge(v3, v1, e6)

	g.DeleteEdge(e1)
	g.DeleteEdge(e2)
	g.DeleteEdge(e3)
	g.DeleteEdge(e3)

	if g.NumEdges() != 3 {
		t.Errorf("expected number of edges: 3, instead of: %d", g.NumEdges())
	}
}

func TestDeleteEdge2(t *testing.T) {
	g, _ := NewGraph("g")

	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")

	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	e7 := NE("e7")

	g.AddEdge(v1, v2, e1)
	g.AddEdge(v1, v2, e2)
	g.AddEdge(v1, v3, e3)
	g.AddEdge(v1, v4, e4)
	g.AddEdge(v2, v1, e5)
	g.AddEdge(v3, v1, e6)
	g.AddEdge(v4, v1, e7)

	g.DeleteEdge(e1)
	g.DeleteEdge(e2)
	g.DeleteEdge(e3)
	g.DeleteEdge(e5)
	g.DeleteEdge(e6)

	ie := g.IncomingGraphEdges(v1)
	oe := g.OutgoingGraphEdges(v1)

	if !reflect.DeepEqual(ie, []Edge{e7}) {
		res := ""
		for _, e := range ie {
			res += e.String() + " "
		}
		t.Errorf("expected incoming graph edges for vertex v1: e7, instead of: %s", res)
	}

	if !reflect.DeepEqual(oe, []Edge{e4}) {
		res := ""
		for _, e := range oe {
			res += e.String() + " "
		}
		t.Errorf("expected outgoing graph edges for vertex v1: e4, instead of: %s", res)
	}
}

func TestFindEdge1(t *testing.T) {
	g, _ := NewGraph("g")

	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	v7 := NV("v7")

	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")
	e5 := NE("e5")
	e6 := NE("e6")
	e7 := NE("e7")
	e8 := NE("e8")
	e9 := NE("e9")
	e10 := NE("e10")

	g.AddEdge(v1, v2, e1)
	g.AddEdge(v1, v4, e2)
	g.AddEdge(v1, v3, e3)
	g.AddEdge(v2, v3, e4)
	g.AddEdge(v2, v4, e5)
	g.AddEdge(v2, v6, e6)
	g.AddEdge(v2, v5, e7)
	g.AddEdge(v3, v6, e8)
	g.AddEdge(v4, v7, e9)
	g.AddEdge(v5, v7, e10)

	if !(g.HasVertex(v1) && g.HasVertex(v4) && g.HasVertex(v7)) {
		t.Errorf("graph expected to have vertices v1, v4, and v7")
	}
	if g.FindEdge(v1, v4) != e2 {
		t.Errorf("edge e2 was not returned")
	}
	if g.FindEdge(v1, v7) != nil {
		t.Errorf("an edge was found although it did not exist")
	}
}
