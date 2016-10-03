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

// Package resources provides the resource framework and idempotent primitives.
package resources

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"log"

	// TODO: should each resource be a sub-package?
	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/global"
)

//go:generate stringer -type=ResState -output=resstate_stringer.go

// The ResState type represents the current activity state of each resource.
type ResState int

// Each ResState should be set properly in the relevant part of the resource.
const (
	ResStateNil ResState = iota
	ResStateWatching
	ResStateEvent // an event has happened, but we haven't poked yet
	ResStateCheckApply
	ResStatePoking
)

// ResUUID is a unique identifier for a resource, namely it's name, and the kind ("type").
type ResUUID interface {
	GetName() string
	Kind() string
	IFF(ResUUID) bool

	Reversed() bool // true means this resource happens before the generator
}

// The BaseUUID struct is used to provide a unique resource identifier.
type BaseUUID struct {
	name string // name and kind are the values of where this is coming from
	kind string

	reversed *bool // piggyback edge information here
}

// The AutoEdge interface is used to implement the autoedges feature.
type AutoEdge interface {
	Next() []ResUUID  // call to get list of edges to add
	Test([]bool) bool // call until false
}

// MetaParams is a struct will all params that apply to every resource.
type MetaParams struct {
	AutoEdge  bool `yaml:"autoedge"`  // metaparam, should we generate auto edges? // XXX should default to true
	AutoGroup bool `yaml:"autogroup"` // metaparam, should we auto group? // XXX should default to true
	Noop      bool `yaml:"noop"`
	// NOTE: there are separate Watch and CheckApply retry and delay values,
	// but I've decided to use the same ones for both until there's a proper
	// reason to want to do something differently for the Watch errors.
	Retry int16  `yaml:"retry"` // metaparam, number of times to retry on error. -1 for infinite
	Delay uint64 `yaml:"delay"` // metaparam, number of milliseconds to wait between retries
}

// The Base interface is everything that is common to all resources.
// Everything here only needs to be implemented once, in the BaseRes.
type Base interface {
	GetName() string // can't be named "Name()" because of struct field
	SetName(string)
	SetKind(string)
	Kind() string
	Meta() *MetaParams
	Events() chan event.Event
	AssociateData(converger.Converger)
	IsWatching() bool
	SetWatching(bool)
	GetState() ResState
	SetState(ResState)
	DoSend(chan event.Event, string) (bool, error)
	SendEvent(event.EventName, bool, bool) bool
	ReadEvent(*event.Event) (bool, bool) // TODO: optional here?
	GroupCmp(Res) bool                   // TODO: is there a better name for this?
	GroupRes(Res) error                  // group resource (arg) into self
	IsGrouped() bool                     // am I grouped?
	SetGrouped(bool)                     // set grouped bool
	GetGroup() []Res                     // return everyone grouped inside me
	SetGroup([]Res)
}

// Res is the minimum interface you need to implement to define a new resource.
type Res interface {
	Base // include everything from the Base interface
	Init() error
	//Validate() error    // TODO: this might one day be added
	GetUUIDs() []ResUUID          // most resources only return one
	Watch(chan event.Event) error // send on channel to signal process() events
	CheckApply(bool) (bool, error)
	AutoEdges() AutoEdge
	Compare(Res) bool
	CollectPattern(string) // XXX: temporary until Res collection is more advanced
}

// BaseRes is the base struct that gets used in every resource.
type BaseRes struct {
	Name       string     `yaml:"name"`
	MetaParams MetaParams `yaml:"meta"` // struct of all the metaparams
	kind       string
	events     chan event.Event
	converger  converger.Converger // converged tracking
	state      ResState
	watching   bool  // is Watch() loop running ?
	isStateOK  bool  // whether the state is okay based on events or not
	isGrouped  bool  // am i contained within a group?
	grouped    []Res // list of any grouped resources
}

// UUIDExistsInUUIDs wraps the IFF method when used with a list of UUID's.
func UUIDExistsInUUIDs(uuid ResUUID, uuids []ResUUID) bool {
	for _, u := range uuids {
		if uuid.IFF(u) {
			return true
		}
	}
	return false
}

// GetName returns the name of the resource.
func (obj *BaseUUID) GetName() string {
	return obj.name
}

// Kind returns the kind of resource.
func (obj *BaseUUID) Kind() string {
	return obj.kind
}

