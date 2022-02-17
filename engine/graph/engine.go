// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

// Package graph contains the actual implementation of the resource graph engine
// that runs the graph of resources in real-time. This package has the algorithm
// that runs all the graph transitions.
package graph

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/semaphore"
)

const (
	// StateDir is the name of the sub directory where all the local
	// resource state is stored.
	StateDir = "state"
)

// Engine encapsulates a generic graph and manages its operations.
type Engine struct {
	Program  string
	Version  string
	Hostname string
	World    engine.World

	// Prefix is a unique directory prefix which can be used. It should be
	// created if needed.
	Prefix    string
	Converger *converger.Coordinator

	Debug bool
	Logf  func(format string, v ...interface{})

	graph     *pgraph.Graph
	nextGraph *pgraph.Graph
	state     map[pgraph.Vertex]*State
	waits     map[pgraph.Vertex]*sync.WaitGroup // wg for the Worker func
	wlock     *sync.Mutex                       // lock around waits map

	slock *sync.Mutex // semaphore lock
	semas map[string]*semaphore.Semaphore

	wg *sync.WaitGroup // wg for the whole engine (only used for close)

	paused    bool // are we paused?
	fastPause bool
}

// Init initializes the internal structures and starts this the graph running.
// If the struct does not validate, or it cannot initialize, then this errors.
// Initially it will contain an empty graph.
func (obj *Engine) Init() error {
	if obj.Program == "" {
		return fmt.Errorf("the Program is empty")
	}
	if obj.Hostname == "" {
		return fmt.Errorf("the Hostname is empty")
	}

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
	obj.wlock = &sync.Mutex{}

	obj.slock = &sync.Mutex{}
	obj.semas = make(map[string]*semaphore.Semaphore)

	obj.wg = &sync.WaitGroup{}

	obj.paused = true // start off true, so we can Resume after first Commit

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

	start := []func() error{} // functions to run after graphsync to start...
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

		pathUID := engineUtil.ResPathUID(res)
		statePrefix := fmt.Sprintf("%s/", path.Join(obj.statePrefix(), pathUID))

		// don't create this unless it *will* be used
		//if err := os.MkdirAll(statePrefix, 0770); err != nil {
		//	return errwrap.Wrapf(err, "can't create state prefix")
		//}

		obj.waits[vertex] = &sync.WaitGroup{}
		obj.state[vertex] = &State{
			Graph:  obj.graph, // Update if we swap the graph!
			Vertex: vertex,

			Program:  obj.Program,
			Version:  obj.Version,
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

		fn := func() error {
			// start the Worker
			obj.wg.Add(1)
			obj.wlock.Lock()
			obj.waits[vertex].Add(1)
			obj.wlock.Unlock()
			go func(v pgraph.Vertex) {
				defer obj.wg.Done()
				defer func() {
					// we need this lock, because this go
					// routine could run when the next fn
					// function above here is running...
					obj.wlock.Lock()
					obj.waits[v].Done()
					obj.wlock.Unlock()
				}()

				obj.Logf("Worker(%s)", v)
				// contains the Watch and CheckApply loops
				err := obj.Worker(v)
				obj.Logf("Worker(%s): Exited(%+v)", v, err)
				obj.state[v].workerErr = err // store the error
				// If the Rewatch metaparam is true, then this will get
				// restarted if we do a graph cmp swap. This is why the
				// graph cmp function runs the removes before the adds.
				// XXX: This should feed into an $error var in the lang.
			}(vertex)
			return nil
		}
		start = append(start, fn) // do this at the end, if it's needed
		return nil
	}

	free := []func() error{} // functions to run after graphsync to reset...
	vertexRemoveFn := func(vertex pgraph.Vertex) error {
		// wait for exit before starting new graph!
		close(obj.state[vertex].removeDone) // causes doneChan to close
		obj.state[vertex].Resume()          // unblock from resume
		obj.waits[vertex].Wait()            // sync

		// close the state and resource
		// FIXME: will this mess up the sync and block the engine?
		if err := obj.state[vertex].Close(); err != nil {
			return errwrap.Wrapf(err, "the Res did not Close")
		}

		// delete to free up memory from old graphs
		fn := func() error {
			delete(obj.state, vertex)
			delete(obj.waits, vertex)
			return nil
		}
		free = append(free, fn) // do this at the end, so we don't panic
		return nil
	}

	// add the Worker swap (reload) on error decision into this vertexCmpFn
	vertexCmpFn := func(v1, v2 pgraph.Vertex) (bool, error) {
		r1, ok1 := v1.(engine.Res)
		r2, ok2 := v2.(engine.Res)
		if !ok1 || !ok2 { // should not happen, previously validated
			return false, fmt.Errorf("not a Res")
		}
		m1 := r1.MetaParams()
		m2 := r2.MetaParams()
		swap1, swap2 := true, true // assume default of true
		if m1 != nil {
			swap1 = m1.Rewatch
		}
		if m2 != nil {
			swap2 = m2.Rewatch
		}

		s1, ok1 := obj.state[v1]
		s2, ok2 := obj.state[v2]
		x1, x2 := false, false
		if ok1 {
			x1 = s1.workerErr != nil && swap1
		}
		if ok2 {
			x2 = s2.workerErr != nil && swap2
		}

		if x1 || x2 {
			// We swap, even if they're the same, so that we reload!
			// This causes an add and remove of the "same" vertex...
			return false, nil
		}

		return engine.VertexCmpFn(v1, v2) // do the normal cmp otherwise
	}

	// If GraphSync succeeds, it updates the receiver graph accordingly...
	// Running the shutdown in vertexRemoveFn does not need to happen in a
	// topologically sorted order because it already paused in that order.
	obj.Logf("graph sync...")
	if err := obj.graph.GraphSync(obj.nextGraph, vertexCmpFn, vertexAddFn, vertexRemoveFn, engine.EdgeCmpFn); err != nil {
		return errwrap.Wrapf(err, "error running graph sync")
	}
	// We run these afterwards, so that we don't unnecessarily start anyone
	// if GraphSync failed in some way. Otherwise we'd have to do clean up!
	for _, fn := range start {
		if err := fn(); err != nil {
			return errwrap.Wrapf(err, "error running start fn")
		}
	}
	// We run these afterwards, so that the state structs (that might get
	// referenced) are not destroyed while someone might poke or use one.
	for _, fn := range free {
		if err := fn(); err != nil {
			return errwrap.Wrapf(err, "error running free fn")
		}
	}
	obj.nextGraph = nil

	// After this point, we must not error or we'd need to restore all of
	// the changes that we'd made to the previously primary graph. This is
	// because this function is meant to atomically swap the graphs safely.

	// Update all the `State` structs with the new Graph pointer.
	for _, vertex := range obj.graph.Vertices() {
		state, exists := obj.state[vertex]
		if !exists {
			continue
		}
		state.Graph = obj.graph // update pointer to graph
	}

	return nil
}

