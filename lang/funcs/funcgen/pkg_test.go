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
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/iancoleman/strcase"
)

const testpkgPath = "./fixtures/testpkg"

// normalizedFunction is a minimal normalized projection of *function for easier
// comparisons.
type normalizedFunction struct {
	GolangFunc   string   // GolangFunc
	InternalName string   // InternalName
	MclName      string   // MclName
	Variadic     bool     // Variadic
	Errorful     bool     // Errorful
	ArgTypes     []string // Args[i].Type
	ReturnTypes  []string // Return[i].Type
}

func normalize(f *function) normalizedFunction {
	n := normalizedFunction{
		GolangFunc:   f.GolangFunc,
		InternalName: f.InternalName,
		MclName:      f.MclName,
		Variadic:     f.Variadic,
		Errorful:     f.Errorful,
	}
	for _, a := range f.Args {
		n.ArgTypes = append(n.ArgTypes, a.Type)
	}
	for _, r := range f.Return {
		n.ReturnTypes = append(n.ReturnTypes, r.Type)
	}
	return n
}

func TestParseFuncs_WithRealFixturePackage(t *testing.T) {
	gp := &golangPackage{
		Name:      testpkgPath,
		Alias:     "",
		MgmtAlias: "",
		Exclude:   []string{"ToLower"}, // verify excludes are honored
	}

	funcs, err := gp.parsefuncs()
	if err != nil {
		t.Fatalf("parsefuncs failed: %v", err)
	}

	actual := make([]normalizedFunction, 0, len(funcs))
	for _, f := range funcs {
		actual = append(actual, normalize(f))
	}
	sortNorms(actual)

	// Build expected set. InternalName prefix must match generator logic:
	internalPrefix := strcase.ToCamel(strings.ReplaceAll(gp.Name, "/", ""))
	internalPrefix = strings.ReplaceAll(internalPrefix, "Html", "HTML")

	// Helper to construct expected entries.
	makeNormalized := func(goFunc string, variadic, errorful bool, argTypes []string, returnTypes []string) normalizedFunction {
		return normalizedFunction{
			GolangFunc:   goFunc,
			InternalName: internalPrefix + goFunc,
			MclName:      strcase.ToSnake(goFunc),
			Variadic:     variadic,
			Errorful:     errorful,
			ArgTypes:     append([]string(nil), argTypes...),
			ReturnTypes:  append([]string(nil), returnTypes...),
		}
	}

	// Expected from fixtures/testpkg (see that package for source):
	// Rejections to be absent:
	//   Lgamma                   -> (float64,int)       : reject (multi non-error results)
	//   Nextafter32              -> float32 params      : reject (float32 not in allowlist)
	//   ToLower                  -> excluded by config  : reject
	//   WithErrorButNothingElse  -> only error          : reject
	//   WithNothingElse          -> no results          : reject
	expected := []normalizedFunction{
		makeNormalized("AllKind", false, false, []string{"int64", "string"}, []string{"float64"}),
		makeNormalized("Join", true, false, []string{"[]string"}, []string{"string"}),            // variadic ...string
		makeNormalized("Max", false, false, []string{"float64", "float64"}, []string{"float64"}), // grouped params
		makeNormalized("SuperByte", false, false, []string{"[]byte", "string"}, []string{"[]byte"}),
		makeNormalized("ToUpper", false, false, []string{"string"}, []string{"string"}),
		makeNormalized("WithError", false, true, []string{"string"}, []string{"string"}), // (T,error)
		makeNormalized("WithInt", false, false, []string{"float64", "int", "int64", "int", "int", "bool", "string"}, []string{"string"}),
	}
	sortNorms(expected)

	// Sanity: ensure we really addressed the intended package (protects against shadowing).
	wantDir := filepath.Join("fixtures", "testpkg")
	wantAbs, err := filepath.Abs(wantDir)
	if err != nil {
		t.Fatal(err)
	}
	gotAbs, err := filepath.Abs(gp.Name)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if gotAbs != wantAbs {
		t.Fatalf("loaded wrong package: got %q, want %q", gotAbs, wantAbs)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("parsed functions differ.\nGot:\n%#v\n\nWant:\n%#v\n", actual, expected)
	}
}

// sortNorms simply sorts a normalizedFunction slice by name.
func sortNorms(ns []normalizedFunction) {
	sort.Slice(ns, func(i, j int) bool {
		if ns[i].GolangFunc != ns[j].GolangFunc {
			return ns[i].GolangFunc < ns[j].GolangFunc
		}
		return ns[i].InternalName < ns[j].InternalName
	})
}
