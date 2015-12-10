// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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
	"fmt"
	"log"
	"time"
)

type Type interface {
	Init()
	GetName() string // can't be named "Name()" because of struct field
	Watch()
	StateOK() bool // TODO: can we rename this to something better?
	Apply() bool
	SetVertex(*Vertex)
	Compare(Type) bool
	SendEvent(eventName, bool)
	GetTimestamp() int64
	UpdateTimestamp() int64
	//Process()
}

type BaseType struct {
	Name      string `yaml:"name"`
	timestamp int64  // last updated timestamp ?
	events    chan Event
	vertex    *Vertex
}

type NoopType struct {
	BaseType `yaml:",inline"`
	Comment  string `yaml:"comment"` // extra field for example purposes
}

func NewNoopType(name string) *NoopType {
	// FIXME: we could get rid of this New constructor and use raw object creation with a required Init()
	return &NoopType{
		BaseType: BaseType{
			Name:   name,
			events: make(chan Event), // unbuffered chan size to avoid stale events
			vertex: nil,
		},
		Comment: "",
	}
}

// initialize structures like channels if created without New constructor
func (obj *BaseType) Init() {
	obj.events = make(chan Event)
}

// this method gets used by all the types, if we have one of (obj NoopType) it would get overridden in that case!
func (obj *BaseType) GetName() string {
	return obj.Name
}

func (obj *BaseType) GetVertex() *Vertex {
	return obj.vertex
}

func (obj *BaseType) SetVertex(v *Vertex) {
	obj.vertex = v
}

// get timestamp of a vertex
func (obj *BaseType) GetTimestamp() int64 {
	return obj.timestamp
}

// update timestamp of a vertex
func (obj *BaseType) UpdateTimestamp() int64 {
	obj.timestamp = time.Now().UnixNano() // update
	return obj.timestamp
}

// can this element run right now?
func (obj *BaseType) OKTimestamp() bool {
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		// if the vertex has a greater timestamp than any pre-req (n)
		// then we can't run right now...
		if obj.GetTimestamp() > n.Type.GetTimestamp() {
			return false
		}
	}
	return true
}

func (obj *BaseType) Poke() bool { // XXX: how can this ever fail and return false? eg: when is a poke not possible and should be rescheduled?
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphEdges(v) {
		n.SendEvent(eventPoke, false) // XXX: should this be sync or not? XXX: try it as async for now, but switch to sync and see if we deadlock -- maybe it's possible, i don't know for sure yet
	}
	return true
}

// push an event into the message queue for a particular type vertex
func (obj *BaseType) SendEvent(event eventName, sync bool) {
	if !sync {
		obj.events <- Event{event, nil, ""}
		return
	}

	resp := make(chan bool)
	obj.events <- Event{event, resp, ""}
	for {
		value := <-resp
		// wait until true value
		if value {
			return
		}
	}
}

// process events when a select gets one
// this handles the pause code too!
func (obj *BaseType) ReadEvent(event *Event) bool {

	event.ACK()
	switch event.Name {
	case eventStart:
		return true

	case eventPoke:
		return true

	case eventExit:
		return false

	case eventPause:
		// wait for next event to continue
		select {
		case e := <-obj.events:
			e.ACK()
			if e.Name == eventExit {
				return false
			} else if e.Name == eventContinue {
				return true
			} else {
				log.Fatal("Unknown event: ", e)
			}
		}

	default:
		log.Fatal("Unknown event: ", event)
	}
	return false // required to keep the stupid go compiler happy
}

// XXX: rename this function
func (obj *BaseType) Process(typ Type) {
	var ok bool

	ok = true
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if obj.OKTimestamp() {
		// XXX XXX: why does this have to be typ instead of just obj! "obj.StateOK undefined (type *BaseType has no field or method StateOK)"

		if !typ.StateOK() { // TODO: can we rename this to something better?
			// throw an error if apply fails...
			// if this fails, don't UpdateTimestamp()
			if !typ.Apply() { // check for error
				ok = false
			}
		}

		if ok {
			// if poke fails, don't update timestamp
			// since we didn't propagate the pokes!
			if obj.Poke() {
				obj.UpdateTimestamp() // this was touched...
			}
		}
	}

}

func (obj *NoopType) Watch() {
	//vertex := obj.vertex // stored with SetVertex
	var send = false // send event?
	for {

		select {
		case event := <-obj.events:

			if ok := obj.ReadEvent(&event); !ok {
				return // exit
			}
			send = true
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false

			obj.Process(obj) // XXX: rename this function
		}
	}
}

func (obj *NoopType) StateOK() bool {
	return true // never needs updating
}

func (obj *NoopType) Apply() bool {
	fmt.Printf("Apply->Noop[%v]\n", obj.Name)
	return true
}

func (obj *NoopType) Compare(typ Type) bool {
	switch typ.(type) {
	// we can only compare NoopType to others of the same type
	case *NoopType:
		return obj.compare(typ.(*NoopType))
	default:
		return false
	}
}

func (obj *NoopType) compare(typ *NoopType) bool {
	if obj.Name != typ.Name {
		return false
	}
	return true
}
