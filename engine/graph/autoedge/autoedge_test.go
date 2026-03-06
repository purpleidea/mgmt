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

package autoedge

import (
	"context"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
)

func init() {
	engine.RegisterResource("autoedgetest", func() engine.Res { return &testRes{} })
	engine.RegisterResource("autoedgetestkind2", func() engine.Res { return &testRes{} })
}

// testUID is a UID type used for testing. Two testUIDs match if they have the
// same path field, following the same pattern as FileUID.
type testUID struct {
	engine.BaseUID

	path string
}

func (obj *testUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*testUID)
	if !ok {
		return false
	}
	return obj.path == res.path
}

// testUID2 is a second UID type used to test type-based index isolation. It
// should never match testUID even with the same path.
type testUID2 struct {
	engine.BaseUID

	path string
}

func (obj *testUID2) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*testUID2)
	if !ok {
		return false
	}
	return obj.path == res.path
}

// testAutoEdge implements the AutoEdge interface for testing.
type testAutoEdge struct {
	edges [][]engine.ResUID
	idx   int
}

func (obj *testAutoEdge) Next() []engine.ResUID {
	if obj.idx >= len(obj.edges) {
		return nil
	}
	uids := obj.edges[obj.idx]
	obj.idx++
	return uids
}

func (obj *testAutoEdge) Test(input []bool) bool {
	return obj.idx < len(obj.edges) // continue if more batches
}

// testAutoEdgeStopOnMiss implements AutoEdge with Test() that stops on the
// first miss, following the pattern used by FileResAutoEdges.
type testAutoEdgeStopOnMiss struct {
	edges [][]engine.ResUID
	idx   int
}

func (obj *testAutoEdgeStopOnMiss) Next() []engine.ResUID {
	if obj.idx >= len(obj.edges) {
		return nil
	}
	uids := obj.edges[obj.idx]
	obj.idx++
	return uids
}

func (obj *testAutoEdgeStopOnMiss) Test(input []bool) bool {
	for _, found := range input {
		if !found {
			return false // stop on first miss
		}
	}
	return obj.idx < len(obj.edges)
}

// testRes is a minimal resource implementation for autoedge testing.
type testRes struct {
	traits.Base
	traits.Edgeable

	init *engine.Init

	autoEdge engine.AutoEdge
	uids     []engine.ResUID
}

func (obj *testRes) Default() engine.Res {
	return &testRes{}
}

func (obj *testRes) Validate() error {
	return nil
}

func (obj *testRes) Init(init *engine.Init) error {
	obj.init = init
	return nil
}

func (obj *testRes) Cleanup() error {
	return nil
}

func (obj *testRes) Watch(context.Context) error {
	return nil
}

func (obj *testRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	return true, nil
}

func (obj *testRes) Cmp(r engine.Res) error {
	_, ok := r.(*testRes)
	if !ok {
		return fmt.Errorf("not a testRes")
	}
	return nil
}

func (obj *testRes) UIDs() []engine.ResUID {
	if obj.uids != nil {
		return obj.uids
	}
	var reversed = false
	return []engine.ResUID{
		&testUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			path: obj.Name(),
		},
	}
}

func (obj *testRes) AutoEdges() (engine.AutoEdge, error) {
	return obj.autoEdge, nil
}

func newTestRes(kind, name string) *testRes {
	n, err := engine.NewNamedResource(kind, name)
	if err != nil {
		panic(fmt.Sprintf("unexpected error: %+v", err))
	}
	r, ok := n.(*testRes)
	if !ok {
		panic("not a testRes")
	}
	return r
}

func testLogf(t *testing.T) func(format string, v ...interface{}) {
	return func(format string, v ...interface{}) {
		if testing.Verbose() {
			t.Logf("autoedge: "+format, v...)
		}
	}
}

// TestAutoEdge1 tests an empty graph.
func TestAutoEdge1(t *testing.T) {
	g, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges, got %d", count)
	}
}

