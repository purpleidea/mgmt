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

package util

import (
	"context"
	"io"
	"os/exec"
	"syscall"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// RunCmd runs the named command with the given arguments, capturing stderr so
// that any failure message from the command is included in the returned error.
// The command runs in its own process group so that it isn't affected by
// signals sent to the parent's process group.
func RunCmd(ctx context.Context, cmdName string, args []string) error {
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// open a pipe to get error messages from os/exec
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errwrap.Wrapf(err, "failed to initialize stderr pipe")
	}

	// start the command
	if err := cmd.Start(); err != nil {
		return errwrap.Wrapf(err, "cmd failed to start")
	}
	// capture any error messages
	b, err := io.ReadAll(stderr)
	if err != nil {
		return errwrap.Wrapf(err, "error reading stderr")
	}
	// wait until cmd exits and return error message if any
	if err := cmd.Wait(); err != nil {
		return errwrap.Wrapf(err, "%s", b)
	}
	return nil
}
