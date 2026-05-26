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

package integration

import (
	"context"
	"errors"
	"os"
	"path"
	"testing"
	"time"
)

func TestInstanceWaitForConvergedAfterActivity(t *testing.T) {
	statusFile := path.Join(t.TempDir(), ConvergerStatusFile)
	if err := os.WriteFile(statusFile, []byte("true\n"), fileMode); err != nil {
		t.Fatalf("could not write status file: %+v", err)
	}

	instance := &Instance{
		Hostname:            "h1",
		convergerStatusFile: statusFile,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err := instance.WaitForConvergedAfterActivity(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected wait result: %+v", err)
	}

	if instance.convergerStatusIndex != 1 {
		t.Fatalf("unexpected status index: %d", instance.convergerStatusIndex)
	}

	if err := os.WriteFile(statusFile, []byte("true\nfalse\ntrue\n"), fileMode); err != nil {
		t.Fatalf("could not write status file: %+v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := instance.WaitForConvergedAfterActivity(ctx); err != nil {
		t.Fatalf("unexpected wait error: %+v", err)
	}
}
