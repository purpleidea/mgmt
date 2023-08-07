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
	"fmt"
	"sort"
	"sync"

	"github.com/purpleidea/mgmt/lang/interfaces"
)

// PostReverseCommit specifies that if we run Reverse, and we had previous items
// pending for Commit, that we should Commit them after our Reverse runs.
// Otherwise they remain on the pending queue and wait for you to run Commit.
const PostReverseCommit = false

// GraphvizDebug enables writing graphviz graphs on each commit. This is very
// slow.
const GraphvizDebug = false

// opapi is the input for any op. This allows us to keeps things compact and it
// also allows us to change API slightly without re-writing code.
type opapi struct {
	GraphAPI interfaces.GraphAPI
	RefCount *RefCount
}

// opfn is an interface that holds the normal op, and the reverse op if we need
// to rollback from the forward fn. Implementations of each op can decide to
// store some internal state when running the forward op which might be needed
// for the possible future reverse op.
type opfn interface {
	fmt.Stringer

	Fn(*opapi) error
	Rev(*opapi) error
}

type opfnSkipRev interface {
	opfn

	// Skip tells us if this op should be skipped from reversing.
	Skip() bool

	// SetSkip specifies that this op should be skipped from reversing.
	SetSkip(bool)
}

type opfnFlag interface {
	opfn

	// Flag reads some misc data.
	Flag() interface{}

	// SetFlag sets some misc data.
	SetFlag(interface{})
}

// RevOp returns the reversed op from an op by packing or unpacking it.
func RevOp(op opfn) opfn {
	if skipOp, ok := op.(opfnSkipRev); ok && skipOp.Skip() {
		return nil // skip
	}

	// XXX: is the reverse of a reverse just undoing it? maybe not but might not matter for us
	if newOp, ok := op.(*opRev); ok {

		if newFlagOp, ok := op.(opfnFlag); ok {
			newFlagOp.SetFlag("does this rev of rev even happen?")
		}

		return newOp.Op // unpack it
	}

	return &opRev{
		Op: op,

		opFlag: &opFlag{},
	} // pack it
}

// opRev switches the Fn and Rev methods by wrapping the contained op in each
// other.
type opRev struct {
	Op opfn

	*opFlag
}

func (obj *opRev) Fn(opapi *opapi) error {
	return obj.Op.Rev(opapi)
}

func (obj *opRev) Rev(opapi *opapi) error {
	return obj.Op.Fn(opapi)
}

func (obj *opRev) String() string {
	return "rev(" + obj.Op.String() + ")" // TODO: is this correct?
}

type opSkip struct {
	skip bool
}

func (obj *opSkip) Skip() bool {
	return obj.skip
}

func (obj *opSkip) SetSkip(skip bool) {
	obj.skip = skip
}

type opFlag struct {
	flag interface{}
}

func (obj *opFlag) Flag() interface{} {
	return obj.flag
}

func (obj *opFlag) SetFlag(flag interface{}) {
	obj.flag = flag
}

type opAddVertex struct {
	F interfaces.Func

	*opSkip
	*opFlag
}

func (obj *opAddVertex) Fn(opapi *opapi) error {
	if opapi.RefCount.VertexInc(obj.F) {
		// add if we're the first reference
		return opapi.GraphAPI.AddVertex(obj.F)
	}

	return nil
}

func (obj *opAddVertex) Rev(opapi *opapi) error {
	opapi.RefCount.VertexDec(obj.F)
	// any removal happens in gc
	return nil
}

func (obj *opAddVertex) String() string {
	return fmt.Sprintf("AddVertex: %+v", obj.F)
}

type opAddEdge struct {
	F1 interfaces.Func
	F2 interfaces.Func
	FE *interfaces.FuncEdge

	*opSkip
	*opFlag
}

