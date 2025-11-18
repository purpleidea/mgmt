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
	"context"
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

func TestJoinErr1(t *testing.T) {
	if reterr := Join(nil); reterr != nil {
		t.Errorf("expected nil result")
	}
}

func TestJoinErr2(t *testing.T) {
	if reterr := Join([]error{}); reterr != nil {
		t.Errorf("expected nil result")
	}
}

func TestJoinErr3(t *testing.T) {
	err := fmt.Errorf("err")
	if reterr := Join([]error{err}); reterr != err {
		t.Errorf("expected err")
	}
}

func TestJoinErr4(t *testing.T) {
	err := fmt.Errorf("err")
	if reterr := Join([]error{err, nil}); reterr != err {
		t.Errorf("expected err")
	}
}

func TestJoinErr5(t *testing.T) {
	err := fmt.Errorf("err")
	if reterr := Join([]error{nil, err}); reterr != err {
		t.Errorf("expected err")
	}
}

func TestJoinErr6(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := fmt.Errorf("err2")
	if reterr := Join([]error{err1, err2}); reterr.Error() != Append(err1, err2).Error() {
		t.Errorf("expected err")
	}
}

func TestJoinErr7(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := fmt.Errorf("err2")
	err3 := fmt.Errorf("err3")
	if reterr := Join([]error{err1, err2, err3}); reterr.Error() != Append(err1, Append(err2, err3)).Error() {
		t.Errorf("expected err")
	}
	if reterr := Join([]error{err1, err2, err3}); reterr.Error() != Append(Append(err1, err2), err3).Error() {
		t.Errorf("expected err")
	}
}

func TestJoinErr8(t *testing.T) {
	err1 := fmt.Errorf("err1")
	var err2 error // nil
	err3 := fmt.Errorf("err3")
	if reterr := Join([]error{err1, err2, err3}); reterr.Error() != Append(err1, Append(err2, err3)).Error() {
		t.Errorf("expected err")
	}
	if reterr := Join([]error{err1, err2, err3}); reterr.Error() != Append(Append(err1, err2), err3).Error() {
		t.Errorf("expected err")
	}
}

func TestJoinErr9(t *testing.T) {
	var err1 error // nil
	var err2 error // nil
	err3 := fmt.Errorf("err3")
	if reterr := Join([]error{err1, err2, err3}); reterr.Error() != Append(err1, Append(err2, err3)).Error() {
		t.Errorf("expected err")
	}
	if reterr := Join([]error{err1, err2, err3}); reterr.Error() != Append(Append(err1, err2), err3).Error() {
		t.Errorf("expected err")
	}
}

func TestJoinErr10(t *testing.T) {
	var err1 error // nil
	var err2 error // nil
	var err3 error // nil
	if reterr := Join([]error{err1, err2, err3}); reterr != nil {
		t.Errorf("expected nil result")
	}
}

func TestWithoutContext1(t *testing.T) {
	if reterr := WithoutContext(nil); reterr != nil {
		t.Errorf("expected nil result")
	}
}

func TestWithoutContext2(t *testing.T) {
	err := fmt.Errorf("err")
	if reterr := WithoutContext(err); reterr != err {
		t.Errorf("expected err")
	}
}

func TestWithoutContext3(t *testing.T) {
	err := context.Canceled
	if reterr := WithoutContext(err); reterr != err {
		t.Errorf("expected err")
	}
}

func TestWithoutContext4(t *testing.T) {
	err1 := fmt.Errorf("err")
	err2 := context.Canceled
	err := Append(err1, err2)
	if reterr := WithoutContext(err); reterr != err1 {
		t.Errorf("expected err")
	}
}

func TestWithoutContext5(t *testing.T) {
	err1 := context.Canceled
	err2 := fmt.Errorf("err")
	err := Append(err1, err2)
	if reterr := WithoutContext(err); reterr != err2 {
		t.Errorf("expected err")
	}
}

func TestWithoutContext6(t *testing.T) {
	err1 := context.Canceled
	err2 := context.Canceled
	err := Append(err1, err2)
	if reterr := WithoutContext(err); reterr != err1 {
		t.Errorf("expected err")
	}
	if reterr := WithoutContext(err); reterr != err2 {
		t.Errorf("expected err")
	}
}

func TestWithoutContext7(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := fmt.Errorf("err2")
	err := Append(err1, err2)
	if reterr := WithoutContext(err); reterr.Error() != err.Error() {
		t.Errorf("expected err")
	}
}

func TestWithoutContext8(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := fmt.Errorf("err2")
	err3 := fmt.Errorf("err3")
	err := Join([]error{err1, err2, err3})
	if reterr := WithoutContext(err); reterr.Error() != err.Error() {
		t.Errorf("expected err")
	}
}

func TestWithoutContext9(t *testing.T) {
	err1 := context.Canceled
	err2 := fmt.Errorf("err2")
	err3 := fmt.Errorf("err3")
	err := Join([]error{err1, err2, err3})
	exp := Join([]error{nil, err2, err3})
	if reterr := WithoutContext(err); reterr.Error() != exp.Error() {
		t.Errorf("expected err")
	}
}

func TestWithoutContext10(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := context.Canceled
	err3 := fmt.Errorf("err3")
	err := Join([]error{err1, err2, err3})
	exp := Join([]error{err1, nil, err3})
	if reterr := WithoutContext(err); reterr.Error() != exp.Error() {
		t.Errorf("expected err")
	}
}

func TestWithoutContext11(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := fmt.Errorf("err2")
	err3 := context.Canceled
	err := Join([]error{err1, err2, err3})
	exp := Join([]error{err1, err2, nil})
	if reterr := WithoutContext(err); reterr.Error() != exp.Error() {
		t.Errorf("expected err")
	}
}

func TestWithoutContext12(t *testing.T) {
	err1 := fmt.Errorf("err1")
	err2 := context.Canceled
	err3 := context.Canceled
	err := Join([]error{err1, err2, err3})
	if reterr := WithoutContext(err); reterr != err1 {
		t.Errorf("expected err")
	}
}

func TestWithoutContext13(t *testing.T) {
	err1 := context.Canceled
	err2 := fmt.Errorf("err2")
	err3 := context.Canceled
	err := Join([]error{err1, err2, err3})
	if reterr := WithoutContext(err); reterr != err2 {
		t.Errorf("expected err")
	}
}

func TestWithoutContext14(t *testing.T) {
	err1 := context.Canceled
	err2 := context.Canceled
	err3 := fmt.Errorf("err3")
	err := Join([]error{err1, err2, err3})
	if reterr := WithoutContext(err); reterr != err3 {
		t.Errorf("expected err")
	}
}

func TestWithoutContext15(t *testing.T) {
	err1 := context.Canceled
	err2 := context.Canceled
	err3 := context.Canceled
	err := Join([]error{err1, err2, err3})
	if reterr := WithoutContext(err); reterr != context.Canceled {
		t.Errorf("expected err")
	}
}

func TestString1(t *testing.T) {
	var err error
	if String(err) != "" {
		t.Errorf("expected empty result")
	}

	msg := "this is an error"
	if err := fmt.Errorf("%s", msg); String(err) != msg {
		t.Errorf("expected different result")
	}
}
