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

package interfaces

import (
	"fmt"
	"sort"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Node represents either a Stmt or an Expr. It contains the minimum set of
// methods that they must both implement. In practice it is not used especially
// often since we usually know which kind of node we want.
type Node interface {
	//fmt.Stringer // already provided by pgraph.Vertex
	pgraph.Vertex // must implement this since we store these in our graphs

	// Apply is a general purpose iterator method that operates on any node.
	Apply(fn func(Node) error) error

	//Parent() Node // TODO: should we implement this?
}

// Stmt represents a statement node in the language. A stmt could be a resource,
// a `bind` statement, or even an `if` statement. (Different from an `if`
// expression.)
type Stmt interface {
	Node

	// Init initializes the populated node and does some basic validation.
	Init(*Data) error

	// Interpolate returns an expanded form of the AST as a new AST. It does
	// a recursive interpolate (copy) of all members in the AST.
	Interpolate() (Stmt, error) // return expanded form of AST as a new AST

	// Copy returns a light copy of the struct. Anything static will not be
	// copied. For a full recursive copy consider using Interpolate instead.
	// TODO: do we need an error in the signature?
	Copy() (Stmt, error)

	// Ordering returns a graph of the scope ordering that represents the
	// data flow. This can be used in SetScope so that it knows the correct
	// order to run it in.
	Ordering(map[string]Node) (*pgraph.Graph, map[Node]string, error)

	// SetScope sets the scope here and propagates it downwards.
	SetScope(*Scope) error

	// Unify returns the list of invariants that this node produces. It does
	// so recursively on any children elements that exist in the AST, and
	// returns the collection to the caller.
	Unify() ([]Invariant, error)

	// Graph returns the reactive function graph expressed by this node.
	Graph() (*pgraph.Graph, error)

	// Output returns the output that this "program" produces. This output
	// is what is used to build the output graph.
	Output() (*Output, error)
}

// Expr represents an expression in the language. Expr implementations must have
// their method receivers implemented as pointer receivers so that they can be
// easily copied and moved around. Expr also implements pgraph.Vertex so that
// these can be stored as pointers in our graph data structure.
type Expr interface {
	Node

	// Init initializes the populated node and does some basic validation.
	Init(*Data) error

	// Interpolate returns an expanded form of the AST as a new AST. It does
	// a recursive interpolate (copy) of all members in the AST. For a light
	// copy use Copy.
	Interpolate() (Expr, error)

	// Copy returns a light copy of the struct. Anything static will not be
	// copied. For a full recursive copy consider using Interpolate instead.
	// TODO: do we need an error in the signature?
	Copy() (Expr, error)

	// Ordering returns a graph of the scope ordering that represents the
	// data flow. This can be used in SetScope so that it knows the correct
	// order to run it in.
	Ordering(map[string]Node) (*pgraph.Graph, map[Node]string, error)

	// SetScope sets the scope here and propagates it downwards.
	SetScope(*Scope) error

	// SetType sets the type definitively, and errors if it is incompatible.
	SetType(*types.Type) error

	// Type returns the type of this expression. It may speculate if it can
	// determine it statically. This errors if it is not yet known.
	Type() (*types.Type, error)

	// Unify returns the list of invariants that this node produces. It does
	// so recursively on any children elements that exist in the AST, and
	// returns the collection to the caller.
	Unify() ([]Invariant, error)

	// Graph returns the reactive function graph expressed by this node.
	Graph() (*pgraph.Graph, error)

	// Func returns a function that represents this reactively.
	Func() (Func, error)

	// SetValue stores the result of the last computation of this expression
	// node.
	SetValue(types.Value) error

	// Value returns the value of this expression in our type system.
	Value() (types.Value, error)
}

// Data provides some data to the node that could be useful during its lifetime.
type Data struct {
	// Fs represents a handle to the filesystem that we're running on. This
	// is necessary for opening files if needed by import statements. The
	// file() paths used to get templates or other files from our deploys
	// come from here, this is *not* used to interact with the host file
	// system to manage file resources or other aspects.
	Fs engine.Fs

	// FsURI is the fs URI of the active filesystem. This is useful to pass
	// to the engine.World API for further consumption.
	FsURI string

	// Base directory (absolute path) that the running code is in. If an
	// import is found, that's a recursive addition, and naturally for that
	// run, this value would be different in the recursion.
	Base string

	// Files is a list of absolute paths seen so far. This includes all
	// previously seen paths, where as the former Offsets parameter did not.
	Files []string

	// Imports stores a graph inside a vertex so we have a current cursor.
	// This means that as we recurse through our import graph (hopefully a
	// DAG) we can know what the parent vertex in our graph is to edge to.
	// If we ever can't topologically sort it, then it has an import loop.
	Imports *pgraph.SelfVertex

	// Metadata is the metadata structure associated with the given parsing.
	// It can be present, which is often the case when importing a module,
	// or it can be nil, which is often the case when parsing a single file.
	// When imports are nested (eg: an imported module imports another one)
	// the metadata structure can recursively point to an earlier structure.
	Metadata *Metadata

	// Modules is an absolute path to a modules directory on the current Fs.
	// It is the directory to use to look for remote modules if we haven't
	// specified an alternative with the metadata Path field. This is
	// usually initialized with the global modules path that can come from
	// the cli or an environment variable, but this only occurs for the
	// initial download/get operation, and obviously not once we're running
	// a deploy, since by then everything in here would have been copied to
	// the runtime fs.
	Modules string

	// Downloader is the interface that must be fulfilled to download
	// modules. If a missing import is found, and this is not nil, then it
	// will be run once in an attempt to get the missing module before it
	// fails outright. In practice, it is recommended to separate this
	// download phase in a separate step from the production running and
	// deploys, however that is not blocked at the level of this interface.
	Downloader Downloader

	//World engine.World // TODO: do we need this?

	// Prefix provides a unique path prefix that we can namespace in. It is
	// currently shared identically across the whole AST. Nodes should be
	// careful to not write on top of other nodes data.
	Prefix string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// Scope represents a mapping between a variables identifier and the
// corresponding expression it is bound to. Local scopes in this language exist
// and are formed by nesting within if statements. Child scopes can shadow
// variables in parent scopes, which is another way of saying they can redefine
// previously used variables as long as the new binding happens within a child
// scope. This is useful so that someone in the top scope can't prevent a child
// module from ever using that variable name again. It might be worth revisiting
// this point in the future if we find it adds even greater code safety. Please
// report any bugs you have written that would have been prevented by this. This
// also contains the currently available functions. They function similarly to
// the variables, and you can add new ones with a function statement definition.
// An interesting note about these is that they exist in a distinct namespace
// from the variables, which could actually contain lambda functions.
type Scope struct {
	Variables map[string]Expr
	Functions map[string]Expr // the Expr will usually be an *ExprFunc
	Classes   map[string]Stmt
	// TODO: It is easier to shift a list, but let's use a map for Indexes
	// for now in case we ever need holes...
	Indexes map[int][]Expr // TODO: use [][]Expr instead?

	Chain []Node // chain of previously seen node's
}

// EmptyScope returns the zero, empty value for the scope, with all the internal
// lists initialized appropriately.
func EmptyScope() *Scope {
	return &Scope{
		Variables: make(map[string]Expr),
		Functions: make(map[string]Expr),
		Classes:   make(map[string]Stmt),
		Indexes:   make(map[int][]Expr),
		Chain:     []Node{},
	}
}

// InitScope initializes any uninitialized part of the struct. It is safe to use
// on scopes with existing data.
func (obj *Scope) InitScope() {
	if obj.Variables == nil {
		obj.Variables = make(map[string]Expr)
	}
	if obj.Functions == nil {
		obj.Functions = make(map[string]Expr)
	}
	if obj.Classes == nil {
		obj.Classes = make(map[string]Stmt)
	}
	if obj.Indexes == nil {
		obj.Indexes = make(map[int][]Expr)
	}
	if obj.Chain == nil {
		obj.Chain = []Node{}
	}
}

// Copy makes a copy of the Scope struct. This ensures that if the internal map
// is changed, it doesn't affect other copies of the Scope. It does *not* copy
// or change the Expr pointers contained within, since these are references, and
// we need those to be consistently pointing to the same things after copying.
func (obj *Scope) Copy() *Scope {
	variables := make(map[string]Expr)
	functions := make(map[string]Expr)
	classes := make(map[string]Stmt)
	indexes := make(map[int][]Expr)
	chain := []Node{}
	if obj != nil { // allow copying nil scopes
		obj.InitScope()                   // safety
		for k, v := range obj.Variables { // copy
			variables[k] = v // we don't copy the expr's!
		}
		for k, v := range obj.Functions { // copy
			functions[k] = v // we don't copy the generator func's
		}
		for k, v := range obj.Classes { // copy
			classes[k] = v // we don't copy the StmtClass!
		}
		for k, v := range obj.Indexes { // copy
			ixs := []Expr{}
			for _, x := range v {
				ixs = append(ixs, x) // we don't copy the expr's!
			}
			indexes[k] = ixs
		}
		for _, x := range obj.Chain { // copy
			chain = append(chain, x) // we don't copy the Stmt pointer!
		}
	}
	return &Scope{
		Variables: variables,
		Functions: functions,
		Classes:   classes,
		Indexes:   indexes,
		Chain:     chain,
	}
}

// Merge takes an existing scope and merges a scope on top of it. If any
// elements had to be overwritten, then the error result will contain some info.
// Even if this errors, the scope will have been merged successfully. The merge
// runs in a deterministic order so that errors will be consistent. Use Copy if
// you don't want to change this destructively.
// FIXME: this doesn't currently merge Chain's... Should it?
func (obj *Scope) Merge(scope *Scope) error {
	var err error
	// collect names so we can iterate in a deterministic order
	namedVariables := []string{}
	namedFunctions := []string{}
	namedClasses := []string{}
	for name := range scope.Variables {
		namedVariables = append(namedVariables, name)
	}
	for name := range scope.Functions {
		namedFunctions = append(namedFunctions, name)
	}
	for name := range scope.Classes {
		namedClasses = append(namedClasses, name)
	}
	sort.Strings(namedVariables)
	sort.Strings(namedFunctions)
	sort.Strings(namedClasses)

	obj.InitScope() // safety

	for _, name := range namedVariables {
		if _, exists := obj.Variables[name]; exists {
			e := fmt.Errorf("variable `%s` was overwritten", name)
			err = errwrap.Append(err, e)
		}
		obj.Variables[name] = scope.Variables[name]
	}
	for _, name := range namedFunctions {
		if _, exists := obj.Functions[name]; exists {
			e := fmt.Errorf("function `%s` was overwritten", name)
			err = errwrap.Append(err, e)
		}
		obj.Functions[name] = scope.Functions[name]
	}
	for _, name := range namedClasses {
		if _, exists := obj.Classes[name]; exists {
			e := fmt.Errorf("class `%s` was overwritten", name)
			err = errwrap.Append(err, e)
		}
		obj.Classes[name] = scope.Classes[name]
	}

	// FIXME: should we merge or overwrite? (I think this isn't even used)
	obj.Indexes = scope.Indexes // overwrite without error

	return err
}

// IsEmpty returns whether or not a scope is empty or not.
// FIXME: this doesn't currently consider Chain's... Should it?
func (obj *Scope) IsEmpty() bool {
	//if obj == nil { // TODO: add me if this turns out to be useful
	//	return true
	//}
	if len(obj.Variables) > 0 {
		return false
	}
	if len(obj.Functions) > 0 {
		return false
	}
	if len(obj.Indexes) > 0 { // FIXME: should we check each one? (unused?)
		return false
	}
	if len(obj.Classes) > 0 {
		return false
	}
	return true
}

// MaxIndexes returns the maximum index of Indexes stored in the scope. If it is
// empty then -1 is returned.
func (obj *Scope) MaxIndexes() int {
	obj.InitScope() // safety
	max := -1
	for k := range obj.Indexes {
		if k > max {
			max = k
		}
	}
	return max
}

// PushIndexes adds a list of expressions at the zeroth index in Indexes after
// firsh pushing everyone else over by one. If you pass in nil input this may
// panic!
func (obj *Scope) PushIndexes(exprs []Expr) {
	if exprs == nil {
		// TODO: is this the right thing to do?
		panic("unexpected nil input")
	}
	obj.InitScope() // safety
	max := obj.MaxIndexes()
	for i := max; i >= 0; i-- { // reverse order
		indexes, exists := obj.Indexes[i]
		if !exists {
			continue
		}
		delete(obj.Indexes, i)
		obj.Indexes[i+1] = indexes // push it
	}

	if obj.Indexes == nil { // in case we weren't initialized yet
		obj.Indexes = make(map[int][]Expr)
	}
	obj.Indexes[0] = exprs // usually the list of Args in ExprCall
}

// PullIndexes takes a list of expressions from the zeroth index in Indexes and
// then pulls everyone over by one. The returned value is only valid if one was
// found at the zeroth index. The returned boolean will be true if it exists.
func (obj *Scope) PullIndexes() ([]Expr, bool) {
	obj.InitScope()         // safety
	if obj.Indexes == nil { // in case we weren't initialized yet
		obj.Indexes = make(map[int][]Expr)
	}

	indexes, exists := obj.Indexes[0] // save for later

	max := obj.MaxIndexes()
	for i := 0; i <= max; i++ {
		ixs, exists := obj.Indexes[i]
		if !exists {
			continue
		}
		delete(obj.Indexes, i)
		if i == 0 { // zero falls off
			continue
		}
		obj.Indexes[i-1] = ixs
	}

	return indexes, exists
}

// Edge is the data structure representing a compiled edge that is used in the
// lang to express a dependency between two resources and optionally send/recv.
type Edge struct {
	Kind1 string // kind of resource
	Name1 string // name of resource
	Send  string // name of field used for send/recv (optional)

	Kind2 string // kind of resource
	Name2 string // name of resource
	Recv  string // name of field used for send/recv (optional)

	Notify bool // is there a notification being sent?
}

// Output is a collection of data returned by a Stmt.
type Output struct { // returned by Stmt
	Resources []engine.Res
	Edges     []*Edge
	//Exported []*Exports // TODO: add exported resources
}

// EmptyOutput returns the zero, empty value for the output, with all the
// internal lists initialized appropriately.
func EmptyOutput() *Output {
	return &Output{
		Resources: []engine.Res{},
		Edges:     []*Edge{},
	}
}
