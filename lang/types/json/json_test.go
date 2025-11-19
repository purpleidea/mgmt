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

package json

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestValueOfJSON0(t *testing.T) {
	str := "true"
	val := &types.BoolValue{V: true}

	v, err := ValueOfJSON(str, types.NewType("bool"))
	if err != nil {
		t.Errorf("json of `%s` errored: `%v`", str, err)
		return
	}
	if err := v.Cmp(val); err != nil {
		t.Errorf("json of `%s` did not match expected: `%v`", str, val)
		return
	}
}

func TestValueOfJSON1(t *testing.T) {
	testCases := []struct {
		str string
		typ *types.Type
		val types.Value
	}{
		{
			str: "false",
			typ: types.NewType("bool"), // we can infer these!
			val: &types.BoolValue{V: false},
		},
		{
			str: "true",
			val: &types.BoolValue{V: true},
		},
		{
			str: `""`, // not empty string, but two quotes!
			val: &types.StrValue{V: ""},
		},
		{
			str: `"hello"`,
			val: &types.StrValue{V: "hello"},
		},
		{
			str: `"hello\tworld"`,
			val: &types.StrValue{V: "hello\tworld"},
		},
		{
			str: `"hello\nworld"`,
			val: &types.StrValue{V: "hello\nworld"},
		},
		{
			str: `"hello\\world"`,
			val: &types.StrValue{V: "hello\\world"},
		},
		{
			str: `"hello\t\nworld"`,
			val: &types.StrValue{V: "hello\t\nworld"},
		},
		{
			str: `"\\"`,
			val: &types.StrValue{V: "\\"},
		},
		{
			str: "0",
			val: &types.IntValue{V: 0},
		},
		{
			str: "0",
			val: &types.IntValue{V: -0},
		},
		{
			str: "42",
			val: &types.IntValue{V: 42},
		},
		{
			str: "-13",
			val: &types.IntValue{V: -13},
		},
		{
			str: "0.0", // TODO: is this correct?
			val: &types.FloatValue{V: 0.0},
		},
		{
			str: "-0.0", // TODO: is this correct?
			val: &types.FloatValue{V: -0.0},
		},
		{
			str: "-4.2",
			val: &types.FloatValue{V: -4.2},
		},
		{
			str: "1.2",
			val: &types.FloatValue{V: 1.2},
		},
		{
			str: "[]",
			val: &types.ListValue{
				T: types.NewType("[]bool"), // any list type will work
				V: []types.Value{},
			},
		},
		{
			str: "[42, -13, 0]",
			//typ: types.NewType("[]int"),
			val: &types.ListValue{
				T: types.NewType("[]int"),
				V: []types.Value{
					&types.IntValue{V: 42},
					&types.IntValue{V: -13},
					&types.IntValue{V: 0},
				},
			},
		},
		{
			str: `["a", "bb", "ccc"]`,
			val: &types.ListValue{
				T: types.NewType("[]str"),
				V: []types.Value{
					&types.StrValue{V: "a"},
					&types.StrValue{V: "bb"},
					&types.StrValue{V: "ccc"},
				},
			},
		},
		{
			str: `[["a", "bb", "ccc"], ["d", "ee", "fff"], ["g", "hh", "iii"]]`,
			val: &types.ListValue{
				T: types.NewType("[][]str"),
				V: []types.Value{
					&types.ListValue{
						T: types.NewType("[]str"),
						V: []types.Value{
							&types.StrValue{V: "a"},
							&types.StrValue{V: "bb"},
							&types.StrValue{V: "ccc"},
						},
					},
					&types.ListValue{
						T: types.NewType("[]str"),
						V: []types.Value{
							&types.StrValue{V: "d"},
							&types.StrValue{V: "ee"},
							&types.StrValue{V: "fff"},
						},
					},
					&types.ListValue{
						T: types.NewType("[]str"),
						V: []types.Value{
							&types.StrValue{V: "g"},
							&types.StrValue{V: "hh"},
							&types.StrValue{V: "iii"},
						},
					},
				},
			},
		},
	}

	d0 := types.NewMap(types.NewType("map{str: int}"))
	testCases = append(testCases, struct {
		str string
		typ *types.Type
		val types.Value
	}{
		str: "{}",
		val: d0,
	})

	// helper
	test := func(str string, typ *types.Type, val types.Value) {
		testCases = append(testCases, struct {
			str string
			typ *types.Type
			val types.Value
		}{
			str: str,
			typ: typ,
			val: val,
		})
	}

	d1 := types.NewMap(types.NewType("map{str: int}"))
	d1.Set(&types.StrValue{V: "answer"}, &types.IntValue{V: 42})
	test(`{"answer": 42}`, nil, d1)

	// json doesn't support non-string keys for maps
	//d2 := types.NewMap(types.NewType("map{int: str}"))
	//d2.Add(&types.IntValue{V: 42}, &types.StrValue{V: "answer"})
	//test(`{42: "answer"}`, nil, d2)

	s0 := types.NewStruct(types.NewType("struct{}"))
	test(`{}`, nil, s0)

	s1 := types.NewStruct(types.NewType("struct{answer int}"))
	test(`{"answer": 0}`, nil, s1)

	s2 := types.NewStruct(types.NewType("struct{answer int; truth bool; hello str}"))
	test(`{"answer": 0, "truth": false, "hello": ""}`, nil, s2)

	s3 := types.NewStruct(types.NewType("struct{answer int; truth bool; hello str; nested []int}"))
	test(`{"answer": 0, "truth": false, "hello": "", "nested": []}`, nil, s3)

	s4 := types.NewStruct(types.NewType("struct{answer int; truth bool; hello str; nested []int}"))
	if err := s4.Set("answer", &types.IntValue{V: 42}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	if err := s4.Set("truth", &types.BoolValue{V: true}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	test(`{"answer": 42, "truth": true, "hello": "", "nested": []}`, nil, s4)

	for index, tc := range testCases {
		name := fmt.Sprintf("test ValueOfJSON1 #%d_", index)

		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Logf("json: %s", tc.str)
			typ := tc.typ
			if typ == nil {
				typ = tc.val.Type() // inspect the expected!
				t.Logf("extracting type: %s", typ)
			}
			v, err := ValueOfJSON(tc.str, typ)
			if err != nil {
				t.Errorf("json of `%s` errored: `%v`", tc.str, err)
				return
			}
			if err := v.Cmp(tc.val); err != nil {
				t.Errorf("json of `%s` did not match expected: `%v`", tc.str, tc.val)
				t.Errorf("error: %v", err)
				t.Logf("got: %+v", v)
				return
			}

		})
	}
}
