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

// Package dage implements a DAG function engine.
// TODO: can we rename this to something more interesting?
package dage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/lang/funcs/ref"
	"github.com/purpleidea/mgmt/lang/funcs/txn"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Engine implements a dag engine which lets us "run" a dag of functions, but
// also allows us to modify it while we are running. The functions we support
// can take one of two forms.
//
// 1) A function that supports the normal interfaces.Func API. It has Call() in
// particular. Most functions are done with only this API.
//
// 2) A function which adds Stream() to support the StreamableFunc API. Some
// functions use this along with Event() to notify of a new value.
//
// 3) A third *possible* (but not currently implemented API) would be one that
// has Stream() but takes an Input and Output channel of values instead. This is
// similar to what we previously had. Of note, every input must generate one
// output value. (And more spontaneous output values are allowed as well.)
//
// Of note, functions that support Call() Can also cause an interrupt to happen.
// It's not clear if this (3) option would be allowed to have a Call() method or
// not, and if it would send interrupts on the Output channel.
//
// Of additional note, some functions also require the "ShapelyFunc" API to work
// correctly. Use of this is rare.
//
// XXX: If this engine continuously receives function events at a higher speed
// than it can process, then it will bog down and consume memory infinitely. We
// should consider adding some sort of warning or error if we get to a certain
// size.
//
// XXX: It's likely that this engine could be made even more efficient by more
// cleverly traversing through the graph. Instead of a topological sort, we
// could have some fancy map that determines what's remaining to go through, so
// that when we "interrupt" we don't needlessly repeatedly visit nodes again and
// trigger the "epoch skip" situations. We could also do the incremental
// toposort so that we don't properly re-run the whole algorithm over and over
// if we're always just computing changes.
//
// XXX: We could consider grouping multiple incoming events into a single
// descent into the DAG. It's not clear if this kind of de-duplication would
// break some "glitch-free" aspects or not. It would probably improve
// performance but we'd have to be careful about how we did it.
//
// XXX: Respect the info().Pure and info().Memo fields somewhere...
type Engine struct {
	// Name is the name used for the instance of the engine and in the graph
	// that is held within it.
	Name string

	Hostname string
	Local    *local.API
	World    engine.World

	Debug bool
	Logf  func(format string, v ...interface{})

	// graph is the internal graph. It is only changed during interrupt.
	graph *pgraph.Graph

	// refCount keeps track of vertex and edge references across the entire
	// graph.
	refCount *ref.Count

	// state stores some per-vertex (function) state
	state map[interfaces.Func]*state

	// wg counts every concurrent process here.
	wg *sync.WaitGroup

	// ag is the aggregation channel, which receives events from any of the
	// StreamableFunc's that are running.
	// XXX: add a mechanism to detect if it gets too full
	ag *util.InfiniteChan[*state]
	//ag chan *state

	// cancel can be called to shutdown Run() after it's started of course.
	cancel func()

	// streamChan is used to send the stream of tables to the outside world.
	streamChan chan interfaces.Table

	// interrupt specifies that a txn "commit" just happened.
	interrupt bool

	// topoSort is the last topological sort we ran.
	topoSort []pgraph.Vertex

	// ops is a list of operations to run during interrupt. This is usually
	// a delete vertex, but others are possible.
	ops []ops

	// err contains the last error after a shutdown occurs.
	err      error
	errMutex *sync.Mutex // guards err

	// graphvizCount keeps a running tally of how many graphs we've
	// generated. This is useful for displaying a sequence (timeline) of
	// graphs in a linear order.
	graphvizCount int64

	// graphvizDirectory stores the generated path for outputting graphviz
	// files if one is not specified at runtime.
	graphvizDirectory string
}

