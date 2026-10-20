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

package recwatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestStartupPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks require an unprivileged user")
	}

	for _, name := range []string{"file", "parent"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "file.txt")
			if err := os.WriteFile(target, []byte("contents\n"), 0600); err != nil {
				t.Fatalf("could not create target file: %v", err)
			}

			blocked := target
			mode := os.FileMode(0600)
			if name == "parent" {
				blocked = dir
				mode = 0700
			}
			if err := os.Chmod(blocked, 0000); err != nil {
				t.Fatalf("could not remove permissions: %v", err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(blocked, mode); err != nil {
					t.Errorf("could not restore permissions: %v", err)
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			rw, err := NewRecWatcher(ctx, target, false)
			defer rw.Cleanup()
			if err == nil || !strings.Contains(err.Error(), "permission denied adding a watch") {
				t.Fatalf("expected startup permission error, got: %v", err)
			}
			select {
			case _, ok := <-rw.Events():
				if ok {
					t.Fatal("unexpected event after failed startup")
				}
			default:
				t.Fatal("events channel is still open after failed startup")
			}
			if err := rw.watcher.Add(target); err != fsnotify.ErrClosed {
				t.Fatalf("expected watcher to be closed, got: %v", err)
			}
		})
	}
}

func TestWatchRuntimeError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(target, []byte("contents\n"), 0600); err != nil {
		t.Fatalf("could not create target file: %v", err)
	}

	rw, err := NewRecWatcher(context.Background(), target, false)
	if err != nil {
		t.Fatalf("could not create watcher: %v", err)
	}
	defer rw.Cleanup()

	watchErr := fmt.Errorf("test watcher error")
	select {
	case rw.watcher.Errors <- watchErr:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out injecting watcher error")
	}

	select {
	case event := <-rw.Events():
		if event == nil || event.Error == nil || !strings.Contains(event.Error.Error(), watchErr.Error()) {
			t.Fatalf("expected runtime watcher error, got: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime watcher error")
	}
}

func TestShutdownClosesWithoutError(t *testing.T) {
	for _, name := range []string{"cancel", "cleanup"} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(target, []byte("contents\n"), 0600); err != nil {
				t.Fatalf("could not create target file: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rw, err := NewRecWatcher(ctx, target, false)
			if err != nil {
				t.Fatalf("could not create watcher: %v", err)
			}
			defer rw.Cleanup()

			// We keep reading the whole time, so that a shutdown
			// error would get delivered if one were ever sent.
			done := make(chan *Event)
			go func() {
				defer close(done)
				for event := range rw.Events() {
					done <- event
				}
			}()

			if name == "cancel" {
				cancel()
			} else {
				rw.Cleanup()
			}

			select {
			case event, ok := <-done:
				if ok {
					t.Fatalf("unexpected event on shutdown: %+v", event)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for events channel to close")
			}
		})
	}
}

func TestWatchDoesNotDescendPastLeaf(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	target := filepath.Join(parent, "file.txt")

	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatalf("could not create parent directory: %v", err)
	}
	if err := os.WriteFile(target, []byte("contents\n"), 0600); err != nil {
		t.Fatalf("could not create target file: %v", err)
	}

	logs := make(chan string, 16)
	ctx, cancel := context.WithCancel(context.Background())
	rw, err := NewRecWatcher(ctx, target, false, Debug(true), Logf(func(format string, v ...interface{}) {
		logs <- fmt.Sprintf(format, v...)
	}))
	if err != nil {
		cancel()
		t.Fatalf("could not create watcher: %v", err)
	}
	defer rw.Cleanup()
	defer cancel()

	events := rw.Events()
	waitForWatchPath(t, logs, target)

	// A parent event can arrive after the watcher is already at the leaf. This
	// used to move the watch index past the end of the target path.
	injectEvent(t, rw, fsnotify.Event{Name: parent, Op: fsnotify.Chmod})
	expectEvent(t, events)
	waitForWatchPath(t, logs, target)

	injectEvent(t, rw, fsnotify.Event{Name: target, Op: fsnotify.Chmod})
	expectEvent(t, events)
}

func injectEvent(t *testing.T, rw *RecWatcher, event fsnotify.Event) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		rw.watcher.Events <- event
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out injecting fsnotify event: %v", event)
	}
}

func waitForWatchPath(t *testing.T, logs <-chan string, path string) {
	t.Helper()

	expected := fmt.Sprintf("watching: %s", path)
	timer := time.After(2 * time.Second)

	for {
		select {
		case log := <-logs:
			if strings.Contains(log, expected) {
				return
			}
		case <-timer:
			t.Fatalf("timed out waiting for recwatch log: %s", expected)
		}
	}
}