// TestAutoEdge2 tests a single vertex (no self-edges).
func TestAutoEdge2(t *testing.T) {
	g, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	r := newTestRes("autoedgetest", "a")
	var reversed = true
	r.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "a",
				},
			},
		},
	}
	g.AddVertex(r)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges (no self-edges), got %d", count)
	}
}

// TestAutoEdge3 tests matching UIDs between two resources with a reversed edge.
func TestAutoEdge3(t *testing.T) {
	g, err := pgraph.NewGraph("g3")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b", // match vertex b
				},
			},
		},
	}

	g.AddVertex(a, b)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	// reversed=true means edge goes from matched vertex (b) -> requester (a)
	if e := g.FindEdge(b, a); e == nil {
		t.Errorf("expected edge b -> a")
	}
	if count := g.NumEdges(); count != 1 {
		t.Errorf("expected 1 edge, got %d", count)
	}
}

// TestAutoEdge4 tests a non-reversed (normal direction) edge.
func TestAutoEdge4(t *testing.T) {
	g, err := pgraph.NewGraph("g4")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	var reversed = false
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
			},
		},
	}

	g.AddVertex(a, b)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	// reversed=false means edge goes from requester (a) -> matched vertex (b)
	if e := g.FindEdge(a, b); e == nil {
		t.Errorf("expected edge a -> b")
	}
	if count := g.NumEdges(); count != 1 {
		t.Errorf("expected 1 edge, got %d", count)
	}
}

// TestAutoEdge5 tests non-matching UIDs.
func TestAutoEdge5(t *testing.T) {
	g, err := pgraph.NewGraph("g5")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "nonexistent",
				},
			},
		},
	}

	g.AddVertex(a, b)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges, got %d", count)
	}
}

// TestAutoEdge6 tests type-based index isolation: a testUID should never match
// a testUID2 even if they have the same path.
func TestAutoEdge6(t *testing.T) {
	g, err := pgraph.NewGraph("g6")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	// b has a testUID2 instead of testUID
	var bReversed = false
	b.uids = []engine.ResUID{
		&testUID2{
			BaseUID: engine.BaseUID{
				Name:     "b",
				Kind:     "autoedgetest",
				Reversed: &bReversed,
			},
			path: "b",
		},
	}

	// a seeks testUID with path "b", but b only has testUID2
	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
			},
		},
	}

	g.AddVertex(a, b)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges (type mismatch), got %d", count)
	}
}

// TestAutoEdge7 tests multiple UIDs per resource.
func TestAutoEdge7(t *testing.T) {
	g, err := pgraph.NewGraph("g7")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	// b has two UIDs
	var bReversed = false
	b.uids = []engine.ResUID{
		&testUID{
			BaseUID: engine.BaseUID{
				Name:     "b",
				Kind:     "autoedgetest",
				Reversed: &bReversed,
			},
			path: "b-primary",
		},
		&testUID{
			BaseUID: engine.BaseUID{
				Name:     "b",
				Kind:     "autoedgetest",
				Reversed: &bReversed,
			},
			path: "b-alias",
		},
	}

	// a seeks uid matching "b-alias"
	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b-alias",
				},
			},
		},
	}

	g.AddVertex(a, b)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if e := g.FindEdge(b, a); e == nil {
		t.Errorf("expected edge b -> a via alias UID")
	}
	if count := g.NumEdges(); count != 1 {
		t.Errorf("expected 1 edge, got %d", count)
	}
}

// TestAutoEdge8 tests multiple Next/Test batches.
func TestAutoEdge8(t *testing.T) {
	g, err := pgraph.NewGraph("g8")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")

	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
			},
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "c",
				},
			},
		},
	}

	g.AddVertex(a, b, c)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if e := g.FindEdge(b, a); e == nil {
		t.Errorf("expected edge b -> a")
	}
	if e := g.FindEdge(c, a); e == nil {
		t.Errorf("expected edge c -> a")
	}
	if count := g.NumEdges(); count != 2 {
		t.Errorf("expected 2 edges, got %d", count)
	}
}

