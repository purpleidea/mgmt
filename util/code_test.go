// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package util

import (
	"testing"
)

func TestCodeIndent(t *testing.T) {
	c1 := Code(
		`
	$root = getenv("MGMT_TEST_ROOT")

	file "${root}/mgmt-hello-world" {
		content => "hello world from @purpleidea\n",
		state => $const.res.file.state.exists,
	}
	`)
	c2 :=
		`$root = getenv("MGMT_TEST_ROOT")

file "${root}/mgmt-hello-world" {
	content => "hello world from @purpleidea\n",
	state => $const.res.file.state.exists,
}
`
	if c1 != c2 {
		t.Errorf("code samples differ")
	}
}
