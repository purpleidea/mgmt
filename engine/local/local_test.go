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

package local

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestWrite(t *testing.T) {
	tmpdir := fmt.Sprintf("%s/", t.TempDir()) // gets cleaned up at end, new dir for each call
	key := "test1"
	value := 42
	if err := valueWrite(context.Background(), tmpdir, key, value); err != nil {
		t.Errorf("error: %+v", err)
		return
	}

	if val, err := valueRead(context.Background(), tmpdir, key); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if !reflect.DeepEqual(value, val) {
		t.Errorf("error: not equal: %+v != %+v", val, value)
		//return
	}

	if err := valueRemove(context.Background(), tmpdir, key); err != nil {
		t.Errorf("error: %+v", err)
		//return
	}
}

// TestValueSetList checks that changing a list value doesn't panic, which it
// did when we compared the values with ==.
func TestValueSetList(t *testing.T) {
	ctx := context.Background()
	value := &Value{}
	value.Init(&ValueInit{
		Prefix: t.TempDir(),
		Logf:   t.Logf,
	})
	for _, val := range [][]string{{"a"}, {"b"}} {
		if err := value.ValueSet(ctx, "key", val); err != nil {
			t.Fatalf("func ValueSet: %v", err)
		}
	}
	if val, err := value.ValueGet(ctx, "key"); err != nil {
		t.Fatalf("func ValueGet: %v", err)
	} else if !reflect.DeepEqual(val, []string{"b"}) {
		t.Fatalf("got: %#v, expected: %#v", val, []string{"b"})
	}
}

// TestValueSetWatcherNotReading checks that ValueSet doesn't wait for a watcher
// which isn't reading its events. It used to, while it held the mutex for every
// key, so one slow watcher stalled every other caller. If that watcher was then
// cancelled, its cleanup waited for that same mutex, which was a deadlock.
func TestValueSetWatcherNotReading(t *testing.T) {
	value := &Value{}
	value.Init(&ValueInit{
		Prefix: t.TempDir(),
		Logf: func(format string, v ...interface{}) {
			t.Logf("value: "+format, v...)
		},
	})

	// timeout runs fn, and fails if it doesn't return in time.
	timeout := func(name string, fn func() error) {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- fn() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: blocked", name)
		}
	}
	set := func(val interface{}) {
		t.Helper()
		timeout("func ValueSet", func() error {
			return value.ValueSet(context.Background(), "key", val)
		})
	}
	get := func() interface{} {
		t.Helper()
		var val interface{}
		timeout("func ValueGet", func() error {
			var err error
			val, err = value.ValueGet(context.Background(), "key")
			return err
		})
		return val
	}
	recv := func(ch chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("no watch event")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := value.ValueWatch(ctx, "key")
	if err != nil {
		t.Fatalf("func ValueWatch: %v", err)
	}

	// Nobody reads ch, so the watcher holds one event, and its buffer holds
	// the next one. The sets after that must coalesce, and not wait.
	for _, val := range []string{"a", "b", "c"} {
		set(val)
	}

	// An event is still pending, and after it we read the newest value.
	recv(ch)
	if val := get(); val != "c" {
		t.Fatalf("got: %v, expected: c", val)
	}

	// Once we've read what's pending, a new change still sends an event.
	for drained := false; !drained; {
		select {
		case <-ch:
		case <-time.After(100 * time.Millisecond):
			drained = true
		}
	}
	set("d")
	recv(ch)

	// Fill the watcher up again, and cancel it while it's stuck sending.
	// Its cleanup must not deadlock with the sets.
	set("e")
	set("f")
	cancel()
	set("g")
	if val := get(); val != "g" {
		t.Fatalf("got: %v, expected: g", val)
	}
}

