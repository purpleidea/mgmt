// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"fmt"
	"log"
	"reflect"

	"github.com/purpleidea/mgmt/global"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// Send points to a value that a resource will send.
type Send struct {
	Res Res    // a handle to the resource which is sending a value
	Key string // the key in the resource that we're sending
}

// SendRecv pulls in the sent values into the receive slots. It is called by the
// receiver and must be given as input the full resource struct to receive on.
func (obj *BaseRes) SendRecv(res Res) (bool, error) {
	log.Printf("%s[%s]: SendRecv...", obj.Kind(), obj.GetName())
	if global.DEBUG {
		log.Printf("%s[%s]: SendRecv: Debug: %+v", obj.Kind(), obj.GetName(), obj.Recv)
	}
	var changed bool // did we update a value?
	var err error
	for k, v := range obj.Recv {
		log.Printf("SendRecv: %s[%s].%s <- %s[%s].%s", obj.Kind(), obj.GetName(), k, v.Res.Kind(), v.Res.GetName(), v.Key)

		// send
		obj1 := reflect.Indirect(reflect.ValueOf(v.Res))
		type1 := obj1.Type()
		value1 := obj1.FieldByName(v.Key)
		kind1 := value1.Kind()

		// recv
		obj2 := reflect.Indirect(reflect.ValueOf(res)) // pass in full struct
		type2 := obj2.Type()
		value2 := obj2.FieldByName(k)
		kind2 := value2.Kind()

		if global.DEBUG {
			log.Printf("Send(%s) has %v: %v", type1, kind1, value1)
			log.Printf("Recv(%s) has %v: %v", type2, kind2, value2)
		}

		// i think we probably want the same kind, at least for now...
		if kind1 != kind2 {
			e := fmt.Errorf("Kind mismatch between %s[%s]: %s and %s[%s]: %s", obj.Kind(), obj.GetName(), kind2, v.Res.Kind(), v.Res.GetName(), kind1)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if the types don't match, we can't use send->recv
		// TODO: do we want to relax this for string -> *string ?
		if e := TypeCmp(value1, value2); e != nil {
			e := errwrap.Wrapf(e, "Type mismatch between %s[%s] and %s[%s]", obj.Kind(), obj.GetName(), v.Res.Kind(), v.Res.GetName())
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if we can't set, then well this is pointless!
		if !value2.CanSet() {
			e := fmt.Errorf("Can't set %s[%s].%s", obj.Kind(), obj.GetName(), k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if we can't interface, we can't compare...
		if !value1.CanInterface() || !value2.CanInterface() {
			e := fmt.Errorf("Can't interface %s[%s].%s", obj.Kind(), obj.GetName(), k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if the values aren't equal, we're changing the receiver
		if !reflect.DeepEqual(value1.Interface(), value2.Interface()) {
			// TODO: can we catch the panics here in case they happen?
			value2.Set(value1) // do it for all types that match
			changed = true
		}
	}
	return changed, err
}

// TypeCmp compares two reflect values to see if they are the same Kind. It can
// look into a ptr Kind to see if the underlying pair of ptr's can TypeCmp too!
func TypeCmp(a, b reflect.Value) error {
	ta, tb := a.Type(), b.Type()
	if ta != tb {
		return fmt.Errorf("Type mismatch: %s != %s", ta, tb)
	}
	// NOTE: it seems we don't need to recurse into pointers to sub check!

	return nil // identical Type()'s
}
