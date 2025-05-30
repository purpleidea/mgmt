// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

//go:build !root

package errwrap

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapfErr1(t *testing.T) {
	if err := Wrapf(nil, "whatever: %d", 42); err != nil {
		t.Errorf("expected nil result")
	}
}

func TestWrapfErr2(t *testing.T) {
	reterr := fmt.Errorf("reterr")
	if err := Wrapf(reterr, "whatever: %d", 42); err == nil {
		t.Errorf("expected err")
	}
}

func TestWrapfErr3(t *testing.T) {
	reterr := fmt.Errorf("reterr")
	if err := Wrapf(reterr, "whatever: %d", 42); !errors.Is(err, reterr) {
		t.Errorf("expected matching err")
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
