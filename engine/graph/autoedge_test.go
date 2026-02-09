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

package graph

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
)

// cacheTestUID is a UID type for cache tests that matches by key.
type cacheTestUID struct {
	engine.BaseUID

	key string
}

// IFF returns true if and only if the two UIDs match on key.
func (obj *cacheTestUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*cacheTestUID)
	if !ok {
		return false
	}
	return obj.key == res.key
}

// cacheTestAutoEdge is a configurable AutoEdge for cache tests.
type cacheTestAutoEdge struct {
	batches [][]engine.ResUID
	index   int
}

// Next returns the next batch of UIDs, or nil when exhausted.
func (obj *cacheTestAutoEdge) Next() []engine.ResUID {
	if obj.index >= len(obj.batches) {
		return nil
	}
	batch := obj.batches[obj.index]
	obj.index++
	return batch
}

// Test returns true if there are more batches to process.
func (obj *cacheTestAutoEdge) Test(input []bool) bool {
	return obj.index < len(obj.batches)
}

// cacheTestRes is a minimal EdgeableRes for cache tests.
type cacheTestRes struct {
	traits.Base
	traits.Edgeable

	testUIDs     []engine.ResUID
	testAutoEdge engine.AutoEdge
	testAutoErr  error
}

func (obj *cacheTestRes) Default() engine.Res                            { return &cacheTestRes{} }
func (obj *cacheTestRes) Validate() error                                { return nil }
func (obj *cacheTestRes) Init(*engine.Init) error                        { return nil }
func (obj *cacheTestRes) Cleanup() error                                 { return nil }
func (obj *cacheTestRes) Watch(context.Context) error                    { return nil }
func (obj *cacheTestRes) CheckApply(context.Context, bool) (bool, error) { return true, nil }
func (obj *cacheTestRes) Cmp(engine.Res) error                           { return nil }
func (obj *cacheTestRes) UIDs() []engine.ResUID                          { return obj.testUIDs }
func (obj *cacheTestRes) AutoEdges() (engine.AutoEdge, error) {
	return obj.testAutoEdge, obj.testAutoErr
}

// makeCacheTestRes creates a cacheTestRes with the given name and kind.
func makeCacheTestRes(name, kind string, uids []engine.ResUID, ae engine.AutoEdge) *cacheTestRes {
	r := &cacheTestRes{
		testUIDs:     uids,
		testAutoEdge: ae,
	}
	r.SetName(name)
	r.SetKind(kind)
	return r
}

// cacheTestNonEdgeable is a vertex that only implements pgraph.Vertex, not
// engine.EdgeableRes. Used to test fingerprinting mixed graphs.
type cacheTestNonEdgeable struct {
	name string
}

func (obj *cacheTestNonEdgeable) String() string { return obj.name }

// boolP returns a pointer to a bool value.
func boolP(b bool) *bool { return &b }

func TestComputeFingerprint(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	g.AddVertex(r1)

	fp1 := computeAutoEdgeFingerprint(g)
	fp2 := computeAutoEdgeFingerprint(g)
	if fp1 != fp2 {
		t.Errorf("fingerprint should be deterministic, got %q and %q", fp1, fp2)
	}
	if fp1 == "" {
		t.Errorf("fingerprint should not be empty")
	}
}

func TestFingerprintVertexOrder(t *testing.T) {
	// Build two graphs with the same vertices added in different order.
	// The fingerprint should be identical because vertices are sorted.
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "b", Kind: "test"},
		key:     "k2",
	}
	r1a := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	r2a := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1a)
	g1.AddVertex(r2a)

	// Second graph: same resources, reversed add order.
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r2b)
	g2.AddVertex(r1b)

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 != fp2 {
		t.Errorf("fingerprint should be order-independent, got %q and %q", fp1, fp2)
	}
}

func TestFingerprintDiffOnUIDChange(t *testing.T) {
	// Use UIDs with different Name values so that their String()
	// representations differ, which is what the fingerprint captures.
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "changed", Kind: "test"},
		key:     "k2",
	}

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	g1.AddVertex(r1)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	// Same resource identity, but different UIDs returned.
	r2 := makeCacheTestRes("a", "test", []engine.ResUID{uid2}, nil)
	g2.AddVertex(r2)

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 == fp2 {
		t.Errorf("fingerprint should differ when UIDs change")
	}
}

