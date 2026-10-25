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

package types

import "testing"

// TestHashableKinds checks that the values which can be hashed are exactly the
// ones of the kinds which isHashableKind says are hashable.
func TestHashableKinds(t *testing.T) {
	for _, typ := range []*Type{
		TypeNil,
		TypeBool,
		TypeStr,
		TypeInt,
		TypeFloat,
		NewType("[]str"),
		NewType("map{str: int}"),
		NewType("struct{a int}"),
		NewType("func(a int) str"),
		TypeVariant,
	} {
		if _, ok := typ.New().(Hashable); ok != isHashableKind(typ.Kind) {
			t.Errorf("%s: value is hashable: %t, kind is hashable: %t", typ, ok, !ok)
		}
	}
}

func TestIsHashableType(t *testing.T) {
	for s, want := range map[string]bool{
		"str":                       true,
		"float":                     true,
		"[]str":                     false,
		"map{str: int}":             false,
		"func() str":                false,
		"variant":                   false,
		"struct{a int; b str}":      true,
		"struct{a int; b []str}":    false,
		"struct{a struct{b bool}}":  true,
		"struct{a struct{b []int}}": false,
	} {
		if got := IsHashableType(NewType(s)); got != want {
			t.Errorf("%s: got %t, want %t", s, got, want)
		}
	}
}
