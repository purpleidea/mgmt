// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package funcs

import (
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// State represents the state of a function vertex. This corresponds to an AST
// expr, which is the memory address (pointer) in the graph.
type State struct {
	Expr interfaces.Expr // pointer to the expr vertex

	handle interfaces.Func // the function (if not nil, we've found it on init)

	init   bool // have we run Init on our func?
	ready  bool // has it received all the args it needs at least once?
	loaded bool // has the func run at least once ?
	closed bool // did we close ourself down?

	notify chan struct{} // ping here when new input values exist

	input  chan types.Value // the top level type must be a struct
	output chan types.Value
	txn    interfaces.Txn // API of graphTxn struct to pass to each function

	mutex *sync.RWMutex // concurrency guard for modifying Expr with String/SetValue
}

// Init creates the function state if it can be found in the registered list.
func (obj *State) Init() error {
	handle, err := obj.Expr.Func() // build one and store it, don't re-gen
	if err != nil {
		return err
	}
	if err := handle.Validate(); err != nil {
		return errwrap.Wrapf(err, "could not validate func")
	}
	obj.handle = handle

	sig := obj.handle.Info().Sig
	if sig.Kind != types.KindFunc {
		return fmt.Errorf("must be kind func")
	}
	if len(sig.Ord) > 0 {
		// since we accept input, better get our notification chan built
		obj.notify = make(chan struct{})
	}

	obj.input = make(chan types.Value)  // we close this when we're done
	obj.output = make(chan types.Value) // we create it, func closes it
	obj.txn = (&graphTxn{}).init()

	obj.mutex = &sync.RWMutex{}

	return nil
}

// String satisfies fmt.Stringer so that these print nicely.
func (obj *State) String() string {
	// TODO: use global mutex since it's harder to add state specific mutex
	//obj.mutex.RLock() // prevent race detector issues against SetValue
	//defer obj.mutex.RUnlock()
	// FIXME: also add read locks on any of the children Expr in obj.Expr
	return obj.Expr.String()
}

// Engine represents the running time varying directed acyclic function graph.
type Engine struct {
	Graph    *pgraph.Graph
	Hostname string
	World    engine.World
	Debug    bool
	Logf     func(format string, v ...interface{})

	// Glitch: https://en.wikipedia.org/wiki/Reactive_programming#Glitches
	Glitch bool // allow glitching? (more responsive, but less accurate)

	ag      chan error // used to aggregate fact events without reflect
	agLock  *sync.Mutex
	agCount int // last one turns out the light (closes the ag channel)

	topologicalSort []pgraph.Vertex // cached sorting of the graph for perf

	state map[pgraph.Vertex]*State // state associated with the vertex

	mutex *sync.RWMutex                 // concurrency guard for the table map
	table map[pgraph.Vertex]types.Value // live table of output values

	loaded     bool          // are all of the funcs loaded?
	loadedChan chan struct{} // funcs loaded signal

	streamChan chan error // signals a new graph can be created or problem

	closeChan chan struct{} // close signal
	wg        *sync.WaitGroup
}

// Init initializes the struct. This is the first call you must make. Do not
// proceed with calls to other methods unless this succeeds first. This also
// loads all the functions by calling Init on each one in the graph.
// TODO: should Init take the graph as an input arg to keep it as a private
// field?
func (obj *Engine) Init() error {
	obj.ag = make(chan error)
	obj.agLock = &sync.Mutex{}
	obj.state = make(map[pgraph.Vertex]*State)
	obj.mutex = &sync.RWMutex{}
	obj.table = make(map[pgraph.Vertex]types.Value)
	obj.loadedChan = make(chan struct{})
	obj.streamChan = make(chan error)
	obj.closeChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}
	topologicalSort, err := obj.Graph.TopologicalSort()
	if err != nil {
		return errwrap.Wrapf(err, "topo sort failed")
	}
	obj.topologicalSort = topologicalSort // cache the result

	for _, vertex := range obj.Graph.Vertices() {
		// is this an interface we can use?
		if _, exists := obj.state[vertex]; exists {
			return fmt.Errorf("vertex (%+v) is not unique in the graph", vertex)
		}

		expr, ok := vertex.(interfaces.Expr)
		if !ok {
			return fmt.Errorf("vertex (%+v) was not an expr", vertex)
		}

		if obj.Debug {
			obj.Logf("Loading func `%s`", vertex)
		}

		obj.state[vertex] = &State{Expr: expr} // store some state!

		e1 := obj.state[vertex].Init()
		e2 := errwrap.Wrapf(e1, "error loading func `%s`", vertex)
		err = errwrap.Append(err, e2) // list of errors
	}
	if err != nil { // usually due to `not found` errors
		return errwrap.Wrapf(err, "could not load requested funcs")
	}
	return nil
}

