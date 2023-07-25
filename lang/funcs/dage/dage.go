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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
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

	graph *pgraph.Graph                   // guarded by graphMutex
	table map[interfaces.Func]types.Value // guarded by tableMutex
	state map[interfaces.Func]*state

	// graphMutex wraps access to the table map.
	graphMutex *sync.Mutex // TODO: &sync.RWMutex{} ?

	// tableMutex wraps access to the table map.
	tableMutex *sync.RWMutex

	// rwmutex wraps any read or write access to the graph or state fields.
	rwmutex *sync.RWMutex
	wg      *sync.WaitGroup

	// pause/resume state machine signals
	pauseChan   chan struct{}
	pausedChan  chan struct{}
	resumeChan  chan struct{}
	resumedChan chan struct{}

	// resend tracks which new nodes might need a new notification
	resend map[interfaces.Func]struct{}

	streamChan chan error

	loaded bool // are all of the funcs loaded?
	//loadedChan chan struct{} // funcs loaded signal

	startedChan chan struct{} // closes when Run() starts

	// wakeChan contains a message when someone has asked for us to wake up.
	wakeChan chan struct{}

	// stats holds some statistics and other debugging information.
	stats *stats // guarded by statsMutex

	// statsMutex wraps access to the stats data.
	statsMutex *sync.RWMutex

	// graphvizMutex wraps access to the Graphviz method.
	graphvizMutex *sync.Mutex

	// graphvizCount keeps a running tally of how many graphs we've
	// generated. This is useful for displaying a sequence (timeline) of
	// graphs in a linear order.
	graphvizCount int64

	// graphvizDirectory stores the generated path for outputting graphviz
	// files if one is not specified at runtime.
	graphvizDirectory string
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
	obj.graphMutex = &sync.Mutex{} // TODO: &sync.RWMutex{} ?
	obj.tableMutex = &sync.RWMutex{}

	obj.rwmutex = &sync.RWMutex{}
	obj.wg = &sync.WaitGroup{}

	obj.pauseChan = make(chan struct{})
	obj.pausedChan = make(chan struct{})
	obj.resumeChan = make(chan struct{})
	obj.resumedChan = make(chan struct{})

	obj.resend = make(map[interfaces.Func]struct{})

	obj.streamChan = make(chan error)
	//obj.loadedChan = make(chan struct{}) // TODO: currently not used
	obj.startedChan = make(chan struct{})

	obj.wakeChan = make(chan struct{}, 1) // hold up to one message

	obj.stats = &stats{
		runningList: make(map[*state]struct{}),
	}
	obj.statsMutex = &sync.RWMutex{}

	obj.graphvizMutex = &sync.Mutex{}
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
		name: f.String(), // cache a name to avoid locks

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
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
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
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		obj.Logf("Engine:AddEdge %p %s: %p %s -> %p %s", fe, fe, f1, f1, f2, f2)
	}

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
		return fmt.Errorf("vertex %p %s doesn't exist", f, f)
	}

	if node.running {
		// cancel the running vertex
		node.cancel() // cancel inner ctx
		node.wg.Wait()
		//if node.notify != nil { // if sig.Ord == 0, we didn't make it!
		//	close(node.notify) // after node.wg.Wait() finishes, we're done
		//}
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
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		obj.Logf("Engine:DeleteVertex: %p %s", f, f)
	}

	return obj.deleteVertex(f) // lockless version
}

// DeleteEdge is the thread-safe way to delete an edge. You will need to call
// the engine Lock method before using this and the Unlock method afterwards.
func (obj *Engine) DeleteEdge(e *interfaces.FuncEdge) error {
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		f1, f2, found := obj.graph.LookupEdge(e)
		if found {
			obj.Logf("Engine:DeleteEdge: %p %s -> %p %s", f1, f1, f2, f2)
		} else {
			obj.Logf("Engine:DeleteEdge: not found %p %s", e, e)
		}
	}

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
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()

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
	obj.graphMutex.Lock()         // XXX: should this be a RLock?
	defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

	return obj.graph.HasVertex(f)
}

// LookupEdge is the thread-safe way to check which vertices (if any) exist
// between an edge in the graph. You will need to call the engine Lock method
// before using this and the Unlock method afterwards.
func (obj *Engine) LookupEdge(fe *interfaces.FuncEdge) (interfaces.Func, interfaces.Func, bool) {
	obj.graphMutex.Lock()         // XXX: should this be a RLock?
	defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

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
// XXX: should Lock take a context if we want to bail mid-way?
// TODO: could we replace pauseChan with SubscribedSignal ?
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
	obj.graphMutex.Unlock() // safe to make changes now
}

