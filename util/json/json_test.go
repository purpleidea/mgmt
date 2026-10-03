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
	"testing"
)

func TestJSONComplex(t *testing.T) {
	type complexes struct {
		C64  complex64
		C128 complex128
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
		in := complexes{C64: complex64(x), C128: x}
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
		if !same(complex128(in.C64), complex128(out.C64)) || !same(in.C128, out.C128) {
			t.Errorf("round trip of %v differs, encoded as: %s", x, b)
		}
	}

	var out complexes
	if err := Unmarshal([]byte(`{"C128":"bad"}`), &out); err == nil {
		t.Errorf("expected an error decoding an invalid complex")
	}
}

func TestJSONFloat(t *testing.T) {
	type floats struct {
		F32 float32
		F64 float64
		P   *float64
		S   []float64
		M   map[string]float32
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
		if !same(float64(in.F32), float64(out.F32)) || !same(in.F64, out.F64) || out.P == nil || !same(*in.P, *out.P) || len(out.S) != 1 || !same(in.S[0], out.S[0]) || !same(float64(in.M["x"]), float64(out.M["x"])) {
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