func TestFingerprintDiffOnEdgeChange(t *testing.T) {
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "b", Kind: "test"},
		key:     "k2",
	}

	// Graph without an explicit edge.
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1a := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	r2a := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g1.AddVertex(r1a, r2a)

	// Graph with an explicit edge.
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g2.AddVertex(r1b, r2b)
	g2.AddEdge(r1b, r2b, &engine.Edge{Name: "explicit"})

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 == fp2 {
		t.Errorf("fingerprint should differ when an explicit edge is added")
	}
}

func TestFingerprintDiffOnDisabledChange(t *testing.T) {
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}

	// Graph with autoedge enabled (default).
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	g1.AddVertex(r1)

	// Graph with autoedge disabled.
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r2 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	r2.SetAutoEdgeMeta(&engine.AutoEdgeMeta{Disabled: true})
	g2.AddVertex(r2)

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 == fp2 {
		t.Errorf("fingerprint should differ when Disabled changes")
	}
}

func TestReplayAutoEdges(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	r1 := makeCacheTestRes("a", "test", nil, nil)
	r2 := makeCacheTestRes("b", "test", nil, nil)
	g.AddVertex(r1, r2)

	edges := []autoEdgeCachedEdge{
		{from: "test[a]", to: "test[b]", name: "test[a] -> test[b]"},
	}

	if err := replayAutoEdges(g, edges); err != nil {
		t.Fatalf("replay should succeed: %v", err)
	}
	if i := g.NumEdges(); i != 1 {
		t.Errorf("expected 1 edge after replay, got: %d", i)
	}
	if e := g.FindEdge(r1, r2); e == nil {
		t.Errorf("edge from r1 to r2 should exist after replay")
	}
}

func TestReplayAutoEdgesMissingVertex(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	r1 := makeCacheTestRes("a", "test", nil, nil)
	g.AddVertex(r1)

	edges := []autoEdgeCachedEdge{
		{from: "test[a]", to: "test[missing]", name: "test edge"},
	}

	if err := replayAutoEdges(g, edges); err == nil {
		t.Errorf("replay should fail when a vertex is missing")
	}
}

// TestReplayAutoEdgesNoPartialModification verifies that when replay fails due
// to a missing vertex, no edges are added to the graph. This tests the
// pre-validation fix: without it, edges before the bad one would be added,
// corrupting the graph for the fallback path.
func TestReplayAutoEdgesNoPartialModification(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	r1 := makeCacheTestRes("a", "test", nil, nil)
	r2 := makeCacheTestRes("b", "test", nil, nil)
	g.AddVertex(r1, r2)

	// First edge is valid, second references a missing vertex.
	edges := []autoEdgeCachedEdge{
		{from: "test[a]", to: "test[b]", name: "good edge"},
		{from: "test[a]", to: "test[gone]", name: "bad edge"},
	}

	if err := replayAutoEdges(g, edges); err == nil {
		t.Fatalf("replay should fail on missing vertex")
	}
	// The pre-validation ensures the first (valid) edge was NOT
	// added before the failure was detected.
	if g.NumEdges() != 0 {
		t.Errorf("failed replay should not modify graph, got %d edge(s)",
			g.NumEdges())
	}
}

func TestSnapshotEdges(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}

	r1 := makeCacheTestRes("a", "test", nil, nil)
	r2 := makeCacheTestRes("b", "test", nil, nil)
	r3 := makeCacheTestRes("c", "test", nil, nil)
	g.AddVertex(r1, r2, r3)
	g.AddEdge(r1, r2, &engine.Edge{Name: "e1"})
	g.AddEdge(r2, r3, &engine.Edge{Name: "e2"})

	snap := snapshotEdges(g)
	if len(snap) != 2 {
		t.Errorf("expected 2 edges in snapshot, got: %d", len(snap))
	}

	if _, ok := snap[[2]string{"test[a]", "test[b]"}]; !ok {
		t.Errorf("snapshot should contain edge a -> b")
	}
	if _, ok := snap[[2]string{"test[b]", "test[c]"}]; !ok {
		t.Errorf("snapshot should contain edge b -> c")
	}
}