func (obj *opAddEdge) Fn(opapi *opapi) error {
	if obj.F1 == obj.F2 { // simplify below code/logic with this easy check
		return fmt.Errorf("duplicate vertex cycle")
	}

	opapi.RefCount.EdgeInc(obj.F1, obj.F2, obj.FE)

	fe := obj.FE // squish multiple edges together if one already exists
	if edge := opapi.GraphAPI.FindEdge(obj.F1, obj.F2); edge != nil {
		args := make(map[string]struct{})
		for _, x := range obj.FE.Args {
			args[x] = struct{}{}
		}
		for _, x := range edge.Args {
			args[x] = struct{}{}
		}
		if len(args) != len(obj.FE.Args)+len(edge.Args) {
			// programming error
			return fmt.Errorf("duplicate arg found")
		}
		newArgs := []string{}
		for x := range args {
			newArgs = append(newArgs, x)
		}
		sort.Strings(newArgs) // for consistency?
		fe = &interfaces.FuncEdge{
			Args: newArgs,
		}
	}

	// The dage API currently smooshes together any existing edge args with
	// our new edge arg names. It also adds the vertices if needed.
	if err := opapi.GraphAPI.AddEdge(obj.F1, obj.F2, fe); err != nil {
		return err
	}

	return nil
}

func (obj *opAddEdge) Rev(opapi *opapi) error {
	opapi.RefCount.EdgeDec(obj.F1, obj.F2, obj.FE)
	return nil
}

func (obj *opAddEdge) String() string {
	return fmt.Sprintf("AddEdge: %+v -> %+v (%+v)", obj.F1, obj.F2, obj.FE)
}

type opDeleteVertex struct {
	F interfaces.Func

	*opSkip
	*opFlag
}

func (obj *opDeleteVertex) Fn(opapi *opapi) error {
	if opapi.RefCount.VertexDec(obj.F) {
		//delete(opapi.RefCount.Vertices, obj.F)    // don't GC this one
		if err := opapi.RefCount.FreeVertex(obj.F); err != nil {
			panic("could not free vertex")
		}
		return opapi.GraphAPI.DeleteVertex(obj.F) // do it here instead
	}
	return nil
}

func (obj *opDeleteVertex) Rev(opapi *opapi) error {
	if opapi.RefCount.VertexInc(obj.F) {
		return opapi.GraphAPI.AddVertex(obj.F)
	}
	return nil
}

func (obj *opDeleteVertex) String() string {
	return fmt.Sprintf("DeleteVertex: %+v", obj.F)
}

// graphTxn holds the state of a transaction and runs it when needed. When this
// has been setup and initialized, it implements the Txn API that can be used by
// functions in their Stream method to modify the function graph while it is
// "running".
type graphTxn struct {
	// Lock is a handle to the lock function to call before the operation.
	Lock func()

	// Unlock is a handle to the unlock function to call before the
	// operation.
	Unlock func()

	// GraphAPI is a handle pointing to the graph API implementation we're
	// using for any txn operations.
	GraphAPI interfaces.GraphAPI

	// RefCount keeps track of vertex and edge references across the entire
	// graph.
	RefCount *RefCount

	// FreeFunc is a function that will get called by a well-behaved user
	// when we're done with this Txn.
	FreeFunc func()

	// ops is a list of operations to run on a graph
	ops []opfn

	// rev is a list of reverse operations to run on a graph
	rev []opfn

	// mutex guards changes to the ops list
	mutex *sync.Mutex
}

// init must be called to initialized the struct before first use. This is
// private because the creator, not the user should run it.
func (obj *graphTxn) init() interfaces.Txn {
	obj.ops = []opfn{}
	obj.rev = []opfn{}
	obj.mutex = &sync.Mutex{}

	return obj // return self so it can be called in a chain
}

// Copy returns a new child Txn that has the same handles, but a separate state.
// This allows you to do an Add*/Commit/Reverse that isn't affected by a
// different user of this transaction.
// TODO: FreeFunc isn't well supported here. Replace or remove this entirely?
func (obj *graphTxn) Copy() interfaces.Txn {
	txn := &graphTxn{
		Lock:     obj.Lock,
		Unlock:   obj.Unlock,
		GraphAPI: obj.GraphAPI,
		RefCount: obj.RefCount, // this is shared across all txn's
		// FreeFunc is shared with the parent.
	}
	return txn.init()
}