// TestAutoEdge9 tests that nil AutoEdges and disabled resources are handled.
func TestAutoEdge9(t *testing.T) {
	g, err := pgraph.NewGraph("g9")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")

	// a has nil autoEdge (returns nil, nil from AutoEdges)
	a.autoEdge = nil

	// b is disabled
	b.SetAutoEdgeMeta(&engine.AutoEdgeMeta{Disabled: true})

	var reversed = true
	c.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "c",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b", // try to match b, but b is disabled
				},
			},
		},
	}

	g.AddVertex(a, b, c)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges (disabled resource), got %d", count)
	}
}

// TestAutoEdge10 tests Test() stopping on miss (first-match-wins pattern).
func TestAutoEdge10(t *testing.T) {
	g, err := pgraph.NewGraph("g10")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")

	var reversed = true
	a.autoEdge = &testAutoEdgeStopOnMiss{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "nonexistent", // miss
				},
			},
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "c", // this should never be reached
				},
			},
		},
	}

	g.AddVertex(a, b, c)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	// second batch should not have been processed
	if e := g.FindEdge(c, a); e != nil {
		t.Errorf("expected no edge c -> a because Test() should have stopped")
	}
	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges, got %d", count)
	}
}

// TestAutoEdge11 tests a large graph (50-vertex chain) for correctness.
func TestAutoEdge11(t *testing.T) {
	g, err := pgraph.NewGraph("g11")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	const n = 50
	resources := make([]*testRes, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("r%d", i)
		r := newTestRes("autoedgetest", name)

		// Each resource (except the first) seeks a reversed edge to the
		// previous one, forming a chain: r0 -> r1 -> r2 -> ...
		if i > 0 {
			prev := fmt.Sprintf("r%d", i-1)
			var reversed = true
			r.autoEdge = &testAutoEdge{
				edges: [][]engine.ResUID{
					{
						&testUID{
							BaseUID: engine.BaseUID{
								Name:     name,
								Kind:     "autoedgetest",
								Reversed: &reversed,
							},
							path: prev,
						},
					},
				},
			}
		}
		resources[i] = r
		g.AddVertex(r)
	}

	if err := AutoEdge(g, false, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	// Check we have exactly n-1 edges forming the chain.
	if count := g.NumEdges(); count != n-1 {
		t.Errorf("expected %d edges, got %d", n-1, count)
	}
	for i := 1; i < n; i++ {
		if e := g.FindEdge(resources[i-1], resources[i]); e == nil {
			t.Errorf("expected edge r%d -> r%d", i-1, i)
		}
	}
}

// TestAutoEdge12 tests determinism across 10 runs.
func TestAutoEdge12(t *testing.T) {
	buildGraph := func() (*pgraph.Graph, []*testRes) {
		g, err := pgraph.NewGraph("gdet")
		if err != nil {
			t.Fatalf("err: %+v", err)
		}

		resources := []*testRes{}
		for i := 0; i < 10; i++ {
			r := newTestRes("autoedgetest", fmt.Sprintf("r%d", i))
			resources = append(resources, r)
			g.AddVertex(r)
		}

		var reversed = true
		// r0 seeks edges to r1..r9
		uids := []engine.ResUID{}
		for i := 1; i < 10; i++ {
			uids = append(uids, &testUID{
				BaseUID: engine.BaseUID{
					Name:     "r0",
					Kind:     "autoedgetest",
					Reversed: &reversed,
				},
				path: fmt.Sprintf("r%d", i),
			})
		}
		resources[0].autoEdge = &testAutoEdge{
			edges: [][]engine.ResUID{uids},
		}

		return g, resources
	}

	// Run 10 times and collect edge counts.
	var firstCount int
	for i := 0; i < 10; i++ {
		g, _ := buildGraph()
		if err := AutoEdge(g, false, testLogf(t)); err != nil {
			t.Fatalf("err: %+v", err)
		}
		count := g.NumEdges()
		if i == 0 {
			firstCount = count
		} else if count != firstCount {
			t.Errorf("non-deterministic: run %d got %d edges, expected %d", i, count, firstCount)
		}
	}
}

// TestAutoEdge13 tests redundant edge skipping: if b->a and c->b already exist
// when c tries to add c->a, it should be skipped as transitive. Processing
// order is alphabetical: a, b, c. So b adds b->a first, then c adds c->b and
// skips c->a because c->b->a already exists.
func TestAutoEdge13(t *testing.T) {
	g, err := pgraph.NewGraph("g13")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")

	var reversed = false
	// b seeks edge to a
	b.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "b",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "a",
				},
			},
		},
	}

	// c seeks edges to b and a in a single batch, so c->b is added first,
	// then c->a is checked against the path c->b->a.
	c.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "c",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "c",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "a",
				},
			},
		},
	}

	g.AddVertex(a, b, c)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	// b->a and c->b should exist
	if e := g.FindEdge(b, a); e == nil {
		t.Errorf("expected edge b -> a")
	}
	if e := g.FindEdge(c, b); e == nil {
		t.Errorf("expected edge c -> b")
	}

	// c->a should be skipped as redundant (c -> b -> a already exists)
	if e := g.FindEdge(c, a); e != nil {
		t.Errorf("expected edge c -> a to be skipped (transitive)")
	}
}

