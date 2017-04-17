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

// Package resources provides the resource framework and idempotent primitives.
package resources

import (
	"fmt"
	"log"
	"math"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	// TODO: should each resource be a sub-package?
	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/prometheus"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
	"golang.org/x/time/rate"
)

var registeredResources = map[string]func() Res{}

// RegisterResource registers a new resource by providing a constructor
// function that returns a resource object ready to be unmarshalled from YAML.
func RegisterResource(name string, creator func() Res) {
	registeredResources[name] = creator
}

// NewEmptyNamedResource returns an empty resource object from a registered
// type, ready to be unmarshalled.
func NewEmptyNamedResource(name string) (Res, error) {
	fn, ok := registeredResources[name]
	if !ok {
		return nil, fmt.Errorf("no resource named %s available", name)
	}
	return fn(), nil
}

//go:generate stringer -type=ResState -output=resstate_stringer.go

// The ResState type represents the current activity state of each resource.
type ResState int

// Each ResState should be set properly in the relevant part of the resource.
const (
	ResStateNil        ResState = iota
	ResStateProcess             // we're in process, but we haven't done much yet
	ResStateCheckApply          // we're about to run CheckApply
	ResStatePoking              // we're done CheckApply, and we're about to poke
)

const refreshPathToken = "refresh"

// World is an interface to the rest of the different graph state. It allows
// the GAPI to store state and exchange information throughout the cluster. It
// is the interface each machine uses to communicate with the rest of the world.
type World interface { // TODO: is there a better name for this interface?
	ResWatch() chan error
	ResExport([]Res) error
	// FIXME: should this method take a "filter" data struct instead of many args?
	ResCollect(hostnameFilter, kindFilter []string) ([]Res, error)

	StrWatch(namespace string) chan error
	StrIsNotExist(error) bool
	StrGet(namespace string) (string, error)
	StrSet(namespace, value string) error
	StrDel(namespace string) error

	StrMapWatch(namespace string) chan error
	StrMapGet(namespace string) (map[string]string, error)
	StrMapSet(namespace, value string) error
	StrMapDel(namespace string) error
}

// Data is the set of input values passed into the pgraph for the resources.
type Data struct {
	Hostname string // uuid for the host
	//Noop     bool
	Converger  converger.Converger
	Prometheus *prometheus.Prometheus
	World      World
	Prefix     string // the prefix to be used for the pgraph namespace
	Debug      bool
	// NOTE: we can add more fields here if needed for the resources.
}

// ResUID is a unique identifier for a resource, namely it's name, and the kind ("type").
type ResUID interface {
	GetName() string
	GetKind() string
	IFF(ResUID) bool

	IsReversed() bool // true means this resource happens before the generator
}

// The BaseUID struct is used to provide a unique resource identifier.
type BaseUID struct {
	Name string // name and kind are the values of where this is coming from
	Kind string

	Reversed *bool // piggyback edge information here
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
	Retry int16      `yaml:"retry"` // metaparam, number of times to retry on error. -1 for infinite
	Delay uint64     `yaml:"delay"` // metaparam, number of milliseconds to wait between retries
	Poll  uint32     `yaml:"poll"`  // metaparam, number of seconds between poll intervals, 0 to watch
	Limit rate.Limit `yaml:"limit"` // metaparam, number of events per second to allow through
	Burst int        `yaml:"burst"` // metaparam, number of events to allow in a burst
	Sema  []string   `yaml:"sema"`  // metaparam, list of semaphore ids (id | id:count)
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
	Retry:     0,        // TODO: is this a good default?
	Delay:     0,        // TODO: is this a good default?
	Poll:      0,        // defaults to watching for events
	Limit:     rate.Inf, // defaults to no limit
	Burst:     0,        // no burst needed on an infinite rate // TODO: is this a good default?
	//Sema:      []string{},
}

// The Base interface is everything that is common to all resources.
// Everything here only needs to be implemented once, in the BaseRes.
type Base interface {
	GetName() string // can't be named "Name()" because of struct field
	SetName(string)
	SetKind(string)
	GetKind() string
	Meta() *MetaParams
	Events() chan *event.Event
	Data() *Data
	IsWorking() bool
	IsQuiescing() bool
	QuiesceGroup() *sync.WaitGroup
	WaitGroup() *sync.WaitGroup
	Setup()
	Reset()
	Converger() converger.Converger
	ConvergerUIDs() (converger.UID, converger.UID, converger.UID)
	GetState() ResState
	SetState(ResState)
	Event() error
	SendEvent(event.Kind, error) error
	ReadEvent(*event.Event) (*error, bool)
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
	Running() error           // notify the engine that Watch started
	Started() <-chan struct{} // returns when the resource has started
	Stopped() <-chan struct{} // returns when the resource has stopped
	Starter(bool)
	Poll() error // poll alternative to watching :(
	ProcessChan() chan *event.Event
	ProcessSync() *sync.WaitGroup
	ProcessExit()
	Prometheus() *prometheus.Prometheus
}

