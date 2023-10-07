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

package interfaces

import (
	"fmt"
	"io"
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

	// TypeCheck returns the list of invariants that this node produces. It
	// does so recursively on any children elements that exist in the AST,
	// and returns the collection to the caller. It calls TypeCheck for
	// child statements, and Infer/Check for child expressions.
	TypeCheck() ([]*UnificationInvariant, error)

	// Graph returns the reactive function graph expressed by this node. It
	// takes in the environment of any functions in scope.
	Graph(env *Env) (*pgraph.Graph, error)

	// Output returns the output that this "program" produces. This output
	// is what is used to build the output graph. It requires the input
	// table of values that are used to populate each function.
	Output(map[Func]types.Value) (*Output, error)
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
	SetScope(*Scope, map[string]Expr) error

	// SetType sets the type definitively, and errors if it is incompatible.
	SetType(*types.Type) error

	// Type returns the type of this expression. It may speculate if it can
	// determine it statically. This errors if it is not yet known.
	Type() (*types.Type, error)

	// Infer returns the type of itself and a collection of invariants. The
	// returned type may contain unification variables. It collects the
	// invariants by calling Check on its children expressions. In making
	// those calls, it passes in the known type for that child to get it to
	// "Check" it. When the type is not known, it should create a new
	// unification variable to pass in to the child Check calls. Infer
	// usually only calls Check on things inside of it, and often does not
	// call another Infer.
	Infer() (*types.Type, []*UnificationInvariant, error)

	// Check is checking that the input type is equal to the object that
	// Check is running on. In doing so, it adds any invariants that are
	// necessary. Check must always call Infer to produce the invariant. The
	// implementation can be generic for all expressions.
	Check(typ *types.Type) ([]*UnificationInvariant, error)

	// TimeCheck determines whether the expression is timeless or not. What that
	// means is subtle, please refer to the documentation for the Timeless type.
	TimeCheck(env map[string]*types.Timeless) (*types.Timeless, error)

	// Graph returns the reactive function graph expressed by this node. It
	// takes in the environment of any functions in scope. It also returns
	// the function for this node.
	Graph(env *Env) (*pgraph.Graph, Func, error)

	// SetValue stores the result of the last computation of this expression
	// node.
	SetValue(types.Value) error

	// Value returns the value of this expression in our type system.
	Value() (types.Value, error)
}

