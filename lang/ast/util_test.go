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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"

	"github.com/spf13/afero"
)

// newTestFs returns an empty filesystem with the given URI. The contents don't
// matter here, we only need each one to be a distinct filesystem.
func newTestFs(t *testing.T, scheme, path string) engine.Fs {
	t.Helper()
	return &util.AferoFs{
		Scheme: scheme,
		Path:   path,
		Afero:  &afero.Afero{Fs: afero.NewMemMapFs()},
	}
}

func TestCollectPrograms(t *testing.T) {
	fs := newTestFs(t, "", "")

	main := &StmtProg{}
	if err := main.Init(&interfaces.Data{
		Fs:       fs,
		Base:     "/tmp/main/",
		Metadata: &interfaces.Metadata{Main: "main.mcl"},
	}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	imported := &StmtProg{}
	if err := imported.Init(&interfaces.Data{
		Fs:       fs,
		Base:     "/tmp/imported/",
		Metadata: &interfaces.Metadata{Main: "main.mcl"},
	}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	main.importProgs = []*StmtProg{
		imported,
		imported,
	}

	programs, err := CollectPrograms(main)
	if err != nil {
		t.Fatalf("func CollectPrograms failed: %+v", err)
	}
	if len(programs) != 2 {
		t.Fatalf("unexpected program count: %d", len(programs))
	}
	if programs[0].File.Path != "/tmp/main/main.mcl" {
		t.Fatalf("unexpected main path: %s", programs[0].File.Path)
	}
	if programs[1].File.Path != "/tmp/imported/main.mcl" {
		t.Fatalf("unexpected imported path: %s", programs[1].File.Path)
	}
	for i, program := range programs {
		if program.File.FS != fs {
			t.Errorf("program %d lost its filesystem", i)
		}
	}
}

// TestCollectProgramsDistinctFs makes sure that two files which have the same
// path, but which live in two different filesystems, are both collected. This
// happens when we import more than one embedded module, since each of those has
// its own root, and therefore its own /main.mcl file.
func TestCollectProgramsDistinctFs(t *testing.T) {
	metadata := &interfaces.Metadata{Main: "main.mcl"}

	main := &StmtProg{}
	if err := main.Init(&interfaces.Data{
		Fs:       newTestFs(t, "", ""),
		Base:     "/tmp/main/",
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	for _, name := range []string{"one", "two"} {
		imported := &StmtProg{}
		if err := imported.Init(&interfaces.Data{
			Fs:       newTestFs(t, "embeddedfs", "/"+name),
			Base:     "/", // each embedded module has its own root
			Metadata: metadata,
		}); err != nil {
			t.Fatalf("func Init failed: %+v", err)
		}
		main.importProgs = append(main.importProgs, imported)
	}

	programs, err := CollectPrograms(main)
	if err != nil {
		t.Fatalf("func CollectPrograms failed: %+v", err)
	}
	if len(programs) != 3 {
		t.Fatalf("unexpected program count: %d", len(programs))
	}

	expected := []string{
		"MemMapFS:///tmp/main/main.mcl",
		"embeddedfs:///one/main.mcl",
		"embeddedfs:///two/main.mcl",
	}
	for i, program := range programs {
		if uri := program.File.URI(); uri != expected[i] {
			t.Errorf("program %d is %s, expected %s", i, uri, expected[i])
		}
	}
}

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
