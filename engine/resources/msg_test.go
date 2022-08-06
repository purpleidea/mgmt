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

//go:build !root

package resources

import (
	"testing"
)

func TestMsgValidate1(t *testing.T) {
	r1 := &MsgRes{
		Priority: "Debug",
	}

	if err := r1.Validate(); err != nil {
		t.Errorf("validate failed with: %v", err)
	}
}

func TestMsgValidate2(t *testing.T) {
	r1 := &MsgRes{
		Priority: "UnrealPriority",
	}

	if err := r1.Validate(); err == nil {
		t.Errorf("validation error is nil")
	}
}
