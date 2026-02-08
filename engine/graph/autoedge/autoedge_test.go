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

// TestUID is a custom UID type for test resources. It matches other TestUID
// values by key using a type assertion, mirroring how real resource UIDs work.
type TestUID struct {
	engine.BaseUID

	key string
}

// IFF returns true if and only if the two UIDs are equivalent.
func (obj *TestUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*TestUID)
	if !ok {
		return false
	}
	return obj.key == res.key
}

// TestUID2 is a second UID type for cross-kind isolation testing. It never
// matches TestUID because the type assertion in IFF fails across types.
type TestUID2 struct {
	engine.BaseUID

	key string
}

// IFF returns true if and only if the two UIDs are equivalent.
func (obj *TestUID2) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*TestUID2)
	if !ok {
		return false
	}
	return obj.key == res.key
}

// testAutoEdgeObj is a configurable AutoEdge implementation for tests. It
// returns pre-configured batches of UIDs and records what Test() received.
type testAutoEdgeObj struct {
	batches  [][]engine.ResUID
	index    int
	testArgs [][]bool
}

// Next returns the next batch of UIDs, or nil when exhausted.
func (obj *testAutoEdgeObj) Next() []engine.ResUID {
	if obj.index >= len(obj.batches) {
		return nil
	}
	batch := obj.batches[obj.index]
	obj.index++
	return batch
}

// Test records the result and returns true if there are more batches.
func (obj *testAutoEdgeObj) Test(input []bool) bool {
	obj.testArgs = append(obj.testArgs, input)
	return obj.index < len(obj.batches)
}

// TestEdgeRes is a minimal resource type that implements engine.EdgeableRes for
// use in autoedge tests. It embeds traits.Base and traits.Edgeable so we get
// Kind, Name, String, MetaParams, and AutoEdgeMeta for free.
type TestEdgeRes struct {
	traits.Base
	traits.Edgeable

	testUIDs     []engine.ResUID
	testAutoEdge engine.AutoEdge
	testAutoErr  error
}

func (obj *TestEdgeRes) Default() engine.Res                            { return &TestEdgeRes{} }
func (obj *TestEdgeRes) Validate() error                                { return nil }
func (obj *TestEdgeRes) Init(*engine.Init) error                        { return nil }
func (obj *TestEdgeRes) Cleanup() error                                 { return nil }
func (obj *TestEdgeRes) Watch(context.Context) error                    { return nil }
func (obj *TestEdgeRes) CheckApply(context.Context, bool) (bool, error) { return true, nil }
func (obj *TestEdgeRes) Cmp(engine.Res) error                           { return nil }
func (obj *TestEdgeRes) UIDs() []engine.ResUID                          { return obj.testUIDs }
func (obj *TestEdgeRes) AutoEdges() (engine.AutoEdge, error) {
	return obj.testAutoEdge, obj.testAutoErr
}

// TestEdgeRes2 is a second resource kind to verify cross-kind isolation.
type TestEdgeRes2 struct {
	traits.Base
	traits.Edgeable

	testUIDs     []engine.ResUID
	testAutoEdge engine.AutoEdge
	testAutoErr  error
}

func (obj *TestEdgeRes2) Default() engine.Res                            { return &TestEdgeRes2{} }
func (obj *TestEdgeRes2) Validate() error                                { return nil }
func (obj *TestEdgeRes2) Init(*engine.Init) error                        { return nil }
func (obj *TestEdgeRes2) Cleanup() error                                 { return nil }
func (obj *TestEdgeRes2) Watch(context.Context) error                    { return nil }
func (obj *TestEdgeRes2) CheckApply(context.Context, bool) (bool, error) { return true, nil }
func (obj *TestEdgeRes2) Cmp(engine.Res) error                           { return nil }
func (obj *TestEdgeRes2) UIDs() []engine.ResUID                          { return obj.testUIDs }
func (obj *TestEdgeRes2) AutoEdges() (engine.AutoEdge, error) {
	return obj.testAutoEdge, obj.testAutoErr
}