// IFF looks at two UUID's and if and only if they are equivalent, returns true.
// If they are not equivalent, it returns false.
// Most resources will want to override this method, since it does the important
// work of actually discerning if two resources are identical in function.
func (obj *BaseUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*BaseUUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// Reversed is part of the ResUUID interface, and true means this resource
// happens before the generator.
func (obj *BaseUUID) Reversed() bool {
	if obj.reversed == nil {
		log.Fatal("Programming error!")
	}
	return *obj.reversed
}

// Init initializes structures like channels if created without New constructor.
func (obj *BaseRes) Init() error {
	obj.events = make(chan event.Event) // unbuffered chan size to avoid stale events
	return nil
}

// GetName is used by all the resources to Get the name.
func (obj *BaseRes) GetName() string {
	return obj.Name
}

// SetName is used to set the name of the resource.
func (obj *BaseRes) SetName(name string) {
	obj.Name = name
}

// SetKind sets the kind. This is used internally for exported resources.
func (obj *BaseRes) SetKind(kind string) {
	obj.kind = kind
}

// Kind returns the kind of resource this is.
func (obj *BaseRes) Kind() string {
	return obj.kind
}

// Meta returns the MetaParams as a reference, which we can then get/set on.
func (obj *BaseRes) Meta() *MetaParams {
	return &obj.MetaParams
}

// Events returns the channel of events to listen on.
func (obj *BaseRes) Events() chan event.Event {
	return obj.events
}

// AssociateData associates some data with the object in question.
func (obj *BaseRes) AssociateData(converger converger.Converger) {
	obj.converger = converger
}

// IsWatching tells us if the Watch() function is running.
func (obj *BaseRes) IsWatching() bool {
	return obj.watching
}

// SetWatching stores the status of if the Watch() function is running.
func (obj *BaseRes) SetWatching(b bool) {
	obj.watching = b
}

// GetState returns the state of the resource.
func (obj *BaseRes) GetState() ResState {
	return obj.state
}

// SetState sets the state of the resource.
func (obj *BaseRes) SetState(state ResState) {
	if global.DEBUG {
		log.Printf("%v[%v]: State: %v -> %v", obj.Kind(), obj.GetName(), obj.GetState(), state)
	}
	obj.state = state
}

// DoSend sends off an event, but doesn't block the incoming event queue. It can
// also recursively call itself when events need processing during the wait.
// I'm not completely comfortable with this fn, but it will have to do for now.
func (obj *BaseRes) DoSend(processChan chan event.Event, comment string) (bool, error) {
	resp := event.NewResp()
	processChan <- event.Event{Name: event.EventNil, Resp: resp, Msg: comment, Activity: true} // trigger process
	e := resp.Wait()
	return false, e // XXX: at the moment, we don't use the exit bool.
	// XXX: this can cause a deadlock. do we need to recursively send? fix event stuff!
	//select {
	//case e := <-resp: // wait for the ACK()
	//	if e != nil { // we got a NACK
	//		return true, e // exit with error
	//	}
	//case event := <-obj.events:
	//	// NOTE: this code should match the similar code below!
	//	//cuuid.SetConverged(false) // TODO ?
	//	if exit, send := obj.ReadEvent(&event); exit {
	//		return true, nil // exit, without error
	//	} else if send {
	//		return obj.DoSend(processChan, comment) // recurse
	//	}
	//}
	//return false, nil // return, no error or exit signal
}

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

// ReadEvent processes events when a select gets one, and handles the pause
// code too! The return values specify if we should exit and poke respectively.
func (obj *BaseRes) ReadEvent(ev *event.Event) (exit, poke bool) {
	ev.ACK()
	switch ev.Name {
	case event.EventStart:
		return false, true

	case event.EventPoke:
		return false, true

	case event.EventBackPoke:
		return false, true // forward poking in response to a back poke!

	case event.EventExit:
		return true, false

	case event.EventPause:
		// wait for next event to continue
		select {
		case e := <-obj.events:
			e.ACK()
			if e.Name == event.EventExit {
				return true, false
			} else if e.Name == event.EventStart { // eventContinue
				return false, false // don't poke on unpause!
			} else {
				// if we get a poke event here, it's a bug!
				log.Fatalf("%v[%v]: Unknown event: %v, while paused!", obj.Kind(), obj.GetName(), e)
			}
		}

	default:
		log.Fatal("Unknown event: ", ev)
	}
	return true, false // required to keep the stupid go compiler happy
}

// GroupCmp compares two resources and decides if they're suitable for grouping
// You'll probably want to override this method when implementing a resource...
func (obj *BaseRes) GroupCmp(res Res) bool {
	return false // base implementation assumes false, override me!
}

// GroupRes groups resource (arg) into self.
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

// IsGrouped determines if we are grouped.
func (obj *BaseRes) IsGrouped() bool { // am I grouped?
	return obj.isGrouped
}

// SetGrouped sets a flag to tell if we are grouped.
func (obj *BaseRes) SetGrouped(b bool) {
	obj.isGrouped = b
}

// GetGroup returns everyone grouped inside me.
func (obj *BaseRes) GetGroup() []Res { // return everyone grouped inside me
	return obj.grouped
}

// SetGroup sets the grouped resources into me.
func (obj *BaseRes) SetGroup(g []Res) {
	obj.grouped = g
}

// Compare is the base compare method, which also handles the metaparams cmp
func (obj *BaseRes) Compare(res Res) bool {
	// TODO: should the AutoEdge values be compared?
	if obj.Meta().AutoEdge != res.Meta().AutoEdge {
		return false
	}
	if obj.Meta().AutoGroup != res.Meta().AutoGroup {
		return false
	}
	if obj.Meta().Noop != res.Meta().Noop {
		// obj is the existing res, res is the *new* resource
		// if we go from no-noop -> noop, we can re-use the obj
		// if we go from noop -> no-noop, we need to regenerate
		if obj.Meta().Noop { // asymmetrical
			return false // going from noop to no-noop!
		}
	}
	if obj.Meta().Retry != res.Meta().Retry {
		return false
	}
	if obj.Meta().Delay != res.Meta().Delay {
		return false
	}
	return true
}

// CollectPattern is used for resource collection.
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