// Setup sets up the internal datastructures needed for this engine. We use this
// earlier step before Run() because it's usually not called concurrently, which
// makes it easier to catch the obvious errors before Run() runs in a goroutine.
func (obj *Engine) Setup() error {
	var err error
	obj.graph, err = pgraph.NewGraph(obj.Name)
	if err != nil {
		return err
	}
	obj.state = make(map[interfaces.Func]*state)

	obj.refCount = (&ref.Count{}).Init()

	obj.wg = &sync.WaitGroup{}
	obj.errMutex = &sync.Mutex{}

	//obj.ag = make(chan *state, 1) // for group events
	//obj.ag = make(chan *state) // normal no buffer, we can't drop any
	obj.ag = util.NewInfiniteChan[*state]() // lock-free but unbounded

	obj.streamChan = make(chan interfaces.Table)

	obj.ops = []ops{}

	return nil
}

// Run kicks off the function engine. You must add the initial graph via Txn
// *before* you run this function.
// XXX: try and fix the engine so you can run either Txn or Run first.
func (obj *Engine) Run(ctx context.Context) error {
	if obj.refCount == nil { // any arbitrary flag would be fine here
		return fmt.Errorf("you must run Setup before first use")
	}

	//obj.wg = &sync.WaitGroup{} // in Setup
	defer obj.wg.Wait()

	// cancel to allow someone to shut everything down...
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	obj.cancel = cancel

	//obj.streamChan = make(chan Table) // in Setup
	defer close(obj.streamChan)

	err := obj.process(ctx, 1) // start like this for now
	obj.errAppend(err)
	return err
}

