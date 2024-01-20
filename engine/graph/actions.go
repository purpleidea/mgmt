// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"

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
	obj.state[vertex].mutex.RLock()   // concurrent read start
	ts := obj.state[vertex].timestamp // race
	obj.state[vertex].mutex.RUnlock() // concurrent read end
	// these are all the vertices pointing TO vertex, eg: ??? -> vertex
	for _, v := range obj.graph.IncomingGraphVertices(vertex) {
		// If the vertex has a greater timestamp than any prerequisite,
		// then we can't run right now. If they're equal (eg: initially
		// with a value of 0) then we also can't run because we should
		// let our pre-requisites go first.
		obj.state[v].mutex.RLock()   // concurrent read start
		t := obj.state[v].timestamp  // race
		obj.state[v].mutex.RUnlock() // concurrent read end
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
func (obj *Engine) Process(ctx context.Context, vertex pgraph.Vertex) error {
	res, isRes := vertex.(engine.Res)
	if !isRes {
		return fmt.Errorf("vertex is not a Res")
	}

	// backpoke! (can be async)
	if vs := obj.BadTimestamps(vertex); len(vs) > 0 {
		// back poke in parallel (sync b/c of waitgroup)
		wg := &sync.WaitGroup{}
		for _, v := range obj.graph.IncomingGraphVertices(vertex) {
			if !pgraph.VertexContains(v, vs) { // only poke what's needed
				continue
			}

			// doesn't really need to be in parallel, but we can...
			wg.Add(1)
			go func(vv pgraph.Vertex) {
				defer wg.Done()
				obj.state[vv].Poke() // async
			}(v)

		}
		wg.Wait()

		// can't continue until timestamp is in sequence, defer for now
		return engine.ErrBackPoke
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
	// this actually checks and sends into resource trees recursively...
	if res, ok := vertex.(engine.RecvableRes); ok {
		if updated, err := obj.SendRecv(res); err != nil {
			return errwrap.Wrapf(err, "could not SendRecv")
		} else if len(updated) > 0 {
			for r, m := range updated { // map[engine.RecvableRes]map[string]bool
				v, ok := r.(pgraph.Vertex)
				if !ok {
					continue
				}
				_, stateExists := obj.state[v] // autogrouped children probably don't have a state
				if !stateExists {
					continue
				}
				for _, changed := range m {
					if !changed {
						continue
					}
					// if changed == true, at least one was updated
					// invalidate cache, mark as dirty
					obj.state[v].setDirty()
					//break // we might have more vertices now
				}

				// re-validate after we change any values
				if err := engine.Validate(r); err != nil {
					return errwrap.Wrapf(err, "failed Validate after SendRecv")
				}
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
	if (!refresh || !isRefreshableRes) && obj.state[vertex].isStateOK.Load() { // mutex RLock/RUnlock
		checkOK, err = true, nil

	} else if noop && (refresh && isRefreshableRes) { // had a refresh to do w/ noop!
		checkOK, err = false, nil // therefore the state is wrong

	} else {
		// run the CheckApply!
		obj.Logf("%s: CheckApply(%t)", res, !noop)
		// if this fails, don't UpdateTimestamp()
		checkOK, err = res.CheckApply(ctx, !noop)
		obj.Logf("%s: CheckApply(%t): Return(%t, %s)", res, !noop, checkOK, engineUtil.CleanError(err))
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
		//obj.state[vertex].mutex.Lock()
		obj.state[vertex].isStateOK.Store(true) // reset
		//obj.state[vertex].mutex.Unlock()
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
		obj.state[vertex].mutex.Lock()                      // concurrent write start
		obj.state[vertex].timestamp = time.Now().UnixNano() // update timestamp (race)
		obj.state[vertex].mutex.Unlock()                    // concurrent write end
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
// vertex execution. This function cannot be "re-run" for the same vertex. The
// retry mechanism stuff happens inside of this. To actually "re-run" you need
// to remove the vertex and build a new one. The engine guarantees that we do
// not allow CheckApply to run while we are paused. That is enforced here.
func (obj *Engine) Worker(vertex pgraph.Vertex) error {
	res, isRes := vertex.(engine.Res)
	if !isRes {
		return fmt.Errorf("vertex is not a resource")
	}

	// bonus safety check
	if res.MetaParams().Burst == 0 && !(res.MetaParams().Limit == rate.Inf) { // blocked
		return fmt.Errorf("permanently limited (rate != Inf, burst = 0)")
	}

	// initialize or reinitialize the meta state for this resource uid
	obj.mlock.Lock()
	if _, exists := obj.metas[engine.PtrUID(res)]; !exists || res.MetaParams().Reset {
		obj.metas[engine.PtrUID(res)] = &engine.MetaState{
			CheckApplyRetry: res.MetaParams().Retry, // lookup the retry value
		}
	}
	metas := obj.metas[engine.PtrUID(res)] // handle
	obj.mlock.Unlock()

	//defer close(obj.state[vertex].stopped) // done signal

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
		defer close(obj.state[vertex].eventsChan) // we close this on behalf of res

		// This is a close reverse-multiplexer. If any of the channels
		// close, then it will cause the doneCtx to cancel. That way,
		// multiple different folks can send a close signal, without
		// every worrying about duplicate channel close panics.
		obj.state[vertex].wg.Add(1)
		go func() {
			defer obj.state[vertex].wg.Done()

			// reverse-multiplexer: any close, causes *the* close!
			select {
			case <-obj.state[vertex].processDone:
			case <-obj.state[vertex].watchDone:
			case <-obj.state[vertex].limitDone:
			case <-obj.state[vertex].retryDone:
			case <-obj.state[vertex].removeDone:
			case <-obj.state[vertex].eventsDone:
			}

			// the main "done" signal gets activated here!
			obj.state[vertex].doneCtxCancel() // cancels doneCtx
		}()

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

						case <-obj.state[vertex].doneCtx.Done():
							return nil
						}
					}
				}()
				if err == errDelayExpired {
					delay = 0 // reset
					continue
				}
			} else if interval := res.MetaParams().Poll; interval > 0 { // poll instead of watching :(
				obj.state[vertex].cuid.StartTimer()
				err = obj.state[vertex].poll(obj.state[vertex].doneCtx, interval)
				obj.state[vertex].cuid.StopTimer() // clean up nicely
			} else {
				obj.state[vertex].cuid.StartTimer()
				if obj.Debug {
					obj.Logf("%s: Watch...", vertex)
				}
				err = res.Watch(obj.state[vertex].doneCtx) // run the watch normally
				if obj.Debug {
					if s := engineUtil.CleanError(err); err != nil {
						obj.Logf("%s: Watch Error: %s", vertex, s)
					} else {
						obj.Logf("%s: Watch Exited...", vertex)
					}
				}
				obj.state[vertex].cuid.StopTimer() // clean up nicely
			}
			if err == nil { // || err == engine.ErrClosed
				return // exited cleanly, we're done
			}
			// we've got an error...
			delay = res.MetaParams().Delay

			if retry < 0 { // infinite retries
				continue
			}
			if retry > 0 { // don't decrement past 0
				retry--
				obj.state[vertex].init.Logf("retrying Watch after %.4f seconds (%d left)", float64(delay)/1000, retry)
				continue
			}
			//if retry == 0 { // optional
			//	err = errwrap.Wrapf(err, "permanent watch error")
			//}
			break // break out of this and send the error
		} // for retry loop

		// this section sends an error...
		// If the CheckApply loop exits and THEN the Watch fails with an
		// error, then we'd be stuck here if exit signal didn't unblock!
		select {
		case obj.state[vertex].eventsChan <- errwrap.Wrapf(err, "watch failed"):
			// send
		}
	}()

	// If this exits cleanly, we must unblock the reverse-multiplexer.
	// I think this additional close is unnecessary, but it's not harmful.
	defer close(obj.state[vertex].eventsDone) // causes doneCtx to cancel
	limiter := rate.NewLimiter(res.MetaParams().Limit, res.MetaParams().Burst)
	var reserv *rate.Reservation
	var reterr error
	var failed bool // has Process permanently failed?
	var closed bool // has the resumeSignal channel closed?
Loop:
	for { // process loop
		// This is the main select where things happen and where we exit
		// from. It's similar to the two "satellite" select's which we
		// might spend some time in if we're retrying or rate limiting.
		// This select is also the main event receiver and is also the
		// only place where we read from the poke channel.
		select {
		case err, ok := <-obj.state[vertex].eventsChan: // read from watch channel
			if !ok {
				return reterr // we only return when chan closes
			}
			// If the Watch method exits with an error, then this
			// channel will get that error propagated to it, which
			// we then save so we can return it to the caller of us.
			if err != nil {
				failed = true
				close(obj.state[vertex].watchDone)   // causes doneCtx to cancel
				reterr = errwrap.Append(reterr, err) // permanent failure
				continue
			}
			if obj.Debug {
				obj.Logf("event received")
			}
			reserv = limiter.ReserveN(time.Now(), 1) // one event
			// reserv.OK() seems to always be true here!

		case _, ok := <-obj.state[vertex].pokeChan: // read from buffered poke channel
			if !ok { // we never close it
				panic("unexpected close of poke channel")
			}
			if obj.Debug {
				obj.Logf("poke received")
			}
			reserv = nil // we didn't receive a real event here...

		case _, ok := <-obj.state[vertex].pauseSignal: // one message
			if !ok {
				obj.state[vertex].pauseSignal = nil
				continue // this is not a new pause message
			}
			// NOTE: If we allowed a doneCtx below to let us out
			// of the resumeSignal wait, then we could loop around
			// and run this again, causing a panic. Instead of this
			// being made safe with a sync.Once, we instead run a
			// close() call inside of the vertexRemoveFn function,
			// which should unblock resumeSignal so we can shutdown.

			// we are paused now, and waiting for resume or exit...
			select {
			case _, ok := <-obj.state[vertex].resumeSignal: // channel closes
				if !ok {
					closed = true
				}
				// resumed!
				// pass through to allow a Process to try to run
				// TODO: consider adding this fast pause here...
				//if obj.fastPause {
				//	obj.Logf("fast pausing on resume")
				//	continue
				//}
			}
		}

		// drop redundant pokes
		for len(obj.state[vertex].pokeChan) > 0 {
			select {
			case <-obj.state[vertex].pokeChan:
			default:
				// race, someone else read one!
			}
		}

		// don't Process anymore if we've already failed or shutdown...
		if failed || closed {
			continue Loop
		}

		// limit delay
		d := time.Duration(0)
		if reserv != nil {
			d = reserv.DelayFrom(time.Now())
		}
		if reserv != nil && d > 0 { // delay
			obj.state[vertex].init.Logf("limited (rate: %v/sec, burst: %d, next: %dms)", res.MetaParams().Limit, res.MetaParams().Burst, d/time.Millisecond)
			timer := time.NewTimer(time.Duration(d) * time.Millisecond)
		LimitWait:
			for {
				// This "satellite" select doesn't need a poke
				// channel because we're already in "event
				// received" mode, and poke doesn't block.
				select {
				case <-timer.C: // the wait is over
					break LimitWait

				// consume other events while we're waiting...
				case e, ok := <-obj.state[vertex].eventsChan: // read from watch channel
					if !ok {
						return reterr // we only return when chan closes
					}
					if e != nil {
						failed = true
						close(obj.state[vertex].limitDone) // causes doneCtx to cancel
						reterr = errwrap.Append(reterr, e) // permanent failure
						break LimitWait
					}
					if obj.Debug {
						obj.Logf("event received in limit")
					}
					// TODO: does this get added in properly?
					limiter.ReserveN(time.Now(), 1) // one event

				// this pause/resume block is the same as the upper main one
				case _, ok := <-obj.state[vertex].pauseSignal:
					if !ok {
						obj.state[vertex].pauseSignal = nil
						break LimitWait
					}
					select {
					case _, ok := <-obj.state[vertex].resumeSignal: // channel closes
						if !ok {
							closed = true
						}
						// resumed!
					}
				}
			}
			timer.Stop() // it's nice to cleanup
			obj.state[vertex].init.Logf("rate limiting expired!")
		}
		// don't Process anymore if we've already failed or shutdown...
		if failed || closed {
			continue Loop
		}
		// end of limit delay

		// retry...
		var err error
		//var retry = res.MetaParams().Retry // lookup the retry value
		var delay uint64
	RetryLoop:
		for { // retry loop
			if delay > 0 {
				timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
			RetryWait:
				for {
					// This "satellite" select doesn't need
					// a poke channel because we're already
					// in "event received" mode, and poke
					// doesn't block.
					select {
					case <-timer.C: // the wait is over
						break RetryWait

					// consume other events while we're waiting...
					case e, ok := <-obj.state[vertex].eventsChan: // read from watch channel
						if !ok {
							return reterr // we only return when chan closes
						}
						if e != nil {
							failed = true
							close(obj.state[vertex].retryDone) // causes doneCtx to cancel
							reterr = errwrap.Append(reterr, e) // permanent failure
							break RetryWait
						}
						if obj.Debug {
							obj.Logf("event received in retry")
						}
						// TODO: does this get added in properly?
						limiter.ReserveN(time.Now(), 1) // one event

					// this pause/resume block is the same as the upper main one
					case _, ok := <-obj.state[vertex].pauseSignal:
						if !ok {
							obj.state[vertex].pauseSignal = nil
							break RetryWait
						}
						select {
						case _, ok := <-obj.state[vertex].resumeSignal: // channel closes
							if !ok {
								closed = true
							}
							// resumed!
						}
					}
				}
				timer.Stop() // it's nice to cleanup
				delay = 0    // reset
				obj.state[vertex].init.Logf("the CheckApply delay expired!")
			}
			// don't Process anymore if we've already failed or shutdown...
			if failed || closed {
				continue Loop
			}

			if obj.Debug {
				obj.Logf("Process(%s)", vertex)
			}
			backPoke := false
			err = obj.Process(obj.state[vertex].doneCtx, vertex)
			if err == engine.ErrBackPoke {
				backPoke = true
				err = nil // for future code safety
			}
			if obj.Debug && backPoke {
				obj.Logf("Process(%s): BackPoke!", vertex)
			}
			if obj.Debug && !backPoke {
				obj.Logf("Process(%s): Return(%s)", vertex, engineUtil.CleanError(err))
			}
			if err == nil && !backPoke && res.MetaParams().RetryReset { // reset it on success!
				metas.CheckApplyRetry = res.MetaParams().Retry // lookup the retry value
			}
			if err == nil || backPoke {
				break RetryLoop
			}
			// we've got an error...
			delay = res.MetaParams().Delay

			if metas.CheckApplyRetry < 0 { // infinite retries
				continue
			}
			if metas.CheckApplyRetry > 0 { // don't decrement past 0
				metas.CheckApplyRetry--
				obj.state[vertex].init.Logf(
					"retrying CheckApply after %.4f seconds (%d left)",
					float64(delay)/1000,
					metas.CheckApplyRetry,
				)
				continue
			}
			//if metas.CheckApplyRetry == 0 { // optional
			//	err = errwrap.Wrapf(err, "permanent process error")
			//}

			// It is important that we shutdown the Watch loop if
			// this dies. If Process fails permanently, we ask it
			// to exit right here... (It happens when we loop...)
			failed = true
			close(obj.state[vertex].processDone) // causes doneCtx to cancel
			reterr = errwrap.Append(reterr, err) // permanent failure
			continue

		} // retry loop

		// When this Process loop exits, it's because something has
		// caused Watch() to shutdown (even if it's our permanent
		// failure from Process), which caused this channel to close.
		// On or more exit signals are possible, and more than one can
		// happen simultaneously.

	} // process loop

	//return nil // unreachable
}
