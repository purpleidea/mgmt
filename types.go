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
	"log"
	"time"
)

//go:generate stringer -type=typeState -output=typestate_stringer.go
type typeState int

const (
	typeNil typeState = iota
	typeWatching
	typeEvent // an event has happened, but we haven't poked yet
	typeApplying
	typePoking
)

//go:generate stringer -type=typeConvergedState -output=typeconvergedstate_stringer.go
type typeConvergedState int

const (
	typeConvergedNil typeConvergedState = iota
	//typeConverged
	typeConvergedTimeout
)

type Type interface {
	Init()
	GetName() string // can't be named "Name()" because of struct field
	GetType() string
	Watch()
	StateOK() bool // TODO: can we rename this to something better?
	Apply() bool
	SetVertex(*Vertex)
	SetConvegedCallback(ctimeout int, converged chan bool)
	Compare(Type) bool
	SendEvent(eventName, bool, bool)
	IsWatching() bool
	SetWatching(bool)
	GetConvergedState() typeConvergedState
	SetConvergedState(typeConvergedState)
	GetState() typeState
	SetState(typeState)
	GetTimestamp() int64
	UpdateTimestamp() int64
	OKTimestamp() bool
	Poke(bool)
	BackPoke()
}

type BaseType struct {
	Name           string `yaml:"name"`
	timestamp      int64  // last updated timestamp ?
	events         chan Event
	vertex         *Vertex
	state          typeState
	convergedState typeConvergedState
	watching       bool // is Watch() loop running ?
	ctimeout       int  // converged timeout
	converged      chan bool
	isStateOK      bool // whether the state is okay based on events or not
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

func (obj *BaseType) GetType() string {
	return "Base"
}

func (obj *BaseType) GetVertex() *Vertex {
	return obj.vertex
}

func (obj *BaseType) SetVertex(v *Vertex) {
	obj.vertex = v
}

func (obj *BaseType) SetConvegedCallback(ctimeout int, converged chan bool) {
	obj.ctimeout = ctimeout
	obj.converged = converged
}

// is the Watch() function running?
func (obj *BaseType) IsWatching() bool {
	return obj.watching
}

// store status of if the Watch() function is running
func (obj *BaseType) SetWatching(b bool) {
	obj.watching = b
}

func (obj *BaseType) GetConvergedState() typeConvergedState {
	return obj.convergedState
}

func (obj *BaseType) SetConvergedState(state typeConvergedState) {
	obj.convergedState = state
}

func (obj *BaseType) GetState() typeState {
	return obj.state
}

func (obj *BaseType) SetState(state typeState) {
	if DEBUG {
		log.Printf("%v[%v]: State: %v -> %v", obj.GetType(), obj.GetName(), obj.GetState(), state)
	}
	obj.state = state
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
		// if they're equal (eg: on init of 0) then we also can't run
		// b/c we should let our pre-req's go first...
		x, y := obj.GetTimestamp(), n.Type.GetTimestamp()
		if DEBUG {
			log.Printf("%v[%v]: OKTimestamp: (%v) >= %v[%v](%v): !%v", obj.GetType(), obj.GetName(), x, n.GetType(), n.GetName(), y, x >= y)
		}
		if x >= y {
			return false
		}
	}
	return true
}

// notify nodes after me in the dependency graph that they need refreshing...
// NOTE: this assumes that this can never fail or need to be rescheduled
func (obj *BaseType) Poke(activity bool) {
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphEdges(v) {
		// XXX: if we're in state event and haven't been cancelled by
		// apply, then we can cancel a poke to a child, right? XXX
		// XXX: if n.Type.GetState() != typeEvent { // is this correct?
		if true { // XXX
			if DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]", v.GetType(), v.GetName(), n.GetType(), n.GetName())
			}
			n.SendEvent(eventPoke, false, activity) // XXX: can this be switched to sync?
		} else {
			if DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]: Skipped!", v.GetType(), v.GetName(), n.GetType(), n.GetName())
			}
		}
	}
}