func TestHTTPPool(t *testing.T) {
	ctx := context.Background()
	pool := &HTTPPool{}
	pool.Init(&HTTPPoolInit{
		Logf: func(format string, v ...interface{}) { t.Logf("pool: "+format, v...) },
	})

	// An unpublished uid reads back as an empty (zero-value) response. This
	// is what lets a function like http.response("bar") report a 0 status
	// code when only an unrelated http:client "foo" resource exists in the
	// graph.
	if resp, err := pool.HTTPGet(ctx, "bar"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if resp == nil || resp.Status != 0 || resp.Path != "" {
		t.Errorf("expected empty response for unknown uid, got: %+v", resp)
		return
	}

	// Watch "foo" before anything is published. We should get one startup
	// event, and then one event for each subsequent change.
	ch, err := pool.HTTPWatch(ctx, "foo")
	if err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv := func(want bool) {
		t.Helper()
		select {
		case _, ok := <-ch:
			if !ok {
				t.Errorf("watch channel closed unexpectedly")
				return
			}
			if !want {
				t.Errorf("got an unexpected watch event")
			}
		case <-time.After(time.Second):
			if want {
				t.Errorf("timeout waiting for watch event")
			}
		}
	}

	recv(true) // startup event

	// Publish a status and path for "foo" and confirm we can read it back.
	if err := pool.HTTPSet(ctx, "foo", 200, "/tmp/foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv(true) // change event
	if resp, err := pool.HTTPGet(ctx, "foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if resp == nil || resp.Status != 200 || resp.Path != "/tmp/foo" {
		t.Errorf("unexpected response: %+v", resp)
		return
	}

	// "bar" must still be unknown: publishing "foo" doesn't leak across.
	if resp, err := pool.HTTPGet(ctx, "bar"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if resp == nil || resp.Status != 0 || resp.Path != "" {
		t.Errorf("expected empty response for unknown uid, got: %+v", resp)
		return
	}

	// Setting the same value again is a no-op that must not notify.
	if err := pool.HTTPSet(ctx, "foo", 200, "/tmp/foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv(false) // no event expected
}

func TestBridge(t *testing.T) {
	ctx := context.Background()
	bridge := &BridgeImpl{}
	bridge.Init(&BridgeInit{
		Logf: func(format string, v ...interface{}) { t.Logf("bridge: "+format, v...) },
	})

	// An unpublished entry reads back as nil, which is the "nothing has
	// been published yet" state that consumers turn into a zero value.
	if val, err := bridge.BridgeGet(ctx, "ns", "foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if val != nil {
		t.Errorf("expected nil for unknown entry, got: %+v", val)
		return
	}

	// An empty namespace or uid must error.
	if err := bridge.BridgeSet(ctx, "", "foo", 42); err == nil {
		t.Errorf("expected error for empty namespace")
		return
	}
	if err := bridge.BridgeSet(ctx, "ns", "", 42); err == nil {
		t.Errorf("expected error for empty uid")
		return
	}
	if err := bridge.BridgeSet(ctx, "n/s", "foo", 42); err == nil {
		t.Errorf("expected error for namespace with slash")
		return
	}

	// Watch before anything is published. We should get one startup event,
	// and then one event for each subsequent change.
	ch, err := bridge.BridgeWatch(ctx, "ns", "foo")
	if err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv := func(want bool) {
		t.Helper()
		select {
		case _, ok := <-ch:
			if !ok {
				t.Errorf("watch channel closed unexpectedly")
				return
			}
			if !want {
				t.Errorf("got an unexpected watch event")
			}
		case <-time.After(time.Second):
			if want {
				t.Errorf("timeout waiting for watch event")
			}
		}
	}

	recv(true) // startup event

	if err := bridge.BridgeSet(ctx, "ns", "foo", 42); err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv(true) // change event
	if val, err := bridge.BridgeGet(ctx, "ns", "foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if !reflect.DeepEqual(val, 42) {
		t.Errorf("unexpected value: %+v", val)
		return
	}

	// The same uid in a different namespace must not collide. This matters
	// because the uid is commonly a user-controlled resource name.
	if val, err := bridge.BridgeGet(ctx, "other", "foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if val != nil {
		t.Errorf("namespaces leaked, got: %+v", val)
		return
	}

	// Setting the same value again is a no-op that must not notify.
	if err := bridge.BridgeSet(ctx, "ns", "foo", 42); err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv(false) // no event expected

	// Unpublishing notifies, and reads back as nil again.
	if err := bridge.BridgeSet(ctx, "ns", "foo", nil); err != nil {
		t.Errorf("error: %+v", err)
		return
	}
	recv(true) // change event
	if val, err := bridge.BridgeGet(ctx, "ns", "foo"); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if val != nil {
		t.Errorf("expected nil after unpublish, got: %+v", val)
		return
	}
}