// Resume runs the currently active graph. It also un-pauses the graph if it was
// paused. Very little that is interesting should happen here. It all happens in
// the Commit method. After Commit, new things are already started, but we still
// need to Resume any pre-existing resources.
func (obj *Engine) Resume() error {
	if !obj.paused {
		return fmt.Errorf("already resumed")
	}

	topoSort, err := obj.graph.TopologicalSort()
	if err != nil {
		return err
	}
	//indegree := obj.graph.InDegree() // compute all of the indegree's
	reversed := pgraph.Reverse(topoSort)

	for _, vertex := range reversed {
		//obj.state[vertex].starter = (indegree[vertex] == 0)
		obj.state[vertex].Resume() // doesn't error
	}
	// we wait for everyone to start before exiting!
	obj.paused = false
	return nil
}

// SetFastPause puts the graph into fast pause mode. This is usually done via
// the argument to the Pause command, but this method can be used if a pause was
// already started, and you'd like subsequent parts to pause quickly. Once in
// fast pause mode for a given pause action, you cannot switch to regular pause.
// This is because once you've started a fast pause, some dependencies might
// have been skipped when fast pausing, and future resources might have missed a
// poke. In general this is only called when you're trying to hurry up the exit.
// XXX: Not implemented
func (obj *Engine) SetFastPause() {
	obj.fastPause = true
}

// Pause the active, running graph.
func (obj *Engine) Pause(fastPause bool) error {
	if obj.paused {
		return fmt.Errorf("already paused")
	}

	obj.fastPause = fastPause
	topoSort, _ := obj.graph.TopologicalSort()
	for _, vertex := range topoSort { // squeeze out the events...
		// The Event is sent to an unbuffered channel, so this event is
		// synchronous, and as a result it blocks until it is received.
		if err := obj.state[vertex].Pause(); err != nil && err != engine.ErrClosed {
			return err
		}
	}

	obj.paused = true

	// we are now completely paused...
	obj.fastPause = false // reset
	return nil
}

// Close triggers a shutdown. Engine must be already paused before this is run.
func (obj *Engine) Close() error {
	emptyGraph, reterr := pgraph.NewGraph("empty")

	// this is a graph switch (graph sync) that switches to an empty graph!
	if err := obj.Load(emptyGraph); err != nil { // copy in empty graph
		reterr = errwrap.Append(reterr, err)
	}
	// FIXME: Do we want to run commit if Load failed? Does this even work?
	// the commit will cause the graph sync to shut things down cleverly...
	if err := obj.Commit(); err != nil {
		reterr = errwrap.Append(reterr, err)
	}

	obj.wg.Wait() // for now, this doesn't need to be a separate Wait() method
	return reterr
}

// Graph returns the running graph.
func (obj *Engine) Graph() *pgraph.Graph {
	return obj.graph
}

// statePrefix returns the dir where all the resource state is stored locally.
func (obj *Engine) statePrefix() string {
	return fmt.Sprintf("%s/", path.Join(obj.Prefix, StateDir))
}