// TestNonEdgeableRes is a vertex that only implements pgraph.Vertex, not
// engine.EdgeableRes. It should be silently ignored by the autoedge algorithm.
type TestNonEdgeableRes struct {
	name string
}

func (obj *TestNonEdgeableRes) String() string { return obj.name }

// makeTestRes creates a TestEdgeRes with the given name, kind, UIDs, and
// autoedge configuration.
func makeTestRes(name, kind string, uids []engine.ResUID, ae engine.AutoEdge) *TestEdgeRes {
	r := &TestEdgeRes{
		testUIDs:     uids,
		testAutoEdge: ae,
	}
	r.SetName(name)
	r.SetKind(kind)
	return r
}

// makeTestRes2 creates a TestEdgeRes2 with the given name, kind, UIDs, and
// autoedge configuration.
func makeTestRes2(name, kind string, uids []engine.ResUID, ae engine.AutoEdge) *TestEdgeRes2 {
	r := &TestEdgeRes2{
		testUIDs:     uids,
		testAutoEdge: ae,
	}
	r.SetName(name)
	r.SetKind(kind)
	return r
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool { return &b }

// testLogf returns a logf function that writes to the test log.
func testLogf(t *testing.T) func(string, ...interface{}) {
	t.Helper()
	return func(format string, v ...interface{}) {
		t.Helper()
		t.Logf("autoedge: "+format, v...)
	}
}

// --- Functional tests ---

func TestAutoEdgeEmptyGraph(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeEmptyGraph")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("autoEdge on empty graph should not error: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("empty graph should have 0 edges, got: %d", i)
	}
}

func TestAutoEdgeSingleVertex(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeSingleVertex")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid}}}
	r := makeTestRes("r1", "test", []engine.ResUID{uid}, ae)
	g.AddVertex(r)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("single vertex should have 0 edges, got: %d", i)
	}
}

func TestAutoEdgeTwoMatching(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeTwoMatching")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	sharedKey := "shared"
	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     sharedKey,
	}
	uid2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     sharedKey,
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid1}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid1}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{uid2}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge, got: %d", i)
	}
}

func TestAutoEdgeTwoNonMatching(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeTwoNonMatching")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "key1",
	}
	uid2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "key2",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid1}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid1}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{uid2}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("non-matching should have 0 edges, got: %d", i)
	}
}

func TestAutoEdgeReversedDirection(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeReversedDirection")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	seekUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "seeker", Kind: "test", Reversed: boolPtr(true)},
		key:     "shared",
	}
	matchUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "matched", Kind: "test", Reversed: boolPtr(true)},
		key:     "shared",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
	seeker := makeTestRes("seeker", "test", []engine.ResUID{seekUID}, ae)
	matched := makeTestRes("matched", "test", []engine.ResUID{matchUID}, nil)
	g.AddVertex(seeker, matched)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge, got: %d", i)
	}
	// reversed: edge from matched -> seeker
	adj := g.Adjacency()
	if _, ok := adj[matched]; !ok {
		t.Errorf("expected edge from matched -> seeker")
	}
}

func TestAutoEdgeNormalDirection(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeNormalDirection")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	seekUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "seeker", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}
	matchUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "matched", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
	seeker := makeTestRes("seeker", "test", []engine.ResUID{seekUID}, ae)
	matched := makeTestRes("matched", "test", []engine.ResUID{matchUID}, nil)
	g.AddVertex(seeker, matched)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge, got: %d", i)
	}
	// normal: edge from seeker -> matched
	adj := g.Adjacency()
	if _, ok := adj[seeker]; !ok {
		t.Errorf("expected edge from seeker -> matched")
	}
}

