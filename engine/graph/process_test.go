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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/pgraph"
)

// runningUpstreamRes is a noop-like resource whose Watch sends a second event
// when the test asks for one, and whose second CheckApply takes a while. It
// records whether that CheckApply is currently running, so that a downstream
// resource can check if it was allowed to overlap with it.
type runningUpstreamRes struct {
	resources.NoopRes

	init *engine.Init

	// slow is how long the second CheckApply blocks for.
	slow time.Duration

	trigger chan struct{} // closed by the test to send the second event

	checkApplyCount   atomic.Int32
	checkApplyRunning atomic.Bool // is the slow CheckApply currently running?

	firstOnce sync.Once
	firstDone chan struct{} // closed when the first CheckApply finishes
}

// Init runs some startup code for this resource.
func (obj *runningUpstreamRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Cmp compares two resources and returns an error if they are not equivalent.
// The graph swap keeps this vertex when the replacement compares equal, which
// is what we want, since the whole point is to keep it running across the swap.
func (obj *runningUpstreamRes) Cmp(r engine.Res) error {
	if _, ok := r.(*runningUpstreamRes); !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *runningUpstreamRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil { // the initial startup event
		return err
	}

	select {
	case <-obj.trigger:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. The second run is slow.
func (obj *runningUpstreamRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.checkApplyCount.Add(1) == 1 {
		obj.firstOnce.Do(func() {
			close(obj.firstDone)
		})
		return true, nil
	}

	obj.checkApplyRunning.Store(true)
	defer obj.checkApplyRunning.Store(false)
	select {
	case <-time.After(obj.slow):
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return true, nil
}

// overlapDownstreamRes is a noop-like resource with a slow Watch startup, whose
// CheckApply records whether the upstream CheckApply was running at the time.
type overlapDownstreamRes struct {
	resources.NoopRes

	init *engine.Init

	// delay is the artificial Watch startup delay before the initial event.
	// It makes sure the upstream is already in its CheckApply by the time
	// we first try to run ours.
	delay time.Duration

	upstream *runningUpstreamRes

	overlap atomic.Bool // did our CheckApply overlap with the upstream one?

	checkApplyOnce sync.Once
	checkApplyDone chan struct{} // closed on the first CheckApply
}

// Init runs some startup code for this resource.
func (obj *overlapDownstreamRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *overlapDownstreamRes) Watch(ctx context.Context) error {
	select {
	case <-time.After(obj.delay): // simulate a slow connect
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. Here it only does bookkeeping.
func (obj *overlapDownstreamRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.upstream.checkApplyRunning.Load() {
		obj.overlap.Store(true)
	}
	obj.checkApplyOnce.Do(func() {
		close(obj.checkApplyDone)
	})
	return true, nil
}

// TestNewVertexWaitsForRunningUpstream checks that a vertex which has never run
// doesn't start its CheckApply while a prerequisite is in the middle of one.
// The timestamp ordering alone can't catch this, because a vertex with a zero
// timestamp passes the comparison against any prerequisite which has completed
// at least once before. The common way to get there is a graph swap which adds
// a new downstream vertex while the kept upstream one still has work to do.
func TestNewVertexWaitsForRunningUpstream(t *testing.T) {
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}

	conv := &converger.Coordinator{
		Timeout: -1, // disabled
		Logf: func(format string, v ...interface{}) {
			logf("converger: "+format, v...)
		},
	}
	if err := conv.Init(); err != nil {
		t.Fatalf("converger Init: %v", err)
	}
	convCtx, convCancel := context.WithCancel(context.Background())
	convWg := &sync.WaitGroup{}
	defer convWg.Wait()
	defer convCancel()
	convWg.Add(1)
	go func() {
		defer convWg.Done()
		_ = conv.Run(convCtx, false) // errors on context cancel
	}()

	ge := &Engine{
		Program:   "mgmt",
		Version:   "0.0.1",
		Hostname:  "localhost",
		Converger: conv,
		Prefix:    t.TempDir(),
		Logf:      logf,
	}
	if err := ge.Init(); err != nil {
		t.Fatalf("engine Init: %v", err)
	}

	newParent := func() *runningUpstreamRes {
		res := &runningUpstreamRes{
			slow:      2 * time.Second,
			trigger:   make(chan struct{}),
			firstDone: make(chan struct{}),
		}
		res.SetKind("noop")
		res.SetName("parent")
		return res
	}
	parent := newParent() // the one which actually runs

	g1, err := pgraph.NewGraph("first")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g1.AddVertex(parent)

	if err := ge.Load(g1); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	if err := ge.Pause(); err != nil { // see the main loop in lib
		t.Fatalf("engine Pause: %v", err)
	}
	if err := ge.Commit(context.Background()); err != nil {
		t.Fatalf("engine Commit: %v", err)
	}
	if err := ge.Resume(); err != nil {
		t.Fatalf("engine Resume: %v", err)
	}
	defer func() {
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()
	defer func() {
		if err := ge.Pause(); err != nil {
			t.Errorf("engine Pause: %v", err)
		}
	}()

	select {
	case <-parent.firstDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("parent CheckApply never ran")
	}

	// Pause, and only then trigger the second event. The parent is idle, so
	// it acks the pause at once, and its Watch then blocks sending the
	// event until we resume. That leaves the parent dirty across the swap,
	// and the moment we resume it starts its slow CheckApply.
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}
	close(parent.trigger)

	child := &overlapDownstreamRes{
		delay:          500 * time.Millisecond,
		upstream:       parent,
		checkApplyDone: make(chan struct{}),
	}
	child.SetKind("noop")
	child.SetName("child")

	g2, err := pgraph.NewGraph("second")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	replacement := newParent() // compares equal, so the running one is kept
	g2.AddVertex(replacement)
	g2.AddVertex(child)
	g2.AddEdge(replacement, child, &engine.Edge{Name: "parent -> child"})

	if err := ge.Load(g2); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	if err := ge.Commit(context.Background()); err != nil {
		t.Fatalf("engine Commit: %v", err)
	}
	if err := ge.Resume(); err != nil {
		t.Fatalf("engine Resume: %v", err)
	}

	select {
	case <-child.checkApplyDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("child CheckApply never ran")
	}

	if child.overlap.Load() {
		t.Errorf("child CheckApply ran while the parent CheckApply was still running")
	}
	if c := parent.checkApplyCount.Load(); c < 2 {
		t.Errorf("parent CheckApply ran %d times, expected the slow second run", c)
	}
}
