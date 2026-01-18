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
	obj.tlock.RLock()
	state := obj.state[vertex]
	obj.tlock.RUnlock()

	vs := []pgraph.Vertex{}
	state.mutex.RLock()   // concurrent read start
	ts := state.timestamp // race
	state.mutex.RUnlock() // concurrent read end
	// these are all the vertices pointing TO vertex, eg: ??? -> vertex
	for _, v := range obj.graph.IncomingGraphVertices(vertex) {
		obj.tlock.RLock()
		state := obj.state[v]
		obj.tlock.RUnlock()

		// If the vertex has a greater timestamp than any prerequisite,
		// then we can't run right now. If they're equal (eg: initially
		// with a value of 0) then we also can't run because we should
		// let our pre-requisites go first.
		state.mutex.RLock()   // concurrent read start
		t := state.timestamp  // race
		state.mutex.RUnlock() // concurrent read end
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

	obj.tlock.RLock()
	state := obj.state[vertex]
	obj.tlock.RUnlock()

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

	// XXX: This code is duplicated in the fancier autogrouping code below!
	//if res, ok := vertex.(engine.RecvableRes); ok {
	//	if obj.Debug {
	//		obj.Logf("SendRecv: %s", res) // receiving here
	//	}
	//	if updated, err := SendRecv(res, nil); err != nil {
	//		return errwrap.Wrapf(err, "could not SendRecv")
	//	} else if len(updated) > 0 {
	//		//for _, s := range graph.UpdatedStrings(updated) {
	//		//	obj.Logf("SendRecv: %s", s)
	//		//}
	//		for r, m := range updated { // map[engine.RecvableRes]map[string]*engine.Send
	//			v, ok := r.(pgraph.Vertex)
	//			if !ok {
	//				continue
	//			}
	//			_, stateExists := obj.state[v] // autogrouped children probably don't have a state
	//			if !stateExists {
	//				continue
	//			}
	//			for s, send := range m {
	//				if !send.Changed {
	//					continue
	//				}
	//				obj.Logf("Send/Recv: %v.%s -> %v.%s", send.Res, send.Key, r, s)
	//				// if send.Changed == true, at least one was updated
	//				// invalidate cache, mark as dirty
	//				obj.state[v].setDirty()
	//				//break // we might have more vertices now
	//			}
	//
	//			// re-validate after we change any values
	//			if err := engine.Validate(r); err != nil {
	//				return errwrap.Wrapf(err, "failed Validate after SendRecv")
	//			}
	//		}
	//	}
	//}

	// Send/Recv *can* receive from someone that was grouped! The sender has
	// to use *their* send/recv handle/implementation, which has to be setup
	// properly by the parent resource during Init(). See: http:server:flag.
	collectSendRecv := []engine.Res{} // found resources

	if res, ok := vertex.(engine.RecvableRes); ok {
		collectSendRecv = append(collectSendRecv, res)
	}

	// If we contain grouped resources, maybe someone inside wants to recv?
	// This code is similar to the above and was added for http:server:ui.
	// XXX: Maybe this block isn't needed, as mentioned we need to check!
	if res, ok := vertex.(engine.GroupableRes); ok {
		process := res.GetGroup() // look through these
		for len(process) > 0 {    // recurse through any nesting
			var x engine.GroupableRes
			x, process = process[0], process[1:] // pop from front!

			for _, g := range x.GetGroup() {
				collectSendRecv = append(collectSendRecv, g.(engine.Res))
			}
		}
	}

	//for _, g := res.GetGroup() // non-recursive, one-layer method
	for _, g := range collectSendRecv { // recursive method!
		r, ok := g.(engine.RecvableRes)
		if !ok {
			continue
		}

		// This section looks almost identical to the above one!
		if updated, err := SendRecv(r, nil); err != nil {
			return errwrap.Wrapf(err, "could not grouped SendRecv")
		} else if len(updated) > 0 {
			//for _, s := range graph.UpdatedStrings(updated) {
			//	obj.Logf("SendRecv: %s", s)
			//}
			for r, m := range updated { // map[engine.RecvableRes]map[string]*engine.Send
				v, ok := r.(pgraph.Vertex)
				if !ok {
					continue
				}
				_, stateExists := obj.state[v] // autogrouped children probably don't have a state
				if !stateExists {
					continue
				}
				for s, send := range m {
					if !send.Changed {
						continue
					}
					obj.Logf("Send/Recv: %v.%s -> %v.%s", send.Res, send.Key, r, s)
					// if send.Changed == true, at least one was updated
					// invalidate cache, mark as dirty
					obj.state[v].setDirty()
					//break // we might have more vertices now
				}

				// re-validate after we change any values
				if err := engine.Validate(r); err != nil {
					return errwrap.Wrapf(err, "failed grouped Validate after SendRecv")
				}
			}
		}
	}
	// XXX: this might not work with two merged "CompatibleRes" resources...
	// XXX: fix that so we can have the mappings to do it in lang/interpret.go ?

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

	// Run the exported resource exporter!
	var exportOK bool
	var exportErr error
	wg := &sync.WaitGroup{}
	wg.Add(1)
	// (Run this concurrently with the CheckApply related stuff below...)
	go func() {
		defer wg.Done()
		// doesn't really need to be in parallel, but we can...
		exportOK, exportErr = obj.Exporter.Export(ctx, res)
	}()

	// Check cached state, to skip CheckApply, but can't skip if refreshing!
	// If the resource doesn't implement refresh, skip the refresh test.
	// FIXME: if desired, check that we pass through refresh notifications!
	if (!refresh || !isRefreshableRes) && state.isStateOK.Load() { // mutex RLock/RUnlock
		checkOK, err = true, nil

	} else if noop && (refresh && isRefreshableRes) { // had a refresh to do w/ noop!
		checkOK, err = false, nil // therefore the state is wrong

	} else if res.MetaParams().Hidden {
		// We're not running CheckApply
		if obj.Debug {
			obj.Logf("%s: Hidden", res)
		}
		checkOK, err = true, nil // default

	} else {
		// run the CheckApply!
		if obj.Debug {
			obj.Logf("%s: CheckApply(%t)", res, !noop)
		}
		// if this fails, don't UpdateTimestamp()
		checkOK, err = res.CheckApply(ctx, !noop)
		if !checkOK && obj.Debug { // don't log on (checkOK == true)
			obj.Logf("%s: CheckApply(%t): Return(%t, %s)", res, !noop, checkOK, engineUtil.CleanError(err))
		}
	}
	wg.Wait()
	checkOK = checkOK && exportOK // always combine
	if err == nil {               // If CheckApply didn't error, look at exportOK.
		// This is because if CheckApply errors we don't need to care or
		// tell anyone about an exporting error.
		err = exportErr
	}

	if checkOK && err != nil { // should never return this way
		return fmt.Errorf("%s: resource programming error: CheckApply(%t): %t, %+v", res, !noop, checkOK, err)
	}

	if !checkOK { // something changed, restart timer
		state.cuid.ResetTimer() // activity!
		if obj.Debug {
			obj.Logf("%s: converger: reset timer", res)
		}
	}

	// if CheckApply ran without noop and without error, state should be good
	if !noop && err == nil { // aka !noop || checkOK
		state.tuid.StartTimer()
		//state.mutex.Lock()
		state.isStateOK.Store(true) // reset
		//state.mutex.Unlock()
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
		state.mutex.Lock()                      // concurrent write start
		state.timestamp = time.Now().UnixNano() // update timestamp (race)
		state.mutex.Unlock()                    // concurrent write end
		for _, v := range obj.graph.OutgoingGraphVertices(vertex) {
			if !obj.OKTimestamp(v) {
				// there is at least another one that will poke this...
				continue
			}

			// If we're pausing (or exiting) then we can skip poking
			// so that the graph doesn't go on running forever until
			// it's completely done. This is an optional feature and
			// we can select it via ^C on user exit or via the GAPI.
			if obj.fastPause.Load() {
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

	obj.tlock.RLock()
	state := obj.state[vertex]
	obj.tlock.RUnlock()

	// bonus safety check
	if res.MetaParams().Burst == 0 && !(res.MetaParams().Limit == rate.Inf) { // blocked
		return fmt.Errorf("permanently limited (rate != Inf, burst = 0)")
	}

	// initialize or reinitialize the meta state for this resource uid
	// if we're using a Hidden resource, we don't support this feature
	// TODO: should we consider supporting it? is it really necessary?
	// XXX: to support this for Hidden, we'd need to handle dupe names
	metas := &engine.MetaState{
		CheckApplyRetry: res.MetaParams().Retry, // lookup the retry value
	}
	if !res.MetaParams().Hidden {
		// Skip this if Hidden since we can have a hidden res that has
		// the same kind+name as a regular res, and this would conflict.
		obj.mlock.Lock()
		if _, exists := obj.metas[engine.PtrUID(res)]; !exists || res.MetaParams().Reset {
			obj.metas[engine.PtrUID(res)] = &engine.MetaState{
				CheckApplyRetry: res.MetaParams().Retry, // lookup the retry value
			}
		}
		metas = obj.metas[engine.PtrUID(res)] // handle
		obj.mlock.Unlock()
	}

	//defer close(state.stopped) // done signal

	state.cuid = obj.Converger.Register()
	state.tuid = obj.Converger.Register()
	// must wait for all users of the cuid to finish *before* we unregister!
	// as a result, this defer happens *before* the below wait group Wait...
	defer state.cuid.Unregister()
	defer state.tuid.Unregister()

	defer state.wg.Wait() // this Worker is the last to exit!

	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		defer close(state.eventsChan) // we close this on behalf of res

		// This is a close reverse-multiplexer. If any of the channels
		// close, then it will cause the doneCtx to cancel. That way,
		// multiple different folks can send a close signal, without
		// every worrying about duplicate channel close panics.
		state.wg.Add(1)
		go func() {
			defer state.wg.Done()

			// reverse-multiplexer: any close, causes *the* close!
			select {
			case <-state.processDone:
			case <-state.watchDone:
			case <-state.limitDone:
			case <-state.retryDone:
			case <-state.removeDone:
			case <-state.eventsDone:
			}

			// the main "done" signal gets activated here!
			state.doneCtxCancel() // cancels doneCtx
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
					defer state.init.Logf("the Watch delay expired!")
					defer timer.Stop() // it's nice to cleanup
					for {
						select {
						case <-timer.C: // the wait is over
							return errDelayExpired // special

						case <-state.doneCtx.Done():
							return nil
						}
					}
				}()
				if err == errDelayExpired {
					delay = 0 // reset
					continue
				}

			} else if res.MetaParams().Hidden {
				// We're not running Watch
				if obj.Debug {
					obj.Logf("%s: Hidden", res)
				}
				state.cuid.StartTimer() // TODO: Should we do this?
				err = state.hidden(state.doneCtx)
				state.cuid.StopTimer() // TODO: Should we do this?

			} else if interval := res.MetaParams().Poll; interval > 0 { // poll instead of watching :(
				state.cuid.StartTimer()
				err = state.poll(state.doneCtx, interval)
				state.cuid.StopTimer() // clean up nicely

			} else {
				state.cuid.StartTimer()
				if obj.Debug {
					obj.Logf("%s: Watch...", vertex)
				}
				err = res.Watch(state.doneCtx)       // run the watch normally
				err = errwrap.NoContextCanceled(err) // strip
				if s := engineUtil.CleanError(err); err != nil {
					obj.Logf("%s: Watch Error: %s", vertex, s)
				} else if obj.Debug {
					obj.Logf("%s: Watch Exited...", vertex)
				}
				state.cuid.StopTimer() // clean up nicely
			}
			if err == nil { // || err == engine.ErrClosed
				return // exited cleanly, we're done
			}
			if err == context.Canceled {
				return // we shutdown nicely on request
			}
			// we've got an error...
			delay = res.MetaParams().Delay

			if retry < 0 { // infinite retries
				continue
			}
			if retry > 0 { // don't decrement past 0
				retry--
				state.init.Logf("retrying Watch after %.4f seconds (%d left)", float64(delay)/1000, retry)
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
		case state.eventsChan <- errwrap.Wrapf(err, "watch failed"):
			// send
		}
	}()

	// If this exits cleanly, we must unblock the reverse-multiplexer.
	// I think this additional close is unnecessary, but it's not harmful.
	defer close(state.eventsDone) // causes doneCtx to cancel
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
		case err, ok := <-state.eventsChan: // read from watch channel
			if !ok {
				return reterr // we only return when chan closes
			}
			// If the Watch method exits with an error, then this
			// channel will get that error propagated to it, which
			// we then save so we can return it to the caller of us.
			if err != nil {
				failed = true
				close(state.watchDone)               // causes doneCtx to cancel
				reterr = errwrap.Append(reterr, err) // permanent failure
				continue
			}
			if obj.Debug {
				obj.Logf("event received")
			}
			reserv = limiter.ReserveN(time.Now(), 1) // one event
			// reserv.OK() seems to always be true here!

		case _, ok := <-state.pokeChan: // read from buffered poke channel
			if !ok { // we never close it
				panic("unexpected close of poke channel")
			}
			if obj.Debug {
				obj.Logf("poke received")
			}
			reserv = nil // we didn't receive a real event here...

		case _, ok := <-state.pauseSignal: // one message
			if !ok {
				state.pauseSignal = nil
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
			case _, ok := <-state.resumeSignal: // channel closes
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
		for len(state.pokeChan) > 0 {
			select {
			case <-state.pokeChan:
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
			state.init.Logf("limited (rate: %v/sec, burst: %d, next: %dms)", res.MetaParams().Limit, res.MetaParams().Burst, d/time.Millisecond)
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
				case e, ok := <-state.eventsChan: // read from watch channel
					if !ok {
						return reterr // we only return when chan closes
					}
					if e != nil {
						failed = true
						close(state.limitDone)             // causes doneCtx to cancel
						reterr = errwrap.Append(reterr, e) // permanent failure
						break LimitWait
					}
					if obj.Debug {
						obj.Logf("event received in limit")
					}
					// TODO: does this get added in properly?
					limiter.ReserveN(time.Now(), 1) // one event

				// this pause/resume block is the same as the upper main one
				case _, ok := <-state.pauseSignal:
					if !ok {
						state.pauseSignal = nil
						break LimitWait
					}
					select {
					case _, ok := <-state.resumeSignal: // channel closes
						if !ok {
							closed = true
						}
						// resumed!
					}
				}
			}
			timer.Stop() // it's nice to cleanup
			state.init.Logf("rate limiting expired!")
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
					case e, ok := <-state.eventsChan: // read from watch channel
						if !ok {
							return reterr // we only return when chan closes
						}
						if e != nil {
							failed = true
							close(state.retryDone)             // causes doneCtx to cancel
							reterr = errwrap.Append(reterr, e) // permanent failure
							break RetryWait
						}
						if obj.Debug {
							obj.Logf("event received in retry")
						}
						// TODO: does this get added in properly?
						limiter.ReserveN(time.Now(), 1) // one event

					// this pause/resume block is the same as the upper main one
					case _, ok := <-state.pauseSignal:
						if !ok {
							state.pauseSignal = nil
							break RetryWait
						}
						select {
						case _, ok := <-state.resumeSignal: // channel closes
							if !ok {
								closed = true
							}
							// resumed!
						}
					}
				}
				timer.Stop() // it's nice to cleanup
				delay = 0    // reset
				state.init.Logf("the CheckApply delay expired!")
			}
			// don't Process anymore if we've already failed or shutdown...
			if failed || closed {
				continue Loop
			}

			if obj.Debug {
				obj.Logf("Process(%s)", vertex)
			}
			backPoke := false
			err = obj.Process(state.doneCtx, vertex)
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
				state.init.Logf(
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
			close(state.processDone)             // causes doneCtx to cancel
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
