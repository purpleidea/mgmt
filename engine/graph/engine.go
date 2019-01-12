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

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/event"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/semaphore"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// Engine encapsulates a generic graph and manages its operations.
type Engine struct {
	Program  string
	Hostname string
	World    engine.World

	// Prefix is a unique directory prefix which can be used. It should be
	// created if needed.
	Prefix    string
	Converger converger.Converger

	Debug bool
	Logf  func(format string, v ...interface{})

	graph     *pgraph.Graph
	nextGraph *pgraph.Graph
	state     map[pgraph.Vertex]*State
	waits     map[pgraph.Vertex]*sync.WaitGroup

	slock *sync.Mutex // semaphore lock
	semas map[string]*semaphore.Semaphore

	wg *sync.WaitGroup

	fastPause bool
}

// Init initializes the internal structures and starts this the graph running.
// If the struct does not validate, or it cannot initialize, then this errors.
// Initially it will contain an empty graph.
func (obj *Engine) Init() error {
	var err error
	if obj.graph, err = pgraph.NewGraph("graph"); err != nil {
		return err
	}

	if obj.Prefix == "" || obj.Prefix == "/" {
		return fmt.Errorf("the prefix of `%s` is invalid", obj.Prefix)
	}
	if err := os.MkdirAll(obj.Prefix, 0770); err != nil {
		return errwrap.Wrapf(err, "can't create prefix")
	}

	obj.state = make(map[pgraph.Vertex]*State)
	obj.waits = make(map[pgraph.Vertex]*sync.WaitGroup)

	obj.slock = &sync.Mutex{}
	obj.semas = make(map[string]*semaphore.Semaphore)

	obj.wg = &sync.WaitGroup{}

	return nil
}

// Load a new graph into the engine. Offline graph operations will be performed
// on this graph. To switch it to the active graph, and run it, use Commit.
func (obj *Engine) Load(newGraph *pgraph.Graph) error {
	if obj.nextGraph != nil {
		return fmt.Errorf("can't overwrite pending graph, use abort")
	}
	obj.nextGraph = newGraph
	return nil
}

// Abort the pending graph and any work in progress on it. After this call you
// may Load a new graph.
func (obj *Engine) Abort() error {
	if obj.nextGraph == nil {
		return fmt.Errorf("there is no pending graph to abort")
	}
	obj.nextGraph = nil
	return nil
}

// Validate validates the pending graph to ensure it is appropriate for the
// engine. This should be called before Commit to avoid any surprises there!
// This prevents an error on Commit which could cause an engine shutdown.
func (obj *Engine) Validate() error {
	for _, vertex := range obj.nextGraph.Vertices() {
		res, ok := vertex.(engine.Res)
		if !ok {
			return fmt.Errorf("not a Res")
		}

		if err := engine.Validate(res); err != nil {
			return errwrap.Wrapf(err, "the Res did not Validate")
		}
	}
	return nil
}

// Apply a function to the pending graph. You must pass in a function which will
// receive this graph as input, and return an error if something does not
// succeed.
func (obj *Engine) Apply(fn func(*pgraph.Graph) error) error {
	return fn(obj.nextGraph)
}

// Commit runs a graph sync and swaps the loaded graph with the current one. If
// it errors, then the running graph wasn't changed. It is recommended that you
// pause the engine before running this, and resume it after you're done.
func (obj *Engine) Commit() error {
	// TODO: Does this hurt performance or graph changes ?

	vertexAddFn := func(vertex pgraph.Vertex) error {
		// some of these validation steps happen before this Commit step
		// in Validate() to avoid erroring here. These are redundant.
		// FIXME: should we get rid of this redundant validation?
		res, ok := vertex.(engine.Res)
		if !ok { // should not happen, previously validated
			return fmt.Errorf("not a Res")
		}
		if obj.Debug {
			obj.Logf("loading resource `%s`", res)
		}

		if _, exists := obj.state[vertex]; exists {
			return fmt.Errorf("the Res state already exists")
		}

		if obj.Debug {
			obj.Logf("Validate(%s)", res)
		}
		err := engine.Validate(res)
		if obj.Debug {
			obj.Logf("Validate(%s): Return(%+v)", res, err)
		}
		if err != nil {
			return errwrap.Wrapf(err, "the Res did not Validate")
		}

		// FIXME: is res.Name() sufficiently unique to use as a UID here?
		pathUID := fmt.Sprintf("%s-%s", res.Kind(), res.Name())
		statePrefix := fmt.Sprintf("%s/", path.Join(obj.Prefix, "state", pathUID))
		// don't create this unless it *will* be used
		//if err := os.MkdirAll(statePrefix, 0770); err != nil {
		//	return errwrap.Wrapf(err, "can't create state prefix")
		//}

		obj.waits[vertex] = &sync.WaitGroup{}
		obj.state[vertex] = &State{
			//Graph: obj.graph, // TODO: what happens if we swap the graph?
			Vertex: vertex,

			Program:  obj.Program,
			Hostname: obj.Hostname,

			World:  obj.World,
			Prefix: statePrefix,
			//Converger: obj.Converger,

			Debug: obj.Debug,
			Logf: func(format string, v ...interface{}) {
				obj.Logf(res.String()+": "+format, v...)
			},
		}
		if err := obj.state[vertex].Init(); err != nil {
			return errwrap.Wrapf(err, "the Res did not Init")
		}
		return nil
	}
	vertexRemoveFn := func(vertex pgraph.Vertex) error {
		// wait for exit before starting new graph!
		obj.state[vertex].Event(event.Exit) // signal an exit
		obj.waits[vertex].Wait()            // sync

		// close the state and resource
		// FIXME: will this mess up the sync and block the engine?
		if err := obj.state[vertex].Close(); err != nil {
			return errwrap.Wrapf(err, "the Res did not Close")
		}

		// delete to free up memory from old graphs
		delete(obj.state, vertex)
		delete(obj.waits, vertex)
		return nil
	}

	// If GraphSync succeeds, it updates the receiver graph accordingly...
	// Running the shutdown in vertexRemoveFn does not need to happen in a
	// topologically sorted order because it already paused in that order.
	obj.Logf("graph sync...")
	if err := obj.graph.GraphSync(obj.nextGraph, engine.VertexCmpFn, vertexAddFn, vertexRemoveFn, engine.EdgeCmpFn); err != nil {
		return errwrap.Wrapf(err, "error running graph sync")
	}
	obj.nextGraph = nil

	// After this point, we must not error or we'd need to restore all of
	// the changes that we'd made to the previously primary graph. This is
	// because this function is meant to atomically swap the graphs safely.

	// TODO: update all the `State` structs with the new Graph pointer
	//for _, vertex := range obj.graph.Vertices() {
	//	state, exists := obj.state[vertex]
	//	if !exists {
	//		continue
	//	}
	//	state.Graph = obj.graph // update pointer to graph
	//}

	return nil
}

