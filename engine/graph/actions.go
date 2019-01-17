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

package graph

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/event"
	"github.com/purpleidea/mgmt/pgraph"

	//multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
	"golang.org/x/time/rate"
)

// OKTimestamp returns true if this vertex can run right now.
func (obj *Engine) OKTimestamp(vertex pgraph.Vertex) bool {
	return len(obj.BadTimestamps(vertex)) == 0
}

// BadTimestamps returns the list of vertices that are causing our timestamp to
// be bad.
func (obj *Engine) BadTimestamps(vertex pgraph.Vertex) []pgraph.Vertex {
	vs := []pgraph.Vertex{}
	ts := obj.state[vertex].timestamp
	// these are all the vertices pointing TO vertex, eg: ??? -> vertex
	for _, v := range obj.graph.IncomingGraphVertices(vertex) {
		// If the vertex has a greater timestamp than any prerequisite,
		// then we can't run right now. If they're equal (eg: initially
		// with a value of 0) then we also can't run because we should
		// let our pre-requisites go first.
		t := obj.state[v].timestamp
		if obj.Debug {
			obj.Logf("OKTimestamp: %d >= %d (%s): !%t", ts, t, v.String(), ts >= t)
		}
		if ts >= t {
			//return false
			vs = append(vs, v)
		}
	}
	return vs // formerly "true" if empty
}

// Process is the primary function to execute a particular vertex in the graph.
func (obj *Engine) Process(vertex pgraph.Vertex) error {
	res, isRes := vertex.(engine.Res)
	if !isRes {
		return fmt.Errorf("vertex is not a Res")
	}

	// Engine Guarantee: Do not allow CheckApply to run while we are paused.
	// This makes the resource able to know that synchronous channel sending
	// to the main loop select in Watch from within CheckApply, will succeed
	// without blocking because the resource went into a paused state. If we
	// are using the Poll metaparam, then Watch will (of course) not be run.
	// FIXME: should this lock be here, or wrapped right around CheckApply ?
	obj.state[vertex].eventsLock.Lock() // this lock is taken within Event()
	defer obj.state[vertex].eventsLock.Unlock()

	// backpoke! (can be async)
	if vs := obj.BadTimestamps(vertex); len(vs) > 0 {
		// back poke in parallel (sync b/c of waitgroup)
		for _, v := range obj.graph.IncomingGraphVertices(vertex) {
			if !pgraph.VertexContains(v, vs) { // only poke what's needed
				continue
			}

			go obj.state[v].Poke() // async

		}
		return nil // can't continue until timestamp is in sequence
	}

	// semaphores!
	// These shouldn't ever block an exit, since the graph should eventually
	// converge causing their them to unlock. More interestingly, since they
	// run in a DAG alphabetically, there is no way to permanently deadlock,
	// assuming that resources individually don't ever block from finishing!
	// The exception is that semaphores with a zero count will always block!
	// TODO: Add a close mechanism to close/unblock zero count semaphores...
	semas := res.MetaParams().Sema
	if obj.Debug && len(semas) > 0 {
		obj.Logf("%s: Sema: P(%s)", res, strings.Join(semas, ", "))
	}
	if err := obj.semaLock(semas); err != nil { // lock
		// NOTE: in practice, this might not ever be truly necessary...
		return fmt.Errorf("shutdown of semaphores")
	}
	defer obj.semaUnlock(semas) // unlock
	if obj.Debug && len(semas) > 0 {
		defer obj.Logf("%s: Sema: V(%s)", res, strings.Join(semas, ", "))
	}

	// sendrecv!
	// connect any senders to receivers and detect if values changed
	if res, ok := vertex.(engine.RecvableRes); ok {
		if updated, err := obj.SendRecv(res); err != nil {
			return errwrap.Wrapf(err, "could not SendRecv")
		} else if len(updated) > 0 {
			for _, changed := range updated {
				if changed { // at least one was updated
					// invalidate cache, mark as dirty
					obj.state[vertex].tuid.StopTimer()
					obj.state[vertex].isStateOK = false
					break
				}
			}
			// re-validate after we change any values
			if err := engine.Validate(res); err != nil {
				return errwrap.Wrapf(err, "failed Validate after SendRecv")
			}
		}
	}

	var ok = true
	var applied = false              // did we run an apply?
	var noop = res.MetaParams().Noop // lookup the noop value
	var refresh bool
	var checkOK bool
	var err error

	// lookup the refresh (notification) variable
	refresh = obj.RefreshPending(vertex) // do i need to perform a refresh?
	refreshableRes, isRefreshableRes := vertex.(engine.RefreshableRes)
	if isRefreshableRes {
		refreshableRes.SetRefresh(refresh) // tell the resource
	}

	// Check cached state, to skip CheckApply, but can't skip if refreshing!
	// If the resource doesn't implement refresh, skip the refresh test.
	// FIXME: if desired, check that we pass through refresh notifications!
	if (!refresh || !isRefreshableRes) && obj.state[vertex].isStateOK {
		checkOK, err = true, nil

	} else if noop && (refresh && isRefreshableRes) { // had a refresh to do w/ noop!
		checkOK, err = false, nil // therefore the state is wrong

		// run the CheckApply!
	} else {
		obj.Logf("%s: CheckApply(%t)", res, !noop)
		// if this fails, don't UpdateTimestamp()
		checkOK, err = res.CheckApply(!noop)
		obj.Logf("%s: CheckApply(%t): Return(%t, %+v)", res, !noop, checkOK, err)
	}

	if checkOK && err != nil { // should never return this way
		return fmt.Errorf("%s: resource programming error: CheckApply(%t): %t, %+v", res, !noop, checkOK, err)
	}

	if !checkOK { // something changed, restart timer
		obj.state[vertex].cuid.ResetTimer() // activity!
		if obj.Debug {
			obj.Logf("%s: converger: reset timer", res)
		}
	}

	// if CheckApply ran without noop and without error, state should be good
	if !noop && err == nil { // aka !noop || checkOK
		obj.state[vertex].tuid.StartTimer()
		obj.state[vertex].isStateOK = true // reset
		if refresh {
			obj.SetUpstreamRefresh(vertex, false) // refresh happened, clear the request
			if isRefreshableRes {
				refreshableRes.SetRefresh(false)
			}
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
			obj.SetDownstreamRefresh(vertex, true)
		}

		// poke! (should (must?) be sync)
		wg := &sync.WaitGroup{}
		// update this timestamp *before* we poke or the poked
		// nodes might fail due to having a too old timestamp!
		obj.state[vertex].timestamp = time.Now().UnixNano() // update timestamp
		for _, v := range obj.graph.OutgoingGraphVertices(vertex) {
			if !obj.OKTimestamp(v) {
				// there is at least another one that will poke this...
				continue
			}

			// If we're pausing (or exiting) then we can skip poking
			// so that the graph doesn't go on running forever until
			// it's completely done. This is an optional feature and
			// we can select it via ^C on user exit or via the GAPI.
			if obj.fastPause {
				obj.Logf("%s: fast pausing, poke skipped", res)
				continue
			}

			// poke each vertex individually, in parallel...
			wg.Add(1)
			go func(vv pgraph.Vertex) {
				defer wg.Done()
				obj.state[vv].Poke()
			}(v)
		}
		wg.Wait()
	}

	return errwrap.Wrapf(err, "error during Process()")
}