func TestAutoEdgeCacheHit(t *testing.T) {
	// Build a graph with two resources that will get an autoedge.
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)

	// First run: populates the cache.
	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("first AutoEdge run failed: %v", err)
	}
	if eng.autoEdgeCache == nil {
		t.Fatalf("cache should be populated after first run")
	}
	if g1.NumEdges() == 0 {
		t.Fatalf("first run should have added edges")
	}

	// Second run: identical graph, should hit cache.
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae2)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b)

	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("second AutoEdge run failed: %v", err)
	}
	if g2.NumEdges() == 0 {
		t.Errorf("cache replay should have added edges")
	}

	// Verify the replayed edge has the correct endpoints and name.
	e := g2.FindEdge(r1b, r2b)
	if e == nil {
		t.Fatalf("expected edge from r1b to r2b after replay")
	}
	if e.String() != "test[a] -> test[b]" {
		t.Errorf("replayed edge name %q != expected %q",
			e.String(), "test[a] -> test[b]")
	}

	// The autoedge object on the second graph should NOT have been
	// consumed, because we replayed from cache instead of running the
	// full algorithm.
	if ae2.index != 0 {
		t.Errorf("cache hit should not consume autoedge batches, index: %d", ae2.index)
	}
}

func TestAutoEdgeCacheMiss(t *testing.T) {
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("first AutoEdge run failed: %v", err)
	}

	savedFP := eng.autoEdgeCache.fingerprint

	// Second run: add a third vertex to change the fingerprint.
	uid3 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "c",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: "other",
	}
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae2)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	r3b := makeCacheTestRes("c", "test", []engine.ResUID{uid3}, nil)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b, r3b)

	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("second AutoEdge run failed: %v", err)
	}

	// The fingerprint should have changed.
	if eng.autoEdgeCache.fingerprint == savedFP {
		t.Errorf("fingerprint should differ after adding a vertex")
	}

	// The autoedge object should have been consumed by the full run.
	if ae2.index == 0 {
		t.Errorf("cache miss should consume autoedge batches")
	}
}

// TestAutoEdgeCacheHitZeroEdges verifies that a cache hit works when AutoEdge
// added zero edges on the first run (no matching resources).
func TestAutoEdgeCacheHitZeroEdges(t *testing.T) {
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: "no-match-1",
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: "no-match-2",
	}

	// UIDs don't match each other, so no autoedges will be added.
	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("first AutoEdge run failed: %v", err)
	}
	if len(eng.autoEdgeCache.edges) != 0 {
		t.Fatalf("expected 0 cached edges, got: %d",
			len(eng.autoEdgeCache.edges))
	}

	// Second run: identical graph, should hit cache.
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae2)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b)

	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("second AutoEdge run failed: %v", err)
	}
	if g2.NumEdges() != 0 {
		t.Errorf("expected 0 edges after cache replay, got: %d",
			g2.NumEdges())
	}
	if ae2.index != 0 {
		t.Errorf("cache hit should not consume batches, index: %d",
			ae2.index)
	}
}

// TestAutoEdgeCacheHitMultipleEdges verifies that when AutoEdge adds multiple
// edges, all of them are cached and replayed correctly.
func TestAutoEdgeCacheHitMultipleEdges(t *testing.T) {
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid3 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "c",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	// r1 seeks uid1, which matches both r2 and r3.
	// r2 seeks uid2, which matches r3 (r1 already covered).
	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid2}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, ae2)
	r3 := makeCacheTestRes("c", "test", []engine.ResUID{uid3}, nil)

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2, r3)

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("first AutoEdge run failed: %v", err)
	}

	firstRunEdges := g1.NumEdges()
	if firstRunEdges < 2 {
		t.Fatalf("expected at least 2 autoedges, got: %d",
			firstRunEdges)
	}
	cachedCount := len(eng.autoEdgeCache.edges)
	if cachedCount != firstRunEdges {
		t.Fatalf("cached %d edges but graph has %d",
			cachedCount, firstRunEdges)
	}

	// Second run: identical graph, replay from cache.
	ae1b := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	ae2b := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid2}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1b)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, ae2b)
	r3b := makeCacheTestRes("c", "test", []engine.ResUID{uid3}, nil)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b, r3b)

	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("second AutoEdge run failed: %v", err)
	}
	if g2.NumEdges() != firstRunEdges {
		t.Errorf("replay should produce %d edges, got: %d",
			firstRunEdges, g2.NumEdges())
	}
	if ae1b.index != 0 || ae2b.index != 0 {
		t.Errorf("cache hit should not consume batches")
	}
}

