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

package resources // TODO: can this be a separate package or will it break the dag?

import (
	"log"
	"sync"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/pgraph"
)

//go:generate stringer -type=graphState -output=graphstate_stringer.go
type graphState uint

const (
	graphStateNil graphState = iota
	graphStateStarting
	graphStateStarted
	graphStatePausing
	graphStatePaused
)

// MGraph is a meta graph structure used to encapsulate a generic graph
// structure alongside some non-generic elements.
type MGraph struct {
	//Graph *pgraph.Graph
	*pgraph.Graph // wrap a graph, and use its methods directly

	Data  *Data
	Debug bool

	state graphState
	// ptr b/c: Mutex/WaitGroup must not be copied after first use
	mutex *sync.Mutex
	wg    *sync.WaitGroup
}

// Init initializes the internal structures.
func (obj *MGraph) Init() {
	obj.mutex = &sync.Mutex{}
	obj.wg = &sync.WaitGroup{}
}

// getState returns the state of the graph. This state is used for optimizing
// certain algorithms by knowing what part of processing the graph is currently
// undergoing.
func (obj *MGraph) getState() graphState {
	//obj.mutex.Lock()
	//defer obj.mutex.Unlock()
	return obj.state
}

// setState sets the graph state and returns the previous state.
func (obj *MGraph) setState(state graphState) graphState {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	prev := obj.getState()
	obj.state = state
	return prev
}

// Update switches our graph structure to the new graph that we pass to it. This
// also updates any references to the old graph so that they're now correct. It
// also updates references to the Data structure that should be passed around.
func (obj *MGraph) Update(newGraph *pgraph.Graph) {
	obj.Graph = newGraph.Copy() // store as new active graph
	// update stored reference to graph and other values that need storing!
	for _, v := range obj.Graph.Vertices() {
		res := VtoR(v)          // resource
		*res.Data() = *obj.Data // push the data around
		res.Update(obj.Graph)   // update graph pointer
	}
}

// Start is a main kick to start the graph. It goes through in reverse
// topological sort order so that events can't hit un-started vertices.
func (obj *MGraph) Start(first bool) { // start or continue
	log.Printf("State: %v -> %v", obj.setState(graphStateStarting), obj.getState())
	defer log.Printf("State: %v -> %v", obj.setState(graphStateStarted), obj.getState())
	t, _ := obj.Graph.TopologicalSort()
	indegree := obj.Graph.InDegree() // compute all of the indegree's
	reversed := pgraph.Reverse(t)
	wg := &sync.WaitGroup{}
	for _, v := range reversed { // run the Setup() for everyone first
		// run these in parallel, as long as we wait before continuing
		wg.Add(1)
		go func(vertex pgraph.Vertex, res Res) {
			defer wg.Done()
			// TODO: can't we do this check outside of the goroutine?
			if !*res.Working() { // if Worker() is not running...
				// NOTE: vertex == res here, but pass in both in
				// case we ever wrap the res in something before
				// we store it as the vertex in the graph struct
				res.Setup(obj.Graph, vertex, res) // initialize some vars in the resource
			}
		}(v, VtoR(v))
	}
	wg.Wait()

	// run through the topological reverse, and start or unpause each vertex
	for _, v := range reversed {
		res := VtoR(v)
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
		// this triggers a CheckApply AFTER Watch is Running()
		// We *don't* need to also do this to new nodes or nodes that
		// are about to get unpaused, because they'll get poked by one
		// of the indegree == 0 vertices, and an important aspect of the
		// Process() function is that even if the state is correct, it
		// will pass through the Poke so that it flows through the DAG.
		res.Starter(indegree[v] == 0)

		var unpause = true
		if !*res.Working() { // if Worker() is not running...
			*res.Working() = true // set Worker() running flag

			unpause = false // doesn't need unpausing on first start
			obj.wg.Add(1)
			// must pass in value to avoid races...
			// see: https://ttboj.wordpress.com/2015/07/27/golang-parallelism-issues-causing-too-many-open-files-error/
			go func(vv pgraph.Vertex) {
				defer obj.wg.Done()
				// unset Worker() running flag just before exit
				defer func() { *VtoR(vv).Working() = false }()
				defer VtoR(vv).Reset()
				// TODO: if a sufficient number of workers error,
				// should something be done? Should these restart
				// after perma-failure if we have a graph change?
				log.Printf("%s: Started", vv)
				if err := VtoR(vv).Worker(); err != nil { // contains the Watch and CheckApply loops
					log.Printf("%s: Exited with failure: %v", vv, err)
					return
				}
				log.Printf("%s: Exited", vv)
			}(v)
		}

		select {
		case <-res.Started(): // block until started
		case <-res.Stopped(): // we failed on init
			// if the resource Init() fails, we don't hang!
		}

		if unpause { // unpause (if needed)
			res.SendEvent(event.EventStart, nil) // sync!
		}
	}
	// we wait for everyone to start before exiting!
}

// Pause sends pause events to the graph in a topological sort order. If you set
// the fastPause argument to true, then it will ask future propagation waves to
// not run through the graph before exiting, and instead will exit much quicker.
func (obj *MGraph) Pause(fastPause bool) {
	log.Printf("State: %v -> %v", obj.setState(graphStatePausing), obj.getState())
	defer log.Printf("State: %v -> %v", obj.setState(graphStatePaused), obj.getState())
	if fastPause {
		obj.Graph.SetValue("fastpause", true) // set flag
	}
	t, _ := obj.Graph.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		VtoR(v).SendEvent(event.EventPause, nil) // sync
	}
	obj.Graph.SetValue("fastpause", false) // reset flag
}

// Exit sends exit events to the graph in a topological sort order.
func (obj *MGraph) Exit() {
	if obj.Graph == nil { // empty graph that wasn't populated yet
		return
	}

	// FIXME: a second ^C could put this into fast pause, but do it for now!
	obj.Pause(true) // implement this with pause to avoid duplicating the code

	t, _ := obj.Graph.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		// turn off the taps...
		VtoR(v).Exit() // sync
	}
	obj.wg.Wait() // for now, this doesn't need to be a separate Wait() method
}
