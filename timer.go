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
	gob.Register(&TimerRes{})
}

// TimerRes is a timer resource for time based events.
type TimerRes struct {
	BaseRes  `yaml:",inline"`
	Interval int `yaml:"interval"` // Interval : Interval between runs
}

// TimerUUID is the UUID struct for TimerRes.
type TimerUUID struct {
	BaseUUID
	name string
}

// NewTimerRes is a constructor for this resource. It also calls Init() for you.
func NewTimerRes(name string, interval int) *TimerRes {
	obj := &TimerRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Interval: interval,
	}
	obj.Init()
	return obj
}

// Init runs some startup code for this resource.
func (obj *TimerRes) Init() {
	obj.BaseRes.kind = "Timer"
	obj.BaseRes.Init() // call base init, b/c we're overrriding
}

// Validate the params that are passed to TimerRes
// Currently we are getting only an interval in seconds
// which gets validated by go compiler
func (obj *TimerRes) Validate() bool {
	return true
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TimerRes) Watch(processChan chan Event, delay time.Duration) error {
	if obj.IsWatching() {
		return nil
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var doSend func(string) (bool, error) // lol, golang doesn't support recursive lambdas
	doSend = func(comment string) (bool, error) {
		resp := NewResp()
		processChan <- Event{eventNil, resp, comment, true} // trigger process
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
				return doSend(comment) // recurse
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
			if exit, err := doSend("pending delayed event"); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}

	// Create a time.Ticker for the given interval
	ticker := time.NewTicker(time.Duration(obj.Interval) * time.Second)
	defer ticker.Stop()

	var send = false

	for {
		obj.SetState(resStateWatching)
		select {
		case <-ticker.C: // received the timer event
			send = true
			log.Printf("%v[%v]: received tick", obj.Kind(), obj.GetName())
		case event := <-obj.events:
			cuuid.SetConverged(false)
			if exit, _ := obj.ReadEvent(&event); exit {
				return nil
			}
		case <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true)
			continue
		}
		if send {
			send = false
			obj.isStateOK = false
			if exit, err := doSend("timer ticked"); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}

// GetUUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *TimerRes) GetUUIDs() []ResUUID {
	x := &TimerUUID{
		BaseUUID: BaseUUID{
			name: obj.GetName(),
			kind: obj.Kind(),
		},
		name: obj.Name,
	}
	return []ResUUID{x}
}

// The AutoEdges method returns the AutoEdges. In this case none are used.
func (obj *TimerRes) AutoEdges() AutoEdge {
	return nil
}

// Compare two resources and return if they are equivalent.
func (obj *TimerRes) Compare(res Res) bool {
	switch res.(type) {
	case *TimerRes:
		res := res.(*TimerRes)
		if !obj.BaseRes.Compare(res) {
			return false
		}
		if obj.Name != res.Name {
			return false
		}
		if obj.Interval != res.Interval {
			return false
		}
	default:
		return false
	}
	return true
}

// CheckApply method for Timer resource. Does nothing, returns happy!
func (obj *TimerRes) CheckApply(apply bool) (bool, error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)
	return true, nil // state is always okay
}
