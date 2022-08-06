// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package types

import (
	"fmt"
	"reflect"
)

// nextPowerOfTwo gets the lowest number higher than v that is a power of two.
func nextPowerOfTwo(v uint32) uint32 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
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