// process could be combined with Run, but it is left separate in case we try to
// build a recursive process operation that runs on a subgraph. It would need an
// incoming graph argument as well, I would expect.
func (obj *Engine) process(ctx context.Context, epoch int64) error {

	mapping := make(map[pgraph.Vertex]int)
	start := 0
	table := make(interfaces.Table) // map[interfaces.Func]types.Value

Start:
	for {
		// If it's our first time, we want to interrupt, because we may
		// not ever get any events otherwise, and we'd block at select
		// waiting on obj.ag forever. Remember that the effect() of Txn
		// causes an interrupt when we add the first graph in. This
		// means we need to do the initial Txn before we startup here!
		if obj.interrupt {
			if obj.Debug {
				obj.Logf("interrupt!")
			}

			// Handle delete (and other ops) first. We keep checking
			// until this is empty, because a Cleanup operation
			// running inside this loop could cause more vertices
			// to be added, and so on.
			for len(obj.ops) > 0 {
				op := obj.ops[0]      // run in same order added
				obj.ops = obj.ops[1:] // queue

				// adds are new vertices which join the graph
				if add, ok := op.(*addVertex); ok {
					table[add.f] = nil // for symmetry

					if err := add.fn(ctx); err != nil { // Init!
						return err
					}

					continue
				}

				// deletes are the list of Func's (vertices)
				// that were deleted in a txn.
				if del, ok := op.(*deleteVertex); ok {
					delete(table, del.f) // cleanup the table

					if err := del.fn(ctx); err != nil { // Cleanup!
						return err
					}

					continue
				}
			}

			// Interrupt should only happen if we changed the graph
			// shape, so recompute the topological sort right here.

			// XXX: Can we efficiently edit the old topoSort by
			// knowing the add/del? If the graph is shrinking, just
			// remove those vertices from our current toposort. If
			// the graph is growing, can we topo sort the subset and
			// put them at the beginning of our old toposort? Is it
			// guaranteed that "spawned nodes" will have earlier
			// precedence than existing stuff? Can we be clever? Can
			// we "float" anything upwards that's needed for the
			// "toposort" by seeing what we're connected to, and
			// sort all of that?
			var err error
			obj.topoSort, err = obj.graph.TopologicalSort()
			if err != nil {
				// programming error
				return err
			}

			// This interrupt must be set *after* the above deletes
			// happen, because those can cause transactions to run,
			// and those transactions run obj.effect() which resets
			// this interrupt value back to true!
			obj.interrupt = false            // reset
			start = 0                        // restart the loop
			for i, v := range obj.topoSort { // TODO: Do it once here, or repeatedly below?
				mapping[v] = i
			}

			goto PreIterate // skip waiting for a new event
		}

		if n := obj.graph.NumVertices(); n == 0 {
			// If we're here, then the engine is done, because we'd
			// block forever.
			return nil
		}
		if obj.Debug {
			obj.Logf("waiting for event...")
		}
		select {
		case node, ok := <-obj.ag.Out:
			if obj.Debug {
				obj.Logf("got event: %v", node)
			}
			if !ok {
				// TODO: If we don't have events, maybe shutdown?
				panic("unexpected event channel shutdown")
			}
			// i is 0 if missing
			i, _ := mapping[node.Func] // get the node to start from...
			start = i
			// XXX: Should we ACK() here so that Stream can "make"
			// the new value available to it's Call() starting now?

		case <-ctx.Done():
			return ctx.Err()
		}

		// We have at least one event now!

	PreIterate:
		valid := true // assume table is valid for this iteration

	Iterate:
		// Iterate down through the graph...
		for i := start; i < len(obj.topoSort); i++ { // formerly: for _, v := range obj.topoSort
			start = i // set for subsequent runs
			v := obj.topoSort[i]
			f, ok := v.(interfaces.Func)
			if !ok {
				panic("not a Func")
			}
			if obj.Debug {
				obj.Logf("topo(%d): %p %+v", i, f, f)
			}
			mapping[v] = i // store for subsequent loops

			node, exists := obj.state[f]
			if !exists {
				panic(fmt.Sprintf("node state missing: %s", f))
			}

			streamableFunc, isStreamable := f.(interfaces.StreamableFunc)
			if isStreamable && !node.started { // don't start twice
				obj.wg.Add(1)
				go func() {
					defer obj.wg.Done()
					// XXX: I think the design should be that
					// if this ever shuts down, then the
					// function engine should shut down, but
					// that the individual Call() can error...
					// This is inline with our os.Readfilewait
					// function which models the logic we want...
					// If the call errors AND we have the Except
					// feature, then we want that Except to run,
					// but if we get a new event, then we should
					// try again. basically revive itself after
					// an errored Call function. Of course if
					// Stream shuts down, we're nuked, so maybe
					// we might want to retry... So maybe a resource
					// could tweak the retry params for such a
					// function??? Or maybe a #pragma kind of
					// directive thing above each function???
					err := streamableFunc.Stream(ctx)
					if err == nil {
						return
					}
					obj.errAppend(err)
					obj.cancel() // error
				}()
				node.started = true
			}

			if node.epoch >= epoch { // we already did this one
				if obj.Debug {
					obj.Logf("epoch skip: %p %v", f, f)
				}
				continue
			}

			// XXX: memoize until graph shape changes?
			incoming := obj.graph.IncomingGraphVertices(f) // []pgraph.Vertex

			// Not all of the incoming edges have been added yet.
			// We start by doing the "easy" count, and if it fails,
			// we fall back on the slightly more expensive, and
			// accurate count. This is because logical edges can be
			// combined into a single physical edge. This happens if
			// we have the same arg (a, b) passed to the same func.
			if n := len(node.Func.Info().Sig.Ord); n != len(incoming) && n != realEdgeCount(obj.graph.IncomingGraphEdges(f)) {
				if obj.Debug {
					obj.Logf("edge skip: %p %v", f, f)
				}

				valid = false
				// If we skip here, we also want to skip any of
				// the vertices that depend on this one. This is
				// because the toposort might offer our children
				// before a non-dependent node which might be
				// the node that causes the interrupt which adds
				// the edge which is currently not added yet.
				continue
			}

			// if no incoming edges, no incoming data, so this noop's

			si := &types.Type{
				// input to functions are structs
				Kind: types.KindStruct,
				Map:  node.Func.Info().Sig.Map,
				Ord:  node.Func.Info().Sig.Ord,
			}
			st := types.NewStruct(si)
			// The above builds a struct with fields
			// populated for each key (empty values)
			// so we need to very carefully check if
			// every field is received before we can
			// safely send it downstream to an edge.
			need := make(map[string]struct{}) // keys we need
			for _, k := range node.Func.Info().Sig.Ord {
				need[k] = struct{}{}
			}

			for _, vv := range incoming {
				ff, ok := vv.(interfaces.Func)
				if !ok {
					panic("not a Func")
				}

				// XXX: do we need a lock around reading obj.state?
				fromNode, exists := obj.state[ff]
				if !exists {
					panic(fmt.Sprintf("missing node state: %s", ff))
				}

				// Node we pull from should be newer epoch than us!
				if node.epoch >= fromNode.epoch {
					//if obj.Debug {
					//	obj.Logf("inner epoch skip: %p %v", f, f)
					//	//obj.Logf("inner epoch skip: NODE(%p is %d): %v FROM(%p is %d) %v", f, node.epoch, f, ff, fromNode.epoch, ff)
					//}
					// Don't set non-valid here because if
					// we have *two* FuncValue's that both
					// interrupt, the first one will happen,
					// and then the reset of the graph can
					// be updated to the current epoch, but
					// when the full graph is ready here, we
					// would skip because of this bool!
					//valid = false // don't do this!

					// The mistake in this check is that if
					// *any* of the incoming edges are not
					// ready, then we skip it all. But one
					// may not even be built yet. So it's a
					// mess. So at least for now, use the
					// below "is nil" check instead.
					//continue Iterate
				}

				value := fromNode.result
				if value == nil {
					//if valid { // must be a programming err!
					//panic(fmt.Sprintf("unexpected nil node result from: %s", ff))
					//}
					// We're reading from a node which got
					// skipped because it didn't have all of
					// its edges yet. (or a programming bug)
					continue Iterate
					// The fromNode epoch check above should
					// make this additional check redundant.
				}

				// set each arg, since one value
				// could get used for multiple
				// function inputs (shared edge)
				// XXX: refactor this edge look up for efficiency since we just did IncomingGraphVertices?
				edge := obj.graph.Adjacency()[ff][f]
				if edge == nil {
					panic(fmt.Sprintf("edge is nil from `%s` to `%s`", ff, f))
				}
				args := edge.(*interfaces.FuncEdge).Args
				for _, arg := range args {
					// Skip edge is unused at this time.
					//if arg == "" { // XXX: special skip edge!
					//	// XXX: we could maybe detect this at the incoming loop above instead
					//	continue
					//}
					// populate struct
					if err := st.Set(arg, value); err != nil {
						//panic(fmt.Sprintf("struct set failure on `%s` from `%s`: %v", node, fromNode, err))
						keys := []string{}
						for k := range st.Struct() {
							keys = append(keys, k)
						}
						panic(fmt.Sprintf("struct set failure on `%s` from `%s`: %v, has: %v", node, fromNode, err, keys))
					}
					if _, exists := need[arg]; !exists {
						keys := []string{}
						for k := range st.Struct() {
							keys = append(keys, k)
						}
						// could be either a duplicate or an unwanted field (edge name)
						panic(fmt.Sprintf("unexpected struct key `%s` on `%s` from `%s`, has(%d): %v", arg, node, fromNode, len(keys), keys))
					}
					delete(need, arg)
				}
			}
			// We just looped through all the incoming edges.

			// XXX: Can we do the above bits -> struct, and then the
			// struct -> list here, all in one faster step for perf?
			args, err := interfaces.StructToCallableArgs(st) // []types.Value, error)
			if err != nil {
				panic(fmt.Sprintf("struct to callable failure on `%s`: %v, has: %v", node, err, st))
			}

			// Call the function.
			if obj.Debug {
				obj.Logf("call: %v", f)
			}
			//node.result, err = f.Call(ctx, args)
			node.result, err = obj.call(ctx, args, f) // recovers!
			// XXX: On error lookup the fallback value if it exists.
			// XXX: This might cause an interrupt + graph addition.
			if err == interfaces.ErrInterrupt {
				// re-run topological sort... at the top!

				obj.interrupt = true // should be set in obj.effect
				continue Start
			}
			if obj.interrupt {
				// We have a function which caused an interrupt,
				// but which didn't return ErrInterrupt. This is
				// a programming error by the function.
				return fmt.Errorf("function didn't interrupt correctly: %s", node)
			}
			if err != nil {
				return err
			}
			if node.result == nil && len(obj.graph.OutgoingGraphVertices(f)) > 0 {
				// XXX: this check may not work if we have our
				// "empty" named edges added on here...
				return fmt.Errorf("unexpected nil value from node: %s", node)
			}
			old := node.epoch
			node.epoch = epoch // store it after a successful call
			if obj.Debug {
				obj.Logf("set epoch(%d) to %d: %p %v", old, epoch, f, f)
			}

			// XXX: Should we check here to see if we can shutdown?
			// For a given node, if Stream is not running, and no
			// incoming nodes are still open, and if we're Pure, and
			// we can memoize, then why not shutdown this node and
			// remove it from the graph? Run a graph interrupt to
			// delete this vertex. This will run Cleanup. Is it safe
			// to also delete the table entry? Is it needed or used?

			if node.result == nil {
				// got an end of line vertex that would normally
				// send a dummy value... don't store in table...
				continue
			}
			table[f] = node.result // build up our table of values

		} // end of single graph traversal

		if !valid { // don't send table yet, it's not complete
			continue
		}

		// Send a table of the complete set of values, which should all
		// have the same epoch, and send it as an event to the outside.
		// We need a copy of the map since we'll keep modifying it now.
		// The table must get cleaned up over time to be consistent. It
		// currently happens in interrupt as a result of a node delete.

		cp := table.Copy()
		if obj.Debug {
			obj.Logf("table:")
			for k, v := range cp {
				obj.Logf("table[%p %v]: %p %+v", k, k, v, v)
			}
		}
		select {
		case obj.streamChan <- cp:

		case <-ctx.Done():
			return ctx.Err()
		}

		// XXX: implement epoch rollover by relabelling all nodes
		epoch++ // increment it after a successful traversal
		if obj.Debug {
			obj.Logf("epoch(%d) increment to %d", epoch-1, epoch)
		}

	} // end big for loop
}

