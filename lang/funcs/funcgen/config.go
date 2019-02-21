// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	Functions functions `yaml:"functions"`
}

type functions []function

type testarg struct {
	Name  string `yaml:"name,omitempty"`
	Type  string `yaml:"type"`
	Value string `yaml:"value"`
}

type arg struct {
	Name string `yaml:"name,omitempty"`
	Type string `yaml:"type"`
}

// ToMcl prints the arg signature as expected by mcl.
func (obj *arg) ToMcl() (string, error) {
	if obj.Type == "string" {
		if obj.Name != "" {
			return fmt.Sprintf("%s str", obj.Name), nil
		}
		return types.TypeStr.String(), nil
	}
	return "", fmt.Errorf("cannot convert %v to mcl", obj)
}

// ToGo prints the arg signature as expected by golang.
func (obj *arg) ToGo() (string, error) {
	if obj.Type == "string" {
		return "Str", nil
	}
	return "", fmt.Errorf("cannot convert %v to go", obj)
}

// ToTestInput prints the arg signature as expected by tests.
func (obj *arg) ToTestInput() (string, error) {
	if obj.Type == "string" {
		return fmt.Sprintf("&types.StrValue{V: %s}", obj.Name), nil
	}
	return "", fmt.Errorf("cannot convert %v to test input", obj)
}