// Validate the graph type checks properly and other tests. Must run Init first.
// This should check that: (1) all vertices have the correct number of inputs,
// (2) that the *Info signatures all match correctly, (3) that the argument
// names match correctly, and that the whole graph is statically correct.
func (obj *Engine) Validate() error {
	inList := func(needle interfaces.Func, haystack []interfaces.Func) bool {
		if needle == nil {
			panic("nil value passed to inList") // catch bugs!
		}
		for _, x := range haystack {
			if needle == x {
				return true
			}
		}
		return false
	}
	var err error
	ptrs := []interfaces.Func{} // Func is a ptr
	for _, vertex := range obj.Graph.Vertices() {
		node := obj.state[vertex]
		// TODO: this doesn't work for facts because they're in the Func
		// duplicate pointers would get closed twice, causing a panic...
		if inList(node.handle, ptrs) { // check for duplicate ptrs!
			e := fmt.Errorf("vertex `%s` has duplicate ptr", vertex)
			err = errwrap.Append(err, e)
		}
		ptrs = append(ptrs, node.handle)
	}
	for _, edge := range obj.Graph.Edges() {
		if _, ok := edge.(*interfaces.FuncEdge); !ok {
			e := fmt.Errorf("edge `%s` was not the correct type", edge)
			err = errwrap.Append(err, e)
		}
	}
	if err != nil {
		return err // stage the errors so the user can fix many at once!
	}

	// check if vertices expecting inputs have them
	for vertex, count := range obj.Graph.InDegree() {
		node := obj.state[vertex]
		if exp := len(node.handle.Info().Sig.Ord); exp != count {
			e := fmt.Errorf("expected %d inputs to `%s`, got %d", exp, node, count)
			if obj.Debug {
				obj.Logf("expected %d inputs to `%s`, got %d", exp, node, count)
				obj.Logf("expected: %+v for `%s`", node.handle.Info().Sig.Ord, node)
			}
			err = errwrap.Append(err, e)
		}
	}

	// expected vertex -> argName
	expected := make(map[*State]map[string]int) // expected input fields
	for vertex1 := range obj.Graph.Adjacency() {
		// check for outputs that don't go anywhere?
		//node1 := obj.state[vertex1]
		//if len(obj.Graph.Adjacency()[vertex1]) == 0 { // no vertex1 -> vertex2
		//	if node1.handle.Info().Sig.Output != nil {
		//		// an output value goes nowhere...
		//	}
		//}
		for vertex2 := range obj.Graph.Adjacency()[vertex1] { // populate
			node2 := obj.state[vertex2]
			expected[node2] = make(map[string]int)
			for _, key := range node2.handle.Info().Sig.Ord {
				expected[node2][key] = 1
			}
		}
	}

	for vertex1 := range obj.Graph.Adjacency() {
		node1 := obj.state[vertex1]
		for vertex2, edge := range obj.Graph.Adjacency()[vertex1] {
			node2 := obj.state[vertex2]
			edge := edge.(*interfaces.FuncEdge)
			// check vertex1 -> vertex2 (with e) is valid

			for _, arg := range edge.Args { // loop over each arg
				sig := node2.handle.Info().Sig
				if len(sig.Ord) == 0 {
					e := fmt.Errorf("no input expected from `%s` to `%s` with arg `%s`", node1, node2, arg)
					err = errwrap.Append(err, e)
					continue
				}

				if count, exists := expected[node2][arg]; !exists {
					e := fmt.Errorf("wrong input name from `%s` to `%s` with arg `%s`", node1, node2, arg)
					err = errwrap.Append(err, e)
				} else if count == 0 {
					e := fmt.Errorf("duplicate input from `%s` to `%s` with arg `%s`", node1, node2, arg)
					err = errwrap.Append(err, e)
				}
				expected[node2][arg]-- // subtract one use

				out := node1.handle.Info().Sig.Out
				if out == nil {
					e := fmt.Errorf("no output possible from `%s` to `%s` with arg `%s`", node1, node2, arg)
					err = errwrap.Append(err, e)
					continue
				}
				typ, exists := sig.Map[arg] // key in struct
				if !exists {
					// second check of this!
					e := fmt.Errorf("wrong input name from `%s` to `%s` with arg `%s`", node1, node2, arg)
					err = errwrap.Append(err, errwrap.Wrapf(e, "programming error"))
					continue
				}

				if typ.Kind == types.KindVariant { // FIXME: hack for now
					// pass (input arg variants)
				} else if out.Kind == types.KindVariant { // FIXME: hack for now
					// pass (output arg variants)
				} else if typ.Cmp(out) != nil {
					e := fmt.Errorf("type mismatch from `%s` (%s) to `%s` (%s) with arg `%s`", node1, out, node2, typ, arg)
					err = errwrap.Append(err, e)
				}
			}
		}
	}

	// check for leftover function inputs which weren't filled up by outputs
	// (we're trying to call a function with fewer input args than required)
	for node, m := range expected { // map[*State]map[string]int
		for arg, count := range m {
			if count != 0 { // count should be zero if all were used
				e := fmt.Errorf("missing input to `%s` on arg `%s`", node, arg)
				err = errwrap.Append(err, e)
			}
		}
	}

	if err != nil {
		return err // stage the errors so the user can fix many at once!
	}

	return nil
}

