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

package lang

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine/graph"
	_ "github.com/purpleidea/mgmt/engine/resources"
	_ "github.com/purpleidea/mgmt/lang/core"
)

// TestNestedIfSubgraphReadd exercises the re-add of a nested if-expression
// subgraph. The outer if's else branch *is* the inner if. Toggling the outer
// condition tears down and later re-adds the inner subgraph. If the inner
// ExprIfFunc keeps a stale `last` value across that removal, then when it is
// re-added with the same condition value it had before, it wrongly skips
// rebuilding its branch and never re-adds the edge to its output vertex. That
// leaves the output starved and the resource never updates. This reproduces the
// "skipped time segment" bug from a datetime.now() driven nested if, but does
// so deterministically by driving both conditions from files.
func TestNestedIfSubgraphReadd(t *testing.T) {
	tmpdir := t.TempDir()
	outerSrc := filepath.Join(tmpdir, "outer")
	innerSrc := filepath.Join(tmpdir, "inner")
	dst := filepath.Join(tmpdir, "output")

	// Both source files must exist before we start, since os.readfile reads
	// them reactively. The inner condition stays "no" for the whole test.
	if err := os.WriteFile(outerSrc, []byte("no"), 0600); err != nil {
		t.Fatalf("write outer source failed: %+v", err)
	}
	if err := os.WriteFile(innerSrc, []byte("no"), 0600); err != nil {
		t.Fatalf("write inner source failed: %+v", err)
	}

	code := fmt.Sprintf(`
import "os"

$outer_src = %q
$inner_src = %q
$dst = %q

$inner = if os.readfile($inner_src) == "yes" {
	"inner-yes"
} else {
	"inner-no"
}

$s = if os.readfile($outer_src) == "yes" {
	"outer"
} else {
	$inner
}

file $dst {
	state => "exists",
	content => $s,
}
`, outerSrc, innerSrc, dst)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lang, err := newTestLang(ctx, t, code)
	if err != nil {
		t.Fatalf("newTestLang failed: %+v", err)
	}
	defer lang.Cleanup()

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := lang.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("lang run failed: %+v", err)
		}
	}()
	defer cancel()

	convergerCtx, convergerCancel := context.WithCancel(context.Background())
	defer convergerCancel()
	coord := &converger.Coordinator{
		Timeout: 0,
		StateFns: converger.StateFns{
			"test": func(context.Context, bool) error { return nil },
		},
		Debug: testing.Verbose(),
		Logf: func(format string, v ...interface{}) {
			t.Logf("converger: "+format, v...)
		},
	}
	if err := coord.Init(); err != nil {
		t.Fatalf("converger init failed: %+v", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := coord.Run(convergerCtx, false); err != nil && err != context.Canceled {
			t.Errorf("converger run failed: %+v", err)
		}
	}()
	defer convergerCancel()

	ge := &graph.Engine{
		Program:   "testing",
		Hostname:  "localhost",
		Converger: coord,
		Prefix:    filepath.Join(tmpdir, "engine") + string(os.PathSeparator),
		Debug:     testing.Verbose(),
		Logf: func(format string, v ...interface{}) {
			t.Logf("engine: "+format, v...)
		},
	}
	if err := ge.Init(); err != nil {
		t.Fatalf("engine init failed: %+v", err)
	}
	defer func() {
		if err := ge.Pause(); err != nil {
			t.Logf("engine pause before shutdown skipped: %+v", err)
		}
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine shutdown failed: %+v", err)
		}
	}()

	stream := lang.Stream(ctx)

	// Initially: outer is "no", so we fall through to the inner if, which is
	// also "no". The inner subgraph is resident and its ExprIfFunc caches
	// last=false while it builds the "inner-no" branch.
	applyNextGraphWithContent(t, ge, stream, dst, "inner-no")

	// Flip outer to "yes": the outer if switches to its "outer" branch and
	// tears down the whole inner subgraph. The inner ExprIfFunc is removed
	// while still remembering last=false.
	if err := os.WriteFile(outerSrc, []byte("yes"), 0600); err != nil {
		t.Fatalf("write outer source failed: %+v", err)
	}
	applyNextGraphWithContent(t, ge, stream, dst, "outer")

	// Flip outer back to "no": the inner subgraph is re-added. Its condition
	// is still "no" (unchanged), so a stale last=false would match and the
	// inner if would skip rebuilding its branch, starving its output and
	// leaving the resource stuck on "outer". With the fix, Init resets last
	// so the branch is rebuilt and we recover "inner-no".
	if err := os.WriteFile(outerSrc, []byte("no"), 0600); err != nil {
		t.Fatalf("write outer source failed: %+v", err)
	}
	applyNextGraphWithContent(t, ge, stream, dst, "inner-no")
}
