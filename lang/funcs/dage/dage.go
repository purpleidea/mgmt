// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

// Package dage implements a DAG function engine.
// TODO: can we rename this to something more interesting?
package dage

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

var (
	// XXX: temp for debugging
	xxxDebuggingGraphvizLock  = &sync.Mutex{}
	xxxDebuggingGraphvizCount = int64(0)
)

// Engine implements a dag engine which lets us "run" a dag of functions, but
// also allows us to modify it while we are running.
// XXX: this could be wrapped with the Txn API we wrote...
type Engine struct {
	// Name is the name used for the instance of the engine and in the graph
	// that is held within it.
	Name string

	Hostname string
	World    engine.World

	Debug bool
	Logf  func(format string, v ...interface{})

	// Glitch: https://en.wikipedia.org/wiki/Reactive_programming#Glitches
	Glitch bool // allow glitching? (more responsive, but less accurate)

	// Callback can be specified as an alternative to using the Stream
	// method to get events. If the context on it is cancelled, then it must
	// shutdown quickly, because this means we are closing and want to
	// disconnect. Whether you want to respect that is up to you, but the
	// engine will not be able to close until you do. If specified, and an
	// error has occurred, it will set that error property.
	Callback func(context.Context, error)

	graph *pgraph.Graph
	table map[interfaces.Func]types.Value
	state map[interfaces.Func]*state

	// mutex wraps any internal operation so that this engine is
	// thread-safe. It especially guards access to the graph.
	mutex *sync.Mutex

	// rwmutex wraps any read or write access to the table or state fields.
	rwmutex *sync.RWMutex
	wg      *sync.WaitGroup

	// pause/resume state machine signals
	pauseChan   chan struct{}
	pausedChan  chan struct{}
	resumeChan  chan struct{}
	resumedChan chan struct{}

	// resend tracks which new nodes might need a new notification
	resend map[interfaces.Func]struct{}

	ag         chan error // used to aggregate events
	wgAg       *sync.WaitGroup
	streamChan chan error

	loaded bool // are all of the funcs loaded?
	//loadedChan chan struct{} // funcs loaded signal

	startedChan chan struct{} // closes when Run() starts
}

// Setup sets up the internal datastructures needed for this engine.
func (obj *Engine) Setup() error {
	var err error
	obj.graph, err = pgraph.NewGraph(obj.Name)
	if err != nil {
		return err
	}
	obj.table = make(map[interfaces.Func]types.Value)
	obj.state = make(map[interfaces.Func]*state)
	obj.mutex = &sync.Mutex{}
	obj.rwmutex = &sync.RWMutex{}
	obj.wg = &sync.WaitGroup{}

	obj.pauseChan = make(chan struct{})
	obj.pausedChan = make(chan struct{})
	obj.resumeChan = make(chan struct{})
	obj.resumedChan = make(chan struct{})

	obj.resend = make(map[interfaces.Func]struct{})

	obj.ag = make(chan error)
	obj.wgAg = &sync.WaitGroup{}
	obj.streamChan = make(chan error)
	//obj.loadedChan = make(chan struct{}) // TODO: currently not used
	obj.startedChan = make(chan struct{})

	return nil
}

// Cleanup cleans up and frees memory and resources after everything is done.
func (obj *Engine) Cleanup() error {
	obj.wg.Wait()        // don't cleanup these before Run() finished
	close(obj.pauseChan) // free
	close(obj.pausedChan)
	close(obj.resumeChan)
	close(obj.resumedChan)
	return nil
}

