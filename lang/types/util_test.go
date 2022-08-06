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

package types

import "testing"

func TestNextPowerOfTwo(t *testing.T) {
	testCases := map[uint32]uint32{
		1: 1,
		2: 2,
		3: 4,
		5: 8,
	}

	for v, exp := range testCases {
		if pow := nextPowerOfTwo(v); pow != exp {
			t.Errorf("function NextPowerOfTwo of `%d` did not match expected: `%d`", pow, exp)
		}
	}
}
