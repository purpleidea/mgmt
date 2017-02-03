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

package pgraph

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/resources"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
	"golang.org/x/time/rate"
)

// GetTimestamp returns the timestamp of a vertex
func (v *Vertex) GetTimestamp() int64 {
	return v.timestamp
}

// UpdateTimestamp updates the timestamp on a vertex and returns the new value
func (v *Vertex) UpdateTimestamp() int64 {
	v.timestamp = time.Now().UnixNano() // update
	return v.timestamp
}

// OKTimestamp returns true if this element can run right now?
func (g *Graph) OKTimestamp(v *Vertex) bool {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphVertices(v) {
		// if the vertex has a greater timestamp than any pre-req (n)
		// then we can't run right now...
		// if they're equal (eg: on init of 0) then we also can't run
		// b/c we should let our pre-req's go first...
		x, y := v.GetTimestamp(), n.GetTimestamp()
		if g.Flags.Debug {
			log.Printf("%s[%s]: OKTimestamp: (%v) >= %s[%s](%v): !%v", v.Kind(), v.GetName(), x, n.Kind(), n.GetName(), y, x >= y)
		}
		if x >= y {
			return false
		}
	}
	return true
}

// Poke tells nodes after me in the dependency graph that they need to refresh.
func (g *Graph) Poke(v *Vertex) error {
	var wg sync.WaitGroup
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphVertices(v) {
		// we can skip this poke if resource hasn't done work yet... it
		// needs to be poked if already running, or not running though!
		// TODO: does this need an || activity flag?
		if n.Res.GetState() != resources.ResStateProcess {
			if g.Flags.Debug {
				log.Printf("%s[%s]: Poke: %s[%s]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			wg.Add(1)
			go func(nn *Vertex) error {
				defer wg.Done()
				//edge := g.Adjacency[v][nn] // lookup
				//notify := edge.Notify && edge.Refresh()
				return nn.SendEvent(event.EventPoke, nil)
			}(n)

		} else {
			if g.Flags.Debug {
				log.Printf("%s[%s]: Poke: %s[%s]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
	// TODO: do something with return values?
	wg.Wait() // wait for all the pokes to complete
	return nil
}

// BackPoke pokes the pre-requisites that are stale and need to run before I can run.
func (g *Graph) BackPoke(v *Vertex) {
	var wg sync.WaitGroup
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphVertices(v) {
		x, y, s := v.GetTimestamp(), n.GetTimestamp(), n.Res.GetState()
		// If the parent timestamp needs poking AND it's not running
		// Process, then poke it. If the parent is in ResStateProcess it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != resources.ResStateProcess && s != resources.ResStateCheckApply) {
			if g.Flags.Debug {
				log.Printf("%s[%s]: BackPoke: %s[%s]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			wg.Add(1)
			go func(nn *Vertex) error {
				defer wg.Done()
				return nn.SendEvent(event.EventBackPoke, nil)
			}(n)

		} else {
			if g.Flags.Debug {
				log.Printf("%s[%s]: BackPoke: %s[%s]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
	// TODO: do something with return values?
	wg.Wait() // wait for all the pokes to complete
}

// RefreshPending determines if any previous nodes have a refresh pending here.
// If this is true, it means I am expected to apply a refresh when I next run.
func (g *Graph) RefreshPending(v *Vertex) bool {
	var refresh bool
	for _, edge := range g.IncomingGraphEdges(v) {
		// if we asked for a notify *and* if one is pending!
		if edge.Notify && edge.Refresh() {
			refresh = true
			break
		}
	}
	return refresh
}

// SetUpstreamRefresh sets the refresh value to any upstream vertices.
func (g *Graph) SetUpstreamRefresh(v *Vertex, b bool) {
	for _, edge := range g.IncomingGraphEdges(v) {
		if edge.Notify {
			edge.SetRefresh(b)
		}
	}
}

// SetDownstreamRefresh sets the refresh value to any downstream vertices.
func (g *Graph) SetDownstreamRefresh(v *Vertex, b bool) {
	for _, edge := range g.OutgoingGraphEdges(v) {
		// if we asked for a notify *and* if one is pending!
		if edge.Notify {
			edge.SetRefresh(b)
		}
	}
}

// Process is the primary function to execute for a particular vertex in the graph.
func (g *Graph) Process(v *Vertex) error {
	obj := v.Res
	if g.Flags.Debug {
		log.Printf("%s[%s]: Process()", obj.Kind(), obj.GetName())
	}
	defer obj.SetState(resources.ResStateNil) // reset state when finished
	obj.SetState(resources.ResStateProcess)
	var ok = true
	var applied = false // did we run an apply?
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if !g.OKTimestamp(v) {
		go g.BackPoke(v)
		return nil
	}
	// timestamp must be okay...

	if g.Flags.Debug {
		log.Printf("%s[%s]: OKTimestamp(%v)", obj.Kind(), obj.GetName(), v.GetTimestamp())
	}

	// connect any senders to receivers and detect if values changed
	if updated, err := obj.SendRecv(obj); err != nil {
		return errwrap.Wrapf(err, "could not SendRecv in Process")
	} else if len(updated) > 0 {
		for _, changed := range updated {
			if changed { // at least one was updated
				obj.StateOK(false) // invalidate cache, mark as dirty
				break
			}
		}
	}

	var noop = obj.Meta().Noop // lookup the noop value
	var refresh bool
	var checkOK bool
	var err error

	if g.Flags.Debug {
		log.Printf("%s[%s]: CheckApply(%t)", obj.Kind(), obj.GetName(), !noop)
	}

	// lookup the refresh (notification) variable
	refresh = g.RefreshPending(v) // do i need to perform a refresh?
	obj.SetRefresh(refresh)       // tell the resource

	// changes can occur after this...
	obj.SetState(resources.ResStateCheckApply)

	// check cached state, to skip CheckApply; can't skip if refreshing
	if !refresh && obj.IsStateOK() {
		checkOK, err = true, nil

		// NOTE: technically this block is wrong because we don't know
		// if the resource implements refresh! If it doesn't, we could
		// skip this, but it doesn't make a big difference under noop!
	} else if noop && refresh { // had a refresh to do w/ noop!
		checkOK, err = false, nil // therefore the state is wrong

		// run the CheckApply!
	} else {
		// if this fails, don't UpdateTimestamp()
		checkOK, err = obj.CheckApply(!noop)

		// TODO: Can the `Poll` converged timeout tracking be a
		// more general method for all converged timeouts? this
		// would simplify the resources by removing boilerplate
		if v.Meta().Poll > 0 {
			if !checkOK { // something changed, restart timer
				cuid := v.Res.ConvergerUID() // get the converger uid used to report status
				cuid.ResetTimer()            // activity!
				if g.Flags.Debug {
					log.Printf("%s[%s]: Converger: ResetTimer", obj.Kind(), obj.GetName())
				}
			}
		}
	}

	if checkOK && err != nil { // should never return this way
		log.Fatalf("%s[%s]: CheckApply(): %t, %+v", obj.Kind(), obj.GetName(), checkOK, err)
	}
	if g.Flags.Debug {
		log.Printf("%s[%s]: CheckApply(): %t, %v", obj.Kind(), obj.GetName(), checkOK, err)
	}

	// if CheckApply ran without noop and without error, state should be good
	if !noop && err == nil { // aka !noop || checkOK
		obj.StateOK(true) // reset
		if refresh {
			g.SetUpstreamRefresh(v, false) // refresh happened, clear the request
			obj.SetRefresh(false)
		}
	}

	if !checkOK { // if state *was* not ok, we had to have apply'ed
		if err != nil { // error during check or apply
			ok = false
		} else {
			applied = true
		}
	}

	// when noop is true we always want to update timestamp
	if noop && err == nil {
		ok = true
	}

	if ok {
		// did we actually do work?
		activity := applied
		if noop {
			activity = false // no we didn't do work...
		}

		if activity { // add refresh flag to downstream edges...
			g.SetDownstreamRefresh(v, true)
		}

		// update this timestamp *before* we poke or the poked
		// nodes might fail due to having a too old timestamp!
		v.UpdateTimestamp()                    // this was touched...
		obj.SetState(resources.ResStatePoking) // can't cancel parent poke
		if err := g.Poke(v); err != nil {
			return errwrap.Wrapf(err, "the Poke() failed")
		}
	}
	// poke at our pre-req's instead since they need to refresh/run...
	return errwrap.Wrapf(err, "could not Process() successfully")
}

// SentinelErr is a sentinal as an error type that wraps an arbitrary error.
type SentinelErr struct {
	err error
}

// Error is the required method to fulfill the error type.
func (obj *SentinelErr) Error() string {
	return obj.err.Error()
}

// Worker is the common run frontend of the vertex. It handles all of the retry
// and retry delay common code, and ultimately returns the final status of this
// vertex execution.
func (g *Graph) Worker(v *Vertex) error {
	// listen for chan events from Watch() and run
	// the Process() function when they're received
	// this avoids us having to pass the data into
	// the Watch() function about which graph it is
	// running on, which isolates things nicely...
	obj := v.Res

	// run the init (should match 1-1 with Close function if this succeeds)
	if err := obj.Init(); err != nil {
		return errwrap.Wrapf(err, "could not Init() resource")
	}

	lock := &sync.Mutex{} // lock around processChan closing and sending
	finished := false     // did we close processChan ?
	processChan := make(chan *event.Event)

	// if the CheckApply run takes longer than the converged
	// timeout, we could inappropriately converge mid-apply!
	// avoid this by blocking convergence with a fake report
	// we also add a similar blocker around the worker loop!
	wcuid := obj.Converger().Register() // get an extra cuid for the worker!
	defer wcuid.Unregister()
	wcuid.SetConverged(true)            // starts off false, and waits for loop timeout
	pcuid := obj.Converger().Register() // get an extra cuid for the process
	defer pcuid.Unregister()
	pcuid.SetConverged(true) // starts off true, because it's not running...

	go func() {
		running := false
		done := make(chan struct{})
		playback := false // do we need to run another one?

		waiting := false
		var timer = time.NewTimer(time.Duration(math.MaxInt64)) // longest duration
		if !timer.Stop() {
			<-timer.C // unnecessary, shouldn't happen
		}

		var delay = time.Duration(v.Meta().Delay) * time.Millisecond
		var retry = v.Meta().Retry // number of tries left, -1 for infinite
		var limiter = rate.NewLimiter(v.Meta().Limit, v.Meta().Burst)
		limited := false

	Loop:
		for {
			select {
			case ev, ok := <-processChan: // must use like this
				if !ok { // processChan closed, let's exit
					break Loop // no event, so no ack!
				}
				if v.Res.Meta().Poll == 0 { // skip for polling
					wcuid.SetConverged(false)
				}

				// if process started, but no action yet, skip!
				if v.Res.GetState() == resources.ResStateProcess {
					if g.Flags.Debug {
						log.Printf("%s[%s]: Skipped event!", v.Kind(), v.GetName())
					}
					ev.ACK() // ready for next message
					continue
				}

				// if running, we skip running a new execution!
				// if waiting, we skip running a new execution!
				if running || waiting {
					if g.Flags.Debug {
						log.Printf("%s[%s]: Playback added!", v.Kind(), v.GetName())
					}
					playback = true
					ev.ACK() // ready for next message
					continue
				}

				// catch invalid rates
				if v.Meta().Burst == 0 && !(v.Meta().Limit == rate.Inf) { // blocked
					e := fmt.Errorf("%s[%s]: Permanently limited (rate != Inf, burst: 0)", v.Kind(), v.GetName())
					v.SendEvent(event.EventExit, &SentinelErr{e})
					ev.ACK() // ready for next message
					continue
				}

				// rate limit
				// FIXME: consider skipping rate limit check if
				// the event is a poke instead of a watch event
				if !limited && !(v.Meta().Limit == rate.Inf) { // skip over the playback event...
					now := time.Now()
					r := limiter.ReserveN(now, 1) // one event
					// r.OK() seems to always be true here!
					d := r.DelayFrom(now)
					if d > 0 { // delay
						limited = true
						playback = true
						log.Printf("%s[%s]: Limited (rate: %v/sec, burst: %d, next: %v)", v.Kind(), v.GetName(), v.Meta().Limit, v.Meta().Burst, d)
						// start the timer...
						timer.Reset(d)
						waiting = true // waiting for retry timer
						ev.ACK()
						continue
					} // otherwise, we run directly!
				}
				limited = false // let one through

				running = true
				go func(ev *event.Event) {
					pcuid.SetConverged(false) // "block" Process
					if e := g.Process(v); e != nil {
						playback = true
						log.Printf("%s[%s]: CheckApply errored: %v", v.Kind(), v.GetName(), e)
						if retry == 0 {
							// wrap the error in the sentinel
							v.SendEvent(event.EventExit, &SentinelErr{e})
							return
						}
						if retry > 0 { // don't decrement the -1
							retry--
						}
						log.Printf("%s[%s]: CheckApply: Retrying after %.4f seconds (%d left)", v.Kind(), v.GetName(), delay.Seconds(), retry)
						// start the timer...
						timer.Reset(delay)
						waiting = true // waiting for retry timer
						return
					}
					retry = v.Meta().Retry // reset on success
					close(done)            // trigger
				}(ev)
				ev.ACK() // sync (now mostly useless)

			case <-timer.C:
				if v.Res.Meta().Poll == 0 { // skip for polling
					wcuid.SetConverged(false)
				}
				waiting = false
				if !timer.Stop() {
					//<-timer.C // blocks, docs are wrong!
				}
				log.Printf("%s[%s]: CheckApply delay expired!", v.Kind(), v.GetName())
				close(done)

			// a CheckApply run (with possibly retry pause) finished
			case <-done:
				if v.Res.Meta().Poll == 0 { // skip for polling
					wcuid.SetConverged(false)
				}
				if g.Flags.Debug {
					log.Printf("%s[%s]: CheckApply finished!", v.Kind(), v.GetName())
				}
				done = make(chan struct{}) // reset
				// re-send this event, to trigger a CheckApply()
				if playback {
					playback = false
					// this lock avoids us sending to
					// channel after we've closed it!
					lock.Lock()
					go func() {
						if !finished {
							// TODO: can this experience indefinite postponement ?
							// see: https://github.com/golang/go/issues/11506
							obj.Event(processChan) // replay a new event
						}
						lock.Unlock()
					}()
				}
				running = false
				pcuid.SetConverged(true) // "unblock" Process

			case <-wcuid.ConvergedTimer():
				wcuid.SetConverged(true) // converged!
				continue
			}
		}
	}()
	var err error // propagate the error up (this is a permanent BAD error!)
	// the watch delay runs inside of the Watch resource loop, so that it
	// can still process signals and exit if needed. It shouldn't run any
	// resource specific code since this is supposed to be a retry delay.
	// NOTE: we're using the same retry and delay metaparams that CheckApply
	// uses. This is for practicality. We can separate them later if needed!
	var watchDelay time.Duration
	var watchRetry = v.Meta().Retry // number of tries left, -1 for infinite
	// watch blocks until it ends, & errors to retry
	for {
		// TODO: do we have to stop the converged-timeout when in this block (perhaps we're in the delay block!)
		// TODO: should we setup/manage some of the converged timeout stuff in here anyways?

		// if a retry-delay was requested, wait, but don't block our events!
		if watchDelay > 0 {
			//var pendingSendEvent bool
			timer := time.NewTimer(watchDelay)
		Loop:
			for {
				select {
				case <-timer.C: // the wait is over
					break Loop // critical

				// TODO: resources could have a separate exit channel to avoid this complexity!?
				case event := <-obj.Events():
					// NOTE: this code should match the similar Res code!
					//cuid.SetConverged(false) // TODO: ?
					if exit, send := obj.ReadEvent(event); exit != nil {
						err := *exit // exit err
						if e := obj.Close(); err == nil {
							err = e
						} else if e != nil {
							err = multierr.Append(err, e) // list of errors
						}
						return err // exit
					} else if send {
						// if we dive down this rabbit hole, our
						// timer.C won't get seen until we get out!
						// in this situation, the Watch() is blocked
						// from performing until CheckApply returns
						// successfully, or errors out. This isn't
						// so bad, but we should document it. Is it
						// possible that some resource *needs* Watch
						// to run to be able to execute a CheckApply?
						// That situation shouldn't be common, and
						// should probably not be allowed. Can we
						// avoid it though?
						//if exit, err := doSend(); exit || err != nil {
						//	return err // we exit or bubble up a NACK...
						//}
						// Instead of doing the above, we can
						// add events to a pending list, and
						// when we finish the delay, we can run
						// them.
						//pendingSendEvent = true // all events are identical for now...
					}
				}
			}
			timer.Stop() // it's nice to cleanup
			log.Printf("%s[%s]: Watch delay expired!", v.Kind(), v.GetName())
			// NOTE: we can avoid the send if running Watch guarantees
			// one CheckApply event on startup!
			//if pendingSendEvent { // TODO: should this become a list in the future?
			//	if exit, err := obj.DoSend(processChan, ""); exit || err != nil {
			//		return err // we exit or bubble up a NACK...
			//	}
			//}
		}

		// TODO: reset the watch retry count after some amount of success
		v.Res.RegisterConverger()
		var e error
		if v.Res.Meta().Poll > 0 { // poll instead of watching :(
			cuid := v.Res.ConvergerUID() // get the converger uid used to report status
			cuid.StartTimer()
			e = v.Res.Poll(processChan)
			cuid.StopTimer() // clean up nicely
		} else {
			e = v.Res.Watch(processChan) // run the watch normally
		}
		v.Res.UnregisterConverger()
		if e == nil { // exit signal
			err = nil // clean exit
			break
		}
		if sentinelErr, ok := e.(*SentinelErr); ok { // unwrap the sentinel
			err = sentinelErr.err
			break // sentinel means, perma-exit
		}
		log.Printf("%s[%s]: Watch errored: %v", v.Kind(), v.GetName(), e)
		if watchRetry == 0 {
			err = fmt.Errorf("Permanent watch error: %v", e)
			break
		}
		if watchRetry > 0 { // don't decrement the -1
			watchRetry--
		}
		watchDelay = time.Duration(v.Meta().Delay) * time.Millisecond
		log.Printf("%s[%s]: Watch: Retrying after %.4f seconds (%d left)", v.Kind(), v.GetName(), watchDelay.Seconds(), watchRetry)
		// We need to trigger a CheckApply after Watch restarts, so that
		// we catch any lost events that happened while down. We do this
		// by getting the Watch resource to send one event once it's up!
		//v.SendEvent(eventPoke, false, false)
	}
	lock.Lock() // lock to avoid a send when closed!
	finished = true
	close(processChan)
	lock.Unlock()

	// close resource and return possible errors if any
	if e := obj.Close(); err == nil {
		err = e
	} else if e != nil {
		err = multierr.Append(err, e) // list of errors
	}
	return err
}

// Start is a main kick to start the graph. It goes through in reverse topological
// sort order so that events can't hit un-started vertices.
func (g *Graph) Start(first bool) { // start or continue
	log.Printf("State: %v -> %v", g.setState(graphStateStarting), g.getState())
	defer log.Printf("State: %v -> %v", g.setState(graphStateStarted), g.getState())
	var wg sync.WaitGroup
	t, _ := g.TopologicalSort()
	// TODO: only calculate indegree if `first` is true to save resources
	indegree := g.InDegree() // compute all of the indegree's
	for _, v := range Reverse(t) {
		// selective poke: here we reduce the number of initial pokes
		// to the minimum required to activate every vertex in the
		// graph, either by direct action, or by getting poked by a
		// vertex that was previously activated. if we poke each vertex
		// that has no incoming edges, then we can be sure to reach the
		// whole graph. Please note: this may mask certain optimization
		// failures, such as any poke limiting code in Poke() or
		// BackPoke(). You might want to disable this selective start
		// when experimenting with and testing those elements.
		// if we are unpausing (since it's not the first run of this
		// function) we need to poke to *unpause* every graph vertex,
		// and not just selectively the subset with no indegree.

		// let the startup code know to poke or not
		v.Res.Starter((!first) || indegree[v] == 0)

		if !v.Res.IsWorking() { // if Worker() is not running...
			g.wg.Add(1)
			// must pass in value to avoid races...
			// see: https://ttboj.wordpress.com/2015/07/27/golang-parallelism-issues-causing-too-many-open-files-error/
			go func(vv *Vertex) {
				defer g.wg.Done()
				// TODO: if a sufficient number of workers error,
				// should something be done? Should these restart
				// after perma-failure if we have a graph change?
				if err := g.Worker(vv); err != nil { // contains the Watch and CheckApply loops
					log.Printf("%s[%s]: Exited with failure: %v", vv.Kind(), vv.GetName(), err)
					return
				}
				log.Printf("%s[%s]: Exited", vv.Kind(), vv.GetName())
			}(v)
		}

		// let the vertices run their startup code in parallel
		wg.Add(1)
		go func(vv *Vertex) {
			defer wg.Done()
			vv.Res.Started() // block until started
		}(v)

		if !first { // unpause!
			v.Res.SendEvent(event.EventStart, nil) // sync!
		}
	}

	wg.Wait() // wait for everyone
}

// Pause sends pause events to the graph in a topological sort order.
func (g *Graph) Pause() {
	log.Printf("State: %v -> %v", g.setState(graphStatePausing), g.getState())
	defer log.Printf("State: %v -> %v", g.setState(graphStatePaused), g.getState())
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		v.SendEvent(event.EventPause, nil)
	}
}

// Exit sends exit events to the graph in a topological sort order.
func (g *Graph) Exit() {
	if g == nil {
		return
	} // empty graph that wasn't populated yet
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		// turn off the taps...
		// XXX: consider instead doing this by closing the Res.events channel instead?
		// XXX: do this by sending an exit signal, and then returning
		// when we hit the 'default' in the select statement!
		// XXX: we can do this to quiesce, but it's not necessary now

		v.SendEvent(event.EventExit, nil)
	}
	g.wg.Wait() // for now, this doesn't need to be a separate Wait() method
}
