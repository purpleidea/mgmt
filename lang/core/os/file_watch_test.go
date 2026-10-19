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

package coreos

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// fileWatchFunc is the part of the file watching functions that we test here.
type fileWatchFunc interface {
	Init(*interfaces.Init) error
	Stream(context.Context) error
	Call(context.Context, []types.Value) (types.Value, error)
	Done() error
}

func TestFileWatchBlockedSend(t *testing.T) {
	fns := map[string]func() fileWatchFunc{
		"file_exists": func() fileWatchFunc { return &FileExistsFunc{} },
		"readfile":    func() fileWatchFunc { return &ReadFileFunc{} },
	}
	for name, fn := range fns {
		for _, mode := range []string{"switch", "done"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				// The race we're checking for is random, so we
				// try it a few times.
				for i := 0; i < 10; i++ {
					testFileWatchBlockedSend(t, fn(), mode)
				}
			})
		}
	}
}

// testFileWatchBlockedSend holds the main Stream loop inside Event, while the
// proxy goroutine blocks sending it a file event. It then either switches the
// filename or calls Done, and Stream must not deadlock while it stops that
// watcher.
func testFileWatchBlockedSend(t *testing.T, f fileWatchFunc, mode string) {
	t.Helper()

	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("hello\n"), 0600); err != nil {
			t.Fatalf("could not create file: %v", err)
		}
	}

	called := make(chan struct{})
	release := make(chan struct{})
	once := &sync.Once{}
	init := &interfaces.Init{
		Event: func(ctx context.Context) error {
			first := false
			once.Do(func() { first = true })
			if !first {
				return nil // only block on the first one
			}
			close(called)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Logf: func(format string, v ...interface{}) {}, // noop
	}
	if err := f.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // unblocks everything if we failed
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- f.Stream(ctx)
	}()

	if _, err := f.Call(ctx, []types.Value{&types.StrValue{V: a}}); err != nil {
		t.Fatalf("func Call: %v", err)
	}
	select {
	case <-called: // Stream is now held inside Event
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the startup event")
	}

	// Get the proxy goroutine to block while sending us a file event.
	if err := os.WriteFile(a, []byte("changed\n"), 0600); err != nil {
		t.Fatalf("could not change file: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	callDone := make(chan struct{})
	if mode == "switch" {
		go func() {
			defer close(callDone)
			_, _ = f.Call(ctx, []types.Value{&types.StrValue{V: b}})
		}()
		time.Sleep(10 * time.Millisecond) // let it get queued
	} else {
		close(callDone)
		if err := f.Done(); err != nil {
			t.Fatalf("func Done: %v", err)
		}
	}
	close(release)

	select {
	case <-callDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out switching the filename")
	}
	if mode == "switch" {
		// The switch has been received, so it's safe to close now.
		if err := f.Done(); err != nil {
			t.Fatalf("func Done: %v", err)
		}
	}

	select {
	case err := <-streamErr:
		if err == nil {
			t.Fatalf("expected an error from Stream after Done")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("func Stream deadlocked")
	}
}
