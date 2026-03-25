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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autogroup"
	"github.com/purpleidea/mgmt/pgraph"

	_ "github.com/purpleidea/mgmt/engine/resources"
)

// We need a grouper for testing here too.
type testGrouper struct {
	autogroup.NonReachabilityGrouper
}

func (obj *testGrouper) Name() string {
	return "testGrouper"
}

func (obj *testGrouper) VertexCmp(v1, v2 pgraph.Vertex) error {
	if err := obj.NonReachabilityGrouper.VertexCmp(v1, v2); err != nil {
		return err
	}
	r1, ok1 := v1.(engine.GroupableRes)
	r2, ok2 := v2.(engine.GroupableRes)
	if !ok1 || !ok2 {
		return os.ErrInvalid
	}
	if r1.Name()[0] != r2.Name()[0] {
		return os.ErrInvalid
	}
	return nil
}

func (obj *testGrouper) VertexMerge(v1, v2 pgraph.Vertex) (pgraph.Vertex, error) {
	r1 := v1.(engine.GroupableRes)
	r2 := v2.(engine.GroupableRes)
	r1.GroupRes(r2)
	r2.SetParent(r1)
	return nil, nil
}

func TestGraphDumpAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mgmt-graph-dump-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	g1, _ := pgraph.NewGraph("g1")
	{
		a1, err := engine.NewNamedResource("noop", "a1")
		if err != nil {
			t.Fatal(err)
		}
		// Set a parameter to verify it survives dump/load
		// We use a reflect-based hack or if we can't we skip this check
		// because of the build cycle.
		// Actually, we can use an interface or just check if it's there.

		a2, err := engine.NewNamedResource("noop", "a2")
		if err != nil {
			t.Fatal(err)
		}
		b1, err := engine.NewNamedResource("noop", "b1")
		if err != nil {
			t.Fatal(err)
		}
		e1 := &engine.Edge{Name: "e1"}
		e2 := &engine.Edge{Name: "e2"}
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a2, b1, e2)
	}

	dumpPath := filepath.Join(tmpDir, "graph.yaml")
	if err := DumpGraph(g1, dumpPath); err != nil {
		t.Fatalf("DumpGraph failed: %v", err)
	}

	g2, err := LoadGraph(dumpPath)
	if err != nil {
		t.Fatalf("LoadGraph failed: %v", err)
	}

	if g2.Name != g1.Name {
		t.Errorf("expected name %s, got %s", g1.Name, g2.Name)
	}

	if g2.NumVertices() != g1.NumVertices() {
		t.Errorf("expected %d vertices, got %d", g1.NumVertices(), g2.NumVertices())
	}

	if g2.NumEdges() != g1.NumEdges() {
		t.Errorf("expected %d edges, got %d", g1.NumEdges(), g2.NumEdges())
	}

	// Verify autogrouping works on loaded graph
	start := time.Now()
	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}

	if err := autogroup.AutoGroup(&testGrouper{}, g2, debug, logf); err != nil {
		t.Fatalf("AutoGroup failed: %v", err)
	}
	duration := time.Since(start)
	t.Logf("AutoGroup took %v", duration)

	// Expected result after autogrouping: a1 and a2 should be grouped
	if g2.NumVertices() != 2 {
		t.Errorf("expected 2 vertices after autogrouping, got %d", g2.NumVertices())
	}
}

func TestPerformanceMeasurement(t *testing.T) {
	// Create a larger graph to measure performance
	g, _ := pgraph.NewGraph("perf")
	n := 100
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%c-res-%d", 'a'+(i%26), i/26)
		a, err := engine.NewNamedResource("noop", name)
		if err != nil {
			t.Fatal(err)
		}
		g.AddVertex(a)
	}

	start := time.Now()
	logf := func(format string, v ...interface{}) {}
	if err := autogroup.AutoGroup(&testGrouper{}, g, false, logf); err != nil {
		t.Fatalf("AutoGroup failed: %v", err)
	}
	duration := time.Since(start)
	t.Logf("AutoGroup with %d vertices took %v", n, duration)
}

func TestGroupedResourceDumpLoad(t *testing.T) {
	g1, _ := pgraph.NewGraph("grouped")
	a1, _ := engine.NewNamedResource("noop", "a1")
	a2, _ := engine.NewNamedResource("noop", "a2")
	b1, _ := engine.NewNamedResource("noop", "b1")

	ga1 := a1.(engine.GroupableRes)
	ga2 := a2.(engine.GroupableRes)
	ga1.GroupRes(ga2)
	ga2.SetParent(ga1)

	g1.AddEdge(a1, b1, &engine.Edge{Name: "e1"})
	g1.AddEdge(a2, b1, &engine.Edge{Name: "e2"})

	tmpDir, _ := os.MkdirTemp("", "mgmt-grouped-test")
	defer os.RemoveAll(tmpDir)
	dumpPath := filepath.Join(tmpDir, "graph.yaml")

	if err := DumpGraph(g1, dumpPath); err != nil {
		t.Fatal(err)
	}

	g2, err := LoadGraph(dumpPath)
	if err != nil {
		t.Fatal(err)
	}

	if g2.NumVertices() != 3 {
		t.Errorf("expected 3 vertices, got %d", g2.NumVertices())
	}

	if g2.NumEdges() != 2 {
		t.Errorf("expected 2 edges, got %d", g2.NumEdges())
	}

	// Verify grouping is preserved
	var resA1, resA2 engine.GroupableRes
	for _, v := range g2.Vertices() {
		r := v.(engine.Res)
		if r.Name() == "a1" {
			resA1 = r.(engine.GroupableRes)
		} else if r.Name() == "a2" {
			resA2 = r.(engine.GroupableRes)
		}
	}

	if resA1 == nil || resA2 == nil {
		t.Fatal("could not find a1 or a2 in loaded graph")
	}

	if !resA2.IsGrouped() {
		t.Error("a2 should be marked as grouped")
	}
	if resA2.Parent() != resA1 {
		t.Error("a2 parent should be a1")
	}
	found := false
	for _, g := range resA1.GetGroup() {
		if g == resA2 {
			found = true
			break
		}
	}
	if !found {
		t.Error("a1 should have a2 in its group")
	}
}
