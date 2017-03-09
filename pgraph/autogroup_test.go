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
	"testing"
)

// all of the following test cases are laid out with the following semantics:
// * vertices which start with the same single letter are considered "like"
// * "like" elements should be merged
// * vertices can have any integer after their single letter "family" type
// * grouped vertices should have a name with a comma separated list of names
// * edges follow the same conventions about grouping

// empty graph
func TestPgraphGrouping1(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	g2 := NewGraph("g2") // expected result
	runGraphCmp(t, g1, g2)
}

// single vertex
func TestPgraphGrouping2(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{                    // grouping to limit variable scope
		a1 := NewVertex(NewNoopResTest("a1"))
		g1.AddVertex(a1)
	}
	g2 := NewGraph("g2") // expected result
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		g2.AddVertex(a1)
	}
	runGraphCmp(t, g1, g2)
}

// two vertices
func TestPgraphGrouping3(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		g1.AddVertex(a1, b1)
	}
	g2 := NewGraph("g2") // expected result
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		g2.AddVertex(a1, b1)
	}
	runGraphCmp(t, g1, g2)
}

// two vertices merge
func TestPgraphGrouping4(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		g1.AddVertex(a1, a2)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// three vertices merge
func TestPgraphGrouping5(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		a3 := NewVertex(NewNoopResTest("a3"))
		g1.AddVertex(a1, a2, a3)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2,a3"))
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// three vertices, two merge
func TestPgraphGrouping6(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		g1.AddVertex(a1, a2, b1)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		g2.AddVertex(a, b1)
	}
	runGraphCmp(t, g1, g2)
}

// four vertices, three merge
func TestPgraphGrouping7(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		a3 := NewVertex(NewNoopResTest("a3"))
		b1 := NewVertex(NewNoopResTest("b1"))
		g1.AddVertex(a1, a2, a3, b1)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2,a3"))
		b1 := NewVertex(NewNoopResTest("b1"))
		g2.AddVertex(a, b1)
	}
	runGraphCmp(t, g1, g2)
}

// four vertices, two&two merge
func TestPgraphGrouping8(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		b2 := NewVertex(NewNoopResTest("b2"))
		g1.AddVertex(a1, a2, b1, b2)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b := NewVertex(NewNoopResTest("b1,b2"))
		g2.AddVertex(a, b)
	}
	runGraphCmp(t, g1, g2)
}

// five vertices, two&three merge
func TestPgraphGrouping9(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		b2 := NewVertex(NewNoopResTest("b2"))
		b3 := NewVertex(NewNoopResTest("b3"))
		g1.AddVertex(a1, a2, b1, b2, b3)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b := NewVertex(NewNoopResTest("b1,b2,b3"))
		g2.AddVertex(a, b)
	}
	runGraphCmp(t, g1, g2)
}

// three unique vertices
func TestPgraphGrouping10(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		c1 := NewVertex(NewNoopResTest("c1"))
		g1.AddVertex(a1, b1, c1)
	}
	g2 := NewGraph("g2") // expected result
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		c1 := NewVertex(NewNoopResTest("c1"))
		g2.AddVertex(a1, b1, c1)
	}
	runGraphCmp(t, g1, g2)
}

// three unique vertices, two merge
func TestPgraphGrouping11(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		b2 := NewVertex(NewNoopResTest("b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		g1.AddVertex(a1, b1, b2, c1)
	}
	g2 := NewGraph("g2") // expected result
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b := NewVertex(NewNoopResTest("b1,b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		g2.AddVertex(a1, b, c1)
	}
	runGraphCmp(t, g1, g2)
}

// simple merge 1
// a1   a2         a1,a2
//   \ /     >>>     |     (arrows point downwards)
//    b              b
func TestPgraphGrouping12(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a2, b1, e2)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		e := NewEdge("e1,e2")
		g2.AddEdge(a, b1, e)
	}
	runGraphCmp(t, g1, g2)
}

// simple merge 2
//    b              b
//   / \     >>>     |     (arrows point downwards)
// a1   a2         a1,a2
func TestPgraphGrouping13(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		g1.AddEdge(b1, a1, e1)
		g1.AddEdge(b1, a2, e2)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		e := NewEdge("e1,e2")
		g2.AddEdge(b1, a, e)
	}
	runGraphCmp(t, g1, g2)
}