// AddVertex adds a vertex to the running graph. The operation will get
// completed when Commit is run.
// XXX: should this be pgraph.Vertex instead of interfaces.Func ?
func (obj *graphTxn) AddVertex(f interfaces.Func) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	opfn := &opAddVertex{
		F: f,

		opSkip: &opSkip{},
		opFlag: &opFlag{},
	}
	obj.ops = append(obj.ops, opfn)

	return obj // return self so it can be called in a chain
}

// AddEdge adds an edge to the running graph. The operation will get completed
// when Commit is run.
// XXX: should this be pgraph.Vertex instead of interfaces.Func ?
// XXX: should this be pgraph.Edge instead of *interfaces.FuncEdge ?
func (obj *graphTxn) AddEdge(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	opfn := &opAddEdge{
		F1: f1,
		F2: f2,
		FE: fe,

		opSkip: &opSkip{},
		opFlag: &opFlag{},
	}
	obj.ops = append(obj.ops, opfn)

	// NOTE: we can't build obj.rev yet because in this case, we'd need to
	// know if the runtime graph contained one of the two pre-existing
	// vertices or not, or if it would get added implicitly by this op!

	return obj // return self so it can be called in a chain
}

// DeleteVertex adds a vertex to the running graph. The operation will get
// completed when Commit is run.
// XXX: should this be pgraph.Vertex instead of interfaces.Func ?
func (obj *graphTxn) DeleteVertex(f interfaces.Func) interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	opfn := &opDeleteVertex{
		F: f,

		opSkip: &opSkip{},
		opFlag: &opFlag{},
	}
	obj.ops = append(obj.ops, opfn)

	return obj // return self so it can be called in a chain
}

// AddReverse appends to the commit queue anything that was staged for reverse.
// This also removes those operations from the reverse queue as if you had
// called Erase. Of note, these operations will not get used on subsequent calls
// to Reverse or AddReverse if either are called. The operation will get
// completed when Commit is run.
func (obj *graphTxn) AddReverse() interfaces.Txn {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	for _, op := range obj.rev { // copy in the rev stuff to commit!
		// mark these as being not reversable (so skip them on reverse!)
		if skipOp, ok := op.(opfnSkipRev); ok {
			skipOp.SetSkip(true)
		}
		obj.ops = append(obj.ops, op)
	}

	obj.rev = []opfn{} // clear

	return obj // return self so it can be called in a chain
}

// commit runs the pending transaction. This is the lockless version that is
// only used internally.
func (obj *graphTxn) commit() error {
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

	// Now request the ref count mutex. This may seem redundant, but it's
	// not. The above graph engine Lock might allow more than one commit
	// through simultaneously depending on implementation. The actual count
	// mathematics must not, and so it has a separate lock. We could lock it
	// per-operation, but that would probably be a lot slower.
	obj.RefCount.Lock()
	defer obj.RefCount.Unlock()

	// TODO: we don't need to do this anymore, because the engine does it!
	// Copy the graph structure, perform the ops, check we didn't add a
	// cycle, and if it's safe, do the real thing. Otherwise error here.
	//g := obj.Graph.Copy() // copy the graph structure
	//for _, x := range obj.ops {
	//	x(g) // call it
	//}
	//if _, err := g.TopologicalSort(); err != nil {
	//	return errwrap.Wrapf(err, "topo sort failed in txn commit")
	//}
	// FIXME: is there anything else we should check? Should we type-check?

	// Now do it for real...
	obj.rev = []opfn{} // clear it for safety
	opapi := &opapi{
		GraphAPI: obj.GraphAPI,
		RefCount: obj.RefCount,
	}
	for _, op := range obj.ops {
		if err := op.Fn(opapi); err != nil { // call it
			// something went wrong (we made a cycle?)
			obj.rev = []opfn{} // clear it, we didn't succeed
			return err
		}

		op = RevOp(op) // reverse the op!
		if op != nil {
			obj.rev = append(obj.rev, op) // add the reverse op
			//obj.rev = append([]opfn{op}, obj.rev...) // add to front
		}
	}
	obj.ops = []opfn{} // clear it

	// garbage collect anything that hit zero!
	// XXX: add gc function to this struct and pass in opapi instead?
	if err := obj.RefCount.GC(obj.GraphAPI); err != nil {
		// programming error or ghosts
		return err
	}

	// XXX: running this on each commit has a huge performance hit.
	// XXX: we could write out the .dot files and run graphviz afterwards
	if engine, ok := obj.GraphAPI.(*Engine); ok && GraphvizDebug {
		//d := time.Now().Unix()
		//if err := engine.graph.ExecGraphviz(fmt.Sprintf("/tmp/txn-graphviz-%d.dot", d)); err != nil {
		//	panic("no graphviz")
		//}
		if err := engine.Graphviz(""); err != nil {
			panic(err) // XXX
		}

		//gv := &pgraph.Graphviz{
		//	Filename: fmt.Sprintf("/tmp/txn-graphviz-%d.dot", d),
		//	Graphs: map[*pgraph.Graph]*pgraph.GraphvizOpts{
		//		engine.graph: nil,
		//	},
		//}
		//if err := gv.Exec(); err != nil {
		//	panic("no graphviz")
		//}
	}
	return nil
}