// event is ultimately called from a function to trigger an event in the engine.
// We'd like for this to never block, because that makes it much easier to
// prevent deadlocks in some tricky functions. On the other side, we don't want
// to necessarily merge events if we want to ensure each sent event gets seen in
// order. A buffered channel would accomplish this, but then it would need a
// fixed size, and if it reached the capacity we'd be in the deadlock situation
// again. Instead, we use a buffered channel of size one, and a queue of data
// which stores the event information.
func (obj *Engine) event(ctx context.Context, state *state) error {
	//f := state.Func // for reference, how to get the Vertex/Func pointer!

	select {
	case obj.ag.In <- state: // buffered to avoid blocking issues
		// tell function engine who had an event... deal with it before
		// we get to handle subsequent ones...
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// effect runs at the end of the transaction, but before it returns.
// XXX: we don't need delta ops if we can just plug into our implementations of
// the addVertex and deleteVertex ...
func (obj *Engine) effect( /*delta *DeltaOps*/ ) error {
	obj.interrupt = true

	// The toposort runs in interrupt. We could save `delta` if it's needed.
	//var err error
	//obj.topoSort, err = obj.graph.TopologicalSort()
	//return err
	return nil
}

// call is a helper to handle the recovering if needed from a function call.
// NOTE: We moved f to be the last arg because golint complains ctx isn't first.
func (obj *Engine) call(ctx context.Context, args []types.Value, f interfaces.Func) (result types.Value, reterr error) {
	defer func() {
		// catch programming errors
		if r := recover(); r != nil {
			obj.Logf("panic in process: %+v", r)
			reterr = fmt.Errorf("panic in process: %+v", r)
		}
	}()

	return f.Call(ctx, args)
}

// Stream returns a channel that you can follow to get aggregated graph events.
// Do not block reading from this channel as you can hold up the entire engine.
func (obj *Engine) Stream() <-chan interfaces.Table {
	return obj.streamChan
}

// Err will contain the last error when Stream shuts down. It waits for all the
// running processes to exit before it returns.
func (obj *Engine) Err() error {
	obj.wg.Wait()
	return obj.err
}

// Txn returns a transaction that is suitable for adding and removing from the
// graph. You must run Setup before this method is called.
func (obj *Engine) Txn() interfaces.Txn {
	if obj.refCount == nil {
		panic("you must run Setup before first use")
	}
	// The very first initial Txn must have a wait group to make sure if we
	// shutdown (in error) that we can Reverse things before the Lock/Unlock
	// loop shutsdown.
	//var free func()
	//if !obj.firstTxn {
	//	obj.firstTxn = true
	//	obj.wgTxn.Add(1)
	//	free = func() {
	//		obj.wgTxn.Done()
	//	}
	//}
	return (&txn.GraphTxn{
		Post:     obj.effect,
		Lock:     func() {}, // noop for now
		Unlock:   func() {}, // noop for now
		GraphAPI: obj,
		RefCount: obj.refCount, // reference counting
		//FreeFunc: free,
	}).Init()
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
		return fmt.Errorf("missing func info for node: %s", f)
	}
	sig := f.Info().Sig
	if sig == nil {
		return fmt.Errorf("missing func sig for node: %s", f)
	}
	if sig.Kind != types.KindFunc {
		return fmt.Errorf("kind is not func for node: %s", f)
	}
	if err := f.Validate(); err != nil {
		return errwrap.Wrapf(err, "did not Validate node: %s", f)
	}

	txn := obj.Txn()

	// This is the one of two places where we modify this map. To avoid
	// concurrent writes, we only do this when we're locked! Anywhere that
	// can read where we are locked must have a mutex around it or do the
	// lookup when we're in an unlocked state.
	node := &state{
		Func: f,
		name: f.String(), // cache a name to avoid locks

		txn: txn,

		//running: false,
		//epoch: 0,
	}

	init := &interfaces.Init{
		Hostname: obj.Hostname,
		Event: func(ctx context.Context) error {
			return obj.event(ctx, node) // pass state to avoid search
		},
		Txn:   node.txn,
		Local: obj.Local,
		World: obj.World,
		Debug: obj.Debug,
		Logf: func(format string, v ...interface{}) {
			// safe Logf in case f.String contains %? chars...
			s := f.String() + ": " + fmt.Sprintf(format, v...)
			obj.Logf("%s", s)
		},
	}

	op := &addVertex{
		f: f,
		fn: func(ctx context.Context) error {
			return f.Init(init) // TODO: should this take a ctx?
		},
	}
	obj.ops = append(obj.ops, op) // mark for cleanup during interrupt

	obj.state[f] = node // do this here b/c we rely on knowing it in real-time

	obj.graph.AddVertex(f) // Txn relies on this happening now while it runs.
	return nil
}

// AddVertex is the thread-safe way to add a vertex. You will need to call the
// engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) AddVertex(f interfaces.Func) error {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	if obj.Debug {
		obj.Logf("Engine:AddVertex: %p %s", f, f)
	}

	return obj.addVertex(f) // lockless version
}

