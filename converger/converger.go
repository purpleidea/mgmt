// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

// Package converger is a facility for reporting the converged state.
package converger

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// StateFns are a map of state functions which are run with the converged state
// whenever it changes.
type StateFns = map[string]func(context.Context, bool) error

// Coordinator is the central converger engine.
type Coordinator struct {
	// Timeout must be zero (instant) or greater seconds to run. If it's -1
	// then this is disabled, and we never run stateFns.
	Timeout int

	// StateFns are functions that run on converged state changes. This can
	// only be edited before the coordinator starts or when it is paused.
	StateFns StateFns

	Debug bool
	Logf  func(format string, v ...interface{})

	// duration is the cached time.Duration form of Timeout. It's negative
	// when the coordinator is disabled.
	duration time.Duration

	// pokeChan receives a message every time we might need to recalculate.
	pokeChan chan struct{}

	// controlChan serializes pause and resume requests through the main
	// loop.
	controlChan chan *controlMsg

	// status contains a reference to each active UID.
	status map[*UID]struct{}

	// paused stores whether we're running or not.
	paused bool

	// converged stores the last externally observed convergence state.
	converged bool

	// holdUntil delays convergence after start, resume, or when we move to
	// a zero worker state. We periodically update this deadline as needed.
	holdUntil time.Time

	// lastStatusCount lets us detect transitions into the zero worker
	// state, since we handle the zero case specially.
	lastStatusCount int

	// ready is closed as soon as it's safe to start running pause/resume.
	ready chan struct{}

	// mutex is used for controlling access to status.
	mutex *sync.RWMutex

	// ctx is canceled when we've been requested to shutdown.
	ctx context.Context

	// wg waits for everything to finish.
	wg *sync.WaitGroup
}

// Init initializes a new converger coordinator before first use.
func (obj *Coordinator) Init() error { // TODO: do we need an error?

	obj.duration = time.Duration(obj.Timeout) * time.Second

	obj.pokeChan = make(chan struct{}, 1) // must be buffered to not block
	obj.controlChan = make(chan *controlMsg)

	obj.status = make(map[*UID]struct{})

	obj.ready = make(chan struct{})

	obj.mutex = &sync.RWMutex{}

	obj.wg = &sync.WaitGroup{}

	return nil
}

// Register creates a new UID which can be used to report converged state. You
// must Unregister each UID before the converger can unblock and exit cleanly.
func (obj *Coordinator) Register() *UID {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	obj.wg.Add(1) // additional tracking for each UID
	uid := &UID{
		duration:  obj.duration,
		converged: &atomic.Bool{},
		poke:      obj.poke,
		mutex:     &sync.Mutex{},
	}

	uid.unregister = func() { obj.Unregister(uid) } // reference to self!
	obj.status[uid] = struct{}{}
	obj.poke()

	return uid
}

// Unregister removes the UID from the converger coordinator. If you supply an
// invalid or unregistered uid to this function, it will panic. An unregistered
// UID is no longer part of the convergence checking.
func (obj *Coordinator) Unregister(uid *UID) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	defer obj.wg.Done() // additional tracking for each UID
	if _, exists := obj.status[uid]; !exists {
		panic("uid is not registered")
	}

	uid.StopTimer() // cleanup
	delete(obj.status, uid)
	obj.poke()
}