// addVertex is the lockless version of the AddVertex function. This is needed
// so that AddEdge can add two vertices within the same lock.
func (obj *Engine) addVertex(f interfaces.Func) error {
	if _, exists := obj.state[f]; exists {
		// don't err dupes, because it makes using the AddEdge API yucky
		return nil
	}

	// add some extra checks for easier debugging
	if f == nil {
		return fmt.Errorf("missing func")
	}
	if f.Info() == nil {
		return fmt.Errorf("missing func info")
	}
	sig := f.Info().Sig
	if sig == nil {
		return fmt.Errorf("missing func sig")
	}
	if sig.Kind != types.KindFunc {
		return fmt.Errorf("must be kind func")
	}
	if err := f.Validate(); err != nil {
		return errwrap.Wrapf(err, "node did not Validate")
	}

	input := make(chan types.Value)
	output := make(chan types.Value)
	graphTxn := &graphTxn{
		GraphAPI: obj,
		Lock:     obj.Lock,
		Unlock:   obj.Unlock,
	}
	txn := graphTxn.init()

	// This is the one of two places where we modify this map. To avoid
	// concurrent writes, we only do this when we're locked! Anywhere that
	// can read where we are locked must have a mutex around it or do the
	// lookup when we're in an unlocked state.
	node := &state{
		Func: f,
		name: f.String(), // cache a name

		input:  input,
		output: output,
		txn:    txn,

		running: false,
		wg:      &sync.WaitGroup{},

		rwmutex: &sync.RWMutex{},
	}
	if len(sig.Ord) > 0 {
		// since we accept input, better get our notification chan built
		node.notify = make(chan struct{})
	}

	init := &interfaces.Init{
		Hostname: obj.Hostname,
		Input:    node.input,
		Output:   node.output,
		Txn:      node.txn,
		World:    obj.World,
		Debug:    obj.Debug,
		Logf: func(format string, v ...interface{}) {
			// safe Logf in case f.String contains %? chars...
			s := f.String() + ": " + fmt.Sprintf(format, v...)
			obj.Logf("%s", s)
		},
	}

	if err := f.Init(init); err != nil {
		return err
	}
	// only now, do we modify the graph
	obj.state[f] = node
	obj.graph.AddVertex(f)
	return nil
}

// AddVertex is the thread-safe way to add a vertex. You will need to call the
// engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) AddVertex(f interfaces.Func) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	return obj.addVertex(f) // lockless version
}

// AddEdge is the thread-safe way to add an edge. You will need to call the
// engine Lock method before using this and the Unlock method afterwards. This
// will automatically run AddVertex on both input vertices if they are not
// already part of the graph. You should only create DAG's as this function
// engine cannot handle cycles and this method will error if you cause a cycle.
func (obj *Engine) AddEdge(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	// safety check to avoid cycles
	g := obj.graph.Copy()
	//g.AddVertex(f1)
	//g.AddVertex(f2)
	g.AddEdge(f1, f2, fe)
	if _, err := g.TopologicalSort(); err != nil {
		return err // not a dag
	}
	// if we didn't cycle, we can modify the real graph safely...

	// Does the graph already have these nodes in it?
	hasf1 := obj.graph.HasVertex(f1)
	//hasf2 := obj.graph.HasVertex(f2)

	if err := obj.addVertex(f1); err != nil { // lockless version
		return err
	}
	if err := obj.addVertex(f2); err != nil {
		// rollback f1 on error of f2
		obj.deleteVertex(f1) // ignore any error
		return err
	}

	// If f1 doesn't exist, let f1 (or it's incoming nodes) get the notify.
	// If f2 is new, then it should get a new notification unless f1 is new.
	// But there's no guarantee we didn't AddVertex(f2); AddEdge(f1, f2, e),
	// so resend if f1 already exists. Otherwise it's not a new notification.
	// previously: `if hasf1 && !hasf2`
	if hasf1 {
		obj.resend[f2] = struct{}{} // resend notification to me
	}
	obj.graph.AddEdge(f1, f2, fe)

	// This shouldn't error, since the test graph didn't find a cycle.
	if _, err := obj.graph.TopologicalSort(); err != nil {
		// programming error
		panic(err) // not a dag
	}

	return nil
}

