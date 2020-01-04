// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestParseFuncs(t *testing.T) {
	testParseFuncsWithFixture(t, "base")
}

func testParseFuncsWithFixture(t *testing.T, fixture string) {
	pkg := &golangPackage{
		Name:    "testpkg",
		Exclude: []string{"ToLower"},
	}

	signatures, err := ioutil.ReadFile(fmt.Sprintf("fixtures/func_%s.txt", fixture))
	if err != nil {
		t.Fatalf("Fixtures (txt) unreadable!\n%v", err)
	}
	f, err := pkg.extractFuncs(string(signatures), false)
	if err != nil {
		t.Fatalf("Error while parsing functions: %v", err)
	}

	expected := &functions{}
	fixtures, err := ioutil.ReadFile(fmt.Sprintf("fixtures/func_%s.yaml", fixture))
	if err != nil {
		t.Fatalf("Fixtures (yaml) unreadable!\n%v", err)
	}
	err = yaml.UnmarshalStrict(fixtures, &expected)
	if err != nil {
		t.Fatalf("Fixtures (yaml) unreadable!\n%v", err)
	}

	if !reflect.DeepEqual(f, *expected) {
		t.Fatalf("Functions differ!\n%v\n%v", f, *expected)
	}
}
