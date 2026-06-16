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

package core

import (
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
)

// safeString runs the String method on a function, recovering from any panic.
// It returns the resulting string and whether the call succeeded. A fresh,
// unbuilt function might legitimately panic in String, so we skip those rather
// than fail the common-case comparison we care about here.
func safeString(fn interfaces.Func) (s string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	return fn.String(), true
}

// TestStringConsistency ensures that two separately-created copies of the same
// registered function produce the same String output. Every function is stored
// as a graph vertex, and two functions that are really the same (even if they
// are distinct copies with different pointer addresses) must compare equal via
// String. This would fail if, for example, a function included a `%p` (the
// pointer address) in its String output. We can't easily test every function in
// its fully built state here, so we at least cover the common case of a freshly
// created function.
func TestStringConsistency(t *testing.T) {
	m := funcs.Map()
	if len(m) == 0 {
		t.Fatal("no functions are registered")
	}

	for name, fn := range m {
		t.Run(name, func(t *testing.T) {
			// Two separate copies, each with its own memory address.
			f1 := fn()
			f2 := fn()

			s1, ok1 := safeString(f1)
			s2, ok2 := safeString(f2)
			if !ok1 || !ok2 {
				t.Logf("skipping %s: String panics on a fresh func", name)
				return
			}

			if s1 != s2 {
				t.Errorf("func `%s` has inconsistent String output between two copies: `%s` != `%s`", name, s1, s2)
			}
		})
	}
}
