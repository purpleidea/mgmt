// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package util

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"syscall"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// SimpleCmdOpts is a list of extra things to pass into the SimpleCmd function.
type SimpleCmdOpts struct {
	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})

	// LogOutput is the path to where we can append the stdout and stderr.
	LogOutput string
}

// SimpleCmd is a simple wrapper for us to run commands how we usually want to.
func SimpleCmd(ctx context.Context, name string, args []string, opts *SimpleCmdOpts) error {
	logf := func(format string, v ...interface{}) {
		if opts == nil {
			return
		}
		opts.Logf(format, v...)
	}

	cmd := exec.CommandContext(ctx, name, args...)

	// ignore signals sent to parent process (we're in our own group)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// Capture stdout and stderr together. Same as CombinedOutput() method.
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b

	logf("running: %s", strings.Join(cmd.Args, " "))
	if err := cmd.Start(); err != nil {
		return errwrap.Wrapf(err, "error starting cmd")
	}

	err := cmd.Wait() // we can unblock this with the timeout
	out := b.String()

	if opts != nil && opts.LogOutput != "" {
		if err := AppendFile(opts.LogOutput, b.Bytes(), 0600); err != nil {
			logf("unable to store log: %v", err)
		} else {
			logf("wrote log to: %s", opts.LogOutput)
		}
	}

	if err == nil {
		logf("command ran successfully!")
		return nil // success!
	}

	exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
	if !ok {
		// command failed in some bad way
		return errwrap.Wrapf(err, "cmd failed in some bad way")
	}
	pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
	wStatus, ok := pStateSys.(syscall.WaitStatus)
	if !ok {
		return errwrap.Wrapf(err, "could not get exit status of cmd")
	}
	exitStatus := wStatus.ExitStatus()
	if exitStatus == 0 {
		// i'm not sure if this could happen
		return errwrap.Wrapf(err, "unexpected cmd exit status of zero")
	}

	logf("cmd: %s", strings.Join(cmd.Args, " "))
	if out == "" {
		logf("cmd exit status %d", exitStatus)
	} else {
		logf("cmd error:\n%s", out) // newline because it's long
	}
	return errwrap.Wrapf(err, "cmd error") // exit status will be in the error
}
