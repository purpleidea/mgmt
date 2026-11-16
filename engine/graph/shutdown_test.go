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
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/pgraph"
)

// cleanupRes is a noop-like resource which counts how many times it was
// initialized and cleaned up, and which can error when it's cleaned up.
type cleanupRes struct {
	resources.NoopRes

	init *engine.Init

	cleanupErr error // what Cleanup returns

	inits    atomic.Int32
	cleanups atomic.Int32
}

// Init runs some startup code for this resource.
func (obj *cleanupRes) Init(init *engine.Init) error {
	obj.inits.Add(1)
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *cleanupRes) Cleanup() error {
	obj.cleanups.Add(1)
	return obj.cleanupErr
}

// Cmp compares two resources and returns an error if they are not equivalent.
// The graph swap keeps this vertex when the replacement compares equal.
func (obj *cleanupRes) Cmp(r engine.Res) error {
	if _, ok := r.(*cleanupRes); !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *cleanupRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil { // the initial startup event
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// newCleanupRes builds a cleanupRes. If fail is true, then its Cleanup errors.
func newCleanupRes(name string, fail bool) *cleanupRes {
	res := &cleanupRes{}
	if fail {
		res.cleanupErr = fmt.Errorf("cleanup %s failed", name)
	}
	res.SetKind("noop")
	res.SetName(name)
	return res
}

// newCleanupGraph builds a graph with no edges out of these resources.
func newCleanupGraph(t *testing.T, name string, resources ...*cleanupRes) *pgraph.Graph {
	t.Helper()
	g, err := pgraph.NewGraph(name)
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	for _, res := range resources {
		g.AddVertex(res)
	}
	return g
}

// shutdownTestEngine runs the engine Shutdown, then the cleanup which came from
// newTestEngine, and it returns what the Shutdown returned. If the Shutdown
// hangs, then it fails the test instead of blocking it forever. It doesn't run
// the cleanup in that case, because that waits on the engine as well, which is
// why we don't defer it in these tests.
func shutdownTestEngine(t *testing.T, ge *Engine, cleanup func()) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- ge.Shutdown()
	}()
	select {
	case err := <-done:
		cleanup()
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("engine Shutdown hung")
		return nil // unreachable
	}
}

// checkCleanups checks how many times each of these resources was initialized
// and cleaned up. Each one is cleaned up as it's removed from the graph.
func checkCleanups(t *testing.T, count int32, resources ...*cleanupRes) {
	t.Helper()
	for _, res := range resources {
		if n := res.inits.Load(); n != count {
			t.Errorf("expected %d inits of %s, got %d", count, res, n)
		}
		if n := res.cleanups.Load(); n != count {
			t.Errorf("expected %d cleanups of %s, got %d", count, res, n)
		}
	}
}

// checkCleanupErrors checks that err has the Cleanup error of each of these.
func checkCleanupErrors(t *testing.T, err error, resources ...*cleanupRes) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected the cleanup errors")
	}
	for _, res := range resources {
		if s := res.cleanupErr.Error(); !strings.Contains(err.Error(), s) {
			t.Errorf("missing the `%s` error in: %v", s, err)
		}
	}
}

// TestShutdownCleanupErrors checks that a resource which errors when it's
// cleaned up doesn't stop a Shutdown from removing the rest of the resources.
// They all error here, so whichever one is removed first, the rest are still to
// come. We must get all of their errors.
func TestShutdownCleanupErrors(t *testing.T) {
	ge, cleanup := newTestEngine(t)

	a := newCleanupRes("a", true)
	b := newCleanupRes("b", true)
	c := newCleanupRes("c", true)

	swapTestGraph(t, ge, newCleanupGraph(t, "first", a, b, c))
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}

	err := shutdownTestEngine(t, ge, cleanup)
	checkCleanupErrors(t, err, a, b, c)
	checkCleanups(t, 1, a, b, c)
	if n := len(ge.state); n != 0 {
		t.Errorf("expected no resource states, got %d", n)
	}
}

// TestSwapCleanupErrors checks that a resource which errors when it's cleaned
// up doesn't stop a graph swap from removing the rest of the resources that it
// has to. We must get all of their errors, and the Shutdown that follows must
// not remove them a second time.
func TestSwapCleanupErrors(t *testing.T) {
	ge, cleanup := newTestEngine(t)

	a := newCleanupRes("a", true)
	b := newCleanupRes("b", true)
	kept := newCleanupRes("kept", false)
	added := newCleanupRes("added", false)

	swapTestGraph(t, ge, newCleanupGraph(t, "first", a, b, kept))
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}

	// This removes the two which error, and adds one. The other is kept,
	// since the one we swap it for compares equal.
	second := newCleanupGraph(t, "second", newCleanupRes("kept", false), added)
	if err := ge.Load(second); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	err := ge.Commit(context.Background())
	checkCleanupErrors(t, err, a, b)
	checkCleanups(t, 1, a, b)

	_ = ge.Abort() // like the main loop in lib does when a Commit fails
	if err := shutdownTestEngine(t, ge, cleanup); err != nil {
		t.Fatalf("engine Shutdown: %v", err)
	}
	checkCleanups(t, 1, a, b, kept, added)
	if n := len(ge.state); n != 0 {
		t.Errorf("expected no resource states, got %d", n)
	}
}

// TestShutdownPendingGraph checks that a Shutdown which is run while a graph is
// still pending shuts down the graph that's running, and that it doesn't run
// the pending one instead. This is a mistake by the caller, so it's an error.
func TestShutdownPendingGraph(t *testing.T) {
	ge, cleanup := newTestEngine(t)

	running := newCleanupRes("running", false)
	pending := newCleanupRes("pending", false)

	swapTestGraph(t, ge, newCleanupGraph(t, "first", running))
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}
	if err := ge.Load(newCleanupGraph(t, "second", pending)); err != nil {
		t.Fatalf("engine Load: %v", err)
	}

	if err := shutdownTestEngine(t, ge, cleanup); err == nil {
		t.Errorf("expected an error about the pending graph")
	}
	checkCleanups(t, 1, running)
	checkCleanups(t, 0, pending) // it never ran
	if n := len(ge.state); n != 0 {
		t.Errorf("expected no resource states, got %d", n)
	}
}
