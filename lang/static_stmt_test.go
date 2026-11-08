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

// TestStaticStmtInstances runs code where a static statement has more than one
// runtime instance (loop iterations or class includes) and where some of those
// instances get removed while the others stay. If the instances shared any
// graph state, then the remaining ones would point at funcs which are gone, and
// interpret would fail with a "missing value in table" error. The counter flips
// the condition on every event, so this would happen right away.
func TestStaticStmtInstances(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "if in for",
			code: `
import "math"
import "test"

$flip = math.mod(test.fastcount(), 2) == 0

for $i, $x in [1, 2] {
	if $x == 1 or $flip {
		test "static" {}
	}
}
`,
		},
		{
			name: "if in class",
			code: `
import "math"
import "test"

$flip = math.mod(test.fastcount(), 2) == 0

class c($b) {
	if $b {
		test "static" {}
	}
}

include c(true)
include c($flip)
`,
		},
		{
			name: "include in if",
			code: `
import "math"
import "test"

$flip = math.mod(test.fastcount(), 2) == 0

class c() {
	test "static" {}
}

include c()

if $flip {
	include c()
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
