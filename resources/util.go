// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
	"reflect"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/recwatch"

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
	err = d.Decode(&output) // pass with &
	if err != nil {
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
		// TODO: golang 1.7+
		// if !ok, then nothing is found
		//if alias, ok := field.Tag.Lookup(StructTag); ok { // golang 1.7+
		if alias := field.Tag.Get(StructTag); alias != "" { // golang 1.6
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

// PathWatch is used to watch a path or file. The function sends an event
// in the case that the path or file changes. The recurse bool allows the user
// to choose whether or not to watch the specified path recursively.
func PathWatch(res Res, pathToWatch string, recurse bool) error {
	var err error
	recWatcher, err := recwatch.NewRecWatcher(pathToWatch, recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	// notify engine that we're running
	if err := res.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error

	for {
		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s watcher error", res)
			}
			send = true
			res.StateOK(false) // dirty

		case event := <-res.Events():
			if exit, send = res.ReadEvent(event); exit != nil {
				return *exit // exit
			}
			//res.StateOK(false) // dirty // these events don't invalidate state
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			res.Event()
		}
	}
}
