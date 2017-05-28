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

package resources

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/prometheus"
	"github.com/purpleidea/mgmt/util"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
	"golang.org/x/time/rate"
)

// SentinelErr is a sentinal as an error type that wraps an arbitrary error.
type SentinelErr struct {
	err error
}

// Error is the required method to fulfill the error type.
func (obj *SentinelErr) Error() string {
	return obj.err.Error()
}

// OKTimestamp returns true if this element can run right now?
func (obj *BaseRes) OKTimestamp() bool {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range obj.Graph.IncomingGraphVertices(obj.Vertex) {
		// if the vertex has a greater timestamp than any pre-req (n)
		// then we can't run right now...
		// if they're equal (eg: on init of 0) then we also can't run
		// b/c we should let our pre-req's go first...
		x, y := obj.Timestamp(), VtoR(n).Timestamp()
		if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
			log.Printf("%s: OKTimestamp: (%v) >= %s(%v): !%v", obj, x, n, y, x >= y)
		}
		if x >= y {
			return false
		}
	}
	return true
}

// Poke tells nodes after me in the dependency graph that they need to refresh.
func (obj *BaseRes) Poke() error {
	// if we're pausing (or exiting) then we should suspend poke's so that
	// the graph doesn't go on running forever until it's completely done!
	// this is an optional feature which we can do by default on user exit
	if b, ok := obj.Graph.Value("fastpause"); ok && util.Bool(b) {
		return nil // TODO: should this be an error instead?
	}

	var wg sync.WaitGroup
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range obj.Graph.OutgoingGraphVertices(obj.Vertex) {
		// we can skip this poke if resource hasn't done work yet... it
		// needs to be poked if already running, or not running though!
		// TODO: does this need an || activity flag?
		if VtoR(n).GetState() != ResStateProcess {
			if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
				log.Printf("%s: Poke: %s", obj, n)
			}
			wg.Add(1)
			go func(nn pgraph.Vertex) error {
				defer wg.Done()
				//edge := obj.Graph.adjacency[v][nn] // lookup
				//notify := edge.Notify && edge.Refresh()
				return VtoR(nn).SendEvent(event.EventPoke, nil)
			}(n)

		} else {
			if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
				log.Printf("%s: Poke: %s: Skipped!", obj, n)
			}
		}
	}
	// TODO: do something with return values?
	wg.Wait() // wait for all the pokes to complete
	return nil
}

// BackPoke pokes the pre-requisites that are stale and need to run before I can run.
func (obj *BaseRes) BackPoke() {
	var wg sync.WaitGroup
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range obj.Graph.IncomingGraphVertices(obj.Vertex) {
		x, y, s := obj.Timestamp(), VtoR(n).Timestamp(), VtoR(n).GetState()
		// If the parent timestamp needs poking AND it's not running
		// Process, then poke it. If the parent is in ResStateProcess it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != ResStateProcess && s != ResStateCheckApply) {
			if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
				log.Printf("%s: BackPoke: %s", obj, n)
			}
			wg.Add(1)
			go func(nn pgraph.Vertex) error {
				defer wg.Done()
				return VtoR(nn).SendEvent(event.EventBackPoke, nil)
			}(n)

		} else {
			if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
				log.Printf("%s: BackPoke: %s: Skipped!", obj, n)
			}
		}
	}
	// TODO: do something with return values?
	wg.Wait() // wait for all the pokes to complete
}

// RefreshPending determines if any previous nodes have a refresh pending here.
// If this is true, it means I am expected to apply a refresh when I next run.
func (obj *BaseRes) RefreshPending() bool {
	var refresh bool
	for _, edge := range obj.Graph.IncomingGraphEdges(obj.Vertex) {
		// if we asked for a notify *and* if one is pending!
		edge := edge.(*Edge) // panic if wrong
		if edge.Notify && edge.Refresh() {
			refresh = true
			break
		}
	}
	return refresh
}

// SetUpstreamRefresh sets the refresh value to any upstream vertices.
func (obj *BaseRes) SetUpstreamRefresh(b bool) {
	for _, edge := range obj.Graph.IncomingGraphEdges(obj.Vertex) {
		edge := edge.(*Edge) // panic if wrong
		if edge.Notify {
			edge.SetRefresh(b)
		}
	}
}

// SetDownstreamRefresh sets the refresh value to any downstream vertices.
func (obj *BaseRes) SetDownstreamRefresh(b bool) {
	for _, edge := range obj.Graph.OutgoingGraphEdges(obj.Vertex) {
		edge := edge.(*Edge) // panic if wrong
		// if we asked for a notify *and* if one is pending!
		if edge.Notify {
			edge.SetRefresh(b)
		}
	}
}