// TestAutoEdge14 tests that direct duplicate edges are skipped.
func TestAutoEdge14(t *testing.T) {
	g, err := pgraph.NewGraph("g14")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	// Pre-add edge a -> b
	g.AddVertex(a, b)
	g.AddEdge(a, b, &engine.Edge{Name: "pre-existing"})

	var reversed = false
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
			},
		},
	}

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	// Should still be exactly 1 edge, the pre-existing one.
	if count := g.NumEdges(); count != 1 {
		t.Errorf("expected 1 edge (no duplicate), got %d", count)
	}
	e := g.FindEdge(a, b)
	if e == nil {
		t.Fatalf("expected edge a -> b")
	}
	if e.String() != "pre-existing" {
		t.Errorf("expected pre-existing edge, got %s", e)
	}
}

// TestAutoEdge15 tests that edges are still added when the destination is not
// reachable from the source through other paths.
func TestAutoEdge15(t *testing.T) {
	g, err := pgraph.NewGraph("g15")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")

	var reversed = false
	// a and c both seek edges to b (independent, no transitive path)
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
			},
		},
	}
	c.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "c",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "b",
				},
			},
		},
	}

	g.AddVertex(a, b, c)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if e := g.FindEdge(a, b); e == nil {
		t.Errorf("expected edge a -> b")
	}
	if e := g.FindEdge(c, b); e == nil {
		t.Errorf("expected edge c -> b")
	}
}

// TestAutoEdge16 tests that non-EdgeableRes vertices in the graph are skipped.
func TestAutoEdge16(t *testing.T) {
	g, err := pgraph.NewGraph("g16")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	// Add a non-EdgeableRes vertex to the graph.
	nonEdgeable := &pgraph.SelfVertex{Name: "noedge", Graph: g}

	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&testUID{
					BaseUID: engine.BaseUID{
						Name:     "a",
						Kind:     "autoedgetest",
						Reversed: &reversed,
					},
					path: "noedge",
				},
			},
		},
	}

	g.AddVertex(a, nonEdgeable)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if count := g.NumEdges(); count != 0 {
		t.Errorf("expected 0 edges, got %d", count)
	}
}

// TestAutoEdge17 tests BaseUID fallback: when a seeking UID uses BaseUID (not a
// specialized type), the full vertex list is checked.
func TestAutoEdge17(t *testing.T) {
	g, err := pgraph.NewGraph("g17")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")

	// b's UID is a BaseUID
	var bReversed = false
	b.uids = []engine.ResUID{
		&engine.BaseUID{
			Name:     "shared-name",
			Kind:     "autoedgetest",
			Reversed: &bReversed,
		},
	}

	// a seeks a BaseUID with the same name
	var reversed = true
	a.autoEdge = &testAutoEdge{
		edges: [][]engine.ResUID{
			{
				&engine.BaseUID{
					Name:     "shared-name",
					Kind:     "autoedgetest",
					Reversed: &reversed,
				},
			},
		},
	}

	g.AddVertex(a, b)

	if err := AutoEdge(g, true, testLogf(t)); err != nil {
		t.Fatalf("err: %+v", err)
	}

	if e := g.FindEdge(b, a); e == nil {
		t.Errorf("expected edge b -> a via BaseUID fallback")
	}
}

