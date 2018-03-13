// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package converger is a facility for reporting the converged state.
package converger

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/util"

	multierr "github.com/hashicorp/go-multierror"
)

// TODO: we could make a new function that masks out the state of certain
// UID's, but at the moment the new Timer code has obsoleted the need...

// Converger is the general interface for implementing a convergence watcher.
type Converger interface { // TODO: need a better name
	Register() UID
	IsConverged(UID) bool         // is the UID converged ?
	SetConverged(UID, bool) error // set the converged state of the UID
	Unregister(UID)
	Start()
	Pause()
	Loop(bool)
	ConvergedTimer(UID) <-chan time.Time
	Status() map[uint64]bool
	Timeout() int                              // returns the timeout that this was created with
	AddStateFn(string, func(bool) error) error // adds a stateFn with a name
	RemoveStateFn(string) error                // remove a stateFn with a given name
}

// UID is the interface resources can use to notify with if converged. You'll
// need to use part of the Converger interface to Register initially too.
type UID interface {
	ID() uint64   // get Id
	Name() string // get a friendly name
	SetName(string)
	IsValid() bool // has Id been initialized ?
	InvalidateID() // set Id to nil
	IsConverged() bool
	SetConverged(bool) error
	Unregister()
	ConvergedTimer() <-chan time.Time
	StartTimer() (func() error, error) // cancellable is the same as StopTimer()
	ResetTimer() error                 // resets counter to zero
	StopTimer() error
}

// converger is an implementation of the Converger interface.
type converger struct {
	timeout   int           // must be zero (instant) or greater seconds to run
	converged bool          // did we converge (state changes of this run Fn)
	channel   chan struct{} // signal here to run an isConverged check
	control   chan bool     // control channel for start/pause
	mutex     *sync.RWMutex // used for controlling access to status and lastid
	lastid    uint64
	status    map[uint64]bool
	stateFns  map[string]func(bool) error // run on converged state changes with state bool
	smutex    *sync.RWMutex               // used for controlling access to stateFns
}

// cuid is an implementation of the UID interface.
type cuid struct {
	converger Converger
	id        uint64
	name      string // user defined, friendly name
	mutex     *sync.Mutex
	timer     chan struct{}
	running   bool // is the above timer running?
	wg        *sync.WaitGroup
}

// NewConverger builds a new converger struct.
func NewConverger(timeout int) Converger {
	return &converger{
		timeout:  timeout,
		channel:  make(chan struct{}),
		control:  make(chan bool),
		mutex:    &sync.RWMutex{},
		lastid:   0,
		status:   make(map[uint64]bool),
		stateFns: make(map[string]func(bool) error),
		smutex:   &sync.RWMutex{},
	}
}

// Register assigns a UID to the caller.
func (obj *converger) Register() UID {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.lastid++
	obj.status[obj.lastid] = false // initialize as not converged
	return &cuid{
		converger: obj,
		id:        obj.lastid,
		name:      fmt.Sprintf("%d", obj.lastid), // some default
		mutex:     &sync.Mutex{},
		timer:     nil,
		running:   false,
		wg:        &sync.WaitGroup{},
	}
}

// IsConverged gets the converged status of a uid.
func (obj *converger) IsConverged(uid UID) bool {
	if !uid.IsValid() {
		panic(fmt.Sprintf("the ID of UID(%s) is nil", uid.Name()))
	}
	obj.mutex.RLock()
	isConverged, found := obj.status[uid.ID()] // lookup
	obj.mutex.RUnlock()
	if !found {
		panic("the ID of UID is unregistered")
	}
	return isConverged
}

// SetConverged updates the converger with the converged state of the UID.
func (obj *converger) SetConverged(uid UID, isConverged bool) error {
	if !uid.IsValid() {
		return fmt.Errorf("the ID of UID(%s) is nil", uid.Name())
	}
	obj.mutex.Lock()
	if _, found := obj.status[uid.ID()]; !found {
		panic("the ID of UID is unregistered")
	}
	obj.status[uid.ID()] = isConverged // set
	obj.mutex.Unlock()                 // unlock *before* poke or deadlock!
	if isConverged != obj.converged {  // only poke if it would be helpful
		// run in a go routine so that we never block... just queue up!
		// this allows us to send events, even if we haven't started...
		go func() { obj.channel <- struct{}{} }()
	}
	return nil
}

