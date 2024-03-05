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

package graph

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// RecvFn represents a custom Recv function which can be used in place of the
// stock, built-in one. This is needed if we want to receive from a different
// resource data source than our own. (Only for special occasions of course!)
type RecvFn func(engine.RecvableRes) (map[string]*engine.Send, error)

// SendRecv pulls in the sent values into the receive slots. It is called by the
// receiver and must be given as input the full resource struct to receive on.
// It applies the loaded values to the resource. It is called recursively, as it
// recurses into any grouped resources found within the first receiver. It
// returns a map of resource pointer, to resource field key, to changed boolean.
func SendRecv(res engine.RecvableRes, fn RecvFn) (map[engine.RecvableRes]map[string]*engine.Send, error) {
	updated := make(map[engine.RecvableRes]map[string]*engine.Send) // list of updated keys
	if groupableRes, ok := res.(engine.GroupableRes); ok {
		for _, x := range groupableRes.GetGroup() { // grouped elements
			recvableRes, ok := x.(engine.RecvableRes)
			if !ok {
				continue
			}
			//if obj.Debug {
			//	obj.Logf("SendRecv: %s: grouped: %s", res, x) // receiving here
			//}
			// We need to recurse here so that autogrouped resources
			// inside autogrouped resources would work... In case we
			// work correctly. We just need to make sure that things
			// are grouped in the correct order, but that is not our
			// problem! Recurse and merge in the changed results...
			innerUpdated, err := SendRecv(recvableRes, fn)
			if err != nil {
				return nil, errwrap.Wrapf(err, "recursive SendRecv error")
			}
			for r, m := range innerUpdated { // res ptr, map
				if _, exists := updated[r]; !exists {
					updated[r] = make(map[string]*engine.Send)
				}
				for s, send := range m { // map[string]*engine.Send
					b := send.Changed
					// don't overwrite in case one exists...
					if old, exists := updated[r][s]; exists {
						b = b || old.Changed // unlikely i think
					}
					if _, exists := updated[r][s]; !exists {
						newSend := &engine.Send{
							Res:     send.Res,
							Key:     send.Key,
							Changed: b,
						}
						updated[r][s] = newSend
					}
					updated[r][s].Changed = b
				}
			}
		}
	}

	var err error
	recv := res.Recv()
	if fn != nil {
		recv, err = fn(res) // use a custom Recv function
		if err != nil {
			return nil, err
		}
	}
	keys := []string{}
	for k := range recv { // map[string]*Send
		keys = append(keys, k)
	}
	sort.Strings(keys)
	//if obj.Debug && len(keys) > 0 {
	//	// NOTE: this could expose private resource data like passwords
	//	obj.Logf("SendRecv: %s recv: %+v", res, strings.Join(keys, ", "))
	//}
	for k, v := range recv { // map[string]*Send
		// v.Res // SendableRes // a handle to the resource which is sending a value
		// v.Key // string      // the key in the resource that we're sending
		if _, exists := updated[res]; !exists {
			updated[res] = make(map[string]*engine.Send)
		}

		//updated[res][k] = false // default
		v.Changed = false   // reset to the default
		updated[res][k] = v // default

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
		//type1 := obj1.Type()
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
		//type2 := obj2.Type()
		value2 := obj2.FieldByName(key2)
		kind2 := value2.Kind()

		//orig := value1
		dest := value2 // save the o.g. because we need the real dest!

		// NOTE: Reminder: obj1 comes from st and it is the *<Res>Sends
		// struct which contains whichever fields that resource sends.
		// For example, this might be *TestSends for the Test resource.
		// The receiver is obj2 and that is actually the resource struct
		// which is a *<Res> and which gets it's fields directly set on.
		// For example, this might be *TestRes for the Test resource.
		//fmt.Printf("obj1(%T): %+v\n", obj1, obj1)
		//fmt.Printf("obj2(%T): %+v\n", obj2, obj2)
		// Lastly, remember that many of the type incompatibilities are
		// caught during type unification, and so we might have overly
		// relaxed the checks here and something could slip by. If we
		// find something, this code will need new checks added back.

		// Here we unpack one-level, and then leave the complex stuff
		// for the Into() method below.
		// for kind1 == reflect.Interface || kind1 == reflect.Ptr // wrong
		// if kind1 == reflect.Interface || kind1 == reflect.Ptr  // wrong
		// for kind1 == reflect.Interface // wrong
		if kind1 == reflect.Interface {
			value1 = value1.Elem() // un-nest one interface
			kind1 = value1.Kind()
		}

		// This second block is identical, but it's just accidentally
		// symmetrical. The types of input structs are different shapes.
		// for kind2 == reflect.Interface || kind2 == reflect.Ptr // wrong
		// if kind2 == reflect.Interface || kind2 == reflect.Ptr  // wrong
		// for kind2 == reflect.Interface // wrong
		if kind2 == reflect.Interface {
			value2 = value2.Elem() // un-nest one interface
			kind2 = value2.Kind()
		}

		//if obj.Debug {
		//	obj.Logf("Send(%s) has %v: %v", type1, kind1, value1)
		//	obj.Logf("Recv(%s) has %v: %v", type2, kind2, value2)
		//}

		// Skip this check in favour of the more complex Into() below...
		//if kind1 != kind2 {
		//	e := fmt.Errorf("send/recv kind mismatch between %s: %s and %s: %s", v.Res, kind1, res, kind2)
		//	err = errwrap.Append(err, e) // list of errors
		//	continue
		//}

		// Skip this check in favour of the more complex Into() below...
		//if e := TypeCmp(value1, value2); e != nil {
		//	e := errwrap.Wrapf(e, "type mismatch between %s and %s", v.Res, res)
		//	err = errwrap.Append(err, e) // list of errors
		//	continue
		//}

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
			// runtime error, probably from using value res
			e := errwrap.Wrapf(e, "mismatch: %s.%s (%s) -> %s.%s (%s)", v.Res, v.Key, kind1, res, k, kind2)
			err = errwrap.Append(err, e) // list of errors
			continue
		}
		//dest.Set(orig)  // do it for all types that match
		//updated[res][k] = true // we updated this key!
		v.Changed = true    // tag this key as updated!
		updated[res][k] = v // we updated this key!
		//obj.Logf("SendRecv: %s.%s -> %s.%s (%+v)", v.Res, v.Key, res, k, fv) // fv may be private data
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

// UpdatedStrings returns a list of strings showing what was updated after a
// Send/Recv run returned the updated datastructure. This is useful for logs.
func UpdatedStrings(updated map[engine.RecvableRes]map[string]*engine.Send) []string {
	out := []string{}
	for r, m := range updated { // map[engine.RecvableRes]map[string]*engine.Send
		for s, send := range m {
			if !send.Changed {
				continue
			}
			x := fmt.Sprintf("%v.%s -> %v.%s", send.Res, send.Key, r, s)
			out = append(out, x)
		}
	}
	return out
}