// Unlock must be used after modifying the running graph. Make sure to Lock
// beforehand.
// XXX: should Unlock take a context if we want to bail mid-way?
func (obj *Engine) Unlock() { // resume
	// this mutex locks at start of Run() and unlocks at finish of Run()
	obj.graphMutex.Lock() // no more changes are allowed
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

// wake sends a message to the wake queue to wake up the main process function
// which would otherwise spin unnecessarily. This can be called anytime, and
// doesn't hurt, it only wastes cpu if there's nothing to do. This does NOT ever
// block, and that's important so it can be called from anywhere.
func (obj *Engine) wake() {
	// The mutex guards the len check to avoid this function sending two
	// messages down the channel, because the second would block if the
	// consumer isn't fast enough. This mutex makes this method effectively
	// asynchronous.
	//obj.wakeMutex.Lock()
	//defer obj.wakeMutex.Unlock()
	//if len(obj.wakeChan) > 0 { // collapse duplicate, pending wake signals
	//	return
	//}
	select {
	case obj.wakeChan <- struct{}{}: // send to chan of length 1
		obj.Logf("wake sent")
	default: // this is a cheap alternative to avoid the mutex altogether!
		// skip sending, we already have a message pending!
	}
}

// process is the inner loop that runs through the entire graph. It can be
// called successively safely, as it is roughly idempotent, and is used to push
// values through the graph. If it is interrupted, it can pick up where it left
// off on the next run. This does however require it to re-check some things,
// but that is the price we pay for being always available to unblock.
// Importantly, re-running this resumes work in progress even if there was
// caching, and that if interrupted, it'll be queued again so as to not drop a
// wakeChan notification!
func (obj *Engine) process(ctx context.Context) (reterr error) {
	defer func() {
		// catch programming errors
		if r := recover(); r != nil {
			obj.Logf("Panic in process: %+v", r)
			reterr = fmt.Errorf("panic in process: %+v", r)
		}
	}()

	// Toposort in dependency order.
	topoSort, err := obj.graph.TopologicalSort()
	if err != nil {
		return err
	}

	loaded := true // assume we emitted at least one value for now...

	outDegree := obj.graph.OutDegree() // map[Vertex]int

	for _, v := range topoSort {
		f, ok := v.(interfaces.Func)
		if !ok {
			panic("not a Func")
		}
		node, exists := obj.state[f]
		if !exists {
			panic(fmt.Sprintf("missing node in iterate: %s", f))
		}

		out, exists := outDegree[f]
		if !exists {
			panic(fmt.Sprintf("missing out degree in iterate: %s", f))
		}

		// XXX: this nodeLoaded stuff is crap
		node.rwmutex.RLock()
		nodeLoaded := node.loaded
		node.rwmutex.RUnlock()
		if !nodeLoaded {
			loaded = false // we were wrong
		}

		// XXX: memoize this which we can do easily now since graph shape doesn't change in this loop!
		incoming := obj.graph.IncomingGraphVertices(f) // []pgraph.Vertex

		//outgoing := obj.graph.OutgoingGraphVertices(f) // []pgraph.Vertex
		//node.isLeaf = len(outgoing) == 0

		node.rwmutex.Lock()
		node.isLeaf = out == 0
		node.rwmutex.Unlock()

		// no incoming edges, so no incoming data
		if len(incoming) == 0 { // we do this below
			if !node.closed {
				node.closed = true
				close(node.input)
			}
			continue
		} // else, process input data below...

		ready := true  // assume all input values are ready for now...
		closed := true // assume all inputs have closed for now...
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
			obj.tableMutex.RLock()
			value, exists := obj.table[ff]
			obj.tableMutex.RUnlock()
			if !exists {
				ready = false  // nope!
				closed = false // can't be, it's not even ready yet
				break
			}
			// XXX: do we need a lock around reading obj.state?
			fromNode, exists := obj.state[ff]
			if !exists {
				panic(fmt.Sprintf("missing node in notify: %s", ff))
			}
			if !fromNode.closed {
				closed = false // if any still open, then we are
			}

			// set each arg, since one value
			// could get used for multiple
			// function inputs (shared edge)
			args := obj.graph.Adjacency()[ff][f].(*interfaces.FuncEdge).Args
			for _, arg := range args {
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
					panic(fmt.Sprintf("unexpected struct key on `%s` from `%s`: %v, has: %v", node, fromNode, err, keys))
				}
				delete(need, arg)
			}
		}

		if !ready || len(need) != 0 {
			//return nil // not ready yet
			break
		}

		// previously it was closed, skip sending
		if node.closed {
			continue
		}

		// XXX: respect the info.Pure and info.Memo fields somewhere here...

		// XXX: keep track of some state about who i sent to last before
		// being interrupted so that I can avoid resending to some nodes
		// if it's not necessary...

		// It's critical to avoid deadlock with this sending select that
		// any events that could happen during this send can be
		// preempted and that future executions of this function can be
		// resumed. We must return with an error to let folks know that
		// we were interrupted.
		obj.Logf("send to func `%s`", node)
		select {
		case node.input <- st: // send to function
			// pass
		case <-node.ctx.Done(): // node died
			obj.wake() // interrupted, so queue again
			// XXX: can this happen now and should we continue or err?
			return node.ctx.Err()
			// continue // probably best to return and come finish later
		case <-ctx.Done():
			obj.wake() // interrupted, so queue again
			return ctx.Err()
		}

		// It's okay if this section gets preempted and we re-run this
		// function. The worst that happens is we end up sending the
		// same input data a second time. This means that we could in
		// theory be causing unnecessary graph changes (and locks which
		// cause preemption here) if nodes that cause locks aren't
		// skipping duplicate/identical input values!
		if closed && !node.closed {
			node.closed = true
			close(node.input)
		}

		// XXX now we need to somehow wait to make sure that node has the time to send at least one output... no we don't or do we?
		// XXX: we could add a counter to each input that gets passed through the function... Eg: if we pass in 4, we should wait until
		// a 4 comes out the output side. But we'd need to change the signature of func for this... Wait for now.

	} // end topoSort loop

	// It's okay if this section gets preempted and we re-run this bit here.
	obj.loaded = loaded // this gets reset when graph adds new nodes

	return nil
}

