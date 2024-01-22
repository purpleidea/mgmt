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

package graph

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// SendRecv pulls in the sent values into the receive slots. It is called by the
// receiver and must be given as input the full resource struct to receive on.
// It applies the loaded values to the resource. It is called recursively, as it
// recurses into any grouped resources found within the first receiver. It
// returns a map of resource pointer, to resource field key, to changed boolean.
func (obj *Engine) SendRecv(res engine.RecvableRes) (map[engine.RecvableRes]map[string]bool, error) {
	updated := make(map[engine.RecvableRes]map[string]bool) // list of updated keys
	if obj.Debug {
		obj.Logf("SendRecv: %s", res) // receiving here
	}
	if groupableRes, ok := res.(engine.GroupableRes); ok {
		for _, x := range groupableRes.GetGroup() { // grouped elements
			recvableRes, ok := x.(engine.RecvableRes)
			if !ok {
				continue
			}
			if obj.Debug {
				obj.Logf("SendRecv: %s: grouped: %s", res, x) // receiving here
			}
			// We need to recurse here so that autogrouped resources
			// inside autogrouped resources would work... In case we
			// work correctly. We just need to make sure that things
			// are grouped in the correct order, but that is not our
			// problem! Recurse and merge in the changed results...
			innerUpdated, err := obj.SendRecv(recvableRes)
			if err != nil {
				return nil, errwrap.Wrapf(err, "recursive SendRecv error")
			}
			for r, m := range innerUpdated { // res ptr, map
				if _, exists := updated[r]; !exists {
					updated[r] = make(map[string]bool)
				}
				for s, b := range m {
					// don't overwrite in case one exists...
					if old, exists := updated[r][s]; exists {
						b = b || old // unlikely i think
					}
					updated[r][s] = b
				}
			}
		}
	}

	recv := res.Recv()
	keys := []string{}
	for k := range recv { // map[string]*Send
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if obj.Debug && len(keys) > 0 {
		// NOTE: this could expose private resource data like passwords
		obj.Logf("SendRecv: %s recv: %+v", res, strings.Join(keys, ", "))
	}
	var err error
	for k, v := range recv { // map[string]*Send
		// v.Res // SendableRes // a handle to the resource which is sending a value
		// v.Key // string      // the key in the resource that we're sending
		if _, exists := updated[res]; !exists {
			updated[res] = make(map[string]bool)
		}

		updated[res][k] = false // default
		v.Changed = false       // reset to the default

		var st interface{} = v.Res // old style direct send/recv
		if true {                  // new style send/recv API
			st = v.Res.Sent()
		}

		if st == nil {
			e := fmt.Errorf("received nil value from: %s", v.Res)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		if e := engineUtil.StructFieldCompat(st, v.Key, res, k); e != nil {
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		// send
		m1, e := engineUtil.StructTagToFieldName(st)
		if e != nil {
			err = errwrap.Append(err, e) // list of errors
			continue
		}
		key1, exists := m1[v.Key]
		if !exists {
			e := fmt.Errorf("requested key of `%s` not found in send struct", v.Key)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		obj1 := reflect.Indirect(reflect.ValueOf(st))
		type1 := obj1.Type()
		value1 := obj1.FieldByName(key1)
		kind1 := value1.Kind()

		// recv
		m2, e := engineUtil.StructTagToFieldName(res)
		if e != nil {
			err = errwrap.Append(err, e) // list of errors
			continue
		}
		key2, exists := m2[k]
		if !exists {
			e := fmt.Errorf("requested key of `%s` not found in recv struct", k)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		obj2 := reflect.Indirect(reflect.ValueOf(res)) // pass in full struct
		type2 := obj2.Type()
		value2 := obj2.FieldByName(key2)
		kind2 := value2.Kind()

		//orig := value1
		dest := value2 // save the o.g. because we need the real dest!

		// For situations where we send a variant to the resource!
		for kind1 == reflect.Interface || kind1 == reflect.Ptr {
			value1 = value1.Elem() // un-nest one interface
			kind1 = value1.Kind()
		}
		for kind2 == reflect.Interface || kind2 == reflect.Ptr {
			value2 = value2.Elem() // un-nest one interface
			kind2 = value2.Kind()
		}

		if obj.Debug {
			obj.Logf("Send(%s) has %v: %v", type1, kind1, value1)
			obj.Logf("Recv(%s) has %v: %v", type2, kind2, value2)
		}

		// i think we probably want the same kind, at least for now...
		if kind1 != kind2 {
			e := fmt.Errorf("send/recv kind mismatch between %s: %s and %s: %s", v.Res, kind1, res, kind2)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		// if the types don't match, we can't use send->recv
		// FIXME: do we want to relax this for string -> *string ?
		if e := TypeCmp(value1, value2); e != nil {
			e := errwrap.Wrapf(e, "type mismatch between %s and %s", v.Res, res)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		// if we can't set, then well this is pointless!
		if !dest.CanSet() {
			e := fmt.Errorf("can't set %s.%s", res, k)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		// if we can't interface, we can't compare...
		if !value1.CanInterface() {
			e := fmt.Errorf("can't interface %s.%s", v.Res, v.Key)
			err = errwrap.Append(err, e) // list of errors
			continue
		}
		if !value2.CanInterface() {
			e := fmt.Errorf("can't interface %s.%s", res, k)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		// if the values aren't equal, we're changing the receiver
		if reflect.DeepEqual(value1.Interface(), value2.Interface()) {
			continue // skip as they're the same, no error needed
		}

		// TODO: can we catch the panics here in case they happen?

		fv, e := types.ValueOf(value1)
		if e != nil {
			e := errwrap.Wrapf(e, "bad value %s.%s", v.Res, v.Key)
			err = errwrap.Append(err, e) // list of errors
			continue
		}

		// mutate the struct field dest with the mcl data in fv
		if e := types.Into(fv, dest); e != nil {
			e := errwrap.Wrapf(e, "bad dest %s.%s", v.Res, v.Key)
			err = errwrap.Append(err, e) // list of errors
			continue
		}
		//dest.Set(orig)  // do it for all types that match
		updated[res][k] = true // we updated this key!
		v.Changed = true       // tag this key as updated!
		obj.Logf("SendRecv: %s.%s -> %s.%s", v.Res, v.Key, res, k)
	}
	return updated, err
}

// TypeCmp compares two reflect values to see if they are the same Kind. It can
// look into a ptr Kind to see if the underlying pair of ptr's can TypeCmp too!
func TypeCmp(a, b reflect.Value) error {
	ta, tb := a.Type(), b.Type()
	if ta != tb {
		return fmt.Errorf("type mismatch: %s != %s", ta, tb)
	}
	// NOTE: it seems we don't need to recurse into pointers to sub check!

	return nil // identical Type()'s
}
