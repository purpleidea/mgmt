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
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
)

// asyncRes is a noop-like resource with the async trait. Its CheckApply hands
// control to the test on entry, and then blocks until the test releases it or
// its context is cancelled. Both signals are unbuffered, so they pair up with
// one run at a time, and the same resource can be driven through several runs.
// Its Watch sends one more event when the test asks for one.
type asyncRes struct {
	resources.NoopRes
	traits.Async

	init *engine.Init

	trigger chan struct{} // closed by the test to send the second event

	entered chan struct{} // CheckApply sends here on entry
	release chan struct{} // CheckApply blocks until it receives from here

	checkApplyCount atomic.Int32
	running         atomic.Bool // is a CheckApply currently running?
	interrupted     atomic.Bool // did a CheckApply see its context cancelled?
}

// Init runs some startup code for this resource.
func (obj *asyncRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Cmp compares two resources and returns an error if they are not equivalent.
// The graph swap keeps this vertex when the replacement compares equal.
func (obj *asyncRes) Cmp(r engine.Res) error {
	if _, ok := r.(*asyncRes); !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *asyncRes) Watch(ctx context.Context) error {
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

// CheckApply checks the state and applies it. It blocks until released.
func (obj *asyncRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	obj.checkApplyCount.Add(1)
	obj.running.Store(true)
	defer obj.running.Store(false)

	select {
	case obj.entered <- struct{}{}:
	case <-ctx.Done():
		obj.interrupted.Store(true)
		return false, ctx.Err()
	}

	select {
	case <-obj.release:
	case <-ctx.Done():
		obj.interrupted.Store(true)
		return false, ctx.Err()
	}
	return true, nil
}

// witnessRes is a noop-like resource whose CheckApply records whether the
// CheckApply of an unrelated async resource was running at the time.
type witnessRes struct {
	resources.NoopRes

	init *engine.Init

	async *asyncRes

	sawRunning atomic.Bool // was the async CheckApply running when we ran?

	checkApplyOnce sync.Once
	checkApplyDone chan struct{} // closed on the first CheckApply
}

// Init runs some startup code for this resource.
func (obj *witnessRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *witnessRes) Cmp(r engine.Res) error {
	if _, ok := r.(*witnessRes); !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *witnessRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. Here it only does bookkeeping.
func (obj *witnessRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.async.running.Load() {
		obj.sawRunning.Store(true)
	}
	obj.checkApplyOnce.Do(func() {
		close(obj.checkApplyDone)
	})
	return true, nil
}

// newTestEngine builds a running converger and an initialized engine for a
// test. The returned cleanup stops the converger, and must run after the engine
// is shut down.
func newTestEngine(t *testing.T) (*Engine, func()) {
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
	convWg.Add(1)
	go func() {
		defer convWg.Done()
		_ = conv.Run(convCtx, false) // errors on context cancel
	}()
	cleanup := func() {
		convCancel()
		convWg.Wait()
	}

	ge := &Engine{
		Program:   "mgmt",
		Version:   "0.0.1",
		Hostname:  "localhost",
		Converger: conv,
		Prefix:    t.TempDir(),
		Logf:      logf,
	}
	if err := ge.Init(); err != nil {
		cleanup()
		t.Fatalf("engine Init: %v", err)
	}

	return ge, cleanup
}

// swapTestGraph loads, validates, pauses, commits, and resumes a graph.
func swapTestGraph(t *testing.T, ge *Engine, g *pgraph.Graph) {
	if err := ge.Load(g); err != nil {
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
}

// TestAsyncCheckApplyAcrossSwap checks that the CheckApply of an async resource
// doesn't block a graph swap, that the rest of the new graph runs while it is
// still going, and that a swap which removes that resource waits for it.
func TestAsyncCheckApplyAcrossSwap(t *testing.T) {
	ge, cleanup := newTestEngine(t)
	defer cleanup()

	newAsync := func() *asyncRes {
		res := &asyncRes{
			trigger: make(chan struct{}),
			entered: make(chan struct{}),
			release: make(chan struct{}),
		}
		res.SetKind("noop")
		res.SetName("async")
		return res
	}
	slow := newAsync() // the one which actually runs

	g1, err := pgraph.NewGraph("first")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g1.AddVertex(slow)
	swapTestGraph(t, ge, g1)
	defer func() {
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()
	// If we fail partway, a CheckApply might still be blocked, and the
	// shutdown above would then wait on it forever. This runs first.
	defer close(slow.release)

	select {
	case <-slow.entered: // the first CheckApply is now running
	case <-time.After(10 * time.Second):
		t.Fatalf("async CheckApply never ran")
	}

	// A pause must land while that CheckApply is still running.
	paused := make(chan error, 1)
	go func() {
		paused <- ge.Pause()
	}()
	select {
	case err := <-paused:
		if err != nil {
			t.Fatalf("engine Pause: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("engine Pause blocked on the async CheckApply")
	}

	// Swap in an unrelated resource, and check that it runs while the async
	// CheckApply is still going.
	newWitness := func() *witnessRes {
		res := &witnessRes{
			async:          slow,
			checkApplyDone: make(chan struct{}),
		}
		res.SetKind("noop")
		res.SetName("witness")
		return res
	}
	witness := newWitness()

	g2, err := pgraph.NewGraph("second")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g2.AddVertex(newAsync()) // compares equal, so the running one is kept
	g2.AddVertex(witness)
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
	case <-witness.checkApplyDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("witness CheckApply never ran")
	}
	if !witness.sawRunning.Load() {
		t.Errorf("witness CheckApply ran only after the async CheckApply finished")
	}
	if !slow.running.Load() {
		t.Errorf("the async CheckApply is not running anymore")
	}

	slow.release <- struct{}{} // let the first CheckApply finish

	// Start a second CheckApply, and then remove the resource. That swap
	// must wait for the CheckApply to finish, and must not cancel it.
	close(slow.trigger)
	select {
	case <-slow.entered: // the second CheckApply is now running
	case <-time.After(10 * time.Second):
		t.Fatalf("async CheckApply never ran a second time")
	}

	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}
	g3, err := pgraph.NewGraph("third")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g3.AddVertex(newWitness()) // the async resource is gone
	if err := ge.Load(g3); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	committed := make(chan error, 1)
	go func() {
		committed <- ge.Commit(context.Background())
	}()
	select {
	case err := <-committed:
		t.Fatalf("engine Commit returned before the async CheckApply finished: %v", err)
	case <-time.After(time.Second):
		// still blocked, as it should be
	}
	if !slow.running.Load() {
		t.Errorf("the removed async CheckApply is not running anymore")
	}

	slow.release <- struct{}{} // let the second CheckApply finish
	select {
	case err := <-committed:
		if err != nil {
			t.Fatalf("engine Commit: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("engine Commit never returned")
	}
	if err := ge.Resume(); err != nil {
		t.Fatalf("engine Resume: %v", err)
	}
	defer func() {
		if err := ge.Pause(); err != nil {
			t.Errorf("engine Pause: %v", err)
		}
	}()

	if slow.interrupted.Load() {
		t.Errorf("the removal cancelled the async CheckApply instead of waiting for it")
	}
	if c := slow.checkApplyCount.Load(); c != 2 {
		t.Errorf("the async CheckApply ran %d times, expected 2", c)
	}
}

// TestAsyncCheckApplySoftInterrupt checks that the user interrupt still cancels
// the context of a running async CheckApply, even though a removal doesn't.
func TestAsyncCheckApplySoftInterrupt(t *testing.T) {
	ge, cleanup := newTestEngine(t)
	defer cleanup()

	slow := &asyncRes{
		trigger: make(chan struct{}),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	slow.SetKind("noop")
	slow.SetName("async")

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(slow)
	swapTestGraph(t, ge, g)
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
	case <-slow.entered: // the CheckApply is now running
	case <-time.After(10 * time.Second):
		t.Fatalf("async CheckApply never ran")
	}

	ge.SoftInterrupt() // the second ^C

	for i := 0; slow.running.Load(); i++ {
		if i > 1000 {
			t.Fatalf("the interrupt never reached the async CheckApply")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !slow.interrupted.Load() {
		t.Errorf("the async CheckApply returned without seeing a cancelled context")
	}
}
