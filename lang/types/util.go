// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package types

import (
	"fmt"
	"reflect"
)

// nextPowerOfTwo gets the lowest number higher than v that is a power of two.
func nextPowerOfTwo(v uint) uint {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}

// TypeStructTagToFieldName returns a mapping from recommended alias to actual
// field name. It returns an error if it finds a collision. It uses the `lang`
// tags. It must be passed a reflect.Type representation of a struct or it will
// error.
// TODO: This is a copy of engineUtil.StructTagToFieldName taking a reflect.Type
func TypeStructTagToFieldName(st reflect.Type) (map[string]string, error) {
	if k := st.Kind(); k != reflect.Struct { // this should be a struct now
		return nil, fmt.Errorf("input doesn't point to a struct, got: %+v", k)
	}

	result := make(map[string]string) // `lang` field tag -> field name

	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		name := field.Name
		// if !ok, then nothing is found
		if alias, ok := field.Tag.Lookup(StructTag); ok { // golang 1.7+
			if val, exists := result[alias]; exists {
				return nil, fmt.Errorf("field `%s` uses the same key `%s` as field `%s`", name, alias, val)
			}
			// empty string ("") is a valid value
			if alias != "" {
				result[alias] = name
			}
		}
	}
	return result, nil
}

// IsComparableKind returns true if you pass it a comparable kind. These have a
// Cmp method on the Value interface that won't panic. Notably KindFunc and any
// other special kinds are not present in this list.
func IsComparableKind(kind Kind) bool {
	switch kind {
	case KindBool:
		return true
	case KindStr:
		return true
	case KindInt:
		return true
	case KindFloat:
		return true
	case KindList:
		return true
	case KindMap:
		return true
	case KindStruct:
		return true
	case KindFunc:
		return false // not comparable!
	}
	return false // others
}

// Iter applies a function to each type in the top-level type. It stops if that
// function errors, and returns that error to the top-level caller. It panics if
// it encounters an invalid or partial type struct. This version starts at the
// top and works its way deeper.
func Iter(typ *Type, fn func(*Type) error) error {
	if err := fn(typ); err != nil {
		return err
	}

	switch typ.Kind {
	case KindBool:
	case KindStr:
	case KindInt:
	case KindFloat:

	case KindList:
		if typ.Val == nil {
			panic("malformed list type")
		}
		if err := Iter(typ.Val, fn); err != nil {
			return err
		}

	case KindMap:
		if typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}
		if err := Iter(typ.Key, fn); err != nil {
			return err
		}
		if err := Iter(typ.Val, fn); err != nil {
			return err
		}

	case KindStruct: // {a bool; b int}
		if typ.Map == nil {
			panic("malformed struct type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed struct length")
		}
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}
			if err := Iter(t, fn); err != nil {
				return err
			}
		}

	case KindFunc:
		if typ.Map == nil {
			panic("malformed func type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed func length")
		}
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func field")
			}
			if err := Iter(t, fn); err != nil {
				return err
			}
		}
		//if typ.Out != nil {
		if err := Iter(typ.Out, fn); err != nil {
			return err
		}
		//}

	case KindVariant:
		if err := Iter(typ.Var, fn); err != nil {
			return err
		}

	case KindUnification:
		if typ.Uni == nil {
			panic("malformed unification variable")
		}
		// nothing to do

	default:
		panic("malformed type")
	}

	return nil
}

// NewUnifiedState builds a new unified state store.
func NewUnifiedState() *UnifiedState {
	return &UnifiedState{
		table: make(map[*Elem]uint),
	}
}

// UnifiedState stores a mapping of unification variable to unique id. This is
// most often used for printing consistent unification variables in your logs.
// It must be built with NewUnifiedState before it can be used or it will panic.
type UnifiedState struct {
	table map[*Elem]uint
}

// String returns a representation of the input type using the specified state.
func (obj *UnifiedState) String(typ *Type) string {
	return typ.string(obj.table)
}

// ListStrToValue is a simple helper function to convert from a list of strings
// in golang to the equivalent in our type system.
func ListStrToValue(input []string) Value {
	l := NewList(TypeListStr)
	for _, x := range input {
		v := &StrValue{
			V: x,
		}
		l.V = append(l.V, v) // be more efficient than using .Add(...)
	}
	return l
}
