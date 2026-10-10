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

func expectEvent(t *testing.T, events <-chan *Event) {
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
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recwatch event")
	}
}