func TestAutoEdgeMultipleUIDs(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeMultipleUIDs")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	uid2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k2",
	}
	uid3 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	uid4 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r3", Kind: "test", Reversed: boolPtr(false)},
		key:     "k2",
	}

	// r1 seeks both k1 and k2 in one batch
	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid1, uid2}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid1, uid2}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{uid3}, nil)
	r3 := makeTestRes("r3", "test", []engine.ResUID{uid4}, nil)
	g.AddVertex(r1, r2, r3)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 2 {
		t.Errorf("should have 2 edges, got: %d", i)
	}
}

func TestAutoEdgeMultipleBatches(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeMultipleBatches")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	uid2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k2",
	}
	matchUID1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	matchUID2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r3", Kind: "test", Reversed: boolPtr(false)},
		key:     "k2",
	}

	// two separate batches
	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid1}, {uid2}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid1, uid2}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{matchUID1}, nil)
	r3 := makeTestRes("r3", "test", []engine.ResUID{matchUID2}, nil)
	g.AddVertex(r1, r2, r3)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 2 {
		t.Errorf("should have 2 edges from 2 batches, got: %d", i)
	}
}

func TestAutoEdgeNilAutoEdge(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeNilAutoEdge")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	// nil autoedge means no edges to seek
	r := makeTestRes("r1", "test", []engine.ResUID{uid}, nil)
	g.AddVertex(r)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("nil autoedge should produce 0 edges, got: %d", i)
	}
}

func TestAutoEdgeDisabled(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeDisabled")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}
	matchUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid}, ae)
	r1.SetAutoEdgeMeta(&engine.AutoEdgeMeta{Disabled: true}) // disabled!
	r2 := makeTestRes("r2", "test", []engine.ResUID{matchUID}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("disabled resource should produce 0 edges, got: %d", i)
	}
}

func TestAutoEdgeSelfMatch(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeSelfMatch")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "self",
	}

	// resource seeks its own UID — should not create a self-edge
	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid}}}
	r := makeTestRes("r1", "test", []engine.ResUID{uid}, ae)
	g.AddVertex(r)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("self-match should produce 0 edges, got: %d", i)
	}
}

func TestAutoEdgeMixedKinds(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeMixedKinds")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	// TestUID and TestUID2 have the same key but different types, so IFF
	// will return false due to the type assertion.
	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test1", Reversed: boolPtr(false)},
		key:     "shared",
	}
	uid2 := &TestUID2{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test2", Reversed: boolPtr(false)},
		key:     "shared",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid1}}}
	r1 := makeTestRes("r1", "test1", []engine.ResUID{uid1}, ae)
	r2 := makeTestRes2("r2", "test2", []engine.ResUID{uid2}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("cross-kind UIDs should not match, got: %d edges", i)
	}
}

func TestAutoEdgeLargeGraph(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeLargeGraph")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	const n = 50
	resources := make([]*TestEdgeRes, n)

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("r%d", i)
		uid := &TestUID{
			BaseUID: engine.BaseUID{Name: name, Kind: "test", Reversed: boolPtr(false)},
			key:     fmt.Sprintf("k%d", i),
		}
		var ae engine.AutoEdge
		if i > 0 {
			// each resource (except first) seeks the previous one
			seekUID := &TestUID{
				BaseUID: engine.BaseUID{Name: name, Kind: "test", Reversed: boolPtr(false)},
				key:     fmt.Sprintf("k%d", i-1),
			}
			ae = &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
		}
		resources[i] = makeTestRes(name, "test", []engine.ResUID{uid}, ae)
		g.AddVertex(resources[i])
	}

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != n-1 {
		t.Errorf("should have %d edges in chain, got: %d", n-1, i)
	}
}

