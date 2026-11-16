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
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
)

const (
	backgroundErrorKind     = "graph-test-background-error"
	backgroundNilKind       = "graph-test-background-nil"
	backgroundKeyedKind     = "graph-test-background-keyed"
	backgroundStopErrorKind = "graph-test-background-stop-error"
)

var errBackgroundTest = errors.New("background test error")

func init() {
	engine.RegisterResource(backgroundErrorKind, func() engine.Res { return &backgroundErrorRes{} })
	engine.RegisterResource(backgroundNilKind, func() engine.Res { return &backgroundNilRes{} })
	engine.RegisterResource(backgroundKeyedKind, func() engine.Res { return &backgroundKeyedRes{} })
	engine.RegisterResource(backgroundStopErrorKind, func() engine.Res { return &backgroundStopErrorRes{} })
}

var _ engine.BackgroundRes = &backgroundErrorRes{}     // compile time check
var _ engine.BackgroundRes = &backgroundNilRes{}       // compile time check
var _ engine.BackgroundRes = &backgroundKeyedRes{}     // compile time check
var _ engine.BackgroundRes = &backgroundStopErrorRes{} // compile time check

// backgroundErrorRes has a background function which errors before it's ready.
type backgroundErrorRes struct {
	resources.NoopRes
	traits.Backgroundable
}

// Background errors without ever sending the ready signal.
func (obj *backgroundErrorRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		return errBackgroundTest
	}
}

// backgroundNilRes has a background function which exits before it's ready.
type backgroundNilRes struct {
	resources.NoopRes
	traits.Backgroundable
}

// Background exits without an error, and without sending the ready signal.
func (obj *backgroundNilRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		return nil
	}
}

// backgroundKeyedRes is a noop-like resource whose background key is a param.
// Its background function and its Watch log when they start and when they stop.
type backgroundKeyedRes struct {
	resources.NoopRes

	init *engine.Init

	// BgKey is the background key of this resource.
	BgKey string

	// Value only exists so that changing it makes a graph swap replace the
	// resource, while it keeps the same key.
	Value int
}

// Init runs some startup code for this resource.
func (obj *backgroundKeyedRes) Init(init *engine.Init) error {
	obj.init = init // save for later
	return obj.NoopRes.Init(init)
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *backgroundKeyedRes) Cmp(r engine.Res) error {
	res, ok := r.(*backgroundKeyedRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	if obj.BgKey != res.BgKey {
		return fmt.Errorf("the BgKey differs")
	}
	if obj.Value != res.Value {
		return fmt.Errorf("the Value differs")
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *backgroundKeyedRes) Watch(ctx context.Context) error {
	backgroundKeyedLog.add("watch " + obj.BgKey)
	defer backgroundKeyedLog.add("unwatch " + obj.BgKey)

	if err := obj.init.Event(ctx); err != nil { // the initial startup event
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}
	return ctx.Err()
}

// Background logs when it starts and when it stops.
func (obj *backgroundKeyedRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		backgroundKeyedLog.add("start " + handle.Key)
		defer backgroundKeyedLog.add("stop " + handle.Key)

		close(ready)

		select {
		case <-ctx.Done():
		}
		return ctx.Err()
	}
}

// BackgroundKey returns the key param.
func (obj *backgroundKeyedRes) BackgroundKey() string {
	return obj.BgKey
}

// backgroundStopErrorRes reports a cleanup error after its background stops.
type backgroundStopErrorRes struct {
	backgroundKeyedRes
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *backgroundStopErrorRes) Cmp(r engine.Res) error {
	res, ok := r.(*backgroundStopErrorRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	return obj.backgroundKeyedRes.Cmp(&res.backgroundKeyedRes)
}

// Background logs its lifecycle and returns a cleanup error on cancellation.
func (obj *backgroundStopErrorRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		backgroundKeyedLog.add("start " + handle.Key)
		close(ready)
		<-ctx.Done()
		backgroundKeyedLog.add("stop " + handle.Key)
		handle.Logf("stopped %s", handle.Key)
		return fmt.Errorf("stop %s failed", handle.Key)
	}
}

// backgroundKeyedLog records what the background functions and the Watch
// functions of backgroundKeyedRes did, in the order that they did it. It's a
// global, because a background function isn't built from a graph resource.
var backgroundKeyedLog = &backgroundEventLog{}

// backgroundEventLog is an ordered list of events, which is safe to use from
// more than one goroutine.
type backgroundEventLog struct {
	mutex  sync.Mutex
	events []string
}

// add appends an event to the log.
func (obj *backgroundEventLog) add(event string) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.events = append(obj.events, event)
}