// poke the pre-requisites that are stale and need to run before I can run...
func (obj *BaseType) BackPoke() {
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		x, y, s := obj.GetTimestamp(), n.Type.GetTimestamp(), n.Type.GetState()
		// if the parent timestamp needs poking AND it's not in state
		// typeEvent, then poke it. If the parent is in typeEvent it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != typeEvent && s != typeApplying) {
			if DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]", v.GetType(), v.GetName(), n.GetType(), n.GetName())
			}
			n.SendEvent(eventBackPoke, false, false) // XXX: can this be switched to sync?
		} else {
			if DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]: Skipped!", v.GetType(), v.GetName(), n.GetType(), n.GetName())
			}
		}
	}
}

// push an event into the message queue for a particular type vertex
func (obj *BaseType) SendEvent(event eventName, sync bool, activity bool) {
	if !sync {
		obj.events <- Event{event, nil, "", activity}
		return
	}

	resp := make(chan bool)
	obj.events <- Event{event, resp, "", activity}
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

	case eventBackPoke:
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
			} else if e.Name == eventStart { // eventContinue
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

// useful for using as: return CleanState() in the StateOK functions when there
// are multiple `true` return exits
func (obj *BaseType) CleanState() bool {
	obj.isStateOK = true
	return true
}

// XXX: rename this function
func Process(obj Type) {
	if DEBUG {
		log.Printf("%v[%v]: Process()", obj.GetType(), obj.GetName())
	}
	obj.SetState(typeEvent)
	var ok bool = true
	var apply bool = false // did we run an apply?
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if obj.OKTimestamp() {
		if DEBUG {
			log.Printf("%v[%v]: OKTimestamp(%v)", obj.GetType(), obj.GetName(), obj.GetTimestamp())
		}
		if !obj.StateOK() { // TODO: can we rename this to something better?
			if DEBUG {
				log.Printf("%v[%v]: !StateOK()", obj.GetType(), obj.GetName())
			}
			// throw an error if apply fails...
			// if this fails, don't UpdateTimestamp()
			obj.SetState(typeApplying)
			if !obj.Apply() { // check for error
				ok = false
			} else {
				apply = true
			}
		}

		if ok {
			// update this timestamp *before* we poke or the poked
			// nodes might fail due to having a too old timestamp!
			obj.UpdateTimestamp()    // this was touched...
			obj.SetState(typePoking) // can't cancel parent poke
			obj.Poke(apply)
		}
		// poke at our pre-req's instead since they need to refresh/run...
	} else {
		// only poke at the pre-req's that need to run
		go obj.BackPoke()
	}
}

func (obj *NoopType) GetType() string {
	return "Noop"
}

func (obj *NoopType) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	//vertex := obj.vertex // stored with SetVertex
	var send = false // send event?
	for {
		obj.SetState(typeWatching) // reset
		select {
		case event := <-obj.events:
			obj.SetConvergedState(typeConvergedNil)
			if ok := obj.ReadEvent(&event); !ok {
				return // exit
			}
			// XXX: should we avoid sending events on UNPAUSE ?
			send = true

		case _ = <-TimeAfterOrBlock(obj.ctimeout):
			obj.SetConvergedState(typeConvergedTimeout)
			obj.converged <- true
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// only do this on certain types of events
			//obj.isStateOK = false // something made state dirty
			Process(obj) // XXX: rename this function
		}
	}
}

func (obj *NoopType) StateOK() bool {
	return true // never needs updating
}

func (obj *NoopType) Apply() bool {
	log.Printf("%v[%v]: Apply", obj.GetType(), obj.GetName())
	return true
}

func (obj *NoopType) Compare(typ Type) bool {
	switch typ.(type) {
	// we can only compare NoopType to others of the same type
	case *NoopType:
		typ := typ.(*NoopType)
		if obj.Name != typ.Name {
			return false
		}
	default:
		return false
	}
	return true
}
