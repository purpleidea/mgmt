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

package ast

import (
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
)

// samePosition reports whether these two textareas cover the same position.
func samePosition(a, b interfaces.Textarea) bool {
	al, ac := a.Pos()
	bl, bc := b.Pos()
	ael, aec := a.End()
	bel, bec := b.End()
	return al == bl && ac == bc && ael == bel && aec == bec
}

func TestExprMapKeepsEntryPositions(t *testing.T) {
	kv := &ExprMapKV{
		Key: &ExprVar{Name: "k"},
		Val: &ExprVar{Name: "v"},
	}
	kv.Locate(1, 2, 3, 4)
	obj := &ExprMap{KVs: []*ExprMapKV{kv}}

	interpolated, err := obj.Interpolate()
	if err != nil {
		t.Fatalf("func Interpolate failed: %+v", err)
	}
	if x := interpolated.(*ExprMap).KVs[0]; !samePosition(x.Textarea, kv.Textarea) {
		t.Errorf("func Interpolate lost the position of the map entry")
	}

	copied, err := obj.Copy()
	if err != nil {
		t.Fatalf("func Copy failed: %+v", err)
	}
	if copied == obj {
		t.Fatalf("func Copy did not copy the map")
	}
	if x := copied.(*ExprMap).KVs[0]; !samePosition(x.Textarea, kv.Textarea) {
		t.Errorf("func Copy lost the position of the map entry")
	}
}

func TestExprStructKeepsFieldPositions(t *testing.T) {
	field := &ExprStructField{
		Name:  "f",
		Value: &ExprVar{Name: "v"},
	}
	field.Locate(1, 2, 3, 4)
	obj := &ExprStruct{Fields: []*ExprStructField{field}}

	interpolated, err := obj.Interpolate()
	if err != nil {
		t.Fatalf("func Interpolate failed: %+v", err)
	}
	if x := interpolated.(*ExprStruct).Fields[0]; !samePosition(x.Textarea, field.Textarea) {
		t.Errorf("func Interpolate lost the position of the struct field")
	}

	copied, err := obj.Copy()
	if err != nil {
		t.Fatalf("func Copy failed: %+v", err)
	}
	if copied == obj {
		t.Fatalf("func Copy did not copy the struct")
	}
	if x := copied.(*ExprStruct).Fields[0]; !samePosition(x.Textarea, field.Textarea) {
		t.Errorf("func Copy lost the position of the struct field")
	}
}
