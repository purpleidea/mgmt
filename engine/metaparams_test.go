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

// +build !root

package engine

import (
	"testing"
)

func TestMetaCmp1(t *testing.T) {
	m1 := &MetaParams{
		Noop: true,
	}
	m2 := &MetaParams{
		Noop: false,
	}

	// TODO: should we allow this? Maybe only with the future Mutate API?
	//if err := m2.Cmp(m1); err != nil { // going from noop(false) -> noop(true) is okay!
	//	t.Errorf("the two resources do not match")
	//}

	if m1.Cmp(m2) == nil { // going from noop(true) -> noop(false) is not okay!
		t.Errorf("the two resources should not match")
	}
}
