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

//go:build linux && !root

package lang

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// countInotifyFds returns the number of inotify file descriptors currently open
// in this process. On Linux an inotify fd shows up in /proc/self/fd as a
// symlink to "anon_inode:inotify".
func countInotifyFds(t *testing.T) int {
	t.Helper()
	const fdDir = "/proc/self/fd"
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		t.Fatalf("could not read %s: %v", fdDir, err)
	}
	count := 0
	for _, entry := range entries {
		// The fd may have been closed between ReadDir and Readlink, so
		// we ignore errors from individual entries.
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(target, "inotify") {
			count++
		}
	}
	return count
}

// TestReadFileWatcherNoInotifyLeak reproduces the scenario behind
// https://github.com/purpleidea/mgmt/issues/982. Each deploy spins up a fresh
// language runtime, and file-watching functions such as os.readfile hold an
// inotify watch for as long as their runtime is alive. If a replaced runtime is
// not shut down, that watch leaks an inotify fd on every deploy, eventually
// exhausting the process. This test stands up and tears down such a runtime
// many times, shutting each one down the way GAPI.Cleanup does (cancel the
// context, then wait for it to fully exit), and asserts that the inotify fd
// count returns to its baseline every time.
func TestReadFileWatcherNoInotifyLeak(t *testing.T) {
	tmpdir := t.TempDir()
	src := filepath.Join(tmpdir, "input")
	if err := os.WriteFile(src, []byte("hello"), 0600); err != nil {
		t.Fatalf("write source failed: %+v", err)
	}
	dst := filepath.Join(tmpdir, "output")

	// os.readfile watches $src, so evaluating this program opens an inotify
	// fd for the life of the runtime. The result feeds a resource so that
	// it is part of the graph and actually gets evaluated.
	code := fmt.Sprintf(`
import "os"

$src = %q
$dst = %q

file $dst {
	state => "exists",
	content => os.readfile($src) <|> "default",
}
`, src, dst)

	// runOnce brings up a language runtime, waits until os.readfile has
	// established its inotify watch (so we know this cycle actually opened
	// an fd), then shuts the runtime down and waits for it to fully exit.
	runOnce := func(t *testing.T) {
		localBase := countInotifyFds(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		lang, err := newTestLang(ctx, t, code)
		if err != nil {
			t.Fatalf("newTestLang failed: %+v", err)
		}

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lang.Run(ctx); err != nil && err != context.Canceled {
				t.Errorf("lang run failed: %+v", err)
			}
		}()

		// Drain the graph stream so the function engine keeps running.
		// This goroutine exits on its own when Run closes the stream.
		stream := lang.Stream(ctx)
		go func() {
			for range stream { //nolint:revive
			}
		}()

		// Wait for the readfile watch to appear, otherwise the test
		// would prove nothing about watcher lifetime.
		deadline := time.After(30 * time.Second)
		for countInotifyFds(t) <= localBase {
			select {
			case <-deadline:
				t.Fatalf("timed out waiting for the readfile inotify watch to appear")
			case <-time.After(5 * time.Millisecond):
			}
		}

		cancel()  // shut the runtime down, the way GAPI.Cleanup does...
		wg.Wait() // ...and wait for it to fully exit
		if err := lang.Cleanup(); err != nil {
			t.Errorf("lang cleanup failed: %+v", err)
		}
	}

	// Warm up once so any one-time watchers are already established, then
	// measure the baseline we expect to return to after each cycle.
	runOnce(t)
	base := countInotifyFds(t)

	const iterations = 20
	for i := 0; i < iterations; i++ {
		runOnce(t)
	}

	// If teardown leaked, we'd be sitting at roughly base+iterations here.
	if got := countInotifyFds(t); got > base {
		t.Errorf("inotify fd leak: %d open at baseline, %d after %d runtime cycles", base, got, iterations)
	}
}
