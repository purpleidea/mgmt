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

package astfmt

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// TestFormat formats an AST that was built programmatically, without any
// position information at all. Everything must fall back to the canonical
// single-line or multi-line layout. The much more complete tests which format
// real source code with positions and comments live in the format package.
func TestFormat(t *testing.T) {
	prog := &ast.StmtProg{
		Body: []interfaces.Stmt{
			&ast.StmtImport{
				Name:  "fmt",
				Alias: "printf",
			},
			&ast.StmtBind{
				Ident: "x",
				Value: &ast.ExprList{
					Elements: []interfaces.Expr{
						&ast.ExprInt{V: 1},
						&ast.ExprStr{V: "hello"},
						&ast.ExprBool{V: true},
					},
				},
			},
			&ast.StmtRes{
				Kind: "test",
				Name: &ast.ExprStr{V: "hello"},
				Contents: []ast.StmtResContents{
					&ast.StmtResField{
						Field: "anotherstr",
						Value: &ast.ExprStr{V: "world"},
					},
					&ast.StmtResMeta{
						Property: "noop",
						MetaExpr: &ast.ExprBool{V: true},
					},
				},
			},
		},
	}

	output, err := Format(context.TODO(), prog, nil)
	if err != nil {
		t.Fatalf("func Format failed: %+v", err)
	}

	expected := `import "fmt" as printf
$x = [1, "hello", true]
test "hello" {
	anotherstr => "world",
	Meta:noop => true,
}
`
	if string(output) != expected {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestFormatNilInput(t *testing.T) {
	if _, err := Format(context.TODO(), nil, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFormatUnsupportedStmt(t *testing.T) {
	prog := &ast.StmtProg{
		Body: []interfaces.Stmt{
			&ast.StmtComment{
				Value: " not from the parser",
			},
		},
	}

	output, err := Format(context.TODO(), prog, nil)
	if err != nil {
		t.Fatalf("func Format failed: %+v", err)
	}
	if string(output) != "# not from the parser\n" {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestTypeString(t *testing.T) {
	tests := []struct {
		name string
		typ  string // input for types.NewType
		want string
	}{
		{name: "bool", typ: "bool", want: "bool"},
		{name: "str", typ: "str", want: "str"},
		{name: "int", typ: "int", want: "int"},
		{name: "float", typ: "float", want: "float"},
		{name: "list", typ: "[]str", want: "[]str"},
		{name: "map", typ: "map{str: int}", want: "map{str: int}"},
		{name: "struct", typ: "struct{a bool; bb int}", want: "struct{a bool; bb int}"},
		{name: "empty struct", typ: "struct{}", want: "struct{}"},
		{name: "func", typ: "func(a str, b int) float", want: "func($a str, $b int) float"},
		{name: "func unnamed", typ: "func(str, int) float", want: "func(str, int) float"},
		{name: "func mixed", typ: "func(str, b int) float", want: "func(str, $b int) float"},
		{name: "variant", typ: "variant", want: "variant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := types.NewType(tt.typ)
			if typ == nil {
				t.Fatalf("could not build type: %s", tt.typ)
			}
			out, err := typeString(typ)
			if err != nil {
				t.Fatalf("func typeString failed: %+v", err)
			}
			if out != tt.want {
				t.Fatalf("unexpected type: got %s, expected %s", out, tt.want)
			}
		})
	}
}

func TestFloatString(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{in: 0, want: "0.0"},
		{in: 1.5, want: "1.5"},
		{in: -13.42, want: "-13.42"},
		{in: 1e6, want: "1000000.0"},
	}

	for _, tt := range tests {
		out, err := floatString(tt.in)
		if err != nil {
			t.Fatalf("func floatString failed: %+v", err)
		}
		if out != tt.want {
			t.Fatalf("unexpected float: got %s, expected %s", out, tt.want)
		}
	}
}
