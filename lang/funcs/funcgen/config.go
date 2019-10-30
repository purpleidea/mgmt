// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
	// Supported: bool, string, int, int64, float64.
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
	case "string":
		return fmt.Sprintf("%s%s", prefix, types.TypeStr.String()), nil
	case "int", "int64":
		return fmt.Sprintf("%s%s", prefix, types.TypeInt.String()), nil
	case "float64":
		return fmt.Sprintf("%s%s", prefix, types.TypeFloat.String()), nil
	default:
		return "", fmt.Errorf("cannot convert %v to mcl", obj)
	}
}

// ToGo prints the arg signature as expected by golang.
func (obj *arg) ToGolang() (string, error) {
	switch obj.Type {
	case "bool":
		return "Bool", nil
	case "string":
		return "Str", nil
	case "int", "int64":
		return "Int", nil
	case "float64":
		return "Float", nil
	default:
		return "", fmt.Errorf("cannot convert %v to golang", obj)
	}
}

// ToTestInput prints the arg signature as expected by tests.
func (obj *arg) ToTestInput() (string, error) {
	switch obj.Type {
	case "bool":
		return fmt.Sprintf("&types.BoolValue{V: %s}", obj.Name), nil
	case "string":
		return fmt.Sprintf("&types.StrValue{V: %s}", obj.Name), nil
	case "int":
		return fmt.Sprintf("&types.IntValue{V: %s}", obj.Name), nil
	case "float":
		return fmt.Sprintf("&types.FloatValue{V: %s}", obj.Name), nil
	default:
		return "", fmt.Errorf("cannot convert %v to test input", obj)
	}
}
