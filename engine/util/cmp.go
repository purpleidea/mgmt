// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
)

// StrListCmp compares two lists of strings. If they are not the same length or
// do not contain identical strings in the same order, then this errors.
func StrListCmp(x, y []string) error {
	if len(x) != len(y) {
		return fmt.Errorf("length differs")
	}
	for i := range x {
		if x[i] != y[i] {
			return fmt.Errorf("the elements at position %d differed", i)
		}
	}

	return nil
}