// isConverged returns true if *every* registered uid has converged.
func (obj *converger) isConverged() bool {
	obj.mutex.RLock() // take a read lock
	defer obj.mutex.RUnlock()
	for _, v := range obj.status {
		if !v { // everyone must be converged for this to be true
			return false
		}
	}
	return true
}

// Unregister dissociates the ConvergedUID from the converged checking.
func (obj *converger) Unregister(uid UID) {
	if !uid.IsValid() {
		panic(fmt.Sprintf("the ID of UID(%s) is nil", uid.Name()))
	}
	obj.mutex.Lock()
	uid.StopTimer() // ignore any errors
	delete(obj.status, uid.ID())
	obj.mutex.Unlock()
	uid.InvalidateID()
}

// Start causes a Converger object to start or resume running.
func (obj *converger) Start() {
	obj.control <- true
}

// Pause causes a Converger object to stop running temporarily.
func (obj *converger) Pause() { // FIXME: add a sync ACK on pause before return
	obj.control <- false
}

// Loop is the main loop for a Converger object. It usually runs in a goroutine.
// TODO: we could eventually have each resource tell us as soon as it converges,
// and then keep track of the time delays here, to avoid callers needing select.
// NOTE: when we have very short timeouts, if we start before all the resources
// have joined the map, then it might appear as if we converged before we did!
func (obj *converger) Loop(startPaused bool) {
	if obj.control == nil {
		panic("converger not initialized correctly")
	}
	if startPaused { // start paused without racing
		select {
		case e := <-obj.control:
			if !e {
				panic("converger expected true")
			}
		}
	}
	for {
		select {
		case e := <-obj.control: // expecting "false" which means pause!
			if e {
				panic("converger expected false")
			}
			// now i'm paused...
			select {
			case e := <-obj.control:
				if !e {
					panic("converger expected true")
				}
				// restart
				// kick once to refresh the check...
				go func() { obj.channel <- struct{}{} }()
				continue
			}

		case <-obj.channel:
			if !obj.isConverged() {
				if obj.converged { // we're doing a state change
					// call the arbitrary functions (takes a read lock!)
					if err := obj.runStateFns(false); err != nil {
						// FIXME: what to do on error ?
					}
				}
				obj.converged = false
				continue
			}

			// we have converged!
			if obj.timeout >= 0 { // only run if timeout is valid
				if !obj.converged { // we're doing a state change
					// call the arbitrary functions (takes a read lock!)
					if err := obj.runStateFns(true); err != nil {
						// FIXME: what to do on error ?
					}
				}
			}
			obj.converged = true
			// loop and wait again...
		}
	}
}

// ConvergedTimer adds a timeout to a select call and blocks until then.
// TODO: this means we could eventually have per resource converged timeouts
func (obj *converger) ConvergedTimer(uid UID) <-chan time.Time {
	// be clever: if i'm already converged, this timeout should block which
	// avoids unnecessary new signals being sent! this avoids fast loops if
	// we have a low timeout, or in particular a timeout == 0
	if uid.IsConverged() {
		// blocks the case statement in select forever!
		return util.TimeAfterOrBlock(-1)
	}
	return util.TimeAfterOrBlock(obj.timeout)
}

// Status returns a map of the converged status of each UID.
func (obj *converger) Status() map[uint64]bool {
	status := make(map[uint64]bool)
	obj.mutex.RLock() // take a read lock
	defer obj.mutex.RUnlock()
	for k, v := range obj.status { // make a copy to avoid the mutex
		status[k] = v
	}
	return status
}

// Timeout returns the timeout in seconds that converger was created with. This
// is useful to avoid passing in the timeout value separately when you're
// already passing in the Converger struct.
func (obj *converger) Timeout() int {
	return obj.timeout
}

