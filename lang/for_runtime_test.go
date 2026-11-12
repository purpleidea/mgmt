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

package lang

import (
	"testing"
	"time"
)

// TestForRuntimeShape runs for and forkv loops whose input changes shape, and
// loops which sit inside a branch that keeps getting removed. The element funcs
// of a loop must run after the loop func, so that when the list shrinks (or the
// keys change) the loop func can rebuild before an element func which is now
// out of range gets to run. And when the branch which contains a loop is
// removed, the loop func and all of its element funcs must go away with it. If
// they didn't, the old element funcs would keep reading the list, and would
// fail as soon as it shrank, and with a constant list the function graph would
// grow on every flip until the engine could no longer keep up.
func TestForRuntimeShape(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "for list shrinks",
			code: `
import "math"
import "test"

$list = if math.mod(test.fastcount(), 2) == 0 { ["a"] } else { ["a", "b", "c"] }

for $i, $x in $list {
	test [$x] {}
}
`,
		},
		{
			name: "forkv keys change",
			code: `
import "math"
import "test"

$m = if math.mod(test.fastcount(), 2) == 0 { {"a" => 1} } else { {"a" => 1, "b" => 2, "c" => 3} }

forkv $k, $v in $m {
	test [$k] {}
}
`,
		},
		{
			name: "for in removed branch",
			code: `
import "math"
import "test"

$c = test.fastcount()
$list = if math.mod($c, 5) == 0 { ["a"] } else { ["a", "b", "c"] }

if math.mod($c, 2) == 0 {
	for $i, $x in $list {
		test [$x] {}
	}
}
`,
		},
		{
			name: "forkv in removed branch",
			code: `
import "math"
import "test"

$c = test.fastcount()
$m = if math.mod($c, 5) == 0 { {"a" => 1} } else { {"a" => 1, "b" => 2, "c" => 3} }

if math.mod($c, 2) == 0 {
	forkv $k, $v in $m {
		test [$k] {}
	}
}
`,
		},
		{
			name: "for with constant list in removed branch",
			code: `
import "math"
import "test"

if math.mod(test.fastcount(), 2) == 0 {
	for $i, $x in ["a", "b", "c"] {
		test [$x] {}
	}
} else {
	test "other" {}
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runLangEvents(t, tc.code, 100, 10*time.Second); err != nil {
				t.Errorf("lang errored: %+v", err)
			}
		})
	}
}
