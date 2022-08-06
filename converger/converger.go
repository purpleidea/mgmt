// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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
	"github.com/purpleidea/mgmt/util/errwrap"
)

// New builds a new converger coordinator.
func New(timeout int64) *Coordinator {
	return &Coordinator{
		timeout: timeout,

		mutex: &sync.RWMutex{},

		//lastid: 0,
		status: make(map[*UID]struct{}),

		//converged: false, // initial state

		pokeChan: make(chan struct{}, 1), // must be buffered

		readyChan: make(chan struct{}), // ready signal

		//paused: false, // starts off as started
		pauseSignal: make(chan struct{}),
		//resumeSignal: make(chan struct{}), // happens on pause
		//pausedAck: util.NewEasyAck(), // happens on pause

		stateFns: make(map[string]func(bool) error),
		smutex:   &sync.RWMutex{},

		closeChan: make(chan struct{}),
		wg:        &sync.WaitGroup{},
	}
}

// Coordinator is the central converger engine.
type Coordinator struct {
	// timeout must be zero (instant) or greater seconds to run. If it's -1
	// then this is disabled, and we never run stateFns.
	timeout int64

	// mutex is used for controlling access to status and lastid.
	mutex *sync.RWMutex

	// lastid contains the last uid we used for registration.
	//lastid    uint64
	// status contains a reference to each active UID.
	status map[*UID]struct{}

	// converged stores the last convergence state. When this changes, we
	// run the stateFns.
	converged bool

	// pokeChan receives a message every time we might need to re-calculate.
	pokeChan chan struct{}

	// readyChan closes to notify any interested parties that the main loop
	// is running.
	readyChan chan struct{}

	// paused represents if this coordinator is paused or not.
	paused bool
	// pauseSignal closes to request a pause of this coordinator.
	pauseSignal chan struct{}
	// resumeSignal closes to request a resume of this coordinator.
	resumeSignal chan struct{}
	// pausedAck is used to send an ack message saying that we've paused.
	pausedAck *util.EasyAck

	// stateFns run on converged state changes.
	stateFns map[string]func(bool) error
	// smutex is used for controlling access to the stateFns map.
	smutex *sync.RWMutex

	// closeChan closes when we've been requested to shutdown.
	closeChan chan struct{}
	// wg waits for everything to finish.
	wg *sync.WaitGroup
}

// Register creates a new UID which can be used to report converged state. You
// must Unregister each UID before Shutdown will be able to finish running.
func (obj *Coordinator) Register() *UID {
	obj.wg.Add(1) // additional tracking for each UID
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	//obj.lastid++
	uid := &UID{
		timeout: obj.timeout, // copy the timeout here
		//id: obj.lastid,
		//name: fmt.Sprintf("%d", obj.lastid), // some default

		poke: obj.poke,

		// timer
		mutex:   &sync.Mutex{},
		timer:   nil,
		running: false,
		wg:      &sync.WaitGroup{},
	}
	uid.unregister = func() { obj.Unregister(uid) } // add unregister func
	obj.status[uid] = struct{}{}                    // TODO: add converged state here?
	return uid
}

// Unregister removes the UID from the converger coordinator. If you supply an
// invalid or unregistered uid to this function, it will panic. An unregistered
// UID is no longer part of the convergence checking.
func (obj *Coordinator) Unregister(uid *UID) {
	defer obj.wg.Done() // additional tracking for each UID
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if _, exists := obj.status[uid]; !exists {
		panic("uid is not registered")
	}
	uid.StopTimer() // ignore any errors
	delete(obj.status, uid)
}

