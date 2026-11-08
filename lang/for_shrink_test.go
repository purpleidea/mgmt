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

// TestForInputChange runs for and forkv loops over an input which changes its
// shape on every event. If the funcs which pull each element out of the input
// ran before the loop func had a chance to rebuild its body, then they'd get an
// input which doesn't have their element anymore, and they would fail. The
// topological sort is in random order, so this would happen pretty quickly.
func TestForInputChange(t *testing.T) {
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "for list shrink",
			code: `
import "math"
import "test"

$list = if math.mod(test.fastcount(), 2) == 0 {
	["a", "b", "c"]
} else {
	["a"]
}

for $i, $x in $list {
	test [$x] {}
}
`,
		},
		{
			name: "forkv map shrink",
			code: `
import "fmt"
import "math"
import "test"

$map = if math.mod(test.fastcount(), 2) == 0 {
	{"a" => 1, "b" => 2, "c" => 3}
} else {
	{"a" => 1}
}

forkv $k, $v in $map {
	test [$k] {
		stringptr => fmt.printf("%d", $v),
	}
}
`,
		},
		{
			name: "forkv map keys change",
			code: `
import "fmt"
import "math"
import "test"

$map = if math.mod(test.fastcount(), 2) == 0 {
	{"a" => 1, "b" => 2}
} else {
	{"a" => 1, "c" => 3}
}

forkv $k, $v in $map {
	test [$k] {
		stringptr => fmt.printf("%d", $v),
	}
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
