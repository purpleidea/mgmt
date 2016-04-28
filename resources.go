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
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"log"
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

// a unique identifier for a resource, namely it's name, and the kind ("type")
type ResUUID interface {
	GetName() string
	Kind() string
	IFF(ResUUID) bool

	Reversed() bool // true means this resource happens before the generator
}

type BaseUUID struct {
	name string // name and kind are the values of where this is coming from
	kind string

	reversed *bool // piggyback edge information here
}

type AutoEdge interface {
	Next() []ResUUID  // call to get list of edges to add
	Test([]bool) bool // call until false
}

type MetaParams struct {
	AutoEdge  bool `yaml:"autoedge"`  // metaparam, should we generate auto edges? // XXX should default to true
	AutoGroup bool `yaml:"autogroup"` // metaparam, should we auto group? // XXX should default to true
}

// this interface is everything that is common to all resources
// everything here only needs to be implemented once, in the BaseRes
type Base interface {
	GetName() string // can't be named "Name()" because of struct field
	SetName(string)
	Kind() string
	GetMeta() MetaParams
	AssociateData(Converger)
	IsWatching() bool
	SetWatching(bool)
	GetState() resState
	SetState(resState)
	SendEvent(eventName, bool, bool) bool
	ReadEvent(*Event) (bool, bool) // TODO: optional here?
	GroupCmp(Res) bool             // TODO: is there a better name for this?
	GroupRes(Res) error            // group resource (arg) into self
	IsGrouped() bool               // am I grouped?
	SetGrouped(bool)               // set grouped bool
	GetGroup() []Res               // return everyone grouped inside me
	SetGroup([]Res)
}

// this is the minimum interface you need to implement to make a new resource
type Res interface {
	Base // include everything from the Base interface
	Init()
	//Validate() bool    // TODO: this might one day be added
	GetUUIDs() []ResUUID // most resources only return one
	Watch(chan Event)    // send on channel to signal process() events
	CheckApply(bool) (bool, error)
	AutoEdges() AutoEdge
	Compare(Res) bool
	CollectPattern(string) // XXX: temporary until Res collection is more advanced
}

type BaseRes struct {
	Name      string     `yaml:"name"`
	Meta      MetaParams `yaml:"meta"` // struct of all the metaparams
	kind      string
	events    chan Event
	converger Converger // converged tracking
	state     resState
	watching  bool  // is Watch() loop running ?
	isStateOK bool  // whether the state is okay based on events or not
	isGrouped bool  // am i contained within a group?
	grouped   []Res // list of any grouped resources
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

// this method gets used by all the resources
func (obj *BaseRes) GetName() string {
	return obj.Name
}

func (obj *BaseRes) SetName(name string) {
	obj.Name = name
}

// return the kind of resource this is
func (obj *BaseRes) Kind() string {
	return obj.kind
}

func (obj *BaseRes) GetMeta() MetaParams {
	return obj.Meta
}

// AssociateData associates some data with the object in question
func (obj *BaseRes) AssociateData(converger Converger) {
	obj.converger = converger
}

// is the Watch() function running?
func (obj *BaseRes) IsWatching() bool {
	return obj.watching
}

// store status of if the Watch() function is running
func (obj *BaseRes) SetWatching(b bool) {
	obj.watching = b
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

// GroupCmp compares two resources and decides if they're suitable for grouping
// You'll probably want to override this method when implementing a resource...
func (obj *BaseRes) GroupCmp(res Res) bool {
	return false // base implementation assumes false, override me!
}

func (obj *BaseRes) GroupRes(res Res) error {
	if l := len(res.GetGroup()); l > 0 {
		return fmt.Errorf("Res: %v already contains %d grouped resources!", res, l)
	}
	if res.IsGrouped() {
		return fmt.Errorf("Res: %v is already grouped!", res)
	}

	obj.grouped = append(obj.grouped, res)
	res.SetGrouped(true) // i am contained _in_ a group
	return nil
}

func (obj *BaseRes) IsGrouped() bool { // am I grouped?
	return obj.isGrouped
}

func (obj *BaseRes) SetGrouped(b bool) {
	obj.isGrouped = b
}

func (obj *BaseRes) GetGroup() []Res { // return everyone grouped inside me
	return obj.grouped
}

func (obj *BaseRes) SetGroup(g []Res) {
	obj.grouped = g
}

func (obj *BaseRes) CollectPattern(pattern string) {
	// XXX: default method is empty
}

// ResToB64 encodes a resource to a base64 encoded string (after serialization)
func ResToB64(res Res) (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(&res) // pass with &
	if err != nil {
		return "", fmt.Errorf("Gob failed to encode: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// B64ToRes decodes a resource from a base64 encoded string (after deserialization)
func B64ToRes(str string) (Res, error) {
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, fmt.Errorf("Base64 failed to decode: %v", err)
	}
	b := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b)
	err = d.Decode(&output) // pass with &
	if err != nil {
		return nil, fmt.Errorf("Gob failed to decode: %v", err)
	}
	res, ok := output.(Res)
	if !ok {
		return nil, fmt.Errorf("Output %v is not a Res", res)

	}
	return res, nil
}