func TestAutoEdgeHierarchy(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeHierarchy")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	// Simulate a file hierarchy: /a/b/c depends on /a/b depends on /a
	uidA := &TestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test", Reversed: boolPtr(true)},
		key:     "/a",
	}
	uidB := &TestUID{
		BaseUID: engine.BaseUID{Name: "b", Kind: "test", Reversed: boolPtr(true)},
		key:     "/a/b",
	}
	uidC := &TestUID{
		BaseUID: engine.BaseUID{Name: "c", Kind: "test", Reversed: boolPtr(true)},
		key:     "/a/b/c",
	}

	// c seeks /a/b then /a (parent hierarchy)
	seekB := &TestUID{
		BaseUID: engine.BaseUID{Name: "c", Kind: "test", Reversed: boolPtr(true)},
		key:     "/a/b",
	}
	seekA := &TestUID{
		BaseUID: engine.BaseUID{Name: "c", Kind: "test", Reversed: boolPtr(true)},
		key:     "/a",
	}
	aeC := &testAutoEdgeObj{batches: [][]engine.ResUID{{seekB}, {seekA}}}

	// b seeks /a
	seekA2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "b", Kind: "test", Reversed: boolPtr(true)},
		key:     "/a",
	}
	aeB := &testAutoEdgeObj{batches: [][]engine.ResUID{{seekA2}}}

	rA := makeTestRes("a", "test", []engine.ResUID{uidA}, nil)
	rB := makeTestRes("b", "test", []engine.ResUID{uidB}, aeB)
	rC := makeTestRes("c", "test", []engine.ResUID{uidC}, aeC)
	g.AddVertex(rA, rB, rC)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	// b->a and c->b (c stops after finding b since Test returns false when
	// batches are exhausted after 2 batches with 1 match each)
	if i := g.NumEdges(); i < 2 {
		t.Errorf("should have at least 2 edges in hierarchy, got: %d", i)
	}
}

func TestAutoEdgeFirstMatchWins(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeFirstMatchWins")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	seekUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "seeker", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}
	matchUID1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "m1", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}
	matchUID2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "m2", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
	seeker := makeTestRes("seeker", "test", []engine.ResUID{seekUID}, ae)
	m1 := makeTestRes("m1", "test", []engine.ResUID{matchUID1}, nil)
	m2 := makeTestRes("m2", "test", []engine.ResUID{matchUID2}, nil)
	g.AddVertex(seeker, m1, m2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	// only one edge even though two vertices match
	if i := g.NumEdges(); i != 1 {
		t.Errorf("first match wins: should have 1 edge, got: %d", i)
	}
}

// testStoppingAutoEdgeObj stops iteration when Test receives a true match.
type testStoppingAutoEdgeObj struct {
	batches  [][]engine.ResUID
	index    int
	testArgs [][]bool
}

func (obj *testStoppingAutoEdgeObj) Next() []engine.ResUID {
	if obj.index >= len(obj.batches) {
		return nil
	}
	batch := obj.batches[obj.index]
	obj.index++
	return batch
}

func (obj *testStoppingAutoEdgeObj) Test(input []bool) bool {
	obj.testArgs = append(obj.testArgs, input)
	// Stop as soon as any match is found (like FileRes does)
	for _, b := range input {
		if b {
			return false
		}
	}
	return obj.index < len(obj.batches)
}

func TestAutoEdgeTestStopsIteration(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeTestStopsIteration")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	uid2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k2",
	}
	matchUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}

	// Two batches, but the stopping autoedge should stop after first match
	ae := &testStoppingAutoEdgeObj{batches: [][]engine.ResUID{{uid1}, {uid2}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid1, uid2}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{matchUID}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	// Only 1 edge because Test() stopped iteration after first match
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge (stopped early), got: %d", i)
	}
	if len(ae.testArgs) != 1 {
		t.Errorf("test should have been called once, got: %d", len(ae.testArgs))
	}
}

