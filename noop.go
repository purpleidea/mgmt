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
)

func init() {
	gob.Register(&NoopRes{})
}

// NoopRes is a no-op resource that does nothing.
type NoopRes struct {
	BaseRes `yaml:",inline"`
	Comment string `yaml:"comment"` // extra field for example purposes
}

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

func (obj *NoopRes) Init() {
	obj.BaseRes.kind = "Noop"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

// validate if the params passed in are valid data
// FIXME: where should this get called ?
func (obj *NoopRes) Validate() bool {
	return true
}

func (obj *NoopRes) Watch(processChan chan Event) {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var send = false // send event?
	var exit = false
	for {
		obj.SetState(resStateWatching) // reset
		select {
		case event := <-obj.events:
			cuuid.SetConverged(false)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(&event); exit {
				return // exit
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
			resp := NewResp()
			processChan <- Event{eventNil, resp, "", true} // trigger process
			resp.ACKWait()                                 // wait for the ACK()
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

// include all params to make a unique identification of this object
// most resources only return one, although some resources return multiple
func (obj *NoopRes) GetUUIDs() []ResUUID {
	x := &NoopUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		name:     obj.Name,
	}
	return []ResUUID{x}
}

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
