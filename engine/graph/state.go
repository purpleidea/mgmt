// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// State stores some state about the resource it is mapped to.
type State struct {
	// Graph is a pointer to the graph that this vertex is part of.
	Graph *pgraph.Graph

	// Vertex is the pointer in the graph that this state corresponds to. It
	// can be converted to a `Res` if necessary.
	// TODO: should this be passed in on Init instead?
	Vertex pgraph.Vertex

	Program  string
	Hostname string
	World    engine.World

	// Prefix is a unique directory prefix which can be used. It should be
	// created if needed.
	Prefix string

	//Converger *converger.Coordinator

	// Debug turns on additional output and behaviours.
	Debug bool

	// Logf is the logging function that should be used to display messages.
	Logf func(format string, v ...interface{})

	timestamp int64 // last updated timestamp
	isStateOK bool  // is state OK or do we need to run CheckApply ?
	workerErr error // did the Worker error?

	// doneChan closes when Watch should shut down. When any of the
	// following channels close, it causes this to close.
	doneChan chan struct{}

	// processDone is closed when the Process/CheckApply function fails
	// permanently, and wants to cause Watch to exit.
	processDone chan struct{}
	// watchDone is closed when the Watch function fails permanently, and we
	// close this to signal we should definitely exit. (Often redundant.)
	watchDone chan struct{} // could be shared with limitDone
	// limitDone is closed when the Watch function fails permanently, and we
	// close this to signal we should definitely exit. This happens inside
	// of the limit loop of the Process section of Worker.
	limitDone chan struct{} // could be shared with watchDone
	// removeDone is closed when the vertexRemoveFn method asks for an exit.
	// This happens when we're switching graphs. The switch to an "empty" is
	// the equivalent of asking for a final shutdown.
	removeDone chan struct{}
	// eventsDone is closed when we shutdown the Process loop because we
	// closed without error. In theory this shouldn't happen, but it could
	// if Watch returns without error for some reason.
	eventsDone chan struct{}

	// eventsChan is the channel that the engine listens on for events from
	// the Watch loop for that resource. The event is nil normally, except
	// when events are sent on this channel from the engine. This only
	// happens as a signaling mechanism when Watch has shutdown and we want
	// to notify the Process loop which reads from this.
	eventsChan chan error // outgoing from resource

	// pokeChan is a separate channel that the Process loop listens on to
	// know when we might need to run Process. It never closes, and is safe
	// to send on since it is buffered.
	pokeChan chan struct{} // outgoing from resource

	// paused represents if this particular res is paused or not.
	paused bool
	// pauseSignal closes to request a pause of this resource.
	pauseSignal chan struct{}
	// resumeSignal closes to request a resume of this resource.
	resumeSignal chan struct{}
	// pausedAck is used to send an ack message saying that we've paused.
	pausedAck *util.EasyAck

	wg *sync.WaitGroup // used for all vertex specific processes

	cuid *converger.UID // primary converger
	tuid *converger.UID // secondary converger

	init *engine.Init // a copy of the init struct passed to res Init
}

