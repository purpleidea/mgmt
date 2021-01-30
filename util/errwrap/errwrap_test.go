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

package errwrap

import (
	"fmt"
	"testing"
)

func TestWrapfErr1(t *testing.T) {
	if err := Wrapf(nil, "whatever: %d", 42); err != nil {
		t.Errorf("expected nil result")
	}
}

func TestAppendErr1(t *testing.T) {
	if err := Append(nil, nil); err != nil {
		t.Errorf("expected nil result")
	}
}

func TestAppendErr2(t *testing.T) {
	reterr := fmt.Errorf("reterr")
	if err := Append(reterr, nil); err != reterr {
		t.Errorf("expected reterr")
	}
}

func TestAppendErr3(t *testing.T) {
	err := fmt.Errorf("err")
	if reterr := Append(nil, err); reterr != err {
		t.Errorf("expected err")
	}
}

func TestString1(t *testing.T) {
	var err error
	if String(err) != "" {
		t.Errorf("expected empty result")
	}

	msg := "this is an error"
	if err := fmt.Errorf(msg); String(err) != msg {
		t.Errorf("expected different result")
	}
}
