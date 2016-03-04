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

//go:generate stringer -type=resState -output=resstate_stringer.go
type resState int

const (
	resStateNil resState = iota
	resStateWatching
	resStateEvent // an event has happened, but we haven't poked yet
	resStateCheckApply
	resStatePoking
)

//go:generate stringer -type=resConvergedState -output=resconvergedstate_stringer.go
type resConvergedState int

const (
	resConvergedNil resConvergedState = iota
	//resConverged
	resConvergedTimeout
)

// a unique identifier for a resource, namely it's name, and the kind ("type")
type ResUUID interface {
	GetName() string
	Kind() string
	IFF(ResUUID) bool

	Reversed() bool // true means this resource happens before the generator
}

type BaseUUID struct {
	name string
	kind string

	reversed *bool // piggyback edge information here
}

type AutoEdge interface {
	Next() []ResUUID  // call to get list of edges to add
	Test([]bool) bool // call until false
}

type MetaParams struct {
	AutoEdge bool `yaml:"autoedge"` // metaparam, should we generate auto edges? // XXX should default to true
}

type Res interface {
	Init()
	GetName() string     // can't be named "Name()" because of struct field
	GetUUIDs() []ResUUID // most resources only return one
	GetMeta() MetaParams
	Kind() string
	Watch()
	CheckApply(bool) (bool, error)
	AutoEdges() AutoEdge
	SetVertex(*Vertex)
	SetConvergedCallback(ctimeout int, converged chan bool)
	Compare(Res) bool
	SendEvent(eventName, bool, bool) bool
	IsWatching() bool
	SetWatching(bool)
	GetConvergedState() resConvergedState
	SetConvergedState(resConvergedState)
	GetState() resState
	SetState(resState)
	GetTimestamp() int64
	UpdateTimestamp() int64
	OKTimestamp() bool
	Poke(bool)
	BackPoke()
}

type BaseRes struct {
	Name           string     `yaml:"name"`
	Meta           MetaParams `yaml:"meta"` // struct of all the metaparams
	kind           string
	timestamp      int64 // last updated timestamp ?
	events         chan Event
	vertex         *Vertex
	state          resState
	convergedState resConvergedState
	watching       bool // is Watch() loop running ?
	ctimeout       int  // converged timeout
	converged      chan bool
	isStateOK      bool // whether the state is okay based on events or not
}

type NoopRes struct {
	BaseRes `yaml:",inline"`
	Comment string `yaml:"comment"` // extra field for example purposes
}

func NewNoopRes(name string) *NoopRes {
	// FIXME: we could get rid of this New constructor and use raw object creation with a required Init()
	obj := &NoopRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Comment: "",
	}
	obj.Init()
	return obj
}

// wraps the IFF method when used with a list of UUID's
func UUIDExistsInUUIDs(uuid ResUUID, uuids []ResUUID) bool {
	for _, u := range uuids {
		if uuid.IFF(u) {
			return true
		}
	}
	return false
}

func (obj *BaseUUID) GetName() string {
	return obj.name
}

func (obj *BaseUUID) Kind() string {
	return obj.kind
}

// if and only if they are equivalent, return true
// if they are not equivalent, return false
// most resource will want to override this method, since it does the important
// work of actually discerning if two resources are identical in function
func (obj *BaseUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*BaseUUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

func (obj *BaseUUID) Reversed() bool {
	if obj.reversed == nil {
		log.Fatal("Programming error!")
	}
	return *obj.reversed
}

// initialize structures like channels if created without New constructor
func (obj *BaseRes) Init() {
	obj.events = make(chan Event) // unbuffered chan size to avoid stale events
}

// this method gets used by all the resources, if we have one of (obj NoopRes) it would get overridden in that case!
func (obj *BaseRes) GetName() string {
	return obj.Name
}

// return the kind of resource this is
func (obj *BaseRes) Kind() string {
	return obj.kind
}

func (obj *BaseRes) GetMeta() MetaParams {
	return obj.Meta
}

func (obj *BaseRes) GetVertex() *Vertex {
	return obj.vertex
}

func (obj *BaseRes) SetVertex(v *Vertex) {
	obj.vertex = v
}

func (obj *BaseRes) SetConvergedCallback(ctimeout int, converged chan bool) {
	obj.ctimeout = ctimeout
	obj.converged = converged
}

// is the Watch() function running?
func (obj *BaseRes) IsWatching() bool {
	return obj.watching
}

// store status of if the Watch() function is running
func (obj *BaseRes) SetWatching(b bool) {
	obj.watching = b
}

func (obj *BaseRes) GetConvergedState() resConvergedState {
	return obj.convergedState
}

func (obj *BaseRes) SetConvergedState(state resConvergedState) {
	obj.convergedState = state
}

func (obj *BaseRes) GetState() resState {
	return obj.state
}

func (obj *BaseRes) SetState(state resState) {
	if DEBUG {
		log.Printf("%v[%v]: State: %v -> %v", obj.Kind(), obj.GetName(), obj.GetState(), state)
	}
	obj.state = state
}

// GetTimestamp returns the timestamp of a vertex
func (obj *BaseRes) GetTimestamp() int64 {
	return obj.timestamp
}

// UpdateTimestamp updates the timestamp on a vertex and returns the new value
func (obj *BaseRes) UpdateTimestamp() int64 {
	obj.timestamp = time.Now().UnixNano() // update
	return obj.timestamp
}

// can this element run right now?
func (obj *BaseRes) OKTimestamp() bool {
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		// if the vertex has a greater timestamp than any pre-req (n)
		// then we can't run right now...
		// if they're equal (eg: on init of 0) then we also can't run
		// b/c we should let our pre-req's go first...
		x, y := obj.GetTimestamp(), n.Res.GetTimestamp()
		if DEBUG {
			log.Printf("%v[%v]: OKTimestamp: (%v) >= %v[%v](%v): !%v", obj.Kind(), obj.GetName(), x, n.Kind(), n.GetName(), y, x >= y)
		}
		if x >= y {
			return false
		}
	}
	return true
}

