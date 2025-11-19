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

package json

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/types"
)

// ValueOfJSON takes a string containing some JSON, and an expected type, and if
// the JSON matches that type, returns the equivalent types.Value matching it.
func ValueOfJSON(data string, typ *types.Type) (types.Value, error) {
	// Unmarshal into interface{}
	var v interface{}
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber() // to preserve number precision
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return convertJSON(v, typ, false)
}

// FlexibleValueOfJSON takes a string containing some JSON, and an expected
// type, and if the JSON matches that type, returns the equivalent types.Value
// matching it. This variant differs from ValueOfJSON in that if a struct field
// is present in the type, but missing from the data, then it will substitute a
// zero value for the data instead of erroring.
func FlexibleValueOfJSON(data string, typ *types.Type) (types.Value, error) {
	// Unmarshal into interface{}
	var v interface{}
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber() // to preserve number precision
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return convertJSON(v, typ, true)
}

// convertJSON is the recursive helper that takes the parsed json data and the
// expected type. If you specify flexible, then it allows missing fields in the
// data. This API may change if we add more modifiers in the future.
func convertJSON(val interface{}, typ *types.Type, flexible bool) (types.Value, error) {
	if typ == nil {
		// TODO: attempt to guess the type and hope it's not ambiguous?
		return nil, fmt.Errorf("type is nil")
	}

	switch typ.Kind {
	case types.KindNil:
		return nil, fmt.Errorf("nil type")

	case types.KindBool:
		v, ok := val.(bool)
		if !ok {
			return nil, fmt.Errorf("type doesn't match bool")
		}
		return &types.BoolValue{V: v}, nil

	case types.KindStr:
		v, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("type doesn't match str")
		}
		return &types.StrValue{V: v}, nil

	case types.KindInt:
		// TODO: do we need uint, int, and so on?
		num, ok := val.(json.Number)
		if !ok {
			return nil, fmt.Errorf("type doesn't match int")
		}
		v, err := num.Int64()
		if err != nil {
			return nil, fmt.Errorf("num doesn't contain int64")
		}

		return &types.IntValue{V: v}, nil

	case types.KindFloat:
		num, ok := val.(json.Number)
		if !ok {
			return nil, fmt.Errorf("type doesn't match float")
		}
		v, err := num.Float64()
		if err != nil {
			return nil, fmt.Errorf("num doesn't contain float64")
		}

		return &types.FloatValue{V: v}, nil

	case types.KindList:
		v, ok := val.([]interface{})
		if !ok {
			return nil, fmt.Errorf("type doesn't match list")
		}

		if typ.Val == nil {
			panic("malformed list type")
		}

		values := []types.Value{}
		for _, x := range v {
			vv, err := convertJSON(x, typ.Val, flexible) // recurse
			if err != nil {
				return nil, err
			}
			values = append(values, vv)
		}

		return &types.ListValue{
			T: typ, // this is a []?1
			V: values,
		}, nil

	case types.KindMap:
		// Amazingly, json only supports string keys!
		v, ok := val.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("type doesn't match map")
		}

		if typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}

		m := make(map[types.Value]types.Value)
		// loop through the list of map keys in undefined order
		for k, x := range v {
			kk, err := convertJSON(k, typ.Key, flexible) // recurse
			if err != nil {
				return nil, err
			}
			vv, err := convertJSON(x, typ.Val, flexible) // recurse
			if err != nil {
				return nil, err
			}

			m[kk] = vv
		}

		return &types.MapValue{
			T: typ,
			V: m,
		}, nil

	case types.KindStruct:
		v, ok := val.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("type doesn't match struct")
		}

		if !flexible && len(v) != len(typ.Ord) {
			// programming error?
			return nil, fmt.Errorf("incompatible number of fields")
		}

		// TODO: do we need this check? What about empty structs?
		if typ.Map == nil {
			panic("malformed struct type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed struct length")
		}

		m := make(map[string]types.Value)
		for _, x := range typ.Ord {
			t, ok := typ.Map[x]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}

			xx, exists := v[x]
			if !exists && !flexible {
				return nil, fmt.Errorf("field %s was not found in struct", x)
			}
			if !exists {
				m[x] = t.New() // zero value
				continue
			}

			vv, err := convertJSON(xx, t, flexible) // recurse
			if err != nil {
				return nil, err
			}
			m[x] = vv
		}

		return &types.StructValue{
			T: typ,
			V: m,
		}, nil

	case types.KindFunc:
		return nil, fmt.Errorf("func type")

	case types.KindVariant:
		return nil, fmt.Errorf("variant type")

	case types.KindUnification:
		panic("can't make new value from unification variable kind")

	}

	panic("malformed type")
}