// deleteVertex is the lockless version of the DeleteVertex function. This is
// needed so that AddEdge can add two vertices within the same lock. It needs
// deleteVertex so it can rollback the first one if the second addVertex fails.
func (obj *Engine) deleteVertex(f interfaces.Func) error {
	node, exists := obj.state[f]
	if !exists {
		return fmt.Errorf("vertex %s doesn't exist", f)
	}

	if node.running {
		// cancel the running vertex
		node.cancel() // cancel inner ctx
		node.wg.Wait()
		if node.notify != nil { // if sig.Ord == 0, we didn't make it!
			close(node.notify) // after node.wg.Wait() finishes, we're done
		}
	}

	// This is the one of two places where we modify this map. To avoid
	// concurrent writes, we only do this when we're locked! Anywhere that
	// can read where we are locked must have a mutex around it or do the
	// lookup when we're in an unlocked state.
	delete(obj.state, f)
	obj.graph.DeleteVertex(f)
	return nil
}

// DeleteVertex is the thread-safe way to delete a vertex. You will need to call
// the engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) DeleteVertex(f interfaces.Func) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	return obj.deleteVertex(f) // lockless version
}

// DeleteEdge is the thread-safe way to delete an edge. You will need to call
// the engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) DeleteEdge(e *interfaces.FuncEdge) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	// Don't bother checking if edge exists first and don't error if it
	// doesn't because it might have gotten deleted when a vertex did, and
	// so there's no need to complain for nothing.
	obj.graph.DeleteEdge(e)

	return nil
}

// AddGraph is the thread-safe way to add a graph. You will need to call the
// engine Lock method before using this and the Unlock method afterwards. This
// will automatically run AddVertex and AddEdge on the graph vertices and edges
// if they are not already part of the graph.
func (obj *Engine) AddGraph(graph *pgraph.Graph) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	var err error
	status := make(map[interfaces.Func]struct{})
Loop:
	for v1, x := range graph.Adjacency() {
		f1, ok := v1.(interfaces.Func)
		if !ok {
			return fmt.Errorf("vertex is not a Func")
		}
		err = obj.addVertex(f1) // lockless version
		if err != nil {
			break Loop
		}
		status[f1] = struct{}{}

		for v2, edge := range x {
			f2, ok := v2.(interfaces.Func)
			if !ok {
				return fmt.Errorf("vertex is not a Func")
			}
			err = obj.addVertex(f2) // make the Init happen now
			if err != nil {
				break Loop
			}
			status[f2] = struct{}{}

			fe, ok := edge.(*interfaces.FuncEdge)
			if !ok {
				return fmt.Errorf("edge is not a FuncEdge")
			}
			// Just do the graph changes ourself because we already
			// add the vertices, and to rollback edge changes, we
			// only need to delete either involved vertex. We do the
			// cycle check manually once here, instead of each time.
			//obj.addEdge(f1, f2, fe) // lockless version
			obj.graph.AddEdge(f1, f2, fe) // cycle check at the end
		}
	}

	rollback := func() {
		for f := range status {
			obj.graph.DeleteVertex(f)
			delete(obj.state, f)
		}
	}

	if err != nil { // rollback
		rollback()
		return err
	}

	if _, err := obj.graph.TopologicalSort(); err != nil {
		rollback()
		return err // not a dag
	}
	// if we didn't cycle, we're done!
	return nil
}

// HasVertex is the thread-safe way to check if a vertex exists in the graph.
// You will need to call the engine Lock method before using this and the Unlock
// method afterwards.
func (obj *Engine) HasVertex(f interfaces.Func) bool {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	return obj.graph.HasVertex(f)
}

// LookupEdge is the thread-safe way to check which vertices (if any) exist
// between an edge in the graph. You will need to call the engine Lock method
// before using this and the Unlock method afterwards.
func (obj *Engine) LookupEdge(fe *interfaces.FuncEdge) (interfaces.Func, interfaces.Func, bool) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	v1, v2, found := obj.graph.LookupEdge(fe)
	if !found {
		return nil, nil, found
	}
	f1, ok := v1.(interfaces.Func)
	if !ok {
		panic("not a Func")
	}
	f2, ok := v2.(interfaces.Func)
	if !ok {
		panic("not a Func")
	}
	return f1, f2, found
}