// triple merge
// a1 a2  a3         a1,a2,a3
//   \ | /     >>>       |      (arrows point downwards)
//     b                 b
func TestPgraphGrouping14(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		a3 := NewVertex(NewNoopResTest("a3"))
		b1 := NewVertex(NewNoopResTest("b1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		e3 := NewEdge("e3")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a2, b1, e2)
		g1.AddEdge(a3, b1, e3)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2,a3"))
		b1 := NewVertex(NewNoopResTest("b1"))
		e := NewEdge("e1,e2,e3")
		g2.AddEdge(a, b1, e)
	}
	runGraphCmp(t, g1, g2)
}

// chain merge
//    a1             a1
//   /  \             |
// b1    b2   >>>   b1,b2   (arrows point downwards)
//   \  /             |
//    c1             c1
func TestPgraphGrouping15(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		b2 := NewVertex(NewNoopResTest("b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		e3 := NewEdge("e3")
		e4 := NewEdge("e4")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a1, b2, e2)
		g1.AddEdge(b1, c1, e3)
		g1.AddEdge(b2, c1, e4)
	}
	g2 := NewGraph("g2") // expected result
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b := NewVertex(NewNoopResTest("b1,b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1,e2")
		e2 := NewEdge("e3,e4")
		g2.AddEdge(a1, b, e1)
		g2.AddEdge(b, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// re-attach 1 (outer)
// technically the second possibility is valid too, depending on which order we
// merge edges in, and if we don't filter out any unnecessary edges afterwards!
// a1    a2         a1,a2        a1,a2
//  |   /             |            |  \
// b1  /      >>>    b1     OR    b1  /   (arrows point downwards)
//  | /               |            | /
// c1                c1           c1
func TestPgraphGrouping16(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		e3 := NewEdge("e3")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(b1, c1, e2)
		g1.AddEdge(a2, c1, e3)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1,e3")
		e2 := NewEdge("e2,e3") // e3 gets "merged through" to BOTH edges!
		g2.AddEdge(a, b1, e1)
		g2.AddEdge(b1, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// re-attach 2 (inner)
// a1    b2          a1
//  |   /             |
// b1  /      >>>   b1,b2   (arrows point downwards)
//  | /               |
// c1                c1
func TestPgraphGrouping17(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b1 := NewVertex(NewNoopResTest("b1"))
		b2 := NewVertex(NewNoopResTest("b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		e3 := NewEdge("e3")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(b1, c1, e2)
		g1.AddEdge(b2, c1, e3)
	}
	g2 := NewGraph("g2") // expected result
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b := NewVertex(NewNoopResTest("b1,b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2,e3")
		g2.AddEdge(a1, b, e1)
		g2.AddEdge(b, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// re-attach 3 (double)
// similar to "re-attach 1", technically there is a second possibility for this
// a2   a1    b2         a1,a2
//   \   |   /             |
//    \ b1  /      >>>   b1,b2   (arrows point downwards)
//     \ | /               |
//      c1                c1
func TestPgraphGrouping18(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		b1 := NewVertex(NewNoopResTest("b1"))
		b2 := NewVertex(NewNoopResTest("b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		e3 := NewEdge("e3")
		e4 := NewEdge("e4")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(b1, c1, e2)
		g1.AddEdge(a2, c1, e3)
		g1.AddEdge(b2, c1, e4)
	}
	g2 := NewGraph("g2") // expected result
	{
		a := NewVertex(NewNoopResTest("a1,a2"))
		b := NewVertex(NewNoopResTest("b1,b2"))
		c1 := NewVertex(NewNoopResTest("c1"))
		e1 := NewEdge("e1,e3")
		e2 := NewEdge("e2,e3,e4") // e3 gets "merged through" to BOTH edges!
		g2.AddEdge(a, b, e1)
		g2.AddEdge(b, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// connected merge 0, (no change!)
// a1            a1
//   \     >>>     \     (arrows point downwards)
//    a2            a2
func TestPgraphGroupingConnected0(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		e1 := NewEdge("e1")
		g1.AddEdge(a1, a2, e1)
	}
	g2 := NewGraph("g2") // expected result ?
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		a2 := NewVertex(NewNoopResTest("a2"))
		e1 := NewEdge("e1")
		g2.AddEdge(a1, a2, e1)
	}
	runGraphCmp(t, g1, g2)
}

// connected merge 1, (no change!)
// a1              a1
//   \               \
//    b      >>>      b      (arrows point downwards)
//     \               \
//      a2              a2
func TestPgraphGroupingConnected1(t *testing.T) {
	g1 := NewGraph("g1") // original graph
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b := NewVertex(NewNoopResTest("b"))
		a2 := NewVertex(NewNoopResTest("a2"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		g1.AddEdge(a1, b, e1)
		g1.AddEdge(b, a2, e2)
	}
	g2 := NewGraph("g2") // expected result ?
	{
		a1 := NewVertex(NewNoopResTest("a1"))
		b := NewVertex(NewNoopResTest("b"))
		a2 := NewVertex(NewNoopResTest("a2"))
		e1 := NewEdge("e1")
		e2 := NewEdge("e2")
		g2.AddEdge(a1, b, e1)
		g2.AddEdge(b, a2, e2)
	}
	runGraphCmp(t, g1, g2)
}