// Commit runs the pending transaction. If there was a pending reverse
// transaction that could have run (it would have been available following a
// Commit success) then this will erase that transaction. Usually you run cycles
// of Commit, followed by Reverse, or only Commit. (You obviously have to
// populate operations before the Commit is run.)
func (obj *graphTxn) Commit() error {
	// Lock our internal state mutex first... this prevents other AddVertex
	// or similar calls from interferring with our work here.
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	return obj.commit()
}

// Clear erases any pending transactions that weren't committed yet.
func (obj *graphTxn) Clear() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	obj.ops = []opfn{} // clear it
}

// Reverse is like Commit, but it commits the reverse transaction to the one
// that previously ran with Commit. If the PostReverseCommit global has been set
// then if there were pending commit operations when this was run, then they are
// run at the end of a successful Reverse. It is generally recommended to not
// queue any operations for Commit if you plan on doing a Reverse, or to run a
// Clear before running Reverse if you want to discard the pending commits.
func (obj *graphTxn) Reverse() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	// first commit all the rev stuff... and then run the pending ops...

	ops := []opfn{}              // save a copy
	for _, op := range obj.ops { // copy
		ops = append(ops, op)
	}
	obj.ops = []opfn{} // clear

	//for _, op := range obj.rev
	for i := len(obj.rev) - 1; i >= 0; i-- { // copy in the rev stuff to commit!
		op := obj.rev[i]
		// mark these as being not reversable (so skip them on reverse!)
		if skipOp, ok := op.(opfnSkipRev); ok {
			skipOp.SetSkip(true)
		}
		obj.ops = append(obj.ops, op)
	}

	//rev := []func(interfaces.GraphAPI){} // for the copy
	//for _, op := range obj.rev { // copy
	//	rev = append(rev, op)
	//}
	obj.rev = []opfn{} // clear

	//rollback := func() {
	//	//for _, op := range rev { // from our safer copy
	//	//for _, op := range obj.ops { // copy back out the rev stuff
	//	for i := len(obj.ops) - 1; i >= 0; i-- { // copy in the rev stuff to commit!
	//		op := obj.rev[i]
	//		obj.rev = append(obj.rev, op)
	//	}
	//	obj.ops = []opfn{}       // clear
	//	for _, op := range ops { // copy the original ops back in
	//		obj.ops = append(obj.ops, op)
	//	}
	//}
	// first commit the reverse stuff
	if err := obj.commit(); err != nil { // lockless version
		// restore obj.rev and obj.ops
		//rollback() // probably not needed
		return err
	}

	// then if we had normal ops queued up, run those or at least restore...
	for _, op := range ops { // copy
		obj.ops = append(obj.ops, op)
	}

	if PostReverseCommit {
		return obj.commit() // lockless version
	}

	return nil
}

// Erase removes the historical information that Reverse would run after Commit.
func (obj *graphTxn) Erase() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	obj.rev = []opfn{} // clear it
}

// Free releases the wait group that was used to lock around this Txn if needed.
// It should get called when we're done with any Txn.
// TODO: this is only used for the initial Txn. Consider expanding it's use. We
// might need to allow Clear to call it as part of the clearing.
func (obj *graphTxn) Free() {
	if obj.FreeFunc != nil {
		obj.FreeFunc()
	}
}
