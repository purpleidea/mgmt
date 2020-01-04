// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

// +build darwin

package coresys

/*
#include <stdlib.h>
*/
import "C"

// macOS/Darwin specific implementation to get load.
func load() (one, five, fifteen float64, err error) {
	avg := []C.double{0, 0, 0}

	C.getloadavg(&avg[0], C.int(len(avg)))

	one = float64(avg[0])
	five = float64(avg[1])
	fifteen = float64(avg[2])

	return
}