// Init initializes structures like channels.
func (obj *State) Init() error {
	res, isRes := obj.Vertex.(engine.Res)
	if !isRes {
		return fmt.Errorf("vertex is not a Res")
	}
	if obj.Hostname == "" {
		return fmt.Errorf("the Hostname is empty")
	}
	if obj.Prefix == "" {
		return fmt.Errorf("the Prefix is empty")
	}
	if obj.Prefix == "/" {
		return fmt.Errorf("the Prefix is root")
	}
	if obj.Logf == nil {
		return fmt.Errorf("the Logf function is missing")
	}

	obj.doneChan = make(chan struct{})

	obj.processDone = make(chan struct{})
	obj.watchDone = make(chan struct{})
	obj.limitDone = make(chan struct{})
	obj.removeDone = make(chan struct{})
	obj.eventsDone = make(chan struct{})

	obj.eventsChan = make(chan error)

	obj.pokeChan = make(chan struct{}, 1) // must be buffered

	//obj.paused = false // starts off as started
	obj.pauseSignal = make(chan struct{})
	//obj.resumeSignal = make(chan struct{}) // happens on pause
	//obj.pausedAck = util.NewEasyAck() // happens on pause

	obj.wg = &sync.WaitGroup{}

	//obj.cuid = obj.Converger.Register() // gets registered in Worker()
	//obj.tuid = obj.Converger.Register() // gets registered in Worker()

	obj.init = &engine.Init{
		Program:  obj.Program,
		Hostname: obj.Hostname,

		// Watch:
		Running: obj.event,
		Event:   obj.event,
		Done:    obj.doneChan,

		// CheckApply:
		Refresh: func() bool {
			res, ok := obj.Vertex.(engine.RefreshableRes)
			if !ok {
				panic("res does not support the Refreshable trait")
			}
			return res.Refresh()
		},
		Send: func(st interface{}) error {
			res, ok := obj.Vertex.(engine.SendableRes)
			if !ok {
				panic("res does not support the Sendable trait")
			}
			// XXX: type check this
			//expected := res.Sends()
			//if err := XXX_TYPE_CHECK(expected, st); err != nil {
			//	return err
			//}

			return res.Send(st) // send the struct
		},
		Recv: func() map[string]*engine.Send { // TODO: change this API?
			res, ok := obj.Vertex.(engine.RecvableRes)
			if !ok {
				panic("res does not support the Recvable trait")
			}
			return res.Recv()
		},

		// FIXME: pass in a safe, limited query func instead?
		// TODO: not implemented, use FilteredGraph
		//Graph: func() *pgraph.Graph {
		//	_, ok := obj.Vertex.(engine.CanGraphQueryRes)
		//	if !ok {
		//		panic("res does not support the GraphQuery trait")
		//	}
		//	return obj.Graph // we return in a func so it's fresh!
		//},

		FilteredGraph: func() (*pgraph.Graph, error) {
			graph, err := pgraph.NewGraph("filtered")
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not create graph")
			}

			// filter graph and build a new one...
			adjacency := obj.Graph.Adjacency()
			for v1 := range adjacency {
				// check we're allowed
				r1, ok := v1.(engine.GraphQueryableRes)
				if !ok {
					continue
				}
				// pass in information on requestor...
				if err := r1.GraphQueryAllowed(
					engine.GraphQueryableOptionKind(res.Kind()),
					engine.GraphQueryableOptionName(res.Name()),
					// TODO: add more information...
				); err != nil {
					continue
				}
				graph.AddVertex(v1)

				for v2, edge := range adjacency[v1] {
					r2, ok := v2.(engine.GraphQueryableRes)
					if !ok {
						continue
					}
					// pass in information on requestor...
					if err := r2.GraphQueryAllowed(
						engine.GraphQueryableOptionKind(res.Kind()),
						engine.GraphQueryableOptionName(res.Name()),
						// TODO: add more information...
					); err != nil {
						continue
					}
					//graph.AddVertex(v2) // redundant
					graph.AddEdge(v1, v2, edge)
				}
			}

			return graph, nil // we return in a func so it's fresh!
		},

		World:  obj.World,
		VarDir: obj.varDir,

		Debug: obj.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.Logf("resource: "+format, v...)
		},
	}

	// run the init
	if obj.Debug {
		obj.Logf("Init(%s)", res)
	}

	// write the reverse request to the disk...
	if err := obj.ReversalInit(); err != nil {
		return err // TODO: test this code path...
	}

	err := res.Init(obj.init)
	if obj.Debug {
		obj.Logf("Init(%s): Return(%+v)", res, err)
	}
	if err != nil {
		return errwrap.Wrapf(err, "could not Init() resource")
	}

	return nil
}