// Lock must be used before modifying the running graph. Make sure to Unlock
// when done.
func (obj *Engine) Lock() { // pause
	obj.rwmutex.Lock() // TODO: or should it go right after pauseChan?
	select {
	case obj.pauseChan <- struct{}{}:
	}
	//obj.rwmutex.Lock() // TODO: or should it go right before pauseChan?

	// waiting for the pause to move to paused...
	select {
	case <-obj.pausedChan:
	}
	// this mutex locks at start of Run() and unlocks at finish of Run()
	obj.mutex.Unlock() // safe to make changes now
}

// Unlock must be used after modifying the running graph. Make sure to Lock
// beforehand.
func (obj *Engine) Unlock() { // resume
	// this mutex locks at start of Run() and unlocks at finish of Run()
	obj.mutex.Lock() // no more changes are allowed
	select {
	case obj.resumeChan <- struct{}{}:
	}
	//obj.rwmutex.Unlock() // TODO: or should it go right after resumedChan?

	// waiting for the resume to move to resumed...
	select {
	case <-obj.resumedChan:
	}
	obj.rwmutex.Unlock() // TODO: or should it go right before resumedChan?
}

// Run kicks off the main engine. This takes a mutex. When we're "paused" the
// mutex is temporarily released until we "resume". Those operations transition
// with the engine Lock and Unlock methods. It is recommended to only add
// vertices to the engine after it's running. If you add them before Run, then
// Run will cause a Lock/Unlock to occur to cycle them in. Lock and Unlock race
// with the cancellation of this Run main loop. Make sure to only call one at a
// time.
func (obj *Engine) Run(ctx context.Context) error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	// XXX: can the above defer get called while we are already unlocked?
	// XXX: is it a possibility if we use <-Started() ?
	obj.wg.Add(1)
	defer obj.wg.Done()
	ctx, cancel := context.WithCancel(ctx) // wrap parent
	defer cancel()

	close(obj.startedChan)

	if n := obj.graph.NumVertices(); n > 0 { // hack to make the api easier
		obj.Logf("graph contained %d vertices before Run", n)
		obj.wg.Add(1)
		go func() {
			defer obj.wg.Done()
			// kick the engine once to pull in any vertices from
			// before we started running!
			defer obj.Unlock()
			obj.Lock()
		}()
	}

	// close the aggregate channel when everyone is done with it...
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		select {
		case <-ctx.Done():
		}
		// don't wait and close ag before we're really done with Run()
		obj.wgAg.Wait() // wait for last ag user to close
		close(obj.ag)   // last one closes the ag channel
	}()

	// aggregate events channel
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.streamChan)
		for {
			var err error
			var ok bool
			select {
			case err, ok = <-obj.ag: // aggregated channel
				if !ok {
					return // channel shutdown
				}
			}

			obj.rwmutex.RLock()
			loaded := obj.loaded // this gets reset when graph adds new nodes
			obj.rwmutex.RUnlock()
			if !loaded {
				obj.rwmutex.RLock()
				// XXX: cache/memoize this value
				vertices := obj.graph.Vertices() // []pgraph.Vertex
				obj.rwmutex.RUnlock()

				// now check if we're ready
				loaded = true // initially assume true
				for _, v := range vertices {
					f, ok := v.(interfaces.Func)
					if !ok {
						panic("not a Func")
					}
					node, exists := obj.state[f]
					if !exists {
						panic(fmt.Sprintf("missing node in loaded: %s", f))
					}

					node.rwmutex.RLock()
					nodeLoaded := node.loaded
					node.rwmutex.RUnlock()
					if !nodeLoaded {
						loaded = false // we were wrong
						// TODO: add better "not loaded" reporting
						if obj.Debug {
							obj.Logf("not yet loaded: %s", node)
						}
						break
					}
				}
				obj.rwmutex.Lock()
				obj.loaded = loaded
				obj.rwmutex.Unlock()
				if loaded {
					// this causes an initial start
					// signal to be sent out below,
					// since the stream sender runs
					if obj.Debug {
						obj.Logf("loaded")
					}
					// TODO: since we can get the load
					// signal multiple times, either wrap
					// this in a *sync.Once or use a
					// different mechanism for this...
					//close(obj.loadedChan) // signal
				} else {
					if err == nil {
						continue // not ready to send signal
					} // pass errors through...
				}
			}

			// now send event...
			if obj.Callback != nil {
				// send stream signal (callback variant)
				obj.Callback(ctx, err)
			} else {
				// send stream signal
				select {
				// send events or errors on streamChan
				case obj.streamChan <- err: // send
				case <-ctx.Done(): // when asked to exit
					return
				}
			}
			if err != nil {
				cancel() // cancel the context!
				return
			}
		}
	}()

	// we start off "running", but we'll have an empty graph initially...
	for {
		// wait until paused/locked request comes in...
		select {
		case <-obj.pauseChan:
			obj.Logf("pausing...")

		case <-ctx.Done(): // when asked to exit
			return nil // we exit happily
		}

		// Toposort for paused workers. We run this before the actual
		// pause completes, because the second we are paused, the graph
		// could then immediately change. We don't need a lock in here
		// because the mutex only unlocks when pause is complete below.
		topoSort1, err := obj.graph.TopologicalSort()
		if err != nil {
			return err
		}
		for _, v := range topoSort1 {
			// XXX API
			//v.Pause?()
			//obj.state[vertex].whatever
			_ = v // XXX
		}

		// pause is complete
		// no exit case from here, must be fully running or paused...
		select {
		case obj.pausedChan <- struct{}{}:
			obj.Logf("paused!")
		}

		//
		// the graph changes shape right here... we are locked right now
		//

		// wait until resumed/unlocked
		select {
		case <-obj.resumeChan:
			obj.Logf("resuming...")

		case <-ctx.Done(): // when asked to exit
			return nil // we exit happily
		}

		// Toposort to run/resume workers. (Bottom of toposort first!)
		topoSort2, err := obj.graph.TopologicalSort()
		if err != nil {
			return err
		}
		reversed := pgraph.Reverse(topoSort2)
		for _, v := range reversed {
			f, ok := v.(interfaces.Func)
			if !ok {
				panic("not a Func")
			}
			node, exists := obj.state[f]
			if !exists {
				panic(fmt.Sprintf("missing node in iterate: %s", f))
			}

			if node.running { // it's not a new vertex
				continue
			}
			//obj.rwmutex.Lock() // already locked between resuming and resumed
			obj.loaded = false // reset this
			//obj.rwmutex.Unlock()
			node.running = true

			innerCtx, innerCancel := context.WithCancel(ctx) // wrap parent
			// we defer innerCancel() in the goroutine to cleanup!
			node.ctx = innerCtx
			node.cancel = innerCancel

			// run mainloop
			obj.wgAg.Add(1)
			node.wg.Add(1)
			go func(f interfaces.Func, node *state) {
				defer node.wg.Done()
				defer obj.wgAg.Done()
				defer node.cancel() // if we close, clean up and send the signal to anyone watching
				if obj.Debug {
					obj.SafeLogf("Running func `%s`", node)
				}
				runErr := f.Stream(node.ctx)
				if obj.Debug {
					obj.SafeLogf("Exiting func `%s`", node)
				}
				if runErr != nil {
					// send to a aggregate channel
					// the first to error will cause ag to
					// shutdown, so make sure we can exit...
					select {
					case obj.ag <- runErr: // send to aggregate channel
					case <-node.ctx.Done():
					}
				}
				// if node never loaded, then we error in the node.output loop!
			}(f, node)

			// process events
			obj.wgAg.Add(1)
			node.wg.Add(1)
			go func(f interfaces.Func, node *state) {
				defer node.wg.Done()
				defer obj.wgAg.Done()

				if node.Func.String() == "FuncValue" {
					fmt.Printf("engine waiting for (%p FuncValue) to output to (%p chan)\n", node.Func, node.output)
				}
				for value := range node.output { // read from channel
					if node.Func.String() == "FuncValue" {
						fmt.Printf("engine received an output from (%p FuncValue)'s output (%p chan)\n", node.Func, node.output)
					}
					if value == nil {
						// bug!
						obj.SafeLogf("func `%s` got nil value", node)
						panic("got nil value")
					}

					obj.rwmutex.RLock()
					cached, exists := obj.table[f]
					obj.rwmutex.RUnlock()
					if !exists { // first value received
						// RACE: do this AFTER value is present!
						//node.loaded = true // not yet please
						obj.SafeLogf("func `%s` started", node)
					} else if value.Cmp(cached) == nil {
						// skip if new value is same as previous
						// if this happens often, it *might* be
						// a bug in the function implementation
						// FIXME: do we need to disable engine
						// caching when using hysteresis?
						obj.SafeLogf("func `%s` skipped", node)
						continue
					}
					obj.rwmutex.Lock()
					obj.table[f] = value // save the latest
					obj.rwmutex.Unlock()
					node.rwmutex.Lock()
					node.loaded = true // set *after* value is in :)
					//obj.Logf("func `%s` changed", node)
					node.rwmutex.Unlock()

					// XXX: I think we need this read lock
					// because we don't want to be adding a
					// new vertex here but then missing to
					// send an event to it because it
					// started after we did the range...
					obj.rwmutex.RLock()
					// XXX: cache/memoize this value
					outgoing := obj.graph.OutgoingGraphVertices(f) // []pgraph.Vertex
					obj.rwmutex.RUnlock()
					// XXX combine with the above locks?

					// TODO: if shutdown, did we still want to do this?
					if obj.Glitch || len(outgoing) == 0 {
						select {
						case obj.ag <- nil: // send to aggregate channel
						case <-node.ctx.Done():
							return
						}
					}

					// notify the receiving vertices
					for _, v := range outgoing {
						f, ok := v.(interfaces.Func)
						if !ok {
							panic("not a Func")
						}
						obj.rwmutex.RLock()
						destNode, exists := obj.state[f]
						if !exists {
							panic(fmt.Sprintf("missing node in outgoing: %s", f))
						}
						obj.rwmutex.RUnlock()
						// I think this _could_ block if
						// the graph changes while we're
						// in here, or if a node crashes
						// and isn't around to receive
						// any more notifications, so we
						// add a select case for that
						// notify loop to not block. We
						// can also have a ctx exit here
						// if the specific node closes.
						select {
						case destNode.notify <- struct{}{}:
						case <-node.ctx.Done(): // our node closed
							continue // safety
						case <-destNode.ctx.Done(): // dest node closed
							continue // any reason to return instead?
						}
					}
				} // end for
				if node.Func.String() == "FuncValue" {
					fmt.Printf("engine saw (%p FuncValue) close its output (%p chan)\n", node.Func, node.output)
				}

				// no more output values are coming...
				//obj.SafeLogf("func `%s` stopped", node)

				// XXX shouldn't we record the fact that obj.output closed, and close
				// the downstream node's obj.input?

				// nodes that never loaded will cause the engine to hang
				if !node.loaded {
					select {
					case obj.ag <- fmt.Errorf("func `%s` stopped before it was loaded", node):
					case <-node.ctx.Done():
						return
					}
				}

			}(f, node)

			incoming := obj.graph.IncomingGraphVertices(v) // []Vertex
			// no incoming edges, so no incoming data
			if len(incoming) == 0 { // TODO: do this here or earlier?
				close(node.input)
				continue
			} // else, process input data below...

			// process function input data
			node.wg.Add(1)
			go func(f interfaces.Func, node *state) {
				defer node.wg.Done()
				defer close(node.input)
				if node.Func.String() == "call" {
					defer fmt.Printf("engine is closing (%p CallFunc)'s input (%p chan)\n", node.Func, node.input)
				}
				if node.notify == nil { // if sig.Ord == 0, it's nil
					// extra safety, should be caught above
					return
				}

				var ready bool
				// the final closing output to this, closes this
				for range node.notify { // new input values
					// now build the struct if we can...
					obj.rwmutex.RLock()
					// XXX: cache/memoize this value
					incoming := obj.graph.IncomingGraphVertices(f) // []pgraph.Vertex
					obj.rwmutex.RUnlock()

					ready = true // assume for now...
					si := &types.Type{
						// input to functions are structs
						Kind: types.KindStruct,
						Map:  node.Func.Info().Sig.Map,
						Ord:  node.Func.Info().Sig.Ord,
					}
					st := types.NewStruct(si)
					for _, v := range incoming {
						obj.rwmutex.RLock()
						args := obj.graph.Adjacency()[v][f].(*interfaces.FuncEdge).Args
						obj.rwmutex.RUnlock()
						v, ok := v.(interfaces.Func)
						if !ok {
							panic("not a Func")
						}
						obj.rwmutex.RLock()
						fromNode, exists := obj.state[v]
						if !exists {
							panic(fmt.Sprintf("missing node in notify: %s", v))
						}
						value, exists := obj.table[v]
						obj.rwmutex.RUnlock()
						if !exists {
							ready = false // nope!
							break
						}

						// set each arg, since one value
						// could get used for multiple
						// function inputs (shared edge)
						for _, arg := range args {
							err := st.Set(arg, value) // populate struct
							if err != nil {
								//panic(fmt.Sprintf("struct set failure on `%s` from `%s`: %v", node, fromNode, err))
								keys := []string{}
								for k := range st.Struct() {
									keys = append(keys, k)
								}
								panic(fmt.Sprintf("struct set failure on `%s` from `%s`: %v, has: %v", node, fromNode, err, keys))
							}
						}
					}
					if !ready {
						continue
					}

					if node.Func.String() == "call" {
						fmt.Printf("engine is sending to (%p CallFunc)'s input (%p chan)\n", node.Func, node.input)
					}
					select {
					case node.input <- st: // send to function

					case <-node.ctx.Done(): // our node closed
						// If the node we're sending to
						// closed, loop until the notify
						// closes as we might get an
						// erroneous or delayed one OR
						// an extra one could come it if
						// the graph changed mid notify.
						continue

					case <-ctx.Done(): // XXX: do we need to close on parent ctx shutdown?
						return
					}
				}
			}(f, node)
		} // end for

		// Send new notifications in case any new edges are sending away
		// to these... They might have already missed the notifications!
		for k := range obj.resend { // resend TO these!
			obj.Graphviz(false) // XXX DEBUG
			node, exists := obj.state[k]
			if !exists {
				continue
			}
			// Run as a goroutine to avoid erroring in parent thread.
			wg.Add(1)
			go func(node *state) {
				defer wg.Done()
				select {
				case node.notify <- struct{}{}:
					obj.Logf("resend to func `%s`", node) // safelogf would deadlock here
				case <-ctx.Done():
					return // no error returned in goroutine!
				}
			}(node)
		}
		obj.resend = make(map[interfaces.Func]struct{}) // reset

		// now check their states...
		for _, v := range reversed {
			v, ok := v.(interfaces.Func)
			if !ok {
				panic("not a Func")
			}
			_ = v
			//close(obj.state[v].startup) XXX once?

			// wait for startup XXX XXX XXX XXX
			//select {
			//case <-obj.state[v].startup:
			////case XXX close?:
			//}

			// XXX API
			//v.Resume?()
			//obj.state[vertex].whatever
		}

		// resume is complete
		// no exit case from here, must be fully running or paused...
		select {
		case obj.resumedChan <- struct{}{}:
			obj.Logf("resumed!")
		}

	} // end for
}