// TestAutoEdgeCacheReplayEdgeName verifies that the replayed edge has the same
// Name as the originally computed autoedge.
func TestAutoEdgeCacheReplayEdgeName(t *testing.T) {
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Capture the edge name from the first run.
	e1 := g1.FindEdge(r1, r2)
	if e1 == nil {
		t.Fatalf("expected edge from r1 to r2 after first run")
	}
	origName := e1.String()

	// Replay on an identical graph.
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae2)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)

	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b)

	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	e2 := g2.FindEdge(r1b, r2b)
	if e2 == nil {
		t.Fatalf("expected edge from r1b to r2b after replay")
	}
	if e2.String() != origName {
		t.Errorf("replayed edge name %q != original %q",
			e2.String(), origName)
	}
}

// TestAutoEdgeCacheMissThenHit verifies that after a cache miss updates the
// cache, the next identical graph transition hits the new cache.
func TestAutoEdgeCacheMissThenHit(t *testing.T) {
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}

	// Run 1: two vertices.
	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}

	// Run 2: three vertices (cache miss, updates cache).
	uid3 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "c",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: "other",
	}
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae2)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	r3b := makeCacheTestRes("c", "test", []engine.ResUID{uid3}, nil)
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b, r3b)
	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if ae2.index == 0 {
		t.Fatalf("run 2 should be a cache miss")
	}
	savedFP := eng.autoEdgeCache.fingerprint

	// Run 3: same three-vertex graph (should hit the updated cache).
	ae3 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1c := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae3)
	r2c := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	r3c := makeCacheTestRes("c", "test", []engine.ResUID{uid3}, nil)
	g3, err := pgraph.NewGraph("g3")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g3.AddVertex(r1c, r2c, r3c)
	eng.nextGraph = g3
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("run 3 failed: %v", err)
	}
	if ae3.index != 0 {
		t.Errorf("run 3 should be a cache hit, but batches consumed")
	}
	if eng.autoEdgeCache.fingerprint != savedFP {
		t.Errorf("fingerprint should not change on cache hit")
	}
	if g3.NumEdges() != g2.NumEdges() {
		t.Errorf("run 3 edges %d != run 2 edges %d",
			g3.NumEdges(), g2.NumEdges())
	}
}

// TestAutoEdgeCacheReplayFallback verifies that when a replay fails (stale
// vertex lookup), we fall through to a full computation that still produces the
// correct edges.
func TestAutoEdgeCacheReplayFallback(t *testing.T) {
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Corrupt the cache: inject an edge referencing a vertex that
	// won't exist. Keep the fingerprint the same so the cache path
	// is taken, but replay will fail on the bad vertex.
	eng.autoEdgeCache.edges = append(eng.autoEdgeCache.edges,
		autoEdgeCachedEdge{
			from: "test[a]",
			to:   "test[gone]",
			name: "bad edge",
		},
	)

	// Build identical graph. The fingerprint matches, replay will
	// fail on "test[gone]", and we should fall through to a full
	// recomputation that succeeds.
	ae2 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae2)
	r2b := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g2.AddVertex(r1b, r2b)

	eng.nextGraph = g2
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("fallback run should not error: %v", err)
	}
	if ae2.index == 0 {
		t.Errorf("full computation should have consumed batches")
	}
	if g2.NumEdges() == 0 {
		t.Errorf("fallback should still produce autoedges")
	}
}

// TestFingerprintDiffOnVertexRemoval verifies that removing a vertex between
// transitions produces a different fingerprint.
func TestFingerprintDiffOnVertexRemoval(t *testing.T) {
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "b", Kind: "test"},
		key:     "k2",
	}

	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1a := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	r2a := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g1.AddVertex(r1a, r2a)

	// Second graph has only one vertex (b removed).
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	g2.AddVertex(r1b)

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 == fp2 {
		t.Errorf("fingerprint should differ when a vertex is removed")
	}
}

// TestFingerprintWithNonEdgeableVertex verifies that a graph containing a
// non-EdgeableRes vertex still produces a stable fingerprint that differs from
// one without it.
func TestFingerprintWithNonEdgeableVertex(t *testing.T) {
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "a", Kind: "test"},
		key:     "k1",
	}

	// Graph with only an EdgeableRes.
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	g1.AddVertex(r1)

	// Graph with the same EdgeableRes plus a non-edgeable vertex.
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1b := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, nil)
	plain := &cacheTestNonEdgeable{name: "plain"}
	g2.AddVertex(r1b, plain)

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 == fp2 {
		t.Errorf("adding a non-edgeable vertex should change fp")
	}

	// Fingerprint of g2 should be stable.
	fp2b := computeAutoEdgeFingerprint(g2)
	if fp2 != fp2b {
		t.Errorf("fingerprint should be deterministic with mixed vertices")
	}
}