// Run starts the main loop for the converger coordinator. It is commonly run
// from a go routine. It blocks until the Shutdown method is run to close it.
// NOTE: when we have very short timeouts, if we start before all the resources
// have joined the map, then it might appear as if we converged before we did!
func (obj *Coordinator) Run(startPaused bool) {
	obj.wg.Add(1)
	wg := &sync.WaitGroup{} // needed for the startPaused
	defer wg.Wait()         // don't leave any leftover go routines running
	if startPaused {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj.Pause() // ignore any errors
			close(obj.readyChan)
		}()
	} else {
		close(obj.readyChan) // we must wait till the wg.Add(1) has happened...
	}
	defer obj.wg.Done()
	for {
		// pause if one was requested...
		select {
		case <-obj.pauseSignal: // channel closes
			obj.pausedAck.Ack() // send ack
			// we are paused now, and waiting for resume or exit...
			select {
			case <-obj.resumeSignal: // channel closes
				// resumed!

			case <-obj.closeChan: // we can always escape
				return
			}

		case _, ok := <-obj.pokeChan: // we got an event (re-calculate)
			if !ok {
				return
			}

			if err := obj.test(); err != nil {
				// FIXME: what to do on error ?
			}

		case <-obj.closeChan: // we can always escape
			return
		}
	}
}

// Ready blocks until the Run loop has started up. This is useful so that we
// don't run Shutdown before we've even started up properly.
func (obj *Coordinator) Ready() {
	select {
	case <-obj.readyChan:
	}
}

// Shutdown sends a signal to the Run loop that it should exit. This blocks
// until it does.
func (obj *Coordinator) Shutdown() {
	close(obj.closeChan)
	obj.wg.Wait()
	close(obj.pokeChan) // free memory?
}

// Pause pauses the coordinator. It should not be called on an already paused
// coordinator. It will block until the coordinator pauses with an
// acknowledgment, or until an exit is requested. If the latter happens it will
// error. It is NOT thread-safe with the Resume() method so only call either one
// at a time.
func (obj *Coordinator) Pause() error {
	if obj.paused {
		return fmt.Errorf("already paused")
	}

	obj.pausedAck = util.NewEasyAck()
	obj.resumeSignal = make(chan struct{}) // build the resume signal
	close(obj.pauseSignal)

	// wait for ack (or exit signal)
	select {
	case <-obj.pausedAck.Wait(): // we got it!
		// we're paused
	case <-obj.closeChan:
		return fmt.Errorf("closing")
	}
	obj.paused = true

	return nil
}

// Resume unpauses the coordinator. It can be safely called on a brand-new
// coordinator that has just started running without incident. It is NOT
// thread-safe with the Pause() method, so only call either one at a time.
func (obj *Coordinator) Resume() {
	// TODO: do we need a mutex around Resume?
	if !obj.paused { // no need to unpause brand-new resources
		return
	}

	obj.pauseSignal = make(chan struct{}) // rebuild for next pause
	close(obj.resumeSignal)
	obj.poke() // unblock and notice the resume if necessary

	obj.paused = false

	// no need to wait for it to resume
	//return // implied
}

// poke sends a message to the coordinator telling it that it should re-evaluate
// whether we're converged or not. This does not block. Do not run this in a
// goroutine. It must not be called after Shutdown has been called.
func (obj *Coordinator) poke() {
	// redundant
	//if len(obj.pokeChan) > 0 {
	//	return
	//}

	select {
	case obj.pokeChan <- struct{}{}:
	default: // if chan is now full because more than one poke happened...
	}
}

// IsConverged returns true if *every* registered uid has converged. If there
// are no registered UID's, then this will return true.
func (obj *Coordinator) IsConverged() bool {
	for _, v := range obj.Status() {
		if !v { // everyone must be converged for this to be true
			return false
		}
	}
	return true
}

// test evaluates whether we're converged or not and runs the state change. It
// is NOT thread-safe.
func (obj *Coordinator) test() error {
	// TODO: add these checks elsewhere to prevent anything from running?
	if obj.timeout < 0 {
		return nil // nothing to do (only run if timeout is valid)
	}

	converged := obj.IsConverged()
	defer func() {
		obj.converged = converged // set this only at the end...
	}()

	if !converged {
		if !obj.converged { // were we previously also not converged?
			return nil // nothing to do
		}

		// we're doing a state change
		// call the arbitrary functions (takes a read lock!)
		return obj.runStateFns(false)
	}

	// we have converged!
	if obj.converged { // were we previously also converged?
		return nil // nothing to do
	}

	// call the arbitrary functions (takes a read lock!)
	return obj.runStateFns(true)
}

// runStateFns runs the list of stored state functions.
func (obj *Coordinator) runStateFns(converged bool) error {
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
		e := fn(converged)
		err = errwrap.Append(err, e) // list of errors
	}
	return err
}