// Res is the minimum interface you need to implement to define a new resource.
type Res interface {
	Base          // include everything from the Base interface
	Default() Res // return a struct with sane defaults as a Res
	Validate() error
	Init() error
	Close() error
	UIDs() []ResUID // most resources only return one
	Watch() error   // send on channel to signal process() events
	CheckApply(apply bool) (checkOK bool, err error)
	AutoEdges() AutoEdge
	Compare(Res) bool
	CollectPattern(string) // XXX: temporary until Res collection is more advanced
	//UnmarshalYAML(unmarshal func(interface{}) error) error // optional
}

// BaseRes is the base struct that gets used in every resource.
type BaseRes struct {
	Name       string           `yaml:"name"`
	MetaParams MetaParams       `yaml:"meta"` // struct of all the metaparams
	Recv       map[string]*Send // mapping of key to receive on from value

	Kind   string
	data   Data
	state  ResState
	prefix string // base prefix for this resource

	eventsLock *sync.Mutex // locks around sending and closing of events channel
	eventsDone bool
	eventsChan chan *event.Event

	processLock *sync.Mutex
	processDone bool
	processChan chan *event.Event
	processSync *sync.WaitGroup

	converger converger.Converger // converged tracking
	cuid      converger.UID
	wcuid     converger.UID
	pcuid     converger.UID

	started   chan struct{} // closed when worker is started/running
	stopped   chan struct{} // closed when worker is stopped/exited
	isStarted bool          // did the started chan already close?
	starter   bool          // does this have indegree == 0 ? XXX: usually?

	quiescing    bool // are we quiescing (pause or exit)
	quiesceGroup *sync.WaitGroup
	waitGroup    *sync.WaitGroup
	working      bool // is the Worker() loop running ?
	debug        bool
	isStateOK    bool // whether the state is okay based on events or not

	isGrouped bool  // am i contained within a group?
	grouped   []Res // list of any grouped resources

	refresh bool // does this resource have a refresh to run?
	//refreshState StatefulBool // TODO: future stateful bool
}

// UnmarshalYAML is the custom unmarshal handler for the BaseRes struct. It is
// primarily useful for setting the defaults, in particular if meta is absent!
// FIXME: how come we can't get this to work properly without dropping fields?
//func (obj *BaseRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
//	DefaultBaseRes := BaseRes{
//		// without specifying a default here, if we don't specify *any*
//		// meta parameters in the yaml file, then the UnmarshalYAML for
//		// the MetaParams struct won't run, and we won't get defaults!
//		MetaParams: DefaultMetaParams, // force a default
//	}

//	type rawBaseRes BaseRes           // indirection to avoid infinite recursion
//	raw := rawBaseRes(DefaultBaseRes) // convert; the defaults go here
//	//raw := rawBaseRes{}

//	if err := unmarshal(&raw); err != nil {
//		return err
//	}

//	*obj = BaseRes(raw) // restore from indirection with type conversion!
//	return nil
//}

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
	return obj.Name
}

// GetKind returns the kind of the resource.
func (obj *BaseUID) GetKind() string {
	return obj.Kind
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
	return obj.Name == res.Name
}

// IsReversed is part of the ResUID interface, and true means this resource
// happens before the generator.
func (obj *BaseUID) IsReversed() bool {
	if obj.Reversed == nil {
		log.Fatal("Programming error!")
	}
	return *obj.Reversed
}

// Validate reports any problems with the struct definition.
func (obj *BaseRes) Validate() error {
	isInf := obj.Meta().Limit == rate.Inf || math.IsInf(float64(obj.Meta().Limit), 1)
	if obj.Meta().Burst == 0 && !isInf { // blocked
		return fmt.Errorf("Permanently limited (rate != Inf, burst: 0)")
	}
	return nil
}

