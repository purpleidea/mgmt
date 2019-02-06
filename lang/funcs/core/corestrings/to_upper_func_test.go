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

package corestrings

import (
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func testToUpper(t *testing.T, input, expected string) {
	inputStr := &types.StrValue{V: input}
	value, err := ToUpper([]types.Value{inputStr})
	if err != nil {
		t.Error(err)
		return
	}
	if value.Str() != expected {
		t.Errorf("Invalid output, expected %s, got %s", expected, value.Str())
	}
}

func TestToUpperSimple(t *testing.T) {
	testToUpper(t, "Hello", "HELLO")
}

func TestToUpperSameString(t *testing.T) {
	testToUpper(t, "HELLO 22", "HELLO 22")
}
