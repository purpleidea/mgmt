// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// Event sends off an event, but doesn't block the incoming event queue.
func (obj *BaseRes) Event() error {
	resp := event.NewResp()
	obj.processLock.Lock()
	if obj.processDone {
		obj.processLock.Unlock()
		return fmt.Errorf("processChan is already closed")
	}
	obj.quiesceGroup.Add(1)                                           // add to processChan queue count
	obj.processChan <- &event.Event{Kind: event.EventNil, Resp: resp} // trigger process
	obj.processLock.Unlock()
	return resp.Wait()
}

// SendEvent pushes an event into the message queue for a particular vertex.
func (obj *BaseRes) SendEvent(ev event.Kind, err error) error {
	if obj.debug {
		if err == nil {
			log.Printf("%s: SendEvent(%+v)", obj, ev)
		} else {
			log.Printf("%s: SendEvent(%+v): %v", obj, ev, err)
		}
	}
	resp := event.NewResp()
	obj.eventsLock.Lock()
	if obj.eventsDone {
		obj.eventsLock.Unlock()
		return fmt.Errorf("eventsChan is already closed")
	}
	obj.eventsChan <- &event.Event{Kind: ev, Resp: resp, Err: err}
	if ev == event.EventExit {
		obj.eventsDone = true
		close(obj.eventsChan) // this is where we properly close this channel!
	}
	obj.eventsLock.Unlock()
	resp.ACKWait() // waits until true (nil) value
	return nil
}

// ReadEvent processes events when a select gets one, and handles the pause
// code too! The return values specify if we should exit and poke respectively.
func (obj *BaseRes) ReadEvent(ev *event.Event) (exit *error, send bool) {
	//ev.ACK()
	err := ev.Error()

	switch ev.Kind {
	case event.EventStart:
		ev.ACK()
		return nil, true

	case event.EventPoke:
		ev.ACK()
		return nil, true

	case event.EventBackPoke:
		ev.ACK()
		return nil, true // forward poking in response to a back poke!

	case event.EventExit:
		obj.quiescing = true
		obj.quiesceGroup.Wait()
		obj.quiescing = false // for symmetry
		ev.ACK()
		// FIXME: what do we do if we have a pending refresh (poke) and an exit?
		return &err, false

	case event.EventPause:
		obj.quiescing = true // set the quiesce flag to avoid event replays
		obj.quiesceGroup.Wait()
		obj.quiescing = false // reset
		ev.ACK()

		// wait for next event to continue, but discard any backpoking!
		for {
			// Consider a graph (V2->V3). If while paused, we add a
			// new resource (V1->V2), when we unpause, V3 will run,
			// and then V2 followed by V1 (reverse topo sort) which
			// can cause V2 to BackPoke to V1 (since V1 needs to go
			// first) which can panic if V1 is not running yet! The
			// solution is to ignore the BackPoke because once that
			// V1 vertex gets running, it will then send off a poke
			// to V2 that it did without the need for the BackPoke!
			select {
			case e, ok := <-obj.Events():
				if !ok { // shutdown
					err := error(nil)
					return &err, false
				}
				//obj.quiescing = true
				//obj.quiesceGroup.Wait() // unnecessary, but symmetrically correct
				//obj.quiescing = false
				e.ACK()
				err := e.Error()
				if e.Kind == event.EventExit {
					return &err, false
				} else if e.Kind == event.EventStart { // eventContinue
					return nil, false // don't poke on unpause!
				} else if e.Kind == event.EventBackPoke {
					continue // silently discard this event while paused
				}
				// if we get a poke event here, it's a bug!
				err = fmt.Errorf("%s: unknown event: %v, while paused", obj, e)
				panic(err) // TODO: return a special sentinel instead?
				//return &err, false
			}
		}
	}
	err = fmt.Errorf("unknown event: %v", ev)
	panic(err) // TODO: return a special sentinel instead?
	//return &err, false
}

// Running is called by the Watch method of the resource once it has started up.
// This signals to the engine to kick off the initial CheckApply resource check.
func (obj *BaseRes) Running() error {
	// TODO: If a non-polling resource wants to use the converger, then it
	// should probably tell Running (via an arg) to not do this. Currently
	// it's a very unlikely race that could cause an early converge if the
	// converge timeout is very short ( ~ 1s) and the Watch method doesn't
	// immediately SetConverged(false) to stop possible early termination.
	if obj.Meta().Poll == 0 { // if not polling, unblock this...
		obj.cuid.SetConverged(true) // a reasonable initial assumption
	}

	obj.StateOK(false)  // assume we're initially dirty
	if !obj.isStarted { // this avoids a double close when/if watch retries
		obj.isStarted = true
		close(obj.started) // send started signal
	}

	var err error
	if obj.starter { // vertices of indegree == 0 should send initial pokes
		err = obj.Event() // trigger a CheckApply
	}
	return err // bubble up any possible error (or nil)
}

// Send points to a value that a resource will send.
type Send struct {
	Res Res    // a handle to the resource which is sending a value
	Key string // the key in the resource that we're sending

	Changed bool // set to true if this key was updated, read only!
}

// SetRecv sets the Res Recv field to given map of Send structs.
func (obj *BaseRes) SetRecv(recv map[string]*Send) {
	obj.Recv = recv
}

// SendRecv pulls in the sent values into the receive slots. It is called by the
// receiver and must be given as input the full resource struct to receive on.
func (obj *BaseRes) SendRecv(res Res) (map[string]bool, error) {
	if obj.debug {
		// NOTE: this could expose private resource data like passwords
		log.Printf("%s: SendRecv: %+v", obj, obj.Recv)
	}
	var updated = make(map[string]bool) // list of updated keys
	var err error
	for k, v := range obj.Recv {
		updated[k] = false // default
		v.Changed = false  // reset to the default
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

		if obj.debug {
			log.Printf("Send(%s) has %v: %v", type1, kind1, value1)
			log.Printf("Recv(%s) has %v: %v", type2, kind2, value2)
		}

		// i think we probably want the same kind, at least for now...
		if kind1 != kind2 {
			e := fmt.Errorf("kind mismatch between %s: %s and %s: %s", v.Res, kind1, obj, kind2)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if the types don't match, we can't use send->recv
		// TODO: do we want to relax this for string -> *string ?
		if e := TypeCmp(value1, value2); e != nil {
			e := errwrap.Wrapf(e, "type mismatch between %s and %s", v.Res, obj)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if we can't set, then well this is pointless!
		if !value2.CanSet() {
			e := fmt.Errorf("can't set %s.%s", obj, k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if we can't interface, we can't compare...
		if !value1.CanInterface() || !value2.CanInterface() {
			e := fmt.Errorf("can't interface %s.%s", obj, k)
			err = multierr.Append(err, e) // list of errors
			continue
		}

		// if the values aren't equal, we're changing the receiver
		if !reflect.DeepEqual(value1.Interface(), value2.Interface()) {
			// TODO: can we catch the panics here in case they happen?
			value2.Set(value1) // do it for all types that match
			updated[k] = true  // we updated this key!
			v.Changed = true   // tag this key as updated!
			log.Printf("SendRecv: %s.%s -> %s.%s", v.Res, v.Key, obj, k)
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
