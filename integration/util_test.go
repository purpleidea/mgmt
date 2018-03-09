// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

func TestCodeIndent(t *testing.T) {
	c1 := Code(
		`
	$root = getenv("MGMT_TEST_ROOT")

	file "${root}/mgmt-hello-world" {
		content => "hello world from @purpleidea\n",
		state => "exists",
	}
	`)
	c2 :=
		`$root = getenv("MGMT_TEST_ROOT")

file "${root}/mgmt-hello-world" {
	content => "hello world from @purpleidea\n",
	state => "exists",
}
`
	if c1 != c2 {
		t.Errorf("code samples differ")
	}
}
