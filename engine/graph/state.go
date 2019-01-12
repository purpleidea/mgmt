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
	"os"
	"path"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/event"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

// State stores some state about the resource it is mapped to.
type State struct {
	// Graph is a pointer to the graph that this vertex is part of.
	//Graph pgraph.Graph

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

	//Converger converger.Converger

	// Debug turns on additional output and behaviours.
	Debug bool

	// Logf is the logging function that should be used to display messages.
	Logf func(format string, v ...interface{})

	timestamp int64 // last updated timestamp
	isStateOK bool  // is state OK or do we need to run CheckApply ?

	// events is a channel of incoming events which is read by the Watch
	// loop for that resource. It receives events like pause, start, and
	// poke. The channel shuts down to signal for Watch to exit.
	eventsChan chan *event.Msg // incoming to resource
	eventsLock *sync.Mutex     // lock around sending and closing of events channel
	eventsDone bool            // is channel closed?

	// outputChan is the channel that the engine listens on for events from
	// the Watch loop for that resource. The event is nil normally, except
	// when events are sent on this channel from the engine. This only
	// happens as a signaling mechanism when Watch has shutdown and we want
	// to notify the Process loop which reads from this.
	outputChan chan error // outgoing from resource

	wg   *sync.WaitGroup
	exit *util.EasyExit

	started chan struct{} // closes when it's started
	stopped chan struct{} // closes when it's stopped

	starter bool // do we have an indegree of 0 ?
	working bool // is the Main() loop running ?

	cuid converger.UID // primary converger
	tuid converger.UID // secondary converger

	init *engine.Init // a copy of the init struct passed to res Init
}