// AddEdge is the thread-safe way to add an edge. You will need to call the
// engine Lock method before using this and the Unlock method afterwards. This
// will automatically run AddVertex on both input vertices if they are not
// already part of the graph. You should only create DAG's as this function
// engine cannot handle cycles and this method will error if you cause a cycle.
func (obj *Engine) AddEdge(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) error {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	if obj.Debug {
		obj.Logf("Engine:AddEdge %p %s: %p %s -> %p %s", fe, fe, f1, f1, f2, f2)
	}

	if obj.Debug { // not needed unless we have buggy graph building code
		// safety check to avoid cycles
		g := obj.graph.Copy()
		//g.AddVertex(f1)
		//g.AddVertex(f2)
		g.AddEdge(f1, f2, fe)
		if _, err := g.TopologicalSort(); err != nil {
			return err // not a dag
		}
		// if we didn't cycle, we can modify the real graph safely...
	}

	// Does the graph already have these nodes in it?
	//hasf1 := obj.graph.HasVertex(f1)
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
	//if hasf1 {
	//	//obj.resend[f2] = struct{}{} // resend notification to me
	//}

	obj.graph.AddEdge(f1, f2, fe) // replaces any existing edge here

	// This shouldn't error, since the test graph didn't find a cycle. But
	// we don't really need to do it, since the interrupt will run it too.
	if obj.Debug { // not needed unless we have buggy graph building code
		if _, err := obj.graph.TopologicalSort(); err != nil {
			// programming error
			panic(err) // not a dag
		}
	}

	return nil
}

