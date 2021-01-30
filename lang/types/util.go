// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// LangFieldNameToStructFieldName returns the mapping from lang (AST) field
// names to field name as used in the struct. The logic here is a bit strange;
// if the resource has struct tags, then it uses those, otherwise it falls back
// to using the lower case versions of things. It might be clever to combine the
// two so that tagged fields are used as such, and others are used in lowercase,
// but this is currently not implemented.
// TODO: should this behaviour be changed?
func LangFieldNameToStructFieldName(st reflect.Type) (map[string]string, error) {
	mapping, err := TypeStructTagToFieldName(st)
	if err != nil {
		return nil, errwrap.Wrapf(err, "resource kind `%s` has bad field mapping", st.Kind())
	}
	if len(mapping) == 0 { // if no `lang` tags exist, get them automatically
		mapping, err = LowerStructFieldNameToFieldName(st)
		if err != nil {
			return nil, errwrap.Wrapf(err, "resource kind `%s` has bad automatic field mapping", st.Kind())
		}
	}

	return mapping, nil // lang field name -> field name
}

// LowerStructFieldNameToFieldName returns a mapping from the lower case version
// of each field name to the actual field name. It only returns public fields.
// It returns an error if it finds a collision.
func LowerStructFieldNameToFieldName(st reflect.Type) (map[string]string, error) {
	result := make(map[string]string) // lower field name -> field name
	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		name := field.Name

		if strings.Title(name) != name { // must have been a priv field
			continue
		}

		if alias := strings.ToLower(name); alias != "" {
			if val, exists := result[alias]; exists {
				return nil, fmt.Errorf("field `%s` uses the same key `%s` as field `%s`", name, alias, val)
			}
			result[alias] = name
		}
	}
	return result, nil
}

// StructKindToFieldNameTypeMap returns a map from field name to expected type
// in the lang type system.
func StructKindToFieldNameTypeMap(st reflect.Type) (map[string]*Type, error) {
	if k := st.Kind(); k != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got: %s", k)
	}

	result := make(map[string]*Type)

	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		name := field.Name
		// TODO: in future, skip over fields that don't have a `lang` tag
		// if name == "Base" { // TODO: hack!!!
		//	continue
		// }

		typ, err := TypeOf(field.Type)
		// some types (eg complex64) aren't convertible, so skip for now...
		if err != nil {
			continue
			// return nil, errwrap.Wrapf(err, "could not identify type of field `%s`", name)
		}
		result[name] = typ
	}

	return result, nil
}

// TypeStructTagToFieldName returns a mapping from recommended alias to actual
// field name. It returns an error if it finds a collision. It uses the `lang`
// tags.
// It must be passed a reflect.Type representation of a struct or it will error.
func TypeStructTagToFieldName(st reflect.Type) (map[string]string, error) {
	if k := st.Kind(); k != reflect.Struct { // this should be a struct now
		return nil, fmt.Errorf("input doesn't point to a struct, got: %+v", k)
	}

	result := make(map[string]string) // `lang` field tag -> field name

	// TODO: fallback to looking up yaml tags, although harder to parse
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

// StructTagToFieldName returns a mapping from recommended alias to actual field
// name. It returns an error if it finds a collision. It uses the `lang` tags.
// It must be passed a ptr to a struct or it will error.
func StructTagToFieldName(stptr interface{}) (map[string]string, error) {
	if stptr == nil {
		return nil, fmt.Errorf("got nil input instead of ptr to struct")
	}
	typ := reflect.TypeOf(stptr)
	if k := typ.Kind(); k != reflect.Ptr { // we only look at *Struct's
		return nil, fmt.Errorf("input is not a ptr, got: %+v", k)
	}
	return TypeStructTagToFieldName(typ.Elem()) // elem for ptr to struct (dereference the pointer)
}

// StructFieldCompat returns whether a send struct and key is compatible with a
// recv struct and key. This inputs must both be a ptr to a string, and a valid
// key that can be found in the struct tag.
// TODO: add a bool to decide if *string to string or string to *string is okay.
func StructFieldCompat(st1 interface{}, key1 string, st2 interface{}, key2 string) error {
	m1, err := StructTagToFieldName(st1)
	if err != nil {
		return err
	}
	k1, exists := m1[key1]
	if !exists {
		return fmt.Errorf("key not found in send struct")
	}

	m2, err := StructTagToFieldName(st2)
	if err != nil {
		return err
	}
	k2, exists := m2[key2]
	if !exists {
		return fmt.Errorf("key not found in recv struct")
	}

	obj1 := reflect.Indirect(reflect.ValueOf(st1))
	// type1 := obj1.Type()
	value1 := obj1.FieldByName(k1)
	kind1 := value1.Kind()

	obj2 := reflect.Indirect(reflect.ValueOf(st2))
	// type2 := obj2.Type()
	value2 := obj2.FieldByName(k2)
	kind2 := value2.Kind()

	if kind1 != kind2 {
		return fmt.Errorf("kind mismatch between %s and %s", kind1, kind2)
	}

	if t1, t2 := value1.Type(), value2.Type(); t1 != t2 {
		return fmt.Errorf("type mismatch between %s and %s", t1, t2)
	}

	if !value2.CanSet() { // if we can't set, then this is pointless!
		return fmt.Errorf("can't set")
	}

	// if we can't interface, we can't compare...
	if !value1.CanInterface() {
		return fmt.Errorf("can't interface the send")
	}
	if !value2.CanInterface() {
		return fmt.Errorf("can't interface the recv")
	}

	return nil
}