// Init initializes structures like channels.
func (obj *State) Init() error {
	obj.eventsChan = make(chan *event.Msg)
	obj.eventsLock = &sync.Mutex{}

	obj.outputChan = make(chan error)

	obj.wg = &sync.WaitGroup{}
	obj.exit = util.NewEasyExit()

	obj.started = make(chan struct{})
	obj.stopped = make(chan struct{})

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

	//obj.cuid = obj.Converger.Register() // gets registered in Worker()
	//obj.tuid = obj.Converger.Register() // gets registered in Worker()

	obj.init = &engine.Init{
		Program:  obj.Program,
		Hostname: obj.Hostname,

		// Watch:
		Running: func() error {
			obj.tuid.StopTimer()
			close(obj.started)    // this is reset in the reset func
			obj.isStateOK = false // assume we're initially dirty
			// optimization: skip the initial send if not a starter
			// because we'll get poked from a starter soon anyways!
			if !obj.starter {
				return nil
			}
			return obj.event()
		},
		Event:  obj.event,
		Events: obj.eventsChan,
		Read:   obj.read,
		Dirty: func() { // TODO: should we rename this SetDirty?
			obj.tuid.StopTimer()
			obj.isStateOK = false
		},

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
	err := res.Close()
	if obj.Debug {
		obj.Logf("Close(%s): Return(%+v)", res, err)
	}

	return err
}

// reset is run to reset the state so that Watch can run a second time. Thus is
// needed for the Watch retry in particular.
func (obj *State) reset() {
	obj.started = make(chan struct{})
	obj.stopped = make(chan struct{})
}

// Poke sends a nil message on the outputChan. This channel is used by the
// resource to signal a possible change. This will cause the Process loop to
// run if it can.
func (obj *State) Poke() {
	// add a wait group on the vertex we're poking!
	obj.wg.Add(1)
	defer obj.wg.Done()

	// now that we've added to the wait group, obj.outputChan won't close...
	// so see if there's an exit signal before we release the wait group!
	// XXX: i don't think this is necessarily happening, but maybe it is?
	// XXX: re-write some of the engine to ensure that: "the sender closes"!
	select {
	case <-obj.exit.Signal():
		return // skip sending the poke b/c we're closing
	default:
	}

	select {
	case obj.outputChan <- nil:

	case <-obj.exit.Signal():
	}
}

// Event sends a Pause or Start event to the resource. It can also be used to
// send Poke events, but it's much more efficient to send them directly instead
// of passing them through the resource.
func (obj *State) Event(msg *event.Msg) {
	// TODO: should these happen after the lock?
	obj.wg.Add(1)
	defer obj.wg.Done()

	obj.eventsLock.Lock()
	defer obj.eventsLock.Unlock()

	if obj.eventsDone { // closing, skip events...
		return
	}

	if msg.Kind == event.KindExit { // set this so future events don't deadlock
		obj.Logf("exit event...")
		obj.eventsDone = true
		close(obj.eventsChan) // causes resource Watch loop to close
		obj.exit.Done(nil)    // trigger exit signal to unblock some cases
		return
	}

	select {
	case obj.eventsChan <- msg:

	case <-obj.exit.Signal():
	}
}

// read is a helper function used inside the main select statement of resources.
// If it returns an error, then this is a signal for the resource to exit.
func (obj *State) read(msg *event.Msg) error {
	switch msg.Kind {
	case event.KindPoke:
		return obj.event() // a poke needs to cause an event...
	case event.KindStart:
		return fmt.Errorf("unexpected start")
	case event.KindPause:
		// pass
	case event.KindExit:
		return engine.ErrSignalExit

	default:
		return fmt.Errorf("unhandled event: %+v", msg.Kind)
	}

	// we're paused now
	select {
	case msg, ok := <-obj.eventsChan:
		if !ok {
			return engine.ErrWatchExit
		}
		switch msg.Kind {
		case event.KindPoke:
			return fmt.Errorf("unexpected poke")
		case event.KindPause:
			return fmt.Errorf("unexpected pause")
		case event.KindStart:
			// resumed
			return nil
		case event.KindExit:
			return engine.ErrSignalExit

		default:
			return fmt.Errorf("unhandled event: %+v", msg.Kind)
		}
	}
}

// event is a helper function to send an event from the resource Watch loop. It
// can be used for the initial `running` event, or any regular event. If it
// returns an error, then the Watch loop must return this error and shutdown.
func (obj *State) event() error {
	// loop until we sent on obj.outputChan or exit with error
	for {
		select {
		// send "activity" event
		case obj.outputChan <- nil:
			return nil // sent event!

		// make sure to keep handling incoming
		case msg, ok := <-obj.eventsChan:
			if !ok {
				return engine.ErrWatchExit
			}
			switch msg.Kind {
			case event.KindPoke:
				// we're trying to send an event, so swallow the
				// poke: it's what we wanted to have happen here
				continue
			case event.KindStart:
				return fmt.Errorf("unexpected start")
			case event.KindPause:
				// pass
			case event.KindExit:
				return engine.ErrSignalExit

			default:
				return fmt.Errorf("unhandled event: %+v", msg.Kind)
			}
		}

		// we're paused now
		select {
		case msg, ok := <-obj.eventsChan:
			if !ok {
				return engine.ErrWatchExit
			}
			switch msg.Kind {
			case event.KindPoke:
				return fmt.Errorf("unexpected poke")
			case event.KindPause:
				return fmt.Errorf("unexpected pause")
			case event.KindStart:
				// resumed
			case event.KindExit:
				return engine.ErrSignalExit

			default:
				return fmt.Errorf("unhandled event: %+v", msg.Kind)
			}
		}
	}
}

// varDir returns the path to a working directory for the resource. It will try
// and create the directory first, and return an error if this failed. The dir
// should be cleaned up by the resource on Close if it wishes to discard the
// contents. If it does not, then a future resource with the same kind and name
// may see those contents in that directory. The resource should clean up the
// contents before use if it is important that nothing exist. It is always
// possible that contents could remain after an abrupt crash, so do not store
// overly sensitive data unless you're aware of the risks.
func (obj *State) varDir(extra string) (string, error) {
	// Using extra adds additional dirs onto our namespace. An empty extra
	// adds no additional directories.
	if obj.Prefix == "" { // safety
		return "", fmt.Errorf("the VarDir prefix is empty")
	}

	// an empty string at the end has no effect
	p := fmt.Sprintf("%s/", path.Join(obj.Prefix, extra))
	if err := os.MkdirAll(p, 0770); err != nil {
		return "", errwrap.Wrapf(err, "can't create prefix in: %s", p)
	}

	// returns with a trailing slash as per the mgmt file res convention
	return p, nil
}

// poll is a replacement for Watch when the Poll metaparameter is used.
func (obj *State) poll(interval uint32) error {
	// create a time.Ticker for the given interval
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// notify engine that we're running
	if err := obj.init.Running(); err != nil {
		return err // exit if requested
	}

	var send = false // send event?
	for {
		select {
		case <-ticker.C: // received the timer event
			obj.init.Logf("polling...")
			send = true
			obj.init.Dirty() // dirty

		case event, ok := <-obj.init.Events:
			if !ok {
				return nil
			}
			if err := obj.init.Read(event); err != nil {
				return err
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			if err := obj.init.Event(); err != nil {
				return err // exit if requested
			}
		}
	}
}
