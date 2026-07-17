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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/etcd"
	etcdClient "github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/util"

	"github.com/spf13/afero"
)

func TestEnginePauseAfterCheckApplyFailure(t *testing.T) {
	ograph, err := runInterpret(t, `test "fail" {
		waitforerror => 100,
	}`+"\n")
	if err != nil {
		t.Fatalf("could not interpret mcl: %+v", err)
	}

	tmpdir := t.TempDir()
	localAPI := (&local.API{
		Prefix: fmt.Sprintf("%s/", filepath.Join(tmpdir, "local")),
		Logf:   func(format string, v ...interface{}) {},
	}).Init()

	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs}
	fs := &util.AferoFs{Afero: afs}
	world := &etcd.World{
		Client:       etcdClient.NewClientFromClient(nil),
		StandaloneFs: fs,
	}
	if err := world.Connect(context.Background(), &engine.WorldInit{
		Logf: func(format string, v ...interface{}) {},
	}); err != nil {
		t.Fatalf("could not connect world: %+v", err)
	}
	defer func() {
		if err := world.Cleanup(); err != nil {
			t.Errorf("could not cleanup world: %+v", err)
		}
	}()

	coordinator := &converger.Coordinator{
		Timeout: -1,
		Logf:    func(format string, v ...interface{}) {},
	}
	if err := coordinator.Init(); err != nil {
		t.Fatalf("could not initialize converger: %+v", err)
	}
	coordinatorCtx, coordinatorCancel := context.WithCancel(context.Background())
	coordinatorDone := make(chan error, 1)
	go func() {
		coordinatorDone <- coordinator.Run(coordinatorCtx, false)
	}()
	defer func() {
		coordinatorCancel()
		if err := <-coordinatorDone; err != nil && err != context.Canceled {
			t.Errorf("converger failed: %+v", err)
		}
	}()

	checkApplyStarted := make(chan struct{})
	var checkApplyStartedOnce sync.Once
	ge := &graph.Engine{
		Program:   "mgmt",
		Version:   "test",
		Hostname:  "localhost",
		Converger: coordinator,
		Local:     localAPI,
		World:     world,
		Prefix:    fmt.Sprintf("%s/", filepath.Join(tmpdir, "engine")),
		Debug:     true,
		Logf: func(format string, v ...interface{}) {
			if strings.Contains(format, "CheckApply(%t)") {
				checkApplyStartedOnce.Do(func() { close(checkApplyStarted) })
			}
		},
	}
	if err := ge.Init(); err != nil {
		t.Fatalf("could not initialize engine: %+v", err)
	}
	engineShutdown := false
	defer func() {
		if engineShutdown {
			return
		}
		if err := ge.Shutdown(); err != nil {
			t.Errorf("could not cleanup engine: %+v", err)
		}
	}()

	if err := ge.Load(ograph); err != nil {
		t.Fatalf("could not load graph: %+v", err)
	}
	if err := ge.Validate(); err != nil {
		t.Fatalf("could not validate graph: %+v", err)
	}
	if err := ge.Pause(false); err != nil {
		t.Fatalf("could not pause empty graph: %+v", err)
	}
	if err := ge.Commit(context.Background()); err != nil {
		t.Fatalf("could not commit graph: %+v", err)
	}
	if err := ge.Resume(); err != nil {
		t.Fatalf("could not resume graph: %+v", err)
	}

	select {
	case <-checkApplyStarted:
	case <-time.After(10 * time.Second):
		t.Fatalf("resource did not start CheckApply")
	}

	if err := ge.Pause(false); err != nil {
		t.Fatalf("could not pause failing graph: %+v", err)
	}
	if err := ge.Resume(); err != nil {
		t.Fatalf("could not resume failed graph: %+v", err)
	}

	// This is the same final pause that Main.Run performs after ^C. Capture a
	// panic so that the rest of the engine can still be drained before the
	// test reports the regression.
	var pauseErr error
	var pausePanic interface{}
	func() {
		defer func() {
			pausePanic = recover()
		}()
		pauseErr = ge.Pause(false)
	}()
	if pausePanic != nil {
		err := ge.Shutdown()
		engineShutdown = true
		if err != nil {
			t.Errorf("could not shutdown engine after panic: %+v", err)
		}
		t.Fatalf("pause panicked during shutdown: %v", pausePanic)
	}
	if pauseErr != nil {
		t.Fatalf("could not pause failed graph for shutdown: %+v", pauseErr)
	}
	err = ge.Shutdown()
	engineShutdown = true
	if err != nil {
		t.Fatalf("could not shutdown engine: %+v", err)
	}
}
