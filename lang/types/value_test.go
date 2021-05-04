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

package types

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
)

func TestPrint1(t *testing.T) {
	testCases := map[Value]string{
		&BoolValue{V: true}:            "true",
		&BoolValue{V: false}:           "false",
		&StrValue{V: ""}:               `""`,
		&StrValue{V: "hello"}:          `"hello"`,
		&StrValue{V: "hello\tworld"}:   `"hello\tworld"`,
		&StrValue{V: "hello\nworld"}:   `"hello\nworld"`,
		&StrValue{V: "hello\\world"}:   `"hello\\world"`,
		&StrValue{V: "hello\t\nworld"}: `"hello\t\nworld"`,
		&StrValue{V: "\\"}:             `"\\"`,
		&IntValue{V: 0}:                "0",
		&IntValue{V: -0}:               "0",
		&IntValue{V: 42}:               "42",
		&IntValue{V: -13}:              "-13",
		&FloatValue{V: 0.0}:            "0", // TODO: is this correct?
		&FloatValue{V: 0}:              "0", // TODO: is this correct?
		&FloatValue{V: -4.2}:           "-4.2",
		&FloatValue{V: 1.2}:            "1.2",
		&FloatValue{V: -0.0}:           "0", // TODO: is this correct?
		&ListValue{V: []Value{}}:       `[]`,
		&ListValue{V: []Value{
			&IntValue{V: 42},
			&IntValue{V: -13},
			&IntValue{V: 0}},
		}: `[42, -13, 0]`,
		&ListValue{V: []Value{
			&StrValue{V: "a"},
			&StrValue{V: "bb"},
			&StrValue{V: "ccc"}},
		}: `["a", "bb", "ccc"]`,
		&ListValue{V: []Value{ // prints okay, but is actually invalid!
			&StrValue{V: "hello"},
			&IntValue{V: 4},
			&BoolValue{V: true}},
		}: `["hello", 4, true]`,

		&ListValue{V: []Value{
			&ListValue{V: []Value{
				&StrValue{V: "a"},
				&StrValue{V: "bb"},
				&StrValue{V: "ccc"},
			}},
			&ListValue{V: []Value{
				&StrValue{V: "d"},
				&StrValue{V: "ee"},
				&StrValue{V: "fff"},
			}},
			&ListValue{V: []Value{
				&StrValue{V: "g"},
				&StrValue{V: "hh"},
				&StrValue{V: "iii"},
			}},
		}}: `[["a", "bb", "ccc"], ["d", "ee", "fff"], ["g", "hh", "iii"]]`,
	}

	d0 := NewMap(NewType("map{str: int}"))
	testCases[d0] = `{}`

	d1 := NewMap(NewType("map{str: int}"))
	d1.Add(&StrValue{V: "answer"}, &IntValue{V: 42})
	testCases[d1] = `{"answer": 42}`

	d2 := NewMap(NewType("map{str: int}"))
	d2.Add(&StrValue{V: "answer"}, &IntValue{V: 42})
	d2.Add(&StrValue{V: "hello"}, &IntValue{V: 13})
	testCases[d2] = `{"answer": 42, "hello": 13}`

	s0 := NewStruct(NewType("struct{}"))
	testCases[s0] = `struct{}`

	s1 := NewStruct(NewType("struct{answer int}"))
	testCases[s1] = `struct{answer: 0}`

	s2 := NewStruct(NewType("struct{answer int; truth bool; hello str}"))
	testCases[s2] = `struct{answer: 0; truth: false; hello: ""}`

	s3 := NewStruct(NewType("struct{answer int; truth bool; hello str; nested []int}"))
	testCases[s3] = `struct{answer: 0; truth: false; hello: ""; nested: []}`

	s4 := NewStruct(NewType("struct{answer int; truth bool; hello str; nested []int}"))
	if err := s4.Set("answer", &IntValue{V: 42}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	if err := s4.Set("truth", &BoolValue{V: true}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	testCases[s4] = `struct{answer: 42; truth: true; hello: ""; nested: []}`

	for v, exp := range testCases { // run all the tests
		if s := v.String(); s != exp {
			t.Errorf("value representation of `%s` did not match expected: `%s`", s, exp)
		}
	}
}

func TestReflectValue1(t *testing.T) {
	// value string representations in golang can be ambiguous, see below...
	testCases := map[Value]string{
		&BoolValue{V: true}:            "true",
		&BoolValue{V: false}:           "false",
		&StrValue{V: ""}:               ``,
		&StrValue{V: "hello"}:          `hello`,
		&StrValue{V: "hello\tworld"}:   "hello\tworld",
		&StrValue{V: "hello\nworld"}:   "hello\nworld",
		&StrValue{V: "hello\\world"}:   "hello\\world",
		&StrValue{V: "hello\t\nworld"}: "hello\t\nworld",
		&StrValue{V: "\\"}:             "\\",
		&IntValue{V: 0}:                "0",
		&IntValue{V: -0}:               "0",
		&IntValue{V: 42}:               "42",
		&IntValue{V: -13}:              "-13",
		&ListValue{
			T: NewType("[]int"),
			V: []Value{},
		}: `[]`,
		&ListValue{
			T: NewType("[]int"),
			V: []Value{
				&IntValue{V: 42},
				&IntValue{V: -13},
				&IntValue{V: 0},
			},
		}: `[42 -13 0]`,
		&ListValue{
			T: NewType("[]str"),
			V: []Value{
				&StrValue{V: "a"},
				&StrValue{V: "bb"},
				&StrValue{V: "ccc"},
			},
		}: `[a bb ccc]`,
		&ListValue{
			T: NewType("[]str"),
			V: []Value{
				&StrValue{V: "a bb ccc"},
			},
		}: `[a bb ccc]`, // note how this is ambiguous in golang!
		&ListValue{
			T: NewType("[][]str"),
			V: []Value{
				&ListValue{
					T: NewType("[]str"),
					V: []Value{
						&StrValue{V: "a"},
						&StrValue{V: "bb"},
						&StrValue{V: "ccc"},
					},
				},
				&ListValue{
					T: NewType("[]str"),
					V: []Value{
						&StrValue{V: "d"},
						&StrValue{V: "ee"},
						&StrValue{V: "fff"},
					},
				},
				&ListValue{
					T: NewType("[]str"),
					V: []Value{
						&StrValue{V: "g"},
						&StrValue{V: "hh"},
						&StrValue{V: "iii"},
					},
				},
			},
		}: `[[a bb ccc] [d ee fff] [g hh iii]]`,
	}

	d0 := NewMap(NewType("map{str: int}"))
	testCases[d0] = `map[]`

	d1 := NewMap(NewType("map{str: int}"))
	d1.Add(&StrValue{V: "answer"}, &IntValue{V: 42})
	testCases[d1] = `map[answer:42]`

	// multiple key maps are tested below since they have multiple outputs
	// TODO: https://github.com/golang/go/issues/21095

	s0 := NewStruct(NewType("struct{}"))
	testCases[s0] = `{}`

	s1 := NewStruct(NewType("struct{Answer int}"))
	testCases[s1] = `{Answer:0}`

	s2 := NewStruct(NewType("struct{Answer int; Truth bool; Hello str}"))
	testCases[s2] = `{Answer:0 Truth:false Hello:}`

	s3 := NewStruct(NewType("struct{Answer int; Truth bool; Hello str; Nested []int}"))
	testCases[s3] = `{Answer:0 Truth:false Hello: Nested:[]}`

	s4 := NewStruct(NewType("struct{Answer int; Truth bool; Hello str; Nested []int}"))
	if err := s4.Set("Answer", &IntValue{V: 42}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	if err := s4.Set("Truth", &BoolValue{V: true}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	testCases[s4] = `{Answer:42 Truth:true Hello: Nested:[]}`

	for v, exp := range testCases { // run all the tests
		//t.Logf("expected: %s", exp)
		if v == nil {
			t.Logf("nil: %s", exp)
			continue
		}
		val := v.Value()
		if s := fmt.Sprintf("%+v", val); s != exp {
			//t.Errorf("value representation of `%s` did not match expected: `%s`", s, exp)
			t.Errorf("value representation of `%s`", s)
			t.Errorf("did not match expected: `%s`", exp)
		}
	}
}

func TestSort1(t *testing.T) {
	type test struct { // an individual test
		values []Value
		sorted []Value
	}
	testCases := []test{
		{
			[]Value{},
			[]Value{},
		},
		{
			[]Value{
				&BoolValue{V: true},
			},
			[]Value{
				&BoolValue{V: true},
			},
		},
		{
			[]Value{
				&BoolValue{V: true},
				&BoolValue{V: false},
			},
			[]Value{
				&BoolValue{V: false},
				&BoolValue{V: true},
			},
		},
		{
			[]Value{
				&BoolValue{V: false},
				&BoolValue{V: false},
				&BoolValue{V: true},
				&BoolValue{V: false},
			},
			[]Value{
				&BoolValue{V: false},
				&BoolValue{V: false},
				&BoolValue{V: false},
				&BoolValue{V: true},
			},
		},
		{
			[]Value{
				&StrValue{V: "c"},
				&StrValue{V: "a"},
				&StrValue{V: "b"},
			},
			[]Value{
				&StrValue{V: "a"},
				&StrValue{V: "b"},
				&StrValue{V: "c"},
			},
		},
		{
			[]Value{
				&StrValue{V: "c"},
				&StrValue{V: "aa"},
				&StrValue{V: "b"},
			},
			[]Value{
				&StrValue{V: "aa"},
				&StrValue{V: "b"},
				&StrValue{V: "c"},
			},
		},
		{
			[]Value{
				&StrValue{V: "c"},
				&StrValue{V: "bb"},
				&StrValue{V: "a"},
			},
			[]Value{
				&StrValue{V: "a"},
				&StrValue{V: "bb"},
				&StrValue{V: "c"},
			},
		},
		{
			[]Value{
				&IntValue{V: 2},
				&IntValue{V: 0},
				&IntValue{V: 3},
				&IntValue{V: 1},
			},
			[]Value{
				&IntValue{V: 0},
				&IntValue{V: 1},
				&IntValue{V: 2},
				&IntValue{V: 3},
			},
		},
		{
			[]Value{
				&IntValue{V: 2},
				&IntValue{V: 0},
				&IntValue{V: -3},
				&IntValue{V: 1},
				&IntValue{V: 42},
			},
			[]Value{
				&IntValue{V: -3},
				&IntValue{V: 0},
				&IntValue{V: 1},
				&IntValue{V: 2},
				&IntValue{V: 42},
			},
		},
		{
			[]Value{
				&ListValue{
					V: []Value{
						&StrValue{V: "c"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "bb"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "a"},
					},
					T: NewType("[]str"),
				},
			},
			[]Value{
				&ListValue{
					V: []Value{
						&StrValue{V: "a"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "bb"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "c"},
					},
					T: NewType("[]str"),
				},
			},
		},
		{
			[]Value{
				&ListValue{
					V: []Value{
						&StrValue{V: "c"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "bb"},
						&StrValue{V: "zz"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "a"},
						&StrValue{V: "zzz"},
					},
					T: NewType("[]str"),
				},
			},
			[]Value{
				&ListValue{
					V: []Value{
						&StrValue{V: "a"},
						&StrValue{V: "zzz"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "bb"},
						&StrValue{V: "zz"},
					},
					T: NewType("[]str"),
				},
				&ListValue{
					V: []Value{
						&StrValue{V: "c"},
					},
					T: NewType("[]str"),
				},
			},
		},
		// FIXME: add map and struct sorting tests
	}

	for index, tc := range testCases { // run all the tests
		v1, v2 := tc.values, tc.sorted
		sort.Sort(ValueSlice(v1)) // sort it :)

		if l1, l2 := len(v1), len(v2); l1 != l2 {
			t.Errorf("sort test #%d: had wrong length got %d, expected %d", index, l1, l2)
			continue
		}
		// cmp two lists each element at a time
		for i := 0; i < len(v1); i++ {
			if err := v1[i].Cmp(v2[i]); err != nil {
				t.Errorf("sort test #%d: value did not match expected: %v", index, err)
				t.Errorf("got: `%+v`", v1)
				t.Errorf("exp: `%+v`", v2)
				break
			}
		}
	}
}

func TestMapReflectValue1(t *testing.T) {
	d := NewMap(NewType("map{str: int}"))
	d.Add(&StrValue{V: "answer"}, &IntValue{V: 42})
	d.Add(&StrValue{V: "hello"}, &IntValue{V: 13})
	// both are valid, since map's aren't sorted
	// imo, golang should at least sort these on display!
	// TODO: https://github.com/golang/go/issues/21095
	exp1 := `map[answer:42 hello:13]`
	exp2 := `map[hello:13 answer:42]`

	val := d.Value()
	if s := fmt.Sprintf("%+v", val); s != exp1 && s != exp2 {
		t.Errorf("value representation of `%s`", s)
		t.Errorf("did not match expected: `%s`", exp1)
		t.Errorf("did not match expected: `%s`", exp2)
	}

	d2 := NewMap(NewType("map{str: str}"))
	d2.Add(&StrValue{V: "answer"}, &StrValue{V: "42 hello:13"})
	val2 := d2.Value()

	if v1, v2 := fmt.Sprintf("%+v", val), fmt.Sprintf("%+v", val2); v1 == v2 {
		t.Logf("golang maps are ambiguous")
	} else {
		//t.Errorf("golang maps are broken ?")
		//t.Errorf("val1: %s", v1)
		//t.Errorf("val2: %s", v2)
	}
}

func TestList1(t *testing.T) {
	l := NewList(NewType("[]int"))
	v := &IntValue{V: 42}
	if err := l.Add(v); err != nil {
		t.Errorf("list could not add value: %s", v)
	}

	value, exists := l.Lookup(0) // the index!
	if !exists {
		t.Errorf("list did not contain our value")
		return
	}

	if err := value.Cmp(&IntValue{V: 42}); err != nil {
		t.Errorf("value did not match our list value")
	}
}

func TestMapLookup1(t *testing.T) {
	d := NewMap(NewType("map{str: int}"))
	k := &StrValue{V: "answer"}
	v := &IntValue{V: 42}
	if err := d.Add(k, v); err != nil {
		t.Errorf("map could not add key %s, val: %s", k, v)
	}

	//value, exists := d.Lookup(k) // not what we want, but would work!
	value, exists := d.Lookup(&StrValue{V: "answer"}) // different pointer!
	if !exists {
		t.Errorf("map did not contain our key")
		return
	}

	if err := value.Cmp(&IntValue{V: 42}); err != nil {
		t.Errorf("value did not match our map key")
	}
}

func TestStruct1(t *testing.T) {
	s := NewStruct(NewType("struct{answer int; truth bool; hello str; nested []int}"))
	if err := s.Set("answer", &IntValue{V: 42}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
	}

	if _, exists := s.Lookup("missing"); exists {
		t.Errorf("struct incorrectly contained our field")
		return
	}

	value, exists := s.Lookup("answer") // different pointer!
	if !exists {
		t.Errorf("struct did not contain our field")
		return
	}

	if err := value.Cmp(&IntValue{V: 42}); err != nil {
		t.Errorf("value did not match our struct field")
	}
}

func TestStruct2(t *testing.T) {
	st := NewStruct(NewType("struct{Answer int; Truth bool; Hello str; Nested []int}"))
	if err := st.Set("Answer", &IntValue{V: 42}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	if err := st.Set("Truth", &BoolValue{V: true}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	v := st.Value()
	//v.Answer = -13 // won't work, this is an interface!
	if val := fmt.Sprintf("%+v", v); val != `{Answer:42 Truth:true Hello: Nested:[]}` {
		t.Errorf("struct displayed wrong value: %s", val)
	}
	if typ := fmt.Sprintf("%T", v); typ != `struct { Answer int64; Truth bool; Hello string; Nested []int64 }` {
		t.Errorf("struct displayed type value: %s", typ)
	}

	// show that golang structs are ambiguous
	st2 := NewStruct(NewType("struct{Answer str}"))
	if err := st2.Set("Answer", &StrValue{V: "42 Truth:true Hello: Nested:[]"}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
		return
	}
	v2 := st2.Value()

	if val1, val2 := fmt.Sprintf("%+v", v), fmt.Sprintf("%+v", v2); val1 == val2 {
		t.Logf("golang structs are ambiguous")
	} else {
		//t.Errorf("golang structs are broken ?")
		//t.Errorf("val1: %s", val1)
		//t.Errorf("val2: %s", val2)
	}
}

func TestValueOf0(t *testing.T) {
	testCases := map[Value]interface{}{
		&BoolValue{V: true}:  true,
		&StrValue{V: "abc"}:  "abc",
		&IntValue{V: 4}:      4,
		&IntValue{V: -4}:     -4,
		&FloatValue{V: 9.87}: 9.87,
		&ListValue{
			T: NewType("[]int"),
			V: []Value{
				&IntValue{V: 1},
				&IntValue{V: 3},
				&IntValue{V: 5},
			},
		}: []int64{1, 3, 5},
		&MapValue{
			T: NewType("map{str: int}"),
			V: map[Value]Value{
				&StrValue{V: "a"}: &IntValue{V: 1},
				&StrValue{V: "b"}: &IntValue{V: 2},
				&StrValue{V: "c"}: &IntValue{V: 3},
			},
		}: map[string]int{"a": 1, "b": 2, "c": 3}, // go map ordering is alphabetically sorted
		&StructValue{
			T: NewType("struct{num int; name str}"),
			V: map[string]Value{
				"num":  &IntValue{V: 42},
				"name": &StrValue{V: "mgmt"},
			},
		}: struct {
			num  int
			name string
		}{42, "mgmt"},
		// TODO: implement ValueOf tests for TypeFunc
	}

	for value, gotyp := range testCases {
		// get reflect.Value, then call ValueOf() for types.Value
		val, err := ValueOf(reflect.ValueOf(gotyp))
		if err != nil {
			t.Errorf("function ValueOf(%+v) returned err %s", gotyp, err)
			continue
		}
		// use string representation comparison as maps are non-deterministic in order
		// and cmp doesn't work as the pointers differ
		if val.String() != value.String() {
			t.Errorf("function ValueOf(%+v) gave %+v and doesn't match expected %+v", gotyp, val, value)
			continue
		}
	}
}

func TestValueOf1(t *testing.T) {
	str := "abc"
	pstr := &str
	value := &StrValue{V: "abc"}

	val, err := ValueOf(reflect.ValueOf(pstr))
	if err != nil {
		t.Errorf("function ValueOf(%+v) returned err %s", pstr, err)
		return
	}
	if val.String() != value.String() {
		t.Errorf("function ValueOf(%+v) gave %+v and doesn't match expected %+v", pstr, val, value)
		return
	}
}

func TestValueOf2(t *testing.T) {
	str := "point"
	pstr := &str

	str2 := "point2"
	pstr2 := &str2
	ppstr2 := &pstr2
	st := struct {
		Num  int      `lang:"num"`
		Name string   `lang:"name"`
		Ptr  *string  `lang:"ptr"`
		Ptr2 **string `lang:"ptr2"`
	}{42, "mgmt", pstr, ppstr2}

	value := &StructValue{
		T: NewType("struct{num int; name str; ptr str; ptr2 str}"),
		V: map[string]Value{
			"num":  &IntValue{V: 42},
			"name": &StrValue{V: "mgmt"},
			"ptr":  &StrValue{V: "point"},
			"ptr2": &StrValue{V: "point2"},
		},
	}

	val, err := ValueOf(reflect.ValueOf(st))
	if err != nil {
		t.Errorf("function ValueOf(%+v) returned err %s", st, err)
		return
	}
	if val.String() != value.String() {
		t.Errorf("function ValueOf(%+v) gave %+v and doesn't match expected %+v", st, val, value)
		return
	}
}

func TestValueOf3(t *testing.T) {
	st := struct {
		Ptr  *string  `lang:"ptr"`
		Ptr2 **string `lang:"ptr2"`
	}{nil, nil}

	// cannot represent nil pointers, expect an err
	val, err := ValueOf(reflect.ValueOf(st))
	if err == nil {
		t.Errorf("function ValueOf(%+v) returned a Value when error was expected: %s", st, val)
		return
	}
	if !errors.Is(err, ErrNilValue) {
		t.Errorf("function ValueOf(%+v) returned err %s but types.ErrNilValue was expected", st, err)
	}
}

func TestValueOf4(t *testing.T) {
	str := "" // test with empty values
	pstr := &str

	str2 := ""
	pstr2 := &str2
	ppstr2 := &pstr2
	st := struct {
		Num  int      `lang:"num"`
		Name string   `lang:"name"`
		Ptr  *string  `lang:"ptr"`
		Ptr2 **string `lang:"ptr2"`
	}{0, "", pstr, ppstr2}

	value := &StructValue{
		T: NewType("struct{num int; name str; ptr str; ptr2 str}"),
		V: map[string]Value{
			"num":  &IntValue{V: 0},
			"name": &StrValue{V: ""},
			"ptr":  &StrValue{V: ""},
			"ptr2": &StrValue{V: ""},
		},
	}

	val, err := ValueOf(reflect.ValueOf(st))
	if err != nil {
		t.Errorf("function ValueOf(%+v) returned err %s", st, err)
		return
	}
	if val.String() != value.String() {
		t.Errorf("function ValueOf(%+v) gave %+v and doesn't match expected %+v", st, val, value)
		return
	}
}

func TestValueInto0(t *testing.T) {
	// converts a Go variable to a types.Value, or panics if any error
	mustValue := func(v interface{}) Value {
		val, err := ValueOfGolang(v)
		if err != nil {
			panic(err)
		}
		return val
	}
	// reflect variant of & on a variable. Creates a pointer in
	// memory, sets the destination, then returns the pointer
	ptrto := func(v interface{}) interface{} {
		p := reflect.New(reflect.TypeOf(v))
		p.Elem().Set(reflect.ValueOf(v))
		return p.Interface()
	}
	ptrstr := func(s string) *string {
		return ptrto(s).(*string)
	}

	// various container variables for below tests
	var b bool
	var s string

	var i int64
	var u uint64
	var i8 int8
	var u8 uint8

	var f float64

	var l []string
	var ll [][]string
	var lptrlptr []*[]*string
	var arr [3]string

	var m map[string]int

	type str1 struct {
		X string
		Y int
	}
	var ms map[string]str1

	var mptr map[string]*string
	var msptr map[string]*str1

	type str2 struct {
		X *string
		Y *int
	}
	var mptrsptr map[string]*str2

	var testCases = []struct {
		// backing container to call Into() on
		container interface{}
		// lang value to be Into()ed
		value Value
		// test comparison data to ensure the Into() worked
		compare interface{}
		// shouldErr set to true if an err is expected
		shouldErr bool
		// shouldPanic set to true if a panic is expected
		shouldPanic bool
	}{
		{
			container: &b,
			value:     mustValue(true),
			compare:   true,
		},
		{
			container: &s,
			value:     mustValue("testing"),
			compare:   "testing",
		},
		{
			container: &i,
			value:     mustValue(int64(-12345)),
			compare:   int64(-12345),
		},
		{
			container: &u,
			value:     mustValue(uint64(math.MaxUint64)),
			compare:   uint64(math.MaxUint64),
		},
		{ // ensure -1 from an int64 fits into an int8
			container: &i8,
			value:     mustValue(int64(-1)),
			compare:   int8(-1),
		},
		{ // ensure valid uint8 from an int64 fits into an uint8
			container: &u8,
			value:     mustValue(int64(200)),
			compare:   uint8(200),
		},
		{ // this test case proves overflows work
			container: &u8,
			value:     mustValue(int64(256)),
			shouldErr: true,
		},
		{ // it would be good to put float32 -> float64 here but precision says no
			container: &f,
			value:     mustValue(float64(1.23)),
			compare:   float64(1.23),
		},
		{
			container: &l,
			value:     mustValue([]string{"1", "2", "3"}),
			compare:   []string{"1", "2", "3"},
		},
		{ // arrays are pretty much the same as slices
			container: &arr,
			value:     mustValue([]string{"1", "2", "3"}),
			compare:   [3]string{"1", "2", "3"},
		},
		{
			container: &ll,
			value:     mustValue([][]string{{"1"}, {"2"}}),
			compare:   [][]string{{"1"}, {"2"}},
		},
		{
			container: &m,
			value:     mustValue(map[string]int{"1": 1, "2": 2}),
			compare:   map[string]int{"1": 1, "2": 2},
		},
		{
			container: &ms,
			value:     mustValue(map[string]str1{"a": {"a", 97}, "b": {"b", 98}}),
			compare:   map[string]str1{"a": {"a", 97}, "b": {"b", 98}},
		},

		// Various error sanity tests. All of these tests should return type errors.
		{ // int into string
			container: &s,
			value:     mustValue(12345),
			shouldErr: true,
		},
		{ // string into int
			container: &i,
			value:     mustValue("hello"),
			shouldErr: true,
		},
		{ // map[int]int into map[string]int
			container: &m,
			value:     mustValue(map[int]int{1: 2, 3: 4}),
			shouldErr: true,
		},
		{ // [4]string  into [3]string
			container: &arr,
			value:     mustValue([4]string{"1", "2", "3", "4"}),
			shouldErr: true,
		},

		// Pointer and pointer-to-pointer tests
		{
			container: ptrto(&s),
			value:     mustValue("pointer to a string"),
			compare:   ptrto("pointer to a string"),
		},
		{
			container: ptrto(ptrto(&s)),
			value:     mustValue("pointer to a pointer to a string"),
			compare:   ptrto(ptrto("pointer to a pointer to a string")),
		},
		{ // tests Into() instantiating the nil pointers in the map values before following/setting the values
			container: &mptr,
			value: mustValue(map[string]string{
				"first":  "firstptr",
				"second": "secondptr",
			}),
			compare: map[string]*string{
				"first":  ptrstr("firstptr"),
				"second": ptrstr("secondptr"),
			},
		},
		{
			container: &lptrlptr,
			value: mustValue([][]string{
				{"hello", "world"},
				{"hola", "món"},
			}),
			// List of pointers to lists of pointers to strings. Confused yet?
			compare: []*[]*string{
				{ptrstr("hello"), ptrstr("world")},
				{ptrstr("hola"), ptrstr("món")},
			},
		},
		{
			container: &msptr,
			value: mustValue(map[string]str1{
				"str1": {
					X: "str",
					Y: 98765,
				},
			}),
			compare: map[string]*str1{
				"str1": {
					X: "str",
					Y: 98765,
				},
			},
		},
		{ // Use str1 to try to Into() str2. They're field-compatible, except str2 uses pointers to values instead.
			container: &mptrsptr,
			value: mustValue(map[string]str1{
				"str2": {
					X: "string pointer",
					// cannot omit any fields, as doing so makes the comparison
					// fail because nil.(*int) != 0. Using .String() or coercing
					// both values to the same type for comparison might work
					Y: 555,
				},
			}),
			compare: map[string]*str2{
				"str2": {
					X: ptrstr("string pointer"),
					Y: ptrto(555).(*int),
				},
			},
		},
	}

	for index, tc := range testCases {
		name := fmt.Sprintf("test Into() %s #%d", reflect.TypeOf(tc.container).Elem(), index)
		// https://github.com/purpleidea/mgmt/pull/629/files#r568305689
		tc := tc
		t.Run(name, func(t *testing.T) {
			rvo := reflect.ValueOf(tc.container)

			if tc.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("function Into() didn't panic but it was expected")
					}
				}()
			}

			// Into() the value into the reflect.Value
			err := Into(tc.value, rvo)

			// check for non/expected errors
			if !tc.shouldErr && err != nil {
				t.Errorf("function Into() returned an error: %s", err)
				return
			} else if tc.shouldErr && err == nil {
				t.Errorf("function Into() didn't return an error but one was expected")
				return
			}

			if tc.shouldErr || tc.shouldPanic {
				// err/panic was expected. no comparison to do here
				return
			}

			// follow the container pointer: (*tc.container).(interface{})
			container := rvo.Elem().Interface()
			// ensure they're identical
			if !reflect.DeepEqual(container, tc.compare) {
				t.Errorf("result %s %+v doesn't match expected %s %+v",
					rvo.Elem().Type(), container, reflect.TypeOf(tc.compare), tc.compare,
				)
				return
			}
		})
	}
}

func TestValueInto1(t *testing.T) {
	testCases := []struct {
		container interface{}
		compare   interface{}
		data      Value
	}{
		{
			container: &[]string{"", "", "three"},
			compare:   []string{"one", "two"},
			data: &ListValue{
				T: NewType("[]str"),
				V: []Value{&StrValue{V: "one"}, &StrValue{V: "two"}},
			},
		},
		{
			container: &[3]string{"", "", "three"},
			compare:   [3]string{"one", "two", ""},
			data: &ListValue{
				T: NewType("[]str"),
				V: []Value{&StrValue{V: "one"}, &StrValue{V: "two"}},
			},
		},
		{
			container: &map[string]string{"3": "three"},
			compare:   map[string]string{"1": "one", "2": "two"},
			data: &MapValue{
				T: NewType("map{str: str}"),
				V: map[Value]Value{
					&StrValue{V: "1"}: &StrValue{V: "one"},
					&StrValue{V: "2"}: &StrValue{V: "two"},
				},
			},
		},
	}

	for index, tc := range testCases {
		name := fmt.Sprintf("test Into() %s #%d", reflect.TypeOf(tc.container).Elem(), index)

		tc := tc
		t.Run(name, func(t *testing.T) {
			ctrVal := reflect.ValueOf(tc.container)

			// ensure Into() clears existing elements out of the container
			if err := Into(tc.data, ctrVal); err != nil {
				t.Errorf("func Into(%+v, %+v) failed: %s", tc.data, tc.container, err)
				return
			}

			// follow the container pointer: (*tc.container).(interface{})
			ctrPtr := ctrVal.Elem().Interface()
			if !reflect.DeepEqual(ctrPtr, tc.compare) {
				t.Errorf("func Into(%+v, %+v) did not clear existing values from the list", tc.data, ctrPtr)
				return
			}
		})
	}
}

func TestValueIntoStructNameMapping(t *testing.T) {
	st := NewStruct(NewType("struct{word str; magic int}"))
	if err := st.Set("word", &StrValue{V: "zing"}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
	}
	if err := st.Set("magic", &IntValue{V: 0x5F3759DF}); err != nil {
		t.Errorf("struct could not set key, error: %v", err)
	}

	var compare struct {
		Word  string `lang:"word"`
		Magic int    `lang:"magic"`
	}
	err := Into(st, reflect.ValueOf(&compare))
	if err != nil {
		t.Errorf("function Into() returned an error: %s", err)
	}

	if compare.Word != "zing" || compare.Magic != 0x5F3759DF {
		t.Errorf("struct field value is missing or incorrect")
	}
}
