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
	"strings"
	"testing"
	"time"
)

// TestExceptUncaughtRead runs programs with a lookup which fails on every other
// event. The error is caught on one use of the value, but a statement also
// reads that same value directly, and nothing caught it there. That must be
// fatal, with the error of the lookup, instead of the statement reading a nil
// value from the table and panicking. Each program starts out with a real
// value, so that the error shows up in a table that replaces a working one.
func TestExceptUncaughtRead(t *testing.T) {
	testCases := []struct {
		name string
		code string
		err  string // the error must contain this
	}{
		{
			name: "res field",
			code: `
import "math"
import "test"

$m = {"a" => "x"}
$key = if math.mod(test.fastcount(), 2) == 0 { "a" } else { "missing" }
$s = $m[$key]

test [$s <|> "caught"] {
	stringptr => $s,
}
`,
			err: `map key not present, want: "missing" in {"a": "x"}: /main.mcl @ 7:6-7:14`,
		},
		{
			// The for loop reads the list directly, and the body
			// catches the error of each element of it.
			name: "for input",
			code: `
import "math"
import "test"

$m = {"a" => ["x", "y"]}
$key = if math.mod(test.fastcount(), 2) == 0 { "a" } else { "missing" }

for $i, $v in $m[$key] {
	test [$v <|> "caught"] {}
}
`,
			err: `map key not present, want: "missing" in {"a": ["x", "y"]}: /main.mcl @ 8:15-8:23`,
		},
		{
			// The forkv loop reads the map directly, and the body
			// catches the error of each key and value of it.
			name: "forkv input",
			code: `
import "fmt"
import "math"
import "test"

$m = {"a" => {"x" => 1, "y" => 2}}
$key = if math.mod(test.fastcount(), 2) == 0 { "a" } else { "missing" }

forkv $k, $v in $m[$key] {
	test [$k <|> "caught"] {
		stringptr => fmt.printf("%d", $v) <|> "caught",
	}
}
`,
			err: `map key not present, want: "missing" in {"a": {"x": 1, "y": 2}}: /main.mcl @ 9:17-9:25`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := runLangEvents(t, tc.code, 100, 10*time.Second)
			if err == nil {
				t.Fatalf("lang passed, expected an error")
			}
			if s := err.Error(); !strings.Contains(s, tc.err) {
				t.Errorf("unexpected error: %s", s)
				t.Logf("expected it to contain: %s", tc.err)
			}
		})
	}
}