func expectEvent(t *testing.T, events <-chan *Event) *Event {
	t.Helper()

	select {
	case event := <-events:
		if event == nil {
			// programming error
			t.Fatalf("unexpected nil recwatch event")
		}
		if event.Error != nil {
			t.Fatalf("unexpected watcher error: %v", event.Error)
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recwatch event")
	}
	return nil // unreachable
}

// expectNoEvent drains any events for a little while, and fails if it gets one.
func expectNoEvent(t *testing.T, events <-chan *Event) {
	t.Helper()

	select {
	case event := <-events:
		t.Fatalf("unexpected recwatch event: %+v", event)
	case <-time.After(200 * time.Millisecond):
	}
}

// drainEvents discards events until things have been quiet for a little while.
// A single file write can cause more than one event.
func drainEvents(t *testing.T, events <-chan *Event) {
	t.Helper()

	for {
		select {
		case event := <-events:
			if event == nil || event.Error != nil {
				t.Fatalf("unexpected recwatch event: %+v", event)
			}
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		path       string
		expandHome bool
		valid      bool
	}{
		{t.TempDir(), false, true},
		{"", false, false},
		{"relative/path", false, false},
		{"~/path", false, false},
		{"~user/path", false, false},
		{"~/path", true, true},
		{"~user", true, false},
		{"relative/path", true, false},
	}

	for _, test := range tests {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		rw, err := NewRecWatcher(ctx, test.path, false, ExpandHome(test.expandHome))
		if err == nil {
			rw.Cleanup()
		}
		cancel()

		if test.valid && err != nil {
			t.Errorf("path %q (expandHome: %t): unexpected error: %v", test.path, test.expandHome, err)
		}
		if !test.valid && (err == nil || !strings.Contains(err.Error(), "path must")) {
			t.Errorf("path %q (expandHome: %t): expected a path error, got: %v", test.path, test.expandHome, err)
		}
	}
}

// TestExpandHomeFollowsPasswd changes package variables, so it must not be run
// in parallel.
func TestExpandHomeFollowsPasswd(t *testing.T) {
	dir := t.TempDir()
	passwd := filepath.Join(dir, "passwd")
	homeA := filepath.Join(dir, "a")
	homeB := filepath.Join(dir, "b")
	for _, x := range []string{homeA, homeB} {
		if err := os.Mkdir(x, 0700); err != nil {
			t.Fatalf("could not create home directory: %v", err)
		}
	}
	touchPasswd := func() {
		t.Helper()
		if err := os.WriteFile(passwd, []byte(time.Now().String()), 0600); err != nil {
			t.Fatalf("could not write passwd file: %v", err)
		}
	}
	touchPasswd()

	mutex := &sync.Mutex{}
	home := "" // the user doesn't exist yet
	setHome := func(s string) {
		mutex.Lock()
		defer mutex.Unlock()
		home = s
	}

	origPasswdFile, origExpandHomeFunc := passwdFile, expandHomeFunc
	t.Cleanup(func() {
		passwdFile, expandHomeFunc = origPasswdFile, origExpandHomeFunc
	})
	passwdFile = passwd
	expandHomeFunc = func(p string) (string, error) {
		mutex.Lock()
		defer mutex.Unlock()
		if home == "" {
			return p, fmt.Errorf("no such user")
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~test/")), nil
	}

	writeFile := func(p string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(time.Now().String()), 0600); err != nil {
			t.Fatalf("could not write file: %v", err)
		}
	}

	rw, err := NewRecWatcher(context.Background(), "~test/file", false, ExpandHome(true))
	if err != nil {
		t.Fatalf("could not create watcher: %v", err)
	}
	defer rw.Cleanup()
	events := rw.Events()

	// The user doesn't exist, so nothing we do there is noticed.
	writeFile(filepath.Join(homeA, "file"))
	expectNoEvent(t, events)

	// The user appears.
	setHome(homeA)
	touchPasswd()
	if event := expectEvent(t, events); event.Body.Name != passwd {
		t.Fatalf("expected an event for the passwd file, got: %s", event.Body.Name)
	}
	drainEvents(t, events)
	writeFile(filepath.Join(homeA, "file"))
	expectEvent(t, events)
	drainEvents(t, events)

	// An unrelated passwd change is not an event for us.
	touchPasswd()
	expectNoEvent(t, events)

	// The home directory moves, and we follow it.
	setHome(homeB)
	touchPasswd()
	expectEvent(t, events)
	drainEvents(t, events)
	writeFile(filepath.Join(homeA, "file"))
	expectNoEvent(t, events)
	writeFile(filepath.Join(homeB, "file"))
	expectEvent(t, events)
	drainEvents(t, events)

	// The user is removed.
	setHome("")
	touchPasswd()
	expectEvent(t, events)
	drainEvents(t, events)
	writeFile(filepath.Join(homeB, "file"))
	expectNoEvent(t, events)

	// We shut down without an error.
	rw.Cleanup()
	select {
	case event, ok := <-events:
		if ok {
			t.Fatalf("unexpected event on shutdown: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for events channel to close")
	}
}

func TestMergeChannelsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// These never close, and we never read the first event, so the merge is
	// blocked sending it to us when we cancel.
	in1 := make(chan *Event, 1)
	in2 := make(chan *Event, 1)
	in1 <- &Event{}
	in2 <- &Event{}
	out := MergeChannels(ctx, in1, in2)
	time.Sleep(10 * time.Millisecond) // let it block
	cancel()

	timer := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return // closed
			}
			// we might win the race against cancel once or twice
		case <-timer:
			t.Fatal("timed out waiting for the merged channel to close")
		}
	}
}

func TestMergeChannelsErrorIsLast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in1 := make(chan *Event)
	in2 := make(chan *Event)
	out := MergeChannels(ctx, in1, in2)

	// The second input keeps on sending events, even after the first one
	// fails, but none of them may follow that error.
	go func() {
		for {
			select {
			case in2 <- &Event{}:
			case <-ctx.Done():
				return
			}
		}
	}()
	watchErr := fmt.Errorf("test watcher error")
	go func() {
		select {
		case in1 <- &Event{Error: watchErr}:
		case <-ctx.Done():
		}
	}()

	timer := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-out:
			if !ok {
				t.Fatal("merged channel closed before the error")
			}
			if event.Error == nil {
				continue // an event from the second input
			}
			if event.Error != watchErr {
				t.Fatalf("unexpected error: %v", event.Error)
			}
		case <-timer:
			t.Fatal("timed out waiting for the error")
		}
		break // we got the error
	}

	select {
	case event, ok := <-out:
		if ok {
			t.Fatalf("unexpected event after the error: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the merged channel to close")
	}
}
