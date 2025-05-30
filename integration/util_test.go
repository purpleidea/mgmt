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
	"os"
	"testing"
)

func TestBinaryPath(t *testing.T) {
	p, err := BinaryPath()
	if err != nil {
		t.Errorf("could not determine binary path: %+v", err)
		return
	}
	if p == "" {
		t.Errorf("binary path was empty")
		return
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Errorf("could not stat binary path: %+v", err)
		return
	}
	// TODO: check file mode is executable
	_ = fi
}

func TestParsePort(t *testing.T) {
	if port, err := ParsePort("http://127.0.0.1:2379"); err != nil {
		t.Errorf("could not determine port: %+v", err)
	} else if port != 2379 {
		t.Errorf("unexpected port: %d", port)
	}

	if port, err := ParsePort("http://127.0.0.1:2381"); err != nil {
		t.Errorf("could not determine port: %+v", err)
	} else if port != 2381 {
		t.Errorf("unexpected port: %d", port)
	}

	if port, err := ParsePort("http://127.0.0.1"); err == nil {
		t.Errorf("expected error, got: %d", port)
	}
}
