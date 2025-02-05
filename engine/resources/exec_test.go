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

package resources

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autoedge"
	"github.com/purpleidea/mgmt/pgraph"
)

func fakeExecInit(t *testing.T) (*engine.Init, *ExecSends) {
	debug := testing.Verbose() // set via the -test.v flag to `go test`
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	execSends := &ExecSends{}
	return &engine.Init{
		Send: func(st interface{}) error {
			x, ok := st.(*ExecSends)
			if !ok {
				return fmt.Errorf("unable to send")
			}
			*execSends = *x // set
			return nil
		},
		Debug: debug,
		Logf:  logf,
	}, execSends
}

func TestExecSendRecv1(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	r1 := &ExecRes{
		Cmd:   "echo hello world",
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Cleanup(); err != nil {
			t.Errorf("cleanup failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(ctx, true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	t.Logf("output is: %v", execSends.Output)
	if execSends.Output != nil {
		t.Logf("output is: %v", *execSends.Output)
	}
	t.Logf("stdout is: %v", execSends.Stdout)
	if execSends.Stdout != nil {
		t.Logf("stdout is: %v", *execSends.Stdout)
	}
	t.Logf("stderr is: %v", execSends.Stderr)
	if execSends.Stderr != nil {
		t.Logf("stderr is: %v", *execSends.Stderr)
	}

	if execSends.Stdout == nil {
		t.Errorf("stdout is nil")
	} else {
		if out := *execSends.Stdout; out != "hello world\n" {
			t.Errorf("got wrong stdout(%d): %s", len(out), out)
		}
	}
}

func TestExecSendRecv2(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	r1 := &ExecRes{
		Cmd:   "echo hello world 1>&2", // to stderr
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Cleanup(); err != nil {
			t.Errorf("cleanup failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(ctx, true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	t.Logf("output is: %v", execSends.Output)
	if execSends.Output != nil {
		t.Logf("output is: %v", *execSends.Output)
	}
	t.Logf("stdout is: %v", execSends.Stdout)
	if execSends.Stdout != nil {
		t.Logf("stdout is: %v", *execSends.Stdout)
	}
	t.Logf("stderr is: %v", execSends.Stderr)
	if execSends.Stderr != nil {
		t.Logf("stderr is: %v", *execSends.Stderr)
	}

	if execSends.Stderr == nil {
		t.Errorf("stderr is nil")
	} else {
		if out := *execSends.Stderr; out != "hello world\n" {
			t.Errorf("got wrong stderr(%d): %s", len(out), out)
		}
	}
}

func TestExecSendRecv3(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	r1 := &ExecRes{
		Cmd:   "echo hello world && echo goodbye world 1>&2", // to stdout && stderr
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Cleanup(); err != nil {
			t.Errorf("cleanup failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(ctx, true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	t.Logf("output is: %v", execSends.Output)
	if execSends.Output != nil {
		t.Logf("output is: %v", *execSends.Output)
	}
	t.Logf("stdout is: %v", execSends.Stdout)
	if execSends.Stdout != nil {
		t.Logf("stdout is: %v", *execSends.Stdout)
	}
	t.Logf("stderr is: %v", execSends.Stderr)
	if execSends.Stderr != nil {
		t.Logf("stderr is: %v", *execSends.Stderr)
	}

	if execSends.Output == nil {
		t.Errorf("output is nil")
	} else {
		// it looks like bash or golang race to the write, so whichever
		// order they come out in is ok, as long as they come out whole
		if out := *execSends.Output; out != "hello world\ngoodbye world\n" && out != "goodbye world\nhello world\n" {
			t.Errorf("got wrong output(%d): %s", len(out), out)
		}
	}

	if execSends.Stdout == nil {
		t.Errorf("stdout is nil")
	} else {
		if out := *execSends.Stdout; out != "hello world\n" {
			t.Errorf("got wrong stdout(%d): %s", len(out), out)
		}
	}

	if execSends.Stderr == nil {
		t.Errorf("stderr is nil")
	} else {
		if out := *execSends.Stderr; out != "goodbye world\n" {
			t.Errorf("got wrong stderr(%d): %s", len(out), out)
		}
	}
}

func TestExecEnv_Empty(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	r1 := &ExecRes{
		Cmd:   "env",
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Cleanup(); err != nil {
			t.Errorf("cleanup failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(ctx, true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	if execSends.Stdout == nil {
		t.Errorf("stdout is nil")
		return
	}
	for _, v := range strings.Split(*execSends.Stdout, "\n") {
		if v == "" {
			continue
		}
		s := strings.SplitN(v, "=", 2)
		if s[0] == "_" || s[0] == "PWD" || s[0] == "SHLVL" {
			// these variables are set by bash and are expected
			continue
		}
		t.Errorf("executed process had an unexpected env variable: %v", s[0])
	}
}

func TestExecEnv_SetByResource(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	r1 := &ExecRes{
		Cmd:   "env",
		Shell: "/bin/bash",
		Env: map[string]string{
			"PURPLE":               "idea",
			"CONTAINS_UNDERSCORES": "and=equal=signs",
		},
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Cleanup(); err != nil {
			t.Errorf("cleanup failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(ctx, true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	if execSends.Stdout == nil {
		t.Errorf("stdout is nil")
		return
	}
	for _, v := range strings.Split(*execSends.Stdout, "\n") {
		if v == "" {
			continue
		}
		s := strings.SplitN(v, "=", 2)
		if s[0] == "_" || s[0] == "PWD" || s[0] == "SHLVL" {
			// these variables are set by bash and are expected
		} else if s[0] == "PURPLE" {
			if s[1] != "idea" {
				t.Errorf("executed process had an unexpected value for env variable: %v", v)
			}
		} else if s[0] == "CONTAINS_UNDERSCORES" {
			if s[1] != "and=equal=signs" {
				t.Errorf("executed process had an unexpected value for env variable: %v", v)
			}
		} else {
			t.Errorf("executed process had an unexpected env variable: %v", s[0])
		}
	}
}

func TestExecTimeoutBehaviour(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	// cmd.Process.Kill() is called on timeout
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmdName := "/bin/sleep"    // it's /usr/bin/sleep on modern distros
	cmdArgs := []string{"300"} // 5 min in seconds
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	// ignore signals sent to parent process (we're in our own group)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		t.Errorf("error starting cmd: %+v", err)
		return
	}

	err := cmd.Wait() // we can unblock this with the timeout

	if err == nil {
		t.Errorf("expected error, got nil")
		return
	}

	exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
	if err != nil && ok {
		pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
		wStatus, ok := pStateSys.(syscall.WaitStatus)
		if !ok {
			t.Errorf("error running cmd")
			return
		}
		if !wStatus.Signaled() {
			t.Errorf("did not get signal, exit status: %d", wStatus.ExitStatus())
			return
		}

		// we get this on timeout, because ctx calls cmd.Process.Kill()
		if sig := wStatus.Signal(); sig != syscall.SIGKILL {
			t.Errorf("got wrong signal: %+v, exit status: %d", sig, wStatus.ExitStatus())
			return
		}

		t.Logf("exit status: %d", wStatus.ExitStatus())
		return

	} else if err != nil {
		t.Errorf("general cmd error")
		return
	}

	// no error
}

func TestExecAutoEdge1(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resUser, err := engine.NewNamedResource("user", "someuser")
	if err != nil {
		t.Errorf("error creating user resource: %v", err)
		return
	}

	resGroup, err := engine.NewNamedResource("group", "somegroup")
	if err != nil {
		t.Errorf("error creating group resource: %v", err)
		return
	}

	resFile, err := engine.NewNamedResource("file", "/somefile")
	if err != nil {
		t.Errorf("error creating group resource: %v", err)
		return
	}

	resExec, err := engine.NewNamedResource("exec", "somefile")
	if err != nil {
		t.Errorf("error creating exec resource: %v", err)
		return
	}
	exc := resExec.(*ExecRes)
	exc.Cmd = resFile.Name()
	exc.User = resUser.Name()
	exc.Group = resGroup.Name()

	g.AddVertex(resUser, resGroup, resFile, resExec)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose() // set via the -test.v flag to `go test`
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	expected, err := pgraph.NewGraph("Expected")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	expectEdge := func(from, to pgraph.Vertex) {
		edge := &engine.Edge{Name: fmt.Sprintf("%s -> %s (expected)", from, to)}
		expected.AddEdge(from, to, edge)
	}
	expectEdge(resFile, resExec)
	expectEdge(resUser, resExec)
	expectEdge(resGroup, resExec)

	vertexCmp := func(v1, v2 pgraph.Vertex) (bool, error) { return v1 == v2, nil } // pointer compare is sufficient
	edgeCmp := func(e1, e2 pgraph.Edge) (bool, error) { return true, nil }         // we don't care about edges here

	if err := expected.GraphCmp(g, vertexCmp, edgeCmp); err != nil {
		t.Errorf("graph doesn't match expected: %s", err)
		return
	}
}
