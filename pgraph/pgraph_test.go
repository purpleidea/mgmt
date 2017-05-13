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

package pgraph

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/resources"
	"github.com/purpleidea/mgmt/util"
)

// NV is a helper function to make testing easier. It creates a new noop vertex.
func NV(s string) *Vertex {
	obj := &resources.NoopRes{
		BaseRes: resources.BaseRes{
			Name: s,
		},
		Comment: "Testing!",
	}
	return NewVertex(obj)
}

func TestPgraphT1(t *testing.T) {

	G, _ := NewGraph("g1")

	if i := G.NumVertices(); i != 0 {
		t.Errorf("should have 0 vertices instead of: %d", i)
	}

	if i := G.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
	}

	v1 := NV("v1")
	v2 := NV("v2")
	e1 := NewEdge("e1")
	G.AddEdge(v1, v2, e1)

	if i := G.NumVertices(); i != 2 {
		t.Errorf("should have 2 vertices instead of: %d", i)
	}

	if i := G.NumEdges(); i != 1 {
		t.Errorf("should have 1 edges instead of: %d", i)
	}
}

func TestPgraphT2(t *testing.T) {

	G, _ := NewGraph("g2")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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
		t.Errorf("should have 6 vertices instead of: %d", i)
	}
}

func TestPgraphT3(t *testing.T) {

	G, _ := NewGraph("g3")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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
		t.Errorf("should have 3 vertices instead of: %d", i)
		t.Errorf("found: %v", out1)
		for _, v := range out1 {
			t.Errorf("value: %v", v.GetName())
		}
	}

	out2 := G.DFS(v4)
	if i := len(out2); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
		t.Errorf("found: %v", out1)
		for _, v := range out1 {
			t.Errorf("value: %v", v.GetName())
		}
	}
}

func TestPgraphT4(t *testing.T) {

	G, _ := NewGraph("g4")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v1, e3)

	out := G.DFS(v1)
	if i := len(out); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
		t.Errorf("found: %v", out)
		for _, v := range out {
			t.Errorf("value: %v", v.GetName())
		}
	}
}

func TestPgraphT5(t *testing.T) {
	G, _ := NewGraph("g5")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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
	out, err := G.FilterGraph("new g5", save)
	if err != nil {
		t.Errorf("failed with: %v", err)
	}

	if i := out.NumVertices(); i != 3 {
		t.Errorf("should have 3 vertices instead of: %d", i)
	}
}

func TestPgraphT6(t *testing.T) {
	G, _ := NewGraph("g6")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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

	graphs, err := G.GetDisconnectedGraphs()
	if err != nil {
		t.Errorf("failed with: %v", err)
	}

	if i := len(graphs); i != 2 {
		t.Errorf("should have 2 graphs instead of: %d", i)
	}
}