// Worker is the common run frontend of the vertex. It handles all of the retry
// and retry delay common code, and ultimately returns the final status of this
// vertex execution.
func (obj *Engine) Worker(vertex pgraph.Vertex) error {
	res, isRes := vertex.(engine.Res)
	if !isRes {
		return fmt.Errorf("vertex is not a resource")
	}

	defer close(obj.state[vertex].stopped) // done signal

	obj.state[vertex].cuid = obj.Converger.Register()
	obj.state[vertex].tuid = obj.Converger.Register()
	// must wait for all users of the cuid to finish *before* we unregister!
	// as a result, this defer happens *before* the below wait group Wait...
	defer obj.state[vertex].cuid.Unregister()
	defer obj.state[vertex].tuid.Unregister()

	defer obj.state[vertex].wg.Wait() // this Worker is the last to exit!

	obj.state[vertex].wg.Add(1)
	go func() {
		defer obj.state[vertex].wg.Done()
		defer close(obj.state[vertex].outputChan) // we close this on behalf of res

		var err error
		var retry = res.MetaParams().Retry // lookup the retry value
		var delay uint64
		for { // retry loop
			// a retry-delay was requested, wait, but don't block events!
			if delay > 0 {
				errDelayExpired := engine.Error("delay exit")
				err = func() error { // slim watch main loop
					timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
					defer obj.state[vertex].init.Logf("the Watch delay expired!")
					defer timer.Stop() // it's nice to cleanup
					for {
						select {
						case <-timer.C: // the wait is over
							return errDelayExpired // special

						case event, ok := <-obj.state[vertex].init.Events:
							if !ok {
								return nil
							}
							if err := obj.state[vertex].init.Read(event); err != nil {
								return err
							}
						}
					}
				}()
				if err == errDelayExpired {
					delay = 0 // reset
					continue
				}
			} else if interval := res.MetaParams().Poll; interval > 0 { // poll instead of watching :(
				obj.state[vertex].cuid.StartTimer()
				err = obj.state[vertex].poll(interval)
				obj.state[vertex].cuid.StopTimer() // clean up nicely
			} else {
				obj.state[vertex].cuid.StartTimer()
				obj.Logf("Watch(%s)", vertex)
				err = res.Watch() // run the watch normally
				obj.Logf("Watch(%s): Exited(%+v)", vertex, err)
				obj.state[vertex].cuid.StopTimer() // clean up nicely
			}
			if err == nil || err == engine.ErrWatchExit || err == engine.ErrSignalExit {
				return // exited cleanly, we're done
			}
			// we've got an error...
			delay = res.MetaParams().Delay

			if retry < 0 { // infinite retries
				obj.state[vertex].reset()
				continue
			}
			if retry > 0 { // don't decrement past 0
				retry--
				obj.state[vertex].init.Logf("retrying Watch after %.4f seconds (%d left)", float64(delay)/1000, retry)
				obj.state[vertex].reset()
				continue
			}
			//if retry == 0 { // optional
			//	err = errwrap.Wrapf(err, "permanent watch error")
			//}
			break // break out of this and send the error
		}
		// this section sends an error...
		// If the CheckApply loop exits and THEN the Watch fails with an
		// error, then we'd be stuck here if exit signal didn't unblock!
		select {
		case obj.state[vertex].outputChan <- errwrap.Wrapf(err, "watch failed"):
			// send
		case <-obj.state[vertex].exit.Signal():
			// pass
		}
	}()

	// bonus safety check
	if res.MetaParams().Burst == 0 && !(res.MetaParams().Limit == rate.Inf) { // blocked
		return fmt.Errorf("permanently limited (rate != Inf, burst = 0)")
	}
	var limiter = rate.NewLimiter(res.MetaParams().Limit, res.MetaParams().Burst)
	// It is important that we shutdown the Watch loop if this exits.
	// Example, if Process errors permanently, we should ask Watch to exit.
	defer obj.state[vertex].Event(event.EventExit) // signal an exit
	for {
		select {
		case err, ok := <-obj.state[vertex].outputChan: // read from watch channel
			if !ok {
				return nil
			}
			if err != nil {
				return err // permanent failure
			}

			// safe to go run the process...
		case <-obj.state[vertex].exit.Signal(): // TODO: is this needed?
			return nil
		}

		now := time.Now()
		r := limiter.ReserveN(now, 1) // one event
		// r.OK() seems to always be true here!
		d := r.DelayFrom(now)
		if d > 0 { // delay
			obj.state[vertex].init.Logf("limited (rate: %v/sec, burst: %d, next: %v)", res.MetaParams().Limit, res.MetaParams().Burst, d)
			var count int
			timer := time.NewTimer(time.Duration(d) * time.Millisecond)
		LimitWait:
			for {
				select {
				case <-timer.C: // the wait is over
					break LimitWait

				// consume other events while we're waiting...
				case e, ok := <-obj.state[vertex].outputChan: // read from watch channel
					if !ok {
						// FIXME: is this logic correct?
						if count == 0 {
							return nil
						}
						// loop, because we have
						// the previous event to
						// run process on first!
						continue
					}
					if e != nil {
						return e // permanent failure
					}
					count++                         // count the events...
					limiter.ReserveN(time.Now(), 1) // one event
				}
			}
			timer.Stop() // it's nice to cleanup
			obj.state[vertex].init.Logf("rate limiting expired!")
		}

		var err error
		var retry = res.MetaParams().Retry // lookup the retry value
		var delay uint64
	Loop:
		for { // retry loop
			if delay > 0 {
				var count int
				timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
			RetryWait:
				for {
					select {
					case <-timer.C: // the wait is over
						break RetryWait

					// consume other events while we're waiting...
					case e, ok := <-obj.state[vertex].outputChan: // read from watch channel
						if !ok {
							// FIXME: is this logic correct?
							if count == 0 {
								// last process error
								return err
							}
							// loop, because we have
							// the previous event to
							// run process on first!
							continue
						}
						if e != nil {
							return e // permanent failure
						}
						count++                         // count the events...
						limiter.ReserveN(time.Now(), 1) // one event
					}
				}
				timer.Stop() // it's nice to cleanup
				delay = 0    // reset
				obj.state[vertex].init.Logf("the CheckApply delay expired!")
			}

			if obj.Debug {
				obj.Logf("Process(%s)", vertex)
			}
			err = obj.Process(vertex)
			if obj.Debug {
				obj.Logf("Process(%s): Return(%+v)", vertex, err)
			}
			if err == nil {
				break Loop
			}
			// we've got an error...
			delay = res.MetaParams().Delay

			if retry < 0 { // infinite retries
				continue
			}
			if retry > 0 { // don't decrement past 0
				retry--
				obj.state[vertex].init.Logf("retrying CheckApply after %.4f seconds (%d left)", float64(delay)/1000, retry)
				continue
			}
			//if retry == 0 { // optional
			//	err = errwrap.Wrapf(err, "permanent process error")
			//}

			// If this exits, defer calls Event(event.EventExit),
			// which will cause the Watch loop to shutdown. Also,
			// if the Watch loop shuts down, that will cause this
			// Process loop to shut down. Also the graph sync can
			// run an Event(event.EventExit) which causes this to
			// shutdown as well. Lastly, it is possible that more
			// that one of these scenarios happens simultaneously.
			return err
		}
	}
	//return nil // unreachable
}
