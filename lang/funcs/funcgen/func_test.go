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
	"os"
	"reflect"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestRenderFuncs(t *testing.T) {
	testRenderFuncsWithFixture(t, "base")
}

func testRenderFuncsWithFixture(t *testing.T, fixture string) {
	pkg := &golangPackage{
		Name:    "testpkg",
		Exclude: []string{"ToLower"},
	}

	funcs := &functions{}
	fixtures, err := os.ReadFile(fmt.Sprintf("fixtures/func_%s.yaml", fixture))
	if err != nil {
		t.Fatalf("Fixtures (yaml) unreadable!\n%v", err)
	}
	err = yaml.UnmarshalStrict(fixtures, &funcs)
	if err != nil {
		t.Fatalf("Fixtures (yaml) unreadable!\n%v", err)
	}

	golangFixtures, err := os.ReadFile(fmt.Sprintf("fixtures/func_%s.tpl", fixture))
	if err != nil {
		t.Fatalf("Fixtures (tpl) unreadable!\n%v", err)
	}

	c := config{
		Packages: []*golangPackage{pkg},
	}

	dstFileName := fmt.Sprintf("func_%s.result", fixture)
	err = generateTemplate(c, *funcs, "fixtures", "templates/generated_funcs.go.tpl", dstFileName)
	if err != nil {
		t.Fatalf("Not generating template!\n%v", err)
	}
	result, err := os.ReadFile(fmt.Sprintf("fixtures/%s", dstFileName))
	if err != nil {
		t.Fatalf("Result unreadable!\n%v", err)
	}

	if !reflect.DeepEqual(golangFixtures, result) {
		t.Fatalf("Functions differ!\n1>\n%v\n2>\n%v", string(golangFixtures), string(result))
	}
}
