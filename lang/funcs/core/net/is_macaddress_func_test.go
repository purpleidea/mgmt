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
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestIsMacAddress(t *testing.T) {
	macAddressTests := []struct {
		name     string
		mac      string
		expected bool
		err      error
	}{
		{"valid mac with :", "0a:1b:2c:3d:4f:56", true, nil},
		{"valid mac with -", "0a-1b-2c-3d-4f-56", true, nil},
		{"valid mac with .", "0a.1b.2c.3d.4f.56", true, nil},
		{"valid mac with UPPERCASE", "0A:1B:2C:3D:4F:56", true, nil},
		{"valid mac with UPPERCASE", "0A-1B-2C-3D-4F-56", true, nil},
		{"valid mac with UPPERCASE", "0A.1B.2C.3D.4F.56", true, nil},
		{"invalid mac invalid chars", "0x:1j:2c:3d:4m:56", false, nil},
		{"invalid mac invalid chars", "0x-1j-2c-3d-4m-56", false, nil},
		{"invalid mac invalid delimiter", "0a*1b*2c*3d*4f*56", false, nil},
		{"invalid mac invalid delimiter", "0a=1b=2c=3d=4f=56", false, nil},
		{"invalid mac invalid delimiter", "0a/1b/2c/3d/4f/56", false, nil},
		{"invalid mac invalid delimiter", "0a;1b;2c;3d;4f;56", false, nil},
		{"invalid mac mixed delimiters", "0a:1b:2c-3d:4f:56", false, nil},
		{"invalid mac invalid delimiter position", "0a::1b2:c3d:4f:56", false, nil},
		{"invalid mac invalid delimiter position", "0a--1b2-c3d-4f-56", false, nil},
	}

	for _, test := range macAddressTests {
		t.Run(test.name, func(t *testing.T) {
			test := test
			// err will always return nil
			output, _ := IsMacAddress([]types.Value{
				&types.StrValue{V: test.mac},
			})
			expectedBool := &types.BoolValue{V: test.expected}

			if err1 := output.Cmp(expectedBool); err1 != nil {
				t.Errorf("mac: %v, expected: %v, got: %v", test.mac, expectedBool, output)
				return
			}

		})
	}

}
