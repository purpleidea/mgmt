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

package corestrings

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestToUpper(t *testing.T) {
	input := &types.StrValue{V: "hello"}
	value, err := ToUpper(context.Background(), []types.Value{input})
	if err != nil {
		t.Error(err)
		return
	}
	if value.Str() != "HELLO" {
		t.Errorf("expected HELLO, got %s", value.Str())
	}
}

func TestTrimSpace(t *testing.T) {
	input := &types.StrValue{V: "  hello world  "}
	value, err := TrimSpace(context.Background(), []types.Value{input})
	if err != nil {
		t.Error(err)
		return
	}
	if value.Str() != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", value.Str())
	}
}

func TestContains(t *testing.T) {
	s := &types.StrValue{V: "hello world"}
	substr := &types.StrValue{V: "world"}
	value, err := Contains(context.Background(), []types.Value{s, substr})
	if err != nil {
		t.Error(err)
		return
	}
	if !value.Bool() {
		t.Errorf("expected true for 'hello world' contains 'world'")
	}

	substr2 := &types.StrValue{V: "xyz"}
	value2, err := Contains(context.Background(), []types.Value{s, substr2})
	if err != nil {
		t.Error(err)
		return
	}
	if value2.Bool() {
		t.Errorf("expected false for 'hello world' contains 'xyz'")
	}
}

func TestHasPrefix(t *testing.T) {
	s := &types.StrValue{V: "/usr/bin/mgmt"}
	prefix := &types.StrValue{V: "/usr"}
	value, err := HasPrefix(context.Background(), []types.Value{s, prefix})
	if err != nil {
		t.Error(err)
		return
	}
	if !value.Bool() {
		t.Errorf("expected true for '/usr/bin/mgmt' has prefix '/usr'")
	}
}

func TestHasSuffix(t *testing.T) {
	s := &types.StrValue{V: "config.yaml"}
	suffix := &types.StrValue{V: ".yaml"}
	value, err := HasSuffix(context.Background(), []types.Value{s, suffix})
	if err != nil {
		t.Error(err)
		return
	}
	if !value.Bool() {
		t.Errorf("expected true for 'config.yaml' has suffix '.yaml'")
	}
}

func TestReplaceAll(t *testing.T) {
	s := &types.StrValue{V: "hello world world"}
	old := &types.StrValue{V: "world"}
	newStr := &types.StrValue{V: "mgmt"}
	value, err := ReplaceAll(context.Background(), []types.Value{s, old, newStr})
	if err != nil {
		t.Error(err)
		return
	}
	if value.Str() != "hello mgmt mgmt" {
		t.Errorf("expected 'hello mgmt mgmt', got '%s'", value.Str())
	}
}