// Close shuts down and performs any cleanup. This is most akin to a "post" or
// cleanup command as the initiator for closing a vertex happens in graph sync.
func (obj *State) Close() error {
	res, isRes := obj.Vertex.(engine.Res)
	if !isRes {
		return fmt.Errorf("vertex is not a Res")
	}

	//if obj.cuid != nil {
	//	obj.cuid.Unregister() // gets unregistered in Worker()
	//}
	//if obj.tuid != nil {
	//	obj.tuid.Unregister() // gets unregistered in Worker()
	//}

	// redundant safety
	obj.wg.Wait() // wait until all poke's and events on me have exited

	// run the close
	if obj.Debug {
		obj.Logf("Close(%s)", res)
	}

	var reverr error
	// clear the reverse request from the disk...
	if err := obj.ReversalClose(); err != nil {
		// TODO: test this code path...
		// TODO: should this be an error or a warning?
		reverr = err
	}

	reterr := res.Close()
	if obj.Debug {
		obj.Logf("Close(%s): Return(%+v)", res, reterr)
	}

	reterr = errwrap.Append(reterr, reverr)

	return reterr
}

// Poke sends a notification on the poke channel. This channel is used to notify
// the Worker to run the Process/CheckApply when it can. This is used when there
// is a need to schedule or reschedule some work which got postponed or dropped.
// This doesn't contain any internal synchronization primitives or wait groups,
// callers are expected to make sure that they don't leave any of these running
// by the time the Worker() shuts down.
func (obj *State) Poke() {
	// redundant
	//if len(obj.pokeChan) > 0 {
	//	return
	//}

	select {
	case obj.pokeChan <- struct{}{}:
	default: // if chan is now full because more than one poke happened...
	}
}

// Pause pauses this resource. It should not be called on any already paused
// resource. It will block until the resource pauses with an acknowledgment, or
// until an exit for that resource is seen. If the latter happens it will error.
// It is NOT thread-safe with the Resume() method so only call either one at a
// time.
func (obj *State) Pause() error {
	if obj.paused {
		return fmt.Errorf("already paused")
	}

	obj.pausedAck = util.NewEasyAck()
	obj.resumeSignal = make(chan struct{}) // build the resume signal
	close(obj.pauseSignal)
	obj.Poke() // unblock and notice the pause if necessary

	// wait for ack (or exit signal)
	select {
	case <-obj.pausedAck.Wait(): // we got it!
		// we're paused
	case <-obj.doneChan:
		return engine.ErrClosed
	}
	obj.paused = true

	return nil
}

// Resume unpauses this resource. It can be safely called on a brand-new
// resource that has just started running without incident. It is NOT
// thread-safe with the Pause() method, so only call either one at a time.
func (obj *State) Resume() {
	// TODO: do we need a mutex around Resume?
	if !obj.paused { // no need to unpause brand-new resources
		return
	}

	obj.pauseSignal = make(chan struct{}) // rebuild for next pause
	close(obj.resumeSignal)
	//obj.Poke() // not needed, we're already waiting for resume

	obj.paused = false

	// no need to wait for it to resume
	//return // implied
}

// event is a helper function to send an event to the CheckApply process loop.
// It can be used for the initial `running` event, or any regular event. You
// should instead use Poke() to "schedule" a new Process/CheckApply loop when
// one might be needed. This method will block until we're unpaused and ready to
// receive on the events channel.
func (obj *State) event() {
	obj.setDirty() // assume we're initially dirty

	select {
	case obj.eventsChan <- nil:
		// send!
	}

	//return // implied
}

// setDirty marks the resource state as dirty. This signals to the engine that
// CheckApply will have some work to do in order to converge it.
func (obj *State) setDirty() {
	obj.tuid.StopTimer()
	obj.isStateOK = false
}

// poll is a replacement for Watch when the Poll metaparameter is used.
func (obj *State) poll(interval uint32) error {
	// create a time.Ticker for the given interval
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case <-ticker.C: // received the timer event
			obj.init.Logf("polling...")

		case <-obj.init.Done: // signal for shutdown request
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}