// Process is the primary function to execute for a particular vertex in the graph.
func (obj *BaseRes) Process() error {
	if obj.debug {
		log.Printf("%s: Process()", obj)
	}
	// FIXME: should these SetState methods be here or after the sema code?
	defer obj.SetState(ResStateNil) // reset state when finished
	obj.SetState(ResStateProcess)

	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if !obj.OKTimestamp() {
		go obj.BackPoke()
		return nil
	}
	// timestamp must be okay...
	if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
		log.Printf("%s: OKTimestamp(%v)", obj, obj.Timestamp())
	}

	// semaphores!
	// These shouldn't ever block an exit, since the graph should eventually
	// converge causing their them to unlock. More interestingly, since they
	// run in a DAG alphabetically, there is no way to permanently deadlock,
	// assuming that resources individually don't ever block from finishing!
	// The exception is that semaphores with a zero count will always block!
	// TODO: Add a close mechanism to close/unblock zero count semaphores...
	semas := obj.Meta().Sema
	if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) && len(semas) > 0 {
		log.Printf("%s: Sema: P(%s)", obj, strings.Join(semas, ", "))
	}
	if err := SemaLock(obj.Graph, semas); err != nil { // lock
		// NOTE: in practice, this might not ever be truly necessary...
		return fmt.Errorf("shutdown of semaphores")
	}
	defer SemaUnlock(obj.Graph, semas) // unlock
	if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) && len(semas) > 0 {
		defer log.Printf("%s: Sema: V(%s)", obj, strings.Join(semas, ", "))
	}

	var ok = true
	var applied = false // did we run an apply?

	// connect any senders to receivers and detect if values changed
	if updated, err := obj.SendRecv(obj.Res); err != nil {
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

	if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
		log.Printf("%s: CheckApply(%t)", obj, !noop)
	}

	// lookup the refresh (notification) variable
	refresh = obj.RefreshPending() // do i need to perform a refresh?
	obj.SetRefresh(refresh)        // tell the resource

	// changes can occur after this...
	obj.SetState(ResStateCheckApply)

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
		checkOK, err = obj.Res.CheckApply(!noop)

		if promErr := obj.Data().Prometheus.UpdateCheckApplyTotal(obj.GetKind(), !noop, !checkOK, err != nil); promErr != nil {
			// TODO: how to error correctly
			log.Printf("%s: Prometheus.UpdateCheckApplyTotal() errored: %v", obj, err)
		}
		// TODO: Can the `Poll` converged timeout tracking be a
		// more general method for all converged timeouts? this
		// would simplify the resources by removing boilerplate
		if obj.Meta().Poll > 0 {
			if !checkOK { // something changed, restart timer
				obj.cuid.ResetTimer() // activity!
				if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
					log.Printf("%s: Converger: ResetTimer", obj)
				}
			}
		}
	}

	if checkOK && err != nil { // should never return this way
		log.Fatalf("%s: CheckApply(): %t, %+v", obj, checkOK, err)
	}
	if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
		log.Printf("%s: CheckApply(): %t, %v", obj, checkOK, err)
	}

	// if CheckApply ran without noop and without error, state should be good
	if !noop && err == nil { // aka !noop || checkOK
		obj.StateOK(true) // reset
		if refresh {
			obj.SetUpstreamRefresh(false) // refresh happened, clear the request
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
			obj.SetDownstreamRefresh(true)
		}

		// update this timestamp *before* we poke or the poked
		// nodes might fail due to having a too old timestamp!
		obj.UpdateTimestamp()        // this was touched...
		obj.SetState(ResStatePoking) // can't cancel parent poke
		if err := obj.Poke(); err != nil {
			return errwrap.Wrapf(err, "the Poke() failed")
		}
	}
	// poke at our pre-req's instead since they need to refresh/run...
	return errwrap.Wrapf(err, "could not Process() successfully")
}

