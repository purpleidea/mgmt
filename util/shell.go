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
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	// EtcPasswdFile is the location of the /etc/passwd file.
	EtcPasswdFile = "/etc/passwd"

	// ErrUsernameNotFound means we couldn't find that username.
	ErrUsernameNotFound = Error("can't find username")
)

// UserShell returns shell of the user.
// TODO: return a well-known error if the user is simply not found.
func UserShell(ctx context.Context, username string) (string, error) {
	// TODO: use getpwnam_r ?

	// First see if we can use `getent` to get the passwd database.
	if shell, err := getentShell(ctx, username); err == nil {
		return shell, nil
	}

	// We can always look in /etc/passwd manually.
	shell, err := passwdShell(ctx, username)
	if err != nil {
		return "", err
	}
	return shell, nil
}

// getentShell gets a user shell by running the `getent` command.
func getentShell(ctx context.Context, username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("empty username")
	}
	getent, err := exec.LookPath("getent")
	if err != nil {
		return "", err
	}
	args := []string{
		"passwd",
		username,
	}
	cmd := exec.CommandContext(ctx, getent, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// XXX: return ErrUsernameNotFound where appropriate
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	fields := strings.SplitN(strings.TrimSuffix(string(output), "\n"), ":", 7)

	if len(fields) != 7 {
		return "", err
	}
	return fields[6], nil
}

// passwdShell gets a user shell by looking through the `/etc/passwd` file.
func passwdShell(ctx context.Context, username string) (string, error) {
	f, err := os.Open(EtcPasswdFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.SplitN(strings.TrimSuffix(s.Text(), "\n"), ":", 7)
		if len(fields) == 7 && fields[0] == username {
			return fields[6], nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}

	return "", ErrUsernameNotFound
}