// notify nodes after me in the dependency graph that they need refreshing...
// NOTE: this assumes that this can never fail or need to be rescheduled
func (obj *BaseRes) Poke(activity bool) {
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphEdges(v) {
		// XXX: if we're in state event and haven't been cancelled by
		// apply, then we can cancel a poke to a child, right? XXX
		// XXX: if n.Res.GetState() != resStateEvent { // is this correct?
		if true { // XXX
			if DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			n.SendEvent(eventPoke, false, activity) // XXX: can this be switched to sync?
		} else {
			if DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
}

// poke the pre-requisites that are stale and need to run before I can run...
func (obj *BaseRes) BackPoke() {
	v := obj.GetVertex()
	g := v.GetGraph()
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		x, y, s := obj.GetTimestamp(), n.Res.GetTimestamp(), n.Res.GetState()
		// if the parent timestamp needs poking AND it's not in state
		// resStateEvent, then poke it. If the parent is in resStateEvent it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != resStateEvent && s != resStateCheckApply) {
			if DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			n.SendEvent(eventBackPoke, false, false) // XXX: can this be switched to sync?
		} else {
			if DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
}

// push an event into the message queue for a particular vertex
func (obj *BaseRes) SendEvent(event eventName, sync bool, activity bool) bool {
	// TODO: isn't this race-y ?
	if !obj.IsWatching() { // element has already exited
		return false // if we don't return, we'll block on the send
	}
	if !sync {
		obj.events <- Event{event, nil, "", activity}
		return true
	}

	resp := make(chan bool)
	obj.events <- Event{event, resp, "", activity}
	for {
		value := <-resp
		// wait until true value
		if value {
			return true
		}
	}
}

// process events when a select gets one, this handles the pause code too!
// the return values specify if we should exit and poke respectively
func (obj *BaseRes) ReadEvent(event *Event) (exit, poke bool) {
	event.ACK()
	switch event.Name {
	case eventStart:
		return false, true

	case eventPoke:
		return false, true

	case eventBackPoke:
		return false, true // forward poking in response to a back poke!

	case eventExit:
		return true, false

	case eventPause:
		// wait for next event to continue
		select {
		case e := <-obj.events:
			e.ACK()
			if e.Name == eventExit {
				return true, false
			} else if e.Name == eventStart { // eventContinue
				return false, false // don't poke on unpause!
			} else {
				// if we get a poke event here, it's a bug!
				log.Fatalf("%v[%v]: Unknown event: %v, while paused!", obj.Kind(), obj.GetName(), e)
			}
		}

	default:
		log.Fatal("Unknown event: ", event)
	}
	return true, false // required to keep the stupid go compiler happy
}

// XXX: rename this function
func Process(obj Res) {
	if DEBUG {
		log.Printf("%v[%v]: Process()", obj.Kind(), obj.GetName())
	}
	obj.SetState(resStateEvent)
	var ok = true
	var apply = false // did we run an apply?
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if obj.OKTimestamp() {
		if DEBUG {
			log.Printf("%v[%v]: OKTimestamp(%v)", obj.Kind(), obj.GetName(), obj.GetTimestamp())
		}

		obj.SetState(resStateCheckApply)
		// if this fails, don't UpdateTimestamp()
		stateok, err := obj.CheckApply(true)
		if stateok && err != nil { // should never return this way
			log.Fatalf("%v[%v]: CheckApply(): %t, %+v", obj.Kind(), obj.GetName(), stateok, err)
		}
		if DEBUG {
			log.Printf("%v[%v]: CheckApply(): %t, %v", obj.Kind(), obj.GetName(), stateok, err)
		}

		if !stateok { // if state *was* not ok, we had to have apply'ed
			if err != nil { // error during check or apply
				ok = false
			} else {
				apply = true
			}
		}

		if ok {
			// update this timestamp *before* we poke or the poked
			// nodes might fail due to having a too old timestamp!
			obj.UpdateTimestamp()        // this was touched...
			obj.SetState(resStatePoking) // can't cancel parent poke
			obj.Poke(apply)
		}
		// poke at our pre-req's instead since they need to refresh/run...
	} else {
		// only poke at the pre-req's that need to run
		go obj.BackPoke()
	}
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

func (obj *NoopRes) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	//vertex := obj.vertex // stored with SetVertex
	var send = false // send event?
	var exit = false
	for {
		obj.SetState(resStateWatching) // reset
		select {
		case event := <-obj.events:
			obj.SetConvergedState(resConvergedNil)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(&event); exit {
				return // exit
			}

		case _ = <-TimeAfterOrBlock(obj.ctimeout):
			obj.SetConvergedState(resConvergedTimeout)
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

// CheckApply method for Noop resource. Does nothing, returns happy!
func (obj *NoopRes) CheckApply(apply bool) (stateok bool, err error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)
	return true, nil // state is always okay
}

type NoopUUID struct {
	BaseUUID
}

func (obj *NoopRes) AutoEdges() AutoEdge {
	return nil
}

// include all params to make a unique identification of this object
// most resources only return one
func (obj *NoopRes) GetUUIDs() []ResUUID {
	x := &NoopUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
	}
	return []ResUUID{x}
}

func (obj *NoopRes) Compare(res Res) bool {
	switch res.(type) {
	// we can only compare NoopRes to others of the same resource
	case *NoopRes:
		res := res.(*NoopRes)
		if obj.Name != res.Name {
			return false
		}
	default:
		return false
	}
	return true
}
