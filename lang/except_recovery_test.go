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
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine/graph"
	"github.com/purpleidea/mgmt/engine/graph/autogroup"
	_ "github.com/purpleidea/mgmt/engine/resources"
	_ "github.com/purpleidea/mgmt/lang/core"
	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/spf13/afero"
)

func TestExceptReadFileRecoveryUpdatesResource(t *testing.T) {
	tmpdir := t.TempDir()
	src := filepath.Join(tmpdir, "input")
	dst := filepath.Join(tmpdir, "output")

	code := fmt.Sprintf(`
import "os"

$src = %q
$dst = %q

file $dst {
	state => "exists",
	content => os.readfile($src) <|> "default",
}
`, src, dst)

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
	converger := &converger.Coordinator{
		Timeout: 0,
		StateFns: converger.StateFns{
			"test": func(context.Context, bool) error { return nil },
		},
		Debug: testing.Verbose(),
		Logf: func(format string, v ...interface{}) {
			t.Logf("converger: "+format, v...)
		},
	}
	if err := converger.Init(); err != nil {
		t.Fatalf("converger init failed: %+v", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := converger.Run(convergerCtx, false); err != nil && err != context.Canceled {
			t.Errorf("converger run failed: %+v", err)
		}
	}()
	defer convergerCancel()

	ge := &graph.Engine{
		Program:   "testing",
		Hostname:  "localhost",
		Converger: converger,
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
	applyNextGraphWithContent(t, ge, stream, dst, "default")

	if err := os.WriteFile(src, []byte("recovered"), 0600); err != nil {
		t.Fatalf("write source failed: %+v", err)
	}
	applyNextGraphWithContent(t, ge, stream, dst, "recovered")

	if err := os.Remove(src); err != nil {
		t.Fatalf("remove source failed: %+v", err)
	}
	applyNextGraphWithContent(t, ge, stream, dst, "default")
}

func newTestLang(ctx context.Context, t *testing.T, code string) (*Lang, error) {
	t.Helper()

	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs}
	fs := &util.AferoFs{Afero: afs}

	output, err := inputs.ParseInput(code, fs)
	if err != nil {
		return nil, errwrap.Wrapf(err, "ParseInput failed")
	}
	for _, fn := range output.Workers {
		if err := fn(fs); err != nil {
			return nil, errwrap.Wrapf(err, "worker execution failed")
		}
	}

	obj := &Lang{
		Fs:    fs,
		Input: "/" + interfaces.MetadataFilename,
		Data: &Data{
			UnificationStrategy: make(map[string]string),
		},
		Debug: testing.Verbose(),
		Logf: func(format string, v ...interface{}) {
			t.Logf("lang: "+format, v...)
		},
	}
	if err := obj.Init(ctx); err != nil {
		return nil, errwrap.Wrapf(err, "lang init failed")
	}
	return obj, nil
}

func applyNextGraphWithContent(t *testing.T, ge *graph.Engine, stream <-chan *pgraph.Graph, path string, content string) {
	t.Helper()

	timeout := time.After(10 * time.Second)
	for {
		select {
		case g, ok := <-stream:
			if !ok {
				t.Fatalf("lang stream closed")
			}
			applyGraph(t, ge, g)
			if waitFileContent(path, content, time.Second) {
				return
			}

		case <-timeout:
			t.Fatalf("timed out waiting for %s to contain %q", path, content)
		}
	}
}

func applyGraph(t *testing.T, ge *graph.Engine, g *pgraph.Graph) {
	t.Helper()

	if err := ge.Load(g); err != nil {
		t.Fatalf("engine load failed: %+v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("engine validate failed: %+v", err)
	}
	if err := ge.AutoGroup(context.TODO(), &autogroup.CachedNonReachabilityGrouper{}); err != nil {
		t.Fatalf("engine autogroup failed: %+v", err)
	}
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine pause failed: %+v", err)
	}
	if err := ge.Commit(context.Background()); err != nil {
		t.Fatalf("engine commit failed: %+v", err)
	}
	if err := ge.Resume(); err != nil {
		t.Fatalf("engine resume failed: %+v", err)
	}
}

func waitFileContent(path string, content string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		data, err := os.ReadFile(path)
		if err == nil && string(data) == content {
			return true
		}

		select {
		case <-deadline:
			return false
		case <-ticker.C:
		}
	}
}
