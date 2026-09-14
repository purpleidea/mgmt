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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/pgraph"
)

// flakyRes is a noop-like resource whose CheckApply fails a set number of times
// before it succeeds, so that a test can hold it in the retry delay.
type flakyRes struct {
	resources.NoopRes

	init *engine.Init

	// failures is how many CheckApply runs fail before one succeeds. A
	// negative value means every one of them fails.
	failures int32

	checkApplyCount atomic.Int32

	checkApplyOnce sync.Once
	checkApplyDone chan struct{} // closed on the first CheckApply
}

// Init runs some startup code for this resource.
func (obj *flakyRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *flakyRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. It fails until it doesn't.
func (obj *flakyRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	count := obj.checkApplyCount.Add(1)
	obj.checkApplyOnce.Do(func() {
		close(obj.checkApplyDone)
	})
	if obj.failures < 0 || count <= obj.failures {
		return false, fmt.Errorf("failure %d", count)
	}
	return true, nil
}

// realizePauseInRetry runs a resource which fails once and then succeeds after
// a retry delay, pauses the engine during that delay, and returns how many
// CheckApply runs had happened by the time the pause landed.
func realizePauseInRetry(t *testing.T, realize bool) int32 {
	ge, cleanup := newTestEngine(t)
	defer cleanup()

	res := &flakyRes{
		failures:       1,
		checkApplyDone: make(chan struct{}),
	}
	res.SetKind("noop")
	res.SetName("flaky")
	res.MetaParams().Realize = realize
	res.MetaParams().Retry = 1
	res.MetaParams().Delay = 1500 // ms

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(res)
	swapTestGraph(t, ge, g)
	defer func() {
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()

	select {
	case <-res.checkApplyDone: // the failure happened, the delay starts
	case <-time.After(10 * time.Second):
		t.Fatalf("the CheckApply never ran")
	}

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
		t.Fatalf("engine Pause never landed")
	}

	return res.checkApplyCount.Load()
}

// TestRealizePauseWaitsForRetry checks that a realize resource holds up a pause
// until its pending work is done, here through a retry delay, and that without
// the metaparam the pause lands in that delay as it always has.
func TestRealizePauseWaitsForRetry(t *testing.T) {
	if c := realizePauseInRetry(t, false); c != 1 {
		t.Errorf("without realize, the pause landed after %d CheckApply runs, expected 1", c)
	}
	if c := realizePauseInRetry(t, true); c != 2 {
		t.Errorf("with realize, the pause landed after %d CheckApply runs, expected 2", c)
	}
}

// TestRealizePauseInterrupted checks that a user interrupt overrides a realize
// resource which is withholding the pause, here forever, since it never stops
// failing.
func TestRealizePauseInterrupted(t *testing.T) {
	ge, cleanup := newTestEngine(t)
	defer cleanup()

	res := &flakyRes{
		failures:       -1, // always
		checkApplyDone: make(chan struct{}),
	}
	res.SetKind("noop")
	res.SetName("flaky")
	res.MetaParams().Realize = true
	res.MetaParams().Retry = -1  // forever
	res.MetaParams().Delay = 500 // ms

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(res)
	swapTestGraph(t, ge, g)
	defer func() {
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()

	select {
	case <-res.checkApplyDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("the CheckApply never ran")
	}

	paused := make(chan error, 1)
	go func() {
		paused <- ge.Pause()
	}()
	select {
	case err := <-paused:
		t.Fatalf("engine Pause landed while the realize resource was pending: %v", err)
	case <-time.After(2 * time.Second):
		// withheld, as it should be
	}

	ge.SoftInterrupt() // the second ^C

	select {
	case err := <-paused:
		if err != nil {
			t.Fatalf("engine Pause: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("engine Pause never landed after the interrupt")
	}
}

// realizeAsyncPrereq builds an async resource feeding a downstream resource,
// holds the async CheckApply in flight, and then pauses the engine. It returns
// whether the pause was withheld while the async CheckApply was still running,
// and how many times the downstream CheckApply had run by the time the pause
// landed. When the downstream resource has the realize metaparam, its async
// prerequisite joins the realize set, so the pause must wait for that
// CheckApply to finish and for the downstream resource to run afterwards.
func realizeAsyncPrereq(t *testing.T, realize bool) (bool, int32) {
	ge, cleanup := newTestEngine(t)
	defer cleanup()

	slow := &asyncRes{
		trigger: make(chan struct{}),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	slow.SetKind("noop")
	slow.SetName("async")

	res := &flakyRes{
		failures:       0, // never fails
		checkApplyDone: make(chan struct{}),
	}
	res.SetKind("noop")
	res.SetName("downstream")
	res.MetaParams().Realize = realize

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(slow)
	g.AddVertex(res)
	g.AddEdge(slow, res, &engine.Edge{Name: "async -> downstream"})
	swapTestGraph(t, ge, g)
	defer func() {
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()
	defer close(slow.release) // unblock the CheckApply if we fail partway

	select {
	case <-slow.entered: // the async CheckApply is now running
	case <-time.After(10 * time.Second):
		t.Fatalf("async CheckApply never ran")
	}

	// The downstream resource is now backpoke-blocked on the in-flight async
	// prerequisite, so it hasn't run yet. Start a pause and see if it lands.
	paused := make(chan error, 1)
	go func() {
		paused <- ge.Pause()
	}()

	blocked := false
	select {
	case err := <-paused:
		if err != nil {
			t.Fatalf("engine Pause: %v", err)
		}
	case <-time.After(2 * time.Second):
		blocked = true // withheld while the async CheckApply runs
	}

	slow.release <- struct{}{} // let the async CheckApply finish

	if blocked { // it was withheld, so now it should land
		select {
		case err := <-paused:
			if err != nil {
				t.Fatalf("engine Pause: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("engine Pause never landed after the prerequisite finished")
		}
	}

	return blocked, res.checkApplyCount.Load()
}

// TestRealizeWaitsForAsyncPrerequisite checks that a realize resource holds up
// a pause until an in-flight async prerequisite finishes and the realize
// resource runs afterwards, and that without the metaparam the async
// prerequisite lets the pause through at once and the downstream resource never
// runs.
func TestRealizeWaitsForAsyncPrerequisite(t *testing.T) {
	if blocked, c := realizeAsyncPrereq(t, false); blocked || c != 0 {
		t.Errorf("without realize, pause blocked=%v after %d downstream runs, expected false and 0", blocked, c)
	}
	if blocked, c := realizeAsyncPrereq(t, true); !blocked || c != 1 {
		t.Errorf("with realize, pause blocked=%v after %d downstream runs, expected true and 1", blocked, c)
	}
}
