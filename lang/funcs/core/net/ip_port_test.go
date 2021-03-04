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
		{"correct ipv4 and port", "192.168.1.1", "80", "192.168.1.1:80", nil},
		{"correct ipv6 and port", "2345:0425:2CA1:0000:0000:0567:5673:23b5", "8080", "2345:0425:2CA1:0000:0000:0567:5673:23b5:8080", nil},
		{"correct ipv4 and port - allow port 0", "192.168.1.1", "0", "192.168.1.1:0", nil},
		{"correct ipv6 and port - allows port 65536", "2345:0425:2CA1::0567:5673:23b5", "65536", "2345:0425:2CA1::0567:5673:23b5:65536", nil},
		// Fail
		{"incorrect ipv4 - octet over 255", "392.868.11.79", "80", "", errors.New("incorrect ip format")},
		{"incorrect ipv4 - CIDR format", "10.10.10.100/8", "23", "", errors.New("incorrect ip format")},
		{"incorrect ipv4 - dots...", "172.16.10..254", "23", "", errors.New("incorrect ip format")},
		{"incorrect ipv6 - double double colon", "5678:A425:2CA1::0567::5673:23b5", "80", "", errors.New("incorrect ip format")},
		{"incorrect ipv6 - non hex chars", "M678:Z425:2CA1::05X7:T673:23b5", "1234", "", errors.New("incorrect ip format")},
		{"incorrect port - outside of range", "192.168.1.1", "65537", "", errors.New("port not in range 0-65536")},
		{"incorrect port - negative port number", "192.168.1.1", "-9483670", "", errors.New("port not in range 0-65536")},
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