// TestUIDExistsInUIDs1 tests with an empty list.
func TestUIDExistsInUIDs1(t *testing.T) {
	var reversed = false
	uid := &testUID{
		BaseUID: engine.BaseUID{
			Name:     "x",
			Kind:     "autoedgetest",
			Reversed: &reversed,
		},
		path: "x",
	}

	if UIDExistsInUIDs(uid, []engine.ResUID{}) {
		t.Errorf("expected false for empty list")
	}
}

// TestUIDExistsInUIDs2 tests with a single match.
func TestUIDExistsInUIDs2(t *testing.T) {
	var reversed = false
	uid1 := &testUID{
		BaseUID: engine.BaseUID{
			Name:     "x",
			Kind:     "autoedgetest",
			Reversed: &reversed,
		},
		path: "x",
	}
	uid2 := &testUID{
		BaseUID: engine.BaseUID{
			Name:     "y",
			Kind:     "autoedgetest",
			Reversed: &reversed,
		},
		path: "x", // same path, should match
	}

	if !UIDExistsInUIDs(uid1, []engine.ResUID{uid2}) {
		t.Errorf("expected true for matching path")
	}
}

// TestUIDExistsInUIDs3 tests with multiple UIDs where one matches.
func TestUIDExistsInUIDs3(t *testing.T) {
	var reversed = false
	uid := &testUID{
		BaseUID: engine.BaseUID{
			Name:     "x",
			Kind:     "autoedgetest",
			Reversed: &reversed,
		},
		path: "target",
	}
	list := []engine.ResUID{
		&testUID{
			BaseUID: engine.BaseUID{
				Name:     "a",
				Kind:     "autoedgetest",
				Reversed: &reversed,
			},
			path: "nope",
		},
		&testUID{
			BaseUID: engine.BaseUID{
				Name:     "b",
				Kind:     "autoedgetest",
				Reversed: &reversed,
			},
			path: "target",
		},
	}

	if !UIDExistsInUIDs(uid, list) {
		t.Errorf("expected true for matching uid in list")
	}
}

// TestIsReachable1 tests direct reachability.
func TestIsReachable1(t *testing.T) {
	g, err := pgraph.NewGraph("reach1")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	g.AddVertex(a, b)
	g.AddEdge(a, b, &engine.Edge{Name: "a->b"})

	m := &edgeMatcher{
		graph:     g,
		adjacency: g.Adjacency(),
	}

	if !m.isReachable(a, b, 10, 100) {
		t.Errorf("expected a -> b to be reachable")
	}
}

// TestIsReachable2 tests transitive reachability.
func TestIsReachable2(t *testing.T) {
	g, err := pgraph.NewGraph("reach2")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")
	g.AddVertex(a, b, c)
	g.AddEdge(a, b, &engine.Edge{Name: "a->b"})
	g.AddEdge(b, c, &engine.Edge{Name: "b->c"})

	m := &edgeMatcher{
		graph:     g,
		adjacency: g.Adjacency(),
	}

	if !m.isReachable(a, c, 10, 100) {
		t.Errorf("expected a -> c to be transitively reachable")
	}
}

// TestIsReachable3 tests diamond reachability.
func TestIsReachable3(t *testing.T) {
	g, err := pgraph.NewGraph("reach3")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")
	d := newTestRes("autoedgetest", "d")
	g.AddVertex(a, b, c, d)
	g.AddEdge(a, b, &engine.Edge{Name: "a->b"})
	g.AddEdge(a, c, &engine.Edge{Name: "a->c"})
	g.AddEdge(b, d, &engine.Edge{Name: "b->d"})
	g.AddEdge(c, d, &engine.Edge{Name: "c->d"})

	m := &edgeMatcher{
		graph:     g,
		adjacency: g.Adjacency(),
	}

	if !m.isReachable(a, d, 10, 100) {
		t.Errorf("expected a -> d to be reachable via diamond")
	}
}