// Start runs the currently active graph. It also un-pauses the graph if it was
// paused.
func (obj *Engine) Start() error {
	topoSort, err := obj.graph.TopologicalSort()
	if err != nil {
		return err
	}
	indegree := obj.graph.InDegree() // compute all of the indegree's
	reversed := pgraph.Reverse(topoSort)

	for _, vertex := range reversed {
		state := obj.state[vertex]
		state.starter = (indegree[vertex] == 0)
		var unpause = true // assume true

		if !state.working { // if not running...
			state.working = true
			unpause = false // doesn't need unpausing if starting
			obj.wg.Add(1)
			obj.waits[vertex].Add(1)
			go func(v pgraph.Vertex) {
				defer obj.wg.Done()
				defer obj.waits[vertex].Done()
				defer func() {
					obj.state[v].working = false
				}()

				obj.Logf("Worker(%s)", v)
				// contains the Watch and CheckApply loops
				err := obj.Worker(v)
				obj.Logf("Worker(%s): Exited(%+v)", v, err)
			}(vertex)
		}

		select {
		case <-state.started:
		case <-state.stopped: // we failed on Watch start
		}

		if unpause { // unpause (if needed)
			obj.state[vertex].Event(event.Start)
		}
	}
	// we wait for everyone to start before exiting!
	return nil
}

// SetFastPause puts the graph into fast pause mode. This is usually done via
// the argument to the Pause command, but this method can be used if a pause was
// already started, and you'd like subsequent parts to pause quickly. Once in
// fast pause mode for a given pause action, you cannot switch to regular pause.
// This is because once you've started a fast pause, some dependencies might
// have been skipped when fast pausing, and future resources might have missed a
// poke. In general this is only called when you're trying to hurry up the exit.
func (obj *Engine) SetFastPause() {
	obj.fastPause = true
}

// Pause the active, running graph. At the moment this cannot error.
func (obj *Engine) Pause(fastPause bool) {
	obj.fastPause = fastPause
	topoSort, _ := obj.graph.TopologicalSort()
	for _, vertex := range topoSort { // squeeze out the events...
		// The Event is sent to an unbuffered channel, so this event is
		// synchronous, and as a result it blocks until it is received.
		obj.state[vertex].Event(event.Pause)
	}

	// we are now completely paused...
	obj.fastPause = false // reset
}

// Close triggers a shutdown. Engine must be already paused before this is run.
func (obj *Engine) Close() error {
	var reterr error

	emptyGraph, err := pgraph.NewGraph("empty")
	if err != nil {
		reterr = multierr.Append(reterr, err) // list of errors
	}

	// this is a graph switch (graph sync) that switches to an empty graph!
	if err := obj.Load(emptyGraph); err != nil { // copy in empty graph
		reterr = multierr.Append(reterr, err)
	}
	// the commit will cause the graph sync to shut things down cleverly...
	if err := obj.Commit(); err != nil {
		reterr = multierr.Append(reterr, err)
	}

	obj.wg.Wait() // for now, this doesn't need to be a separate Wait() method
	return reterr
}

// Graph returns the running graph.
func (obj *Engine) Graph() *pgraph.Graph {
	return obj.graph
}
