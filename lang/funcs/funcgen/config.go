// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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
	"math/bits"

	"github.com/purpleidea/mgmt/lang/types"
)

type config struct {
	Packages golangPackages `yaml:"packages"`
}

type functions []function

type arg struct {
	// Name is the name of the argument.
	Name string `yaml:"name,omitempty"`
	// Value is the value of the argument.
	Value string `yaml:"value,omitempty"`
	// Type is the type of the argument.
	// Supported: bool, string, int, int64, float64, []byte.
	Type string `yaml:"type"`
}

// GolangType prints the golang equivalent of a mcl type.
func (obj *arg) GolangType() string {
	t := obj.Type
	if t == "float" {
		return "float64"
	}
	return t
}

// ToMcl prints the arg signature as expected by mcl.
func (obj *arg) ToMcl() (string, error) {
	var prefix string
	if obj.Name != "" {
		prefix = fmt.Sprintf("%s ", obj.Name)
	}
	switch obj.Type {
	case "bool":
		return fmt.Sprintf("%s%s", prefix, types.TypeBool.String()), nil
	case "string", "[]byte":
		return fmt.Sprintf("%s%s", prefix, types.TypeStr.String()), nil
	case "int", "int64":
		return fmt.Sprintf("%s%s", prefix, types.TypeInt.String()), nil
	case "float64":
		return fmt.Sprintf("%s%s", prefix, types.TypeFloat.String()), nil
	case "[]string":
		return fmt.Sprintf("%s%s", prefix, types.NewType("[]str").String()), nil
	default:
		return "", fmt.Errorf("cannot convert %v to mcl", obj.Type)
	}
}

// OldToGolang prints the arg signature as expected by golang. This is only used
// for returns.
func (obj *arg) OldToGolang() (string, error) {
	switch obj.Type {
	case "bool":
		return "Bool", nil
	case "string", "[]byte":
		return "Str", nil
	case "int", "int64":
		return "Int", nil
	case "float64":
		return "Float", nil
	//case "[]string":
	// XXX: Lists don't fit well with this code design. Refactor!
	default:
		return "", fmt.Errorf("cannot convert %v to golang", obj)
	}
}

// ToGolang prints the arg signature as expected by golang.
func (obj *arg) ToGolang(val string) (string, error) {
	switch obj.Type {
	case "bool":
		return fmt.Sprintf("%s.Bool()", val), nil

	case "string", "[]byte":
		return fmt.Sprintf("%s.Str()", val), nil

	case "int":
		// TODO: consider switching types.Value int64 to int everywhere
		if bits.UintSize == 32 { // special case for 32 bit golang
			return fmt.Sprintf("int(%s.Int())", val), nil
		}
		fallthrough
	case "int64":
		return fmt.Sprintf("%s.Int()", val), nil

	case "float64":
		return fmt.Sprintf("%s.Float()", val), nil

	case "[]string":
		// This function is in the child util package and is imported by
		// the template.
		return fmt.Sprintf("util.MclListToGolang(%s)", val), nil

	default:
		return "", fmt.Errorf("cannot convert %v to golang", obj)
	}
}

// ToTestInput prints the arg signature as expected by tests.
func (obj *arg) ToTestInput() (string, error) {
	switch obj.Type {
	case "bool":
		return fmt.Sprintf("&types.BoolValue{V: %s}", obj.Name), nil
	case "string", "[]byte":
		return fmt.Sprintf("&types.StrValue{V: %s}", obj.Name), nil
	case "int":
		return fmt.Sprintf("&types.IntValue{V: %s}", obj.Name), nil
	case "float":
		return fmt.Sprintf("&types.FloatValue{V: %s}", obj.Name), nil
	default:
		return "", fmt.Errorf("cannot convert %v to test input", obj)
	}
}