// innerWorker is the CheckApply runner that reads from processChan.
func (obj *BaseRes) innerWorker() {
	running := false
	done := make(chan struct{})
	playback := false // do we need to run another one?

	waiting := false
	var timer = time.NewTimer(time.Duration(math.MaxInt64)) // longest duration
	if !timer.Stop() {
		<-timer.C // unnecessary, shouldn't happen
	}

	var delay = time.Duration(obj.Meta().Delay) * time.Millisecond
	var retry = obj.Meta().Retry // number of tries left, -1 for infinite
	var limiter = rate.NewLimiter(obj.Meta().Limit, obj.Meta().Burst)
	limited := false

	wg := &sync.WaitGroup{} // wait for Process routine to exit

Loop:
	for {
		select {
		case ev, ok := <-obj.processChan: // must use like this
			if !ok { // processChan closed, let's exit
				break Loop // no event, so no ack!
			}
			if obj.Meta().Poll == 0 { // skip for polling
				obj.wcuid.SetConverged(false)
			}

			// if process started, but no action yet, skip!
			if obj.GetState() == ResStateProcess {
				if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
					log.Printf("%s: Skipped event!", obj)
				}
				ev.ACK() // ready for next message
				obj.quiesceGroup.Done()
				continue
			}

			// if running, we skip running a new execution!
			// if waiting, we skip running a new execution!
			if running || waiting {
				if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
					log.Printf("%s: Playback added!", obj)
				}
				playback = true
				ev.ACK() // ready for next message
				obj.quiesceGroup.Done()
				continue
			}

			// catch invalid rates
			if obj.Meta().Burst == 0 && !(obj.Meta().Limit == rate.Inf) { // blocked
				e := fmt.Errorf("%s: Permanently limited (rate != Inf, burst: 0)", obj)
				ev.ACK() // ready for next message
				obj.quiesceGroup.Done()
				obj.SendEvent(event.EventExit, &SentinelErr{e})
				continue
			}

			// rate limit
			// FIXME: consider skipping rate limit check if
			// the event is a poke instead of a watch event
			if !limited && !(obj.Meta().Limit == rate.Inf) { // skip over the playback event...
				now := time.Now()
				r := limiter.ReserveN(now, 1) // one event
				// r.OK() seems to always be true here!
				d := r.DelayFrom(now)
				if d > 0 { // delay
					limited = true
					playback = true
					log.Printf("%s: Limited (rate: %v/sec, burst: %d, next: %v)", obj, obj.Meta().Limit, obj.Meta().Burst, d)
					// start the timer...
					timer.Reset(d)
					waiting = true // waiting for retry timer
					ev.ACK()
					obj.quiesceGroup.Done()
					continue
				} // otherwise, we run directly!
			}
			limited = false // let one through

			wg.Add(1)
			running = true
			go func(ev *event.Event) {
				obj.pcuid.SetConverged(false) // "block" Process
				defer wg.Done()
				if e := obj.Process(); e != nil {
					playback = true
					log.Printf("%s: CheckApply errored: %v", obj, e)
					if retry == 0 {
						if err := obj.Data().Prometheus.UpdateState(obj.String(), obj.GetKind(), prometheus.ResStateHardFail); err != nil {
							// TODO: how to error this?
							log.Printf("%s: Prometheus.UpdateState() errored: %v", obj, err)
						}

						// wrap the error in the sentinel
						obj.quiesceGroup.Done() // before the Wait that happens in SendEvent!
						obj.SendEvent(event.EventExit, &SentinelErr{e})
						return
					}
					if retry > 0 { // don't decrement the -1
						retry--
					}
					if err := obj.Data().Prometheus.UpdateState(obj.String(), obj.GetKind(), prometheus.ResStateSoftFail); err != nil {
						// TODO: how to error this?
						log.Printf("%s: Prometheus.UpdateState() errored: %v", obj, err)
					}
					log.Printf("%s: CheckApply: Retrying after %.4f seconds (%d left)", obj, delay.Seconds(), retry)
					// start the timer...
					timer.Reset(delay)
					waiting = true // waiting for retry timer
					// don't obj.quiesceGroup.Done() b/c
					// the timer is running and it can exit!
					return
				}
				retry = obj.Meta().Retry // reset on success
				close(done)              // trigger
			}(ev)
			ev.ACK() // sync (now mostly useless)

		case <-timer.C:
			if obj.Meta().Poll == 0 { // skip for polling
				obj.wcuid.SetConverged(false)
			}
			waiting = false
			if !timer.Stop() {
				//<-timer.C // blocks, docs are wrong!
			}
			log.Printf("%s: CheckApply delay expired!", obj)
			close(done)

		// a CheckApply run (with possibly retry pause) finished
		case <-done:
			if obj.Meta().Poll == 0 { // skip for polling
				obj.wcuid.SetConverged(false)
			}
			if b, ok := obj.Graph.Value("debug"); ok && util.Bool(b) {
				log.Printf("%s: CheckApply finished!", obj)
			}
			done = make(chan struct{}) // reset
			// re-send this event, to trigger a CheckApply()
			if playback {
				// this lock avoids us sending to
				// channel after we've closed it!
				// TODO: can this experience indefinite postponement ?
				// see: https://github.com/golang/go/issues/11506
				// pause or exit is in process if not quiescing!
				if !obj.quiescing {
					playback = false
					obj.quiesceGroup.Add(1) // lock around it, b/c still running...
					go func() {
						obj.Event() // replay a new event
						obj.quiesceGroup.Done()
					}()
				}
			}
			running = false
			obj.pcuid.SetConverged(true) // "unblock" Process
			obj.quiesceGroup.Done()

		case <-obj.wcuid.ConvergedTimer():
			obj.wcuid.SetConverged(true) // converged!
			continue
		}
	}
	wg.Wait()
	return
}