// deleteVertex is the lockless version of the DeleteVertex function. This is
// needed so that AddEdge can add two vertices within the same lock. It needs
// deleteVertex so it can rollback the first one if the second addVertex fails.
func (obj *Engine) deleteVertex(f interfaces.Func) error {
	node, exists := obj.state[f]
	if !exists {
		return fmt.Errorf("vertex %p %s doesn't exist", f, f)
	}
	_ = node

	// This is the one of two places where we modify this map. To avoid
	// concurrent writes, we only do this when we're locked! Anywhere that
	// can read where we are locked must have a mutex around it or do the
	// lookup when we're in an unlocked state.

	op := &deleteVertex{
		f: f,
		fn: func(ctx context.Context) error {
			// XXX: do we run f.Done() first ? Did it run elsewhere?
			cleanableFunc, ok := f.(interfaces.CleanableFunc)
			if !ok {
				return nil
			}
			return cleanableFunc.Cleanup(ctx)
		},
	}
	obj.ops = append(obj.ops, op) // mark for cleanup during interrupt

	delete(obj.state, f) // do this here b/c we rely on knowing it in real-time

	obj.graph.DeleteVertex(f) // Txn relies on this happening now while it runs.
	return nil
}

// DeleteVertex is the thread-safe way to delete a vertex. You will need to call
// the engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) DeleteVertex(f interfaces.Func) error {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	if obj.Debug {
		obj.Logf("Engine:DeleteVertex: %p %s", f, f)
	}

	return obj.deleteVertex(f) // lockless version
}