// TestFingerprintMultipleUIDsOrder verifies that the fingerprint is the same
// regardless of the order UIDs() returns its elements, since the fingerprint
// sorts UID strings internally.
func TestFingerprintMultipleUIDsOrder(t *testing.T) {
	uidA := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "x", Kind: "test"},
		key:     "kx",
	}
	uidB := &cacheTestUID{
		BaseUID: engine.BaseUID{Name: "y", Kind: "test"},
		key:     "ky",
	}

	// Graph where UIDs are returned in order [uidA, uidB].
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1 := makeCacheTestRes("r", "test",
		[]engine.ResUID{uidA, uidB}, nil)
	g1.AddVertex(r1)

	// Graph where UIDs are returned in order [uidB, uidA].
	g2, err := pgraph.NewGraph("g2")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r2 := makeCacheTestRes("r", "test",
		[]engine.ResUID{uidB, uidA}, nil)
	g2.AddVertex(r2)

	fp1 := computeAutoEdgeFingerprint(g1)
	fp2 := computeAutoEdgeFingerprint(g2)
	if fp1 != fp2 {
		t.Errorf("fingerprint should be UID-order-independent")
	}
}

// TestAutoEdgeCacheSuccessiveHits verifies that the cache works across three
// consecutive identical transitions, not just one.
func TestAutoEdgeCacheSuccessiveHits(t *testing.T) {
	sharedKey := "shared"
	uid1 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "a",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}
	uid2 := &cacheTestUID{
		BaseUID: engine.BaseUID{
			Name:     "b",
			Kind:     "test",
			Reversed: boolP(false),
		},
		key: sharedKey,
	}

	eng := &Engine{
		Debug: testing.Verbose(),
		Logf:  t.Logf,
	}

	// Run 1: cold cache.
	ae1 := &cacheTestAutoEdge{
		batches: [][]engine.ResUID{{uid1}},
	}
	r1 := makeCacheTestRes("a", "test", []engine.ResUID{uid1}, ae1)
	r2 := makeCacheTestRes("b", "test", []engine.ResUID{uid2}, nil)
	g1, err := pgraph.NewGraph("g1")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	g1.AddVertex(r1, r2)
	eng.nextGraph = g1
	if err := eng.AutoEdge(); err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}
	edgeCount := g1.NumEdges()

	// Runs 2-4: all should hit cache.
	for i := 2; i <= 4; i++ {
		ae := &cacheTestAutoEdge{
			batches: [][]engine.ResUID{{uid1}},
		}
		ra := makeCacheTestRes("a", "test",
			[]engine.ResUID{uid1}, ae)
		rb := makeCacheTestRes("b", "test",
			[]engine.ResUID{uid2}, nil)
		g, err := pgraph.NewGraph("g")
		if err != nil {
			t.Fatalf("run %d: error creating graph: %v", i, err)
		}
		g.AddVertex(ra, rb)
		eng.nextGraph = g
		if err := eng.AutoEdge(); err != nil {
			t.Fatalf("run %d failed: %v", i, err)
		}
		if ae.index != 0 {
			t.Errorf("run %d: should be cache hit", i)
		}
		if g.NumEdges() != edgeCount {
			t.Errorf("run %d: expected %d edges, got %d",
				i, edgeCount, g.NumEdges())
		}
	}
}

// TestSnapshotEdgesEmpty verifies snapshotEdges on a graph with vertices but no
// edges.
func TestSnapshotEdgesEmpty(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1 := makeCacheTestRes("a", "test", nil, nil)
	g.AddVertex(r1)

	snap := snapshotEdges(g)
	if len(snap) != 0 {
		t.Errorf("expected 0 edges in snapshot, got: %d", len(snap))
	}
}

// TestReplayAutoEdgesEmpty verifies that replaying an empty cached edge list
// succeeds and adds nothing.
func TestReplayAutoEdgesEmpty(t *testing.T) {
	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("error creating graph: %v", err)
	}
	r1 := makeCacheTestRes("a", "test", nil, nil)
	g.AddVertex(r1)

	if err := replayAutoEdges(g, nil); err != nil {
		t.Errorf("replaying nil edges should succeed: %v", err)
	}
	if err := replayAutoEdges(g, []autoEdgeCachedEdge{}); err != nil {
		t.Errorf("replaying empty edges should succeed: %v", err)
	}
	if g.NumEdges() != 0 {
		t.Errorf("expected 0 edges, got: %d", g.NumEdges())
	}
}