// Run starts the main loop for the converger coordinator. It is commonly run
// from a go routine. It blocks until shutdown is requested by the supplied
// context, or until the optional Shutdown helper is run.
func (obj *Coordinator) Run(ctx context.Context, startPaused bool) error {
	defer obj.wg.Wait() // make sure everyone unregistered before we exit
	// If we wanted a public Wait method we could add an additional +1 here.
	//obj.wg.Add(1)
	//defer obj.wg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	obj.ctx = ctx

	obj.paused = startPaused
	obj.lastStatusCount = obj.statusCount()
	if !startPaused && obj.duration >= 0 {
		obj.holdUntil = time.Now().Add(obj.duration)
	}

	var holdTimer *time.Timer
	stopHold := func() {
		if holdTimer == nil {
			return
		}
		if !holdTimer.Stop() { // drain
			select {
			case <-holdTimer.C:
			default:
			}
		}
		holdTimer = nil
	}
	defer stopHold()

	close(obj.ready) // don't pause/resume until we're done writing above
	for {
		// This looks through the world and runs the stateFns if they
		// changed. This updates our core variables and is not pure!
		if err := obj.recalculate(ctx, time.Now()); err != nil {
			return err
		}

		// calculate how long we should wait until the next wake up...
		stopHold()
		var holdTimerC <-chan time.Time
		if !obj.paused && obj.duration >= 0 && !obj.holdUntil.IsZero() {
			if remaining := time.Until(obj.holdUntil); remaining > 0 {
				holdTimer = time.NewTimer(remaining)
				holdTimerC = holdTimer.C
			}
		}

		select {
		case <-obj.pokeChan:

		case <-holdTimerC:

		case msg := <-obj.controlChan:
			if msg.pause == obj.paused {
				if obj.paused {
					panic("already paused")
				} else {
					panic("already resumed")
				}
			}

			obj.paused = msg.pause // set the state!

			if msg.pause || obj.converged || obj.duration < 0 {
				obj.holdUntil = time.Time{} // forever...
			} else {
				obj.holdUntil = time.Now().Add(obj.duration)
			}

			close(msg.reply) // send non-blocking, fast message!

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Pause pauses the coordinator. It should not be called on an already paused
// coordinator. It will block until the coordinator pauses, or until shutdown.
// While we are paused, no "converger" action will occur.
func (obj *Coordinator) Pause() {
	obj.helperPauseResume(true)
}

// Resume unpauses the coordinator. It can be safely called on a brand-new
// coordinator that has just started running without incident. The "converger"
// actions will occur while we are resumed and running.
func (obj *Coordinator) Resume() {
	obj.helperPauseResume(false)
}

// helperPauseResume is the actual pause/resume mechanism.
func (obj *Coordinator) helperPauseResume(pause bool) {
	select {
	case <-obj.ready: // don't race with write on obj.ctx above and read here
	}
	reply := make(chan struct{})

	select {
	case obj.controlChan <- &controlMsg{pause: pause, reply: reply}:
	case <-obj.ctx.Done():
		return
	}

	// TODO: do we really need a reply with the synchronous control message?
	select {
	case <-reply:
	case <-obj.ctx.Done():
	}
}

// poke sends a message to the coordinator telling it that it should re-evaluate
// whether we're converged or not. This does not block.
func (obj *Coordinator) poke() {
	select {
	case obj.pokeChan <- struct{}{}:
	default:
	}
}

// recalculate is where the magic happens. We take a look at the universe, and
// decide if we should run or notify about a state transition. This only errors
// if one of the stateFns errors.
func (obj *Coordinator) recalculate(ctx context.Context, now time.Time) error {
	if obj.duration < 0 || obj.paused {
		return nil
	}

	count, isConverged := obj.convergedSnapshot() // what's our state now?
	if count == 0 && obj.lastStatusCount != 0 {
		// If the last UID unregisters, we'd suddenly move to a zero
		// worker state and historically we'd immediate be converged,
		// but we don't want that instant converge, we want the timer to
		// begin counting from that point, so wait $duration more time.
		obj.holdUntil = now.Add(obj.duration)
	}
	obj.lastStatusCount = count // store new count

	nextState := isConverged
	if !obj.holdUntil.IsZero() && now.Before(obj.holdUntil) {
		nextState = false // state is now not converged!
	}

	if nextState == obj.converged { // state didn't change
		return nil
	}
	obj.converged = nextState // update it for next time

	// Down here we do the actual work since the state just changed!
	obj.logTransition(obj.converged)
	return obj.runStateFns(ctx, obj.converged)
}

// convergedSnapshot looks at the converged status of each registered entry. It
// returns whether the whole is converged or not (one non-converged means we are
// not converged) and how many total we are.
func (obj *Coordinator) convergedSnapshot() (int, bool) {
	obj.mutex.RLock()
	defer obj.mutex.RUnlock()

	count := len(obj.status)
	for uid := range obj.status {
		if !uid.isConverged() {
			return count, false
		}
	}

	return count, true // converged!
}

// statusCount returns the number of registered entries we have.
func (obj *Coordinator) statusCount() int {
	obj.mutex.RLock()
	defer obj.mutex.RUnlock()
	return len(obj.status)
}

// logTransition spits out the expected log message depending on our state.
func (obj *Coordinator) logTransition(converged bool) {
	if obj.Logf == nil {
		return
	}
	if converged {
		obj.Logf("converged for %d seconds", obj.Timeout)
		return
	}
	obj.Logf("unconverged...")
}

// runStateFns runs the list of stored state functions.
func (obj *Coordinator) runStateFns(ctx context.Context, converged bool) error {

	keys := []string{}
	for k := range obj.StateFns {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var err error
	for _, name := range keys { // run in deterministic order
		fn := obj.StateFns[name]
		e := fn(ctx, converged)
		// TODO: if ctx exits, should we keep running the loop?
		err = errwrap.Append(err, e)
	}

	return err
}

// UID represents one of the probes for the converger coordinator. It is created
// by calling the Register method of the Coordinator struct. It should be freed
// after use with Unregister. It is public only so that it's easier to use by
// callers since this type is part of the public API that coordinator exposes.
type UID struct {
	// duration is a copy of the coordinator duration. It's negative when
	// the coordinator is disabled.
	duration time.Duration

	// converged stores the convergence state of this particular UID.
	converged *atomic.Bool

	// unregister stores a reference to the unregister function.
	unregister func()

	// poke stores a reference to the main poke function.
	poke func()

	mutex   *sync.Mutex
	timer   *time.Timer
	running bool
}

// Unregister removes this UID from the converger coordinator. An unregistered
// UID is no longer part of the convergence checking.
func (obj *UID) Unregister() {
	obj.unregister() // call this stored fn
}

// WithTimer starts the timer and returns a stop function which can be used in
// defer, giving you an ergonomic way of timing with a function scoped region.
//
// example:
//
//	func SomeWorker() {
//		defer uid.WithTimer()() // start timer and stops it on defer!
//
//		fmt.Printf("doing some work...")
//		time.Sleep(5 * time.Second) # some work to delay the converger!
//		fmt.Printf("done working!")
//	}
func (obj *UID) WithTimer() func() {
	obj.StartTimer()
	return obj.StopTimer
}

// StartTimer runs a timer that marks this UID as converged on timeout. Calling
// StartTimer on an already-running timer is equivalent to ResetTimer.
func (obj *UID) StartTimer() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	obj.running = true

	changed := obj.setConvergedChanged(false)
	obj.resetTimer()

	if changed {
		obj.poke()
	}
}

// StopTimer stops the running timer. If the timer isn't running this is a
// no-op.
func (obj *UID) StopTimer() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if !obj.running {
		return
	}
	obj.running = false

	changed := obj.setConvergedChanged(false)
	obj.stopTimer()

	if changed {
		obj.poke()
	}
}

// ResetTimer resets the timer to zero. If the timer isn't running this is a
// no-op.
func (obj *UID) ResetTimer() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if !obj.running {
		return
	}

	changed := obj.setConvergedChanged(false)
	obj.resetTimer()

	if changed {
		obj.poke()
	}
}

// stopTimer stops and clears the timer. You must take the mutex before running
// this function.
func (obj *UID) stopTimer() {
	if obj.timer == nil {
		return
	}
	obj.timer.Stop()
	obj.timer = nil
}

// resetTimer arms a new timer. When it fires, the callback checks the timer
// identity to discard itself if a concurrent stop/reschedule already replaced
// it. You must take the mutex before running this function.
func (obj *UID) resetTimer() {
	obj.stopTimer() // always stop anything first

	if obj.duration < 0 {
		return
	}

	var t *time.Timer
	t = time.AfterFunc(obj.duration, func() {
		obj.mutex.Lock()
		defer obj.mutex.Unlock()

		if obj.timer != t { // superseded
			return
		}
		obj.timer = nil
		changed := obj.setConvergedChanged(true) // we are converged!

		if changed {
			obj.poke()
		}
	})
	obj.timer = t
}

// isConverged reports whether this UID is converged or not.
func (obj *UID) isConverged() bool {
	return obj.converged.Load()
}

// setConvergedChanged sets the converged state and returns whether it changed
// from the previous state. This is needed so we know whether to poke or not. We
// don't need to take the mutex before running this function (although it's fine
// if it's held at the time) because the read/write is via an atomic bool.
func (obj *UID) setConvergedChanged(isConverged bool) bool {
	previous := obj.converged.Load()
	obj.converged.Store(isConverged)
	return previous != isConverged
}

// controlMsg sends the pause/resume msg into the controlChan and receives a
// reply.
// XXX: do we need a reply now that the channel is sync?
type controlMsg struct {
	pause bool
	reply chan struct{}
}
