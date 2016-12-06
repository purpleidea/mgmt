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
	"os"
	"path"

	// TODO: should each resource be a sub-package?
	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/global"

	errwrap "github.com/pkg/errors"
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

const refreshPathToken = "refresh"

// Data is the set of input values passed into the pgraph for the resources.
type Data struct {
	//Hostname string         // uuid for the host
	//Noop     bool
	Converger converger.Converger
	Prefix    string // the prefix to be used for the pgraph namespace
	// NOTE: we can add more fields here if needed for the resources.
}

// ResUID is a unique identifier for a resource, namely it's name, and the kind ("type").
type ResUID interface {
	GetName() string
	Kind() string
	IFF(ResUID) bool

	Reversed() bool // true means this resource happens before the generator
}

// The BaseUID struct is used to provide a unique resource identifier.
type BaseUID struct {
	name string // name and kind are the values of where this is coming from
	kind string

	reversed *bool // piggyback edge information here
}

// The AutoEdge interface is used to implement the autoedges feature.
type AutoEdge interface {
	Next() []ResUID   // call to get list of edges to add
	Test([]bool) bool // call until false
}

// MetaParams is a struct will all params that apply to every resource.
type MetaParams struct {
	AutoEdge  bool `yaml:"autoedge"`  // metaparam, should we generate auto edges?
	AutoGroup bool `yaml:"autogroup"` // metaparam, should we auto group?
	Noop      bool `yaml:"noop"`
	// NOTE: there are separate Watch and CheckApply retry and delay values,
	// but I've decided to use the same ones for both until there's a proper
	// reason to want to do something differently for the Watch errors.
	Retry int16  `yaml:"retry"` // metaparam, number of times to retry on error. -1 for infinite
	Delay uint64 `yaml:"delay"` // metaparam, number of milliseconds to wait between retries
}

// UnmarshalYAML is the custom unmarshal handler for the MetaParams struct. It
// is primarily useful for setting the defaults.
func (obj *MetaParams) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawMetaParams MetaParams           // indirection to avoid infinite recursion
	raw := rawMetaParams(DefaultMetaParams) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = MetaParams(raw) // restore from indirection with type conversion!
	return nil
}

// DefaultMetaParams are the defaults to be used for undefined metaparams.
var DefaultMetaParams = MetaParams{
	AutoEdge:  true,
	AutoGroup: true,
	Noop:      false,
	Retry:     0, // TODO: is this a good default?
	Delay:     0, // TODO: is this a good default?
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
	AssociateData(*Data)
	IsWatching() bool
	SetWatching(bool)
	GetState() ResState
	SetState(ResState)
	DoSend(chan event.Event, string) (bool, error)
	SendEvent(event.EventName, bool, bool) bool
	ReadEvent(*event.Event) (bool, bool)   // TODO: optional here?
	Refresh() bool                         // is there a pending refresh to run?
	SetRefresh(bool)                       // set the refresh state of this resource
	SendRecv(Res) (map[string]bool, error) // send->recv data passing function
	IsStateOK() bool
	StateOK(b bool)
	GroupCmp(Res) bool  // TODO: is there a better name for this?
	GroupRes(Res) error // group resource (arg) into self
	IsGrouped() bool    // am I grouped?
	SetGrouped(bool)    // set grouped bool
	GetGroup() []Res    // return everyone grouped inside me
	SetGroup([]Res)
	VarDir(string) (string, error)
}

// Res is the minimum interface you need to implement to define a new resource.
type Res interface {
	Base // include everything from the Base interface
	Init() error
	//Validate() error    // TODO: this might one day be added
	GetUIDs() []ResUID            // most resources only return one
	Watch(chan event.Event) error // send on channel to signal process() events
	CheckApply(apply bool) (checkOK bool, err error)
	AutoEdges() AutoEdge
	Compare(Res) bool
	CollectPattern(string) // XXX: temporary until Res collection is more advanced
}

