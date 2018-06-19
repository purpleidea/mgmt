// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
)

// Stmt represents a statement node in the language. A stmt could be a resource,
// a `bind` statement, or even an `if` statement. (Different from an `if`
// expression.)
type Stmt interface {
	Init(*Data) error            // initialize the populated node and validate
	Interpolate() (Stmt, error)  // return expanded form of AST as a new AST
	SetScope(*Scope) error       // set the scope here and propagate it downwards
	Unify() ([]Invariant, error) // TODO: is this named correctly?
	Graph() (*pgraph.Graph, error)
	Output() (*Output, error)
}

// Expr represents an expression in the language. Expr implementations must have
// their method receivers implemented as pointer receivers so that they can be
// easily copied and moved around. Expr also implements pgraph.Vertex so that
// these can be stored as pointers in our graph data structure.
type Expr interface {
	pgraph.Vertex               // must implement this since we store these in our graphs
	Init(*Data) error           // initialize the populated node and validate
	Interpolate() (Expr, error) // return expanded form of AST as a new AST
	SetScope(*Scope) error      // set the scope here and propagate it downwards
	SetType(*types.Type) error  // sets the type definitively, errors if incompatible
	Type() (*types.Type, error)
	Unify() ([]Invariant, error) // TODO: is this named correctly?
	Graph() (*pgraph.Graph, error)
	Func() (Func, error) // a function that represents this reactively
	SetValue(types.Value) error
	Value() (types.Value, error)
}

// Data provides some data to the node that could be useful during its lifetime.
type Data struct {
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
// report any bugs you have written that would have been prevented by this.
type Scope struct {
	Variables map[string]Expr
	//Functions map[string]??? // TODO: do we want a separate namespace for user defined functions?
	Classes map[string]Stmt

	Chain []Stmt // chain of previously seen stmt's
}

// Empty returns the zero, empty value for the scope, with all the internal
// lists initialized appropriately.
func (obj *Scope) Empty() *Scope {
	return &Scope{
		Variables: make(map[string]Expr),
		//Functions: ???,
		Classes: make(map[string]Stmt),
		Chain:   []Stmt{},
	}
}

// Copy makes a copy of the Scope struct. This ensures that if the internal map
// is changed, it doesn't affect other copies of the Scope. It does *not* copy
// or change the Expr pointers contained within, since these are references, and
// we need those to be consistently pointing to the same things after copying.
func (obj *Scope) Copy() *Scope {
	variables := make(map[string]Expr)
	classes := make(map[string]Stmt)
	chain := []Stmt{}
	if obj != nil { // allow copying nil scopes
		for k, v := range obj.Variables { // copy
			variables[k] = v // we don't copy the expr's!
		}
		for k, v := range obj.Classes { // copy
			classes[k] = v // we don't copy the StmtClass!
		}
		for _, x := range obj.Chain { // copy
			chain = append(chain, x) // we don't copy the Stmt pointer!
		}
	}
	return &Scope{
		Variables: variables,
		Classes:   classes,
		Chain:     chain,
	}
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

// Empty returns the zero, empty value for the output, with all the internal
// lists initialized appropriately.
func (obj *Output) Empty() *Output {
	return &Output{
		Resources: []engine.Res{},
		Edges:     []*Edge{},
	}
}