func TestAutoEdgeTestReceivesBooleans(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeTestReceivesBooleans")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid1 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "match",
	}
	uid2 := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "nomatch",
	}
	matchUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "match",
	}

	// Batch with two UIDs: one matches, one doesn't
	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid1, uid2}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid1, uid2}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{matchUID}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if len(ae.testArgs) != 1 {
		t.Fatalf("expected 1 Test call, got: %d", len(ae.testArgs))
	}
	result := ae.testArgs[0]
	if len(result) != 2 {
		t.Fatalf("expected 2 booleans, got: %d", len(result))
	}
	if !result[0] {
		t.Errorf("first UID should match (true), got false")
	}
	if result[1] {
		t.Errorf("second UID should not match (false), got true")
	}
}

func TestAutoEdgeNonEdgeableVertex(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeNonEdgeableVertex")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid}, ae)
	nonEdge := &TestNonEdgeableRes{name: "nope"}
	g.AddVertex(r1, nonEdge)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("non-edgeable vertex should not match, got: %d edges", i)
	}
}

func TestAutoEdgeError(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeError")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	r := &TestEdgeRes{
		testAutoErr: fmt.Errorf("broken autoedge"),
	}
	r.SetName("broken")
	r.SetKind("test")
	g.AddVertex(r)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err == nil {
		t.Errorf("expected error from broken AutoEdges(), got nil")
	}
}

func TestAutoEdgeDeterministic(t *testing.T) {
	const runs = 10

	buildGraph := func() (*pgraph.Graph, error) {
		g, err := pgraph.NewGraph("det")
		if err != nil {
			return nil, err
		}

		for i := 0; i < 10; i++ {
			name := fmt.Sprintf("r%d", i)
			uid := &TestUID{
				BaseUID: engine.BaseUID{Name: name, Kind: "test", Reversed: boolPtr(false)},
				key:     fmt.Sprintf("k%d", i),
			}
			var ae engine.AutoEdge
			if i > 0 {
				seekUID := &TestUID{
					BaseUID: engine.BaseUID{Name: name, Kind: "test", Reversed: boolPtr(false)},
					key:     fmt.Sprintf("k%d", i-1),
				}
				ae = &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
			}
			r := makeTestRes(name, "test", []engine.ResUID{uid}, ae)
			g.AddVertex(r)
		}
		return g, nil
	}

	// Get reference result
	refGraph, err := buildGraph()
	if err != nil {
		t.Fatalf("error creating reference graph: %v", err)
	}
	if err := AutoEdge(refGraph, false, testLogf(t)); err != nil {
		t.Fatalf("error running reference AutoEdge: %v", err)
	}
	refEdges := refGraph.NumEdges()

	for i := 1; i < runs; i++ {
		g, err := buildGraph()
		if err != nil {
			t.Fatalf("run %d: error creating graph: %v", i, err)
		}
		if err := AutoEdge(g, false, testLogf(t)); err != nil {
			t.Fatalf("run %d: error running AutoEdge: %v", i, err)
		}
		if got := g.NumEdges(); got != refEdges {
			t.Errorf("run %d: got %d edges, want %d", i, got, refEdges)
		}
	}
}

// --- UIDExistsInUIDs tests ---

func TestUIDExistsInUIDsEmpty(t *testing.T) {
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	if UIDExistsInUIDs(uid, nil) {
		t.Errorf("empty list should return false")
	}
	if UIDExistsInUIDs(uid, []engine.ResUID{}) {
		t.Errorf("empty slice should return false")
	}
}