// Run kicks off the main engine. This takes a mutex. When we're "paused" the
// mutex is temporarily released until we "resume". Those operations transition
// with the engine Lock and Unlock methods. It is recommended to only add
// vertices to the engine after it's running. If you add them before Run, then
// Run will cause a Lock/Unlock to occur to cycle them in. Lock and Unlock race
// with the cancellation of this Run main loop. Make sure to only call one at a
// time.
func (obj *Engine) Run(ctx context.Context) (reterr error) {
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()

	// XXX: can the above defer get called while we are already unlocked?
	// XXX: is it a possibility if we use <-Started() ?

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	defer func() {
		// catch programming errors
		if r := recover(); r != nil {
			obj.Logf("Panic in Run: %+v", r)
			reterr = fmt.Errorf("panic in Run: %+v", r)
		}
	}()

	ctx, cancel := context.WithCancel(ctx) // wrap parent
	defer cancel()

	// Add a wait before the "started" signal runs so that Cleanup waits.
	obj.wg.Add(1)
	defer obj.wg.Done()

	// Send the start signal.
	close(obj.startedChan)

	if n := obj.graph.NumVertices(); n > 0 { // hack to make the api easier
		obj.Logf("graph contained %d vertices before Run", n)
		wg.Add(1)
		go func() {
			defer wg.Done()
			// kick the engine once to pull in any vertices from
			// before we started running!
			defer obj.Unlock()
			obj.Lock()
		}()
	}

	// Aggregation channel and wait group.
	ag := make(chan error)
	wgAg := &sync.WaitGroup{}

	// close the aggregate channel when everyone is done with it...
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
		}
		// don't wait and close ag before we're really done with Run()
		wgAg.Wait() // wait for last ag user to close
		close(ag)   // last one closes the ag channel
	}()

	// aggregate events channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(obj.streamChan)
		for {
			var err error
			var ok bool
			select {
			case err, ok = <-ag: // aggregated channel
				if !ok {
					return // channel shutdown
				}
			}

			// XXX: check obj.loaded first?

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

	// We need to keep the main loop running until everyone else has shut
	// down. When the top context closes, we wait for everyone to finish,
	// and then we shut down this main context.
	//mainCtx, mainCancel := context.WithCancel(ctx) // wrap parent
	mainCtx, mainCancel := context.WithCancel(context.Background()) // DON'T wrap parent, close on your own terms
	defer mainCancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
		}

		// XXX RENAME wgAg because of its use here
		wgAg.Wait() // wait until all the routines have closed
		mainCancel()
	}()

	wgFn := &sync.WaitGroup{} // wg for process function runner
	defer wgFn.Wait()         // extra safety

	// XXX XXX XXX
	//go func() { // XXX: debugging to make sure we didn't forget to wake someone...
	//	for {
	//		obj.wake() // new value, so send wake up
	//		time.Sleep(3 * time.Second)
	//	}
	//}()
	// XXX XXX XXX

	// we start off "running", but we'll have an empty graph initially...
	for {

		// After we've resumed, we can try to exit. (shortcut)
		// NOTE: If someone calls Lock(), which would send to
		// obj.pauseChan, it *won't* deadlock here because mainCtx is
		// only closed when all the worker waitgroups close first!
		select {
		case <-mainCtx.Done(): // when asked to exit
			return nil // we exit happily
		default:
		}

		// run through our graph, check for pause request occasionally
		for {
			// Start the process run for this iteration of the loop.
			ctxFn, cancelFn := context.WithCancel(context.Background())
			// we run cancelFn() below to cleanup!
			var errFn error
			chanFn := make(chan struct{}) // normal exit signal
			wgFn.Add(1)
			go func() {
				defer wgFn.Done()
				defer close(chanFn) // signal that I exited
				for {
					obj.Logf("process...")
					if errFn = obj.process(ctxFn); errFn != nil { // store
						obj.Logf("process end err: %+v...", errFn)
						return
					}
					obj.Logf("process end...")
					// If process finishes without error, we
					// should sit here and wait until we get
					// run again from a wake-up, or we exit.
					select {
					case <-obj.wakeChan: // wait until something has actually woken up...
						obj.Logf("process wakeup...")
						// loop!
					case <-ctxFn.Done():
						errFn = context.Canceled
						return
					}
				}
			}()

			chPause := false
			ctxExit := false
			select {
			//case <-obj.wakeChan:
			// this happens entirely in the process inner, inner loop now.

			case <-chanFn: // process exited on it's own in error!
				// pass

			case <-obj.pauseChan:
				obj.Logf("pausing...")
				chPause = true

			case <-mainCtx.Done(): // when asked to exit
				//return nil // we exit happily
				ctxExit = true
			}

			cancelFn()  // cancel the process function
			wgFn.Wait() // wait for the process function to return
			if errFn == nil {
				return fmt.Errorf("unexpected nil error in process")
			}
			if errFn != nil && errFn != context.Canceled {
				return errwrap.Wrapf(errFn, "process error")
			}
			//if errFn == context.Canceled {
			//	// ignore, we asked for it
			//}

			if ctxExit {
				return nil // we exit happily
			}
			if chPause {
				break
			}
			// programming error
			return fmt.Errorf("unhandled process state")
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

			innerCtx, innerCancel := context.WithCancel(ctx) // wrap parent (not mainCtx)
			// we defer innerCancel() in the goroutine to cleanup!
			node.ctx = innerCtx
			node.cancel = innerCancel

			// run mainloop
			wgAg.Add(1)
			node.wg.Add(1)
			go func(f interfaces.Func, node *state) {
				defer node.wg.Done()
				defer wgAg.Done()
				defer node.cancel() // if we close, clean up and send the signal to anyone watching
				if obj.Debug {
					obj.Logf("Running func `%s`", node)
					obj.statsMutex.Lock()
					obj.stats.runningList[node] = struct{}{}
					obj.statsMutex.Unlock()
				}

				fn := func(nodeCtx context.Context) (reterr error) {
					defer func() {
						// catch programming errors
						if r := recover(); r != nil {
							obj.Logf("Panic in Stream of func `%s`: %+v", node, r)
							reterr = fmt.Errorf("panic in Stream of func `%s`: %+v", node, r)
						}
					}()
					return f.Stream(nodeCtx)
				}
				runErr := fn(node.ctx) // wrap with recover()
				if obj.Debug {
					obj.Logf("Exiting func `%s`", node)
					obj.statsMutex.Lock()
					delete(obj.stats.runningList, node)
					obj.statsMutex.Unlock()
				}
				if runErr != nil {
					// send to a aggregate channel
					// the first to error will cause ag to
					// shutdown, so make sure we can exit...
					select {
					case ag <- runErr: // send to aggregate channel
					case <-node.ctx.Done():
					}
				}
				// if node never loaded, then we error in the node.output loop!
			}(f, node)

			// consume output
			wgAg.Add(1)
			node.wg.Add(1)
			go func(f interfaces.Func, node *state) {
				defer node.wg.Done()
				defer wgAg.Done()

				for value := range node.output { // read from channel
					if value == nil {
						// bug!
						obj.Logf("func `%s` got nil value", node)
						panic("got nil value")
					}

					obj.tableMutex.RLock()
					cached, exists := obj.table[f]
					obj.tableMutex.RUnlock()
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
					obj.tableMutex.Lock()
					obj.table[f] = value // save the latest
					obj.tableMutex.Unlock()
					node.rwmutex.Lock()
					node.loaded = true // set *after* value is in :)
					//obj.Logf("func `%s` changed", node)
					node.rwmutex.Unlock()
					obj.wake() // new value, so send wake up

					// XXX: I think we need this read lock
					// because we don't want to be adding a
					// new vertex here but then missing to
					// send an event to it because it
					// started after we did the range...

					node.rwmutex.RLock()
					isLeaf := node.isLeaf
					node.rwmutex.RUnlock()

					// TODO: if shutdown, did we still want to do this?
					if obj.Glitch || isLeaf {
						select {
						case ag <- nil: // send to aggregate channel
						case <-node.ctx.Done():
							//return
						}
					}

				} // end for

				// no more output values are coming...
				//obj.Logf("func `%s` stopped", node)

				// XXX shouldn't we record the fact that obj.output closed, and close
				// the downstream node's obj.input?

				// nodes that never loaded will cause the engine to hang
				if !node.loaded {
					select {
					case ag <- fmt.Errorf("func `%s` stopped before it was loaded", node):
					case <-node.ctx.Done():
						return
					}
				}

			}(f, node)

		} // end for

		// Send new notifications in case any new edges are sending away
		// to these... They might have already missed the notifications!
		for k := range obj.resend { // resend TO these!
			//obj.Graphviz("") // XXX DEBUG
			node, exists := obj.state[k]
			if !exists {
				continue
			}
			// Run as a goroutine to avoid erroring in parent thread.
			wg.Add(1)
			go func(node *state) {
				defer wg.Done()
				obj.Logf("resend to func `%s`", node)
				obj.wake() // new value, so send wake up
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
	obj.tableMutex.RLock()
	defer obj.tableMutex.RUnlock()
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
	obj.tableMutex.Lock() // differs from above RLock around obj.table
	defer obj.tableMutex.Unlock()
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

// Return some statistics in a human-readable form.
func (obj *Engine) Stats() string {
	defer obj.statsMutex.RUnlock()
	obj.statsMutex.RLock()

	return obj.stats.String()
}

// Graphviz writes out the diagram of a graph to be used for visualization and
// debugging. You must not modify the graph (eg: during Lock) when calling this
// method.
func (obj *Engine) Graphviz(dir string) error {
	// XXX: would this deadlock if we added this?
	//obj.graphMutex.Lock()         // XXX: should this be a RLock?
	//defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

	obj.graphvizMutex.Lock()
	defer obj.graphvizMutex.Unlock()

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

	dashedEdges, err := pgraph.NewGraph("dashedEdges")
	if err != nil {
		return err
	}
	for _, v1 := range obj.graph.Vertices() {
		// if it's a ChannelBasedSinkFunc...
		if cb, ok := v1.(*simple.ChannelBasedSinkFunc); ok {
			// ...then add a dashed edge to its output
			dashedEdges.AddEdge(v1, cb.Target, &pgraph.SimpleEdge{
				Name: "channel", // secret channel
			})
		}
		// if it's a ChannelBasedSourceFunc...
		if cb, ok := v1.(*simple.ChannelBasedSourceFunc); ok {
			// ...then add a dashed edge from its input
			dashedEdges.AddEdge(cb.Source, v1, &pgraph.SimpleEdge{
				Name: "channel", // secret channel
			})
		}
	}

	gv := &pgraph.Graphviz{
		Name:     obj.graph.GetName(),
		Filename: fmt.Sprintf("%s/%d.dot", dir, obj.graphvizCount),
		Graphs: map[*pgraph.Graph]*pgraph.GraphvizOpts{
			obj.graph: nil,
			dashedEdges: {
				Style: "dashed",
			},
		},
	}

	if err := gv.Exec(); err != nil {
		return err
	}
	return nil
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
	closed bool // is our input closed?

	isLeaf bool // is my out degree zero?

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

// stats holds some statistics and other debugging information.
type stats struct {

	// runningList keeps track of which nodes are still running.
	runningList map[*state]struct{}
}

func (obj *stats) String() string {
	s := "stats:\n"
	s += "\trunningList:\n"
	names := []string{}
	for k := range obj.runningList {
		names = append(names, k.String())
	}
	sort.Strings(names)
	for _, name := range names {
		s += fmt.Sprintf("\t * %s\n", name)
	}
	return s
}