// Init initializes structures like channels if created without New constructor.
func (obj *BaseRes) Init() error {
	if obj.debug {
		log.Printf("%s[%s]: Init()", obj.GetKind(), obj.GetName())
	}
	if obj.Kind == "" {
		return fmt.Errorf("resource did not set kind")
	}

	obj.cuid = obj.Converger().Register()
	obj.wcuid = obj.Converger().Register() // get a cuid for the worker!
	obj.pcuid = obj.Converger().Register() // get a cuid for the process

	obj.processLock = &sync.Mutex{} // lock around processChan closing and sending
	obj.processDone = false         // did we close processChan ?
	obj.processChan = make(chan *event.Event)
	obj.processSync = &sync.WaitGroup{}

	obj.quiescing = false // no quiesce operation is happening at the moment
	obj.quiesceGroup = &sync.WaitGroup{}

	obj.waitGroup = &sync.WaitGroup{} // Init and Close must be 1-1 matched!
	obj.waitGroup.Add(1)
	obj.working = true // Worker method should now be running...

	// FIXME: force a sane default until UnmarshalYAML on *BaseRes works...
	if obj.Meta().Burst == 0 && obj.Meta().Limit == 0 { // blocked
		obj.Meta().Limit = rate.Inf
	}
	if math.IsInf(float64(obj.Meta().Limit), 1) { // yaml `.inf` -> rate.Inf
		obj.Meta().Limit = rate.Inf
	}

	//dir, err := obj.VarDir("")
	//if err != nil {
	//	return errwrap.Wrapf(err, "the VarDir failed in Init()")
	//}
	// TODO: this StatefulBool implementation could be eventually swappable
	//obj.refreshState = &DiskBool{Path: path.Join(dir, refreshPathToken)}

	if err := obj.Prometheus().AddManagedResource(fmt.Sprintf("%s[%s]", obj.GetKind(), obj.GetName()), obj.GetKind()); err != nil {
		return errwrap.Wrapf(err, "could not increase prometheus counter!")
	}

	return nil
}

// Close shuts down and performs any cleanup.
func (obj *BaseRes) Close() error {
	if obj.debug {
		log.Printf("%s[%s]: Close()", obj.GetKind(), obj.GetName())
	}

	obj.pcuid.Unregister()
	obj.wcuid.Unregister()
	obj.cuid.Unregister()

	obj.working = false // Worker method should now be closing...
	close(obj.stopped)
	obj.waitGroup.Done()

	if err := obj.Prometheus().RemoveManagedResource(fmt.Sprintf("%s[%s]", obj.GetKind(), obj.GetName()), obj.GetKind()); err != nil {
		return errwrap.Wrapf(err, "could not decrease prometheus counter!")
	}

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
	obj.Kind = kind
}

// GetKind returns the kind of resource this is.
func (obj *BaseRes) GetKind() string {
	return obj.Kind
}

// Meta returns the MetaParams as a reference, which we can then get/set on.
func (obj *BaseRes) Meta() *MetaParams {
	return &obj.MetaParams
}

// Events returns the channel of events to listen on.
func (obj *BaseRes) Events() chan *event.Event {
	return obj.eventsChan
}

// Data returns an associable handle to some data passed in to the resource.
func (obj *BaseRes) Data() *Data {
	return &obj.data
}

// IsWorking tells us if the Worker() function is running. Not thread safe.
func (obj *BaseRes) IsWorking() bool {
	return obj.working
}

// IsQuiescing returns if there is a quiesce operation in progress. Pause and
// exit both meet this criteria, and this tells some systems to wind down, such
// as the event replay mechanism.
func (obj *BaseRes) IsQuiescing() bool { return obj.quiescing }

// QuiesceGroup returns the sync group associated with the quiesce operations.
func (obj *BaseRes) QuiesceGroup() *sync.WaitGroup { return obj.quiesceGroup }

// WaitGroup returns a sync.WaitGroup which is open when the resource is done.
// This is more useful than a closed channel signal, since it can be re-used
// safely without having to recreate it and worry about stale channel handles.
func (obj *BaseRes) WaitGroup() *sync.WaitGroup { return obj.waitGroup }

// Setup does some work which must happen before the Worker starts. It happens
// once per Worker startup. It can happen in parallel with other Setup calls, so
// add locks around any operation that's not thread-safe.
func (obj *BaseRes) Setup() {
	obj.started = make(chan struct{}) // closes when started
	obj.stopped = make(chan struct{}) // closes when stopped

	obj.eventsLock = &sync.Mutex{}
	obj.eventsDone = false
	obj.eventsChan = make(chan *event.Event) // unbuffered chan to avoid stale events
}

// Reset from Setup. These can get called for different vertices in parallel.
func (obj *BaseRes) Reset() {
	return
}

// Converger returns the converger object used by the system. It can be used to
// register new convergers if needed.
func (obj *BaseRes) Converger() converger.Converger {
	return obj.data.Converger
}

