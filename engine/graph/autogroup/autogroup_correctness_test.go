// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

//go:build !root

package autogroup

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
)

// --- 1. VertexMerge Edge Reattachment ---

/*
// TestMergeDiamond: shared predecessor, two outgoing merge
// a1→b, a2→b, b→c1, b→c2   merge c1,c2
//
//  a1  a2          a1  a2
//   \ /             \ /
//    b       >>>     b
//   / \              |
//  c1  c2          c1,c2
*/
func TestMergeDiamond(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		c2 := NewNoopResTest("c2")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(a2, b1, NE("e2"))
		g1.AddEdge(b1, c1, NE("e3"))
		g1.AddEdge(b1, c2, NE("e4"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		b1 := NewNoopResTest("b1")
		c := NewNoopResTest("c1,c2")
		g2.AddEdge(a, b1, NE("e1,e2"))
		g2.AddEdge(b1, c, NE("e3,e4"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMergeWideIncoming: many shared predecessors (different families), merge a's
// p1,q1,r1 → a1; p1,q1,r1 → a2   merge a1,a2
// p,q,r are different families so they don't merge with each other.
*/
func TestMergeWideIncoming(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		p1 := NewNoopResTest("p1")
		q1 := NewNoopResTest("q1")
		r1 := NewNoopResTest("r1")
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		g1.AddEdge(p1, a1, NE("e1"))
		g1.AddEdge(q1, a1, NE("e2"))
		g1.AddEdge(r1, a1, NE("e3"))
		g1.AddEdge(p1, a2, NE("e4"))
		g1.AddEdge(q1, a2, NE("e5"))
		g1.AddEdge(r1, a2, NE("e6"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		p1 := NewNoopResTest("p1")
		q1 := NewNoopResTest("q1")
		r1 := NewNoopResTest("r1")
		a := NewNoopResTest("a1,a2")
		g2.AddEdge(p1, a, NE("e1,e4"))
		g2.AddEdge(q1, a, NE("e2,e5"))
		g2.AddEdge(r1, a, NE("e3,e6"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMergeWideOutgoing: many shared successors (different families), merge a's
// a1→p1,q1,r1; a2→p1,q1,r1   merge a1,a2
// p,q,r are different families so they don't merge with each other.
*/
func TestMergeWideOutgoing(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		p1 := NewNoopResTest("p1")
		q1 := NewNoopResTest("q1")
		r1 := NewNoopResTest("r1")
		g1.AddEdge(a1, p1, NE("e1"))
		g1.AddEdge(a1, q1, NE("e2"))
		g1.AddEdge(a1, r1, NE("e3"))
		g1.AddEdge(a2, p1, NE("e4"))
		g1.AddEdge(a2, q1, NE("e5"))
		g1.AddEdge(a2, r1, NE("e6"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		p1 := NewNoopResTest("p1")
		q1 := NewNoopResTest("q1")
		r1 := NewNoopResTest("r1")
		g2.AddEdge(a, p1, NE("e1,e4"))
		g2.AddEdge(a, q1, NE("e2,e5"))
		g2.AddEdge(a, r1, NE("e3,e6"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMergeLongChain: merge with one vertex having long outgoing chain
// a1→b→c→d, a2 (isolated)   merge a1,a2
//
// a1→b→c→d        a1,a2→b→c→d
// a2         >>>
*/
func TestMergeLongChain(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, c1, NE("e2"))
		g1.AddEdge(c1, d1, NE("e3"))
		g1.AddVertex(a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		g2.AddEdge(a, b1, NE("e1"))
		g2.AddEdge(b1, c1, NE("e2"))
		g2.AddEdge(c1, d1, NE("e3"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMergeReattachBothDirections: incoming + outgoing edges both reattach
// x→a1→y, a2 (isolated)   merge a1,a2
//
// x→a1→y       x→a1,a2→y
// a2       >>>
*/
func TestMergeReattachBothDirections(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		x1 := NewNoopResTest("x1")
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		y1 := NewNoopResTest("y1")
		g1.AddEdge(x1, a1, NE("e1"))
		g1.AddEdge(a1, y1, NE("e2"))
		g1.AddVertex(a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		x1 := NewNoopResTest("x1")
		a := NewNoopResTest("a1,a2")
		y1 := NewNoopResTest("y1")
		g2.AddEdge(x1, a, NE("e1"))
		g2.AddEdge(a, y1, NE("e2"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMergeParallelChains: two parallel chains converging, intermediate merge
// a1→b1→c, a2→b2→c   merge b1,b2
//
// a1→b1→c       a1→       →c
//          >>>     b1,b2
// a2→b2→c       a2→       →c
*/
func TestMergeParallelChains(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		b1 := NewNoopResTest("b1")
		b2 := NewNoopResTest("b2")
		c1 := NewNoopResTest("c1")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, c1, NE("e2"))
		g1.AddEdge(a2, b2, NE("e3"))
		g1.AddEdge(b2, c1, NE("e4"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		b := NewNoopResTest("b1,b2")
		c1 := NewNoopResTest("c1")
		g2.AddEdge(a, b, NE("e1,e3"))
		g2.AddEdge(b, c1, NE("e2,e4"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMergeWithBypass: merge when one vertex has direct + transitive path
// a1→b→c, a1→c, a2→c   merge a1,a2
// The bypass edge a1→c is preserved since a1 can reach c via b.
//
//  a1→b→c       a1,a2→b→c
//  a1→c    >>>  a1,a2→c
//  a2→c
*/
func TestMergeWithBypass(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, c1, NE("e2"))
		g1.AddEdge(a1, c1, NE("e3"))
		g1.AddEdge(a2, c1, NE("e4"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		g2.AddEdge(a, b1, NE("e1"))
		g2.AddEdge(b1, c1, NE("e2"))
		g2.AddEdge(a, c1, NE("e3,e4"))
	}
	runGraphCmp(t, g1, g2)
}

// TestMergeIsolatedPair: simplest merge, no edge work needed
func TestMergeIsolatedPair(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		g1.AddVertex(a1, a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// --- 2. Reachability Prevention ---

// TestNoMergeDirectEdge: direct dependency prevents merge
func TestNoMergeDirectEdge(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		g1.AddEdge(a1, a2, NE("e1"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		g2.AddEdge(a1, a2, NE("e1"))
	}
	runGraphCmp(t, g1, g2)
}

// TestNoMergeTransitive: transitive path prevents merge
func TestNoMergeTransitive(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		a2 := NewNoopResTest("a2")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, a2, NE("e2"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		a2 := NewNoopResTest("a2")
		g2.AddEdge(a1, b1, NE("e1"))
		g2.AddEdge(b1, a2, NE("e2"))
	}
	runGraphCmp(t, g1, g2)
}

// TestNoMergeLongTransitive: long transitive path prevents merge
func TestNoMergeLongTransitive(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		a2 := NewNoopResTest("a2")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, c1, NE("e2"))
		g1.AddEdge(c1, d1, NE("e3"))
		g1.AddEdge(d1, a2, NE("e4"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		a2 := NewNoopResTest("a2")
		g2.AddEdge(a1, b1, NE("e1"))
		g2.AddEdge(b1, c1, NE("e2"))
		g2.AddEdge(c1, d1, NE("e3"))
		g2.AddEdge(d1, a2, NE("e4"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestNoMergeOneDirection: a1,a2 blocked, but a3 merges with one of them
// a1→b→a2, a3 (isolated)
// a1 and a2 are reachable, so they can't merge.
// a3 is not reachable from/to a1, so a3 merges with a1 (or a2).
*/
func TestNoMergeOneDirection(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		a2 := NewNoopResTest("a2")
		a3 := NewNoopResTest("a3")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, a2, NE("e2"))
		g1.AddVertex(a3)
	}
	// a3 merges with a2 (non-reachable from each other). a1 stays because
	// a1→b→a2. After merge: a1→b→a2,a3
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		a := NewNoopResTest("a2,a3")
		g2.AddEdge(a1, b1, NE("e1"))
		g2.AddEdge(b1, a, NE("e2"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestNoMergeMixedReachability: some pairs reachable, others not; partial merge
// a1→b→a2, a3, a4 (a3 and a4 isolated)
// a1<->a2 blocked by reachability. a3, a4 can merge with a1 (or a2).
*/
func TestNoMergeMixedReachability(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		a2 := NewNoopResTest("a2")
		a3 := NewNoopResTest("a3")
		a4 := NewNoopResTest("a4")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(b1, a2, NE("e2"))
		g1.AddVertex(a3)
		g1.AddVertex(a4)
	}
	// a3 and a4 merge with a2 (non-reachable from each other). a1 stays
	// separate because a1→b→a2.
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		a := NewNoopResTest("a2,a3,a4")
		g2.AddEdge(a1, b1, NE("e1"))
		g2.AddEdge(b1, a, NE("e2"))
	}
	runGraphCmp(t, g1, g2)
}

// --- 3. Multi-Step Merge Sequences ---

// TestMultiMerge3Way: 3 same-kind vertices all merge into one
func TestMultiMerge3Way(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		a3 := NewNoopResTest("a3")
		g1.AddVertex(a1, a2, a3)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2,a3")
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// TestMultiMerge5Way: 5-way merge cascade
func TestMultiMerge5Way(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		for i := 1; i <= 5; i++ {
			g1.AddVertex(NewNoopResTest(fmt.Sprintf("a%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2,a3,a4,a5")
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// TestMultiMergeTwoFamilies: two independent families merge separately
func TestMultiMergeTwoFamilies(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		for i := 1; i <= 3; i++ {
			g1.AddVertex(NewNoopResTest(fmt.Sprintf("a%d", i)))
		}
		for i := 1; i <= 3; i++ {
			g1.AddVertex(NewNoopResTest(fmt.Sprintf("b%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2,a3")
		b := NewNoopResTest("b1,b2,b3")
		g2.AddVertex(a, b)
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMultiMergeWithDeps: sequential merges with shared dependency
// a1→b, a2→b, a3→b   merge a1,a2,a3
//
//  a1 a2 a3        a1,a2,a3
//   \ | /    >>>       |
//     b                b
*/
func TestMultiMergeWithDeps(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		a3 := NewNoopResTest("a3")
		b1 := NewNoopResTest("b1")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(a2, b1, NE("e2"))
		g1.AddEdge(a3, b1, NE("e3"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2,a3")
		b1 := NewNoopResTest("b1")
		g2.AddEdge(a, b1, NE("e1,e2,e3"))
	}
	runGraphCmp(t, g1, g2)
}

// --- 4. Complex Graph Structures ---

/*
// TestComplexDiamondStack: two stacked diamonds
// a→{b1,b2}→c→{d1,d2}→e   merge b's then d's
//
//      a               a
//     / \              |
//    b1  b2    >>>   b1,b2
//     \ /              |
//      c               c
//     / \              |
//    d1  d2          d1,d2
//     \ /              |
//      e               e
*/
func TestComplexDiamondStack(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		b2 := NewNoopResTest("b2")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		d2 := NewNoopResTest("d2")
		e1 := NewNoopResTest("e1")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(a1, b2, NE("e2"))
		g1.AddEdge(b1, c1, NE("e3"))
		g1.AddEdge(b2, c1, NE("e4"))
		g1.AddEdge(c1, d1, NE("e5"))
		g1.AddEdge(c1, d2, NE("e6"))
		g1.AddEdge(d1, e1, NE("e7"))
		g1.AddEdge(d2, e1, NE("e8"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b := NewNoopResTest("b1,b2")
		c1 := NewNoopResTest("c1")
		d := NewNoopResTest("d1,d2")
		e1 := NewNoopResTest("e1")
		g2.AddEdge(a1, b, NE("e1,e2"))
		g2.AddEdge(b, c1, NE("e3,e4"))
		g2.AddEdge(c1, d, NE("e5,e6"))
		g2.AddEdge(d, e1, NE("e7,e8"))
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestComplexWideLayer: wide fan-out/in with 5 intermediate vertices
// a→{b1..b5}→c   merge all b's
//
//       a              a
//    /|/|\          |
//  b1 b2 b3 b4 b5  >>> b1,b2,b3,b4,b5
//    \|\|/          |
//       c              c
*/
func TestComplexWideLayer(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		c1 := NewNoopResTest("c1")
		for i := 1; i <= 5; i++ {
			b := NewNoopResTest(fmt.Sprintf("b%d", i))
			g1.AddEdge(a1, b, NE(fmt.Sprintf("ea%d", i)))
			g1.AddEdge(b, c1, NE(fmt.Sprintf("eb%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b := NewNoopResTest("b1,b2,b3,b4,b5")
		c1 := NewNoopResTest("c1")
		g2.AddEdge(a1, b, NE("ea1,ea2,ea3,ea4,ea5"))
		g2.AddEdge(b, c1, NE("eb1,eb2,eb3,eb4,eb5"))
	}
	runGraphCmp(t, g1, g2)
}

// TestDisconnectedSubgraphs: merging works across disconnected components
func TestDisconnectedSubgraphs(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		// component 1: isolated a's
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		g1.AddVertex(a1, a2)
		// component 2: b's with edge to c
		b1 := NewNoopResTest("b1")
		b2 := NewNoopResTest("b2")
		c1 := NewNoopResTest("c1")
		g1.AddEdge(b1, c1, NE("e1"))
		g1.AddEdge(b2, c1, NE("e2"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		b := NewNoopResTest("b1,b2")
		c1 := NewNoopResTest("c1")
		g2.AddVertex(a)
		g2.AddEdge(b, c1, NE("e1,e2"))
	}
	runGraphCmp(t, g1, g2)
}

// TestSingletonKinds: all different families, no merges happen
func TestSingletonKinds(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		g1.AddVertex(a1, b1, c1, d1)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		c1 := NewNoopResTest("c1")
		d1 := NewNoopResTest("d1")
		g2.AddVertex(a1, b1, c1, d1)
	}
	runGraphCmp(t, g1, g2)
}

/*
// TestMixedMergeAndNoMerge: a3 can merge despite edges between a's and b's
// a1→b1, a2→b2, a3 (isolated, same kind as a1,a2)
// a1 and a2 are not reachable from each other (different b targets), and a3
// is isolated, so all three a's merge.
*/
func TestMixedMergeAndNoMerge(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		a3 := NewNoopResTest("a3")
		b1 := NewNoopResTest("b1")
		b2 := NewNoopResTest("b2")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(a2, b2, NE("e2"))
		g1.AddVertex(a3)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2,a3")
		b := NewNoopResTest("b1,b2")
		g2.AddEdge(a, b, NE("e1,e2"))
	}
	runGraphCmp(t, g1, g2)
}

// --- 5. Hierarchical Kind Grouping ---

// TestHierarchicalTwoLevel: two-level hierarchy merges
func TestHierarchicalTwoLevel(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewKindNoopResTest("nooptestkind:foo", "a1")
		a2 := NewKindNoopResTest("nooptestkind:foo:hello", "a2")
		g1.AddVertex(a1, a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// TestHierarchicalThreeLevel: three-level merges in correct order
func TestHierarchicalThreeLevel(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewKindNoopResTest("nooptestkind:foo", "a1")
		a2 := NewKindNoopResTest("nooptestkind:foo:world", "a2")
		a3 := NewKindNoopResTest("nooptestkind:foo:world:big", "a3")
		g1.AddVertex(a1, a2, a3)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2,a3")
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// TestHierarchicalSeparateFamilies: different family prefixes don't merge
func TestHierarchicalSeparateFamilies(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewKindNoopResTest("nooptestkind:foo", "a1")
		b1 := NewKindNoopResTest("nooptestkind:foo", "b1")
		g1.AddVertex(a1, b1)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		b1 := NewNoopResTest("b1")
		g2.AddVertex(a1, b1)
	}
	runGraphCmp(t, g1, g2)
}

// TestHierarchicalWithEdges: hierarchical merge with shared successor
func TestHierarchicalWithEdges(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewKindNoopResTest("nooptestkind:foo", "a1")
		a2 := NewKindNoopResTest("nooptestkind:foo:hello", "a2")
		c1 := NewNoopResTest("c1")
		g1.AddEdge(a1, c1, NE("e1"))
		g1.AddEdge(a2, c1, NE("e2"))
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTest("a1,a2")
		c1 := NewNoopResTest("c1")
		g2.AddEdge(a, c1, NE("e1,e2"))
	}
	runGraphCmp(t, g1, g2)
}

// --- 6. Semaphore / Metadata Preservation ---

// TestSemaPreservedWithEdges: semas merge when edges present
func TestSemaPreservedWithEdges(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTestSema("a1", []string{"s:1"})
		a2 := NewNoopResTestSema("a2", []string{"s:2"})
		b1 := NewNoopResTest("b1")
		g1.AddEdge(a1, b1, NE("e1"))
		g1.AddEdge(a2, b1, NE("e2"))
	}
	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := AutoGroup(&testGrouper{}, g1, debug, logf); err != nil {
		t.Fatalf("AutoGroup: %v", err)
	}
	if g1.NumVertices() != 2 {
		t.Fatalf("expected 2 vertices, got %d", g1.NumVertices())
	}
	for v := range g1.Adjacency() {
		r := v.(engine.Res)
		names := strings.Split(r.Name(), ",")
		sort.Strings(names)
		if ListStrCmp(names, []string{"a1", "a2"}) {
			semas := r.MetaParams().Sema
			sort.Strings(semas)
			expected := []string{"s:1", "s:2"}
			if !ListStrCmp(semas, expected) {
				t.Errorf("merged vertex semas: got %v, want %v", semas, expected)
			}
		}
	}
}

// TestSemaDuplicateRemoval: duplicate semas are deduplicated
func TestSemaDuplicateRemoval(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTestSema("a1", []string{"s:1", "s:2"})
		a2 := NewNoopResTestSema("a2", []string{"s:2", "s:3"})
		g1.AddVertex(a1, a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTestSema("a1,a2", []string{"s:1", "s:2", "s:3"})
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// TestSemaEmptyMerge: empty + non-empty sema lists merge
func TestSemaEmptyMerge(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1") // no semas
		a2 := NewNoopResTestSema("a2", []string{"s:1"})
		g1.AddVertex(a1, a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a := NewNoopResTestSema("a1,a2", []string{"s:1"})
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// --- 7. Determinism ---

// TestDeterminism10 runs autogrouping 20 times on 10 same-kind vertices and
// verifies the result is identical every time.
func TestDeterminism10(t *testing.T) {
	runDeterminismTest(t, 10, 20, false)
}

// TestDeterminismWithEdges runs autogrouping 20 times on a graph with edges
// and verifies consistent results despite map iteration randomness.
func TestDeterminismWithEdges(t *testing.T) {
	for iter := 0; iter < 20; iter++ {
		g1, _ := pgraph.NewGraph("g1")
		{
			a1 := NewNoopResTest("a1")
			a2 := NewNoopResTest("a2")
			a3 := NewNoopResTest("a3")
			a4 := NewNoopResTest("a4")
			b1 := NewNoopResTest("b1")
			g1.AddEdge(a1, b1, NE("e1"))
			g1.AddEdge(a2, b1, NE("e2"))
			g1.AddEdge(a3, b1, NE("e3"))
			g1.AddVertex(a4)
		}
		debug := false
		logf := func(format string, v ...interface{}) {}
		if err := AutoGroup(&testGrouper{}, g1, debug, logf); err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}

		// verify topology
		if g1.NumVertices() != 2 {
			t.Fatalf("iter %d: expected 2 vertices, got %d", iter, g1.NumVertices())
		}
		if g1.NumEdges() != 1 {
			t.Fatalf("iter %d: expected 1 edge, got %d", iter, g1.NumEdges())
		}

		// verify the merged vertex name
		found := false
		for v := range g1.Adjacency() {
			r := v.(engine.Res)
			names := strings.Split(r.Name(), ",")
			sort.Strings(names)
			name := strings.Join(names, ",")
			if name == "a1,a2,a3,a4" {
				found = true
			}
		}
		if !found {
			t.Fatalf("iter %d: expected merged vertex a1,a2,a3,a4", iter)
		}
	}
}

// runDeterminismTest creates n same-kind vertices and runs autogrouping iters
// times, verifying the result is always 1 merged vertex.
func runDeterminismTest(t *testing.T, n, iters int, withEdges bool) {
	t.Helper()
	for iter := 0; iter < iters; iter++ {
		g, _ := pgraph.NewGraph("g")
		for i := 0; i < n; i++ {
			g.AddVertex(NewNoopResTest(fmt.Sprintf("a%d", i)))
		}
		debug := false
		logf := func(format string, v ...interface{}) {}
		if err := AutoGroup(&testGrouper{}, g, debug, logf); err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		if g.NumVertices() != 1 {
			t.Fatalf("iter %d: expected 1 vertex, got %d", iter, g.NumVertices())
		}
		// verify name is deterministic
		for v := range g.Adjacency() {
			r := v.(engine.Res)
			names := strings.Split(r.Name(), ",")
			sort.Strings(names)
			expected := []string{}
			for i := 0; i < n; i++ {
				expected = append(expected, fmt.Sprintf("a%d", i))
			}
			sort.Strings(expected)
			if !ListStrCmp(names, expected) {
				t.Fatalf("iter %d: got name %s, expected %v", iter, r.Name(), expected)
			}
		}
	}
}

// --- 8. Edge Cases ---

// TestSingleVertex: one vertex, no crash, no merge
func TestSingleVertex(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		g1.AddVertex(a1)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		g2.AddVertex(a1)
	}
	runGraphCmp(t, g1, g2)
}

// TestAutoGroupDisabled: disabled autogroup flag prevents merge
func TestAutoGroupDisabled(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		a2.AutoGroupMeta().Disabled = true // disable grouping for a2
		g1.AddVertex(a1, a2)
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		a1 := NewNoopResTest("a1")
		a2 := NewNoopResTest("a2")
		g2.AddVertex(a1, a2)
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeGroupMerge: correctness at moderate scale (50 same-kind vertices)
func TestLargeGroupMerge(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	{
		for i := 0; i < 50; i++ {
			g1.AddVertex(NewNoopResTest(fmt.Sprintf("a%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		names := []string{}
		for i := 0; i < 50; i++ {
			names = append(names, fmt.Sprintf("a%d", i))
		}
		sort.Strings(names)
		a := NewNoopResTest(strings.Join(names, ","))
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// --- 9. Large-Scale Tests ---

// TestLargeChainNoMerge: 100-vertex linear chain where no merging should occur.
// Vertices alternate between families a and b: a0→b0→a1→b1→...→a49→b49
// Since every same-family pair is reachable, no merges happen.
func TestLargeChainNoMerge(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	g2, _ := pgraph.NewGraph("g2")

	var prev1, prev2 *NoopResTest
	for i := 0; i < 50; i++ {
		aName := fmt.Sprintf("a%d", i)
		bName := fmt.Sprintf("b%d", i)
		a1 := NewNoopResTest(aName)
		b1 := NewNoopResTest(bName)
		a2 := NewNoopResTest(aName)
		b2 := NewNoopResTest(bName)

		if prev1 != nil {
			g1.AddEdge(prev1, a1, NE(fmt.Sprintf("e%da", i)))
			g2.AddEdge(prev2, a2, NE(fmt.Sprintf("e%da", i)))
		}
		g1.AddEdge(a1, b1, NE(fmt.Sprintf("e%db", i)))
		g2.AddEdge(a2, b2, NE(fmt.Sprintf("e%db", i)))

		prev1 = b1
		prev2 = b2
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeFanOutMerge: a single root fans out to 100 same-kind vertices that
// all get merged into one. root→{a0,a1,...,a99}  →  root→a0,...,a99
func TestLargeFanOutMerge(t *testing.T) {
	const n = 100
	g1, _ := pgraph.NewGraph("g1")
	{
		root := NewNoopResTest("r1")
		for i := 0; i < n; i++ {
			a := NewNoopResTest(fmt.Sprintf("a%d", i))
			g1.AddEdge(root, a, NE(fmt.Sprintf("e%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		root := NewNoopResTest("r1")
		names := []string{}
		edgeNames := []string{}
		for i := 0; i < n; i++ {
			names = append(names, fmt.Sprintf("a%d", i))
			edgeNames = append(edgeNames, fmt.Sprintf("e%d", i))
		}
		sort.Strings(names)
		sort.Strings(edgeNames)
		a := NewNoopResTest(strings.Join(names, ","))
		g2.AddEdge(root, a, NE(strings.Join(edgeNames, ",")))
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeFanInMerge: 100 same-kind vertices all point to a single sink and
// get merged. {a0,...,a99}→sink  →  a0,...,a99→sink
func TestLargeFanInMerge(t *testing.T) {
	const n = 100
	g1, _ := pgraph.NewGraph("g1")
	{
		sink := NewNoopResTest("s1")
		for i := 0; i < n; i++ {
			a := NewNoopResTest(fmt.Sprintf("a%d", i))
			g1.AddEdge(a, sink, NE(fmt.Sprintf("e%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		sink := NewNoopResTest("s1")
		names := []string{}
		edgeNames := []string{}
		for i := 0; i < n; i++ {
			names = append(names, fmt.Sprintf("a%d", i))
			edgeNames = append(edgeNames, fmt.Sprintf("e%d", i))
		}
		sort.Strings(names)
		sort.Strings(edgeNames)
		a := NewNoopResTest(strings.Join(names, ","))
		g2.AddEdge(a, sink, NE(strings.Join(edgeNames, ",")))
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeWideDiamond: wide diamond with 50 intermediate vertices between a
// source and a sink. source→{b0,...,b49}→sink  →  source→b0,...,b49→sink
func TestLargeWideDiamond(t *testing.T) {
	const n = 50
	g1, _ := pgraph.NewGraph("g1")
	{
		src := NewNoopResTest("s1")
		dst := NewNoopResTest("d1")
		for i := 0; i < n; i++ {
			b := NewNoopResTest(fmt.Sprintf("b%d", i))
			g1.AddEdge(src, b, NE(fmt.Sprintf("ei%d", i)))
			g1.AddEdge(b, dst, NE(fmt.Sprintf("eo%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		src := NewNoopResTest("s1")
		dst := NewNoopResTest("d1")
		names := []string{}
		einNames := []string{}
		eoutNames := []string{}
		for i := 0; i < n; i++ {
			names = append(names, fmt.Sprintf("b%d", i))
			einNames = append(einNames, fmt.Sprintf("ei%d", i))
			eoutNames = append(eoutNames, fmt.Sprintf("eo%d", i))
		}
		sort.Strings(names)
		sort.Strings(einNames)
		sort.Strings(eoutNames)
		b := NewNoopResTest(strings.Join(names, ","))
		g2.AddEdge(src, b, NE(strings.Join(einNames, ",")))
		g2.AddEdge(b, dst, NE(strings.Join(eoutNames, ",")))
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeMultiFamilyLayers: 3 layers of 20 vertices each, with 3 families
// per layer. Families within the same layer merge; cross-layer edges prevent
// cross-layer merging.
//
// Layer 1: a1..a7, b1..b7, c1..c6 (20 vertices, 3 families)
// Layer 2: d1..d7, e1..e7, f1..f6 (20 vertices, 3 families)
// Layer 3: g1..g7, h1..h7, i1..i6 (20 vertices, 3 families)
// Edges: every vertex in layer 1 → every vertex in layer 2,
//
//	every vertex in layer 2 → every vertex in layer 3.
func TestLargeMultiFamilyLayers(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")

	// create 3 layers
	type layerInfo struct {
		families []struct {
			prefix string
			count  int
		}
		vertices []*NoopResTest
	}

	layers := []struct {
		prefix []string
		counts []int
	}{
		{[]string{"a", "b", "c"}, []int{7, 7, 6}},
		{[]string{"d", "e", "f"}, []int{7, 7, 6}},
		{[]string{"g", "h", "i"}, []int{7, 7, 6}},
	}

	allLayers := make([][]*NoopResTest, 3)
	for li, layer := range layers {
		for fi, prefix := range layer.prefix {
			for j := 1; j <= layer.counts[fi]; j++ {
				v := NewNoopResTest(fmt.Sprintf("%s%d", prefix, j))
				allLayers[li] = append(allLayers[li], v)
				g1.AddVertex(v)
			}
		}
	}

	// add edges between layers
	edgeIdx := 0
	for _, v1 := range allLayers[0] {
		for _, v2 := range allLayers[1] {
			g1.AddEdge(v1, v2, NE(fmt.Sprintf("e%d", edgeIdx)))
			edgeIdx++
		}
	}
	for _, v1 := range allLayers[1] {
		for _, v2 := range allLayers[2] {
			g1.AddEdge(v1, v2, NE(fmt.Sprintf("e%d", edgeIdx)))
			edgeIdx++
		}
	}

	// run autogroup
	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := AutoGroup(&testGrouper{}, g1, debug, logf); err != nil {
		t.Fatalf("AutoGroup: %v", err)
	}

	// within each layer, same-letter families should merge
	// layer 1: a1..a7 → one vertex, b1..b7 → one, c1..c6 → one = 3 vertices
	// layer 2: d1..d7, e1..e7, f1..f6 = 3 vertices
	// layer 3: g1..g7, h1..h7, i1..i6 = 3 vertices
	// total = 9 vertices
	if n := g1.NumVertices(); n != 9 {
		t.Errorf("expected 9 vertices after grouping, got %d", n)
		t.Logf("graph: %v%v", g1, fullPrint(g1))
	}

	// verify each expected merged vertex exists
	expectedFamilies := map[string][]string{
		"a": namelist("a", 7),
		"b": namelist("b", 7),
		"c": namelist("c", 6),
		"d": namelist("d", 7),
		"e": namelist("e", 7),
		"f": namelist("f", 6),
		"g": namelist("g", 7),
		"h": namelist("h", 7),
		"i": namelist("i", 6),
	}

	for v := range g1.Adjacency() {
		r := v.(engine.GroupableRes)
		allNames := strings.Split(r.Name(), ",")
		for _, x := range r.GetGroup() {
			allNames = append(allNames, x.Name())
		}
		allNames = util.StrRemoveDuplicatesInList(allNames)
		sort.Strings(allNames)
		if len(allNames) == 0 {
			continue
		}
		prefix := string(allNames[0][0])
		expected, ok := expectedFamilies[prefix]
		if !ok {
			t.Errorf("unexpected family prefix %q in vertex %s", prefix, r.Name())
			continue
		}
		sort.Strings(expected)
		if !ListStrCmp(allNames, expected) {
			t.Errorf("family %q: got %v, expected %v", prefix, allNames, expected)
		}
	}
}

// TestLargeParallelChainsPartialMerge: 20 parallel chains of length 3, where
// the heads (layer 1) are all same-kind, middles (layer 2) all same-kind, and
// tails (layer 3) all same-kind. But since each head reaches its own mid and
// tail transitively, heads can still merge with other heads that share no path
// to each other.
//
// a0→b0→c0
// a1→b1→c1
// ...
// a19→b19→c19
//
// All a's are non-reachable from each other → they merge.
// All b's are non-reachable from each other → they merge.
// All c's are non-reachable from each other → they merge.
// Result: a0..a19 → b0..b19 → c0..c19
func TestLargeParallelChainsPartialMerge(t *testing.T) {
	const n = 20
	g1, _ := pgraph.NewGraph("g1")
	{
		for i := 0; i < n; i++ {
			a := NewNoopResTest(fmt.Sprintf("a%d", i))
			b := NewNoopResTest(fmt.Sprintf("b%d", i))
			c := NewNoopResTest(fmt.Sprintf("c%d", i))
			g1.AddEdge(a, b, NE(fmt.Sprintf("eab%d", i)))
			g1.AddEdge(b, c, NE(fmt.Sprintf("ebc%d", i)))
		}
	}
	g2, _ := pgraph.NewGraph("g2")
	{
		aNames := []string{}
		bNames := []string{}
		cNames := []string{}
		eabNames := []string{}
		ebcNames := []string{}
		for i := 0; i < n; i++ {
			aNames = append(aNames, fmt.Sprintf("a%d", i))
			bNames = append(bNames, fmt.Sprintf("b%d", i))
			cNames = append(cNames, fmt.Sprintf("c%d", i))
			eabNames = append(eabNames, fmt.Sprintf("eab%d", i))
			ebcNames = append(ebcNames, fmt.Sprintf("ebc%d", i))
		}
		sort.Strings(aNames)
		sort.Strings(bNames)
		sort.Strings(cNames)
		sort.Strings(eabNames)
		sort.Strings(ebcNames)
		a := NewNoopResTest(strings.Join(aNames, ","))
		b := NewNoopResTest(strings.Join(bNames, ","))
		c := NewNoopResTest(strings.Join(cNames, ","))
		g2.AddEdge(a, b, NE(strings.Join(eabNames, ",")))
		g2.AddEdge(b, c, NE(strings.Join(ebcNames, ",")))
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeMixedReachabilityGrid: 10x2 grid where columns have same-kind
// vertices connected through an intermediate row, preventing some merges.
//
// a0→x→a1, a2→x→a3, a4→x→a5, ...
// This creates 5 independent chains (a0→x0→a1, a2→x1→a3, etc.)
// Within each chain: a_even and a_odd can't merge (reachable).
// Across chains: a_even's can merge with other a_even's, and a_odd's with
// other a_odd's (not reachable).
func TestLargeMixedReachabilityGrid(t *testing.T) {
	const chains = 5
	g1, _ := pgraph.NewGraph("g1")
	{
		for i := 0; i < chains; i++ {
			aEven := NewNoopResTest(fmt.Sprintf("a%d", i*2))
			x := NewNoopResTest(fmt.Sprintf("x%d", i))
			aOdd := NewNoopResTest(fmt.Sprintf("a%d", i*2+1))
			g1.AddEdge(aEven, x, NE(fmt.Sprintf("e%da", i)))
			g1.AddEdge(x, aOdd, NE(fmt.Sprintf("e%db", i)))
		}
	}

	// run autogroup manually to verify structure
	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := AutoGroup(&testGrouper{}, g1, debug, logf); err != nil {
		t.Fatalf("AutoGroup: %v", err)
	}

	// x0..x4 merge (all non-reachable from each other)
	// a_even's (a0,a2,a4,a6,a8) merge (non-reachable from each other)
	// a_odd's (a1,a3,a5,a7,a9) merge (non-reachable from each other)
	// Result: 3 vertices, 2 edges
	if n := g1.NumVertices(); n != 3 {
		t.Errorf("expected 3 vertices, got %d", n)
		t.Logf("graph: %v%v", g1, fullPrint(g1))
	}
	if n := g1.NumEdges(); n != 2 {
		t.Errorf("expected 2 edges, got %d", n)
	}
}

// TestLargeIsolated200: 200 isolated vertices across 4 families of 50 each.
// All within each family merge; no cross-family merge.
func TestLargeIsolated200(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1")
	g2, _ := pgraph.NewGraph("g2")

	families := []string{"a", "b", "c", "d"}
	for _, prefix := range families {
		names := []string{}
		for i := 0; i < 50; i++ {
			name := fmt.Sprintf("%s%d", prefix, i)
			g1.AddVertex(NewNoopResTest(name))
			names = append(names, name)
		}
		sort.Strings(names)
		g2.AddVertex(NewNoopResTest(strings.Join(names, ",")))
	}
	runGraphCmp(t, g1, g2)
}

// TestLargeDiamondChain: chain of 10 diamonds, each with 2 intermediate
// vertices. s→{m0_0,m0_1}→{m1_0,m1_1}→...→{m9_0,m9_1}→t
// The m pairs at each level merge because they share predecessors/successors
// and are not reachable from each other.
func TestLargeDiamondChain(t *testing.T) {
	const levels = 10
	g1, _ := pgraph.NewGraph("g1")
	{
		src := NewNoopResTest("s1")
		prev := []pgraph.Vertex{src}
		for level := 0; level < levels; level++ {
			m0 := NewNoopResTest(fmt.Sprintf("m%d", level*2))
			m1 := NewNoopResTest(fmt.Sprintf("m%d", level*2+1))
			for _, p := range prev {
				g1.AddEdge(p, m0, NE(fmt.Sprintf("e%d_%s_0", level, p.(engine.Res).Name())))
				g1.AddEdge(p, m1, NE(fmt.Sprintf("e%d_%s_1", level, p.(engine.Res).Name())))
			}
			prev = []pgraph.Vertex{m0, m1}
		}
		sink := NewNoopResTest("t1")
		for _, p := range prev {
			g1.AddEdge(p, sink, NE(fmt.Sprintf("efinal_%s", p.(engine.Res).Name())))
		}
	}

	// run autogroup
	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := AutoGroup(&testGrouper{}, g1, debug, logf); err != nil {
		t.Fatalf("AutoGroup: %v", err)
	}

	// At each level, the pair of m vertices should merge.
	// Result: s1 → m0,m1 → m2,m3 → ... → m18,m19 → t1
	// = 1 (source) + 10 (merged pairs) + 1 (sink) = 12 vertices
	if n := g1.NumVertices(); n != 12 {
		t.Errorf("expected 12 vertices, got %d", n)
		t.Logf("graph: %v%v", g1, fullPrint(g1))
	}

	// Should be a linear chain of 11 edges
	if n := g1.NumEdges(); n != 11 {
		t.Errorf("expected 11 edges, got %d", n)
	}
}

// namelist generates a sorted list of names like ["a1", "a2", ..., "aN"]
func namelist(prefix string, n int) []string {
	names := []string{}
	for i := 1; i <= n; i++ {
		names = append(names, fmt.Sprintf("%s%d", prefix, i))
	}
	sort.Strings(names)
	return names
}