func (obj *Engine) Stream() <-chan error {
	return obj.streamChan
}

// Table returns a copy of the populated data table of values. We return a copy
// because since these values are constantly changing, we need an atomic
// snapshot to present to the consumer of this API.
// TODO: is this globally glitch consistent?
// TODO: do we need an API to return a single value? (wrapped in read locks)
func (obj *Engine) Table() map[interfaces.Func]types.Value {
	obj.rwmutex.RLock()
	defer obj.rwmutex.RUnlock()
	table := make(map[interfaces.Func]types.Value)
	for k, v := range obj.table {
		//table[k] = v.Copy() // TODO: do we need to copy these values?
		table[k] = v
	}
	return table
}

// Apply is similar to Table in that it gives you access to the internal output
// table of data, the difference being that it instead passes this information
// to a function of your choosing and holds a read/write lock during the entire
// time that your function is synchronously executing. If you use this function
// to spawn any goroutines that read or write data, then you're asking for a
// panic.
// XXX: does this need to be a Lock? Can it be an RLock? Check callers!
func (obj *Engine) Apply(fn func(map[interfaces.Func]types.Value) error) error {
	// XXX: does this need to be a Lock? Can it be an RLock? Check callers!
	obj.rwmutex.Lock() // differs from above RLock around obj.table
	defer obj.rwmutex.Unlock()
	table := make(map[interfaces.Func]types.Value)
	for k, v := range obj.table {
		//table[k] = v.Copy() // TODO: do we need to copy these values?
		table[k] = v
	}

	return fn(table)
}

