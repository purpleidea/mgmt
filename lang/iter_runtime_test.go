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

// TestIterRuntimeShape runs the iter functions which build a subgraph at
// runtime inside a branch that keeps getting removed. When that branch goes
// away, the function and all of the element funcs it built must go away with
// it. If they didn't, the old element funcs would keep reading the list, and
// would fail as soon as it shrank, and with a constant list the function graph
// would grow on every flip until the engine could no longer keep up.
func TestIterRuntimeShape(t *testing.T) {
	// Each of these is inside a branch which gets removed when $c is odd.
	// The list it reads shrinks when $c is a multiple of five.
	header := `
import "iter"
import "math"
import "test"

$c = test.fastcount()
$a = math.mod($c, 2) == 0
$list = if math.mod($c, 5) == 0 { ["a"] } else { ["a", "b", "c"] }
`
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "map",
			code: `
if $a {
	test iter.map($list, func($x) { $x }) {}
}
`,
		},
		{
			name: "map through a bind",
			code: `
if $a {
	$l = iter.map($list, func($x) { $x })
	test $l {}
}
`,
		},
		{
			name: "filter",
			code: `
if $a {
	test iter.filter($list, func($x) { $x != "b" }) {}
}
`,
		},
		{
			name: "reduce",
			code: `
if $a {
	test [iter.reduce($list, func($x, $y) { $x + $y })] {}
}
`,
		},
		{
			name: "reduce with an initial value",
			code: `
if $a {
	test [iter.reduce("z", $list, func($x, $y) { $x + $y })] {}
}
`,
		},
		{
			name: "zip",
			code: `
if $a {
	test iter.zip($list, ["x", "y", "z"], func($x, $y) { $x + $y }) {}
}
`,
		},
		{
			name: "map with a constant list",
			code: `
if $a {
	test iter.map(["a", "b", "c"], func($x) { $x }) {}
} else {
	test "other" {}
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runLangEvents(t, header+tc.code, 100, 10*time.Second); err != nil {
				t.Errorf("lang errored: %+v", err)
			}
		})
	}
}

// TestIterInputErrorCaught makes sure that an error in the input list of one of
// the iter functions which build a subgraph at runtime can still get caught by
// the except operator, after that subgraph was built for an empty list. Such a
// subgraph has no element funcs, so whatever passes the list along to them has
// nowhere to send an error unless it gets sent on to the output. If it
// couldn't, it would be a sink, and an error which arrives at a sink is fatal.
func TestIterInputErrorCaught(t *testing.T) {
	// The list is empty when $c is even, and an error when it's odd.
	header := `
import "convert"
import "fmt"
import "iter"
import "math"
import "test"

$c = test.fastcount()
$list = if math.mod($c, 2) == 0 { [] } else { [convert.str_to_int("x")] }
`
	testCases := []struct {
		name string
		code string
	}{
		{
			name: "map",
			code: `
test [fmt.printf("%v", iter.map($list, func($x) { $x * 2 }) <|> [-1])] {}
`,
		},
		{
			name: "filter",
			code: `
test [fmt.printf("%v", iter.filter($list, func($x) { $x > 0 }) <|> [-1])] {}
`,
		},
		{
			name: "reduce with an initial value",
			code: `
test [fmt.printf("%v", iter.reduce(0, $list, func($x, $y) { $x + $y }) <|> -1)] {}
`,
		},
		{
			name: "zip",
			code: `
test [fmt.printf("%v", iter.zip([1], $list, func($x, $y) { $x + $y }) <|> [-1])] {}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runLangEvents(t, header+tc.code, 100, 10*time.Second); err != nil {
				t.Errorf("lang errored: %+v", err)
			}
		})
	}
}
