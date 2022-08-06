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

package corenet

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestCidrToIP(t *testing.T) {
	cidrtests := []struct {
		name     string
		input    string
		expected string
		err      error
	}{
		// IPv4 success.
		{"IPv4 cidr", "192.0.2.12/24", "192.0.2.12", nil},
		{"spaced IPv4 cidr ", "  192.168.42.13/24  ", "192.168.42.13", nil},

		//IPv4 failure - tests error.
		{"invalid IPv4 cidr", "192.168.42.13/33", "", fmt.Errorf("invalid CIDR address: 192.168.42.13/33")},

		// IPV6 success.
		{"IPv6 cidr", "2001:db8::/32", "2001:db8::", nil},
		{"spaced IPv6 cidr ", "   2001:db8::/32   ", "2001:db8::", nil},

		// IPv6 failure - tests error.
		{"invalid IPv6 cidr", "2001:db8::/333", "", fmt.Errorf("invalid CIDR address: 2001:db8::/333")},
	}

	for _, ts := range cidrtests {
		test := ts
		t.Run(test.name, func(t *testing.T) {
			output, err := CidrToIP([]types.Value{&types.StrValue{V: test.input}})
			expectedStr := &types.StrValue{V: test.expected}

			if test.err != nil && err.Error() != test.err.Error() {
				t.Errorf("input: %s, expected error: %q, got: %q", test.input, test.err, err)
				return
			} else if test.err != nil && err == nil {
				t.Errorf("input: %s, expected error: %v, but got nil", test.input, test.err)
				return
			} else if test.err == nil && err != nil {
				t.Errorf("input: %s, did not expect error but got: %#v", test.input, err)
				return
			}
			if err1 := output.Cmp(expectedStr); err1 != nil {
				t.Errorf("input: %s, expected: %s, got: %s", test.input, expectedStr, output)
				return
			}
		})
	}
}
