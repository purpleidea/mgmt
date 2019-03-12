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

package graph

import (
	"fmt"
	"reflect"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	multierr "github.com/hashicorp/go-multierror"
)

// SendRecv pulls in the sent values into the receive slots. It is called by the
// receiver and must be given as input the full resource struct to receive on.
// It applies the loaded values to the resource.
func (obj *Engine) SendRecv(res engine.RecvableRes) (map[string]bool, error) {
	recv := res.Recv()
	if obj.Debug {
		// NOTE: this could expose private resource data like passwords
		obj.Logf("%s: SendRecv: %+v", res, recv)
	}
	var updated = make(map[string]bool) // list of updated keys
	var err error
	for k, v := range recv {
		updated[k] = false // default
		v.Changed = false  // reset to the default

		var st interface{} = v.Res // old style direct send/recv
		if true {                  // new style send/recv API
			st = v.Res.Sent()
		}

		if st == nil {
			e := fmt.Errorf("received nil value from: %s", v.Res)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		if e := engineUtil.StructFieldCompat(st, v.Key, res, k); e != nil {
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// send
		m1, e := engineUtil.StructTagToFieldName(st)
		if e != nil {
			err = multierr.Append(err, e) // list of errors
			continue
		}
		key1, exists := m1[v.Key]
		if !exists {
			e := fmt.Errorf("requested key of `%s` not found in send struct", v.Key)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		obj1 := reflect.Indirect(reflect.ValueOf(st))
		type1 := obj1.Type()
		value1 := obj1.FieldByName(key1)
		kind1 := value1.Kind()

		// recv
		m2, e := engineUtil.StructTagToFieldName(res)
		if e != nil {
			err = multierr.Append(err, e) // list of errors
			continue
		}
		key2, exists := m2[k]
		if !exists {
			e := fmt.Errorf("requested key of `%s` not found in recv struct", k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		obj2 := reflect.Indirect(reflect.ValueOf(res)) // pass in full struct
		type2 := obj2.Type()
		value2 := obj2.FieldByName(key2)
		kind2 := value2.Kind()

		if obj.Debug {
			obj.Logf("Send(%s) has %v: %v", type1, kind1, value1)
			obj.Logf("Recv(%s) has %v: %v", type2, kind2, value2)
		}

		// i think we probably want the same kind, at least for now...
		if kind1 != kind2 {
			e := fmt.Errorf("kind mismatch between %s: %s and %s: %s", v.Res, kind1, res, kind2)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if the types don't match, we can't use send->recv
		// FIXME: do we want to relax this for string -> *string ?
		if e := TypeCmp(value1, value2); e != nil {
			e := errwrap.Wrapf(e, "type mismatch between %s and %s", v.Res, res)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if we can't set, then well this is pointless!
		if !value2.CanSet() {
			e := fmt.Errorf("can't set %s.%s", res, k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if we can't interface, we can't compare...
		if !value1.CanInterface() || !value2.CanInterface() {
			e := fmt.Errorf("can't interface %s.%s", res, k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if the values aren't equal, we're changing the receiver
		if !reflect.DeepEqual(value1.Interface(), value2.Interface()) {
			// TODO: can we catch the panics here in case they happen?
			value2.Set(value1) // do it for all types that match
			updated[k] = true  // we updated this key!
			v.Changed = true   // tag this key as updated!
			obj.Logf("SendRecv: %s.%s -> %s.%s", v.Res, v.Key, res, k)
		}
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