// take waits until there are at least n events, and then it returns all of them
// and empties the log. If they don't arrive in time, it returns what it has,
// and the caller will notice the wrong count.
func (obj *backgroundEventLog) take(n int) []string {
	deadline := time.Now().Add(10 * time.Second)
	for {
		obj.mutex.Lock()
		if len(obj.events) >= n || time.Now().After(deadline) {
			events := obj.events
			obj.events = []string{}
			obj.mutex.Unlock()
			return events
		}
		obj.mutex.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

// checkBackgroundEvents checks that the events happened in the order of the
// groups. The order of the events inside of each group doesn't matter.
func checkBackgroundEvents(t *testing.T, events []string, groups ...[]string) {
	t.Helper()
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	if len(events) != count {
		t.Fatalf("expected %d events, got: %v", count, events)
	}
	i := 0
	for _, group := range groups {
		got := slices.Clone(events[i : i+len(group)])
		exp := slices.Clone(group)
		slices.Sort(got)
		slices.Sort(exp)
		if !slices.Equal(got, exp) {
			t.Fatalf("expected %v at position %d, got: %v", group, i, events)
		}
		i += len(group)
	}
}

// TestStartBackgroundEarlyExit checks that a background function which exits
// before it's ready makes StartBackground return right away, with its error.
func TestStartBackgroundEarlyExit(t *testing.T) {
	tests := []struct {
		kind   string
		cancel error // what the engine Cancel func should receive
	}{
		{backgroundErrorKind, errBackgroundTest},
		{backgroundNilKind, nil},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			mutex := &sync.Mutex{}
			var cause error
			obj := &Engine{
				Cancel: func(err error) {
					mutex.Lock()
					defer mutex.Unlock()
					cause = err
				},
				Logf:    t.Logf,
				bgState: make(map[bgID]*bgState),
			}

			// This ctx is never cancelled by the test, so a hang
			// here would mean that we only noticed the exit on a
			// timeout.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := obj.StartBackground(ctx, tc.kind, "")
			if err == nil {
				t.Fatalf("expected an error")
			}
			if ctx.Err() != nil {
				t.Fatalf("we timed out instead: %v", err)
			}
			if tc.cancel != nil && !errors.Is(err, tc.cancel) {
				t.Errorf("unexpected error: %v", err)
			}
			if n := len(obj.bgState); n != 0 {
				t.Errorf("expected no background state, got %d", n)
			}

			mutex.Lock()
			defer mutex.Unlock()
			if cause != tc.cancel {
				t.Errorf("unexpected engine cancel: %v", cause)
			}
		})
	}
}

// TestBackgroundKeys checks that there's one background function for each kind
// and key, that a graph swap keeps the ones which are still needed running even
// when the resources which need them are replaced, and that it stops the ones
// that aren't needed anymore before it starts any new ones.
func TestBackgroundKeys(t *testing.T) {
	ge, cleanup := newTestEngine(t)
	defer cleanup()

	newRes := func(name, key string, value int) *backgroundKeyedRes {
		res := &backgroundKeyedRes{
			BgKey: key,
			Value: value,
		}
		res.SetKind(backgroundKeyedKind)
		res.SetName(name)
		return res
	}
	newGraph := func(name string, resources ...*backgroundKeyedRes) *pgraph.Graph {
		g, err := pgraph.NewGraph(name)
		if err != nil {
			t.Fatalf("pgraph NewGraph: %v", err)
		}
		for _, res := range resources {
			g.AddVertex(res)
		}
		return g
	}

	_ = backgroundKeyedLog.take(0) // start with an empty log

	running := true
	defer func() {
		if !running {
			return
		}
		_ = ge.Pause() // errors if we failed while paused, that's okay
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()

	// Two resources share the "a" key, so there are only two backgrounds,
	// and both of them are running before any Watch starts.
	swapTestGraph(t, ge, newGraph("first",
		newRes("a1", "a", 1),
		newRes("a2", "a", 1),
		newRes("b", "b", 1),
	))
	checkBackgroundEvents(t, backgroundKeyedLog.take(5),
		[]string{"start a", "start b"},
		[]string{"watch a", "watch a", "watch b"},
	)

	// Replace a1, remove a2 and b, and add c. The "a" background keeps on
	// running, and the "b" one is stopped before the "c" one starts. Both
	// of those happen after the removed Watch functions have all returned.
	swapTestGraph(t, ge, newGraph("second",
		newRes("a1", "a", 2),
		newRes("c", "c", 1),
	))
	checkBackgroundEvents(t, backgroundKeyedLog.take(7),
		[]string{"unwatch a", "unwatch a", "unwatch b"},
		[]string{"stop b"},
		[]string{"start c"},
		[]string{"watch a", "watch c"},
	)

	// Shutting down stops every background, after every Watch is done.
	running = false
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}
	if err := ge.Shutdown(); err != nil {
		t.Fatalf("engine Shutdown: %v", err)
	}
	checkBackgroundEvents(t, backgroundKeyedLog.take(4),
		[]string{"unwatch a", "unwatch c"},
		[]string{"stop a", "stop c"},
	)
}

