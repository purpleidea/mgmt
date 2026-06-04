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

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <file> [file...]\n", os.Args[0])
		os.Exit(2)
	}

	exitCode := 0
	for _, filename := range os.Args[1:] {
		if errs := check(filename); len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Error in %s: %v\n", filename, err)
			}
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func check(filename string) []error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return []error{err}
	}

	var errs []error
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}

		// Method receivers in Go can have only one parameter in the list.
		field := fn.Recv.List[0]
		if len(field.Names) == 0 {
			pos := fset.Position(field.Pos())
			errs = append(errs, fmt.Errorf("%d:%d: method receiver must be named 'obj', but it is unnamed", pos.Line, pos.Column))
			continue
		}

		name := field.Names[0].Name
		if name != "obj" {
			pos := fset.Position(field.Names[0].Pos())
			errs = append(errs, fmt.Errorf("%d:%d: method receiver must be named 'obj', but it is named '%s'", pos.Line, pos.Column, name))
		}
	}

	return errs
}
