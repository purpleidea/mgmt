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

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/global"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// SendEvent pushes an event into the message queue for a particular vertex
func (obj *BaseRes) SendEvent(ev event.EventName, sync bool, activity bool) bool {
	// TODO: isn't this race-y ?
	if !obj.IsWatching() { // element has already exited
		return false // if we don't return, we'll block on the send
	}
	if !sync {
		obj.events <- event.Event{Name: ev, Resp: nil, Msg: "", Activity: activity}
		return true
	}

	resp := event.NewResp()
	obj.events <- event.Event{Name: ev, Resp: resp, Msg: "", Activity: activity}
	resp.ACKWait() // waits until true (nil) value
	return true
}

// DoSend sends off an event, but doesn't block the incoming event queue.
func (obj *BaseRes) DoSend(processChan chan event.Event, comment string) (exit bool, err error) {
	resp := event.NewResp()
	processChan <- event.Event{Name: event.EventNil, Resp: resp, Activity: false, Msg: comment} // trigger process
	e := resp.Wait()
	return false, e // XXX: at the moment, we don't use the exit bool.
}

// ReadEvent processes events when a select gets one, and handles the pause
// code too! The return values specify if we should exit and poke respectively.
func (obj *BaseRes) ReadEvent(ev *event.Event) (exit, send bool) {
	ev.ACK()
	var poke bool
	// ensure that a CheckApply runs by sending with a dirty state...
	if ev.GetActivity() { // if previous node did work, and we were notified...
		obj.StateOK(false) // dirty
		poke = true        // poke!
		// XXX: this should be elsewhere in case Watch isn't used (eg: Polling instead...)
		// XXX: unless this is used in our "fallback" polling implementation???
		obj.SetRefresh(true)
	}

	switch ev.Name {
	case event.EventStart:
		send = true || poke
		return

	case event.EventPoke:
		send = true || poke
		return

	case event.EventBackPoke:
		send = true || poke
		return // forward poking in response to a back poke!

	case event.EventExit:
		// FIXME: what do we do if we have a pending refresh (poke) and an exit?
		return true, false

	case event.EventPause:
		// wait for next event to continue
		select {
		case e, ok := <-obj.Events():
			if !ok { // shutdown
				return true, false
			}
			e.ACK()
			if e.Name == event.EventExit {
				return true, false
			} else if e.Name == event.EventStart { // eventContinue
				return false, false // don't poke on unpause!
			} else {
				// if we get a poke event here, it's a bug!
				log.Fatalf("%s[%s]: Unknown event: %v, while paused!", obj.Kind(), obj.GetName(), e)
			}
		}

	default:
		log.Fatal("Unknown event: ", ev)
	}
	return true, false // required to keep the stupid go compiler happy
}

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
