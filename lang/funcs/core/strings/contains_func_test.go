// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

func TestContains(t *testing.T) {
	containsTests := []struct {
		name     string
		s        string
		substr   string
		expected string
		err      error
	}{
		// Success
		{"does contain", "tomorrow", "row", "true", nil},
		{"does not contain", "fighter", "light", "false", nil},
	}

	for _, test := range containsTests {
		t.Run(test.name, func(t *testing.T) {
			output, err := Contains([]types.Value{
				&types.StrValue{V: test.s},
				&types.StrValue{V: test.substr},
			})
			expectedStr := &types.StrValue{V: test.expected}

			if test.err != nil && err.Error() != test.err.Error() {
				t.Errorf("s: %s, substr %s, expected error: %q, got %q", test.s, test.substr, test.err, err)
				return
			} else if test.err != nil && err == nil {
				t.Errorf("s: %s, substr: %s, expected error: %v, but got nil", test.s, test.substr, test.err)
				return
			} else if test.err == nil && err != nil {
				t.Errorf("s: %s, substr %s, did not expect error but got: %#v", test.s, test.substr, err)
				return
			}
			if err1 := output.Cmp(expectedStr); err1 != nil {
				t.Errorf("s: %s, substr: %s, expected: %s, got: %s", test.s, test.substr, expectedStr, output)
				return
			}

		})
	}

}
