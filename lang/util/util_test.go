// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package util

import (
	"fmt"
	"sort"
	"testing"
)

func TestValidateVarName(t *testing.T) {
	testCases := map[string]error{
		"":        fmt.Errorf("got empty var name"),
		"hello":   nil,
		"NOPE":    fmt.Errorf("invalid var name: `NOPE`"),
		"$foo":    fmt.Errorf("invalid var name: `$foo`"),
		".":       fmt.Errorf("invalid var name: `.`"),
		"..":      fmt.Errorf("invalid var name: `..`"),
		"_":       fmt.Errorf("invalid var name: `_`"),
		"__":      fmt.Errorf("invalid var name: `__`"),
		"0":       fmt.Errorf("invalid var name: `0`"),
		"1":       fmt.Errorf("invalid var name: `1`"),
		"42":      fmt.Errorf("invalid var name: `42`"),
		"X":       fmt.Errorf("invalid var name: `X`"),
		"x":       nil,
		"x0":      nil,
		"x1":      nil,
		"x42":     nil,
		"x42.foo": nil,
		"x42_foo": nil,

		// XXX: fix these test cases
		//"x.": fmt.Errorf("invalid var name: x."),
		//"x_": fmt.Errorf("invalid var name: x_"),
	}

	keys := []string{}
	for k := range testCases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e, ok := testCases[k]
		if !ok {
			// programming error
			t.Errorf("missing test case: %s", k)
			continue
		}

		err := ValidateVarName(k)
		if err == nil && e == nil {
			continue
		}
		if err == nil && e != nil {
			t.Errorf("key: %s did not error, expected: %s", k, e.Error())
			continue
		}
		if err != nil && e == nil {
			t.Errorf("key: %s expected no error, got: %s", k, err.Error())
			continue
		}
		if err.Error() != e.Error() {
			t.Errorf("key: %s did not have correct error, expected: %s", k, err.Error())
			continue
		}
	}
}