func TestUIDExistsInUIDsSingleMatch(t *testing.T) {
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	other := &TestUID{
		BaseUID: engine.BaseUID{Name: "y", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	if !UIDExistsInUIDs(uid, []engine.ResUID{other}) {
		t.Errorf("matching UID should return true")
	}
}

func TestUIDExistsInUIDsSingleNoMatch(t *testing.T) {
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	other := &TestUID{
		BaseUID: engine.BaseUID{Name: "y", Kind: "test", Reversed: boolPtr(false)},
		key:     "k2",
	}
	if UIDExistsInUIDs(uid, []engine.ResUID{other}) {
		t.Errorf("non-matching UID should return false")
	}
}

func TestUIDExistsInUIDsMatchFirst(t *testing.T) {
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
		key:     "k1",
	}
	list := []engine.ResUID{
		&TestUID{BaseUID: engine.BaseUID{Name: "a", Kind: "test", Reversed: boolPtr(false)}, key: "k1"},
		&TestUID{BaseUID: engine.BaseUID{Name: "b", Kind: "test", Reversed: boolPtr(false)}, key: "k2"},
		&TestUID{BaseUID: engine.BaseUID{Name: "c", Kind: "test", Reversed: boolPtr(false)}, key: "k3"},
	}
	if !UIDExistsInUIDs(uid, list) {
		t.Errorf("match on first should return true")
	}
}

func TestUIDExistsInUIDsMatchLast(t *testing.T) {
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
		key:     "k3",
	}
	list := []engine.ResUID{
		&TestUID{BaseUID: engine.BaseUID{Name: "a", Kind: "test", Reversed: boolPtr(false)}, key: "k1"},
		&TestUID{BaseUID: engine.BaseUID{Name: "b", Kind: "test", Reversed: boolPtr(false)}, key: "k2"},
		&TestUID{BaseUID: engine.BaseUID{Name: "c", Kind: "test", Reversed: boolPtr(false)}, key: "k3"},
	}
	if !UIDExistsInUIDs(uid, list) {
		t.Errorf("match on last should return true")
	}
}

func TestUIDExistsInUIDsNoMatch(t *testing.T) {
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
		key:     "k4",
	}
	list := []engine.ResUID{
		&TestUID{BaseUID: engine.BaseUID{Name: "a", Kind: "test", Reversed: boolPtr(false)}, key: "k1"},
		&TestUID{BaseUID: engine.BaseUID{Name: "b", Kind: "test", Reversed: boolPtr(false)}, key: "k2"},
		&TestUID{BaseUID: engine.BaseUID{Name: "c", Kind: "test", Reversed: boolPtr(false)}, key: "k3"},
	}
	if UIDExistsInUIDs(uid, list) {
		t.Errorf("no match should return false")
	}
}

func TestAutoEdgeDisabledTarget(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeDisabledTarget")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	// r1 seeks a UID that r2 holds, but r2 is disabled so it should not
	// be matched as a target.
	uid := &TestUID{
		BaseUID: engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}
	matchUID := &TestUID{
		BaseUID: engine.BaseUID{Name: "r2", Kind: "test", Reversed: boolPtr(false)},
		key:     "shared",
	}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{uid}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{uid}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{matchUID}, nil)
	r2.SetAutoEdgeMeta(&engine.AutoEdgeMeta{Disabled: true}) // target is disabled
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 0 {
		t.Errorf("disabled target should not be matched, got: %d edges", i)
	}
}

func TestAutoEdgeBaseUIDFallback(t *testing.T) {
	g, err := pgraph.NewGraph("TestAutoEdgeBaseUIDFallback")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	// Use a bare BaseUID (not a custom type) as the seeking UID. This
	// exercises the uidTypeIndex fallback path because the concrete type
	// of the seek UID (*engine.BaseUID) won't match the *TestUID entries
	// in the index, so the code must fall back to scanning all vertices.
	seekUID := &engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)}

	// r2 holds a TestUID that matches via BaseUID.IFF (name+kind match).
	matchUID := &engine.BaseUID{Name: "r1", Kind: "test", Reversed: boolPtr(false)}

	ae := &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
	r1 := makeTestRes("r1", "test", []engine.ResUID{seekUID}, ae)
	r2 := makeTestRes("r2", "test", []engine.ResUID{matchUID}, nil)
	g.AddVertex(r1, r2)

	if err := AutoEdge(g, testing.Verbose(), testLogf(t)); err != nil {
		t.Errorf("error running AutoEdge: %v", err)
	}
	if i := g.NumEdges(); i != 1 {
		t.Errorf("baseUID fallback should produce 1 edge, got: %d", i)
	}
}