// Worker is the common run frontend of the vertex. It handles all of the retry
// and retry delay common code, and ultimately returns the final status of this
// vertex execution.
func (obj *BaseRes) Worker() error {
	// listen for chan events from Watch() and run
	// the Process() function when they're received
	// this avoids us having to pass the data into
	// the Watch() function about which graph it is
	// running on, which isolates things nicely...
	if obj.debug {
		log.Printf("%s: Worker: Running", obj)
		defer log.Printf("%s: Worker: Stopped", obj)
	}
	// run the init (should match 1-1 with Close function)
	if err := obj.Res.Init(); err != nil {
		obj.ProcessExit()
		// always exit the worker function by finishing with Close()
		if e := obj.Res.Close(); e != nil {
			err = multierr.Append(err, e) // list of errors
		}
		return errwrap.Wrapf(err, "could not Init() resource")
	}

	// if the CheckApply run takes longer than the converged
	// timeout, we could inappropriately converge mid-apply!
	// avoid this by blocking convergence with a fake report
	// we also add a similar blocker around the worker loop!
	// XXX: put these in Init() ?
	// get extra cuids (worker, process)
	obj.wcuid.SetConverged(true) // starts off false, and waits for loop timeout
	obj.pcuid.SetConverged(true) // starts off true, because it's not running...

	obj.processSync.Add(1)
	go func() {
		defer obj.processSync.Done()
		obj.innerWorker()
	}()

	var err error // propagate the error up (this is a permanent BAD error!)
	// the watch delay runs inside of the Watch resource loop, so that it
	// can still process signals and exit if needed. It shouldn't run any
	// resource specific code since this is supposed to be a retry delay.
	// NOTE: we're using the same retry and delay metaparams that CheckApply
	// uses. This is for practicality. We can separate them later if needed!
	var watchDelay time.Duration
	var watchRetry = obj.Meta().Retry // number of tries left, -1 for infinite
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
						obj.ProcessExit()
						err := *exit // exit err
						if e := obj.Res.Close(); err == nil {
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
			log.Printf("%s: Watch delay expired!", obj)
			// NOTE: we can avoid the send if running Watch guarantees
			// one CheckApply event on startup!
			//if pendingSendEvent { // TODO: should this become a list in the future?
			//	if err := obj.Event() err != nil {
			//		return err // we exit or bubble up a NACK...
			//	}
			//}
		}

		// TODO: reset the watch retry count after some amount of success
		var e error
		if obj.Meta().Poll > 0 { // poll instead of watching :(
			obj.cuid.StartTimer()
			e = obj.Poll()
			obj.cuid.StopTimer() // clean up nicely
		} else {
			e = obj.Res.Watch() // run the watch normally
		}
		if e == nil { // exit signal
			err = nil // clean exit
			break
		}
		if sentinelErr, ok := e.(*SentinelErr); ok { // unwrap the sentinel
			err = sentinelErr.err
			break // sentinel means, perma-exit
		}
		log.Printf("%s: Watch errored: %v", obj, e)
		if watchRetry == 0 {
			err = fmt.Errorf("Permanent watch error: %v", e)
			break
		}
		if watchRetry > 0 { // don't decrement the -1
			watchRetry--
		}
		watchDelay = time.Duration(obj.Meta().Delay) * time.Millisecond
		log.Printf("%s: Watch: Retrying after %.4f seconds (%d left)", obj, watchDelay.Seconds(), watchRetry)
		// We need to trigger a CheckApply after Watch restarts, so that
		// we catch any lost events that happened while down. We do this
		// by getting the Watch resource to send one event once it's up!
		//v.SendEvent(eventPoke, false, false)
	}

	obj.ProcessExit()
	// close resource and return possible errors if any
	if e := obj.Res.Close(); err == nil {
		err = e
	} else if e != nil {
		err = multierr.Append(err, e) // list of errors
	}
	return err
}