// Run starts up this function engine and gets it all running. It errors if the
// startup failed for some reason. On success, use the Stream and Table methods
// for future interaction with the engine, and the Close method to shut it off.
func (obj *Engine) Run() error {
	if len(obj.topologicalSort) == 0 { // no funcs to load!
		close(obj.loadedChan)
		close(obj.streamChan)
		return nil
	}

	// TODO: build a timer that runs while we wait for all funcs to startup.
	// after some delay print a message to tell us which funcs we're waiting
	// for to startup and that they are slow and blocking everyone, and then
	// fail permanently after the timeout so that bad code can't block this!

	// loop through all funcs that we might need
	obj.agAdd(len(obj.topologicalSort))
	for _, vertex := range obj.topologicalSort {
		node := obj.state[vertex]
		if obj.Debug {
			obj.SafeLogf("Startup func `%s`", node)
		}
		// populate each txn object with the handles it needs...
		obj.state[vertex].txn.(*graphTxn).Graph = obj.Graph   // pgraph
		obj.state[vertex].txn.(*graphTxn).Lock = obj.Lock     // fn
		obj.state[vertex].txn.(*graphTxn).Unlock = obj.Unlock // fn

		incoming := obj.Graph.IncomingGraphVertices(vertex) // []Vertex

		init := &interfaces.Init{
			Hostname: obj.Hostname,
			Input:    node.input,
			Output:   node.output,
			Txn:      node.txn,
			World:    obj.World,
			Debug:    obj.Debug,
			Logf: func(format string, v ...interface{}) {
				obj.Logf("func: "+format, v...)
			},
		}
		if err := node.handle.Init(init); err != nil {
			return errwrap.Wrapf(err, "could not init func `%s`", node)
		}
		node.init = true // we've successfully initialized

		// no incoming edges, so no incoming data
		if len(incoming) == 0 { // TODO: do this here or earlier?
			close(node.input)
		} else {
			// process function input data
			obj.wg.Add(1)
			go func(vertex pgraph.Vertex) {
				node := obj.state[vertex]
				defer obj.wg.Done()
				defer close(node.input)
				var ready bool
				// the final closing output to this, closes this
				for range node.notify { // new input values
					// now build the struct if we can...

					ready = true // assume for now...
					si := &types.Type{
						// input to functions are structs
						Kind: types.KindStruct,
						Map:  node.handle.Info().Sig.Map,
						Ord:  node.handle.Info().Sig.Ord,
					}
					st := types.NewStruct(si)
					for _, v := range incoming {
						args := obj.Graph.Adjacency()[v][vertex].(*interfaces.FuncEdge).Args
						from := obj.state[v]
						obj.mutex.RLock()
						value, exists := obj.table[v]
						obj.mutex.RUnlock()
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
								panic(fmt.Sprintf("struct set failure on `%s` from `%s`: %v", node, from, err))
							}
						}
					}
					if !ready {
						continue
					}

					select {
					case node.input <- st: // send to function

					case <-obj.closeChan:
						return
					}
				}
			}(vertex)
		}

		obj.wg.Add(1)
		go func(vertex pgraph.Vertex) { // run function
			node := obj.state[vertex]
			defer obj.wg.Done()
			if obj.Debug {
				obj.SafeLogf("Running func `%s`", node)
			}
			err := node.handle.Stream()
			if obj.Debug {
				obj.SafeLogf("Exiting func `%s`", node)
			}
			if err != nil {
				// we closed with an error...
				err := errwrap.Wrapf(err, "problem streaming func `%s`", node)
				select {
				case obj.ag <- err: // send to aggregate channel

				case <-obj.closeChan:
					return
				}
			}
		}(vertex)

		obj.wg.Add(1)
		go func(vertex pgraph.Vertex) { // process function output data
			node := obj.state[vertex]
			defer obj.wg.Done()
			defer obj.agDone(vertex)
			outgoing := obj.Graph.OutgoingGraphVertices(vertex) // []Vertex
			for value := range node.output {                    // read from channel
				obj.mutex.RLock()
				cached, exists := obj.table[vertex]
				obj.mutex.RUnlock()
				if !exists { // first value received
					// RACE: do this AFTER value is present!
					//node.loaded = true // not yet please
					obj.Logf("func `%s` started", node)
				} else if value.Cmp(cached) == nil {
					// skip if new value is same as previous
					// if this happens often, it *might* be
					// a bug in the function implementation
					// FIXME: do we need to disable engine
					// caching when using hysteresis?
					obj.Logf("func `%s` skipped", node)
					continue
				}
				obj.mutex.Lock()
				// XXX: maybe we can get rid of the table...
				obj.table[vertex] = value // save the latest
				node.mutex.Lock()
				if err := node.Expr.SetValue(value); err != nil {
					node.mutex.Unlock() // don't block node.String()
					panic(fmt.Sprintf("could not set value for `%s`: %+v", node, err))
				}
				node.loaded = true // set *after* value is in :)
				obj.Logf("func `%s` changed", node)
				node.mutex.Unlock()
				obj.mutex.Unlock()

				// FIXME: will this actually prevent glitching?
				// if we only notify the aggregate channel when
				// we're at the bottom of the topo sort (eg: no
				// outgoing vertices to notify) then we'll have
				// a glitch free subtree in the programs ast...
				if obj.Glitch || len(outgoing) == 0 {
					select {
					case obj.ag <- nil: // send to aggregate channel

					case <-obj.closeChan:
						return
					}
				}

				// notify the receiving vertices
				for _, v := range outgoing {
					node := obj.state[v]
					select {
					case node.notify <- struct{}{}:

					case <-obj.closeChan:
						return
					}
				}
			}
			// no more output values are coming...
			obj.Logf("func `%s` stopped", node)

			// nodes that never loaded will cause the engine to hang
			if !node.loaded {
				select {
				case obj.ag <- fmt.Errorf("func `%s` stopped before it was loaded", node):
				case <-obj.closeChan:
					return
				}
			}
		}(vertex)
	}

	// send event on streamChan when any of the (aggregated) facts change
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.streamChan)
	Loop:
		for {
			var err error
			var ok bool
			select {
			case err, ok = <-obj.ag: // aggregated channel
				if !ok {
					break Loop // channel shutdown
				}

				if !obj.loaded {
					// now check if we're ready
					var loaded = true // initially assume true
					for _, vertex := range obj.topologicalSort {
						node := obj.state[vertex]
						node.mutex.RLock()
						nodeLoaded := node.loaded
						node.mutex.RUnlock()
						if !nodeLoaded {
							loaded = false // we were wrong
							// TODO: add better "not loaded" reporting
							if obj.Debug {
								obj.Logf("not yet loaded: %s", node)
							}
							break
						}
					}
					obj.loaded = loaded

					if obj.loaded {
						// this causes an initial start
						// signal to be sent out below,
						// since the stream sender runs
						if obj.Debug {
							obj.Logf("loaded")
						}
						close(obj.loadedChan) // signal
					} else {
						if err == nil {
							continue // not ready to send signal
						} // pass errors through...
					}
				}

			case <-obj.closeChan:
				return
			}

			// send stream signal
			select {
			// send events or errors on streamChan, eg: func failure
			case obj.streamChan <- err: // send
				if err != nil {
					return
				}
			case <-obj.closeChan:
				return
			}
		}
	}()

	return nil
}

