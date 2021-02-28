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

package corenet

import (
	"errors"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestIPPort(t *testing.T) {
	ipporttests := []struct {
		name     string
		ip       string
		port     string
		expected string
		err      error
	}{
		// Success
		{"success", "192.168.1.1", "80", "192.168.1.1:80", nil},
		// Fail
		{"fail", "192.168.1.1", "9483670", "", errors.New("port number must be between 1-65536")},
	}

	for _, test := range ipporttests {
		t.Run(test.name, func(t *testing.T) {
			output, err := IPPort([]types.Value{
				&types.StrValue{V: test.ip},
				&types.StrValue{V: test.port},
			})
			expectedStr := &types.StrValue{V: test.expected}

			if test.err != nil && err.Error() != test.err.Error() {
				t.Errorf("ip: %s, port %s, expected error: %q, got %q", test.ip, test.port, test.err, err)
				return
			} else if test.err != nil && err == nil {
				t.Errorf("ip: %s, port: %s, expected error: %v, but got nil", test.ip, test.port, test.err)
				return
			} else if test.err == nil && err != nil {
				t.Errorf("ip: %s, port %s, did not expect error but got: %#v", test.ip, test.port, err)
				return
			}
			if err1 := output.Cmp(expectedStr); err1 != nil {
				t.Errorf("ip: %s, port: %s, expected: %s, got: %s", test.ip, test.port, expectedStr, output)
				return
			}

		})
	}

}
