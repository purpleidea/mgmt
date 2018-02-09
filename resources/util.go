// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package resources

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"os/user"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/lang/types"

	errwrap "github.com/pkg/errors"
)

const (
	// StructTag is the key we use in struct field names for key mapping.
	StructTag = "lang"
)

// ResourceSlice is a linear list of resources. It can be sorted.
type ResourceSlice []Res

func (rs ResourceSlice) Len() int           { return len(rs) }
func (rs ResourceSlice) Swap(i, j int)      { rs[i], rs[j] = rs[j], rs[i] }
func (rs ResourceSlice) Less(i, j int) bool { return rs[i].String() < rs[j].String() }

// Sort the list of resources and return a copy without modifying the input.
func Sort(rs []Res) []Res {
	resources := []Res{}
	for _, r := range rs { // copy
		resources = append(resources, r)
	}
	sort.Sort(ResourceSlice(resources))
	return resources
	// sort.Sort(ResourceSlice(rs)) // this is wrong, it would modify input!
	//return rs
}

// ResToB64 encodes a resource to a base64 encoded string (after serialization).
func ResToB64(res Res) (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(&res) // pass with &
	if err != nil {
		return "", errwrap.Wrapf(err, "gob failed to encode")
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// B64ToRes decodes a resource from a base64 encoded string (after deserialization).
func B64ToRes(str string) (Res, error) {
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, errwrap.Wrapf(err, "base64 failed to decode")
	}
	b := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b)
	if err := d.Decode(&output); err != nil { // pass with &
		return nil, errwrap.Wrapf(err, "gob failed to decode")
	}
	res, ok := output.(Res)
	if !ok {
		return nil, fmt.Errorf("output `%v` is not a Res", output)
	}
	return res, nil
}

// StructTagToFieldName returns a mapping from recommended alias to actual field
// name. It returns an error if it finds a collision. It uses the `lang` tags.
func StructTagToFieldName(res Res) (map[string]string, error) {
	// TODO: fallback to looking up yaml tags, although harder to parse
	result := make(map[string]string) // `lang` field tag -> field name
	st := reflect.TypeOf(res).Elem()  // elem for ptr to res
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

// LowerStructFieldNameToFieldName returns a mapping from the lower case version
// of each field name to the actual field name. It only returns public fields.
// It returns an error if it finds a collision.
func LowerStructFieldNameToFieldName(res Res) (map[string]string, error) {
	result := make(map[string]string) // lower field name -> field name
	st := reflect.TypeOf(res).Elem()  // elem for ptr to res
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

// LangFieldNameToStructFieldName returns the mapping from lang (AST) field
// names to field name as used in the struct. The logic here is a bit strange;
// if the resource has struct tags, then it uses those, otherwise it falls back
// to using the lower case versions of things. It might be clever to combine the
// two so that tagged fields are used as such, and others are used in lowercase,
// but this is currently not implemented.
// TODO: should this behaviour be changed?
func LangFieldNameToStructFieldName(kind string) (map[string]string, error) {
	res, err := NewResource(kind)
	if err != nil {
		return nil, err
	}
	mapping, err := StructTagToFieldName(res)
	if err != nil {
		return nil, errwrap.Wrapf(err, "resource kind `%s` has bad field mapping", kind)
	}
	if len(mapping) == 0 { // if no `lang` tags exist, get them automatically
		mapping, err = LowerStructFieldNameToFieldName(res)
		if err != nil {
			return nil, errwrap.Wrapf(err, "resource kind `%s` has bad automatic field mapping", kind)
		}
	}

	return mapping, nil // lang field name -> field name
}

// StructKindToFieldNameTypeMap returns a map from field name to expected type
// in the lang type system.
func StructKindToFieldNameTypeMap(kind string) (map[string]*types.Type, error) {
	res, err := NewResource(kind)
	if err != nil {
		return nil, err
	}

	sv := reflect.ValueOf(res).Elem() // pointer to struct, then struct
	if k := sv.Kind(); k != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got: %s", k)
	}

	result := make(map[string]*types.Type)

	st := reflect.TypeOf(res).Elem() // pointer to struct, then struct
	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		name := field.Name
		// TODO: in future, skip over fields that don't have a `lang` tag
		//if name == "BaseRes" { // TODO: hack!!!
		//	continue
		//}

		typ, err := types.TypeOf(field.Type)
		// some types (eg complex64) aren't convertible, so skip for now...
		if err != nil {
			continue
			//return nil, errwrap.Wrapf(err, "could not identify type of field `%s`", name)
		}
		result[name] = typ
	}

	return result, nil
}

// LangFieldNameToStructType returns the mapping from lang (AST) field names,
// and the expected type in our type system for each.
func LangFieldNameToStructType(kind string) (map[string]*types.Type, error) {
	// returns a mapping between fieldName and expected *types.Type
	fieldNameTypMap, err := StructKindToFieldNameTypeMap(kind)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not determine types for `%s` resource", kind)
	}

	mapping, err := LangFieldNameToStructFieldName(kind)
	if err != nil {
		return nil, err
	}

	// transform from field name to tag name
	typMap := make(map[string]*types.Type)
	for name, typ := range fieldNameTypMap {
		if strings.Title(name) != name {
			continue // skip private fields
		}

		found := false
		for k, v := range mapping {
			if v != name {
				continue
			}
			// found
			if found { // previously found!
				return nil, fmt.Errorf("duplicate mapping for: %s", name)
			}
			typMap[k] = typ
			found = true // :)
		}
		if !found {
			return nil, fmt.Errorf("could not find mapping for: %s", name)
		}
	}

	return typMap, nil
}

// GetUID returns the UID of an user. It supports an UID or an username. Caller
// should first check user is not empty. It will return an error if it can't
// lookup the UID or username.
func GetUID(username string) (int, error) {
	userObj, err := user.LookupId(username)
	if err == nil {
		return strconv.Atoi(userObj.Uid)
	}

	userObj, err = user.Lookup(username)
	if err == nil {
		return strconv.Atoi(userObj.Uid)
	}

	return -1, errwrap.Wrapf(err, "user lookup error (%s)", username)
}

// GetGID returns the GID of a group. It supports a GID or a group name. Caller
// should first check group is not empty. It will return an error if it can't
// lookup the GID or group name.
func GetGID(group string) (int, error) {
	groupObj, err := user.LookupGroupId(group)
	if err == nil {
		return strconv.Atoi(groupObj.Gid)
	}

	groupObj, err = user.LookupGroup(group)
	if err == nil {
		return strconv.Atoi(groupObj.Gid)
	}

	return -1, errwrap.Wrapf(err, "group lookup error (%s)", group)
}