// TestBackgroundStopErrors checks that when background functions error as they
// get stopped, the rest of them get stopped anyway, and that we get all of the
// errors. It does this for a Shutdown, and for a graph swap. A swap which fails
// like this must not start anything new, and the engine must still be able to
// shut down cleanly after it.
func TestBackgroundStopErrors(t *testing.T) {
	for _, swap := range []bool{false, true} {
		name := "shutdown"
		if swap {
			name = "swap"
		}
		t.Run(name, func(t *testing.T) {
			ge, cleanup := newTestEngine(t)
			defer cleanup()
			ge.Cancel = func(error) {} // the errors are expected here

			first, err := pgraph.NewGraph("first")
			if err != nil {
				t.Fatalf("pgraph NewGraph: %v", err)
			}
			for _, key := range []string{"a", "b"} {
				res := &backgroundStopErrorRes{}
				res.BgKey = key
				res.SetKind(backgroundStopErrorKind)
				res.SetName(key)
				first.AddVertex(res)
			}
			second, err := pgraph.NewGraph("second")
			if err != nil {
				t.Fatalf("pgraph NewGraph: %v", err)
			}
			res := &backgroundKeyedRes{BgKey: "c"}
			res.SetKind(backgroundKeyedKind)
			res.SetName("c")
			second.AddVertex(res)

			_ = backgroundKeyedLog.take(0) // start with an empty log

			done := false
			defer func() {
				if done {
					return
				}
				// We failed partway, so try not to leak.
				_ = ge.Pause()    // errors if we're already paused
				_ = ge.Abort()    // errors if nothing is pending
				_ = ge.Shutdown() // errors if a background does
			}()

			swapTestGraph(t, ge, first)
			checkBackgroundEvents(t, backgroundKeyedLog.take(4),
				[]string{"start a", "start b"},
				[]string{"watch a", "watch b"},
			)
			if err := ge.Pause(); err != nil {
				t.Fatalf("engine Pause: %v", err)
			}

			// Neither the empty graph of a Shutdown, nor the second
			// graph, need the two backgrounds, so both get stopped,
			// and both of them error.
			if swap {
				if err := ge.Load(second); err != nil {
					t.Fatalf("engine Load: %v", err)
				}
				if err := ge.Validate(); err != nil {
					t.Fatalf("engine Validate: %v", err)
				}
				err = ge.Commit(context.Background())
			} else {
				err = ge.Shutdown()
			}
			if err == nil {
				t.Fatalf("expected the background stop errors")
			}
			for _, key := range []string{"a", "b"} {
				if s := "stop " + key + " failed"; !strings.Contains(err.Error(), s) {
					t.Errorf("missing the `%s` error in: %v", s, err)
				}
			}
			// All of this happens before that call returns, so we
			// don't wait. The "c" background must not have started.
			checkBackgroundEvents(t, backgroundKeyedLog.take(0),
				[]string{"unwatch a", "unwatch b"},
				[]string{"stop a", "stop b"},
			)

			if swap {
				// This is what the main loop in lib does when a
				// Commit fails. The new resource was added, but
				// it never ran, and it must still shut down.
				if err := ge.Abort(); err != nil {
					t.Fatalf("engine Abort: %v", err)
				}
				if err := ge.Shutdown(); err != nil {
					t.Fatalf("engine Shutdown: %v", err)
				}
				// It never ran, so there's no Watch to see.
				checkBackgroundEvents(t, backgroundKeyedLog.take(0))
			}
			done = true

			if n := len(ge.bgState); n != 0 {
				t.Errorf("expected no background functions, got %d", n)
			}
			if n := len(ge.state); n != 0 {
				t.Errorf("expected no resource states, got %d", n)
			}
			if n := len(ge.waits); n != 0 {
				t.Errorf("expected no resource wait groups, got %d", n)
			}
		})
	}
}

// TestBackgroundKeyLogging checks that a key is logged literally, even when it
// contains percent signs, and that it doesn't consume the message arguments.
func TestBackgroundKeyLogging(t *testing.T) {
	mutex := &sync.Mutex{}
	lines := []string{}
	obj := &Engine{
		Cancel:  func(error) {}, // the stop error is expected
		bgState: make(map[bgID]*bgState),
		Logf: func(format string, v ...interface{}) {
			mutex.Lock()
			defer mutex.Unlock()
			lines = append(lines, fmt.Sprintf(format, v...))
		},
	}
	key := "fe80::1%eth0"
	defer func() { _ = obj.StopBackground(backgroundStopErrorKind, key) }()
	if err := obj.StartBackground(context.Background(), backgroundStopErrorKind, key); err != nil {
		t.Fatalf("engine StartBackground: %v", err)
	}
	if err := obj.StopBackground(backgroundStopErrorKind, key); err == nil {
		t.Fatalf("expected the background stop error")
	}
	expected := fmt.Sprintf("background(%s, %q): stopped %s", backgroundStopErrorKind, key, key)
	mutex.Lock()
	defer mutex.Unlock()
	if !slices.Contains(lines, expected) {
		t.Errorf("expected log %q, got: %q", expected, lines)
	}
}