// Lock takes a "lock" so that we can safely modify the running graph without
// shutting it down. We don't want to stop and restart it because that would
// lose any state that the currently running functions might be holding.
func (obj *Engine) Lock() {
	// XXX: not implemented yet
	panic("not implemented")
}

// Unlock frees that "lock" so that we can safely resume the running graph that
// we previously just paused. We don't want to stop and restart it because that
// would lose any state that the currently running functions might be holding.
func (obj *Engine) Unlock() {
	// XXX: not implemented yet
	panic("not implemented")
}

// agAdd registers a user on the ag channel.
func (obj *Engine) agAdd(i int) {
	defer obj.agLock.Unlock()
	obj.agLock.Lock()
	obj.agCount += i
}

// agDone closes the channel if we're the last one using it.
func (obj *Engine) agDone(vertex pgraph.Vertex) {
	defer obj.agLock.Unlock()
	obj.agLock.Lock()
	node := obj.state[vertex]
	node.closed = true

	// FIXME: (perf) cache this into a table which we narrow down with each
	// successive call. look at the outgoing vertices that I would affect...
	for _, v := range obj.Graph.OutgoingGraphVertices(vertex) { // close for each one
		// now determine who provides inputs to that vertex...
		var closed = true
		for _, vv := range obj.Graph.IncomingGraphVertices(v) {
			// are they all closed?
			if !obj.state[vv].closed {
				closed = false
				break
			}
		}
		if closed { // if they're all closed, we can close the input
			close(obj.state[v].notify)
		}
	}

	if obj.agCount == 0 {
		close(obj.ag)
	}
}