// AddStateFn adds a state function to be run on change of converged state.
func (obj *Coordinator) AddStateFn(name string, stateFn func(bool) error) error {
	obj.smutex.Lock()
	defer obj.smutex.Unlock()
	if _, exists := obj.stateFns[name]; exists {
		return fmt.Errorf("a stateFn with that name already exists")
	}
	obj.stateFns[name] = stateFn
	return nil
}

// RemoveStateFn removes a state function from running on change of converged
// state.
func (obj *Coordinator) RemoveStateFn(name string) error {
	obj.smutex.Lock()
	defer obj.smutex.Unlock()
	if _, exists := obj.stateFns[name]; !exists {
		return fmt.Errorf("a stateFn with that name doesn't exist")
	}
	delete(obj.stateFns, name)
	return nil
}

// Status returns a map of the converged status of each UID.
func (obj *Coordinator) Status() map[*UID]bool {
	status := make(map[*UID]bool)
	obj.mutex.RLock() // take a read lock
	defer obj.mutex.RUnlock()
	for k := range obj.status {
		status[k] = k.IsConverged()
	}
	return status
}

// Timeout returns the timeout in seconds that converger was created with. This
// is useful to avoid passing in the timeout value separately when you're
// already passing in the Coordinator struct.
func (obj *Coordinator) Timeout() int64 {
	return obj.timeout
}

// UID represents one of the probes for the converger coordinator. It is created
// by calling the Register method of the Coordinator struct. It should be freed
// after use with Unregister.
type UID struct {
	// timeout is a copy of the main timeout. It could eventually be used
	// for per-UID timeouts too.
	timeout int64
	// isConverged stores the convergence state of this particular UID.
	isConverged bool

	// poke stores a reference to the main poke function.
	poke func()
	// unregister stores a reference to the unregister function.
	unregister func()

	// timer
	mutex   *sync.Mutex
	timer   chan struct{}
	running bool // is the timer running?
	wg      *sync.WaitGroup
}

// Unregister removes this UID from the converger coordinator. An unregistered
// UID is no longer part of the convergence checking.
func (obj *UID) Unregister() {
	obj.unregister()
}

// IsConverged reports whether this UID is converged or not.
func (obj *UID) IsConverged() bool {
	return obj.isConverged
}

// SetConverged sets the convergence state of this UID. This is used by the
// running timer if one is started. The timer will overwrite any value set by
// this method.
func (obj *UID) SetConverged(isConverged bool) {
	obj.isConverged = isConverged
	obj.poke() // notify of change
}

// ConvergedTimer adds a timeout to a select call and blocks until then.
// TODO: this means we could eventually have per resource converged timeouts
func (obj *UID) ConvergedTimer() <-chan time.Time {
	// be clever: if i'm already converged, this timeout should block which
	// avoids unnecessary new signals being sent! this avoids fast loops if
	// we have a low timeout, or in particular a timeout == 0
	if obj.IsConverged() {
		// blocks the case statement in select forever!
		return util.TimeAfterOrBlock(-1)
	}
	return util.TimeAfterOrBlock(int(obj.timeout))
}

// StartTimer runs a timer that sets us as converged on timeout. It also returns
// a handle to the StopTimer function which should be run before exit.
func (obj *UID) StartTimer() (func() error, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if obj.running {
		return obj.StopTimer, fmt.Errorf("timer already started")
	}
	obj.timer = make(chan struct{})
	obj.running = true
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		for {
			select {
			case _, ok := <-obj.timer: // reset signal channel
				if !ok {
					return
				}
				obj.SetConverged(false)

			case <-obj.ConvergedTimer():
				obj.SetConverged(true) // converged!
				select {
				case _, ok := <-obj.timer: // reset signal channel
					if !ok {
						return
					}
				}
			}
		}
	}()
	return obj.StopTimer, nil
}

// ResetTimer resets the timer to zero.
func (obj *UID) ResetTimer() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if obj.running {
		obj.timer <- struct{}{} // send the reset message
		return nil
	}
	return fmt.Errorf("timer hasn't been started")
}

// StopTimer stops the running timer.
func (obj *UID) StopTimer() error {
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
