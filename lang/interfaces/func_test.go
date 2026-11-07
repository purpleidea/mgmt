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

package interfaces

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

// testFunc is a minimal func, which we only use as a key in a table.
type testFunc struct {
	name string
}

// String returns the name of this func.
func (obj *testFunc) String() string { return obj.name }

// Validate always passes.
func (obj *testFunc) Validate() error { return nil }

// Info returns empty info.
func (obj *testFunc) Info() *Info { return &Info{} }

// Init does nothing.
func (obj *testFunc) Init(*Init) error { return nil }

// Call always errors, since it should never be called.
func (obj *testFunc) Call(context.Context, []types.Value) (types.Value, error) {
	return nil, fmt.Errorf("not implemented")
}

// TestTableValue checks that a func which failed gives us its error instead of
// a value, and that a missing (or nil) value is an error too.
func TestTableValue(t *testing.T) {
	f := &testFunc{name: "f"}
	value := &types.StrValue{V: "x"}
	failure := fmt.Errorf("oops")

	testCases := []struct {
		name  string
		table Table
		value types.Value // the value we expect, if any
		err   error       // the error we expect, if any
	}{
		{name: "value", table: Table{f: value}, value: value},
		{name: "error", table: Table{f: &types.ErrValue{Typ: types.TypeStr, Err: failure}}, err: failure},
		{name: "missing", table: Table{}, err: ErrTableNoValue},
		{name: "nil", table: Table{f: nil}, err: ErrTableNoValue},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := tc.table.Value(f)
			if !errors.Is(err, tc.err) {
				t.Errorf("unexpected error: %v", err)
			}
			if v != tc.value {
				t.Errorf("unexpected value: %v", v)
			}
		})
	}
}

// TestTableCmpErrValue checks that comparing two tables never compares a real
// value with an error value which has the same type, since most values panic on
// that, and that two error values are compared by their message.
func TestTableCmpErrValue(t *testing.T) {
	str := func(s string) types.Value {
		return &types.StrValue{V: s}
	}
	errValue := func(s string) types.Value {
		return &types.ErrValue{
			Typ: types.TypeStr,
			Err: fmt.Errorf("%s", s),
		}
	}

	testCases := []struct {
		name string
		v1   types.Value
		v2   types.Value
		same bool
	}{
		{name: "values", v1: str("x"), v2: str("x"), same: true},
		{name: "value and error", v1: str("x"), v2: errValue("oops"), same: false},
		{name: "error and value", v1: errValue("oops"), v2: str("x"), same: false},
		{name: "errors", v1: errValue("oops"), v2: errValue("oops"), same: true},
		{name: "different errors", v1: errValue("oops"), v2: errValue("other"), same: false},
	}

	f := &testFunc{name: "f"}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			table1 := Table{f: tc.v1}
			table2 := Table{f: tc.v2}
			err := table1.Cmp(table2)
			if tc.same && err != nil {
				t.Errorf("tables differ: %v", err)
			}
			if !tc.same && err == nil {
				t.Errorf("tables are the same, expected them to differ")
			}
		})
	}
}