// ScopeGrapher adds a method to turn an AST (Expr or Stmt) into a graph so that
// we can debug the SetScope compilation phase.
type ScopeGrapher interface {
	Node

	// ScopeGraph adds nodes and vertices to the supplied graph.
	ScopeGraph(g *pgraph.Graph)
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

	// LexParser is a function that needs to get passed in to run the lexer
	// and parser to build the initial AST. This is passed in this way to
	// avoid dependency cycles.
	LexParser func(io.Reader) (Stmt, error)

	// StrInterpolater is a function that needs to get passed in to run the
	// string interpolation. This is passed in this way to avoid dependency
	// cycles.
	StrInterpolater func(string, *Pos, *Data) (Expr, error)

	// SourceFinder is a function that returns the contents of a source file
	// when requested by filename. This data is used to annotate error
	// messages with some context from the source, and as a result is
	// optional. This function is passed in this way so that the different
	// consumers of this can use different methods to find the source. The
	// three main users are: (1) normal GAPI CLI, before the bundle is
	// created, (2) the main bundled execution, and (3) the tests.
	SourceFinder SourceFinderFunc

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

// AbsFilename returns the absolute filename path to the code this Data struct
// is running. This is used to pull out a filename for error messages.
func (obj *Data) AbsFilename() string {
	// TODO: is this correct? Do we want to check if Metadata is nil?
	if obj == nil || obj.Metadata == nil { // for tests
		return ""
	}
	return obj.Base + obj.Metadata.Main
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
	// Variables maps the scope of name to Expr.
	Variables map[string]Expr

	// Functions is the scope of functions.
	//
	// The Expr will usually be an *ExprFunc. (Actually it's usually or
	// always an *ExprSingleton, which wraps an *ExprFunc now.)
	Functions map[string]Expr

	// Classes map the name of the class to the class.
	Classes map[string]Stmt

	// Iterated is a flag that is true if this scope is inside of a for
	// loop.
	Iterated bool

	Chain []Node // chain of previously seen node's
}

// EmptyScope returns the zero, empty value for the scope, with all the internal
// lists initialized appropriately.
func EmptyScope() *Scope {
	return &Scope{
		Variables: make(map[string]Expr),
		Functions: make(map[string]Expr),
		Classes:   make(map[string]Stmt),
		Iterated:  false,
		Chain:     []Node{},
	}
}

// Copy makes a copy of the Scope struct. This ensures that if the internal map
// is changed, it doesn't affect other copies of the Scope. It does *not* copy
// or change the Expr pointers contained within, since these are references, and
// we need those to be consistently pointing to the same things after copying.
func (obj *Scope) Copy() *Scope {
	if obj == nil { // allow copying nil scopes
		return EmptyScope()
	}

	variables := make(map[string]Expr)
	functions := make(map[string]Expr)
	classes := make(map[string]Stmt)
	iterated := obj.Iterated
	chain := []Node{}

	for k, v := range obj.Variables { // copy
		variables[k] = v // we don't copy the expr's!
	}
	for k, v := range obj.Functions { // copy
		functions[k] = v // we don't copy the generator func's
	}
	for k, v := range obj.Classes { // copy
		classes[k] = v // we don't copy the StmtClass!
	}
	for _, x := range obj.Chain { // copy
		chain = append(chain, x) // we don't copy the Stmt pointer!
	}

	return &Scope{
		Variables: variables,
		Functions: functions,
		Classes:   classes,
		Iterated:  iterated,
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

	if scope.Iterated { // XXX: how should we merge this?
		obj.Iterated = scope.Iterated
	}

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
	if len(obj.Classes) > 0 {
		return false
	}
	return true
}

// Env is an environment which contains the relevant mappings. This is used at
// the Graph(...) stage of the compiler. It does not contain classes.
type Env struct {
	// Variables map and Expr to a *FuncSingleton which deduplicates the
	// use of a function.
	Variables map[Expr]*FuncSingleton

	// Functions contains the captured environment, because when we're
	// recursing into a StmtFunc which is defined inside a for loop, we can
	// use that to get the right Env.Variables map. As for the function
	// itself, it's the same in each loop iteration, therefore, we find it
	// in obj.expr of ExprCall. (Functions map[string]*Env) But actually,
	// our new version is now this:
	Functions map[Expr]*Env
}

// EmptyEnv returns the zero, empty value for the scope, with all the internal
// lists initialized appropriately.
func EmptyEnv() *Env {
	return &Env{
		Variables: make(map[Expr]*FuncSingleton),
		Functions: make(map[Expr]*Env),
	}
}

// Copy makes a copy of the Env struct. This ensures that if the internal maps
// are changed, it doesn't affect other copies of the Env. It does *not* copy or
// change the pointers contained within, since these are references, and we need
// those to be consistently pointing to the same things after copying.
func (obj *Env) Copy() *Env {
	if obj == nil { // allow copying nil envs
		return EmptyEnv()
	}

	variables := make(map[Expr]*FuncSingleton)
	functions := make(map[Expr]*Env)

	for k, v := range obj.Variables { // copy
		variables[k] = v // we don't copy the func's!
	}
	for k, v := range obj.Functions { // copy
		functions[k] = v // we don't copy the generator func's
	}

	return &Env{
		Variables: variables,
		Functions: functions,
	}
}

// FuncSingleton is a singleton system for storing a singleton func and its
// corresponding graph. You must pass in a `MakeFunc` builder method to generate
// these. The graph which is returned from this must contain that Func as a
// node.
type FuncSingleton struct {
	// MakeFunc builds and returns a Func and a graph that it must be
	// contained within.
	// XXX: Add Txn as an input arg?
	MakeFunc func() (*pgraph.Graph, Func, error)

	g *pgraph.Graph
	f Func
}

// GraphFunc returns the previously saved graph and func if they exist. If they
// do not, then it calls the MakeFunc method to get them, and saves a copy for
// next time.
// XXX: Add Txn as an input arg?
func (obj *FuncSingleton) GraphFunc() (*pgraph.Graph, Func, error) {
	// If obj.f already exists, just use that.
	if obj.f != nil { // && obj.g != nil
		return obj.g, obj.f, nil
	}

	var err error
	obj.g, obj.f, err = obj.MakeFunc() // XXX: Add Txn as an input arg?
	if err != nil {
		return nil, nil, err
	}
	if obj.g == nil {
		return nil, nil, fmt.Errorf("unexpected nil graph")
	}
	if obj.f == nil {
		return nil, nil, fmt.Errorf("unexpected nil function")
	}
	return obj.g, obj.f, nil
}

// Arg represents a name identifier for a func or class argument declaration and
// is sometimes accompanied by a type. This does not satisfy the Expr interface.
type Arg struct {
	Name string
	Type *types.Type // nil if unspecified (needs to be solved for)
}

// String returns a short representation of this arg.
func (obj *Arg) String() string {
	s := obj.Name
	if obj.Type != nil {
		s += fmt.Sprintf(" %s", obj.Type.String())
	}
	return s
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

// PositionableNode is the interface implemented by AST nodes that store their
// code position. It is implemented by node types that embed Textarea.
type PositionableNode interface {
	// IsSet returns if the position was already set with Locate already.
	IsSet() bool

	// Locate sets the position in zero-based (start line, start column, end
	// line, end column) format.
	Locate(int, int, int, int)

	// Pos returns the zero-based start line and then start column position.
	Pos() (int, int)

	// End returns the zero-based end line and then end column position.
	End() (int, int)

	// String returns a friendly representation of the positions.
	String() string
}

// TextDisplayer is a graph node that is aware of its position in the source
// code, and can emit a textual representation of that part of the source.
type TextDisplayer interface {
	// Byline returns a simple version of the error location.
	Byline() string

	// HighlightText returns a textual representation of this definition
	// for this node in source.
	HighlightText() string
}

// SourceFinderFunc is the function signature used to return the contents of a
// source file when requested by filename. This data is used to annotate error
// messages with some context from the source, and as a result is optional.
type SourceFinderFunc = func(string) ([]byte, error)
