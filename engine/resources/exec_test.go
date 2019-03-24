// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// +build !root

package resources

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
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
	r1 := &ExecRes{
		Cmd:   "echo hello world",
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Close(); err != nil {
			t.Errorf("close failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(true); err != nil {
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
	r1 := &ExecRes{
		Cmd:   "echo hello world 1>&2", // to stderr
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Close(); err != nil {
			t.Errorf("close failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(true); err != nil {
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
	r1 := &ExecRes{
		Cmd:   "echo hello world && echo goodbye world 1>&2", // to stdout && stderr
		Shell: "/bin/bash",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
	defer func() {
		if err := r1.Close(); err != nil {
			t.Errorf("close failed with: %v", err)
		}
	}()
	init, execSends := fakeExecInit(t)
	if err := r1.Init(init); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(true); err != nil {
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

func TestExecTimeoutBehaviour(t *testing.T) {
	// cmd.Process.Kill() is called on timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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
