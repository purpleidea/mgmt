// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"testing"
)

func TestExecSendRecv1(t *testing.T) {
	r1 := &ExecRes{
		BaseRes: BaseRes{
			Name: "exec1",
			Kind: "exec",
			//MetaParams: MetaParams,
		},
		Cmd:   "echo hello world",
		Shell: "/bin/bash",
	}

	r1.Setup(nil, r1, r1)
	defer func() {
		if err := r1.Close(); err != nil {
			t.Errorf("close failed with: %v", err)
		}
	}()
	if err := r1.Init(); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	t.Logf("output is: %v", r1.Output)
	if r1.Output != nil {
		t.Logf("output is: %v", *r1.Output)
	}
	t.Logf("stdout is: %v", r1.Stdout)
	if r1.Stdout != nil {
		t.Logf("stdout is: %v", *r1.Stdout)
	}
	t.Logf("stderr is: %v", r1.Stderr)
	if r1.Stderr != nil {
		t.Logf("stderr is: %v", *r1.Stderr)
	}

	if r1.Stdout == nil {
		t.Errorf("stdout is nil")
	} else {
		if out := *r1.Stdout; out != "hello world\n" {
			t.Errorf("got wrong stdout(%d): %s", len(out), out)
		}
	}
}

func TestExecSendRecv2(t *testing.T) {
	r1 := &ExecRes{
		BaseRes: BaseRes{
			Name: "exec1",
			Kind: "exec",
			//MetaParams: MetaParams,
		},
		Cmd:   "echo hello world 1>&2", // to stderr
		Shell: "/bin/bash",
	}

	r1.Setup(nil, r1, r1)
	defer func() {
		if err := r1.Close(); err != nil {
			t.Errorf("close failed with: %v", err)
		}
	}()
	if err := r1.Init(); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	t.Logf("output is: %v", r1.Output)
	if r1.Output != nil {
		t.Logf("output is: %v", *r1.Output)
	}
	t.Logf("stdout is: %v", r1.Stdout)
	if r1.Stdout != nil {
		t.Logf("stdout is: %v", *r1.Stdout)
	}
	t.Logf("stderr is: %v", r1.Stderr)
	if r1.Stderr != nil {
		t.Logf("stderr is: %v", *r1.Stderr)
	}

	if r1.Stderr == nil {
		t.Errorf("stderr is nil")
	} else {
		if out := *r1.Stderr; out != "hello world\n" {
			t.Errorf("got wrong stderr(%d): %s", len(out), out)
		}
	}
}

func TestExecSendRecv3(t *testing.T) {
	r1 := &ExecRes{
		BaseRes: BaseRes{
			Name: "exec1",
			Kind: "exec",
			//MetaParams: MetaParams,
		},
		Cmd:   "echo hello world && echo goodbye world 1>&2", // to stdout && stderr
		Shell: "/bin/bash",
	}

	r1.Setup(nil, r1, r1)
	defer func() {
		if err := r1.Close(); err != nil {
			t.Errorf("close failed with: %v", err)
		}
	}()
	if err := r1.Init(); err != nil {
		t.Errorf("init failed with: %v", err)
	}
	// run artificially without the entire engine
	if _, err := r1.CheckApply(true); err != nil {
		t.Errorf("checkapply failed with: %v", err)
	}

	t.Logf("output is: %v", r1.Output)
	if r1.Output != nil {
		t.Logf("output is: %v", *r1.Output)
	}
	t.Logf("stdout is: %v", r1.Stdout)
	if r1.Stdout != nil {
		t.Logf("stdout is: %v", *r1.Stdout)
	}
	t.Logf("stderr is: %v", r1.Stderr)
	if r1.Stderr != nil {
		t.Logf("stderr is: %v", *r1.Stderr)
	}

	if r1.Output == nil {
		t.Errorf("output is nil")
	} else {
		// it looks like bash or golang race to the write, so whichever
		// order they come out in is ok, as long as they come out whole
		if out := *r1.Output; out != "hello world\ngoodbye world\n" && out != "goodbye world\nhello world\n" {
			t.Errorf("got wrong output(%d): %s", len(out), out)
		}
	}

	if r1.Stdout == nil {
		t.Errorf("stdout is nil")
	} else {
		if out := *r1.Stdout; out != "hello world\n" {
			t.Errorf("got wrong stdout(%d): %s", len(out), out)
		}
	}

	if r1.Stderr == nil {
		t.Errorf("stderr is nil")
	} else {
		if out := *r1.Stderr; out != "goodbye world\n" {
			t.Errorf("got wrong stderr(%d): %s", len(out), out)
		}
	}
}
