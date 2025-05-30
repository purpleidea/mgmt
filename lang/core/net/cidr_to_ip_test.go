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

package corenet

import (
	"context"
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

		// IPv4 failure - tests error.
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
			output, err := CidrToIP(context.Background(), []types.Value{&types.StrValue{V: test.input}})
			expectedStr := &types.StrValue{V: test.expected}

			if (test.err == nil) && err != nil {
				t.Errorf("input: %s, did not expect error but got: %#v", test.input, err)
				return
			}
			if (test.err != nil) && err != nil {
				s := err.Error() // convert to string
				if s == test.err.Error() {
					return
				}
				t.Errorf("input: %s, expected error: %q, got: %q", test.input, test.err, err)
				return
			}
			if (test.err != nil) && err == nil {
				t.Errorf("input: %s, expected error: %v, but got nil", test.input, test.err)
				return
			}

			if err := output.Cmp(expectedStr); err != nil {
				t.Errorf("input: %s, expected: %s, got: %s", test.input, expectedStr, output)
				return
			}
		})
	}
}

func TestCidrToMask(t *testing.T) {
	cidrtests := []struct {
		name     string
		input    string
		expected string
		err      error
	}{
		// IPv4 success.
		{"IPv4 cidr", "192.0.2.12/24", "255.255.255.0", nil},
		{"Another IPv4 cidr", "192.0.2.12/13", "255.248.0.0", nil},
		{"spaced IPv4 cidr ", "  192.168.42.13/24  ", "255.255.255.0", nil},

		// IPv4 failure - tests error.
		{"invalid IPv4 cidr", "192.168.42.13/33", "", fmt.Errorf("invalid CIDR address: 192.168.42.13/33")},

		// IPV6 success.
		// TODO: nobody knows how this should work
		//{"IPv6 cidr", "2001:db8::/32", "ffff:ffff::", nil},
		//{"spaced IPv6 cidr ", "   2001:db8::/32   ", "ffff:ffff::", nil},

		// IPv6 failure - tests error.
		{"invalid IPv6 cidr", "2001:db8::/333", "", fmt.Errorf("invalid CIDR address: 2001:db8::/333")},
	}

	for _, ts := range cidrtests {
		test := ts
		t.Run(test.name, func(t *testing.T) {
			output, err := CidrToMask(context.Background(), []types.Value{&types.StrValue{V: test.input}})
			expectedStr := &types.StrValue{V: test.expected}

			if (test.err == nil) && err != nil {
				t.Errorf("input: %s, did not expect error but got: %#v", test.input, err)
				return
			}
			if (test.err != nil) && err != nil {
				s := err.Error() // convert to string
				if s == test.err.Error() {
					return
				}
				t.Errorf("input: %s, expected error: %q, got: %q", test.input, test.err, err)
				return
			}
			if (test.err != nil) && err == nil {
				t.Errorf("input: %s, expected error: %v, but got nil", test.input, test.err)
				return
			}

			if err := output.Cmp(expectedStr); err != nil {
				t.Errorf("input: %s, expected: %s, got: %s", test.input, expectedStr, output)
				return
			}
		})
	}
}