// TestIsReachable4 tests disconnected vertices are not reachable.
func TestIsReachable4(t *testing.T) {
	g, err := pgraph.NewGraph("reach4")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	g.AddVertex(a, b)

	m := &edgeMatcher{
		graph:     g,
		adjacency: g.Adjacency(),
	}

	if m.isReachable(a, b, 10, 100) {
		t.Errorf("expected a -> b to NOT be reachable (disconnected)")
	}
}

// TestIsReachable5 tests that reverse direction is not reachable.
func TestIsReachable5(t *testing.T) {
	g, err := pgraph.NewGraph("reach5")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	g.AddVertex(a, b)
	g.AddEdge(a, b, &engine.Edge{Name: "a->b"})

	m := &edgeMatcher{
		graph:     g,
		adjacency: g.Adjacency(),
	}

	if m.isReachable(b, a, 10, 100) {
		t.Errorf("expected b -> a to NOT be reachable (wrong direction)")
	}
}

// TestIsReachable6 tests that depth and visit limits are respected.
func TestIsReachable6(t *testing.T) {
	g, err := pgraph.NewGraph("reach6")
	if err != nil {
		t.Fatalf("err: %+v", err)
	}

	// Build a chain a -> b -> c -> d
	a := newTestRes("autoedgetest", "a")
	b := newTestRes("autoedgetest", "b")
	c := newTestRes("autoedgetest", "c")
	d := newTestRes("autoedgetest", "d")
	g.AddVertex(a, b, c, d)
	g.AddEdge(a, b, &engine.Edge{Name: "a->b"})
	g.AddEdge(b, c, &engine.Edge{Name: "b->c"})
	g.AddEdge(c, d, &engine.Edge{Name: "c->d"})

	m := &edgeMatcher{
		graph:     g,
		adjacency: g.Adjacency(),
	}

	// With maxDepth=1, a -> d should not be found (needs 3 hops)
	if m.isReachable(a, d, 1, 100) {
		t.Errorf("expected a -> d to NOT be reachable with maxDepth=1")
	}

	// With enough depth, it should be found
	if !m.isReachable(a, d, 10, 100) {
		t.Errorf("expected a -> d to be reachable with maxDepth=10")
	}
}

// BenchmarkAutoEdge10 benchmarks autoedge with 10 resources.
func BenchmarkAutoEdge10(b *testing.B) {
	benchmarkAutoEdge(b, 10)
}

// BenchmarkAutoEdge100 benchmarks autoedge with 100 resources.
func BenchmarkAutoEdge100(b *testing.B) {
	benchmarkAutoEdge(b, 100)
}

// BenchmarkAutoEdge1000 benchmarks autoedge with 1000 resources.
func BenchmarkAutoEdge1000(b *testing.B) {
	benchmarkAutoEdge(b, 1000)
}

// BenchmarkAutoEdge10000 benchmarks autoedge with 10000 resources.
func BenchmarkAutoEdge10000(b *testing.B) {
	benchmarkAutoEdge(b, 10000)
}

// benchmarkAutoEdge is a helper that builds a chain of n resources and runs
// AutoEdge.
func benchmarkAutoEdge(b *testing.B, n int) {
	b.Helper()

	silentLogf := func(format string, v ...interface{}) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g, err := pgraph.NewGraph("bench")
		if err != nil {
			b.Fatalf("err: %+v", err)
		}

		resources := make([]*testRes, n)
		for j := 0; j < n; j++ {
			name := fmt.Sprintf("r%d", j)
			r := newTestRes("autoedgetest", name)
			if j > 0 {
				prev := fmt.Sprintf("r%d", j-1)
				var reversed = true
				r.autoEdge = &testAutoEdge{
					edges: [][]engine.ResUID{
						{
							&testUID{
								BaseUID: engine.BaseUID{
									Name:     name,
									Kind:     "autoedgetest",
									Reversed: &reversed,
								},
								path: prev,
							},
						},
					},
				}
			}
			resources[j] = r
			g.AddVertex(r)
		}
		b.StartTimer()

		if err := AutoEdge(g, false, silentLogf); err != nil {
			b.Fatalf("err: %+v", err)
		}
	}
}

