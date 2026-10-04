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
	"math"
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"

	"golang.org/x/time/rate"
)

func TestJSONComplex(t *testing.T) {
	type named complex64 // a named type must work too
	type complexes struct {
		C64  complex64
		C128 complex128
		N    named
	}
	inf, nan := math.Inf(1), math.NaN()
	values := []complex128{
		0,
		complex(math.Copysign(0, -1), 0),
		1 + 2i,
		complex(-3, 0.25),
		complex(math.MaxFloat32, math.SmallestNonzeroFloat32),
		complex(math.MaxFloat64, math.SmallestNonzeroFloat64),
		complex(inf, -inf),
		complex(nan, 1),
	}
	// same compares by bits, so that NaN and -0 are checked exactly
	same := func(a, b complex128) bool {
		return math.Float64bits(real(a)) == math.Float64bits(real(b)) &&
			math.Float64bits(imag(a)) == math.Float64bits(imag(b))
	}
	for _, x := range values {
		in := complexes{C64: complex64(x), C128: x, N: named(x)}
		b, err := Marshal(in)
		if err != nil {
			t.Errorf("func Marshal(%v): %v", x, err)
			continue
		}
		var out complexes
		if err := Unmarshal(b, &out); err != nil {
			t.Errorf("func Unmarshal(%s): %v", b, err)
			continue
		}
		if !same(complex128(in.C64), complex128(out.C64)) || !same(in.C128, out.C128) || !same(complex128(in.N), complex128(out.N)) {
			t.Errorf("round trip of %v differs, encoded as: %s", x, b)
		}
	}

	var out complexes
	if err := Unmarshal([]byte(`{"C128":"bad"}`), &out); err == nil {
		t.Errorf("expected an error decoding an invalid complex")
	}
}

func TestJSONFloat(t *testing.T) {
	type named float32 // a named type must work too
	type floats struct {
		F32 float32
		F64 float64
		P   *float64
		S   []float64
		M   map[string]float32
		N   named
		L   rate.Limit // the named float type in the meta params
	}
	values := []float64{
		0,
		math.Copysign(0, -1),
		1.5,
		math.MaxFloat32,
		math.SmallestNonzeroFloat64,
		math.Inf(1),
		math.Inf(-1),
		math.NaN(),
	}
	// same compares by bits, so that NaN and -0 are checked exactly
	same := func(a, b float64) bool {
		return math.Float64bits(a) == math.Float64bits(b)
	}
	for _, x := range values {
		p := x
		in := floats{
			F32: float32(x),
			F64: x,
			P:   &p,
			S:   []float64{x},
			M:   map[string]float32{"x": float32(x)},
			N:   named(x),
			L:   rate.Limit(x),
		}
		b, err := Marshal(in)
		if err != nil {
			t.Errorf("func Marshal(%v): %v", x, err)
			continue
		}
		var out floats
		if err := Unmarshal(b, &out); err != nil {
			t.Errorf("func Unmarshal(%s): %v", b, err)
			continue
		}
		if !same(float64(in.F32), float64(out.F32)) || !same(in.F64, out.F64) || out.P == nil || !same(*in.P, *out.P) || len(out.S) != 1 || !same(in.S[0], out.S[0]) || !same(float64(in.M["x"]), float64(out.M["x"])) || !same(float64(in.N), float64(out.N)) || !same(float64(in.L), float64(out.L)) {
			t.Errorf("round trip of %v differs, encoded as: %s", x, b)
		}
	}

	// finite floats should stay as json numbers
	for x, expected := range map[float64]string{
		1.5:          `{"F":1.5}`,
		math.Inf(1):  `{"F":"Infinity"}`,
		math.Inf(-1): `{"F":"-Infinity"}`,
	} {
		b, err := Marshal(struct{ F float64 }{F: x})
		if err != nil {
			t.Errorf("func Marshal(%v): %v", x, err)
			continue
		}
		if s := string(b); s != expected {
			t.Errorf("expected %v to encode as %s, got: %s", x, expected, s)
		}
	}

	for _, s := range []string{`{"F64":"1.5"}`, `{"F64":"nan"}`, `{"F32":"Inf"}`} {
		var out floats
		if err := Unmarshal([]byte(s), &out); err == nil {
			t.Errorf("expected an error decoding: %s", s)
		}
	}
}

