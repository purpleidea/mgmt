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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package util

import (
	"reflect"
	"strings"
)

// LookupSubcommand returns the name of the subcommand in the obj, of a struct.
// This is useful for determining the name of the subcommand that was activated.
// It returns an empty string if a specific name was not found.
func LookupSubcommand(obj interface{}, st interface{}) string {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr { // max one de-referencing
		val = val.Elem()
	}

	v := reflect.ValueOf(st) // value of the struct
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := val.Field(i) // value of the field
		if f.Interface() != v.Interface() {
			continue
		}

		field := typ.Field(i)
		alias, ok := field.Tag.Lookup("arg")
		if !ok {
			continue
		}

		// XXX: `arg` needs a split by comma first or fancier parsing
		prefix := "subcommand"
		split := strings.Split(alias, ":")
		if len(split) != 2 || split[0] != prefix {
			continue
		}

		return split[1] // found
	}
	return "" // not found
}