// ConvergerUIDs returns the ConvergerUIDs for the resource. This is called by
// the various methods that need one of these ConvergerUIDs. They are registered
// by the Init method and unregistered on the resource Close.
func (obj *BaseRes) ConvergerUIDs() (cuid, wcuid, pcuid converger.UID) {
	return obj.cuid, obj.wcuid, obj.pcuid
}

// GetState returns the state of the resource.
func (obj *BaseRes) GetState() ResState {
	return obj.state
}

// SetState sets the state of the resource.
func (obj *BaseRes) SetState(state ResState) {
	if obj.debug {
		log.Printf("%s[%s]: State: %v -> %v", obj.GetKind(), obj.GetName(), obj.GetState(), state)
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

// ProcessChan returns the chan that resources send events to. Internal API!
func (obj *BaseRes) ProcessChan() chan *event.Event { return obj.processChan }

// ProcessSync returns the WaitGroup that blocks until the innerWorker closes.
func (obj *BaseRes) ProcessSync() *sync.WaitGroup { return obj.processSync }

// ProcessExit causes the innerWorker to close and waits until it does so.
func (obj *BaseRes) ProcessExit() {
	obj.processLock.Lock() // lock to avoid a send when closed!
	obj.processDone = true
	close(obj.processChan)
	obj.processLock.Unlock()
	obj.processSync.Wait()
}

// GroupCmp compares two resources and decides if they're suitable for grouping
// You'll probably want to override this method when implementing a resource...
func (obj *BaseRes) GroupCmp(res Res) bool {
	return false // base implementation assumes false, override me!
}

// GroupRes groups resource (arg) into self.
func (obj *BaseRes) GroupRes(res Res) error {
	if l := len(res.GetGroup()); l > 0 {
		return fmt.Errorf("the %v resource already contains %d grouped resources", res, l)
	}
	if res.IsGrouped() {
		return fmt.Errorf("the %v resource is already grouped", res)
	}

	// merging two resources into one should yield the sum of their semas
	if semas := res.Meta().Sema; len(semas) > 0 {
		obj.Meta().Sema = append(obj.Meta().Sema, semas...)
		obj.Meta().Sema = util.StrRemoveDuplicatesInList(obj.Meta().Sema)
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
	if obj.Meta().Poll != res.Meta().Poll {
		return false
	}
	if obj.Meta().Limit != res.Meta().Limit {
		return false
	}
	if obj.Meta().Burst != res.Meta().Burst {
		return false
	}

	// are the two slices the same?
	cmpSlices := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		sort.Strings(a)
		sort.Strings(b)
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	if !cmpSlices(obj.Meta().Sema, res.Meta().Sema) {
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
		return "", fmt.Errorf("the VarDir prefix is empty")
	}
	if obj.GetKind() == "" {
		return "", fmt.Errorf("the VarDir kind is empty")
	}
	if obj.GetName() == "" {
		return "", fmt.Errorf("the VarDir name is empty")
	}

	// FIXME: is obj.GetName() sufficiently unique to use as a UID here?
	uid := obj.GetName()
	p := fmt.Sprintf("%s/", path.Join(obj.prefix, obj.GetKind(), uid, extra))
	if err := os.MkdirAll(p, 0770); err != nil {
		return "", errwrap.Wrapf(err, "can't create prefix for %s[%s]", obj.GetKind(), obj.GetName())
	}
	return p, nil
}

// Started returns a channel that closes when the resource has started up.
func (obj *BaseRes) Started() <-chan struct{} { return obj.started }

// Stopped returns a channel that closes when the worker has finished running.
func (obj *BaseRes) Stopped() <-chan struct{} { return obj.stopped }

// Starter sets the starter bool. This defines if a vertex has an indegree of 0.
// If we have an indegree of 0, we'll need to be a poke initiator in the graph.
func (obj *BaseRes) Starter(b bool) { obj.starter = b }

// Poll is the watch replacement for when we want to poll, which outputs events.
func (obj *BaseRes) Poll() error {
	cuid, _, _ := obj.ConvergerUIDs() // get the converger uid used to report status

	// create a time.Ticker for the given interval
	ticker := time.NewTicker(time.Duration(obj.Meta().Poll) * time.Second)
	defer ticker.Stop()

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}
	cuid.SetConverged(false) // quickly stop any converge due to Running()

	var send = false
	var exit *error
	for {
		select {
		case <-ticker.C: // received the timer event
			log.Printf("%s[%s]: polling...", obj.GetKind(), obj.GetName())
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			cuid.ResetTimer() // important
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		if send {
			send = false
			obj.Event()
		}
	}
}

// Prometheus returns the prometheus instance.
func (obj *BaseRes) Prometheus() *prometheus.Prometheus {
	return obj.Data().Prometheus
}