// RLock takes a read lock on the data that gets written to the AST, so that
// interpret can be run without anything changing part way through.
func (obj *Engine) RLock() {
	obj.mutex.RLock()
}

// RUnlock frees a read lock on the data that gets written to the AST, so that
// interpret can be run without anything changing part way through.
func (obj *Engine) RUnlock() {
	obj.mutex.RUnlock()
}

// SafeLogf logs a message, although it adds a read lock around the logging in
// case a `node` argument is passed in which would set off the race detector.
func (obj *Engine) SafeLogf(format string, v ...interface{}) {
	// We're adding a global mutex, because it's harder to only isolate the
	// individual node specific mutexes needed since it may contain others!
	if len(v) > 0 {
		obj.mutex.RLock()
	}
	obj.Logf(format, v...)
	if len(v) > 0 {
		obj.mutex.RUnlock()
	}
}

// Stream returns a channel of engine events. Wait for nil events to know when
// the Table map has changed. An error event means this will shutdown shortly.
// Do not run the Table function before we've received one non-error event.
func (obj *Engine) Stream() chan error {
	return obj.streamChan
}

// Close shuts down the function engine. It waits till everything has finished.
func (obj *Engine) Close() error {
	var err error
	for _, vertex := range obj.topologicalSort { // FIXME: should we do this in reverse?
		node := obj.state[vertex]
		if node.init { // did we Init this func?
			if e := node.handle.Close(); e != nil {
				e := errwrap.Wrapf(e, "problem closing func `%s`", node)
				err = errwrap.Append(err, e) // list of errors
			}
		}
	}
	close(obj.closeChan)
	obj.wg.Wait() // wait so that each func doesn't need to do this in close
	return err
}

