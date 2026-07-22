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

package ast

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestValueToExprPreservesStructFieldOrder(t *testing.T) {
	fields := ""
	for i := 0; i < 16; i++ {
		fields += fmt.Sprintf("field%02d int; ", i)
	}
	typ := types.NewType("struct{" + fields + "}")
	value := types.NewStruct(typ)

	for i := 0; i < 100; i++ {
		expr, err := ValueToExpr(value)
		if err != nil {
			t.Fatal(err)
		}
		result, ok := expr.(*ExprStruct)
		if !ok {
			t.Fatalf("conversion returned %T", expr)
		}
		for index, field := range result.Fields {
			if expected := typ.Ord[index]; field.Name != expected {
				t.Fatalf("field %d is %q, expected %q", index, field.Name, expected)
			}
		}
	}
}
