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

package resources

import (
	"math"
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	utilJSON "github.com/purpleidea/mgmt/util/json"
)

// TestResToB64RoundTrip encodes every registered resource kind with ResToB64,
// with non-default values in all of its trait meta params, and checks that
// decoding it with B64ToRes gives back an identical resource. This catches
// exported trait fields which collide when an encoder flattens the embedded
// traits into the resource, since those get silently dropped. It also checks
// that every field type in use, such as the complex numbers and the infinite
// floats of the test resource, can be encoded. Each kind is checked a second
// time with all of its nil pointer, slice and map fields set to a pointer to a
// zero value, or empty, since those must not come back as nil.
func TestResToB64RoundTrip(t *testing.T) {
	for _, kind := range engine.RegisteredResourcesNames() {
		for _, empty := range []bool{false, true} {
			name := kind + "/nil"
			if empty {
				name = kind + "/empty"
			}
			t.Run(name, func(t *testing.T) {
				res, err := engine.NewNamedResource(kind, "probe")
				if err != nil {
					t.Fatalf("func NewNamedResource: %v", err)
				}
				res = res.Default()
				res.SetKind(kind)
				res.SetName("probe")

				meta := engine.DefaultMetaParams.Copy()
				meta.Retry = 3
				meta.Sema = []string{"a:1"}
				meta.Export = []string{"*"}
				res.SetMetaParams(meta)
				if r, ok := res.(engine.EdgeableRes); ok {
					r.SetAutoEdgeMeta(&engine.AutoEdgeMeta{Disabled: true})
				}
				if r, ok := res.(engine.GroupableRes); ok {
					r.SetAutoGroupMeta(&engine.AutoGroupMeta{Disabled: true})
				}
				if r, ok := res.(engine.ReversibleRes); ok {
					r.SetReversibleMeta(&engine.ReversibleMeta{
						Disabled:  true,
						Reversal:  true,
						Overwrite: true,
					})
				}
				if r, ok := res.(*TestRes); ok {
					r.Complex64 = 1.5 + 2i
					r.Complex128 = -3 + 0.25i
					r.Float32 = float32(math.Inf(-1))
					r.Float64 = math.Inf(1)
				}
				if empty {
					setEmpty(res)
				}

				str, err := engineUtil.ResToB64(res)
				if err != nil {
					t.Fatalf("func ResToB64: %v", err)
				}
				out, err := engineUtil.B64ToRes(str)
				if err != nil {
					t.Fatalf("func B64ToRes: %v", err)
				}
				if !reflect.DeepEqual(res, out) {
					b1, _ := utilJSON.Marshal(res)
					b2, _ := utilJSON.Marshal(out)
					t.Errorf("round trip differs:\nin:  %s\nout: %s", b1, b2)
				}
			})
		}
	}
}

// setEmpty sets each nil pointer, slice and map field of the resource to an
// empty value of that type.
func setEmpty(res engine.Res) {
	v := reflect.ValueOf(res).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() { // private
			continue
		}
		switch f.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
			if !f.IsNil() {
				continue
			}
			if x, ok := emptyValue(f.Type()); ok {
				f.Set(x)
			}
		}
	}
}

// emptyValue returns a non-nil but empty value of the pointer, slice or map
// type, such as a pointer to a zero value, or a pointer to an empty slice. It
// returns false for a pointer to an interface, since a pointer to a nil one
// can't be told apart from a nil pointer by json (or gob).
func emptyValue(typ reflect.Type) (reflect.Value, bool) {
	switch typ.Kind() {
	case reflect.Slice:
		return reflect.MakeSlice(typ, 0, 0), true
	case reflect.Map:
		return reflect.MakeMap(typ), true
	case reflect.Pointer:
		ptr := reflect.New(typ.Elem())
		switch typ.Elem().Kind() {
		case reflect.Interface:
			return reflect.Value{}, false
		case reflect.Pointer, reflect.Slice, reflect.Map:
			x, ok := emptyValue(typ.Elem())
			if !ok {
				return reflect.Value{}, false
			}
			ptr.Elem().Set(x)
		}
		return ptr, true
	}
	return reflect.Value{}, false
}