// BaseRes is the base struct that gets used in every resource.
type BaseRes struct {
	Name       string           `yaml:"name"`
	MetaParams MetaParams       `yaml:"meta"` // struct of all the metaparams
	Recv       map[string]*Send // mapping of key to receive on from value

	kind      string
	events    chan event.Event
	converger converger.Converger // converged tracking
	prefix    string              // base prefix for this resource
	state     ResState
	watching  bool  // is Watch() loop running ?
	isStateOK bool  // whether the state is okay based on events or not
	isGrouped bool  // am i contained within a group?
	grouped   []Res // list of any grouped resources
	refresh   bool  // does this resource have a refresh to run?
	//refreshState StatefulBool // TODO: future stateful bool
}

// UIDExistsInUIDs wraps the IFF method when used with a list of UID's.
func UIDExistsInUIDs(uid ResUID, uids []ResUID) bool {
	for _, u := range uids {
		if uid.IFF(u) {
			return true
		}
	}
	return false
}

// GetName returns the name of the resource.
func (obj *BaseUID) GetName() string {
	return obj.name
}

// Kind returns the kind of resource.
func (obj *BaseUID) Kind() string {
	return obj.kind
}

// IFF looks at two UID's and if and only if they are equivalent, returns true.
// If they are not equivalent, it returns false.
// Most resources will want to override this method, since it does the important
// work of actually discerning if two resources are identical in function.
func (obj *BaseUID) IFF(uid ResUID) bool {
	res, ok := uid.(*BaseUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// Reversed is part of the ResUID interface, and true means this resource
// happens before the generator.
func (obj *BaseUID) Reversed() bool {
	if obj.reversed == nil {
		log.Fatal("Programming error!")
	}
	return *obj.reversed
}

// Init initializes structures like channels if created without New constructor.
func (obj *BaseRes) Init() error {
	if obj.kind == "" {
		return fmt.Errorf("Resource did not set kind!")
	}
	obj.events = make(chan event.Event) // unbuffered chan to avoid stale events
	//dir, err := obj.VarDir("")
	//if err != nil {
	//	return errwrap.Wrapf(err, "VarDir failed in Init()")
	//}
	// TODO: this StatefulBool implementation could be eventually swappable
	//obj.refreshState = &DiskBool{Path: path.Join(dir, refreshPathToken)}
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
func (obj *BaseRes) AssociateData(data *Data) {
	obj.converger = data.Converger
	obj.prefix = data.Prefix
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
		log.Printf("%s[%s]: State: %v -> %v", obj.Kind(), obj.GetName(), obj.GetState(), state)
	}
	obj.state = state
}

// IsStateOK returns the cached state value.
func (obj *BaseRes) IsStateOK() bool {
	return obj.isStateOK
}

// StateOK sets the cached state value.
func (obj *BaseRes) StateOK(b bool) {
	obj.isStateOK = b
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

// Compare is the base compare method, which also handles the metaparams cmp.
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

// VarDir returns the path to a working directory for the resource. It will try
// and create the directory first, and return an error if this failed.
func (obj *BaseRes) VarDir(extra string) (string, error) {
	// Using extra adds additional dirs onto our namespace. An empty extra
	// adds no additional directories.
	if obj.prefix == "" {
		return "", fmt.Errorf("VarDir prefix is empty!")
	}
	if obj.Kind() == "" {
		return "", fmt.Errorf("VarDir kind is empty!")
	}
	if obj.GetName() == "" {
		return "", fmt.Errorf("VarDir name is empty!")
	}

	// FIXME: is obj.GetName() sufficiently unique to use as a UID here?
	uid := obj.GetName()
	p := fmt.Sprintf("%s/", path.Join(obj.prefix, obj.Kind(), uid, extra))
	if err := os.MkdirAll(p, 0770); err != nil {
		return "", errwrap.Wrapf(err, "Can't create prefix for %s[%s]", obj.Kind(), obj.GetName())
	}
	return p, nil
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