// --- Benchmarks ---

func benchAutoEdge(b *testing.B, n int, matchPct float64, uidsPerRes int) {
	b.Helper()

	// Build graph once outside the timer
	g, err := pgraph.NewGraph("bench")
	if err != nil {
		b.Fatalf("error creating graph: %v", err)
	}

	matchCount := int(float64(n) * matchPct)

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("r%d", i)
		uids := make([]engine.ResUID, 0, uidsPerRes)
		for j := 0; j < uidsPerRes; j++ {
			uids = append(uids, &TestUID{
				BaseUID: engine.BaseUID{Name: name, Kind: "test", Reversed: boolPtr(false)},
				key:     fmt.Sprintf("k%d-%d", i, j),
			})
		}

		r := makeTestRes(name, "test", uids, nil)
		g.AddVertex(r)
	}

	// Store resources for autoedge configuration during each iteration
	vertices := g.VerticesSorted()

	b.ReportAllocs()
	b.ResetTimer()

	for iter := 0; iter < b.N; iter++ {
		// Build a fresh graph with autoedges each iteration
		bg, _ := pgraph.NewGraph("bench-iter")
		for _, v := range vertices {
			bg.AddVertex(v)
		}

		// Configure autoedges: each resource (after matchCount) seeks the
		// first matchCount resources
		for i, v := range vertices {
			res := v.(*TestEdgeRes)
			if i < matchCount && i > 0 {
				seekUID := &TestUID{
					BaseUID: engine.BaseUID{Name: res.Name(), Kind: "test", Reversed: boolPtr(false)},
					key:     fmt.Sprintf("k%d-0", i-1),
				}
				res.testAutoEdge = &testAutoEdgeObj{batches: [][]engine.ResUID{{seekUID}}}
			} else {
				res.testAutoEdge = nil
			}
		}

		AutoEdge(bg, false, func(string, ...interface{}) {}) //nolint:errcheck
	}
}

func BenchmarkAutoEdge10(b *testing.B)    { benchAutoEdge(b, 10, 0.5, 1) }
func BenchmarkAutoEdge100(b *testing.B)   { benchAutoEdge(b, 100, 0.5, 1) }
func BenchmarkAutoEdge1000(b *testing.B)  { benchAutoEdge(b, 1000, 0.5, 1) }
func BenchmarkAutoEdge10000(b *testing.B) { benchAutoEdge(b, 10000, 0.5, 1) }

func BenchmarkAutoEdgeNoMatch(b *testing.B)  { benchAutoEdge(b, 1000, 0.0, 1) }
func BenchmarkAutoEdgeAllMatch(b *testing.B) { benchAutoEdge(b, 1000, 1.0, 1) }

func BenchmarkAutoEdgeMultiUID(b *testing.B) { benchAutoEdge(b, 100, 0.5, 10) }

func BenchmarkUIDExistsInUIDs(b *testing.B) {
	sizes := []int{1, 5, 10, 50}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size%d", size), func(b *testing.B) {
			uid := &TestUID{
				BaseUID: engine.BaseUID{Name: "x", Kind: "test", Reversed: boolPtr(false)},
				key:     "target",
			}
			list := make([]engine.ResUID, size)
			for i := 0; i < size-1; i++ {
				list[i] = &TestUID{
					BaseUID: engine.BaseUID{Name: fmt.Sprintf("n%d", i), Kind: "test", Reversed: boolPtr(false)},
					key:     fmt.Sprintf("k%d", i),
				}
			}
			// Last element matches
			list[size-1] = &TestUID{
				BaseUID: engine.BaseUID{Name: "match", Kind: "test", Reversed: boolPtr(false)},
				key:     "target",
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				UIDExistsInUIDs(uid, list)
			}
		})
	}
}