func TestPgraphT7(t *testing.T) {

	G, _ := NewGraph("g7")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
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

func TestPgraphT8(t *testing.T) {

	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	if VertexContains(v1, []*Vertex{v1, v2, v3}) != true {
		t.Errorf("should be true instead of false.")
	}

	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	if VertexContains(v4, []*Vertex{v5, v6}) != false {
		t.Errorf("should be false instead of true.")
	}

	v7 := NV("v7")
	v8 := NV("v8")
	v9 := NV("v9")
	if VertexContains(v8, []*Vertex{v7, v8, v9}) != true {
		t.Errorf("should be true instead of false.")
	}

	v1b := NV("v1") // same value, different objects
	if VertexContains(v1b, []*Vertex{v1, v2, v3}) != false {
		t.Errorf("should be false instead of true.")
	}
}

func TestPgraphT9(t *testing.T) {

	G, _ := NewGraph("g9")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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

	outdegree := G.OutDegree() // map[*Vertex]int
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
	match := reflect.DeepEqual(s, []*Vertex{v1, v2, v3, v4, v5, v6}) || reflect.DeepEqual(s, []*Vertex{v1, v3, v2, v4, v5, v6})
	if err != nil || !match {
		t.Errorf("topological sort failed, error: %v", err)
		str := "Found:"
		for _, v := range s {
			str += " " + v.Res.GetName()
		}
		t.Errorf(str)
	}
}

func TestPgraphT10(t *testing.T) {

	G, _ := NewGraph("g10")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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

	if _, err := G.TopologicalSort(); err == nil {
		t.Errorf("topological sort passed, but graph is cyclic")
	}
}

// empty
func TestPgraphReachability0(t *testing.T) {
	{
		G, _ := NewGraph("g")
		result := G.Reachability(nil, nil)
		if result != nil {
			t.Logf("reachability failed")
			str := "Got:"
			for _, v := range result {
				str += " " + v.Res.GetName()
			}
			t.Errorf(str)
		}
	}
	{
		G, _ := NewGraph("g")
		v1 := NV("v1")
		v6 := NV("v6")

		result := G.Reachability(v1, v6)
		expected := []*Vertex{}

		if !reflect.DeepEqual(result, expected) {
			t.Logf("reachability failed")
			str := "Got:"
			for _, v := range result {
				str += " " + v.Res.GetName()
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
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		e3 := NewEdge("e3")
		e4 := NewEdge("e4")
		e5 := NewEdge("e5")
		G.AddEdge(v1, v2, e1)
		G.AddEdge(v2, v3, e2)
		G.AddEdge(v1, v4, e3)
		G.AddEdge(v3, v4, e4)
		G.AddEdge(v3, v5, e5)

		result := G.Reachability(v1, v6)
		expected := []*Vertex{}

		if !reflect.DeepEqual(result, expected) {
			t.Logf("reachability failed")
			str := "Got:"
			for _, v := range result {
				str += " " + v.Res.GetName()
			}
			t.Errorf(str)
		}
	}
}

// simple linear path
func TestPgraphReachability1(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
	e1 := NewEdge("e1")
	e2 := NewEdge("e2")
	e3 := NewEdge("e3")
	e4 := NewEdge("e4")
	e5 := NewEdge("e5")
	//e6 := NewEdge("e6")
	G.AddEdge(v1, v2, e1)
	G.AddEdge(v2, v3, e2)
	G.AddEdge(v3, v4, e3)
	G.AddEdge(v4, v5, e4)
	G.AddEdge(v5, v6, e5)

	result := G.Reachability(v1, v6)
	expected := []*Vertex{v1, v2, v3, v4, v5, v6}

	if !reflect.DeepEqual(result, expected) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.Res.GetName()
		}
		t.Errorf(str)
	}
}

// pick one of two correct paths
func TestPgraphReachability2(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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

	result := G.Reachability(v1, v6)
	expected1 := []*Vertex{v1, v2, v4, v5, v6}
	expected2 := []*Vertex{v1, v3, v4, v5, v6}

	// !xor test
	if reflect.DeepEqual(result, expected1) == reflect.DeepEqual(result, expected2) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.Res.GetName()
		}
		t.Errorf(str)
	}
}

// pick shortest path
func TestPgraphReachability3(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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
	G.AddEdge(v1, v5, e5)
	G.AddEdge(v5, v6, e6)

	result := G.Reachability(v1, v6)
	expected := []*Vertex{v1, v5, v6}

	if !reflect.DeepEqual(result, expected) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.Res.GetName()
		}
		t.Errorf(str)
	}
}

// direct path
func TestPgraphReachability4(t *testing.T) {
	G, _ := NewGraph("g")
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")
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
	G.AddEdge(v1, v6, e6)

	result := G.Reachability(v1, v6)
	expected := []*Vertex{v1, v6}

	if !reflect.DeepEqual(result, expected) {
		t.Logf("reachability failed")
		str := "Got:"
		for _, v := range result {
			str += " " + v.Res.GetName()
		}
		t.Errorf(str)
	}
}

func TestPgraphT11(t *testing.T) {
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	v6 := NV("v6")

	if rev := Reverse([]*Vertex{}); !reflect.DeepEqual(rev, []*Vertex{}) {
		t.Errorf("reverse of vertex slice failed")
	}

	if rev := Reverse([]*Vertex{v1}); !reflect.DeepEqual(rev, []*Vertex{v1}) {
		t.Errorf("reverse of vertex slice failed")
	}

	if rev := Reverse([]*Vertex{v1, v2, v3, v4, v5, v6}); !reflect.DeepEqual(rev, []*Vertex{v6, v5, v4, v3, v2, v1}) {
		t.Errorf("reverse of vertex slice failed")
	}

	if rev := Reverse([]*Vertex{v6, v5, v4, v3, v2, v1}); !reflect.DeepEqual(rev, []*Vertex{v1, v2, v3, v4, v5, v6}) {
		t.Errorf("reverse of vertex slice failed")
	}
}

