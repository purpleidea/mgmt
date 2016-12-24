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

	errwrap "github.com/pkg/errors"
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

// Poke notifies nodes after me in the dependency graph that they need refreshing...
// NOTE: this assumes that this can never fail or need to be rescheduled
func (g *Graph) Poke(v *Vertex, activity bool) error {
	var wg sync.WaitGroup
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphVertices(v) {
		// XXX: if we're in state event and haven't been cancelled by
		// apply, then we can cancel a poke to a child, right? XXX
		// XXX: if n.Res.getState() != resources.ResStateEvent || activity { // is this correct?
		if true || activity { // XXX: ???
			if g.Flags.Debug {
				log.Printf("%s[%s]: Poke: %s[%s]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			wg.Add(1)
			go func(nn *Vertex) error {
				defer wg.Done()
				edge := g.Adjacency[v][nn] // lookup
				notify := edge.Notify && edge.Refresh()

				// FIXME: is it okay that this is sync?
				nn.SendEvent(event.EventPoke, true, notify)
				// TODO: check return value?
				return nil // never error for now...
			}(n)

		} else {
			if g.Flags.Debug {
				log.Printf("%s[%s]: Poke: %s[%s]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
	wg.Wait() // wait for all the pokes to complete
	return nil
}

// BackPoke pokes the pre-requisites that are stale and need to run before I can run.
func (g *Graph) BackPoke(v *Vertex) {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphVertices(v) {
		x, y, s := v.GetTimestamp(), n.GetTimestamp(), n.Res.GetState()
		// if the parent timestamp needs poking AND it's not in state
		// ResStateEvent, then poke it. If the parent is in ResStateEvent it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != resources.ResStateEvent && s != resources.ResStateCheckApply) {
			if g.Flags.Debug {
				log.Printf("%s[%s]: BackPoke: %s[%s]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			// FIXME: is it okay that this is sync?
			n.SendEvent(event.EventBackPoke, true, false)
		} else {
			if g.Flags.Debug {
				log.Printf("%s[%s]: BackPoke: %s[%s]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
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
	obj.SetState(resources.ResStateEvent)
	var ok = true
	var applied = false // did we run an apply?
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if g.OKTimestamp(v) {
		if g.Flags.Debug {
			log.Printf("%s[%s]: OKTimestamp(%v)", obj.Kind(), obj.GetName(), v.GetTimestamp())
		}

		obj.SetState(resources.ResStateCheckApply)

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
			// if the CheckApply run takes longer than the converged
			// timeout, we could inappropriately converge mid-apply!
			// avoid this by blocking convergence with a fake report
			block := obj.Converger().Register() // get an extra cuid
			block.SetConverged(false)           // block while CheckApply runs!

			// if this fails, don't UpdateTimestamp()
			checkOK, err = obj.CheckApply(!noop)

			block.SetConverged(true) // unblock
			block.Unregister()

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
			if err := g.Poke(v, activity); err != nil {
				return errwrap.Wrapf(err, "the Poke() failed")
			}
		}
		// poke at our pre-req's instead since they need to refresh/run...
		return errwrap.Wrapf(err, "could not Process() successfully")
	}
	// else... only poke at the pre-req's that need to run
	go g.BackPoke(v)
	return nil
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
	// TODO: is there a better system for the `Watching` flag?
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	processChan := make(chan event.Event)
	go func() {
		running := false
		var timer = time.NewTimer(time.Duration(math.MaxInt64)) // longest duration
		if !timer.Stop() {
			<-timer.C // unnecessary, shouldn't happen
		}
		var delay = time.Duration(v.Meta().Delay) * time.Millisecond
		var retry = v.Meta().Retry // number of tries left, -1 for infinite
		var saved event.Event
	Loop:
		for {
			// this has to be synchronous, because otherwise the Res
			// event loop will keep running and change state,
			// causing the converged timeout to fire!
			select {
			case event, ok := <-processChan: // must use like this
				if running && ok {
					// we got an event that wasn't a close,
					// while we were waiting for the timer!
					// if this happens, it might be a bug:(
					log.Fatalf("%s[%s]: Worker: Unexpected event: %+v", v.Kind(), v.GetName(), event)
				}
				if !ok { // processChan closed, let's exit
					break Loop // no event, so no ack!
				}

				// the above mentioned synchronous part, is the
				// running of this function, paired with an ack.
				if e := g.Process(v); e != nil {
					saved = event
					log.Printf("%s[%s]: CheckApply errored: %v", v.Kind(), v.GetName(), e)
					if retry == 0 {
						// wrap the error in the sentinel
						event.ACKNACK(&SentinelErr{e}) // fail the Watch()
						break Loop
					}
					if retry > 0 { // don't decrement the -1
						retry--
					}
					log.Printf("%s[%s]: CheckApply: Retrying after %.4f seconds (%d left)", v.Kind(), v.GetName(), delay.Seconds(), retry)
					// start the timer...
					timer.Reset(delay)
					running = true
					continue
				}
				retry = v.Meta().Retry // reset on success
				event.ACK()            // sync

			case <-timer.C:
				if !timer.Stop() {
					//<-timer.C // blocks, docs are wrong!
				}
				running = false
				log.Printf("%s[%s]: CheckApply delay expired!", v.Kind(), v.GetName())
				// re-send this failed event, to trigger a CheckApply()
				go func() { processChan <- saved }()
				// TODO: should we send a fake event instead?
				//saved = nil
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
					if exit, send := obj.ReadEvent(&event); exit {
						return nil // exit
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
		if v.Meta().Poll > 0 { // poll instead of watching :(
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
	close(processChan)
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
		if (!first) || indegree[v] == 0 {
			v.Res.Starter(true) // let the startup code know to poke
		}

		if !v.Res.IsWatching() { // if Watch() is not running...
			g.wg.Add(1)
			// must pass in value to avoid races...
			// see: https://ttboj.wordpress.com/2015/07/27/golang-parallelism-issues-causing-too-many-open-files-error/
			go func(vv *Vertex) {
				defer g.wg.Done()
				// TODO: if a sufficient number of workers error,
				// should something be done? Will these restart
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
			v.Res.SendEvent(event.EventStart, true, false) // sync!
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
		v.SendEvent(event.EventPause, true, false)
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

		v.SendEvent(event.EventExit, true, false)
	}
	g.wg.Wait() // for now, this doesn't need to be a separate Wait() method
}