// DeleteEdge is the thread-safe way to delete an edge. You will need to call
// the engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) DeleteEdge(fe *interfaces.FuncEdge) error {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	if obj.Debug {
		f1, f2, found := obj.graph.LookupEdge(fe)
		if found {
			obj.Logf("Engine:DeleteEdge: %p %s -> %p %s", f1, f1, f2, f2)
		} else {
			obj.Logf("Engine:DeleteEdge: not found %p %s", fe, fe)
		}
	}

	// Don't bother checking if edge exists first and don't error if it
	// doesn't because it might have gotten deleted when a vertex did, and
	// so there's no need to complain for nothing.
	obj.graph.DeleteEdge(fe)

	return nil
}

// HasVertex is the thread-safe way to check if a vertex exists in the graph.
// You will need to call the engine Lock method before using this and the Unlock
// method afterwards.
func (obj *Engine) HasVertex(f interfaces.Func) bool {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	return obj.graph.HasVertex(f)
}

// LookupEdge is the thread-safe way to check which vertices (if any) exist
// between an edge in the graph. You will need to call the engine Lock method
// before using this and the Unlock method afterwards.
func (obj *Engine) LookupEdge(fe *interfaces.FuncEdge) (interfaces.Func, interfaces.Func, bool) {
	// No mutex needed here since this func runs in a non-concurrent Txn.

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

// FindEdge is the thread-safe way to check which edge (if any) exists between
// two vertices in the graph. This is an important method in edge removal,
// because it's what you really need to know for DeleteEdge to work. Requesting
// a specific deletion isn't very sensical in this library when specified as the
// edge pointer, since we might replace it with a new edge that has new arg
// names. Instead, use this to look up what relationship you want, and then
// DeleteEdge to remove it. You will need to call the engine Lock method before
// using this and the Unlock method afterwards.
func (obj *Engine) FindEdge(f1, f2 interfaces.Func) *interfaces.FuncEdge {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	edge := obj.graph.FindEdge(f1, f2)
	if edge == nil {
		return nil
	}
	fe, ok := edge.(*interfaces.FuncEdge)
	if !ok {
		panic("edge is not a FuncEdge")
	}

	return fe
}

// Graph returns a copy of the contained graph.
func (obj *Engine) Graph() *pgraph.Graph {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	return obj.graph.Copy()
}

// ExecGraphviz writes out the diagram of a graph to be used for visualization
// and debugging. You must not modify the graph (eg: during Lock) when calling
// this method.
func (obj *Engine) ExecGraphviz(ctx context.Context, dir string) error {
	// No mutex needed here since this func runs in a non-concurrent Txn.

	// No mutex is needed at this time because we only run this in txn's and
	// it should only be run with debugging enabled. Bring your own mutex.
	//obj.graphvizMutex.Lock()
	//defer obj.graphvizMutex.Unlock()

	obj.graphvizCount++ // increment

	if dir == "" {
		dir = obj.graphvizDirectory
	}
	if dir == "" { // XXX: hack for ergonomics
		d := time.Now().UnixMilli()
		dir = fmt.Sprintf("/tmp/dage-graphviz-%s-%d/", obj.Name, d)
		obj.graphvizDirectory = dir
	}
	if !strings.HasSuffix(dir, "/") {
		return fmt.Errorf("dir must end with a slash")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	//dashedEdges, err := pgraph.NewGraph("dashedEdges")
	//if err != nil {
	//	return err
	//}
	//for _, v1 := range obj.graph.Vertices() {
	//	// if it's a ChannelBasedSinkFunc...
	//	if cb, ok := v1.(*structs.ChannelBasedSinkFunc); ok {
	//		// ...then add a dashed edge to its output
	//		dashedEdges.AddEdge(v1, cb.Target, &pgraph.SimpleEdge{
	//			Name: "channel", // secret channel
	//		})
	//	}
	//	// if it's a ChannelBasedSourceFunc...
	//	if cb, ok := v1.(*structs.ChannelBasedSourceFunc); ok {
	//		// ...then add a dashed edge from its input
	//		dashedEdges.AddEdge(cb.Source, v1, &pgraph.SimpleEdge{
	//			Name: "channel", // secret channel
	//		})
	//	}
	//}

	gv := &pgraph.Graphviz{
		Name:     obj.graph.GetName(),
		Filename: fmt.Sprintf("%s/%d.dot", dir, obj.graphvizCount),
		Graphs: map[*pgraph.Graph]*pgraph.GraphvizOpts{
			obj.graph: nil,
			//dashedEdges: {
			//	Style: "dashed",
			//},
		},
	}

	if err := gv.Exec(ctx); err != nil {
		return err
	}
	return nil
}

// errAppend is a simple helper function.
func (obj *Engine) errAppend(err error) {
	obj.errMutex.Lock()
	obj.err = errwrap.Append(obj.err, err)
	obj.errMutex.Unlock()
}

// state tracks some internal vertex-specific state information.
type state struct {
	Func interfaces.Func
	name string // cache a name here for safer concurrency

	txn interfaces.Txn // API of GraphTxn struct to pass to each function

	// started is true if this is a StreamableFunc, and Stream was started.
	started bool

	// epoch represents the "iteration count" through the graph. All values
	// in a returned table should be part of the same epoch. This guarantees
	// that they're all consistent with respect to each other.
	epoch int64 // if this rolls over, we've been running for too many years

	// result is the latest output from calling this function.
	result types.Value
}

// String implements the fmt.Stringer interface for pretty printing!
func (obj *state) String() string {
	if obj.name != "" {
		return obj.name
	}

	return obj.Func.String()
}

// ops is either an addVertex or deleteVertex operation.
type ops interface {
}

// addVertex is one of the "ops" that are possible.
type addVertex struct {
	f  interfaces.Func
	fn func(context.Context) error
}

// deleteVertex is one of the "ops" that are possible.
type deleteVertex struct {
	f  interfaces.Func
	fn func(context.Context) error
}

// realEdgeCount tells us how many "logical" edges there are. We have shared
// edges which represent more than one value, when the same value is passed more
// than once. This takes those into account correctly.
func realEdgeCount(edges []pgraph.Edge) int {
	total := 0
	for _, edge := range edges {
		fe, ok := edge.(*interfaces.FuncEdge)
		if !ok {
			total++
			continue
		}
		total += len(fe.Args) // these can represent more than one edge!
	}
	return total
}