type NoopResTest struct {
	resources.NoopRes
}

func (obj *NoopResTest) GroupCmp(r resources.Res) bool {
	res, ok := r.(*NoopResTest)
	if !ok {
		return false
	}

	// TODO: implement this in vertexCmp for *testGrouper instead?
	if strings.Contains(res.Name, ",") { // HACK
		return false // element to be grouped is already grouped!
	}

	// group if they start with the same letter! (helpful hack for testing)
	return obj.Name[0] == res.Name[0]
}

func NewNoopResTest(name string) *NoopResTest {
	obj := &NoopResTest{
		NoopRes: resources.NoopRes{
			BaseRes: resources.BaseRes{
				Name: name,
				MetaParams: resources.MetaParams{
					AutoGroup: true, // always autogroup
				},
			},
		},
	}
	return obj
}

// ListStrCmp compares two lists of strings
func ListStrCmp(a, b []string) bool {
	//fmt.Printf("CMP: %v with %v\n", a, b) // debugging
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GraphCmp compares the topology of two graphs and returns nil if they're equal
// It also compares if grouped element groups are identical
func GraphCmp(g1, g2 *Graph) error {
	if n1, n2 := g1.NumVertices(), g2.NumVertices(); n1 != n2 {
		return fmt.Errorf("graph g1 has %d vertices, while g2 has %d", n1, n2)
	}
	if e1, e2 := g1.NumEdges(), g2.NumEdges(); e1 != e2 {
		return fmt.Errorf("graph g1 has %d edges, while g2 has %d", e1, e2)
	}

	var m = make(map[*Vertex]*Vertex) // g1 to g2 vertex correspondence
Loop:
	// check vertices
	for v1 := range g1.adjacency { // for each vertex in g1

		l1 := strings.Split(v1.GetName(), ",") // make list of everyone's names...
		for _, x1 := range v1.GetGroup() {
			l1 = append(l1, x1.GetName()) // add my contents
		}
		l1 = util.StrRemoveDuplicatesInList(l1) // remove duplicates
		sort.Strings(l1)

		// inner loop
		for v2 := range g2.adjacency { // does it match in g2 ?

			l2 := strings.Split(v2.GetName(), ",")
			for _, x2 := range v2.GetGroup() {
				l2 = append(l2, x2.GetName())
			}
			l2 = util.StrRemoveDuplicatesInList(l2) // remove duplicates
			sort.Strings(l2)

			// does l1 match l2 ?
			if ListStrCmp(l1, l2) { // cmp!
				m[v1] = v2
				continue Loop
			}
		}
		return fmt.Errorf("graph g1, has no match in g2 for: %v", v1.GetName())
	}
	// vertices (and groups) match :)

	// check edges
	for v1 := range g1.adjacency { // for each vertex in g1
		v2 := m[v1] // lookup in map to get correspondance
		// g1.adjacency[v1] corresponds to g2.adjacency[v2]
		if e1, e2 := len(g1.adjacency[v1]), len(g2.adjacency[v2]); e1 != e2 {
			return fmt.Errorf("graph g1, vertex(%v) has %d edges, while g2, vertex(%v) has %d", v1.GetName(), e1, v2.GetName(), e2)
		}

		for vv1, ee1 := range g1.adjacency[v1] {
			vv2 := m[vv1]
			ee2 := g2.adjacency[v2][vv2]

			// these are edges from v1 -> vv1 via ee1 (graph 1)
			// to cmp to edges from v2 -> vv2 via ee2 (graph 2)

			// check: (1) vv1 == vv2 ? (we've already checked this!)
			l1 := strings.Split(vv1.GetName(), ",") // make list of everyone's names...
			for _, x1 := range vv1.GetGroup() {
				l1 = append(l1, x1.GetName()) // add my contents
			}
			l1 = util.StrRemoveDuplicatesInList(l1) // remove duplicates
			sort.Strings(l1)

			l2 := strings.Split(vv2.GetName(), ",")
			for _, x2 := range vv2.GetGroup() {
				l2 = append(l2, x2.GetName())
			}
			l2 = util.StrRemoveDuplicatesInList(l2) // remove duplicates
			sort.Strings(l2)

			// does l1 match l2 ?
			if !ListStrCmp(l1, l2) { // cmp!
				return fmt.Errorf("graph g1 and g2 don't agree on: %v and %v", vv1.GetName(), vv2.GetName())
			}

			// check: (2) ee1 == ee2
			if ee1.Name != ee2.Name {
				return fmt.Errorf("graph g1 edge(%v) doesn't match g2 edge(%v)", ee1.Name, ee2.Name)
			}
		}
	}

	// check meta parameters
	for v1 := range g1.adjacency { // for each vertex in g1
		for v2 := range g2.adjacency { // does it match in g2 ?
			s1, s2 := v1.Meta().Sema, v2.Meta().Sema
			sort.Strings(s1)
			sort.Strings(s2)
			if !reflect.DeepEqual(s1, s2) {
				return fmt.Errorf("vertex %s and vertex %s have different semaphores", v1.GetName(), v2.GetName())
			}
		}
	}

	return nil // success!
}

type testGrouper struct {
	// TODO: this algorithm may not be correct in all cases. replace if needed!
	nonReachabilityGrouper // "inherit" what we want, and reimplement the rest
}

func (ag *testGrouper) name() string {
	return "testGrouper"
}

func (ag *testGrouper) vertexMerge(v1, v2 *Vertex) (v *Vertex, err error) {
	if err := v1.Res.GroupRes(v2.Res); err != nil { // group them first
		return nil, err
	}
	// HACK: update the name so it matches full list of self+grouped
	obj := v1.Res
	names := strings.Split(obj.GetName(), ",") // load in stored names
	for _, n := range obj.GetGroup() {
		names = append(names, n.GetName()) // add my contents
	}
	names = util.StrRemoveDuplicatesInList(names) // remove duplicates
	sort.Strings(names)
	obj.SetName(strings.Join(names, ","))
	return // success or fail, and no need to merge the actual vertices!
}

func (ag *testGrouper) edgeMerge(e1, e2 *Edge) *Edge {
	// HACK: update the name so it makes a union of both names
	n1 := strings.Split(e1.Name, ",") // load
	n2 := strings.Split(e2.Name, ",") // load
	names := append(n1, n2...)
	names = util.StrRemoveDuplicatesInList(names) // remove duplicates
	sort.Strings(names)
	return NewEdge(strings.Join(names, ","))
}

func (g *Graph) fullPrint() (str string) {
	str += "\n"
	for v := range g.adjacency {
		if semas := v.Meta().Sema; len(semas) > 0 {
			str += fmt.Sprintf("* v: %v; sema: %v\n", v.GetName(), semas)
		} else {
			str += fmt.Sprintf("* v: %v\n", v.GetName())
		}
		// TODO: add explicit grouping data?
	}
	for v1 := range g.adjacency {
		for v2, e := range g.adjacency[v1] {
			str += fmt.Sprintf("* e: %v -> %v # %v\n", v1.GetName(), v2.GetName(), e.Name)
		}
	}
	return
}

// helper function
func runGraphCmp(t *testing.T, g1, g2 *Graph) {
	ch := g1.autoGroup(&testGrouper{}) // edits the graph
	for range ch {                     // bleed the channel or it won't run :(
		// pass
	}
	err := GraphCmp(g1, g2)
	if err != nil {
		t.Logf("  actual (g1): %v%v", g1, g1.fullPrint())
		t.Logf("expected (g2): %v%v", g2, g2.fullPrint())
		t.Logf("Cmp error:")
		t.Errorf("%v", err)
	}
}

func TestDurationAssumptions(t *testing.T) {
	var d time.Duration
	if (d == 0) != true {
		t.Errorf("empty time.Duration is no longer equal to zero")
	}
	if (d > 0) != false {
		t.Errorf("empty time.Duration is now greater than zero")
	}
}