// BenchmarkAutoEdgeNoMatch benchmarks autoedge when no UIDs match.
func BenchmarkAutoEdgeNoMatch(b *testing.B) {
	silentLogf := func(format string, v ...interface{}) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g, err := pgraph.NewGraph("bench-nomatch")
		if err != nil {
			b.Fatalf("err: %+v", err)
		}

		const n = 100
		for j := 0; j < n; j++ {
			name := fmt.Sprintf("r%d", j)
			r := newTestRes("autoedgetest", name)
			var reversed = true
			r.autoEdge = &testAutoEdge{
				edges: [][]engine.ResUID{
					{
						&testUID{
							BaseUID: engine.BaseUID{
								Name:     name,
								Kind:     "autoedgetest",
								Reversed: &reversed,
							},
							path: "nonexistent",
						},
					},
				},
			}
			g.AddVertex(r)
		}
		b.StartTimer()

		if err := AutoEdge(g, false, silentLogf); err != nil {
			b.Fatalf("err: %+v", err)
		}
	}
}

// BenchmarkAutoEdgeAllMatch benchmarks autoedge when every resource matches the
// first resource.
func BenchmarkAutoEdgeAllMatch(b *testing.B) {
	silentLogf := func(format string, v ...interface{}) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g, err := pgraph.NewGraph("bench-allmatch")
		if err != nil {
			b.Fatalf("err: %+v", err)
		}

		const n = 100
		resources := make([]*testRes, n)
		for j := 0; j < n; j++ {
			name := fmt.Sprintf("r%d", j)
			r := newTestRes("autoedgetest", name)
			resources[j] = r
			g.AddVertex(r)
		}

		// r0 seeks edges to all others
		uids := []engine.ResUID{}
		var reversed = true
		for j := 1; j < n; j++ {
			uids = append(uids, &testUID{
				BaseUID: engine.BaseUID{
					Name:     "r0",
					Kind:     "autoedgetest",
					Reversed: &reversed,
				},
				path: fmt.Sprintf("r%d", j),
			})
		}
		resources[0].autoEdge = &testAutoEdge{
			edges: [][]engine.ResUID{uids},
		}
		b.StartTimer()

		if err := AutoEdge(g, false, silentLogf); err != nil {
			b.Fatalf("err: %+v", err)
		}
	}
}

// BenchmarkAutoEdgeMultiUID benchmarks autoedge with resources that have
// multiple UIDs each.
func BenchmarkAutoEdgeMultiUID(b *testing.B) {
	silentLogf := func(format string, v ...interface{}) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g, err := pgraph.NewGraph("bench-multiuid")
		if err != nil {
			b.Fatalf("err: %+v", err)
		}

		const n = 100
		const uidsPerRes = 5
		for j := 0; j < n; j++ {
			name := fmt.Sprintf("r%d", j)
			r := newTestRes("autoedgetest", name)

			// Each resource has multiple UIDs
			var rev = false
			uids := []engine.ResUID{}
			for k := 0; k < uidsPerRes; k++ {
				uids = append(uids, &testUID{
					BaseUID: engine.BaseUID{
						Name:     name,
						Kind:     "autoedgetest",
						Reversed: &rev,
					},
					path: fmt.Sprintf("%s-uid%d", name, k),
				})
			}
			r.uids = uids

			if j > 0 {
				prev := fmt.Sprintf("r%d-uid0", j-1)
				var reversed = true
				r.autoEdge = &testAutoEdge{
					edges: [][]engine.ResUID{
						{
							&testUID{
								BaseUID: engine.BaseUID{
									Name:     name,
									Kind:     "autoedgetest",
									Reversed: &reversed,
								},
								path: prev,
							},
						},
					},
				}
			}
			g.AddVertex(r)
		}
		b.StartTimer()

		if err := AutoEdge(g, false, silentLogf); err != nil {
			b.Fatalf("err: %+v", err)
		}
	}
}
