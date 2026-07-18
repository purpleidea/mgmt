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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func TestMultiErr(t *testing.T) {
	var err error
	e := fmt.Errorf("some error")
	err = errwrap.Append(err, e) // build an error from a nil base
	// ensure that this lib allows us to append to a nil
	if err == nil {
		t.Errorf("missing error")
	}
}

type timeoutCheckApplyRes struct {
	resources.NoopRes
}

func (obj *timeoutCheckApplyRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func TestSafeCheckApplyTimeout(t *testing.T) {
	res := &timeoutCheckApplyRes{}
	res.MetaParams().Timeout = 1

	start := time.Now()
	checkOK, err := safeCheckApply(context.Background(), res, true)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if checkOK {
		t.Fatalf("expected failed check result")
	}
	if !strings.Contains(err.Error(), "timeout after") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

// slowWatchRes is a noop-like resource with an artificially slow Watch startup,
// which records whether CheckApply ever ran before its Watch sent the initial
// startup event.
type slowWatchRes struct {
	resources.NoopRes

	init *engine.Init

	// delay is the artificial Watch startup delay before the initial event.
	// It simulates a Watch which is slow to build its connections.
	delay time.Duration

	watchStarted    atomic.Bool // set right before the initial event
	earlyCheckApply atomic.Bool // did CheckApply run before that event?

	checkApplyOnce sync.Once
	checkApplyDone chan struct{} // closed on the first CheckApply
}

// Init runs some startup code for this resource.
func (obj *slowWatchRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *slowWatchRes) Watch(ctx context.Context) error {
	select {
	case <-time.After(obj.delay): // simulate a slow connect
	case <-ctx.Done():
		return ctx.Err()
	}

	obj.watchStarted.Store(true) // must happen before the initial event
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. Here it only does bookkeeping.
func (obj *slowWatchRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if !obj.watchStarted.Load() {
		obj.earlyCheckApply.Store(true)
	}
	obj.checkApplyOnce.Do(func() {
		close(obj.checkApplyDone)
	})
	return true, nil
}

// TestNoCheckApplyBeforeWatchStartup checks the engine invariant that the
// CheckApply of a resource must never run before the Watch of that resource has
// sent its initial startup event. A parent resource which finishes its own
// CheckApply quickly pokes its children right away, and that poke must not run
// the CheckApply of a child while the Watch of that child is still starting.
func TestNoCheckApplyBeforeWatchStartup(t *testing.T) {
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

	parent := &resources.NoopRes{}
	parent.SetKind("noop")
	parent.SetName("parent")

	child := &slowWatchRes{
		delay:          time.Second,
		checkApplyDone: make(chan struct{}),
	}
	child.SetKind("noop")
	child.SetName("child")

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(parent)
	g.AddVertex(child)
	g.AddEdge(parent, child, &engine.Edge{Name: "parent -> child"})

	if err := ge.Load(g); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	if err := ge.Pause(false); err != nil { // see the main loop in lib
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
		if err := ge.Pause(false); err != nil {
			t.Errorf("engine Pause: %v", err)
		}
	}()

	select {
	case <-child.checkApplyDone:
	case <-time.After(10 * time.Second):
		t.Fatalf("child CheckApply never ran")
	}

	if child.earlyCheckApply.Load() {
		t.Errorf("child CheckApply ran before its Watch sent the initial event")
	}
}

// restartWatchRes is a noop-like resource whose first Watch fails on purpose
// while its first CheckApply is still running. It records whether a restarted
// Watch ever overlapped with that running CheckApply, and whether the engine
// interrupted that CheckApply for the restart.
type restartWatchRes struct {
	resources.NoopRes

	init *engine.Init

	watchCount      atomic.Int32
	checkApplyCount atomic.Int32

	checkApplyRunning atomic.Bool // is a CheckApply currently running?
	overlap           atomic.Bool // did a restarted Watch overlap with it?
	interrupted       atomic.Bool // was the first CheckApply interrupted?

	firstCheckApplyStarted chan struct{} // closed when CheckApply first runs

	checkApplyOnce sync.Once
	checkApplyDone chan struct{} // closed on the second CheckApply
}

// Init runs some startup code for this resource.
func (obj *restartWatchRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Watch is the primary listener for this resource and it outputs events. The
// first invocation fails once the first CheckApply is running, so that the
// engine retries it.
func (obj *restartWatchRes) Watch(ctx context.Context) error {
	count := obj.watchCount.Add(1)
	if count > 1 && obj.checkApplyRunning.Load() {
		obj.overlap.Store(true) // a restarted Watch must not see this
	}

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	if count == 1 {
		select {
		case <-obj.firstCheckApplyStarted:
		case <-ctx.Done():
			return ctx.Err()
		}
		return fmt.Errorf("watch failed on purpose")
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. The first invocation blocks until
// its context is interrupted, or until a timeout which simulates slow work and
// keeps an engine without the interrupt from hanging this test.
func (obj *restartWatchRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	obj.checkApplyRunning.Store(true)
	defer obj.checkApplyRunning.Store(false)

	if obj.checkApplyCount.Add(1) == 1 {
		close(obj.firstCheckApplyStarted)
		select {
		case <-ctx.Done():
			obj.interrupted.Store(true)
			return false, ctx.Err()
		case <-time.After(3 * time.Second):
			return true, nil // the engine never interrupted us
		}
	}

	obj.checkApplyOnce.Do(func() {
		close(obj.checkApplyDone)
	})
	return true, nil
}

// TestWatchRestartSerializesCheckApply checks the engine invariant that when
// Watch fails and gets retried, any running CheckApply is interrupted and has
// finished before the replacement Watch starts, and that no new CheckApply runs
// until that replacement Watch sends its initial startup event.
func TestWatchRestartSerializesCheckApply(t *testing.T) {
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

	res := &restartWatchRes{
		firstCheckApplyStarted: make(chan struct{}),
		checkApplyDone:         make(chan struct{}),
	}
	res.SetKind("noop")
	res.SetName("restarter")
	res.MetaParams().Retry = 1 // allow one Watch retry

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(res)

	if err := ge.Load(g); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	if err := ge.Pause(false); err != nil { // see the main loop in lib
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
		if err := ge.Pause(false); err != nil {
			t.Errorf("engine Pause: %v", err)
		}
	}()

	select {
	case <-res.checkApplyDone:
	case <-time.After(10 * time.Second):
		t.Errorf("second CheckApply never ran")
	}

	if c := res.watchCount.Load(); c < 2 {
		t.Errorf("watch never restarted, ran %d time(s)", c)
	}
	if res.overlap.Load() {
		t.Errorf("a restarted Watch overlapped with a running CheckApply")
	}
	if !res.interrupted.Load() {
		t.Errorf("the first CheckApply was not interrupted for the restart")
	}
}

// swallowedEventRes is a noop-like resource whose Watch sends a change event
// while its first CheckApply is still running. If the dirty mark of that event
// gets clobbered when the first CheckApply completes successfully, then the
// event never causes a second CheckApply, and the change is swallowed.
type swallowedEventRes struct {
	resources.NoopRes

	init *engine.Init

	checkApplyCount atomic.Int32

	checkApplyStarted chan struct{} // closed when the first CheckApply runs
	eventSending      chan struct{} // closed just before the change event

	checkApplyOnce sync.Once
	checkApplyDone chan struct{} // closed on the second CheckApply
}

// Init runs some startup code for this resource.
func (obj *swallowedEventRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Watch is the primary listener for this resource and it outputs events. It
// sends one change event while the first CheckApply is still running.
func (obj *swallowedEventRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil { // initial event
		return err
	}

	select {
	case <-obj.checkApplyStarted:
	case <-ctx.Done():
		return ctx.Err()
	}

	close(obj.eventSending)
	if err := obj.init.Event(ctx); err != nil { // the change notification
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// CheckApply checks the state and applies it. The first invocation returns
// successfully only after Watch is inside its blocking change event send.
func (obj *swallowedEventRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.checkApplyCount.Add(1) == 1 {
		close(obj.checkApplyStarted)
		select {
		case <-obj.eventSending:
		case <-ctx.Done():
			return false, ctx.Err()
		}
		// Wait for the Watch goroutine to make it into the blocking
		// event send, so that our success below is what runs last.
		time.Sleep(200 * time.Millisecond)
		return true, nil
	}

	obj.checkApplyOnce.Do(func() {
		close(obj.checkApplyDone)
	})
	return true, nil
}

// TestEventDuringCheckApplyNotSwallowed checks that an event which Watch sends
// while a CheckApply is still running is not swallowed when that CheckApply
// completes successfully, and that it causes another CheckApply to run.
func TestEventDuringCheckApplyNotSwallowed(t *testing.T) {
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

	res := &swallowedEventRes{
		checkApplyStarted: make(chan struct{}),
		eventSending:      make(chan struct{}),
		checkApplyDone:    make(chan struct{}),
	}
	res.SetKind("noop")
	res.SetName("swallower")

	g, err := pgraph.NewGraph("test")
	if err != nil {
		t.Fatalf("pgraph NewGraph: %v", err)
	}
	g.AddVertex(res)

	if err := ge.Load(g); err != nil {
		t.Fatalf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine Validate: %v", err)
	}
	if err := ge.Pause(false); err != nil { // see the main loop in lib
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
		if err := ge.Pause(false); err != nil {
			t.Errorf("engine Pause: %v", err)
		}
	}()

	select {
	case <-res.checkApplyDone:
	case <-time.After(5 * time.Second):
		t.Errorf("the change event was swallowed, CheckApply never re-ran")
	}
}

func TestStatePauseClosed(t *testing.T) {
	doneCtx, doneCtxCancel := context.WithCancel(context.Background())
	doneCtxCancel()

	state := &State{
		doneCtx: doneCtx,
		paused:  true,
	}
	if err := state.Pause(); err != engine.ErrClosed {
		t.Fatalf("expected closed error, got: %v", err)
	}
}
