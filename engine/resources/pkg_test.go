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

//go:build !root

package resources

import (
	"testing"
)

func TestNilList1(t *testing.T) {
	var x []string
	if x != nil { // we have this expectation for obj.fileList in pkg
		t.Errorf("list should have been nil, was: %+v", x)
	}
	x = []string{} // empty list
	if x == nil {
		t.Errorf("list should have been empty, was: %+v", x)
	}
}