// Started returns a channel that closes when the Run function finishes starting
// up. This is useful so that we can wait before calling any of the mutex things
// that would normally panic if Run wasn't started up first.
func (obj *Engine) Started() <-chan struct{} {
	return obj.startedChan
}

// SafeLogf logs a message, although it adds a read lock around the logging in
// case a `node` argument is passed in which would set off the race detector. It
// is not guaranteed to log this method synchronously.
// XXX: this is so ugly, can we do something better?
func (obj *Engine) SafeLogf(format string, v ...interface{}) {
	obj.Logf(format, v...)
	return // XXX
	// We're adding a waitgroup and running it with a goroutine, because it
	// seems it can happen when we're locked and this can cause a deadlock!
	// We're adding a global mutex, because it's harder to only isolate the
	// individual node specific mutexes needed since it may contain others!
	obj.wg.Add(1)
	go func() { // avoid deadlocks with rwmutex
		defer obj.wg.Done()
		if len(v) > 0 {
			obj.rwmutex.RLock()
		}
		obj.Logf(format, v...)
		if len(v) > 0 {
			obj.rwmutex.RUnlock()
		}
	}()
}

// Graphviz is a temporary method used for debugging.
func (obj *Engine) Graphviz(lock bool) {
	if lock {
		obj.rwmutex.RLock()
		defer obj.rwmutex.RUnlock()
	}

	xxxDebuggingGraphvizLock.Lock()
	defer xxxDebuggingGraphvizLock.Unlock()

	if xxxDebuggingGraphvizCount == 0 {
		xxxDebuggingGraphvizCount = time.Now().Unix()
	}

	d := time.Now().UnixMilli()
	if err := os.MkdirAll(fmt.Sprintf("/tmp/engine-graphviz-%d/", xxxDebuggingGraphvizCount), 0755); err != nil {
		panic(err)
	}
	if err := obj.graph.ExecGraphviz("dot", fmt.Sprintf("/tmp/engine-graphviz-%d/%d.dot", xxxDebuggingGraphvizCount, d), ""); err != nil {
		panic("no graphviz")
	}
}

// state tracks some internal vertex-specific state information.
type state struct {
	Func interfaces.Func
	name string // cache a name here for safer concurrency

	notify chan struct{} // ping here when new input values exist

	input  chan types.Value // the top level type must be a struct
	output chan types.Value
	txn    interfaces.Txn // API of graphTxn struct to pass to each function

	//init   bool // have we run Init on our func?
	//ready  bool // has it received all the args it needs at least once?
	loaded bool // has the func run at least once ?
	//closed bool // did we close ourself down?

	running bool
	wg      *sync.WaitGroup
	ctx     context.Context // per state ctx (inner ctx)
	cancel  func()          // cancel above inner ctx

	rwmutex *sync.RWMutex // concurrency guard for reading/modifying this state
}

// String implements the fmt.Stringer interface for pretty printing!
func (obj *state) String() string {
	if obj.name != "" {
		return obj.name
	}

	return obj.Func.String()
}