// graphTxn holds the state of a transaction and runs it when needed. When this
// has been setup and initialized, it implements the Txn API that can be used by
// functions in their Stream method to modify the function graph while it is
// "running".
type graphTxn struct {
	// Graph is a handle pointing to the graph structure we're modifying.
	Graph *pgraph.Graph

	// Lock is a handle to the lock function to call before the operation.
	Lock func()

	// Unlock is a handle to the unlock function to call before the
	// operation.
	Unlock func()

	// ops is a list of operations to run on a graph
	ops []func(*pgraph.Graph)

	// mutex guards changes to the ops list
	mutex *sync.Mutex
}

// init must be called to initialized the struct before first use. This is
// private because the creator, not the user should run it.
func (obj *graphTxn) init() interfaces.Txn {
	obj.ops = []func(*pgraph.Graph){}
	obj.mutex = &sync.Mutex{}

	return obj // return self so it can be called in a chain
}

// AddVertex adds a vertex to the running graph. The operation will get
// completed when Commit is run.
// XXX: should this be interfaces.Expr instead of pgraph.Vertex ?
func (obj *graphTxn) AddVertex(xv ...pgraph.Vertex) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	op := func(g *pgraph.Graph) {
		g.AddVertex(xv...)
	}
	obj.ops = append(obj.ops, op)

	return obj // return self so it can be called in a chain
}

// AddEdge adds an edge to the running graph. The operation will get completed
// when Commit is run.
// XXX: should this be interfaces.Expr instead of pgraph.Vertex ?
// XXX: should this be interfaces.FuncEdge instead of pgraph.Edge ?
func (obj *graphTxn) AddEdge(v1, v2 pgraph.Vertex, e pgraph.Edge) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	op := func(g *pgraph.Graph) {
		g.AddEdge(v1, v2, e)
	}
	obj.ops = append(obj.ops, op)

	return obj // return self so it can be called in a chain
}

// DeleteVertex adds a vertex to the running graph. The operation will get
// completed when Commit is run.
// XXX: should this be interfaces.Expr instead of pgraph.Vertex ?
func (obj *graphTxn) DeleteVertex(xv ...pgraph.Vertex) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	op := func(g *pgraph.Graph) {
		g.DeleteVertex(xv...)
	}
	obj.ops = append(obj.ops, op)

	return obj // return self so it can be called in a chain
}

// DeleteEdge adds a vertex to the running graph. The operation will get
// completed when Commit is run.
// XXX: should this be interfaces.FuncEdge instead of pgraph.Edge ?
func (obj *graphTxn) DeleteEdge(xe ...pgraph.Edge) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	op := func(g *pgraph.Graph) {
		g.DeleteEdge(xe...)
	}
	obj.ops = append(obj.ops, op)

	return obj // return self so it can be called in a chain
}

// Commit runs the pending transaction.
func (obj *graphTxn) Commit() error {
	// Lock our internal state mutex first... this prevents other AddVertex
	// or similar calls from interferring with our work here.
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if len(obj.ops) == 0 { // nothing to do
		return nil
	}

	// TODO: Instead of requesting the below locks, it's conceivable that we
	// could either write an engine that doesn't require pausing the graph
	// with a lock, or one that doesn't in the specific case being changed
	// here need locks. And then in theory we'd have improved performance
	// from the function engine. For our function consumers, the Txn API
	// would never need to change, so we don't break API! A simple example
	// is the len(ops) == 0 one right above. A simplification, but shows we
	// aren't forced to call the locks even when we get Commit called here.

	// Now request the lock from the actual graph engine.
	obj.Lock()
	defer obj.Unlock()

	// Copy the graph structure, perform the ops, check we didn't add a
	// cycle, and if it's safe, do the real thing. Otherwise error here.
	g := obj.Graph.Copy() // copy the graph structure
	for _, x := range obj.ops {
		x(g) // call it
	}
	if _, err := g.TopologicalSort(); err != nil {
		return errwrap.Wrapf(err, "topo sort failed in txn commit")
	}
	// FIXME: is there anything else we should check? Should we type-check?

	// Now do it for real...
	for _, x := range obj.ops {
		x(obj.Graph) // call it
	}
	obj.ops = []func(*pgraph.Graph){} // clear it
	return nil
}

// Clear erases any pending transactions that weren't committed yet.
func (obj *graphTxn) Clear() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	obj.ops = []func(*pgraph.Graph){} // clear it
}
