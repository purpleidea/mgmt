// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package interfaces

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
)

// FuncSig is the simple signature that is used throughout our implementations.
type FuncSig = func([]types.Value) (types.Value, error)

// Info is a static representation of some information about the function. It is
// used for static analysis and type checking. If you break this contract, you
// might cause a panic.
type Info struct {
	Pure bool        // is the function pure? (can it be memoized?)
	Memo bool        // should the function be memoized? (false if too much output)
	Slow bool        // is the function slow? (avoid speculative execution)
	Sig  *types.Type // the signature of the function, must be KindFunc
	Err  error       // is this a valid function, or was it created improperly?
}

// Init is the structure of values and references which is passed into all
// functions on initialization.
type Init struct {
	Hostname string // uuid for the host
	//Noop bool

	// Input is where a chan (stream) of values will get sent to this node.
	// The engine will close this `input` chan.
	Input chan types.Value

	// Output is the chan (stream) of values to get sent out from this node.
	// The Stream function must close this `output` chan.
	Output chan types.Value

	// Txn provides a transaction API that can be used to modify the
	// function graph while it is "running". This should not be used by most
	// nodes, and when it is used, it should be used carefully.
	Txn Txn

	// TODO: should we pass in a *Scope here for functions like template() ?

	Local *local.API
	World engine.World

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Func is the interface that any valid func must fulfill. It is very simple,
// but still event driven. Funcs should attempt to only send values when they
// have changed.
// TODO: should we support a static version of this interface for funcs that
// never change to avoid the overhead of the goroutine and channel listener?
type Func interface {
	fmt.Stringer // so that this can be stored as a Vertex

	Validate() error // FIXME: this is only needed for PolyFunc. Get it moved and used!

	// Info returns some information about the function in question, which
	// includes the function signature. For a polymorphic function, this
	// might not be known until after Build was called. As a result, the
	// sig should be allowed to return a partial or variant type if it is
	// not known yet. This is because the Info method might be called
	// speculatively to aid in type unification.
	Info() *Info

	// Init passes some important values and references to the function.
	Init(*Init) error

	// Stream is the mainloop of the function. It reads and writes from
	// channels to return the changing values that this func has over time.
	// It should shutdown and cleanup when the input context is cancelled.
	// It must not exit before any goroutines it spawned have terminated.
	// It must close the Output chan if it's done sending new values out. It
	// must send at least one value, or return an error. It may also return
	// an error at anytime if it can't continue.
	Stream(context.Context) error
}

// PolyFunc is an interface for functions which are statically polymorphic. In
// other words, they are functions which before compile time are polymorphic,
// but after a successful compilation have a fixed static signature. This makes
// implementing what would appear to be generic or polymorphic instead something
// that is actually static and that still has the language safety properties.
// Our engine requires that by the end of compilation, everything is static.
// This is needed so that values can flow safely along the DAG that represents
// their execution. If the types could change, then we wouldn't be able to
// safely pass values around.
//
// NOTE: This interface is similar to OldPolyFunc, except that it uses a Unify
// method that works differently than the original Polymorphisms method. This
// allows us to build invariants that are used directly by the type unification
// solver.
type PolyFunc interface {
	Func // implement everything in Func but add the additional requirements

	// Unify returns the list of invariants that this func produces. It is a
	// way for a polymorphic function to describe its type requirements. It
	// would be expected for this function to return at least one
	// ExclusiveInvariant or GeneratorInvariant, since these are two common
	// mechanisms for polymorphic functions to describe their constraints.
	// The important realization behind this method is that the collecting
	// of possible invariants, must happen *before* the solver runs so that
	// the solver can look at all the available logic *simultaneously* to
	// find a solution if we want to be able to reliably solve for things.
	// The input argument that it receives is the expression pointer that it
	// is unifying against-- in other words, the pointer is its own handle.
	// This is different than the `obj` reference of this function
	// implementation because _that_ handle is not the object/pointer in the
	// AST that we're discussing when performing type unification. Put
	// another way: the Expr input is the ExprFunc, not the ExprCall.
	Unify(Expr) ([]Invariant, error)

	// Build takes the known or unified type signature for this function and
	// finalizes this structure so that it is now determined, and ready to
	// function as a normal function would. (The normal methods in the Func
	// interface are all that should be needed or used after this point.)
	// Of note, the names of the specific input args shouldn't matter as
	// long as they are unique. Their position doesn't matter. This is so
	// that unification can use "arg0", "arg1", "argN"... if they can't be
	// determined statically. Build can transform them into it's desired
	// form, and must return the type (with the correct arg names) that it
	// will use. These are used when constructing the function graphs. This
	// means that when this is called from SetType, it can set the correct
	// type arg names, and this will also match what's in function Info().
	Build(*types.Type) (*types.Type, error)
}

// OldPolyFunc is an interface for functions which are statically polymorphic.
// In other words, they are functions which before compile time are polymorphic,
// but after a successful compilation have a fixed static signature. This makes
// implementing what would appear to be generic or polymorphic instead something
// that is actually static and that still has the language safety properties.
type OldPolyFunc interface {
	Func // implement everything in Func but add the additional requirements

	// Polymorphisms returns a list of possible function type signatures. It
	// takes as input a list of partial "hints" as to limit the number of
	// possible results it returns. These partial hints take the form of a
	// function type signature (with as many types in it specified and the
	// rest set to nil) and any known static values for the input args. If
	// the partial type is not nil, then the Ord parameter must be of the
	// correct arg length. If any types are specified, then the array must
	// be of that length as well, with the known ones filled in. Some
	// static polymorphic functions require a minimal amount of hinting or
	// they will be unable to return any possible result that is not
	// infinite in length. If you expect to need to return an infinite (or
	// very large) amount of results, then you should return an error
	// instead. The arg names in your returned func type signatures should
	// be in the standardized "a..b..c" format. Use util.NumToAlpha if you
	// want to convert easily.
	Polymorphisms(*types.Type, []types.Value) ([]*types.Type, error)

	// Build takes the known or unified type signature for this function and
	// finalizes this structure so that it is now determined, and ready to
	// function as a normal function would. (The normal methods in the Func
	// interface are all that should be needed or used after this point.)
	// Of note, the names of the specific input args shouldn't matter as
	// long as they are unique. Their position doesn't matter. This is so
	// that unification can use "arg0", "arg1", "argN"... if they can't be
	// determined statically. Build can transform them into it's desired
	// form, and must return the type (with the correct arg names) that it
	// will use. These are used when constructing the function graphs. This
	// means that when this is called from SetType, it can set the correct
	// type arg names, and this will also match what's in function Info().
	Build(*types.Type) (*types.Type, error)
}

// CallableFunc is a function that can be called statically if we want to do it
// speculatively or from a resource.
type CallableFunc interface {
	Func // implement everything in Func but add the additional requirements

	// Call this function with the input args and return the value if it is
	// possible to do so at this time. To transform from the single value,
	// graph representation of the callable values into a linear, standard
	// args list for use here, you can use the StructToCallableArgs
	// function.
	Call(args []types.Value) (types.Value, error)
}

// NamedArgsFunc is a function that uses non-standard function arg names. If you
// don't implement this, then the argnames (if specified) must correspond to the
// a, b, c...z, aa, ab...az, ba...bz, and so on sequence.
type NamedArgsFunc interface {
	Func // implement everything in Func but add the additional requirements

	// ArgGen implements the arg name generator function. By default, we use
	// the util.NumToAlpha function when this interface isn't implemented...
	ArgGen(int) (string, error)
}

// FuncData is some data that is passed into the function during compilation. It
// helps provide some context about the AST and the deploy for functions that
// might need it.
// TODO: Consider combining this with the existing Data struct or more of it...
// TODO: Do we want to add line/col/file values here, and generalize this?
type FuncData struct {
	// Fs represents a handle to the filesystem that we're running on. This
	// is necessary for opening files if needed by import statements. The
	// file() paths used to get templates or other files from our deploys
	// come from here, this is *not* used to interact with the host file
	// system to manage file resources or other aspects.
	Fs engine.Fs

	// FsURI is the fs URI of the active filesystem. This is useful to pass
	// to the engine.World API for further consumption.
	FsURI string

	// Base directory (absolute path) that the running code is in. This is a
	// copy of the value from the Expr and Stmt Data struct for Init.
	Base string
}

// DataFunc is a function that accepts some context from the AST and deploy
// before Init and runtime. If you don't wish to accept this data, then don't
// implement this method and you won't get any. This is mostly useful for
// special functions that are useful in core.
// TODO: This could be replaced if a func ever needs a SetScope method...
type DataFunc interface {
	Func // implement everything in Func but add the additional requirements

	// SetData is used by the language to pass our function some code-level
	// context.
	SetData(*FuncData)
}

// FuncEdge links an output vertex (value) to an input vertex with a named
// argument.
type FuncEdge struct {
	Args []string // list of named args that this edge sends to
}

// String displays the list of arguments this edge satisfies. It is a required
// property to be a valid pgraph.Edge.
func (obj *FuncEdge) String() string {
	return strings.Join(obj.Args, ", ")
}

// GraphAPI is a subset of the available graph operations that are possible on a
// pgraph that is used for storing functions. The minimum subset are those which
// are needed for implementing the Txn interface.
type GraphAPI interface {
	AddVertex(Func) error
	AddEdge(Func, Func, *FuncEdge) error
	DeleteVertex(Func) error
	DeleteEdge(*FuncEdge) error
	//AddGraph(*pgraph.Graph) error

	//Adjacency() map[Func]map[Func]*FuncEdge
	HasVertex(Func) bool
	FindEdge(Func, Func) *FuncEdge
	LookupEdge(*FuncEdge) (Func, Func, bool)

	// Graph returns a copy of the current graph.
	Graph() *pgraph.Graph
}

// Txn is the interface that the engine graph API makes available so that
// functions can modify the function graph dynamically while it is "running".
// This could be implemented in one of two methods.
//
// Method 1: Have a pair of graph Lock and Unlock methods. Queue up the work to
// do and when we "commit" the transaction, we're just queuing up the work to do
// and then we run it all surrounded by the lock.
//
// Method 2: It's possible that we might eventually be able to actually modify
// the running graph without even causing it to pause at all. In this scenario,
// the "commit" would just directly perform those operations without even using
// the Lock and Unlock mutex operations. This is why we don't expose those in
// the API. It's also safer because someone can't forget to run Unlock which
// would block the whole code base.
type Txn interface {
	// AddVertex adds a vertex to the running graph. The operation will get
	// completed when Commit is run.
	AddVertex(Func) Txn

	// AddEdge adds an edge to the running graph. The operation will get
	// completed when Commit is run.
	AddEdge(Func, Func, *FuncEdge) Txn

	// DeleteVertex removes a vertex from the running graph. The operation
	// will get completed when Commit is run.
	DeleteVertex(Func) Txn

	// DeleteEdge removes an edge from the running graph. It removes the
	// edge that is found between the two input vertices. The operation will
	// get completed when Commit is run. The edge is part of the signature
	// so that it is both symmetrical with AddEdge, and also easier to
	// reverse in theory.
	// NOTE: This is not supported since there's no sane Reverse with GC.
	// XXX: Add this in but just don't let it be reversible?
	//DeleteEdge(Func, Func, *FuncEdge) Txn

	// AddGraph adds a graph to the running graph. The operation will get
	// completed when Commit is run. This function panics if your graph
	// contains vertices that are not of type interfaces.Func or if your
	// edges are not of type *interfaces.FuncEdge.
	AddGraph(*pgraph.Graph) Txn

	// Commit runs the pending transaction.
	Commit() error

	// Clear erases any pending transactions that weren't committed yet.
	Clear()

	// Reverse runs the reverse commit of the last successful operation to
	// Commit. AddVertex is reversed by DeleteVertex, and vice-versa, and
	// the same for AddEdge and DeleteEdge. Keep in mind that if AddEdge is
	// called with either vertex not already part of the graph, it will
	// implicitly add them, but the Reverse operation will not necessarily
	// know that. As a result, it's recommended to not perform operations
	// that have implicit Adds or Deletes. Notwithstanding the above, the
	// initial Txn implementation can and does try to track these changes
	// so that it can correctly reverse them, but this is not guaranteed by
	// API, and it could contain bugs.
	Reverse() error

	// Erase removes the historical information that Reverse would run after
	// Commit.
	Erase()

	// Free releases the wait group that was used to lock around this Txn if
	// needed. It should get called when we're done with any Txn.
	Free()

	// Copy returns a new child Txn that has the same handles, but a
	// separate state. This allows you to do an Add*/Commit/Reverse that
	// isn't affected by a different user of this transaction.
	Copy() Txn

	// Graph returns a copy of the graph. It returns what has been already
	// committed.
	Graph() *pgraph.Graph
}

// StructToCallableArgs transforms the single value, graph representation of the
// callable values into a linear, standard args list.
func StructToCallableArgs(st types.Value) ([]types.Value, error) {
	if st == nil {
		return nil, fmt.Errorf("empty struct")
	}
	typ := st.Type()
	if typ == nil {
		return nil, fmt.Errorf("empty type")
	}
	if kind := typ.Kind; kind != types.KindStruct {
		return nil, fmt.Errorf("incorrect kind, got: %s", kind)
	}
	structValues := st.Struct() // map[string]types.Value
	if structValues == nil {
		return nil, fmt.Errorf("empty values")
	}

	args := []types.Value{}
	for i, x := range typ.Ord { // in the correct order
		v, exists := structValues[x]
		if !exists {
			return nil, fmt.Errorf("invalid input value at %d", i)
		}

		args = append(args, v)
	}
	return args, nil
}
