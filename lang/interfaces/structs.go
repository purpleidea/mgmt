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

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// ExprAny is a placeholder expression that is used for type unification hacks.
type ExprAny struct {
	typ *types.Type
}

// String returns a short representation of this expression.
func (obj *ExprAny) String() string { return "any" }

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprAny) Apply(fn func(Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprAny) Init(*Data) error { return nil }

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it simply returns itself, as no interpolation is possible.
func (obj *ExprAny) Interpolate() (Expr, error) {
	return &ExprAny{
		typ: obj.typ,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprAny) Copy() (Expr, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprAny) Ordering(produces map[string]Node) (*pgraph.Graph, map[Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "could not create graph")
	}
	graph.AddVertex(obj)

	cons := make(map[Node]string)
	return graph, cons, nil
}

// SetScope does nothing for this struct, because it has no child nodes, and it
// does not need to know about the parent scope.
func (obj *ExprAny) SetScope(*Scope) error { return nil }

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprAny) SetType(typ *types.Type) error {
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprAny) Type() (*types.Type, error) {
	if obj.typ == nil {
		return nil, ErrTypeCurrentlyUnknown
	}
	return obj.typ, nil
}

// Unify returns the list of invariants that this node produces. It recursively
// calls Unify on any children elements that exist in the AST, and returns the
// collection to the caller.
func (obj *ExprAny) Unify() ([]Invariant, error) {
	invariants := []Invariant{
		&AnyInvariant{ // it has to be something, anything!
			Expr: obj,
		},
	}
	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it, and
// the edges from all of the child graphs to this.
func (obj *ExprAny) Graph() (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("any")
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not create graph")
	}
	graph.AddVertex(obj)
	return graph, nil
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprAny) Func() (Func, error) {
	return nil, fmt.Errorf("programming error") // this should not be called
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child elements (the list elements) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprAny) SetValue(value types.Value) error {
	return fmt.Errorf("programming error") // this should not be called
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprAny) Value() (types.Value, error) {
	return nil, fmt.Errorf("programming error") // this should not be called
}