// AddStateFn adds a state function to be run on change of converged state.
func (obj *converger) AddStateFn(name string, stateFn func(bool) error) error {
	obj.smutex.Lock()
	defer obj.smutex.Unlock()
	if _, exists := obj.stateFns[name]; exists {
		return fmt.Errorf("a stateFn with that name already exists")
	}
	obj.stateFns[name] = stateFn
	return nil
}

// RemoveStateFn adds a state function to be run on change of converged state.
func (obj *converger) RemoveStateFn(name string) error {
	obj.smutex.Lock()
	defer obj.smutex.Unlock()
	if _, exists := obj.stateFns[name]; !exists {
		return fmt.Errorf("a stateFn with that name doesn't exist")
	}
	delete(obj.stateFns, name)
	return nil
}

// runStateFns runs the listed of stored state functions.
func (obj *converger) runStateFns(converged bool) error {
	obj.smutex.RLock()
	defer obj.smutex.RUnlock()
	var keys []string
	for k := range obj.stateFns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var err error
	for _, name := range keys { // run in deterministic order
		fn := obj.stateFns[name]
		// call an arbitrary function
		if e := fn(converged); e != nil {
			err = multierr.Append(err, e) // list of errors
		}
	}
	return err
}

// ID returns the unique id of this UID object.
func (obj *cuid) ID() uint64 {
	return obj.id
}

// Name returns a user defined name for the specific cuid.
func (obj *cuid) Name() string {
	return obj.name
}

// SetName sets a user defined name for the specific cuid.
func (obj *cuid) SetName(name string) {
	obj.name = name
}

// IsValid tells us if the id is valid or has already been destroyed.
func (obj *cuid) IsValid() bool {
	return obj.id != 0 // an id of 0 is invalid
}

// InvalidateID marks the id as no longer valid.
func (obj *cuid) InvalidateID() {
	obj.id = 0 // an id of 0 is invalid
}

// IsConverged is a helper function to the regular IsConverged method.
func (obj *cuid) IsConverged() bool {
	return obj.converger.IsConverged(obj)
}

// SetConverged is a helper function to the regular SetConverged notification.
func (obj *cuid) SetConverged(isConverged bool) error {
	return obj.converger.SetConverged(obj, isConverged)
}

// Unregister is a helper function to unregister myself.
func (obj *cuid) Unregister() {
	obj.converger.Unregister(obj)
}

// ConvergedTimer is a helper around the regular ConvergedTimer method.
func (obj *cuid) ConvergedTimer() <-chan time.Time {
	return obj.converger.ConvergedTimer(obj)
}

// StartTimer runs an invisible timer that automatically converges on timeout.
func (obj *cuid) StartTimer() (func() error, error) {
	obj.mutex.Lock()
	if !obj.running {
		obj.timer = make(chan struct{})
		obj.running = true
	} else {
		obj.mutex.Unlock()
		return obj.StopTimer, fmt.Errorf("timer already started")
	}
	obj.mutex.Unlock()
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		for {
			select {
			case _, ok := <-obj.timer: // reset signal channel
				if !ok { // channel is closed
					return // false to exit
				}
				obj.SetConverged(false)

			case <-obj.ConvergedTimer():
				obj.SetConverged(true) // converged!
				select {
				case _, ok := <-obj.timer: // reset signal channel
					if !ok { // channel is closed
						return // false to exit
					}
				}
			}
		}
	}()
	return obj.StopTimer, nil
}

// ResetTimer resets the counter to zero if using a StartTimer internally.
func (obj *cuid) ResetTimer() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if obj.running {
		obj.timer <- struct{}{} // send the reset message
		return nil
	}
	return fmt.Errorf("timer hasn't been started")
}

// StopTimer stops the running timer permanently until a StartTimer is run.
func (obj *cuid) StopTimer() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if !obj.running {
		return fmt.Errorf("timer isn't running")
	}
	close(obj.timer)
	obj.wg.Wait()
	obj.running = false
	return nil
}