func TestJSONInterface(t *testing.T) {
	type holder struct {
		I interface{}
		P *interface{}
		L []interface{}
	}

	// a struct value, with the golang type that lang uses for it
	st := reflect.New(types.NewType("struct{a int; b str}").Reflect()).Elem()
	st.Field(0).SetInt(42)
	st.Field(1).SetString("hello")

	values := []interface{}{
		nil,
		true,
		"hello",
		"",
		int64(0),
		int64(math.MaxInt64), // too big for a float64
		1.5,
		math.Inf(-1), // needs our float encoding inside of the interface
		[]string{},
		[]string(nil),
		[]int64{1, 2},
		map[string]int64{"a": 1},
		map[int64]string{2: "b"},
		[]map[string]float64{{"x": math.Inf(1)}},
		st.Interface(),
	}
	for _, x := range values {
		in := holder{I: x, L: []interface{}{x}}
		if x != nil { // a pointer to a nil interface is encoded as null
			p := x
			in.P = &p
		}
		b, err := Marshal(in)
		if err != nil {
			t.Errorf("func Marshal(%#v): %v", x, err)
			continue
		}
		var out holder
		if err := Unmarshal(b, &out); err != nil {
			t.Errorf("func Unmarshal(%s): %v", b, err)
			continue
		}
		if !reflect.DeepEqual(in, out) {
			t.Errorf("round trip of %#v differs, encoded as: %s", x, b)
		}
	}

	// golang types which aren't exactly the golang type of a lang type
	for _, x := range []interface{}{
		int(1),
		uint8(1),
		[]interface{}{"a"},
		func() {},
	} {
		if b, err := Marshal(holder{I: x}); err == nil {
			t.Errorf("expected an error encoding %T, got: %s", x, b)
		}
	}

	for _, s := range []string{
		`{"I":42}`,
		`{"I":{"value":42,"type":"int"}}`,
		`{"I":{"type":"int","value":42,"extra":1}}`,
		`{"I":{"type":"int","value":"42"}}`,
		`{"I":{"type":"nope","value":42}}`,
		`{"I":{"type":"variant","value":42}}`,
		`{"I":{"type":"[]variant","value":[42]}}`,
	} {
		var out holder
		if err := Unmarshal([]byte(s), &out); err == nil {
			t.Errorf("expected an error decoding %s, got: %#v", s, out)
		}
	}
}

func TestJSONMap(t *testing.T) {
	// a map with struct keys, with the golang type that lang uses for it
	st := types.NewType("struct{a int; b str}").Reflect()
	key1 := reflect.New(st).Elem()
	key1.Field(0).SetInt(1)
	key1.Field(1).SetString("x")
	key2 := reflect.New(st).Elem()
	key2.Field(0).SetInt(2)
	structs := reflect.MakeMap(reflect.MapOf(st, reflect.TypeOf(true)))
	structs.SetMapIndex(key1, reflect.ValueOf(true))
	structs.SetMapIndex(key2, reflect.ValueOf(false))

	values := []struct {
		x    interface{}
		lang bool // is this a lang type, which can be in an interface?
	}{
		{map[bool]string{true: "a", false: "b"}, true},
		{map[bool]string{}, true},
		{map[bool]string(nil), true},
		{map[bool]float64{true: math.Inf(1)}, true}, // our float encoding
		{map[bool][]string{true: {}, false: nil}, true},
		{structs.Interface(), true},
		{map[[2]int64]string{{1, 2}: "a"}, false},
		{map[interface{}]string{"x": "a", int64(1): "b", nil: "c"}, false},
	}
	for _, v := range values {
		ins := []interface{}{v.x}
		if v.lang { // also check it in an interface, as lang would set it
			ins = append(ins, &struct{ I interface{} }{I: v.x})
		}
		for _, in := range ins {
			b, err := Marshal(in)
			if err != nil {
				t.Errorf("func Marshal(%#v): %v", in, err)
				continue
			}
			out := reflect.New(reflect.TypeOf(in))
			if err := Unmarshal(b, out.Interface()); err != nil {
				t.Errorf("func Unmarshal(%s): %v", b, err)
				continue
			}
			if !reflect.DeepEqual(in, out.Elem().Interface()) {
				t.Errorf("round trip of %#v differs, encoded as: %s", in, b)
			}
		}
	}

	// the pairs are sorted, and maps with string keys are still objects
	for x, expected := range map[string]interface{}{
		`[[false,"b"],[true,"a"]]`: map[bool]string{true: "a", false: "b"},
		`[]`:                       map[bool]string{},
		`null`:                     map[bool]string(nil),
		`{"a":true,"b":false}`:     map[string]bool{"b": false, "a": true},
	} {
		b, err := Marshal(expected)
		if err != nil {
			t.Errorf("func Marshal(%#v): %v", expected, err)
			continue
		}
		if s := string(b); s != x {
			t.Errorf("expected %#v to encode as %s, got: %s", expected, x, s)
		}
	}

	for _, s := range []string{
		`[[true,"a"],[true,"b"]]`, // duplicate key
		`{"true":"a"}`,
		`[[true]]`,
		`[[true,"a","b"]]`,
		`[true,"a"]`,
		`[["true","a"]]`,
	} {
		var out map[bool]string
		if err := Unmarshal([]byte(s), &out); err == nil {
			t.Errorf("expected an error decoding %s, got: %#v", s, out)
		}
	}
}
