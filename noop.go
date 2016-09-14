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

package main

import (
	"encoding/gob"
	"log"
	"time"
)

func init() {
	gob.Register(&NoopRes{})
}

// NoopRes is a no-op resource that does nothing.
type NoopRes struct {
	BaseRes `yaml:",inline"`
	Comment string `yaml:"comment"` // extra field for example purposes
}

// NewNoopRes is a constructor for this resource. It also calls Init() for you.
func NewNoopRes(name string) *NoopRes {
	obj := &NoopRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Comment: "",
	}
	obj.Init()
	return obj
}

// Init runs some startup code for this resource.
func (obj *NoopRes) Init() {
	obj.BaseRes.kind = "Noop"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

// validate if the params passed in are valid data
// FIXME: where should this get called ?
func (obj *NoopRes) Validate() bool {
	return true
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *NoopRes) Watch(processChan chan Event, delay time.Duration) error {
	if obj.IsWatching() {
		return nil // TODO: should this be an error?
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var doSend func() (bool, error) // lol, golang doesn't support recursive lambdas
	doSend = func() (bool, error) {
		resp := NewResp()
		processChan <- Event{eventNil, resp, "", true} // trigger process
		select {
		case e := <-resp: // wait for the ACK()
			if e != nil { // we got a NACK
				return true, e // exit with error
			}

		case event := <-obj.events:
			// NOTE: this code should match the similar code below!
			cuuid.SetConverged(false)
			if exit, send := obj.ReadEvent(&event); exit {
				return true, nil // exit, without error
			} else if send {
				return doSend() // recurse
			}
		}
		return false, nil // return, no error or exit signal
	}

	// if a retry-delay was requested, wait, but don't block our events!
	if delay > 0 {
		var pendingSendEvent bool
		timer := time.NewTimer(delay)
	Loop:
		for {
			select {
			case <-timer.C: // the wait is over
				break Loop // critical

			case event := <-obj.events:
				// NOTE: this code should match the similar code below!
				cuuid.SetConverged(false)
				if exit, send := obj.ReadEvent(&event); exit {
					return nil // exit
				} else if send {
					// NOTE: see long comment in the file resource
					//if exit, err := doSend(); exit || err != nil {
					//	return err // we exit or bubble up a NACK...
					//}
					pendingSendEvent = true // all events are identical for now...
				}
			}
		}
		timer.Stop() // it's nice to cleanup
		log.Printf("%s[%s]: Delay expired!", obj.Kind(), obj.GetName())
		if pendingSendEvent { // TODO: should this become a list in the future?
			if exit, err := doSend(); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}

	var send = false // send event?
	var exit = false
	for {
		obj.SetState(resStateWatching) // reset
		select {
		case event := <-obj.events:
			cuuid.SetConverged(false)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}

		case <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// only do this on certain types of events
			//obj.isStateOK = false // something made state dirty
			if exit, err := doSend(); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}

// CheckApply method for Noop resource. Does nothing, returns happy!
func (obj *NoopRes) CheckApply(apply bool) (checkok bool, err error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)
	return true, nil // state is always okay
}

// NoopUUID is the UUID struct for NoopRes.
type NoopUUID struct {
	BaseUUID
	name string
}

// The AutoEdges method returns the AutoEdges. In this case none are used.
func (obj *NoopRes) AutoEdges() AutoEdge {
	return nil
}

// GetUUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *NoopRes) GetUUIDs() []ResUUID {
	x := &NoopUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		name:     obj.Name,
	}
	return []ResUUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *NoopRes) GroupCmp(r Res) bool {
	_, ok := r.(*NoopRes)
	if !ok {
		// NOTE: technically we could group a noop into any other
		// resource, if that resource knew how to handle it, although,
		// since the mechanics of inter-kind resource grouping are
		// tricky, avoid doing this until there's a good reason.
		return false
	}
	return true // noop resources can always be grouped together!
}

// Compare two resources and return if they are equivalent.
func (obj *NoopRes) Compare(res Res) bool {
	switch res.(type) {
	// we can only compare NoopRes to others of the same resource
	case *NoopRes:
		res := res.(*NoopRes)
		// calling base Compare is unneeded for the noop res
		//if !obj.BaseRes.Compare(res) { // call base Compare
		//	return false
		//}
		if obj.Name != res.Name {
			return false
		}
	default:
		return false
	}
	return true
}
