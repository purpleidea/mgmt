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

// Package converger is a facility for reporting the converged state.
package converger

import (
	"fmt"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/util"
)

// TODO: we could make a new function that masks out the state of certain
// UID's, but at the moment the new Timer code has obsoleted the need...

// Converger is the general interface for implementing a convergence watcher
type Converger interface { // TODO: need a better name
	Register() ConvergerUID
	IsConverged(ConvergerUID) bool         // is the UID converged ?
	SetConverged(ConvergerUID, bool) error // set the converged state of the UID
	Unregister(ConvergerUID)
	Start()
	Pause()
	Loop(bool)
	ConvergedTimer(ConvergerUID) <-chan time.Time
	Status() map[uint64]bool
	Timeout() int                // returns the timeout that this was created with
	SetStateFn(func(bool) error) // sets the stateFn
}

// ConvergerUID is the interface resources can use to notify with if converged
// you'll need to use part of the Converger interface to Register initially too
type ConvergerUID interface {
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

// converger is an implementation of the Converger interface
type converger struct {
	timeout   int              // must be zero (instant) or greater seconds to run
	stateFn   func(bool) error // run on converged state changes with state bool
	converged bool             // did we converge (state changes of this run Fn)
	channel   chan struct{}    // signal here to run an isConverged check
	control   chan bool        // control channel for start/pause
	mutex     sync.RWMutex     // used for controlling access to status and lastid
	lastid    uint64
	status    map[uint64]bool
}

// convergerUID is an implementation of the ConvergerUID interface
type convergerUID struct {
	converger Converger
	id        uint64
	name      string // user defined, friendly name
	mutex     sync.Mutex
	timer     chan struct{}
	running   bool // is the above timer running?
}

// NewConverger builds a new converger struct
func NewConverger(timeout int, stateFn func(bool) error) *converger {
	return &converger{
		timeout: timeout,
		stateFn: stateFn,
		channel: make(chan struct{}),
		control: make(chan bool),
		lastid:  0,
		status:  make(map[uint64]bool),
	}
}

// Register assigns a ConvergerUID to the caller
func (obj *converger) Register() ConvergerUID {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.lastid++
	obj.status[obj.lastid] = false // initialize as not converged
	return &convergerUID{
		converger: obj,
		id:        obj.lastid,
		name:      fmt.Sprintf("%d", obj.lastid), // some default
		timer:     nil,
		running:   false,
	}
}

// IsConverged gets the converged status of a uid
func (obj *converger) IsConverged(uid ConvergerUID) bool {
	if !uid.IsValid() {
		panic(fmt.Sprintf("Id of ConvergerUID(%s) is nil!", uid.Name()))
	}
	obj.mutex.RLock()
	isConverged, found := obj.status[uid.ID()] // lookup
	obj.mutex.RUnlock()
	if !found {
		panic("Id of ConvergerUID is unregistered!")
	}
	return isConverged
}

// SetConverged updates the converger with the converged state of the UID
func (obj *converger) SetConverged(uid ConvergerUID, isConverged bool) error {
	if !uid.IsValid() {
		return fmt.Errorf("Id of ConvergerUID(%s) is nil!", uid.Name())
	}
	obj.mutex.Lock()
	if _, found := obj.status[uid.ID()]; !found {
		panic("Id of ConvergerUID is unregistered!")
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

// isConverged returns true if *every* registered uid has converged
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

// Unregister dissociates the ConvergedUID from the converged checking
func (obj *converger) Unregister(uid ConvergerUID) {
	if !uid.IsValid() {
		panic(fmt.Sprintf("Id of ConvergerUID(%s) is nil!", uid.Name()))
	}
	obj.mutex.Lock()
	uid.StopTimer() // ignore any errors
	delete(obj.status, uid.ID())
	obj.mutex.Unlock()
	uid.InvalidateID()
}

// Start causes a Converger object to start or resume running
func (obj *converger) Start() {
	obj.control <- true
}

// Pause causes a Converger object to stop running temporarily
func (obj *converger) Pause() { // FIXME: add a sync ACK on pause before return
	obj.control <- false
}

// Loop is the main loop for a Converger object; it usually runs in a goroutine
// TODO: we could eventually have each resource tell us as soon as it converges
// and then keep track of the time delays here, to avoid callers needing select
// NOTE: when we have very short timeouts, if we start before all the resources
// have joined the map, then it might appears as if we converged before we did!
func (obj *converger) Loop(startPaused bool) {
	if obj.control == nil {
		panic("Converger not initialized correctly")
	}
	if startPaused { // start paused without racing
		select {
		case e := <-obj.control:
			if !e {
				panic("Converger expected true!")
			}
		}
	}
	for {
		select {
		case e := <-obj.control: // expecting "false" which means pause!
			if e {
				panic("Converger expected false!")
			}
			// now i'm paused...
			select {
			case e := <-obj.control:
				if !e {
					panic("Converger expected true!")
				}
				// restart
				// kick once to refresh the check...
				go func() { obj.channel <- struct{}{} }()
				continue
			}

		case <-obj.channel:
			if !obj.isConverged() {
				if obj.converged { // we're doing a state change
					if obj.stateFn != nil {
						// call an arbitrary function
						if err := obj.stateFn(false); err != nil {
							// FIXME: what to do on error ?
						}
					}
				}
				obj.converged = false
				continue
			}

			// we have converged!
			if obj.timeout >= 0 { // only run if timeout is valid
				if !obj.converged { // we're doing a state change
					if obj.stateFn != nil {
						// call an arbitrary function
						if err := obj.stateFn(true); err != nil {
							// FIXME: what to do on error ?
						}
					}
				}
			}
			obj.converged = true
			// loop and wait again...
		}
	}
}

// ConvergedTimer adds a timeout to a select call and blocks until then
// TODO: this means we could eventually have per resource converged timeouts
func (obj *converger) ConvergedTimer(uid ConvergerUID) <-chan time.Time {
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

// SetStateFn sets the state function to be run on change of converged state.
func (obj *converger) SetStateFn(stateFn func(bool) error) {
	obj.stateFn = stateFn
}

// Id returns the unique id of this UID object
func (obj *convergerUID) ID() uint64 {
	return obj.id
}

// Name returns a user defined name for the specific convergerUID.
func (obj *convergerUID) Name() string {
	return obj.name
}

// SetName sets a user defined name for the specific convergerUID.
func (obj *convergerUID) SetName(name string) {
	obj.name = name
}

// IsValid tells us if the id is valid or has already been destroyed
func (obj *convergerUID) IsValid() bool {
	return obj.id != 0 // an id of 0 is invalid
}

// InvalidateID marks the id as no longer valid
func (obj *convergerUID) InvalidateID() {
	obj.id = 0 // an id of 0 is invalid
}

// IsConverged is a helper function to the regular IsConverged method
func (obj *convergerUID) IsConverged() bool {
	return obj.converger.IsConverged(obj)
}

// SetConverged is a helper function to the regular SetConverged notification
func (obj *convergerUID) SetConverged(isConverged bool) error {
	return obj.converger.SetConverged(obj, isConverged)
}

// Unregister is a helper function to unregister myself
func (obj *convergerUID) Unregister() {
	obj.converger.Unregister(obj)
}

// ConvergedTimer is a helper around the regular ConvergedTimer method
func (obj *convergerUID) ConvergedTimer() <-chan time.Time {
	return obj.converger.ConvergedTimer(obj)
}

// StartTimer runs an invisible timer that automatically converges on timeout.
func (obj *convergerUID) StartTimer() (func() error, error) {
	obj.mutex.Lock()
	if !obj.running {
		obj.timer = make(chan struct{})
		obj.running = true
	} else {
		obj.mutex.Unlock()
		return obj.StopTimer, fmt.Errorf("Timer already started!")
	}
	obj.mutex.Unlock()
	go func() {
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
func (obj *convergerUID) ResetTimer() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if obj.running {
		obj.timer <- struct{}{} // send the reset message
		return nil
	}
	return fmt.Errorf("Timer hasn't been started!")
}

// StopTimer stops the running timer permanently until a StartTimer is run.
func (obj *convergerUID) StopTimer() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if !obj.running {
		return fmt.Errorf("Timer isn't running!")
	}
	close(obj.timer)
	obj.running = false
	return nil
}
