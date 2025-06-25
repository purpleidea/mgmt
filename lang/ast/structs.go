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

// Package ast contains the structs implementing and some utility functions for
// interacting with the abstract syntax tree for the mcl language.
package ast

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/core"
	"github.com/purpleidea/mgmt/lang/embedded"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/ref"
	"github.com/purpleidea/mgmt/lang/funcs/structs"
	"github.com/purpleidea/mgmt/lang/funcs/txn"
	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/types/full"
	unificationUtil "github.com/purpleidea/mgmt/lang/unification/util"
	langUtil "github.com/purpleidea/mgmt/lang/util"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"golang.org/x/time/rate"
)

const (
	// EdgeNotify declares an edge a -> b, such that a notification occurs.
	// This is most similar to "notify" in Puppet.
	EdgeNotify = "notify"

	// EdgeBefore declares an edge a -> b, such that no notification occurs.
	// This is most similar to "before" in Puppet.
	EdgeBefore = "before"

	// EdgeListen declares an edge a <- b, such that a notification occurs.
	// This is most similar to "subscribe" in Puppet.
	EdgeListen = "listen"

	// EdgeDepend declares an edge a <- b, such that no notification occurs.
	// This is most similar to "require" in Puppet.
	EdgeDepend = "depend"

	// MetaField is the prefix used to specify a meta parameter for the res.
	MetaField = "meta"

	// AllowBareClassIncluding specifies that a simple include without an
	// `as` suffix, will be pulled in under the name of the included class.
	// We want this on if it turns out to be common to pull in values from
	// classes.
	//
	// If we allow bare including of classes, then we have to also prevent
	// duplicate class inclusion for many cases. For example:
	//
	//	class c1($s) {
	//		test $s {}
	//		$x = "${s}"
	//	}
	//	include c1("hey")
	//	include c1("there")
	//	test $x {}
	//
	// What value should $x have? We want to import two useful `test`
	// resources, but with a bare import this makes `$x` ambiguous. We'd
	// have to detect this and ensure this is a compile time error to use
	// it. Being able to allow compatible, duplicate classes is a key
	// important feature of the language, and as a result, enabling this
	// would probably be disastrous. The fact that the import statement
	// allows bare imports is an ergonomic consideration that is allowed
	// because duplicate imports aren't essential. As an aside, the use of
	// bare imports isn't recommended because it makes it more difficult to
	// know where certain things are coming from.
	AllowBareClassIncluding = false

	// AllowBareIncludes specifies that you're allowed to use an include
	// which flattens the included scope on top of the current scope. This
	// means includes of the form: `include foo as *`. These are unlikely to
	// get enabled for many reasons.
	AllowBareIncludes = false

	// AllowBareImports specifies that you're allowed to use an import which
	// flattens the imported scope on top of the current scope. This means
	// imports of the form: `import foo as *`. These are being provisionally
	// enabled, despite being less explicit and harder to parse.
	AllowBareImports = true

	// AllowUserDefinedPolyFunc specifies if we allow user-defined
	// polymorphic functions or not. At the moment this is not implemented.
	// XXX: not implemented
	AllowUserDefinedPolyFunc = false

	// RequireStrictModulePath can be set to true if you wish to ignore any
	// of the metadata parent path searching. By default that is allowed,
	// unless it is disabled per module with ParentPathBlock. This option is
	// here in case we decide that the parent module searching is confusing.
	RequireStrictModulePath = false

	// RequireTopologicalOrdering specifies if the code *must* be written in
	// a topologically correct order. This prevents "out-of-order" code that
	// is valid, but possibly confusing to the read. The main author
	// (purpleidea) believes that this is better of as false. This is
	// because occasionally code might be more logical when out-of-order,
	// and hiding the fundamental structure of the language isn't elegant.
	RequireTopologicalOrdering = false

	// TopologicalOrderingWarning specifies whether a warning is emitted if
	// the code is not in a topologically correct order. If this warning is
	// seen too often, then we should consider disabling this by default.
	TopologicalOrderingWarning = true

	// varOrderingPrefix is a magic prefix used for the Ordering graph.
	varOrderingPrefix = "var:"

	// paramOrderingPrefix is a magic prefix used for the Ordering graph.
	paramOrderingPrefix = "param:"

	// funcOrderingPrefix is a magic prefix used for the Ordering graph.
	funcOrderingPrefix = "func:"

	// classOrderingPrefix is a magic prefix used for the Ordering graph.
	classOrderingPrefix = "class:"

	// scopedOrderingPrefix is a magic prefix used for the Ordering graph.
	// It is shared between imports and include as.
	scopedOrderingPrefix = "scoped:"

	// ErrNoStoredScope is an error that tells us we can't get a scope here.
	ErrNoStoredScope = util.Error("scope is not stored in this node")

	// ErrFuncPointerNil is an error that explains the function pointer for
	// table lookup is missing. If this happens, it's most likely a
	// programming error.
	ErrFuncPointerNil = util.Error("missing func pointer for table")

	// ErrTableNoValue is an error that explains the table is missing a
	// value. If this happens, it's most likely a programming error.
	ErrTableNoValue = util.Error("missing value in table")
)

var (
	// orderingGraphSingleton is used for debugging the ordering graph.
	orderingGraphSingleton = false
)

// StmtBind is a representation of an assignment, which binds a variable to an
// expression.
type StmtBind struct {
	interfaces.Textarea

	data *interfaces.Data

	Ident string
	Value interfaces.Expr
	Type  *types.Type
}

// String returns a short representation of this statement.
func (obj *StmtBind) String() string {
	return fmt.Sprintf("bind(%s)", obj.Ident)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtBind) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Value.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtBind) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Ident == "" {
		return fmt.Errorf("bind ident is empty")
	}

	return obj.Value.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtBind) Interpolate() (interfaces.Stmt, error) {
	interpolated, err := obj.Value.Interpolate()
	if err != nil {
		return nil, err
	}
	return &StmtBind{
		Textarea: obj.Textarea,
		data:     obj.data,
		Ident:    obj.Ident,
		Value:    interpolated,
		Type:     obj.Type,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtBind) Copy() (interfaces.Stmt, error) {
	copied := false
	value, err := obj.Value.Copy()
	if err != nil {
		return nil, err
	}
	if value != obj.Value { // must have been copied, or pointer would be same
		copied = true
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtBind{
		Textarea: obj.Textarea,
		data:     obj.data,
		Ident:    obj.Ident,
		Value:    value,
		Type:     obj.Type,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// We only really care about the consumers here, because the "produces" aspect
// of this resource is handled by the StmtProg Ordering function. This is
// because the "prog" allows out-of-order statements, therefore it solves this
// by running an early (second) loop through the program and peering into this
// Stmt and extracting the produced name.
func (obj *StmtBind) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtbindvalue"}
	graph.AddEdge(obj.Value, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Value.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "stmtbind"}
		graph.AddEdge(n, k, edge)
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtBind) SetScope(scope *interfaces.Scope) error {
	emptyContext := map[string]interfaces.Expr{}
	return obj.Value.SetScope(scope, emptyContext)
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtBind) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	// Don't call obj.Value.Check here!
	typ, invariants, err := obj.Value.Infer()
	if err != nil {
		return nil, err
	}

	typExpr := obj.Type
	if obj.Type == nil {
		typExpr = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Value,
		Expect: typExpr, // obj.Type
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular bind statement adds its linked expression to
// the graph. It is not logically done in the ExprVar since that could exist
// multiple times for the single binding operation done here.
func (obj *StmtBind) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	g, _, err := obj.privateGraph(env)
	return g, err
}

// privateGraph is a more general version of Graph which also returns a Func.
func (obj *StmtBind) privateGraph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	g, f, err := obj.Value.Graph(env)
	return g, f, err
}

// Output for the bind statement produces no output. Any values of interest come
// from the use of the var which this binds the expression to.
func (obj *StmtBind) Output(map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	return interfaces.EmptyOutput(), nil
}

// StmtRes is a representation of a resource and possibly some edges. The `Name`
// value can be a single string or a list of strings. The former will produce a
// single resource, the latter produces a list of resources. Using this list
// mechanism is a safe alternative to traditional flow control like `for` loops.
// The `Name` value can only be a single string when it can be detected
// statically. Otherwise, it is assumed that a list of strings should be
// expected. More mechanisms to determine if the value is static may be added
// over time.
// TODO: Consider expanding Name to have this return a list of Res's in the
// Output function if it is a map[name]struct{}, or even a map[[]name]struct{}.
type StmtRes struct {
	interfaces.Textarea

	data *interfaces.Data

	Kind     string            // kind of resource, eg: pkg, file, svc, etc...
	Name     interfaces.Expr   // unique name for the res of this kind
	namePtr  interfaces.Func   // ptr for table lookup
	Contents []StmtResContents // list of fields/edges in parsed order

	// Collect specifies that we are "collecting" exported resources. The
	// names come from other hosts (or from ourselves during a self-export).
	Collect bool
}

// String returns a short representation of this statement.
func (obj *StmtRes) String() string {
	// TODO: add .String() for Contents and Name
	return fmt.Sprintf("res(%s)", obj.Kind)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtRes) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Name.Apply(fn); err != nil {
		return err
	}
	for _, x := range obj.Contents {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtRes) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Kind == "" {
		return fmt.Errorf("res kind is empty")
	}

	if strings.Contains(obj.Kind, "_") && obj.Kind != interfaces.PanicResKind {
		return fmt.Errorf("kind must not contain underscores")
	}

	if err := obj.Name.Init(data); err != nil {
		return err
	}
	fieldNames := make(map[string]struct{})
	metaNames := make(map[string]struct{})
	foundCollect := false
	for _, x := range obj.Contents {

		// Duplicate checking for identical field names.
		if line, ok := x.(*StmtResField); ok {
			// Was the field already seen in this resource?
			if _, exists := fieldNames[line.Field]; exists {
				return fmt.Errorf("resource has duplicate field of: %s", line.Field)
			}
			fieldNames[line.Field] = struct{}{}
		}

		// NOTE: you can have as many *StmtResEdge lines as you want =D

		if line, ok := x.(*StmtResMeta); ok {
			// Was the meta entry already seen in this resource?
			// Ignore the generic MetaField struct field for now.
			// You're allowed to have more than one Meta field, but
			// they can't contain the same field twice.
			// FIXME: Allow duplicates in certain fields, such as
			// ones that are lists... In this case, they merge...
			if _, exists := metaNames[line.Property]; exists && line.Property != MetaField {
				return fmt.Errorf("resource has duplicate meta entry of: %s", line.Property)
			}
			metaNames[line.Property] = struct{}{}
		}

		// Duplicate checking for more than one StmtResCollect entry.
		if stmt, ok := x.(*StmtResCollect); ok && foundCollect {
			// programming error
			return fmt.Errorf("duplicate collect body in res")

		} else if ok {
			if stmt.Kind != obj.Kind {
				// programming error
				return fmt.Errorf("unexpected kind mismatch")
			}
			foundCollect = true // found one
		}

		if err := x.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtRes) Interpolate() (interfaces.Stmt, error) {
	name, err := obj.Name.Interpolate()
	if err != nil {
		return nil, err
	}

	contents := []StmtResContents{}
	for _, x := range obj.Contents { // make sure we preserve ordering...
		interpolated, err := x.Interpolate()
		if err != nil {
			return nil, err
		}
		contents = append(contents, interpolated)
	}

	return &StmtRes{
		Textarea: obj.Textarea,
		data:     obj.data,
		Kind:     obj.Kind,
		Name:     name,
		Contents: contents,
		Collect:  obj.Collect,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtRes) Copy() (interfaces.Stmt, error) {
	copied := false
	name, err := obj.Name.Copy()
	if err != nil {
		return nil, err
	}
	if name != obj.Name { // must have been copied, or pointer would be same
		copied = true
	}

	copiedContents := false
	contents := []StmtResContents{}
	for _, x := range obj.Contents { // make sure we preserve ordering...
		cp, err := x.Copy()
		if err != nil {
			return nil, err
		}
		if cp != x {
			copiedContents = true
		}
		contents = append(contents, cp)
	}
	if copiedContents {
		copied = true
	} else {
		contents = obj.Contents // don't re-package it unnecessarily!
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtRes{
		Textarea: obj.Textarea,
		data:     obj.data,
		Kind:     obj.Kind,
		Name:     name,
		Contents: contents,
		Collect:  obj.Collect,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtRes) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraints: We know the name has to be satisfied before
	// this res statement itself can be used, since we depend on that value.
	edge := &pgraph.SimpleEdge{Name: "stmtresname"}
	graph.AddEdge(obj.Name, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Name.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "stmtres"}
		graph.AddEdge(n, k, edge)
	}

	for _, node := range obj.Contents {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtrescontents1"}
		graph.AddEdge(node, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtrescontents2"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtRes) SetScope(scope *interfaces.Scope) error {
	if err := obj.Name.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
		return err
	}
	for _, x := range obj.Contents {
		if err := x.SetScope(scope); err != nil {
			return err
		}
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtRes) TypeCheck() ([]*interfaces.UnificationInvariant, error) {

	// Don't call obj.Name.Check here!
	typ, invariants, err := obj.Name.Infer()
	if err != nil {
		return nil, err
	}

	for _, x := range obj.Contents {
		invars, err := x.TypeCheck(obj.Kind) // pass in the resource kind
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	// Optimization: If we know it's an str, no need for exclusives!
	// TODO: Check other cases, like if it's a function call, and we know it
	// can only return a single string. (Eg: fmt.printf for example.)
	isString := false
	isListString := false
	isCollectType := false
	typCollectFuncInType := types.NewType(funcs.CollectFuncInType)

	if _, ok := obj.Name.(*ExprStr); ok {
		// It's a string! (A plain string was specified.)
		isString = true
	}
	if typ, err := obj.Name.Type(); err == nil {
		// It has type of string! (Might be an interpolation specified.)
		if typ.Cmp(types.TypeStr) == nil {
			isString = true
		}
		if typ.Cmp(types.TypeListStr) == nil {
			isListString = true
		}
		if typ.Cmp(typCollectFuncInType) == nil {
			isCollectType = true
		}
	}

	var typExpr *types.Type // nil

	// If we pass here, we only allow []str, no need for exclusives!
	if isString {
		typExpr = types.TypeStr
	}
	if isListString {
		typExpr = types.TypeListStr // default for regular resources
	}
	if isCollectType && obj.Collect {
		typExpr = typCollectFuncInType
	}

	if !obj.Collect && typExpr == nil {
		typExpr = types.TypeListStr // default for regular resources
	}
	if obj.Collect && typExpr == nil {
		// TODO: do we want a default for collect ?
		typExpr = typCollectFuncInType // default for collect resources
	}

	if typExpr == nil { // If we don't know for sure, then we unify it all.
		typExpr = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Name,
		Expect: typExpr, // the name
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the resources created, but rather, once all the values (expressions) with
// no outgoing edges have produced at least a single value, then the resources
// know they're able to be built.
//
// This runs right after type unification. For this particular resource, we can
// do some additional static analysis, but only after unification has been done.
// Since I don't think it's worth extending the Stmt API for this, we can do the
// checks here at the beginning, and error out if something was invalid. In this
// particular case, the issue is one of catching duplicate meta fields.
func (obj *StmtRes) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	metaNames := make(map[string]struct{})
	for _, x := range obj.Contents {
		line, ok := x.(*StmtResMeta)
		if !ok {
			continue
		}

		properties := []string{line.Property} // "noop" or "Meta" or...
		if line.Property == MetaField {
			// If this is the generic MetaField struct field, then
			// we lookup the type signature to see which fields are
			// defined. You're allowed to have more than one Meta
			// field, but they can't contain the same field twice.

			typ, err := line.MetaExpr.Type() // must be known now
			if err != nil {
				// programming error in type unification
				return nil, errwrap.Wrapf(err, "unknown resource meta type")
			}
			if t := typ.Kind; t != types.KindStruct {
				return nil, fmt.Errorf("unexpected resource meta kind of: %s", t)
			}
			properties = typ.Ord // list of field names in this struct
		}

		for _, property := range properties {
			// Was the meta entry already seen in this resource?
			if _, exists := metaNames[property]; exists {
				return nil, fmt.Errorf("resource has duplicate meta entry of: %s", property)
			}
			metaNames[property] = struct{}{}
		}
	}

	graph, err := pgraph.NewGraph("res")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.Name.Graph(env)
	if err != nil {
		return nil, err
	}

	graph.AddGraph(g)
	obj.namePtr = f

	for _, x := range obj.Contents {
		g, err := x.Graph(env)
		if err != nil {
			return nil, err
		}
		graph.AddGraph(g)
	}

	return graph, nil
}

// Output returns the output that this "program" produces. This output is what
// is used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output. In
// the case of this resource statement, this is definitely the case.
func (obj *StmtRes) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	if obj.namePtr == nil {
		return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
	}
	nameValue, exists := table[obj.namePtr]
	if !exists {
		return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
	}

	// the host in this output is who the data is from
	mapping, err := obj.collect(table) // gives us (name, host, data)
	if err != nil {
		return nil, err
	}
	if mapping == nil { // for when we're not collecting
		mapping = make(map[string]map[string]string)
	}

	typCollectFuncInType := types.NewType(funcs.CollectFuncInType)

	names := []string{} // list of names to build (TODO: map instead?)
	switch {
	case types.TypeStr.Cmp(nameValue.Type()) == nil:
		name := nameValue.Str() // must not panic
		names = append(names, name)
		for n := range mapping { // delete everything else
			if n == name {
				continue
			}
			delete(mapping, n)
		}
		if !obj.Collect { // mapping is empty, add a stub
			mapping[name] = map[string]string{
				"*": "", // empty data
			}
		}

	case types.TypeListStr.Cmp(nameValue.Type()) == nil:
		for _, x := range nameValue.List() { // must not panic
			name := x.Str() // must not panic
			names = append(names, name)
			if !obj.Collect { // mapping is empty, add a stub
				mapping[name] = map[string]string{
					"*": "", // empty data
				}
			}
		}
		for n := range mapping { // delete everything else
			if util.StrInList(n, names) {
				continue
			}
			delete(mapping, n)
		}

	case obj.Collect && typCollectFuncInType.Cmp(nameValue.Type()) == nil:
		hosts := make(map[string]string)
		for _, x := range nameValue.List() { // must not panic
			st, ok := x.(*types.StructValue)
			if !ok {
				// programming error
				return nil, fmt.Errorf("value is not a struct")
			}
			name, exists := st.Lookup(funcs.CollectFuncInFieldName)
			if !exists {
				// programming error?
				return nil, fmt.Errorf("name field is missing")
			}
			host, exists := st.Lookup(funcs.CollectFuncInFieldHost)
			if !exists {
				// programming error?
				return nil, fmt.Errorf("host field is missing")
			}

			s := name.Str() // must not panic
			if s == "" {
				return nil, fmt.Errorf("empty name")
			}
			names = append(names, s)
			// host is the input telling us who we want to pull from
			hosts[s] = host.Str() // correspondence map
			if hosts[s] == "" {
				return nil, fmt.Errorf("empty host")
			}
			if hosts[s] == "*" { // safety
				return nil, fmt.Errorf("invalid star host")
			}
		}
		for n, m := range mapping { // delete everything else
			if !util.StrInList(n, names) {
				delete(mapping, n)
				continue
			}
			host := hosts[n] // the matching host for the name
			for h := range m {
				if h == host {
					continue
				}
				delete(mapping[n], h)
			}
		}

	default:
		// programming error
		return nil, fmt.Errorf("unhandled resource name type: %+v", nameValue.Type())
	}

	resources := []engine.Res{}
	edges := []*interfaces.Edge{}

	apply, err := obj.metaparams(table)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error generating metaparams")
	}

	// TODO: sort?
	for name, m := range mapping {
		for host, data := range m {
			// host may be * if not collecting
			// data may be empty if not collecting
			_ = host                                    // unused atm
			res, err := obj.resource(table, name, data) // one at a time
			if err != nil {
				return nil, errwrap.Wrapf(err, "error building resource")
			}
			apply(res) // apply metaparams, does not return anything

			resources = append(resources, res)

			edgeList, err := obj.edges(table, name)
			if err != nil {
				return nil, errwrap.Wrapf(err, "error building edges")
			}
			edges = append(edges, edgeList...)
		}
	}

	return &interfaces.Output{
		Resources: resources,
		Edges:     edges,
	}, nil
}

// collect is a helper function to pull out the collected resource data.
func (obj *StmtRes) collect(table map[interfaces.Func]types.Value) (map[string]map[string]string, error) {
	if !obj.Collect {
		return nil, nil // nothing to do
	}

	var val types.Value // = nil
	typCollectFuncOutType := types.NewType(funcs.CollectFuncOutType)

	for _, line := range obj.Contents {
		x, ok := line.(*StmtResCollect)
		if !ok {
			continue
		}
		if x.Kind != obj.Kind { // should have been caught in Init
			// programming error
			return nil, fmt.Errorf("unexpected kind mismatch")
		}
		if x.valuePtr == nil {
			return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
		}

		fv, exists := table[x.valuePtr]
		if !exists {
			return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
		}

		if err := fv.Type().Cmp(typCollectFuncOutType); err != nil { // "[]struct{name str; host str; data str}"
			// programming error
			return nil, fmt.Errorf("resource collect has invalid type: `%+v`", err)
		}

		val = fv // found
		break
	}

	if val == nil {
		// programming error?
		return nil, nil // nothing found
	}

	m := make(map[string]map[string]string) // name, host, data

	// TODO: Store/cache this in an efficient form to avoid loops...
	// TODO: Eventually collect func should, for efficiency, return:
	// map{struct{name str; host str}: str} // key => $data
	for _, x := range val.List() { // must not panic
		st, ok := x.(*types.StructValue)
		if !ok {
			// programming error
			return nil, fmt.Errorf("value is not a struct")
		}
		name, exists := st.Lookup(funcs.CollectFuncOutFieldName)
		if !exists {
			// programming error?
			return nil, fmt.Errorf("name field is missing")
		}
		host, exists := st.Lookup(funcs.CollectFuncOutFieldHost)
		if !exists {
			// programming error?
			return nil, fmt.Errorf("host field is missing")
		}
		data, exists := st.Lookup(funcs.CollectFuncOutFieldData)
		if !exists {
			// programming error?
			return nil, fmt.Errorf("data field is missing")
		}

		// found!
		n := name.Str() // must not panic
		h := host.Str() // must not panic

		if n == "" {
			// programming error
			return nil, fmt.Errorf("name field is empty")
		}
		if h == "" {
			// programming error
			return nil, fmt.Errorf("host field is empty")
		}
		if h == "*" {
			// programming error
			return nil, fmt.Errorf("host field is a start")
		}

		if _, exists := m[n]; !exists {
			m[n] = make(map[string]string)
		}
		m[n][h] = data.Str() // must not panic
	}

	return m, nil
}

// resource is a helper function to generate the res that comes from this.
// TODO: it could memoize some of the work to avoid re-computation when looped
func (obj *StmtRes) resource(table map[interfaces.Func]types.Value, resName, data string) (engine.Res, error) {
	res, err := engine.NewNamedResource(obj.Kind, resName)
	if err != nil {
		return nil, errwrap.Wrapf(err, "cannot create resource kind `%s` with named `%s`", obj.Kind, resName)
	}

	// Here we start off by using the collected resource as the base params.
	// Then we overwrite over it below using the normal param setup methods.
	if obj.Collect && data != "" {
		// TODO: Do we want to have an alternate implementation of this
		// to go along with the ExportableRes encoding variant?
		if res, err = engineUtil.B64ToRes(data); err != nil {
			return nil, fmt.Errorf("can't convert from B64: %v", err)
		}
		if res.Kind() != obj.Kind { // should have been caught somewhere
			// programming error
			return nil, fmt.Errorf("unexpected kind mismatch")
		}
		obj.data.Logf("collect: %s", res)

		// XXX: Do we want to change any metaparams when we collect?
		// XXX: Do we want to change any metaparams when we export?
		//res.MetaParams().Hidden = false // unlikely, but I considered
		res.MetaParams().Export = []string{} // don't re-export
	}

	sv := reflect.ValueOf(res).Elem() // pointer to struct, then struct
	if k := sv.Kind(); k != reflect.Struct {
		panic(fmt.Sprintf("expected struct, got: %s", k))
	}

	mapping, err := engineUtil.LangFieldNameToStructFieldName(obj.Kind)
	if err != nil {
		return nil, err
	}
	st := reflect.TypeOf(res).Elem() // pointer to struct, then struct

	// FIXME: we could probably simplify this code...
	for _, line := range obj.Contents {
		x, ok := line.(*StmtResField)
		if !ok {
			continue
		}

		if x.Condition != nil {
			if x.conditionPtr == nil {
				return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
			}
			b, exists := table[x.conditionPtr]
			if !exists {
				return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
			}

			if !b.Bool() { // if value exists, and is false, skip it
				continue
			}
		}

		typ, err := x.Value.Type()
		if err != nil {
			return nil, errwrap.Wrapf(err, "resource field `%s` did not return a type", x.Field)
		}

		name, exists := mapping[x.Field] // lookup recommended field name
		if !exists {                     // this should be caught during unification.
			return nil, fmt.Errorf("field `%s` does not exist", x.Field) // user made a typo?
		}

		tf, exists := st.FieldByName(name) // exported field type
		if !exists {
			return nil, fmt.Errorf("field `%s` type does not exist", x.Field)
		}

		f := sv.FieldByName(name) // exported field
		if !f.IsValid() || !f.CanSet() {
			return nil, fmt.Errorf("field `%s` cannot be set", name) // field is broken?
		}

		// is expr type compatible with expected field type?
		t, err := types.ResTypeOf(tf.Type)
		if err != nil {
			return nil, errwrap.Wrapf(err, "resource field `%s` has no compatible type", x.Field)
		}
		if t == nil {
			// possible programming error
			return nil, fmt.Errorf("resource field `%s` of nil type cannot match type `%+v`", x.Field, typ)
		}

		// Let the variants pass through...
		if err := t.Cmp(typ); err != nil && t.Kind != types.KindVariant {
			return nil, errwrap.Wrapf(err, "resource field `%s` of type `%+v`, cannot take type `%+v`", x.Field, t, typ)
		}

		if x.valuePtr == nil {
			return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
		}
		fv, exists := table[x.valuePtr]
		if !exists {
			return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
		}

		// mutate the struct field f with the mcl data in fv
		if err := types.Into(fv, f); err != nil {
			return nil, err
		}
	}

	return res, nil
}

// edges is a helper function to generate the edges that come from the resource.
func (obj *StmtRes) edges(table map[interfaces.Func]types.Value, resName string) ([]*interfaces.Edge, error) {
	edges := []*interfaces.Edge{}

	// to and from self, map of kind, name, notify
	var to = make(map[string]map[string]bool)   // to this from self
	var from = make(map[string]map[string]bool) // from this to self

	for _, line := range obj.Contents {
		x, ok := line.(*StmtResEdge)
		if !ok {
			continue
		}

		if x.Condition != nil {
			if x.conditionPtr == nil {
				return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
			}
			b, exists := table[x.conditionPtr]
			if !exists {
				return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
			}

			if !b.Bool() { // if value exists, and is false, skip it
				continue
			}
		}

		if x.EdgeHalf.namePtr == nil {
			return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
		}
		nameValue, exists := table[x.EdgeHalf.namePtr]
		if !exists {
			return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
		}

		// the edge name can be a single string or a list of strings...

		names := []string{} // list of names to build
		switch {
		case types.TypeStr.Cmp(nameValue.Type()) == nil:
			name := nameValue.Str() // must not panic
			names = append(names, name)

		case types.TypeListStr.Cmp(nameValue.Type()) == nil:
			for _, x := range nameValue.List() { // must not panic
				name := x.Str() // must not panic
				names = append(names, name)
			}

		default:
			// programming error
			return nil, fmt.Errorf("unhandled resource name type: %+v", nameValue.Type())
		}

		kind := x.EdgeHalf.Kind
		for _, name := range names {
			var notify bool

			switch p := x.Property; p {
			// a -> b
			// a notify b
			// a before b
			case EdgeNotify:
				notify = true
				fallthrough
			case EdgeBefore:
				if m, exists := to[kind]; !exists {
					to[kind] = make(map[string]bool)
				} else if n, exists := m[name]; exists {
					notify = notify || n // collate
				}
				to[kind][name] = notify // to this from self

			// b -> a
			// b listen a
			// b depend a
			case EdgeListen:
				notify = true
				fallthrough
			case EdgeDepend:
				if m, exists := from[kind]; !exists {
					from[kind] = make(map[string]bool)
				} else if n, exists := m[name]; exists {
					notify = notify || n // collate
				}
				from[kind][name] = notify // from this to self

			default:
				return nil, fmt.Errorf("unknown property: %s", p)
			}
		}
	}

	// TODO: we could detect simple loops here (if `from` and `to` have the
	// same entry) but we can leave this to the proper dag checker later on

	for kind, x := range to { // to this from self
		for name, notify := range x {
			edge := &interfaces.Edge{
				Kind1: obj.Kind,
				Name1: resName, // self
				//Send: "",

				Kind2: kind,
				Name2: name,
				//Recv: "",

				Notify: notify,
			}
			edges = append(edges, edge)
		}
	}
	for kind, x := range from { // from this to self
		for name, notify := range x {
			edge := &interfaces.Edge{
				Kind1: kind,
				Name1: name,
				//Send: "",

				Kind2: obj.Kind,
				Name2: resName, // self
				//Recv: "",

				Notify: notify,
			}
			edges = append(edges, edge)
		}
	}

	return edges, nil
}

// metaparams is a helper function to get the metaparams that come from the
// resource AST so we can eventually set them on the individual resource.
func (obj *StmtRes) metaparams(table map[interfaces.Func]types.Value) (func(engine.Res), error) {
	apply := []func(engine.Res){}

	for _, line := range obj.Contents {
		x, ok := line.(*StmtResMeta)
		if !ok {
			continue
		}

		if x.Condition != nil {
			if x.conditionPtr == nil {
				return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
			}
			b, exists := table[x.conditionPtr]
			if !exists {
				return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
			}

			if !b.Bool() { // if value exists, and is false, skip it
				continue
			}
		}

		if x.metaExprPtr == nil {
			return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
		}
		v, exists := table[x.metaExprPtr]
		if !exists {
			return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
		}

		switch p := strings.ToLower(x.Property); p {
		// TODO: we could add these fields dynamically if we were fancy!
		case "noop":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Noop = v.Bool() // must not panic
			})

		case "retry":
			x := v.Int() // must not panic
			// TODO: check that it doesn't overflow
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Retry = int16(x)
			})

		case "retryreset":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().RetryReset = v.Bool() // must not panic
			})

		case "delay":
			x := v.Int() // must not panic
			// TODO: check that it isn't signed
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Delay = uint64(x)
			})

		case "poll":
			x := v.Int() // must not panic
			// TODO: check that it doesn't overflow and isn't signed
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Poll = uint32(x)
			})

		case "limit": // rate.Limit
			x := v.Float() // must not panic
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Limit = rate.Limit(x)
			})

		case "burst":
			x := v.Int() // must not panic
			// TODO: check that it doesn't overflow
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Burst = int(x)
			})

		case "reset":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Reset = v.Bool() // must not panic
			})

		case "sema": // []string
			values := []string{}
			for _, x := range v.List() { // must not panic
				s := x.Str() // must not panic
				values = append(values, s)
			}
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Sema = values
			})

		case "rewatch":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Rewatch = v.Bool() // must not panic
			})

		case "realize":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Realize = v.Bool() // must not panic
			})

		case "dollar":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Dollar = v.Bool() // must not panic
			})

		case "hidden":
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Hidden = v.Bool() // must not panic
			})

		case "export": // []string
			values := []string{}
			for _, x := range v.List() { // must not panic
				s := x.Str() // must not panic
				values = append(values, s)
			}
			apply = append(apply, func(res engine.Res) {
				res.MetaParams().Export = values
			})

		case "reverse":
			apply = append(apply, func(res engine.Res) {
				r, ok := res.(engine.ReversibleRes)
				if !ok {
					return
				}
				// *engine.ReversibleMeta
				rm := r.ReversibleMeta() // get current values
				rm.Disabled = !v.Bool()  // must not panic
				r.SetReversibleMeta(rm)  // set
			})

		case "autoedge":
			apply = append(apply, func(res engine.Res) {
				r, ok := res.(engine.EdgeableRes)
				if !ok {
					return
				}
				// *engine.AutoEdgeMeta
				aem := r.AutoEdgeMeta()  // get current values
				aem.Disabled = !v.Bool() // must not panic
				r.SetAutoEdgeMeta(aem)   // set
			})

		case "autogroup":
			apply = append(apply, func(res engine.Res) {
				r, ok := res.(engine.GroupableRes)
				if !ok {
					return
				}
				// *engine.AutoGroupMeta
				agm := r.AutoGroupMeta() // get current values
				agm.Disabled = !v.Bool() // must not panic
				r.SetAutoGroupMeta(agm)  // set

			})

		case MetaField:
			if val, exists := v.Struct()["noop"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Noop = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["retry"]; exists {
				x := val.Int() // must not panic
				// TODO: check that it doesn't overflow
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Retry = int16(x)
				})
			}
			if val, exists := v.Struct()["retryreset"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().RetryReset = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["delay"]; exists {
				x := val.Int() // must not panic
				// TODO: check that it isn't signed
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Delay = uint64(x)
				})
			}
			if val, exists := v.Struct()["poll"]; exists {
				x := val.Int() // must not panic
				// TODO: check that it doesn't overflow and isn't signed
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Poll = uint32(x)
				})
			}
			if val, exists := v.Struct()["limit"]; exists {
				x := val.Float() // must not panic
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Limit = rate.Limit(x)
				})
			}
			if val, exists := v.Struct()["burst"]; exists {
				x := val.Int() // must not panic
				// TODO: check that it doesn't overflow
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Burst = int(x)
				})
			}
			if val, exists := v.Struct()["reset"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Reset = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["sema"]; exists {
				values := []string{}
				for _, x := range val.List() { // must not panic
					s := x.Str() // must not panic
					values = append(values, s)
				}
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Sema = values
				})
			}
			if val, exists := v.Struct()["rewatch"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Rewatch = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["realize"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Realize = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["dollar"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Dollar = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["hidden"]; exists {
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Hidden = val.Bool() // must not panic
				})
			}
			if val, exists := v.Struct()["export"]; exists {
				values := []string{}
				for _, x := range val.List() { // must not panic
					s := x.Str() // must not panic
					values = append(values, s)
				}
				apply = append(apply, func(res engine.Res) {
					res.MetaParams().Export = values
				})
			}
			if val, exists := v.Struct()["reverse"]; exists {
				apply = append(apply, func(res engine.Res) {
					r, ok := res.(engine.ReversibleRes)
					if !ok {
						return
					}
					// *engine.ReversibleMeta
					rm := r.ReversibleMeta()  // get current values
					rm.Disabled = !val.Bool() // must not panic
					r.SetReversibleMeta(rm)   // set
				})
			}
			if val, exists := v.Struct()["autoedge"]; exists {
				apply = append(apply, func(res engine.Res) {
					r, ok := res.(engine.EdgeableRes)
					if !ok {
						return
					}
					// *engine.AutoEdgeMeta
					aem := r.AutoEdgeMeta()    // get current values
					aem.Disabled = !val.Bool() // must not panic
					r.SetAutoEdgeMeta(aem)     // set
				})
			}
			if val, exists := v.Struct()["autogroup"]; exists {
				apply = append(apply, func(res engine.Res) {
					r, ok := res.(engine.GroupableRes)
					if !ok {
						return
					}
					// *engine.AutoGroupMeta
					agm := r.AutoGroupMeta()   // get current values
					agm.Disabled = !val.Bool() // must not panic
					r.SetAutoGroupMeta(agm)    // set

				})
			}

		default:
			return nil, fmt.Errorf("unknown property: %s", p)
		}
	}

	fn := func(res engine.Res) {
		for _, f := range apply {
			f(res)
		}
	}

	return fn, nil
}

// StmtResContents is the interface that is met by the resource contents. Look
// closely for while it is similar to the Stmt interface, it is quite different.
type StmtResContents interface {
	interfaces.Node
	Init(*interfaces.Data) error
	Interpolate() (StmtResContents, error) // different!
	Copy() (StmtResContents, error)
	Ordering(map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error)
	SetScope(*interfaces.Scope) error
	TypeCheck(kind string) ([]*interfaces.UnificationInvariant, error)
	Graph(env *interfaces.Env) (*pgraph.Graph, error)
}

// StmtResField represents a single field in the parsed resource representation.
// This does not satisfy the Stmt interface.
type StmtResField struct {
	interfaces.Textarea

	data *interfaces.Data

	Field        string
	Value        interfaces.Expr
	valuePtr     interfaces.Func // ptr for table lookup
	Condition    interfaces.Expr // the value will be used if nil or true
	conditionPtr interfaces.Func // ptr for table lookup
}

// String returns a short representation of this statement.
func (obj *StmtResField) String() string {
	// TODO: add .String() for Condition and Value
	return fmt.Sprintf("resfield(%s)", obj.Field)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtResField) Apply(fn func(interfaces.Node) error) error {
	if obj.Condition != nil {
		if err := obj.Condition.Apply(fn); err != nil {
			return err
		}
	}
	if err := obj.Value.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtResField) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Field == "" {
		return fmt.Errorf("res field name is empty")
	}

	if obj.Condition != nil {
		if err := obj.Condition.Init(data); err != nil {
			return err
		}
	}
	return obj.Value.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// This interpolate is different It is different from the interpolate found in
// the Expr and Stmt interfaces because it returns a different type as output.
func (obj *StmtResField) Interpolate() (StmtResContents, error) {
	interpolated, err := obj.Value.Interpolate()
	if err != nil {
		return nil, err
	}
	var condition interfaces.Expr
	if obj.Condition != nil {
		condition, err = obj.Condition.Interpolate()
		if err != nil {
			return nil, err
		}
	}
	return &StmtResField{
		Textarea:  obj.Textarea,
		data:      obj.data,
		Field:     obj.Field,
		Value:     interpolated,
		Condition: condition,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtResField) Copy() (StmtResContents, error) {
	copied := false
	value, err := obj.Value.Copy()
	if err != nil {
		return nil, err
	}
	if value != obj.Value { // must have been copied, or pointer would be same
		copied = true
	}

	var condition interfaces.Expr
	if obj.Condition != nil {
		condition, err = obj.Condition.Copy()
		if err != nil {
			return nil, err
		}
		if condition != obj.Condition {
			copied = true
		}

	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtResField{
		Textarea:  obj.Textarea,
		data:      obj.data,
		Field:     obj.Field,
		Value:     value,
		Condition: condition,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtResField) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtresfieldvalue"}
	graph.AddEdge(obj.Value, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)
	nodes := []interfaces.Expr{obj.Value}
	if obj.Condition != nil {
		nodes = append(nodes, obj.Condition)

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtresfieldcondition"}
		graph.AddEdge(obj.Condition, obj, edge) // prod -> cons
	}

	for _, node := range nodes {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtresfield"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtResField) SetScope(scope *interfaces.Scope) error {
	if err := obj.Value.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
		return err
	}
	if obj.Condition != nil {
		if err := obj.Condition.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
			return err
		}
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions. It is different from the TypeCheck method
// found in the Stmt interface because it adds an input parameter.
func (obj *StmtResField) TypeCheck(kind string) ([]*interfaces.UnificationInvariant, error) {
	typ, invariants, err := obj.Value.Infer()
	if err != nil {
		return nil, err
	}

	//invars, err := obj.Value.Check(typ) // don't call this here!

	if obj.Condition != nil {
		typ, invars, err := obj.Condition.Infer()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)

		// XXX: Is this needed?
		invar := &interfaces.UnificationInvariant{
			Node:   obj,
			Expr:   obj.Condition,
			Expect: types.TypeBool,
			Actual: typ,
		}
		invariants = append(invariants, invar)
	}

	// TODO: unfortunately this gets called separately for each field... if
	// we could cache this, it might be worth looking into for performance!
	// XXX: Should this return unification variables instead of variant types?
	typMap, err := engineUtil.LangFieldNameToStructType(kind)
	if err != nil {
		return nil, err
	}

	field := strings.TrimSpace(obj.Field)
	if len(field) != len(obj.Field) {
		return nil, fmt.Errorf("field was wrapped in whitespace")
	}
	if len(strings.Fields(field)) != 1 {
		return nil, fmt.Errorf("field was empty or contained spaces")
	}

	typExpr, exists := typMap[obj.Field]
	if !exists {
		return nil, fmt.Errorf("field `%s` does not exist in `%s` resource", obj.Field, kind)
	}
	if typExpr == nil {
		// possible programming error
		return nil, fmt.Errorf("type for field `%s` in `%s` is nil", obj.Field, kind)
	}
	if typExpr.Kind == types.KindVariant { // special path, res field has interface{}
		typExpr = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}

	// regular scenario
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Value,
		Expect: typExpr,
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the resources created, but rather, once all the values (expressions) with
// no outgoing edges have produced at least a single value, then the resources
// know they're able to be built.
func (obj *StmtResField) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("resfield")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.Value.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)
	obj.valuePtr = f

	if obj.Condition != nil {
		g, f, err := obj.Condition.Graph(env)
		if err != nil {
			return nil, err
		}
		graph.AddGraph(g)
		obj.conditionPtr = f
	}

	return graph, nil
}

// StmtResEdge represents a single edge property in the parsed resource
// representation. This does not satisfy the Stmt interface.
type StmtResEdge struct {
	interfaces.Textarea

	data *interfaces.Data

	Property     string // TODO: iota constant instead?
	EdgeHalf     *StmtEdgeHalf
	Condition    interfaces.Expr // the value will be used if nil or true
	conditionPtr interfaces.Func // ptr for table lookup
}

// String returns a short representation of this statement.
func (obj *StmtResEdge) String() string {
	// TODO: add .String() for Condition and EdgeHalf
	return fmt.Sprintf("resedge(%s)", obj.Property)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtResEdge) Apply(fn func(interfaces.Node) error) error {
	if obj.Condition != nil {
		if err := obj.Condition.Apply(fn); err != nil {
			return err
		}
	}
	if err := obj.EdgeHalf.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtResEdge) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Property == "" {
		return fmt.Errorf("res edge property is empty")
	}
	if obj.Property != EdgeNotify && obj.Property != EdgeBefore && obj.Property != EdgeListen && obj.Property != EdgeDepend {
		return fmt.Errorf("invalid property: `%s`", obj.Property)
	}

	if obj.Condition != nil {
		if err := obj.Condition.Init(data); err != nil {
			return err
		}
	}
	return obj.EdgeHalf.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// This interpolate is different It is different from the interpolate found in
// the Expr and Stmt interfaces because it returns a different type as output.
func (obj *StmtResEdge) Interpolate() (StmtResContents, error) {
	interpolated, err := obj.EdgeHalf.Interpolate()
	if err != nil {
		return nil, err
	}
	var condition interfaces.Expr
	if obj.Condition != nil {
		condition, err = obj.Condition.Interpolate()
		if err != nil {
			return nil, err
		}
	}
	return &StmtResEdge{
		Textarea:  obj.Textarea,
		data:      obj.data,
		Property:  obj.Property,
		EdgeHalf:  interpolated,
		Condition: condition,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtResEdge) Copy() (StmtResContents, error) {
	copied := false
	edgeHalf, err := obj.EdgeHalf.Copy()
	if err != nil {
		return nil, err
	}
	if edgeHalf != obj.EdgeHalf { // must have been copied, or pointer would be same
		copied = true
	}

	var condition interfaces.Expr
	if obj.Condition != nil {
		condition, err = obj.Condition.Copy()
		if err != nil {
			return nil, err
		}
		if condition != obj.Condition {
			copied = true
		}
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtResEdge{
		Textarea:  obj.Textarea,
		data:      obj.data,
		Property:  obj.Property,
		EdgeHalf:  edgeHalf,
		Condition: condition,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtResEdge) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtresedgehalf"}
	// TODO: obj.EdgeHalf or obj.EdgeHalf.Name ?
	graph.AddEdge(obj.EdgeHalf.Name, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)
	nodes := []interfaces.Expr{obj.EdgeHalf.Name}
	if obj.Condition != nil {
		nodes = append(nodes, obj.Condition)

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtresedgecondition"}
		graph.AddEdge(obj.Condition, obj, edge) // prod -> cons
	}

	for _, node := range nodes {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtresedge"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtResEdge) SetScope(scope *interfaces.Scope) error {
	if err := obj.EdgeHalf.SetScope(scope); err != nil {
		return err
	}
	if obj.Condition != nil {
		if err := obj.Condition.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
			return err
		}
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions. It is different from the TypeCheck method
// found in the Stmt interface because it adds an input parameter.
func (obj *StmtResEdge) TypeCheck(kind string) ([]*interfaces.UnificationInvariant, error) {
	invariants, err := obj.EdgeHalf.TypeCheck()
	if err != nil {
		return nil, err
	}

	//invars, err := obj.Value.Check(typ) // don't call this here!

	if obj.Condition != nil {
		typ, invars, err := obj.Condition.Infer()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)

		// XXX: Is this needed?
		invar := &interfaces.UnificationInvariant{
			Node:   obj,
			Expr:   obj.Condition,
			Expect: types.TypeBool,
			Actual: typ,
		}
		invariants = append(invariants, invar)
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the resources created, but rather, once all the values (expressions) with
// no outgoing edges have produced at least a single value, then the resources
// know they're able to be built.
func (obj *StmtResEdge) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("resedge")
	if err != nil {
		return nil, err
	}

	g, err := obj.EdgeHalf.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)

	if obj.Condition != nil {
		g, f, err := obj.Condition.Graph(env)
		if err != nil {
			return nil, err
		}
		graph.AddGraph(g)
		obj.conditionPtr = f
	}

	return graph, nil
}

// StmtResMeta represents a single meta value in the parsed resource
// representation. It can also contain a struct that contains one or more meta
// parameters. If it contains such a struct, then the `Property` field contains
// the string found in the MetaField constant, otherwise this field will
// correspond to the particular meta parameter specified. This does not satisfy
// the Stmt interface.
type StmtResMeta struct {
	interfaces.Textarea

	data *interfaces.Data

	Property     string // TODO: iota constant instead?
	MetaExpr     interfaces.Expr
	metaExprPtr  interfaces.Func // ptr for table lookup
	Condition    interfaces.Expr // the value will be used if nil or true
	conditionPtr interfaces.Func // ptr for table lookup
}

// String returns a short representation of this statement.
func (obj *StmtResMeta) String() string {
	// TODO: add .String() for Condition and MetaExpr
	return fmt.Sprintf("resmeta(%s)", obj.Property)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtResMeta) Apply(fn func(interfaces.Node) error) error {
	if obj.Condition != nil {
		if err := obj.Condition.Apply(fn); err != nil {
			return err
		}
	}
	if err := obj.MetaExpr.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtResMeta) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Property == "" {
		return fmt.Errorf("res meta property is empty")
	}

	switch p := strings.ToLower(obj.Property); p {
	// TODO: we could add these fields dynamically if we were fancy!
	case "noop":
	case "retry":
	case "retryreset":
	case "delay":
	case "poll":
	case "limit":
	case "burst":
	case "reset":
	case "sema":
	case "rewatch":
	case "realize":
	case "dollar":
	case "hidden":
	case "export":
	case "reverse":
	case "autoedge":
	case "autogroup":
	case MetaField:

	default:
		return fmt.Errorf("invalid property: `%s`", obj.Property)
	}

	if obj.Condition != nil {
		if err := obj.Condition.Init(data); err != nil {
			return err
		}
	}
	return obj.MetaExpr.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// This interpolate is different It is different from the interpolate found in
// the Expr and Stmt interfaces because it returns a different type as output.
func (obj *StmtResMeta) Interpolate() (StmtResContents, error) {
	interpolated, err := obj.MetaExpr.Interpolate()
	if err != nil {
		return nil, err
	}
	var condition interfaces.Expr
	if obj.Condition != nil {
		condition, err = obj.Condition.Interpolate()
		if err != nil {
			return nil, err
		}
	}
	return &StmtResMeta{
		Textarea:  obj.Textarea,
		data:      obj.data,
		Property:  obj.Property,
		MetaExpr:  interpolated,
		Condition: condition,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtResMeta) Copy() (StmtResContents, error) {
	copied := false
	metaExpr, err := obj.MetaExpr.Copy()
	if err != nil {
		return nil, err
	}
	if metaExpr != obj.MetaExpr { // must have been copied, or pointer would be same
		copied = true
	}

	var condition interfaces.Expr
	if obj.Condition != nil {
		condition, err = obj.Condition.Copy()
		if err != nil {
			return nil, err
		}
		if condition != obj.Condition {
			copied = true
		}
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtResMeta{
		Textarea:  obj.Textarea,
		data:      obj.data,
		Property:  obj.Property,
		MetaExpr:  metaExpr,
		Condition: condition,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtResMeta) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtresmetaexpr"}
	graph.AddEdge(obj.MetaExpr, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)
	nodes := []interfaces.Expr{obj.MetaExpr}
	if obj.Condition != nil {
		nodes = append(nodes, obj.Condition)

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtresmetacondition"}
		graph.AddEdge(obj.Condition, obj, edge) // prod -> cons
	}

	for _, node := range nodes {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtresmeta"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtResMeta) SetScope(scope *interfaces.Scope) error {
	if err := obj.MetaExpr.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
		return err
	}
	if obj.Condition != nil {
		if err := obj.Condition.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
			return err
		}
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions. It is different from the TypeCheck method
// found in the Stmt interface because it adds an input parameter.
func (obj *StmtResMeta) TypeCheck(kind string) ([]*interfaces.UnificationInvariant, error) {

	typ, invariants, err := obj.MetaExpr.Infer()
	if err != nil {
		return nil, err
	}

	//invars, err := obj.MetaExpr.Check(typ) // don't call this here!

	if obj.Condition != nil {
		typ, invars, err := obj.Condition.Infer()
		if err != nil {
			return nil, err
		}

		invariants = append(invariants, invars...)

		// XXX: Is this needed?
		invar := &interfaces.UnificationInvariant{
			Node:   obj,
			Expr:   obj.Condition,
			Expect: types.TypeBool,
			Actual: typ,
		}
		invariants = append(invariants, invar)
	}

	var typExpr *types.Type
	//typExpr = &types.Type{
	//	Kind: types.KindUnification,
	//	Uni:  types.NewElem(), // unification variable, eg: ?1
	//}

	// add additional invariants based on what's in obj.Property !!!
	switch p := strings.ToLower(obj.Property); p {
	// TODO: we could add these fields dynamically if we were fancy!
	case "noop":
		typExpr = types.TypeBool

	case "retry":
		typExpr = types.TypeInt

	case "retryreset":
		typExpr = types.TypeBool

	case "delay":
		typExpr = types.TypeInt

	case "poll":
		typExpr = types.TypeInt

	case "limit": // rate.Limit
		typExpr = types.TypeFloat

	case "burst":
		typExpr = types.TypeInt

	case "reset":
		typExpr = types.TypeBool

	case "sema":
		typExpr = types.TypeListStr

	case "rewatch":
		typExpr = types.TypeBool

	case "realize":
		typExpr = types.TypeBool

	case "dollar":
		typExpr = types.TypeBool

	case "hidden":
		typExpr = types.TypeBool

	case "export":
		typExpr = types.TypeListStr

	case "reverse":
		// TODO: We might want more parameters about how to reverse.
		typExpr = types.TypeBool

	case "autoedge":
		typExpr = types.TypeBool

	case "autogroup":
		typExpr = types.TypeBool

	// autoedge and autogroup aren't part of the `MetaRes` interface, but we
	// can merge them in here for simplicity in the public user interface...
	case MetaField:
		// FIXME: allow partial subsets of this struct, and in any order
		// FIXME: we might need an updated unification engine to do this
		wrap := func(reverse *types.Type) *types.Type {
			return types.NewType(fmt.Sprintf("struct{noop bool; retry int; retryreset bool; delay int; poll int; limit float; burst int; reset bool; sema []str; rewatch bool; realize bool; dollar bool; hidden bool; export []str; reverse %s; autoedge bool; autogroup bool}", reverse.String()))
		}
		// TODO: We might want more parameters about how to reverse.
		typExpr = wrap(types.TypeBool)

	default:
		return nil, fmt.Errorf("unknown property: %s", p)
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.MetaExpr,
		Expect: typExpr,
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the resources created, but rather, once all the values (expressions) with
// no outgoing edges have produced at least a single value, then the resources
// know they're able to be built.
func (obj *StmtResMeta) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("resmeta")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.MetaExpr.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)
	obj.metaExprPtr = f

	if obj.Condition != nil {
		g, f, err := obj.Condition.Graph(env)
		if err != nil {
			return nil, err
		}
		graph.AddGraph(g)
		obj.conditionPtr = f

	}

	return graph, nil
}

// StmtResCollect represents hidden resource collection data in the resource.
// This does not satisfy the Stmt interface.
type StmtResCollect struct {
	//Textarea
	data *interfaces.Data

	Kind     string
	Value    interfaces.Expr
	valuePtr interfaces.Func // ptr for table lookup
}

// String returns a short representation of this statement.
func (obj *StmtResCollect) String() string {
	// TODO: add .String() for Condition and Value
	return fmt.Sprintf("rescollect(%s)", obj.Kind)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtResCollect) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Value.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtResCollect) Init(data *interfaces.Data) error {
	obj.data = data
	//obj.Textarea.Setup(data)

	if obj.Kind == "" {
		return fmt.Errorf("res kind is empty")
	}

	return obj.Value.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// This interpolate is different It is different from the interpolate found in
// the Expr and Stmt interfaces because it returns a different type as output.
func (obj *StmtResCollect) Interpolate() (StmtResContents, error) {
	interpolated, err := obj.Value.Interpolate()
	if err != nil {
		return nil, err
	}
	return &StmtResCollect{
		//Textarea: obj.Textarea,
		data:  obj.data,
		Kind:  obj.Kind,
		Value: interpolated,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtResCollect) Copy() (StmtResContents, error) {
	copied := false
	value, err := obj.Value.Copy()
	if err != nil {
		return nil, err
	}
	if value != obj.Value { // must have been copied, or pointer would be same
		copied = true
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtResCollect{
		//Textarea: obj.Textarea,
		data:  obj.data,
		Kind:  obj.Kind,
		Value: value,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtResCollect) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtrescollectvalue"}
	graph.AddEdge(obj.Value, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)
	nodes := []interfaces.Expr{obj.Value}

	for _, node := range nodes {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtrescollect"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtResCollect) SetScope(scope *interfaces.Scope) error {
	if err := obj.Value.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
		return err
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions. It is different from the TypeCheck method
// found in the Stmt interface because it adds an input parameter.
func (obj *StmtResCollect) TypeCheck(kind string) ([]*interfaces.UnificationInvariant, error) {
	typ, invariants, err := obj.Value.Infer()
	if err != nil {
		return nil, err
	}

	//invars, err := obj.Value.Check(typ) // don't call this here!

	if !engine.IsKind(kind) {
		return nil, fmt.Errorf("invalid resource kind: %s", kind)
	}

	typExpr := types.NewType(funcs.CollectFuncOutType)
	if typExpr == nil {
		return nil, fmt.Errorf("unexpected nil type")
	}

	// regular scenario
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Value,
		Expect: typExpr,
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the resources created, but rather, once all the values (expressions) with
// no outgoing edges have produced at least a single value, then the resources
// know they're able to be built.
func (obj *StmtResCollect) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("rescollect")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.Value.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)
	obj.valuePtr = f

	return graph, nil
}

// StmtEdge is a representation of a dependency. It also supports send/recv.
// Edges represents that the first resource (Kind/Name) listed in the
// EdgeHalfList should happen in the resource graph *before* the next resource
// in the list. If there are multiple StmtEdgeHalf structs listed, then they
// should represent a chain, eg: a->b->c, should compile into a->b & b->c. If
// specified, values are sent and received along these edges if the Send/Recv
// names are compatible and listed. In this case of Send/Recv, only lists of
// length two are legal.
type StmtEdge struct {
	interfaces.Textarea

	data *interfaces.Data

	EdgeHalfList []*StmtEdgeHalf // represents a chain of edges

	// TODO: should notify be an Expr?
	Notify bool // specifies that this edge sends a notification as well
}

// String returns a short representation of this statement.
func (obj *StmtEdge) String() string {
	return "edge" // TODO: improve this
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtEdge) Apply(fn func(interfaces.Node) error) error {
	for _, x := range obj.EdgeHalfList {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtEdge) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	for _, x := range obj.EdgeHalfList {
		if err := x.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// TODO: could we expand the Name's from the EdgeHalf (if they're lists) to have
// them return a list of Edges's ?
// XXX: type check the kind1:send -> kind2:recv fields are compatible!
// XXX: we won't know the names yet, but it's okay.
func (obj *StmtEdge) Interpolate() (interfaces.Stmt, error) {
	edgeHalfList := []*StmtEdgeHalf{}
	for _, x := range obj.EdgeHalfList {
		edgeHalf, err := x.Interpolate()
		if err != nil {
			return nil, err
		}
		edgeHalfList = append(edgeHalfList, edgeHalf)
	}

	return &StmtEdge{
		Textarea:     obj.Textarea,
		data:         obj.data,
		EdgeHalfList: edgeHalfList,
		Notify:       obj.Notify,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtEdge) Copy() (interfaces.Stmt, error) {
	copied := false
	edgeHalfList := []*StmtEdgeHalf{}
	for _, x := range obj.EdgeHalfList {
		edgeHalf, err := x.Copy()
		if err != nil {
			return nil, err
		}
		if edgeHalf != x { // must have been copied, or pointer would be same
			copied = true
		}
		edgeHalfList = append(edgeHalfList, edgeHalf)
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtEdge{
		Textarea:     obj.Textarea,
		data:         obj.data,
		EdgeHalfList: edgeHalfList,
		Notify:       obj.Notify,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtEdge) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)

	for _, edgeHalf := range obj.EdgeHalfList {
		node := edgeHalf.Name
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtedgehalf"}
		graph.AddEdge(node, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtedge"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtEdge) SetScope(scope *interfaces.Scope) error {
	for _, x := range obj.EdgeHalfList {
		if err := x.SetScope(scope); err != nil {
			return err
		}
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtEdge) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	// XXX: Should we check the edge lengths here?

	// TODO: this sort of sideloaded validation could happen in a dedicated
	// Validate() function, but for now is here for lack of a better place!
	if len(obj.EdgeHalfList) == 1 {
		return nil, fmt.Errorf("can't create an edge with only one half")
	}
	if len(obj.EdgeHalfList) == 2 {
		sr1 := obj.EdgeHalfList[0].SendRecv
		sr2 := obj.EdgeHalfList[1].SendRecv
		if (sr1 == "") != (sr2 == "") { // xor
			return nil, fmt.Errorf("you must specify both send/recv fields or neither")
		}

		if sr1 != "" && sr2 != "" {
			k1 := obj.EdgeHalfList[0].Kind
			k2 := obj.EdgeHalfList[1].Kind

			r1, err := engine.NewResource(k1)
			if err != nil {
				return nil, err
			}
			r2, err := engine.NewResource(k2)
			if err != nil {
				return nil, err
			}
			res1, ok := r1.(engine.SendableRes)
			if !ok {
				return nil, fmt.Errorf("cannot send from resource of kind: %s", k1)
			}
			res2, ok := r2.(engine.RecvableRes)
			if !ok {
				return nil, fmt.Errorf("cannot recv to resource of kind: %s", k2)
			}

			// Check that the kind1:send -> kind2:recv fields are type
			// compatible! We won't know the names yet, but it's okay.
			if err := engineUtil.StructFieldCompat(res1.Sends(), sr1, res2, sr2); err != nil {
				p1 := k1 // print defaults
				p2 := k2
				if v, err := obj.EdgeHalfList[0].Name.Value(); err == nil { // statically known
					// display something nicer
					if v.Type().Kind == types.KindStr {
						p1 = engine.Repr(k1, v.Str())
					} else if v.Type().Cmp(types.TypeListStr) == nil {
						p1 = engine.Repr(k1, v.String())
					}
				}
				if v, err := obj.EdgeHalfList[1].Name.Value(); err == nil {
					if v.Type().Kind == types.KindStr {
						p2 = engine.Repr(k2, v.Str())
					} else if v.Type().Cmp(types.TypeListStr) == nil {
						p2 = engine.Repr(k2, v.String())
					}
				}
				return nil, errwrap.Wrapf(err, "cannot send/recv from %s.%s to %s.%s", p1, sr1, p2, sr2)
			}
		}
	}

	invariants := []*interfaces.UnificationInvariant{}

	for _, x := range obj.EdgeHalfList {
		if x.SendRecv != "" && len(obj.EdgeHalfList) != 2 { // XXX: mod 2?
			return nil, fmt.Errorf("send/recv edges must come in pairs")
		}

		invars, err := x.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the edges created, but rather, once all the values (expressions) with no
// outgoing function graph edges have produced at least a single value, then the
// edges know they're able to be built.
func (obj *StmtEdge) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("edge")
	if err != nil {
		return nil, err
	}

	for _, x := range obj.EdgeHalfList {
		g, err := x.Graph(env)
		if err != nil {
			return nil, err
		}
		graph.AddGraph(g)
	}

	return graph, nil
}

// Output returns the output that this "program" produces. This output is what
// is used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output. In
// the case of this edge statement, this is definitely the case. This edge stmt
// returns output consisting of edges.
func (obj *StmtEdge) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	edges := []*interfaces.Edge{}

	// EdgeHalfList goes in a chain, so we increment like i++ and not i+=2.
	for i := 0; i < len(obj.EdgeHalfList)-1; i++ {
		if obj.EdgeHalfList[i].namePtr == nil {
			return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
		}
		nameValue1, exists := table[obj.EdgeHalfList[i].namePtr]
		if !exists {
			return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
		}

		// the edge name can be a single string or a list of strings...

		names1 := []string{} // list of names to build
		switch {
		case types.TypeStr.Cmp(nameValue1.Type()) == nil:
			name := nameValue1.Str() // must not panic
			names1 = append(names1, name)

		case types.TypeListStr.Cmp(nameValue1.Type()) == nil:
			for _, x := range nameValue1.List() { // must not panic
				name := x.Str() // must not panic
				names1 = append(names1, name)
			}

		default:
			// programming error
			return nil, fmt.Errorf("unhandled resource name type: %+v", nameValue1.Type())
		}

		if obj.EdgeHalfList[i+1].namePtr == nil {
			return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
		}
		nameValue2, exists := table[obj.EdgeHalfList[i+1].namePtr]
		if !exists {
			return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
		}

		names2 := []string{} // list of names to build
		switch {
		case types.TypeStr.Cmp(nameValue2.Type()) == nil:
			name := nameValue2.Str() // must not panic
			names2 = append(names2, name)

		case types.TypeListStr.Cmp(nameValue2.Type()) == nil:
			for _, x := range nameValue2.List() { // must not panic
				name := x.Str() // must not panic
				names2 = append(names2, name)
			}

		default:
			// programming error
			return nil, fmt.Errorf("unhandled resource name type: %+v", nameValue2.Type())
		}

		for _, name1 := range names1 {
			for _, name2 := range names2 {
				edge := &interfaces.Edge{
					Kind1: obj.EdgeHalfList[i].Kind,
					Name1: name1,
					Send:  obj.EdgeHalfList[i].SendRecv,

					Kind2: obj.EdgeHalfList[i+1].Kind,
					Name2: name2,
					Recv:  obj.EdgeHalfList[i+1].SendRecv,

					Notify: obj.Notify,
				}
				edges = append(edges, edge)
			}
		}
	}

	return &interfaces.Output{
		Edges: edges,
	}, nil
}

// StmtEdgeHalf represents half of an edge in the parsed edge representation.
// This does not satisfy the Stmt interface. The `Name` value can be a single
// string or a list of strings. The former will produce a single edge half, the
// latter produces a list of resources. Using this list mechanism is a safe
// alternative to traditional flow control like `for` loops. The `Name` value
// can only be a single string when it can be detected statically. Otherwise, it
// is assumed that a list of strings should be expected. More mechanisms to
// determine if the value is static may be added over time.
type StmtEdgeHalf struct {
	interfaces.Textarea

	data *interfaces.Data

	Kind     string          // kind of resource, eg: pkg, file, svc, etc...
	Name     interfaces.Expr // unique name for the res of this kind
	namePtr  interfaces.Func // ptr for table lookup
	SendRecv string          // name of field to send/recv from/to, empty to ignore
}

// String returns a short representation of this statement.
func (obj *StmtEdgeHalf) String() string {
	// TODO: add .String() for Name
	return fmt.Sprintf("edgehalf(%s)", obj.Kind)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtEdgeHalf) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Name.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtEdgeHalf) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)
	if obj.Kind == "" {
		return fmt.Errorf("edge half kind is empty")
	}
	if strings.Contains(obj.Kind, "_") {
		return fmt.Errorf("kind must not contain underscores")
	}

	return obj.Name.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// This interpolate is different It is different from the interpolate found in
// the Expr and Stmt interfaces because it returns a different type as output.
func (obj *StmtEdgeHalf) Interpolate() (*StmtEdgeHalf, error) {
	name, err := obj.Name.Interpolate()
	if err != nil {
		return nil, err
	}

	return &StmtEdgeHalf{
		Textarea: obj.Textarea,
		Kind:     obj.Kind,
		Name:     name,
		SendRecv: obj.SendRecv,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtEdgeHalf) Copy() (*StmtEdgeHalf, error) {
	copied := false
	name, err := obj.Name.Copy()
	if err != nil {
		return nil, err
	}
	if name != obj.Name { // must have been copied, or pointer would be same
		copied = true
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtEdgeHalf{
		Textarea: obj.Textarea,
		Kind:     obj.Kind,
		Name:     name,
		SendRecv: obj.SendRecv,
	}, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtEdgeHalf) SetScope(scope *interfaces.Scope) error {
	return obj.Name.SetScope(scope, map[string]interfaces.Expr{})
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtEdgeHalf) TypeCheck() ([]*interfaces.UnificationInvariant, error) {

	if obj.Kind == "" {
		return nil, fmt.Errorf("missing resource kind in edge")
	}

	typ, invariants, err := obj.Name.Infer()
	if err != nil {
		return nil, err
	}

	if obj.SendRecv != "" {
		// FIXME: write this function (get expected type of field)
		//invar, err := StructFieldInvariant(obj.Kind, obj.SendRecv)
		//if err != nil {
		//	return nil, err
		//}
		//invariants = append(invariants, invar...)
	}

	// Optimization: If we know it's an str, no need for exclusives!
	// TODO: Check other cases, like if it's a function call, and we know it
	// can only return a single string. (Eg: fmt.printf for example.)
	isString := false
	if _, ok := obj.Name.(*ExprStr); ok {
		// It's a string! (A plain string was specified.)
		isString = true
	}
	if typ, err := obj.Name.Type(); err == nil {
		// It has type of string! (Might be an interpolation specified.)
		if typ.Cmp(types.TypeStr) == nil {
			isString = true
		}
	}

	typExpr := types.TypeListStr // default

	// If we pass here, we only allow []str, no need for exclusives!
	if isString {
		typExpr = types.TypeStr
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Name,
		Expect: typExpr, // the name
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. It is interesting to note that nothing directly adds an edge
// to the resources created, but rather, once all the values (expressions) with
// no outgoing edges have produced at least a single value, then the resources
// know they're able to be built.
func (obj *StmtEdgeHalf) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	g, f, err := obj.Name.Graph(env)
	if err != nil {
		return nil, err
	}
	obj.namePtr = f
	return g, nil
}

// StmtIf represents an if condition that contains between one and two branches
// of statements to be executed based on the evaluation of the boolean condition
// over time. In particular, this is different from an ExprIf which returns a
// value, where as this produces some Output. Normally if one of the branches is
// optional, it is the else branch, although this struct allows either to be
// optional, even if it is not commonly used.
type StmtIf struct {
	interfaces.Textarea

	data *interfaces.Data

	Condition    interfaces.Expr
	conditionPtr interfaces.Func // ptr for table lookup
	ThenBranch   interfaces.Stmt // optional, but usually present
	ElseBranch   interfaces.Stmt // optional
}

// String returns a short representation of this statement.
func (obj *StmtIf) String() string {
	s := fmt.Sprintf("if( %s )", obj.Condition.String())

	if obj.ThenBranch != nil {
		s += fmt.Sprintf(" { %s }", obj.ThenBranch.String())
	} else {
		s += " { }"
	}
	if obj.ElseBranch != nil {
		s += fmt.Sprintf(" else { %s }", obj.ElseBranch.String())
	}

	return s
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtIf) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Condition.Apply(fn); err != nil {
		return err
	}
	if obj.ThenBranch != nil {
		if err := obj.ThenBranch.Apply(fn); err != nil {
			return err
		}
	}
	if obj.ElseBranch != nil {
		if err := obj.ElseBranch.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtIf) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if err := obj.Condition.Init(data); err != nil {
		return err
	}
	if obj.ThenBranch != nil {
		if err := obj.ThenBranch.Init(data); err != nil {
			return err
		}
	}
	if obj.ElseBranch != nil {
		if err := obj.ElseBranch.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtIf) Interpolate() (interfaces.Stmt, error) {
	condition, err := obj.Condition.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate Condition")
	}
	var thenBranch interfaces.Stmt
	if obj.ThenBranch != nil {
		thenBranch, err = obj.ThenBranch.Interpolate()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not interpolate ThenBranch")
		}
	}
	var elseBranch interfaces.Stmt
	if obj.ElseBranch != nil {
		elseBranch, err = obj.ElseBranch.Interpolate()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not interpolate ElseBranch")
		}
	}
	return &StmtIf{
		Textarea:   obj.Textarea,
		data:       obj.data,
		Condition:  condition,
		ThenBranch: thenBranch,
		ElseBranch: elseBranch,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtIf) Copy() (interfaces.Stmt, error) {
	copied := false
	condition, err := obj.Condition.Copy()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not copy Condition")
	}
	if condition != obj.Condition { // must have been copied, or pointer would be same
		copied = true
	}

	var thenBranch interfaces.Stmt
	if obj.ThenBranch != nil {
		thenBranch, err = obj.ThenBranch.Copy()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not copy ThenBranch")
		}
		if thenBranch != obj.ThenBranch {
			copied = true
		}
	}
	var elseBranch interfaces.Stmt
	if obj.ElseBranch != nil {
		elseBranch, err = obj.ElseBranch.Copy()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not copy ElseBranch")
		}
		if elseBranch != obj.ElseBranch {
			copied = true
		}
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtIf{
		Textarea:   obj.Textarea,
		data:       obj.data,
		Condition:  condition,
		ThenBranch: thenBranch,
		ElseBranch: elseBranch,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtIf) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraints: We know the condition has to be satisfied
	// before this if statement itself can be used, since we depend on that
	// value.
	edge := &pgraph.SimpleEdge{Name: "stmtif"}
	graph.AddEdge(obj.Condition, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Condition.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "stmtifcondition"}
		graph.AddEdge(n, k, edge)
	}

	nodes := []interfaces.Stmt{}
	if obj.ThenBranch != nil {
		nodes = append(nodes, obj.ThenBranch)

		// additional constraints...
		edge1 := &pgraph.SimpleEdge{Name: "stmtifthencondition"}
		graph.AddEdge(obj.Condition, obj.ThenBranch, edge1) // prod -> cons
		edge2 := &pgraph.SimpleEdge{Name: "stmtifthen"}
		graph.AddEdge(obj.ThenBranch, obj, edge2) // prod -> cons
	}
	if obj.ElseBranch != nil {
		nodes = append(nodes, obj.ElseBranch)

		// additional constraints...
		edge1 := &pgraph.SimpleEdge{Name: "stmtifelsecondition"}
		graph.AddEdge(obj.Condition, obj.ElseBranch, edge1) // prod -> cons
		edge2 := &pgraph.SimpleEdge{Name: "stmtifelse"}
		graph.AddEdge(obj.ElseBranch, obj, edge2) // prod -> cons
	}

	for _, node := range nodes { // "dry"
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtifbranch"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtIf) SetScope(scope *interfaces.Scope) error {
	if err := obj.Condition.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
		return err
	}
	if obj.ThenBranch != nil {
		if err := obj.ThenBranch.SetScope(scope); err != nil {
			return err
		}
	}
	if obj.ElseBranch != nil {
		if err := obj.ElseBranch.SetScope(scope); err != nil {
			return err
		}
	}
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtIf) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	// Don't call obj.Condition.Check here!
	typ, invariants, err := obj.Condition.Infer()
	if err != nil {
		return nil, err
	}

	typExpr := types.TypeBool // default
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Condition,
		Expect: typExpr, // the condition
		Actual: typ,
	}
	invariants = append(invariants, invar)

	if obj.ThenBranch != nil {
		invars, err := obj.ThenBranch.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	if obj.ElseBranch != nil {
		invars, err := obj.ElseBranch.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular if statement doesn't do anything clever here
// other than adding in both branches of the graph. Since we're functional, this
// shouldn't have any ill effects.
// XXX: is this completely true if we're running technically impure, but safe
// built-in functions on both branches? Can we turn off half of this?
func (obj *StmtIf) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("if")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.Condition.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)
	obj.conditionPtr = f

	for _, x := range []interfaces.Stmt{obj.ThenBranch, obj.ElseBranch} {
		if x == nil {
			continue
		}
		g, err := x.Graph(env)
		if err != nil {
			return nil, err
		}
		graph.AddGraph(g)
	}

	return graph, nil
}

// Output returns the output that this "program" produces. This output is what
// is used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output.
func (obj *StmtIf) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	if obj.conditionPtr == nil {
		return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
	}
	b, exists := table[obj.conditionPtr]
	if !exists {
		return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
	}

	var output *interfaces.Output
	var err error
	if b.Bool() { // must not panic!
		if obj.ThenBranch != nil { // logically then branch is optional
			output, err = obj.ThenBranch.Output(table)
		}
	} else {
		if obj.ElseBranch != nil { // else branch is optional
			output, err = obj.ElseBranch.Output(table)
		}
	}
	if err != nil {
		return nil, err
	}

	resources := []engine.Res{}
	edges := []*interfaces.Edge{}
	if output != nil {
		resources = append(resources, output.Resources...)
		edges = append(edges, output.Edges...)
	}

	return &interfaces.Output{
		Resources: resources,
		Edges:     edges,
	}, nil
}

// StmtFor represents an iteration over a list. The body contains statements.
type StmtFor struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	Index string // no $ prefix
	Value string // no $ prefix

	TypeIndex *types.Type
	TypeValue *types.Type

	indexParam *ExprParam
	valueParam *ExprParam

	Expr    interfaces.Expr
	exprPtr interfaces.Func // ptr for table lookup
	Body    interfaces.Stmt // optional, but usually present

	iterBody []interfaces.Stmt
}

// String returns a short representation of this statement.
func (obj *StmtFor) String() string {
	// TODO: improve/change this if needed
	s := fmt.Sprintf("for($%s, $%s)", obj.Index, obj.Value)
	s += fmt.Sprintf(" in %s", obj.Expr.String())
	if obj.Body != nil {
		s += fmt.Sprintf(" { %s }", obj.Body.String())
	}
	return s
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtFor) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Expr.Apply(fn); err != nil {
		return err
	}
	if obj.Body != nil {
		if err := obj.Body.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtFor) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	obj.iterBody = []interfaces.Stmt{}

	if err := obj.Expr.Init(data); err != nil {
		return err
	}
	if obj.Body != nil {
		if err := obj.Body.Init(data); err != nil {
			return err
		}
	}
	// XXX: remove this check if we can!
	for _, stmt := range obj.Body.(*StmtProg).Body {
		if _, ok := stmt.(*StmtImport); !ok {
			continue
		}
		return fmt.Errorf("a StmtImport can't be contained inside a StmtFor")
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtFor) Interpolate() (interfaces.Stmt, error) {
	expr, err := obj.Expr.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate Expr")
	}
	var body interfaces.Stmt
	if obj.Body != nil {
		body, err = obj.Body.Interpolate()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not interpolate Body")
		}
	}
	return &StmtFor{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope, // XXX: Should we copy/include this here?

		Index: obj.Index,
		Value: obj.Value,

		TypeIndex: obj.TypeIndex,
		TypeValue: obj.TypeValue,

		indexParam: obj.indexParam, // XXX: Should we copy/include this here?
		valueParam: obj.valueParam, // XXX: Should we copy/include this here?

		Expr:    expr,
		exprPtr: obj.exprPtr, // XXX: Should we copy/include this here?
		Body:    body,

		iterBody: obj.iterBody, // XXX: Should we copy/include this here?
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtFor) Copy() (interfaces.Stmt, error) {
	copied := false
	expr, err := obj.Expr.Copy()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not copy Expr")
	}
	if expr != obj.Expr { // must have been copied, or pointer would be same
		copied = true
	}

	var body interfaces.Stmt
	if obj.Body != nil {
		body, err = obj.Body.Copy()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not copy Body")
		}
		if body != obj.Body {
			copied = true
		}
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtFor{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope, // XXX: Should we copy/include this here?

		Index: obj.Index,
		Value: obj.Value,

		TypeIndex: obj.TypeIndex,
		TypeValue: obj.TypeValue,

		indexParam: obj.indexParam, // XXX: Should we copy/include this here?
		valueParam: obj.valueParam, // XXX: Should we copy/include this here?

		Expr:    expr,
		exprPtr: obj.exprPtr, // XXX: Should we copy/include this here?
		Body:    body,

		iterBody: obj.iterBody, // XXX: Should we copy/include this here?
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtFor) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraints: We know the condition has to be satisfied
	// before this for statement itself can be used, since we depend on that
	// value.
	edge := &pgraph.SimpleEdge{Name: "stmtforexpr1"}
	graph.AddEdge(obj.Expr, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Expr.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "stmtforexpr2"}
		graph.AddEdge(n, k, edge)
	}

	if obj.Body == nil { // return early
		return graph, cons, nil
	}

	// additional constraints...
	edge1 := &pgraph.SimpleEdge{Name: "stmtforbodyexpr"}
	graph.AddEdge(obj.Expr, obj.Body, edge1) // prod -> cons
	edge2 := &pgraph.SimpleEdge{Name: "stmtforbody1"}
	graph.AddEdge(obj.Body, obj, edge2) // prod -> cons

	nodes := []interfaces.Stmt{obj.Body} // XXX: are there more to add?

	for _, node := range nodes { // "dry"
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtforbody2"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtFor) SetScope(scope *interfaces.Scope) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope // store for later

	if err := obj.Expr.SetScope(scope, map[string]interfaces.Expr{}); err != nil { // XXX: empty sctx?
		return err
	}

	if obj.Body == nil { // no loop body, we're done early
		return nil
	}

	// We need to build the two ExprParam's here, and those will contain the
	// type unification variables, so we might as well populate those parts
	// now, rather than waiting for the subsequent TypeCheck step.

	typExprIndex := obj.TypeIndex
	if obj.TypeIndex == nil {
		typExprIndex = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}
	// We know this one is types.TypeInt, but we only officially determine
	// that in the subsequent TypeCheck step since we need to relate things
	// to the input param so it can be easily solved if it's a variable...
	obj.indexParam = newExprParam(
		obj.Index,
		typExprIndex,
	)

	typExprValue := obj.TypeValue
	if obj.TypeValue == nil {
		typExprValue = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}
	obj.valueParam = newExprParam(
		obj.Value,
		typExprValue,
	)

	newScope := scope.Copy()
	newScope.Iterated = true // important!
	newScope.Variables[obj.Index] = obj.indexParam
	newScope.Variables[obj.Value] = obj.valueParam

	return obj.Body.SetScope(newScope)
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtFor) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	// Don't call obj.Expr.Check here!
	typ, invariants, err := obj.Expr.Infer()
	if err != nil {
		return nil, err
	}

	// The type unification variables get created in SetScope! (If needed!)
	typExprIndex := obj.indexParam.typ
	typExprValue := obj.valueParam.typ

	typExpr := &types.Type{
		Kind: types.KindList,
		Val:  typExprValue,
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Expr,
		Expect: typExpr, // the list
		Actual: typ,
	}
	invariants = append(invariants, invar)

	// The following two invariants are needed to ensure the ExprParam's are
	// added to the unification solver so that we actually benefit from that
	// relationship and solution!
	invarIndex := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.indexParam,
		Expect: typExprIndex,  // the list index type
		Actual: types.TypeInt, // here we finally also say it's an int!
	}
	invariants = append(invariants, invarIndex)

	invarValue := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.valueParam,
		Expect: typExprValue, // the list element type
		Actual: typExprValue,
	}
	invariants = append(invariants, invarValue)

	if obj.Body != nil {
		invars, err := obj.Body.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular for statement has lots of complex magic to
// make it all work.
func (obj *StmtFor) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("for")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.Expr.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)
	obj.exprPtr = f

	if obj.Body == nil { // no loop body, we're done early
		return graph, nil
	}

	mutex := &sync.Mutex{}

	// This gets called once per iteration, each time the list changes.
	appendToIterBody := func(innerTxn interfaces.Txn, index int, value interfaces.Func) error {
		// Extend the environment with the two loop variables.
		extendedEnv := env.Copy()

		// calling convention
		extendedEnv.Variables[obj.indexParam.envKey] = &interfaces.FuncSingleton{
			MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
				f := &structs.ConstFunc{
					Textarea: obj.Textarea, // XXX: advance by `for ` chars?

					Value: &types.IntValue{
						V: int64(index),
					},
					NameHint: obj.Index, // XXX: is this right?
				}
				g, err := pgraph.NewGraph("g")
				if err != nil {
					return nil, nil, err
				}
				g.AddVertex(f)
				return g, f, nil
			},
		}

		// XXX: create the function in ForFunc instead?
		//extendedEnv.Variables[obj.Index] = index
		//extendedEnv.Variables[obj.Value] = value
		//extendedEnv.Variables[obj.valueParam.envKey] = value
		extendedEnv.Variables[obj.valueParam.envKey] = &interfaces.FuncSingleton{ // XXX: We could set this one statically
			MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
				f := value
				g, err := pgraph.NewGraph("g")
				if err != nil {
					return nil, nil, err
				}
				g.AddVertex(f)
				return g, f, nil
			},
		}

		// NOTE: We previously considered doing a "copy singletons" here
		// instead, but decided we didn't need it after all.
		body, err := obj.Body.Copy()
		if err != nil {
			return err
		}

		mutex.Lock()
		obj.iterBody = append(obj.iterBody, body)
		// TODO: Can we avoid using append and do it this way instead?
		//obj.iterBody[index] = body
		mutex.Unlock()

		// Create a subgraph from the lambda's body, instantiating the
		// lambda's parameters with the args and the other variables
		// with the nodes in the captured environment.
		subgraph, err := body.Graph(extendedEnv)
		if err != nil {
			return errwrap.Wrapf(err, "could not create the lambda body's subgraph")
		}

		innerTxn.AddGraph(subgraph)

		// We don't need an output func because body.Graph is a
		// statement and it doesn't return an interfaces.Func,
		// only the expression versions return those!
		return nil
	}

	// Add a vertex for the list passing itself.
	edgeName := structs.ForFuncArgNameList
	forFunc := &structs.ForFunc{
		Textarea: obj.Textarea,

		IndexType: obj.indexParam.typ,
		ValueType: obj.valueParam.typ,

		EdgeName: edgeName,

		AppendToIterBody: appendToIterBody,
		ClearIterBody: func(length int) { // XXX: use length?
			mutex.Lock()
			obj.iterBody = []interfaces.Stmt{}
			mutex.Unlock()
		},
	}
	graph.AddVertex(forFunc)
	graph.AddEdge(f, forFunc, &interfaces.FuncEdge{
		Args: []string{edgeName},
	})

	return graph, nil
}

// Output returns the output that this "program" produces. This output is what
// is used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output.
func (obj *StmtFor) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	if obj.exprPtr == nil {
		return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
	}
	expr, exists := table[obj.exprPtr]
	if !exists {
		return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
	}

	if obj.Body == nil { // logically body is optional
		return &interfaces.Output{}, nil // XXX: test this doesn't panic anything
	}

	resources := []engine.Res{}
	edges := []*interfaces.Edge{}

	list := expr.List() // must not panic!

	for index := range list {
		// index is a golang int, value is an mcl types.Value
		// XXX: Do we need a mutex around this iterBody access?
		output, err := obj.iterBody[index].Output(table)
		if err != nil {
			return nil, err
		}

		if output != nil {
			resources = append(resources, output.Resources...)
			edges = append(edges, output.Edges...)
		}
	}

	return &interfaces.Output{
		Resources: resources,
		Edges:     edges,
	}, nil
}

// StmtForKV represents an iteration over a map. The body contains statements.
type StmtForKV struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	Key string // no $ prefix
	Val string // no $ prefix

	TypeKey *types.Type
	TypeVal *types.Type

	keyParam *ExprParam
	valParam *ExprParam

	Expr    interfaces.Expr
	exprPtr interfaces.Func // ptr for table lookup
	Body    interfaces.Stmt // optional, but usually present

	iterBody map[types.Value]interfaces.Stmt
}

// String returns a short representation of this statement.
func (obj *StmtForKV) String() string {
	// TODO: improve/change this if needed
	s := fmt.Sprintf("forkv($%s, $%s)", obj.Key, obj.Val)
	s += fmt.Sprintf(" in %s", obj.Expr.String())
	if obj.Body != nil {
		s += fmt.Sprintf(" { %s }", obj.Body.String())
	}
	return s
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtForKV) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Expr.Apply(fn); err != nil {
		return err
	}
	if obj.Body != nil {
		if err := obj.Body.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtForKV) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	obj.iterBody = make(map[types.Value]interfaces.Stmt)

	if err := obj.Expr.Init(data); err != nil {
		return err
	}
	if obj.Body != nil {
		if err := obj.Body.Init(data); err != nil {
			return err
		}
	}
	// XXX: remove this check if we can!
	for _, stmt := range obj.Body.(*StmtProg).Body {
		if _, ok := stmt.(*StmtImport); !ok {
			continue
		}
		return fmt.Errorf("a StmtImport can't be contained inside a StmtForKV")
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtForKV) Interpolate() (interfaces.Stmt, error) {
	expr, err := obj.Expr.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate Expr")
	}
	var body interfaces.Stmt
	if obj.Body != nil {
		body, err = obj.Body.Interpolate()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not interpolate Body")
		}
	}
	return &StmtForKV{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope, // XXX: Should we copy/include this here?

		Key: obj.Key,
		Val: obj.Val,

		TypeKey: obj.TypeKey,
		TypeVal: obj.TypeVal,

		keyParam: obj.keyParam, // XXX: Should we copy/include this here?
		valParam: obj.valParam, // XXX: Should we copy/include this here?

		Expr:    expr,
		exprPtr: obj.exprPtr, // XXX: Should we copy/include this here?
		Body:    body,

		iterBody: obj.iterBody, // XXX: Should we copy/include this here?
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtForKV) Copy() (interfaces.Stmt, error) {
	copied := false
	expr, err := obj.Expr.Copy()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not copy Expr")
	}
	if expr != obj.Expr { // must have been copied, or pointer would be same
		copied = true
	}

	var body interfaces.Stmt
	if obj.Body != nil {
		body, err = obj.Body.Copy()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not copy Body")
		}
		if body != obj.Body {
			copied = true
		}
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtForKV{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope, // XXX: Should we copy/include this here?

		Key: obj.Key,
		Val: obj.Val,

		TypeKey: obj.TypeKey,
		TypeVal: obj.TypeVal,

		keyParam: obj.keyParam, // XXX: Should we copy/include this here?
		valParam: obj.valParam, // XXX: Should we copy/include this here?

		Expr:    expr,
		exprPtr: obj.exprPtr, // XXX: Should we copy/include this here?
		Body:    body,

		iterBody: obj.iterBody, // XXX: Should we copy/include this here?
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtForKV) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraints: We know the condition has to be satisfied
	// before this for statement itself can be used, since we depend on that
	// value.
	edge := &pgraph.SimpleEdge{Name: "stmtforkvexpr1"}
	graph.AddEdge(obj.Expr, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Expr.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "stmtforkvexpr2"}
		graph.AddEdge(n, k, edge)
	}

	if obj.Body == nil { // return early
		return graph, cons, nil
	}

	// additional constraints...
	edge1 := &pgraph.SimpleEdge{Name: "stmtforkvbodyexpr"}
	graph.AddEdge(obj.Expr, obj.Body, edge1) // prod -> cons
	edge2 := &pgraph.SimpleEdge{Name: "stmtforkvbody1"}
	graph.AddEdge(obj.Body, obj, edge2) // prod -> cons

	nodes := []interfaces.Stmt{obj.Body} // XXX: are there more to add?

	for _, node := range nodes { // "dry"
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtforkvbody2"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtForKV) SetScope(scope *interfaces.Scope) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope // store for later

	if err := obj.Expr.SetScope(scope, map[string]interfaces.Expr{}); err != nil { // XXX: empty sctx?
		return err
	}

	if obj.Body == nil { // no loop body, we're done early
		return nil
	}

	// We need to build the two ExprParam's here, and those will contain the
	// type unification variables, so we might as well populate those parts
	// now, rather than waiting for the subsequent TypeCheck step.

	typExprKey := obj.TypeKey
	if obj.TypeKey == nil {
		typExprKey = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}
	obj.keyParam = newExprParam(
		obj.Key,
		typExprKey,
	)

	typExprVal := obj.TypeVal
	if obj.TypeVal == nil {
		typExprVal = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}
	obj.valParam = newExprParam(
		obj.Val,
		typExprVal,
	)

	newScope := scope.Copy()
	newScope.Iterated = true // important!
	newScope.Variables[obj.Key] = obj.keyParam
	newScope.Variables[obj.Val] = obj.valParam

	return obj.Body.SetScope(newScope)
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtForKV) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	// Don't call obj.Expr.Check here!
	typ, invariants, err := obj.Expr.Infer()
	if err != nil {
		return nil, err
	}

	// The type unification variables get created in SetScope! (If needed!)
	typExprKey := obj.keyParam.typ
	typExprVal := obj.valParam.typ

	typExpr := &types.Type{
		Kind: types.KindMap,
		Key:  typExprKey,
		Val:  typExprVal,
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Expr,
		Expect: typExpr, // the map
		Actual: typ,
	}
	invariants = append(invariants, invar)

	// The following two invariants are needed to ensure the ExprParam's are
	// added to the unification solver so that we actually benefit from that
	// relationship and solution!
	invarKey := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.keyParam,
		Expect: typExprKey, // the map key type
		Actual: typExprKey, // not necessarily an int!
	}
	invariants = append(invariants, invarKey)

	invarVal := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.valParam,
		Expect: typExprVal, // the map val type
		Actual: typExprVal,
	}
	invariants = append(invariants, invarVal)

	if obj.Body != nil {
		invars, err := obj.Body.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular for statement has lots of complex magic to
// make it all work.
func (obj *StmtForKV) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	graph, err := pgraph.NewGraph("forkv")
	if err != nil {
		return nil, err
	}

	g, f, err := obj.Expr.Graph(env)
	if err != nil {
		return nil, err
	}
	graph.AddGraph(g)
	obj.exprPtr = f

	if obj.Body == nil { // no loop body, we're done early
		return graph, nil
	}

	mutex := &sync.Mutex{}

	// This gets called once per iteration, each time the map changes.
	setOnIterBody := func(innerTxn interfaces.Txn, ptr types.Value, key, val interfaces.Func) error {
		// Extend the environment with the two loop variables.
		extendedEnv := env.Copy()

		// calling convention
		extendedEnv.Variables[obj.keyParam.envKey] = &interfaces.FuncSingleton{
			MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
				f := key
				g, err := pgraph.NewGraph("g")
				if err != nil {
					return nil, nil, err
				}
				g.AddVertex(f)
				return g, f, nil
			},
		}

		// XXX: create the function in ForKVFunc instead?
		extendedEnv.Variables[obj.valParam.envKey] = &interfaces.FuncSingleton{ // XXX: We could set this one statically
			MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
				f := val
				g, err := pgraph.NewGraph("g")
				if err != nil {
					return nil, nil, err
				}
				g.AddVertex(f)
				return g, f, nil
			},
		}

		// NOTE: We previously considered doing a "copy singletons" here
		// instead, but decided we didn't need it after all.
		body, err := obj.Body.Copy()
		if err != nil {
			return err
		}

		mutex.Lock()
		obj.iterBody[ptr] = body
		// XXX: Do we fake our map by giving each key an index too?
		// NOTE: We can't do append since the key might not be an int.
		//obj.iterBody = append(obj.iterBody, body) // not possible
		mutex.Unlock()

		// Create a subgraph from the lambda's body, instantiating the
		// lambda's parameters with the args and the other variables
		// with the nodes in the captured environment.
		subgraph, err := body.Graph(extendedEnv)
		if err != nil {
			return errwrap.Wrapf(err, "could not create the lambda body's subgraph")
		}

		innerTxn.AddGraph(subgraph)

		// We don't need an output func because body.Graph is a
		// statement and it doesn't return an interfaces.Func,
		// only the expression versions return those!
		return nil
	}

	// Add a vertex for the map passing itself.
	edgeName := structs.ForKVFuncArgNameMap
	forKVFunc := &structs.ForKVFunc{
		Textarea: obj.Textarea,

		KeyType: obj.keyParam.typ,
		ValType: obj.valParam.typ,

		EdgeName: edgeName,

		SetOnIterBody: setOnIterBody,
		ClearIterBody: func(length int) { // XXX: use length?
			mutex.Lock()
			obj.iterBody = map[types.Value]interfaces.Stmt{}
			mutex.Unlock()
		},
	}
	graph.AddVertex(forKVFunc)
	graph.AddEdge(f, forKVFunc, &interfaces.FuncEdge{
		Args: []string{edgeName},
	})

	return graph, nil
}

// Output returns the output that this "program" produces. This output is what
// is used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output.
func (obj *StmtForKV) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	if obj.exprPtr == nil {
		return nil, fmt.Errorf("%w: %T", ErrFuncPointerNil, obj)
	}
	expr, exists := table[obj.exprPtr]
	if !exists {
		return nil, fmt.Errorf("%w: %T", ErrTableNoValue, obj)
	}

	if obj.Body == nil { // logically body is optional
		return &interfaces.Output{}, nil // XXX: test this doesn't panic anything
	}

	resources := []engine.Res{}
	edges := []*interfaces.Edge{}

	m := expr.Map() // must not panic!

	for key := range m {
		// key and val are both an mcl types.Value
		// XXX: Do we need a mutex around this iterBody access?
		if _, exists := obj.iterBody[key]; !exists {
			// programming error
			return nil, fmt.Errorf("programming error on key: %s", key)
		}
		output, err := obj.iterBody[key].Output(table)
		if err != nil {
			return nil, err
		}

		if output != nil {
			resources = append(resources, output.Resources...)
			edges = append(edges, output.Edges...)
		}
	}

	return &interfaces.Output{
		Resources: resources,
		Edges:     edges,
	}, nil
}

// StmtProg represents a list of stmt's. This usually occurs at the top-level of
// any program, and often within an if stmt. It also contains the logic so that
// the bind statement's are correctly applied in this scope, and irrespective of
// their order of definition.
type StmtProg struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for use by imports

	// TODO: should this be a map? if so, how would we sort it to loop it?
	importProgs []*StmtProg // list of child programs after running SetScope
	importFiles []string    // list of files seen during the SetScope import

	nodeOrder []interfaces.Stmt // used for .Graph

	Body []interfaces.Stmt
}

// String returns a short representation of this statement.
func (obj *StmtProg) String() string {
	return "prog" // TODO: improve this
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtProg) Apply(fn func(interfaces.Node) error) error {
	for _, x := range obj.Body {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}

	// might as well Apply on these too, to make file collection easier, etc
	for _, x := range obj.importProgs {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtProg) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	obj.importProgs = []*StmtProg{}
	obj.importFiles = []string{}
	obj.nodeOrder = []interfaces.Stmt{}
	for _, x := range obj.Body {
		if err := x.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// This particular implementation can currently modify the source AST in-place,
// and then finally return a copy. This isn't ideal, but it is much more optimal
// as it avoids a lot of copying, and the code is simpler. If we need our AST to
// be static, then we can improve this.
func (obj *StmtProg) Interpolate() (interfaces.Stmt, error) {
	// First, make a list of class name to class pointer.
	classes := make(map[string]*StmtClass)
	for _, x := range obj.Body {
		stmt, ok := x.(*StmtClass)
		if !ok {
			continue
		}
		if _, exists := classes[stmt.Name]; exists {
			return nil, fmt.Errorf("duplicate class name of: `%s`", stmt.Name)
		}
		// if it contains a colon we could skip it (perf busy work)
		//if strings.Contains(stmt.Name, interfaces.ClassSep) {
		//	continue
		//}
		classes[stmt.Name] = stmt
	}

	// Now, loop through (in reverse so that remove will work without
	// breaking the index offset) the body and pull any colon prefixed class
	// into the base class that it belongs inside. We also rename it to pop
	// off the front prefix name once it's inside the new base class. This
	// is all syntactic sugar to implement the class child nesting.
	for i := len(obj.Body) - 1; i >= 0; i-- { // reverse order for remove
		stmt, ok := obj.Body[i].(*StmtClass)
		if !ok || stmt.Name == "" {
			continue
		}

		// equivalent to: strings.Contains(stmt.Name, interfaces.ClassSep)
		split := strings.Split(stmt.Name, interfaces.ClassSep)
		if len(split) == 0 || len(split) == 1 {
			continue
		}
		if split[0] == "" { // prefix, eg: `:foo:bar`
			return nil, fmt.Errorf("class name prefix is empty")
		}

		class, exists := classes[split[0]]
		if !exists {
			continue
		}
		prog, ok := class.Body.(*StmtProg) // probably a *StmtProg
		if !ok {
			// TODO: print warning or error?
			continue
		}

		// It's not ideal to modify things here, but we do since it's so
		// much easier and faster to do it like this. We can use copies
		// if it turns out we need to preserve the original input AST.
		stmt.Name = strings.Join(split[1:], interfaces.ClassSep) // new name w/o prefix
		prog.Body = append(prog.Body, stmt)                      // append it to child body
		obj.Body = append(obj.Body[:i], obj.Body[i+1:]...)       // remove it (from the end)
	}

	// Now perform the normal recursive interpolation calls.
	body := []interfaces.Stmt{}
	for _, x := range obj.Body {
		interpolated, err := x.Interpolate()
		if err != nil {
			return nil, err
		}
		body = append(body, interpolated)
	}
	return &StmtProg{
		Textarea:    obj.Textarea,
		data:        obj.data,
		scope:       obj.scope,
		importProgs: obj.importProgs, // TODO: do we even need this here?
		importFiles: obj.importFiles,
		nodeOrder:   obj.nodeOrder,
		Body:        body,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtProg) Copy() (interfaces.Stmt, error) {
	copied := false
	body := []interfaces.Stmt{}

	m := make(map[interfaces.Stmt]interfaces.Stmt) // mapping

	for _, x := range obj.Body {
		cp, err := x.Copy()
		if err != nil {
			return nil, err
		}
		if cp != x { // must have been copied, or pointer would be same
			copied = true
		}
		body = append(body, cp)
		m[x] = cp // store mapping
	}

	newNodeOrder := []interfaces.Stmt{}
	for _, n := range obj.nodeOrder {
		newNodeOrder = append(newNodeOrder, m[n])
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtProg{
		Textarea:    obj.Textarea,
		data:        obj.data,
		scope:       obj.scope,
		importProgs: obj.importProgs, // TODO: do we even need this here?
		importFiles: obj.importFiles,
		nodeOrder:   newNodeOrder, // we need to update this
		Body:        body,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// The interesting part of the Ordering determination happens right here in
// StmtProg. This first looks at all the children to see what this produces, and
// then it recursively builds the graph by looking into all the children with
// this information from the first pass. We link production and consumption via
// a unique string name which is used to determine flow. Of particular note, all
// of this happens *before* SetScope, so we cannot follow references in the
// scope. The input to this method is a mapping of the the produced unique names
// in the parent "scope", to their associated node pointers. This returns a map
// of what is consumed in the child AST. The map is reversed, because two
// different nodes could consume the same variable key.
// TODO: deal with StmtImport's by returning them as first if necessary?
func (obj *StmtProg) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	prod := make(map[string]interfaces.Node)
	for _, x := range obj.Body {
		if stmt, ok := x.(*StmtImport); ok {
			if stmt.Name == "" {
				return nil, nil, fmt.Errorf("missing class name")
			}
			uid := scopedOrderingPrefix + stmt.Name // ordering id

			if stmt.Alias == interfaces.BareSymbol {
				// XXX: I think we need to parse these first...
				// XXX: Somehow make sure these appear at the
				// top of the topo-sort for the StmtProg...
				// XXX: Maybe add edges between StmtProg and me?
				continue
			}

			if stmt.Alias != "" {
				uid = scopedOrderingPrefix + stmt.Alias // ordering id
			}

			n, exists := prod[uid]
			if exists {
				return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid, n)
			}
			prod[uid] = stmt // store
		}

		if stmt, ok := x.(*StmtBind); ok {
			if stmt.Ident == "" {
				return nil, nil, fmt.Errorf("missing bind name")
			}
			uid := varOrderingPrefix + stmt.Ident // ordering id
			n, exists := prod[uid]
			if exists {
				return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid, n)
			}
			prod[uid] = stmt // store
		}

		if stmt, ok := x.(*StmtFunc); ok {
			if stmt.Name == "" {
				return nil, nil, fmt.Errorf("missing func name")
			}
			uid := funcOrderingPrefix + stmt.Name // ordering id
			n, exists := prod[uid]
			if exists {
				return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid, n)
			}
			prod[uid] = stmt // store
		}

		if stmt, ok := x.(*StmtClass); ok {
			if stmt.Name == "" {
				return nil, nil, fmt.Errorf("missing class name")
			}
			uid := classOrderingPrefix + stmt.Name // ordering id
			n, exists := prod[uid]
			if exists {
				return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid, n)
			}
			prod[uid] = stmt // store
		}

		if stmt, ok := x.(*StmtInclude); ok {
			if stmt.Name == "" {
				return nil, nil, fmt.Errorf("missing include name")
			}
			if stmt.Alias == "" { // not consumed
				continue
			}
			uid := scopedOrderingPrefix + stmt.Alias // ordering id
			n, exists := prod[uid]
			if exists {
				return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid, n)
			}
			prod[uid] = stmt // store
		}

		// XXX: I have no idea if this is needed or is done correctly.
		// XXX: If I add it, it turns this into a dag.
		//if stmt, ok := x.(*StmtFor); ok {
		//	if stmt.Index == "" {
		//		return nil, nil, fmt.Errorf("missing index name")
		//	}
		//	uid1 := varOrderingPrefix + stmt.Index // ordering id
		//	if n, exists := prod[uid1]; exists {
		//		return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid1, n)
		//	}
		//	prod[uid1] = stmt // store
		//
		//	if stmt.Value == "" {
		//		return nil, nil, fmt.Errorf("missing value name")
		//	}
		//	uid2 := varOrderingPrefix + stmt.Value // ordering id
		//	if n, exists := prod[uid2]; exists {
		//		return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid2, n)
		//	}
		//	prod[uid2] = stmt // store
		//}
		//if stmt, ok := x.(*StmtForKV); ok {
		//	if stmt.Key == "" {
		//		return nil, nil, fmt.Errorf("missing index name")
		//	}
		//	uid1 := varOrderingPrefix + stmt.Key // ordering id
		//	if n, exists := prod[uid1]; exists {
		//		return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid1, n)
		//	}
		//	prod[uid1] = stmt // store
		//
		//	if stmt.Val == "" {
		//		return nil, nil, fmt.Errorf("missing val name")
		//	}
		//	uid2 := varOrderingPrefix + stmt.Val // ordering id
		//	if n, exists := prod[uid2]; exists {
		//		return nil, nil, fmt.Errorf("duplicate assignment to `%s`, have: %s", uid2, n)
		//	}
		//	prod[uid2] = stmt // store
		//}
	}

	newProduces := CopyNodeMapping(produces) // don't modify the input map!

	// Overwrite anything in this scope with the shadowed parent variable!
	for key, val := range prod {
		newProduces[key] = val // copy, and overwrite (shadow) any parent var
	}

	cons := make(map[interfaces.Node]string) // swapped!

	for _, node := range obj.Body {
		g, c, err := node.Ordering(newProduces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtprognode"}
		graph.AddEdge(node, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := newProduces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtprog1"}
			// We want the convention to be produces -> consumes.
			graph.AddEdge(n, k, edge)
		}
	}

	// TODO: is this redundant? do we need it?
	for key, val := range newProduces { // string, node
		for x, str := range cons { // node, string
			if key != str {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtprog2"}
			graph.AddEdge(val, x, edge) // prod -> cons
		}
	}

	// The consumes which have already been matched to one of our produces
	// must not be also matched to a produce from our caller. Is that clear?
	newCons := make(map[interfaces.Node]string) // don't modify the input map!
	for k, v := range cons {
		if _, exists := prod[v]; exists {
			continue
		}
		newCons[k] = v // "remaining" values from cons
	}

	return graph, newCons, nil
}

// nextVertex is a helper function that builds a vertex for recursion detection.
func (obj *StmtProg) nextVertex(info *interfaces.ImportData) (*pgraph.SelfVertex, error) {
	// graph-based recursion detection
	// TODO: is this sufficiently unique, but not incorrectly unique?
	// TODO: do we need to clean uvid for consistency so the compare works?
	uvid := obj.data.Base + ";" + info.Name // unique vertex id
	importVertex := obj.data.Imports        // parent vertex
	if importVertex == nil {
		return nil, fmt.Errorf("programming error: missing import vertex")
	}
	importGraph := importVertex.Graph // existing graph (ptr stored within)
	nextVertex := &pgraph.SelfVertex{ // new vertex (if one doesn't already exist)
		Name:  uvid,        // import name
		Graph: importGraph, // store a reference to ourself
	}
	for _, v := range importGraph.VerticesSorted() { // search for one first
		gv, ok := v.(*pgraph.SelfVertex)
		if !ok { // someone misused the vertex
			return nil, fmt.Errorf("programming error: unexpected vertex type")
		}
		if gv.Name == uvid {
			nextVertex = gv // found the same name (use this instead!)
			// this doesn't necessarily mean a cycle. a dag is okay
			break
		}
	}

	// add an edge
	edge := &pgraph.SimpleEdge{Name: ""} // TODO: name me?
	importGraph.AddEdge(importVertex, nextVertex, edge)
	if _, err := importGraph.TopologicalSort(); err != nil {
		// TODO: print the cycle in a prettier way (with file names?)
		obj.data.Logf("import: not a dag:\n%s", importGraph.Sprint())
		return nil, errwrap.Wrapf(err, "recursive import of: `%s`", info.Name)
	}

	return nextVertex, nil
}

// importScope is a helper function called from SetScope. If it can't find a
// particular scope, then it can also run the downloader if it is available.
func (obj *StmtProg) importScope(info *interfaces.ImportData, scope *interfaces.Scope) (*interfaces.Scope, error) {
	if obj.data.Debug {
		obj.data.Logf("import: %s", info.Name)
	}
	// the abs file path that we started actively running SetScope on is:
	// obj.data.Base + obj.data.Metadata.Main
	// but recursive imports mean this is not always the active file...

	// attempt to load an embedded system import first (pure mcl rather than golang)
	if fs, err := embedded.Lookup(info.Name); info.IsSystem && err == nil {
		nextVertex, err := obj.nextVertex(info)
		if err != nil {
			return nil, err
		}

		// Our embedded scope might also have some functions to add in!
		systemScope, err := obj.importSystemScope(info.Name)
		if err != nil {
			return nil, errwrap.Wrapf(err, "embedded system import of `%s` failed", info.Name)
		}
		newScope := scope.Copy()
		if err := newScope.Merge(systemScope); err != nil { // errors if something was overwritten
			// XXX: we get a false positive b/c we overwrite the initial scope!
			// XXX: when we switch to append, this problem will go away...
			//return nil, errwrap.Wrapf(err, "duplicate scope element(s) in module found")
		}

		//tree, err := util.FsTree(fs, "/")
		//if err != nil {
		//	return nil, err
		//}
		//obj.data.Logf("tree:\n%s", tree)

		s := "/" + interfaces.MetadataFilename // standard entry point
		//s := "/" // would this directory parser approach be better?
		input, err := inputs.ParseInput(s, fs) // use my FS
		if err != nil {
			return nil, errwrap.Wrapf(err, "embedded could not activate an input parser")
		}

		// The files we're pulling in are already embedded, so we must
		// not try to copy them in from disk or it won't succeed.
		input.Files = []string{} // clear

		embeddedScope, err := obj.importScopeWithParsedInputs(input, newScope, nextVertex)
		if err != nil {
			return nil, errwrap.Wrapf(err, "embedded import of `%s` failed", info.Name)
		}
		return embeddedScope, nil
	}

	if info.IsSystem { // system imports are the exact name, eg "fmt"
		systemScope, err := obj.importSystemScope(info.Name)
		if err != nil {
			return nil, errwrap.Wrapf(err, "system import of `%s` failed", info.Name)
		}
		return systemScope, nil
	}

	nextVertex, err := obj.nextVertex(info) // everyone below us uses this!
	if err != nil {
		return nil, err
	}

	if info.IsLocal {
		// append the relative addition of where the running code is, on
		// to the base path that the metadata file (data) is relative to
		// if the main code file has no additional directory, then it is
		// okay, because Dirname collapses down to the empty string here
		importFilePath := obj.data.Base + util.Dirname(obj.data.Metadata.Main) + info.Path
		if obj.data.Debug {
			obj.data.Logf("import: file: %s", importFilePath)
		}
		// don't do this collection here, it has moved elsewhere...
		//obj.importFiles = append(obj.importFiles, importFilePath) // save for CollectFiles

		localScope, err := obj.importScopeWithInputs(importFilePath, scope, nextVertex)
		if err != nil {
			return nil, errwrap.Wrapf(err, "local import of `%s` failed", info.Name)
		}
		return localScope, nil
	}

	// Now, info.IsLocal is false... we're dealing with a remote import!

	// This takes the current metadata as input so it can use the Path
	// directory to search upwards if we wanted to look in parent paths.
	// Since this is an fqdn import, it must contain a metadata file...
	modulesPath, err := interfaces.FindModulesPath(obj.data.Metadata, obj.data.Base, obj.data.Modules)
	if err != nil {
		return nil, errwrap.Wrapf(err, "module path error")
	}
	importFilePath := modulesPath + info.Path + interfaces.MetadataFilename

	if !RequireStrictModulePath { // look upwards
		modulesPathList, err := interfaces.FindModulesPathList(obj.data.Metadata, obj.data.Base, obj.data.Modules)
		if err != nil {
			return nil, errwrap.Wrapf(err, "module path list error")
		}
		for _, mp := range modulesPathList { // first one to find a file
			x := mp + info.Path + interfaces.MetadataFilename
			if _, err := obj.data.Fs.Stat(x); err == nil {
				// found a valid location, so keep using it!
				modulesPath = mp
				importFilePath = x
				break
			}
		}
		// If we get here, and we didn't find anything, then we use the
		// originally decided, most "precise" location... The reason we
		// do that is if the sysadmin wishes to require all the modules
		// to come from their top-level (or higher-level) directory, it
		// can be done by adding the code there, so that it is found in
		// the above upwards search. Otherwise, we just do what the mod
		// asked for and use the path/ directory if it wants its own...
	}
	if obj.data.Debug {
		obj.data.Logf("import: modules path: %s", modulesPath)
		obj.data.Logf("import: file: %s", importFilePath)
	}
	// don't do this collection here, it has moved elsewhere...
	//obj.importFiles = append(obj.importFiles, importFilePath) // save for CollectFiles

	// invoke the download when a path is missing, if the downloader exists
	// we need to invoke the recursive checker before we run this download!
	// this should cleverly deal with skipping modules that are up-to-date!
	if obj.data.Downloader != nil {
		// run downloader stuff first
		if err := obj.data.Downloader.Get(info, modulesPath); err != nil {
			obj.data.Logf("download of `%s` failed", info.Name)
			return nil, err
		}
	}

	// takes the full absolute path to the metadata.yaml file
	remoteScope, err := obj.importScopeWithInputs(importFilePath, scope, nextVertex)
	if err != nil {
		return nil, errwrap.Wrapf(err, "remote import of `%s` failed", info.Name)
	}
	return remoteScope, nil
}

// importSystemScope takes the name of a built-in system scope (eg: "fmt") and
// returns the scope struct for that built-in. This function is slightly less
// trivial than expected, because the scope is built from both native mcl code
// and golang code as well. The native mcl code is compiled in with "embed".
// TODO: can we memoize?
func (obj *StmtProg) importSystemScope(name string) (*interfaces.Scope, error) {
	// this basically loop through the registeredFuncs and includes
	// everything that starts with the name prefix and a period, and then
	// lexes and parses the compiled in code, and adds that on top of the
	// scope. we error if there's a duplicate!

	isEmpty := true // assume empty (which should cause an error)

	functions := FuncPrefixToFunctionsScope(name) // runs funcs.LookupPrefix
	if len(functions) > 0 {
		isEmpty = false
	}

	// perform any normal "startup" for these functions...
	for _, fn := range functions {
		// XXX: is this the right place for this, or should it be elsewhere?
		// XXX: do we need a modified obj.data for this b/c it's in a scope?
		if err := fn.Init(obj.data); err != nil {
			return nil, errwrap.Wrapf(err, "could not init function")
		}
		// TODO: do we want to run Interpolate or SetScope?
	}

	// TODO: pass `data` into ast.VarPrefixToVariablesScope ?
	variables := VarPrefixToVariablesScope(name) // strips prefix!

	// initial scope, built from core golang code
	scope := &interfaces.Scope{
		// TODO: we could use the core API for variables somehow...
		Variables: variables,
		Functions: functions, // map[string]Expr
		// TODO: we could add a core API for classes too!
		//Classes: make(map[string]interfaces.Stmt),
	}

	// TODO: the obj.data.Fs filesystem handle is unused for now, but might
	// be useful if we ever ship all the specific versions of system modules
	// to the remote machines as well, and we want to load off of it...

	// now add any compiled-in mcl code
	paths, err := core.AssetNames()
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read asset names")
	}
	// results are not sorted by default (ascertained by reading the code!)
	sort.Strings(paths)
	newScope := interfaces.EmptyScope()
	// XXX: consider using a virtual `append *` statement to combine these instead.
	for _, p := range paths {
		// we only want code from this prefix
		prefix := funcs.CoreDir + name + "/"
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		// we only want code from this directory level, so skip children
		// heuristically, a child mcl file will contain a path separator
		if strings.Contains(p[len(prefix):], "/") {
			continue
		}

		b, err := core.Asset(p)
		if err != nil {
			return nil, errwrap.Wrapf(err, "can't read asset: `%s`", p)
		}

		// to combine multiple *.mcl files from the same directory, we
		// lex and parse each one individually, which each produces a
		// scope struct. we then merge the scope structs, while making
		// sure we don't overwrite any values. (this logic is only valid
		// for modules, as top-level code combines the output values
		// instead.)

		reader := bytes.NewReader(b) // wrap the byte stream

		// now run the lexer/parser to do the import
		ast, err := obj.data.LexParser(reader)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not generate AST from import `%s`", name)
		}
		if obj.data.Debug {
			obj.data.Logf("behold, the AST: %+v", ast)
		}

		//obj.data.Logf("init...")
		//obj.data.Logf("import: %s", ?) // TODO: add this for symmetry?
		// init and validate the structure of the AST
		// some of this might happen *after* interpolate in SetScope or later...
		if err := ast.Init(obj.data); err != nil {
			return nil, errwrap.Wrapf(err, "could not init and validate AST")
		}

		if obj.data.Debug {
			obj.data.Logf("interpolating...")
		}
		// interpolate strings and other expansionable nodes in AST
		interpolated, err := ast.Interpolate()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not interpolate AST from import `%s`", name)
		}

		if obj.data.Debug {
			obj.data.Logf("scope building...")
		}
		// propagate the scope down through the AST...
		// most importantly, we ensure that the child imports will run!
		// we pass in *our* parent scope, which will include the globals
		if err := interpolated.SetScope(scope); err != nil {
			return nil, errwrap.Wrapf(err, "could not set scope from import `%s`", name)
		}

		// is the root of our ast a program?
		prog, ok := interpolated.(*StmtProg)
		if !ok {
			return nil, fmt.Errorf("import `%s` did not return a program", name)
		}

		if prog.scope == nil { // pull out the result
			continue // nothing to do here, continue with the next!
		}

		// check for unwanted top-level elements in this module/scope
		// XXX: add a test case to test for this in our core modules!
		if err := prog.IsModuleUnsafe(); err != nil {
			return nil, errwrap.Wrapf(err, "module contains unused statements")
		}

		if !prog.scope.IsEmpty() {
			isEmpty = false // this module/scope isn't empty
		}

		// save a reference to the prog for future usage in TypeCheck/Graph/Etc...
		// XXX: we don't need to do this if we can combine with Append!
		obj.importProgs = append(obj.importProgs, prog)

		// attempt to merge
		// XXX: test for duplicate var/func/class elements in a test!
		if err := newScope.Merge(prog.scope); err != nil { // errors if something was overwritten
			// XXX: we get a false positive b/c we overwrite the initial scope!
			// XXX: when we switch to append, this problem will go away...
			//return nil, errwrap.Wrapf(err, "duplicate scope element(s) in module found")
		}
	}

	if err := scope.Merge(newScope); err != nil { // errors if something was overwritten
		// XXX: we get a false positive b/c we overwrite the initial scope!
		// XXX: when we switch to append, this problem will go away...
		//return nil, errwrap.Wrapf(err, "duplicate scope element(s) found")
	}

	// when importing a system scope, we only error if there are zero class,
	// function, or variable statements in the scope. We error in this case,
	// because it is non-sensical to import such a scope.
	if isEmpty {
		return nil, fmt.Errorf("could not find any non-empty scope named: %s", name)
	}

	return scope, nil
}

// importScopeWithInputs returns a local or remote scope from an inputs string.
// The inputs string is the common frontend for a lot of our parsing decisions.
func (obj *StmtProg) importScopeWithInputs(s string, scope *interfaces.Scope, parentVertex *pgraph.SelfVertex) (*interfaces.Scope, error) {
	input, err := inputs.ParseInput(s, obj.data.Fs)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not activate an input parser")
	}

	return obj.importScopeWithParsedInputs(input, scope, parentVertex)
}

// importScopeWithParsedInputs returns a local or remote scope from an already
// parsed inputs string which presents as a parsed input struct.
func (obj *StmtProg) importScopeWithParsedInputs(input *inputs.ParsedInput, scope *interfaces.Scope, parentVertex *pgraph.SelfVertex) (*interfaces.Scope, error) {
	// TODO: rm this old, and incorrect, linear file duplicate checking...
	// recursion detection (i guess following the imports has to be a dag!)
	// run recursion detection by checking for duplicates in the seen files
	// TODO: do the paths need to be cleaned for "../", etc before compare?
	//for _, name := range obj.data.Files { // existing seen files
	//	if util.StrInList(name, input.Files) {
	//		return nil, fmt.Errorf("recursive import of: `%s`", name)
	//	}
	//}

	reader := bytes.NewReader(input.Main)

	// nested logger
	logf := func(format string, v ...interface{}) {
		//obj.data.Logf("import: "+format, v...) // don't nest!
		obj.data.Logf(format, v...)
	}

	// build new list of files
	files := []string{}
	files = append(files, input.Files...)
	files = append(files, obj.data.Files...)

	// store a reference to the parent metadata
	metadata := input.Metadata
	metadata.Metadata = obj.data.Metadata

	// now run the lexer/parser to do the import
	ast, err := obj.data.LexParser(reader)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not generate AST from import")
	}
	if obj.data.Debug {
		logf("behold, the AST: %+v", ast)
	}

	//logf("init...")
	logf("import: %s", input.Base)
	// init and validate the structure of the AST
	data := &interfaces.Data{
		// TODO: add missing fields here if/when needed
		Fs:       input.FS,       // formerly: obj.data.Fs,
		FsURI:    input.FS.URI(), // formerly: obj.data.FsURI,
		Base:     input.Base,     // new base dir (absolute path)
		Files:    files,
		Imports:  parentVertex, // the parent vertex that imported me
		Metadata: metadata,
		Modules:  obj.data.Modules,

		LexParser:       obj.data.LexParser,
		Downloader:      obj.data.Downloader,
		StrInterpolater: obj.data.StrInterpolater,
		SourceFinder:    obj.data.SourceFinder,
		//World: obj.data.World, // TODO: do we need this?

		//Prefix: obj.Prefix, // TODO: add a path on?
		Debug: obj.data.Debug,
		Logf:  logf,
	}
	// some of this might happen *after* interpolate in SetScope or later...
	if err := ast.Init(data); err != nil {
		return nil, errwrap.Wrapf(err, "could not init and validate AST")
	}

	if obj.data.Debug {
		logf("interpolating...")
	}
	// interpolate strings and other expansionable nodes in AST
	interpolated, err := ast.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate AST from import")
	}

	if obj.data.Debug {
		logf("scope building...")
	}
	// propagate the scope down through the AST...
	// most importantly, we ensure that the child imports will run!
	// we pass in *our* parent scope, which will include the globals
	if err := interpolated.SetScope(scope); err != nil {
		return nil, errwrap.Wrapf(err, "could not set scope from import")
	}

	// we DON'T do this here anymore, since Apply() digs into the children!
	//// this nested ast needs to pass the data up into the parent!
	//fileList, err := CollectFiles(interpolated)
	//if err != nil {
	//	return nil, errwrap.Wrapf(err, "could not collect files")
	//}
	//obj.importFiles = append(obj.importFiles, fileList...) // save for CollectFiles

	// is the root of our ast a program?
	prog, ok := interpolated.(*StmtProg)
	if !ok {
		return nil, fmt.Errorf("import did not return a program")
	}

	// check for unwanted top-level elements in this module/scope
	// XXX: add a test case to test for this in our core modules!
	if err := prog.IsModuleUnsafe(); err != nil {
		return nil, errwrap.Wrapf(err, "module contains unused statements")
	}

	// when importing a system scope, we only error if there are zero class,
	// function, or variable statements in the scope. We error in this case,
	// because it is non-sensical to import such a scope.
	if prog.scope.IsEmpty() {
		return nil, fmt.Errorf("could not find any non-empty scope")
	}
	if obj.data.Debug {
		obj.data.Logf("imported scope:")
		for k, v := range prog.scope.Variables {
			// print the type of v
			obj.data.Logf("\t%s: %T", k, v)
		}
	}

	// save a reference to the prog for future usage in TypeCheck/Graph/Etc...
	obj.importProgs = append(obj.importProgs, prog)

	// collecting these here is more elegant (and possibly more efficient!)
	obj.importFiles = append(obj.importFiles, input.Files...) // save for CollectFiles

	return prog.scope, nil
}

// SetScope propagates the scope into its list of statements. It does so
// cleverly by first collecting all bind and func statements and adding those
// into the scope after checking for any collisions. Finally it pushes the new
// scope downwards to all child statements. If we support user defined function
// polymorphism via multiple function definition, then these are built together
// here. This SetScope is the one which follows the import statements. If it
// can't follow one (perhaps it wasn't downloaded yet, and is missing) then it
// leaves some information about these missing imports in the AST and errors, so
// that a subsequent AST traversal (usually via Apply) can collect this detailed
// information to be used by the downloader. When it propagates the scope
// downwards, it first pushes it into all the classes, and then into everything
// else (including the include stmt's) because the include statements require
// that the scope already be known so that it can be combined with the include
// args.
func (obj *StmtProg) SetScope(scope *interfaces.Scope) error {
	newScope := scope.Copy()

	// start by looking for any `import` statements to pull into the scope!
	// this will run child lexing/parsing, interpolation, and scope setting
	imports := make(map[string]struct{})
	aliases := make(map[string]struct{})

	// keep track of new imports, to ensure they don't overwrite each other!
	// this is different from scope shadowing which is allowed in new scopes
	newVariables := make(map[string]string)
	newFunctions := make(map[string]string)
	newClasses := make(map[string]string)
	// TODO: If we added .Ordering() for *StmtImport, we could combine this
	// loop with the main nodeOrder sorted topological ordering loop below!
	for _, x := range obj.Body {
		imp, ok := x.(*StmtImport)
		if !ok {
			continue
		}
		// check for duplicates *in this scope*
		if _, exists := imports[imp.Name]; exists {
			return fmt.Errorf("import `%s` already exists in this scope", imp.Name)
		}

		result, err := langUtil.ParseImportName(imp.Name)
		if err != nil {
			return errwrap.Wrapf(err, "import `%s` is not valid", imp.Name)
		}
		alias := result.Alias // this is what we normally call the import

		if imp.Alias != "" { // this is what the user decided as the name
			alias = imp.Alias // use alias if specified
		}
		if _, exists := aliases[alias]; exists {
			return fmt.Errorf("import alias `%s` already exists in this scope", alias)
		}

		// run the scope importer...
		importedScope, err := obj.importScope(result, scope)
		if err != nil {
			obj.data.Logf("import scope `%s` failed", imp.Name)
			return err
		}

		// read from stored scope which was previously saved in SetScope
		// add to scope, (overwriting, aka shadowing is ok)
		// rename scope values, adding the alias prefix
		// check that we don't overwrite a new value from another import
		// TODO: do this in a deterministic (sorted) order
		for name, x := range importedScope.Variables {
			newName := alias + interfaces.ModuleSep + name
			if alias == interfaces.BareSymbol {
				if !AllowBareImports {
					return fmt.Errorf("bare imports disabled at compile time for import of `%s`", imp.Name)
				}
				newName = name
			}
			if previous, exists := newVariables[newName]; exists && alias != interfaces.BareSymbol {
				// don't overwrite in same scope
				return fmt.Errorf("can't squash variable `%s` from `%s` by import of `%s`", newName, previous, imp.Name)
			}
			newVariables[newName] = imp.Name
			newScope.Variables[newName] = x // merge
		}
		for name, x := range importedScope.Functions {
			newName := alias + interfaces.ModuleSep + name
			if alias == interfaces.BareSymbol {
				if !AllowBareImports {
					return fmt.Errorf("bare imports disabled at compile time for import of `%s`", imp.Name)
				}
				newName = name
			}
			if previous, exists := newFunctions[newName]; exists && alias != interfaces.BareSymbol {
				// don't overwrite in same scope
				return fmt.Errorf("can't squash function `%s` from `%s` by import of `%s`", newName, previous, imp.Name)
			}
			newFunctions[newName] = imp.Name
			newScope.Functions[newName] = x
		}
		for name, x := range importedScope.Classes {
			newName := alias + interfaces.ModuleSep + name
			if alias == interfaces.BareSymbol {
				if !AllowBareImports {
					return fmt.Errorf("bare imports disabled at compile time for import of `%s`", imp.Name)
				}
				newName = name
			}
			if previous, exists := newClasses[newName]; exists && alias != interfaces.BareSymbol {
				// don't overwrite in same scope
				return fmt.Errorf("can't squash class `%s` from `%s` by import of `%s`", newName, previous, imp.Name)
			}
			newClasses[newName] = imp.Name
			newScope.Classes[newName] = x
		}

		// everything has been merged, move on to next import...
		imports[imp.Name] = struct{}{} // mark as found in scope
		if alias != interfaces.BareSymbol {
			aliases[alias] = struct{}{}
		}
	}

	// TODO: this could be called once at the top-level, and then cached...
	// TODO: it currently gets called inside child programs, which is slow!
	orderingGraph, _, err := obj.Ordering(nil) // XXX: pass in globals from scope?
	// TODO: look at consumed variables, and prevent startup of unused ones?
	if err != nil {
		return errwrap.Wrapf(err, "could not generate ordering")
	}

	// debugging visualizations
	if obj.data.Debug && orderingGraphSingleton {
		obj.data.Logf("running graphviz for ordering graph...")
		if err := orderingGraph.ExecGraphviz("/tmp/graphviz-ordering.dot"); err != nil {
			obj.data.Logf("graphviz: errored: %+v", err)
		}
		//if err := orderingGraphFiltered.ExecGraphviz("/tmp/graphviz-ordering-filtered.dot"); err != nil {
		//	obj.data.Logf("graphviz: errored: %+v", err)
		//}
		// Only generate the top-level one, to prevent overwriting this!
		orderingGraphSingleton = false
	}

	// If we don't do this deterministically the type unification errors can
	// flip from `type error: int != str` to `type error: str != int` etc...
	nodeOrder, err := orderingGraph.DeterministicTopologicalSort() // sorted!
	if err != nil {
		// TODO: print the cycle in a prettier way (with names?)
		if obj.data.Debug {
			obj.data.Logf("set scope: not a dag:\n%s", orderingGraph.Sprint())
			//obj.data.Logf("set scope: not a dag:\n%s", orderingGraphFiltered.Sprint())
		}
		return errwrap.Wrapf(err, "recursive reference while setting scope")
	}
	if obj.data.Debug { // XXX: catch ordering errors in the logs
		obj.data.Logf("nodeOrder:")
		for i, x := range nodeOrder {
			obj.data.Logf("nodeOrder[%d]: %+v", i, x)
		}
	}

	// XXX: implement ValidTopoSortOrder!
	//topoSanity := (RequireTopologicalOrdering || TopologicalOrderingWarning)
	//if topoSanity && !orderingGraphFiltered.ValidTopoSortOrder(nodeOrder) {
	//	msg := "code is out of order, you're insane!"
	//	if TopologicalOrderingWarning {
	//		obj.data.Logf(msg)
	//		if obj.data.Debug {
	//			// TODO: print out of order problems
	//		}
	//	}
	//	if RequireTopologicalOrdering {
	//		return fmt.Errorf(msg)
	//	}
	//}

	// TODO: move this function to a utility package
	stmtInList := func(needle interfaces.Stmt, haystack []interfaces.Stmt) bool {
		for _, x := range haystack {
			if needle == x {
				return true
			}
		}
		return false
	}

	stmts := []interfaces.Stmt{}
	for _, x := range nodeOrder { // these are in the correct order for SetScope
		stmt, ok := x.(interfaces.Stmt)
		if !ok {
			continue
		}
		if _, ok := x.(*StmtImport); ok { // TODO: should we skip this?
			continue
		}
		if !stmtInList(stmt, obj.Body) {
			// Skip any unwanted additions that we pulled in.
			continue
		}
		stmts = append(stmts, stmt)
	}
	if obj.data.Debug {
		obj.data.Logf("prog: set scope: ordering: %+v", stmts)
	}
	obj.nodeOrder = stmts // save for .Graph()

	// Track all the bind statements, functions, and classes. This is used
	// for duplicate checking. These might appear out-of-order as code, but
	// are iterated in the topologically sorted node order. When we collect
	// all the functions, we group by name (if polyfunc is ok) and we also
	// do something similar for classes.
	// TODO: if we ever allow poly classes, then group in lists by name
	binds := make(map[string]struct{}) // bind existence in this scope
	functions := make(map[string][]*StmtFunc)
	classes := make(map[string]struct{})
	//includes := make(map[string]struct{}) // duplicates are allowed

	// Optimization: In addition to importantly skipping the parts of the
	// graph that don't belong in this StmtProg, this also causes
	// un-consumed statements to be skipped. As a result, this simplifies
	// the graph significantly in cases of unused code, because they're not
	// given a chance to SetScope even though they're in the StmtProg list.

	// In the below loop which we iterate over in the correct scope order,
	// we build up the scope (loopScope) as we go, so that subsequent uses
	// of the scope include earlier definitions and scope additions.
	loopScope := newScope.Copy()
	funcCount := make(map[string]int) // count the occurrences of a func
	for _, x := range nodeOrder {     // these are in the correct order for SetScope
		stmt, ok := x.(interfaces.Stmt)
		if !ok {
			continue
		}
		if _, ok := x.(*StmtImport); ok { // TODO: should we skip this?
			continue
		}
		if !stmtInList(stmt, obj.Body) {
			// Skip any unwanted additions that we pulled in.
			continue
		}

		capturedScope := loopScope.Copy()
		if err := stmt.SetScope(capturedScope); err != nil {
			return err
		}

		if bind, ok := x.(*StmtBind); ok {
			// check for duplicates *in this scope*
			if _, exists := binds[bind.Ident]; exists {
				return fmt.Errorf("var `%s` already exists in this scope", bind.Ident)
			}

			binds[bind.Ident] = struct{}{} // mark as found in scope

			if loopScope.Iterated {
				exprIterated := newExprIterated(bind.Ident, bind.Value)
				loopScope.Variables[bind.Ident] = exprIterated
			} else {
				// add to scope, (overwriting, aka shadowing is ok)
				loopScope.Variables[bind.Ident] = &ExprTopLevel{
					Definition: &ExprSingleton{
						Definition: bind.Value,

						mutex: &sync.Mutex{}, // TODO: call Init instead
					},
					CapturedScope: capturedScope,
				}
			}

			if obj.data.Debug { // TODO: is this message ever useful?
				obj.data.Logf("prog: set scope: bind collect: (%+v): %+v (%T) is %p", bind.Ident, bind.Value, bind.Value, bind.Value)
			}

			continue // optional
		}

		if fn, ok := x.(*StmtFunc); ok {
			_, exists := functions[fn.Name]
			if !exists {
				functions[fn.Name] = []*StmtFunc{} // initialize
			}

			// check for duplicates *in this scope*
			if exists && !AllowUserDefinedPolyFunc {
				return fmt.Errorf("func `%s` already exists in this scope", fn.Name)
			}

			count := 1 // XXX: number of overloaded definitions of the same name (get from ordering eventually)
			funcCount[fn.Name]++

			// collect functions (if multiple, this is a polyfunc)
			functions[fn.Name] = append(functions[fn.Name], fn)

			if funcCount[fn.Name] < count {
				continue // delay SetScope for later...
			}

			fnList := functions[fn.Name] // []*StmtFunc

			if obj.data.Debug { // TODO: is this message ever useful?
				obj.data.Logf("prog: set scope: collect: (%+v -> %d): %+v (%T)", fn.Name, len(fnList), fnList[0].Func, fnList[0].Func)
			}

			// add to scope, (overwriting, aka shadowing is ok)
			if len(fnList) == 1 {
				f := fnList[0].Func // local reference to avoid changing it in the loop...
				// add to scope, (overwriting, aka shadowing is ok)

				if loopScope.Iterated {
					// XXX: ExprPoly or ExprTopLevel might
					// end up breaking something here...
					//loopScope.Functions[fn.Name] = &ExprIterated{
					//	Name: fn.Name,
					//	Definition: &ExprPoly{
					//		Definition: &ExprTopLevel{
					//			Definition:    f, // store the *ExprFunc
					//			CapturedScope: capturedScope,
					//		},
					//	},
					//}
					// We reordered the nesting here to try and fix some bug.
					loopScope.Functions[fn.Name] = &ExprPoly{
						Definition: newExprIterated(
							fn.Name,
							&ExprTopLevel{
								Definition:    f, // store the *ExprFunc
								CapturedScope: capturedScope,
							},
						),
					}

				} else {
					loopScope.Functions[fn.Name] = &ExprPoly{ // XXX: is this ExprPoly approach optimal?
						Definition: &ExprTopLevel{
							Definition:    f, // store the *ExprFunc
							CapturedScope: capturedScope,
						},
					}
				}

				continue
			}

			// build polyfunc's
			// XXX: not implemented
			return fmt.Errorf("user-defined polyfuncs of length %d are not supported", len(fnList))
		}

		if class, ok := x.(*StmtClass); ok {
			// check for duplicates *in this scope*
			if _, exists := classes[class.Name]; exists {
				return fmt.Errorf("class `%s` already exists in this scope", class.Name)
			}

			classes[class.Name] = struct{}{} // mark as found in scope

			// add to scope, (overwriting, aka shadowing is ok)
			loopScope.Classes[class.Name] = class

			continue
		}

		// now collect any include contents
		if include, ok := x.(*StmtInclude); ok {
			// We actually don't want to check for duplicates, that
			// is allowed, if we `include foo as bar` twice it will
			// currently not work, but if possible, we can allow it.
			// check for duplicates *in this scope*
			//if _, exists := includes[include.Name]; exists {
			//	return fmt.Errorf("include `%s` already exists in this scope", include.Name)
			//}

			alias := ""
			if AllowBareClassIncluding {
				alias = include.Name // this is what we would call the include
			}
			if include.Alias != "" { // this is what the user decided as the name
				alias = include.Alias // use alias if specified
			}
			if alias == "" {
				continue // there isn't anything to do here
			}

			// NOTE: This gets caught in ordering instead of here...
			// deal with alias duplicates and * includes and so on...
			if _, exists := aliases[alias]; exists {
				// TODO: track separately to give a better error message here
				return fmt.Errorf("import/include alias `%s` already exists in this scope", alias)
			}

			if include.class == nil {
				// programming error
				return fmt.Errorf("programming error: class `%s` not found", include.Name)
			}
			// This includes any variable from the top-level scope
			// that is visible (and captured) inside the class, and
			// re-exported when included with `as`. This is the
			// "tricky case", but it turns out it's better this way.
			// Example:
			//
			//	$x = "i am x"	# i am now top-level
			//	class c1() {
			//		$whatever = fmt.printf("i can see: %s", $x)
			//	}
			//	include c1 as i1
			//	test $i1.x {}		# tricky
			//	test $i1.whatever {}	# easy
			//
			// We want to allow the tricky case to prevent needing
			// to write code like: `$x = $x` inside of class c1 to
			// get the same effect.

			//includedScope := include.class.Body.scope // conceptually
			prog, ok := include.class.Body.(*StmtProg)
			if !ok {
				return fmt.Errorf("programming error: prog not found in class Body")
			}
			// XXX: .Copy() ?
			includedScope := prog.scope

			// read from stored scope which was previously saved in SetScope
			// add to scope, (overwriting, aka shadowing is ok)
			// rename scope values, adding the alias prefix
			// check that we don't overwrite a new value from another include
			// TODO: do this in a deterministic (sorted) order
			for name, x := range includedScope.Variables {
				newName := alias + interfaces.ModuleSep + name
				if alias == interfaces.BareSymbol { // not supported by parser atm!
					if !AllowBareIncludes {
						return fmt.Errorf("bare includes disabled at compile time for include of `%s`", include.Name)
					}
					newName = name
				}
				if previous, exists := newVariables[newName]; exists && alias != interfaces.BareSymbol {
					// don't overwrite in same scope
					return fmt.Errorf("can't squash variable `%s` from `%s` by include of `%s`", newName, previous, include.Name)
				}
				newVariables[newName] = include.Name
				loopScope.Variables[newName] = x // merge
			}
			for name, x := range includedScope.Functions {
				newName := alias + interfaces.ModuleSep + name
				if alias == interfaces.BareSymbol { // not supported by parser atm!
					if !AllowBareIncludes {
						return fmt.Errorf("bare includes disabled at compile time for include of `%s`", include.Name)
					}
					newName = name
				}
				if previous, exists := newFunctions[newName]; exists && alias != interfaces.BareSymbol {
					// don't overwrite in same scope
					return fmt.Errorf("can't squash function `%s` from `%s` by include of `%s`", newName, previous, include.Name)
				}
				newFunctions[newName] = include.Name
				loopScope.Functions[newName] = x
			}
			for name, x := range includedScope.Classes {
				newName := alias + interfaces.ModuleSep + name
				if alias == interfaces.BareSymbol { // not supported by parser atm!
					if !AllowBareIncludes {
						return fmt.Errorf("bare includes disabled at compile time for include of `%s`", include.Name)
					}
					newName = name
				}
				if previous, exists := newClasses[newName]; exists && alias != interfaces.BareSymbol {
					// don't overwrite in same scope
					return fmt.Errorf("can't squash class `%s` from `%s` by include of `%s`", newName, previous, include.Name)
				}
				newClasses[newName] = include.Name
				loopScope.Classes[newName] = x
			}

			// everything has been merged, move on to next include...
			//includes[include.Name] = struct{}{} // don't mark as found in scope
			if alias != interfaces.BareSymbol { // XXX: check if this one and the above ones in this collection are needed too
				aliases[alias] = struct{}{} // do track these as a bonus
			}
		}

	}

	obj.scope = loopScope // save a reference in case we're read by an import

	if obj.data.Debug {
		obj.data.Logf("prog: set scope: finished")
	}

	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtProg) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	for _, x := range obj.Body {
		// We skip this because it will be instantiated potentially with
		// different types.
		if _, ok := x.(*StmtClass); ok {
			continue
		}

		// We skip this because it will be instantiated potentially with
		// different types.
		if _, ok := x.(*StmtFunc); ok {
			continue
		}

		// We skip this one too since we pull it in at the use site.
		if _, ok := x.(*StmtBind); ok {
			continue
		}

		invars, err := x.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	// add invariants from SetScope's imported child programs
	for _, x := range obj.importProgs {
		invars, err := x.TypeCheck()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might.
func (obj *StmtProg) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	g, _, err := obj.updateEnv(env)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// updateEnv is a more general version of Graph.
func (obj *StmtProg) updateEnv(env *interfaces.Env) (*pgraph.Graph, *interfaces.Env, error) {
	graph, err := pgraph.NewGraph("prog")
	if err != nil {
		return nil, nil, err
	}

	loopEnv := env.Copy()

	// In this loop, we want to skip over StmtClass, StmtFunc, and StmtBind,
	// but only in their "normal" boring definition modes.
	for _, x := range obj.nodeOrder {
		stmt, ok := x.(interfaces.Stmt)
		if !ok {
			continue
		}

		//if _, ok := x.(*StmtImport); ok { // TODO: should we skip this?
		//	continue
		//}

		// skip over *StmtClass here
		if _, ok := x.(*StmtClass); ok {
			continue
		}

		if include, ok := x.(*StmtInclude); ok && obj.scope.Iterated {
			// The include can bring a bunch of variables into scope
			// so we need an entry for them in the loop Env. Let's
			// fill it up.
			//g, extendedEnv, err := include.class.Body.(*StmtProg).updateEnv(loopEnv)
			g, extendedEnv, err := include.updateEnv(loopEnv)
			//g, err := include.Graph(loopEnv)
			if err != nil {
				return nil, nil, err
			}
			loopEnv = extendedEnv // XXX: Sam says we need to change this for trickier cases!

			graph.AddGraph(g) // We DO want to add this to graph.
			continue
		}

		if bind, ok := x.(*StmtBind); ok {
			if !obj.scope.Iterated {
				continue // We do NOTHING, not even add to Graph
			}

			// always squash, this is shadowing...
			expr, exists := obj.scope.Variables[bind.Ident]
			if !exists {
				// TODO: is this a programming error?
				return nil, nil, fmt.Errorf("programming error")
			}
			exprIterated, ok := expr.(*ExprIterated)
			if !ok {
				// TODO: is this a programming error?
				return nil, nil, fmt.Errorf("programming error")
			}
			//loopEnv.Variables[exprIterated.envKey] = f
			loopEnv.Variables[exprIterated.envKey] = &interfaces.FuncSingleton{
				MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
					return bind.privateGraph(loopEnv)
				},
			}

			//graph.AddGraph(g) // We DO want to add this to graph.
			continue
		}

		if stmtFunc, ok := x.(*StmtFunc); ok {
			if !obj.scope.Iterated {
				continue // We do NOTHING, not even add to Graph
			}

			expr, exists := obj.scope.Functions[stmtFunc.Name] // map[string]Expr
			if !exists {
				// programming error
				return nil, nil, fmt.Errorf("programming error 1")
			}
			exprPoly, ok := expr.(*ExprPoly)
			if !ok {
				// programming error
				return nil, nil, fmt.Errorf("programming error 2")
			}
			if _, ok := exprPoly.Definition.(*ExprIterated); !ok {
				// programming error
				return nil, nil, fmt.Errorf("programming error 3")
			}

			// always squash, this is shadowing...
			// XXX: We probably don't need to copy here says Sam.
			loopEnv.Functions[expr] = loopEnv.Copy() // captured env

			// We NEVER want to add anything to the graph here.
			continue
		}

		g, err := stmt.Graph(loopEnv)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)
	}

	// add graphs from SetScope's imported child programs
	//for _, x := range obj.importProgs {
	//	g, err := x.Graph(env)
	//	if err != nil {
	//		return nil, err
	//	}
	//	graph.AddGraph(g)
	//}

	return graph, loopEnv, nil
}

// Output returns the output that this "program" produces. This output is what
// is used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output.
func (obj *StmtProg) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	resources := []engine.Res{}
	edges := []*interfaces.Edge{}

	for _, stmt := range obj.Body {
		// skip over *StmtClass here so its Output method can be used...
		if _, ok := stmt.(*StmtClass); ok {
			// don't read output from StmtClass, it
			// gets consumed by StmtInclude instead
			continue
		}
		// skip over StmtFunc, even though it doesn't produce anything!
		if _, ok := stmt.(*StmtFunc); ok {
			continue
		}
		// skip over StmtBind, even though it doesn't produce anything!
		if _, ok := stmt.(*StmtBind); ok {
			continue
		}

		output, err := stmt.Output(table)
		if err != nil {
			return nil, err
		}
		if output != nil {
			resources = append(resources, output.Resources...)
			edges = append(edges, output.Edges...)
		}
	}

	// nothing to add from SetScope's imported child programs

	return &interfaces.Output{
		Resources: resources,
		Edges:     edges,
	}, nil
}

// IsModuleUnsafe returns whether or not this StmtProg is unsafe to consume as a
// module scope. IOW, if someone writes a module which is imported and which has
// statements other than bind, func, class or import, then it is not correct to
// import, since those other elements wouldn't be used, and might provide a
// false belief that they'll get included when mgmt imports that module.
// SetScope should be called before this is used. (TODO: verify this)
// TODO: return a multierr with all the unsafe elements, to provide better info
// TODO: technically this could be a method on Stmt, possibly using Apply...
func (obj *StmtProg) IsModuleUnsafe() error { // TODO: rename this function?
	for _, x := range obj.Body {
		// stmt's allowed: import, bind, func, class
		// stmt's not-allowed: for, forkv, if, include, res, edge
		switch x.(type) {
		case *StmtImport:
		case *StmtBind:
		case *StmtFunc:
		case *StmtClass:
		case *StmtComment: // possibly not even parsed
			// all of these are safe
		default:
			// something else unsafe (unused)
			return fmt.Errorf("found stmt: %s", x.String())
		}
	}
	return nil
}

// StmtFunc represents a user defined function. It binds the specified name to
// the supplied function in the current scope and irrespective of the order of
// definition.
type StmtFunc struct {
	interfaces.Textarea

	data *interfaces.Data

	Name string
	Func interfaces.Expr
	Type *types.Type
}

// String returns a short representation of this statement.
func (obj *StmtFunc) String() string {
	return fmt.Sprintf("func(%s)", obj.Name)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtFunc) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Func.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtFunc) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Name == "" {
		return fmt.Errorf("func name is empty")
	}

	if err := obj.Func.Init(data); err != nil {
		return err
	}
	// no errors
	return nil
}

// Interpolate returns a new node (or itself) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtFunc) Interpolate() (interfaces.Stmt, error) {
	interpolated, err := obj.Func.Interpolate()
	if err != nil {
		return nil, err
	}

	return &StmtFunc{
		Textarea: obj.Textarea,
		data:     obj.data,
		Name:     obj.Name,
		Func:     interpolated,
		Type:     obj.Type,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtFunc) Copy() (interfaces.Stmt, error) {
	copied := false
	fn, err := obj.Func.Copy()
	if err != nil {
		return nil, err
	}
	if fn != obj.Func { // must have been copied, or pointer would be same
		copied = true
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtFunc{
		Textarea: obj.Textarea,
		data:     obj.data,
		Name:     obj.Name,
		Func:     fn,
		Type:     obj.Type,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// We only really care about the consumers here, because the "produces" aspect
// of this resource is handled by the StmtProg Ordering function. This is
// because the "prog" allows out-of-order statements, therefore it solves this
// by running an early (second) loop through the program and peering into this
// Stmt and extracting the produced name.
func (obj *StmtFunc) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtfuncfunc"}
	graph.AddEdge(obj.Func, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Func.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "stmtfunc"}
		graph.AddEdge(n, k, edge)
	}

	// The consumes which have already been matched to one of our produces
	// must not be also matched to a produce from our caller. Is that clear?
	//newCons := make(map[interfaces.Node]string) // don't modify the input map!
	//for k, v := range cons {
	//	if _, exists := prod[v]; exists {
	//		continue
	//	}
	//	newCons[k] = v // "remaining" values from cons
	//}
	//
	//return graph, newCons, nil

	return graph, cons, nil
}

// SetScope sets the scope of the child expression bound to it. It seems this is
// necessary in order to reach this, in particular in situations when a bound
// expression points to a previously bound expression.
func (obj *StmtFunc) SetScope(scope *interfaces.Scope) error {
	return obj.Func.SetScope(scope, map[string]interfaces.Expr{})
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtFunc) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	if obj.Name == "" {
		return nil, fmt.Errorf("missing function name")
	}

	// Don't call obj.Func.Check here!
	typ, invariants, err := obj.Func.Infer()
	if err != nil {
		return nil, err
	}

	typExpr := obj.Type
	if obj.Type == nil {
		typExpr = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}

	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj.Func,
		Expect: typExpr, // obj.Type
		Actual: typ,
	}
	invariants = append(invariants, invar)

	// I think the invariants should come in from ExprCall instead, because
	// ExprCall operates on an instantiated copy of the contained ExprFunc
	// which will have different pointers than what is seen here.

	// nope!
	// Don't call obj.Func.Check here!
	//typ, invariants, err := obj.Func.Infer()
	//if err != nil {
	//	return nil, err
	//}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular func statement adds its linked expression to
// the graph.
func (obj *StmtFunc) Graph(*interfaces.Env) (*pgraph.Graph, error) {
	//return obj.Func.Graph(nil) // nope!
	return pgraph.NewGraph("stmtfunc") // do this in ExprCall instead
}

// Output for the func statement produces no output. Any values of interest come
// from the use of the func which this binds the function to.
func (obj *StmtFunc) Output(map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	return interfaces.EmptyOutput(), nil
}

// StmtClass represents a user defined class. It's effectively a program body
// that can optionally take some parameterized inputs.
// TODO: We don't currently support defining polymorphic classes (eg: different
// signatures for the same class name) but it might be something to consider.
type StmtClass struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	Name string
	Args []*interfaces.Arg // XXX: sam thinks we should name this Params and interfaces.Param
	Body interfaces.Stmt   // probably a *StmtProg
}

// String returns a short representation of this statement.
func (obj *StmtClass) String() string {
	return fmt.Sprintf("class(%s)", obj.Name)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtClass) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Body.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtClass) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Name == "" {
		return fmt.Errorf("class name is empty")
	}

	return obj.Body.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtClass) Interpolate() (interfaces.Stmt, error) {
	interpolated, err := obj.Body.Interpolate()
	if err != nil {
		return nil, err
	}

	args := obj.Args
	if obj.Args == nil {
		args = []*interfaces.Arg{}
	}

	return &StmtClass{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		Name:     obj.Name,
		Args:     args, // ensure this has length == 0 instead of nil
		Body:     interpolated,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtClass) Copy() (interfaces.Stmt, error) {
	copied := false
	body, err := obj.Body.Copy()
	if err != nil {
		return nil, err
	}
	if body != obj.Body { // must have been copied, or pointer would be same
		copied = true
	}

	args := obj.Args
	if obj.Args == nil {
		args = []*interfaces.Arg{}
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtClass{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		Name:     obj.Name,
		Args:     args, // ensure this has length == 0 instead of nil
		Body:     body,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// We only really care about the consumers here, because the "produces" aspect
// of this resource is handled by the StmtProg Ordering function. This is
// because the "prog" allows out-of-order statements, therefore it solves this
// by running an early (second) loop through the program and peering into this
// Stmt and extracting the produced name.
// TODO: Is Ordering in StmtInclude done properly and in sync with this?
// XXX: do we need to add ordering around named args, eg: obj.Args Name strings?
func (obj *StmtClass) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	prod := make(map[string]interfaces.Node)
	for _, arg := range obj.Args {
		uid := varOrderingPrefix + arg.Name // ordering id
		//node, exists := produces[uid]
		//if exists {
		//	edge := &pgraph.SimpleEdge{Name: "stmtclassarg"}
		//	graph.AddEdge(node, obj, edge) // prod -> cons
		//}
		prod[uid] = &ExprParam{Name: arg.Name} // placeholder
	}

	newProduces := CopyNodeMapping(produces) // don't modify the input map!

	// Overwrite anything in this scope with the shadowed parent variable!
	for key, val := range prod {
		newProduces[key] = val // copy, and overwrite (shadow) any parent var
	}

	// additional constraint...
	edge := &pgraph.SimpleEdge{Name: "stmtclassbody"}
	graph.AddEdge(obj.Body, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, cons, err := obj.Body.Ordering(newProduces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	// The consumes which have already been matched to one of our produces
	// must not be also matched to a produce from our caller. Is that clear?
	newCons := make(map[interfaces.Node]string) // don't modify the input map!
	for k, v := range cons {
		if _, exists := prod[v]; exists {
			continue
		}
		newCons[k] = v // "remaining" values from cons
	}

	return graph, newCons, nil
}

// SetScope sets the scope of the child expression bound to it. It seems this is
// necessary in order to reach this, in particular in situations when a bound
// expression points to a previously bound expression.
func (obj *StmtClass) SetScope(scope *interfaces.Scope) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}

	// We want to capture what was in scope at the definition site of the
	// class so that when we `include` the class, the body of the class is
	// expanded with the variables which were in scope at the definition
	// site and not the variables which were in scope at the include site.
	obj.scope = scope // store for later

	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtClass) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	if obj.Name == "" {
		return nil, fmt.Errorf("missing class name")
	}

	// TODO: do we need to add anything else here because of the obj.Args ?

	invariants, err := obj.Body.TypeCheck()
	if err != nil {
		return nil, err
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular func statement adds its linked expression to
// the graph.
func (obj *StmtClass) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	return obj.Body.Graph(env)
}

// Output for the class statement produces no output. Any values of interest
// come from the use of the include which this binds the statements to. This is
// usually called from the parent in StmtProg, but it skips running it so that
// it can be called from the StmtInclude Output method.
func (obj *StmtClass) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	return obj.Body.Output(table)
}

// StmtInclude causes a user defined class to get used. It's effectively the way
// to call a class except that it produces output instead of a value. Most of
// the interesting logic for classes happens here or in StmtProg.
type StmtInclude struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope

	class *StmtClass   // copy of class that we're using
	orig  *StmtInclude // original pointer to this

	Name        string
	Args        []interfaces.Expr
	argsEnvKeys []*ExprIterated
	Alias       string
}

// String returns a short representation of this statement.
func (obj *StmtInclude) String() string {
	return fmt.Sprintf("include(%s)", obj.Name)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtInclude) Apply(fn func(interfaces.Node) error) error {
	// If the class exists, then descend into it, because at this point, the
	// copy of the original class that is stored here, is the effective
	// class that we care about for type unification, and everything else...
	// It's not clear if this is needed, but it's probably nor harmful atm.
	if obj.class != nil {
		if err := obj.class.Apply(fn); err != nil {
			return err
		}
	}
	for _, x := range obj.Args {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtInclude) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Name == "" {
		return fmt.Errorf("include name is empty")
	}

	for _, x := range obj.Args {
		if err := x.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtInclude) Interpolate() (interfaces.Stmt, error) {
	args := []interfaces.Expr{}
	for _, x := range obj.Args {
		interpolated, err := x.Interpolate()
		if err != nil {
			return nil, err
		}
		args = append(args, interpolated)
	}

	orig := obj
	if obj.orig != nil { // preserve the original pointer (the identifier!)
		orig = obj.orig
	}
	return &StmtInclude{
		Textarea:    obj.Textarea,
		data:        obj.data,
		scope:       obj.scope,
		class:       obj.class, // XXX: Should we copy this?
		orig:        orig,
		Name:        obj.Name,
		Args:        args,
		argsEnvKeys: obj.argsEnvKeys, // update this if we interpolate after SetScope
		Alias:       obj.Alias,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtInclude) Copy() (interfaces.Stmt, error) {
	copied := false
	args := []interfaces.Expr{}
	for _, x := range obj.Args {
		cp, err := x.Copy()
		if err != nil {
			return nil, err
		}
		if cp != x { // must have been copied, or pointer would be same
			copied = true
		}
		args = append(args, cp)
	}

	// TODO: is this necessary? (I doubt it even gets used.)
	orig := obj
	if obj.orig != nil { // preserve the original pointer (the identifier!)
		orig = obj.orig
		copied = true // TODO: is this what we want?
	}

	// Sometimes when we run copy it's legal for obj.class to be nil.
	var newClass *StmtClass
	if obj.class != nil {
		stmt, err := obj.class.Copy()
		if err != nil {
			return nil, err
		}
		class, ok := stmt.(*StmtClass)
		if !ok {
			// programming error
			return nil, fmt.Errorf("unexpected copy failure")
		}
		if class != obj.class {
			copied = true // TODO: is this what we want?
		}
		newClass = class
	}

	if !copied { // it's static
		return obj, nil
	}
	return &StmtInclude{
		Textarea:    obj.Textarea,
		data:        obj.data,
		scope:       obj.scope,
		class:       newClass, // This seems necessary!
		orig:        orig,
		Name:        obj.Name,
		Args:        args,
		argsEnvKeys: obj.argsEnvKeys,
		Alias:       obj.Alias,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// TODO: Is Ordering in StmtClass done properly and in sync with this?
func (obj *StmtInclude) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	if obj.Name == "" {
		return nil, nil, fmt.Errorf("missing class name")
	}

	uid := classOrderingPrefix + obj.Name // ordering id

	node, exists := produces[uid]
	if exists {
		edge := &pgraph.SimpleEdge{Name: "stmtinclude1"}
		graph.AddEdge(node, obj, edge) // prod -> cons
	}

	// equivalent to: strings.Contains(obj.Name, interfaces.ModuleSep)
	if split := strings.Split(obj.Name, interfaces.ModuleSep); len(split) > 1 {
		// we contain a dot
		uid = scopedOrderingPrefix + split[0] // just the first prefix

		// TODO: do we also want this second edge??
		node, exists := produces[uid]
		if exists {
			edge := &pgraph.SimpleEdge{Name: "stmtinclude2"}
			graph.AddEdge(node, obj, edge) // prod -> cons
		}
	}
	// It's okay to replace the normal `class` prefix, because we have the
	// fancier `scoped:` prefix which matches more generally...

	// TODO: we _can_ produce two uid's here, is it okay we only offer one?
	cons := make(map[interfaces.Node]string)
	cons[obj] = uid

	for _, node := range obj.Args {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "stmtincludeargs1"}
		graph.AddEdge(node, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "stmtincludeargs2"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for use in this statement. Since this is the first
// location where recursion would play an important role, this also detects and
// handles the recursion scenario.
func (obj *StmtInclude) SetScope(scope *interfaces.Scope) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope

	stmt, exists := scope.Classes[obj.Name]
	if !exists {
		if obj.data.Debug || true { // TODO: leave this on permanently?
			classScopeFeedback(scope, obj.data.Logf)
		}
		err := fmt.Errorf("class `%s` does not exist in this scope", obj.Name)
		return interfaces.HighlightHelper(obj, obj.data.Logf, err)
	}
	class, ok := stmt.(*StmtClass)
	if !ok {
		return fmt.Errorf("class scope of `%s` does not contain a class", obj.Name)
	}

	// Is it even possible for the signatures to not match?
	if len(class.Args) != len(obj.Args) {
		err := fmt.Errorf("class `%s` expected %d args but got %d", obj.Name, len(class.Args), len(obj.Args))
		return interfaces.HighlightHelper(obj, obj.data.Logf, err)
	}

	if obj.class != nil {
		// possible programming error
		return fmt.Errorf("include already contains a class pointer")
	}

	// make sure to propagate the scope to our input args!
	for _, x := range obj.Args {
		if err := x.SetScope(scope, map[string]interfaces.Expr{}); err != nil {
			return err
		}
	}

	for i := len(scope.Chain) - 1; i >= 0; i-- { // reverse order
		x, ok := scope.Chain[i].(*StmtInclude)
		if !ok {
			continue
		}

		if x == obj.orig { // look for my original self
			// scope chain found!
			obj.class = class // same pointer, don't copy
			return fmt.Errorf("recursive class `%s` found", obj.Name)
			//return nil // if recursion was supported
		}
	}

	// helper function to keep things more logical
	cp := func(input *StmtClass) (*StmtClass, error) {
		copied, err := input.Copy() // this does a light copy
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not copy class")
		}
		class, ok := copied.(*StmtClass) // convert it back again
		if !ok {
			return nil, fmt.Errorf("copied class named `%s` is not a class", obj.Name)
		}
		return class, nil
	}

	copied, err := cp(class) // copy it for each use of the include
	if err != nil {
		return errwrap.Wrapf(err, "could not copy class")
	}
	obj.class = copied

	// We start with the scope that the class had, and we augment it with
	// our parameterized arg variables, which will be needed in that scope.
	newScope := obj.class.scope.Copy()

	if obj.scope.Iterated { // Sam says NOT obj.class.scope
		obj.argsEnvKeys = make([]*ExprIterated, len(obj.class.Args)) // or just append() in loop below...

		// Add our args `include foo(42, "bar", true)` into the class scope.
		for i, param := range obj.class.Args { // copy
			// NOTE: similar to StmtProg.SetScope (StmtBind case)
			obj.argsEnvKeys[i] = newExprIterated(
				param.Name,
				obj.Args[i],
			)
			newScope.Variables[param.Name] = obj.argsEnvKeys[i]
		}
	} else {
		// Add our args `include foo(42, "bar", true)` into the class scope.
		for i, param := range obj.class.Args { // copy
			newScope.Variables[param.Name] = &ExprTopLevel{
				Definition: &ExprSingleton{
					Definition: obj.Args[i],

					mutex: &sync.Mutex{}, // TODO: call Init instead
				},
				CapturedScope: newScope,
			}
		}
	}

	// recursion detection
	newScope.Chain = append(newScope.Chain, obj.orig) // add stmt to list
	newScope.Classes[obj.Name] = copied               // overwrite with new pointer
	newScope.Iterated = scope.Iterated                // very important!

	// NOTE: This would overwrite the scope that was previously set here,
	// which would break the scoping rules. Scopes are propagated into
	// class definitions, but not into include definitions. Which is why we
	// need to use the original scope of the class as it was set as the
	// basis for this scope, so that we overwrite it only with the arg
	// changes.
	//
	// Whether this body is iterated or not, does not depend on whether the
	// class definition site is inside of a for loop but on whether the
	// StmtInclude is inside of a for loop. So we set that Iterated var
	// above.
	if err := obj.class.Body.SetScope(newScope); err != nil {
		return err
	}

	// no errors
	return nil
}

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtInclude) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	if obj.Name == "" {
		return nil, fmt.Errorf("missing include name")
	}
	if obj.class == nil {
		// possible programming error
		return nil, fmt.Errorf("include doesn't contain a class pointer yet")
	}

	// Is it even possible for the signatures to not match?
	if len(obj.class.Args) != len(obj.Args) {
		return nil, fmt.Errorf("class `%s` expected %d args but got %d", obj.Name, len(obj.class.Args), len(obj.Args))
	}

	// do this here because we skip doing it in the StmtProg parent
	invariants, err := obj.class.TypeCheck()
	if err != nil {
		return nil, err
	}

	for i, x := range obj.Args {
		// Don't call x.Check here!
		typ, invars, err := x.Infer()
		if err != nil {
			return nil, err
		}
		invariants = append(invariants, invars...)

		// XXX: Should we be doing this stuff here?

		// TODO: are additional invariants required?
		// add invariants between the args and the class
		if typExpr := obj.class.Args[i].Type; typExpr != nil {
			invar := &interfaces.UnificationInvariant{
				Node:   obj,
				Expr:   x,
				Expect: typExpr, // type of arg
				Actual: typ,
			}
			invariants = append(invariants, invar)
		}
	}

	return invariants, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular func statement adds its linked expression to
// the graph.
func (obj *StmtInclude) Graph(env *interfaces.Env) (*pgraph.Graph, error) {
	g, _, err := obj.updateEnv(env)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// updateEnv is a more general version of Graph.
//
// Normally, an ExprIterated.Name is the same as the name in the corresponding
// StmtBind. When StmtInclude AS is used, the name in ExprIterated is the short
// name (like `result`), and the StmtBind.Ident is the short name (like
// `result`), but the ExprVar and the ExprCall use $iterated.result. Instead of
// the short name (in ExprIterated.Name and the environment) we should either
// use $iterated.result or the ExprIterated pointer. We are currently trying the
// latter (the pointer).
//
// More importantly: StmtFor.Graph copies the body of the ? for N times. If
// there are any StmtBind's their ExprSingleton's are cleared, so that each
// iteration gets its own Func. If there is a StmtInclude, it contains a copy of
// the class body, and this body is also copied once per iteration. However, the
// variables in that body do not contain the thing to which they refer, that is
// called the "referend" (since we just have a string name) and therefore if it
// refers to an ExprSingleton, that ExprSingleton does not get cleared, and
// every iteration of the loop gets the same value for that variable. This is
// fine under normal circumstances because the thing to which the ExprVar refers
// might be defined outside of the for loop, in which case, we don't want to
// copy it once per iteration. The problem is that in this case, the variable is
// pointing to something inside the for loop which is somehow not being copied.
func (obj *StmtInclude) updateEnv(env *interfaces.Env) (*pgraph.Graph, *interfaces.Env, error) {
	graph, err := pgraph.NewGraph("include")
	if err != nil {
		return nil, nil, err
	}

	if obj.class == nil {
		// programming error
		return nil, nil, fmt.Errorf("can't include class %s, contents are nil", obj.Name)
	}

	if obj.scope.Iterated {
		loopEnv := env.Copy()

		// This args stuff is here since it's not technically needed in
		// the non-iterated case, and it would only make the function
		// graph bigger if that arg isn't used. The arg gets pulled in
		// to the function graph via ExprSingleton otherwise.
		for i, arg := range obj.Args {
			//g, f, err := arg.Graph(env)
			//if err != nil {
			//	return nil, nil, err
			//}
			//graph.AddGraph(g)
			//paramName := obj.class.Args[i].Name
			//loopEnv.Variables[paramName] = f
			//loopEnv.Variables[obj.argsEnvKeys[i]] = f
			loopEnv.Variables[obj.argsEnvKeys[i]] = &interfaces.FuncSingleton{
				MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
					return arg.Graph(env)
				},
			}
		}

		g, extendedEnv, err := obj.class.Body.(*StmtProg).updateEnv(loopEnv)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)
		loopEnv = extendedEnv

		return graph, loopEnv, nil
	}

	g, err := obj.class.Graph(env)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g)

	return graph, env, nil
}

// Output returns the output that this include produces. This output is what is
// used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output. The
// ultimate source of this output comes from the previously defined StmtClass
// which should be found in our scope.
func (obj *StmtInclude) Output(table map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	return obj.class.Output(table)
}

// StmtImport adds the exported scope definitions of a module into the current
// scope. It can be used anywhere a statement is allowed, and can even be nested
// inside a class definition. By convention, it is commonly used at the top of a
// file. As with any statement, it produces output, but that output is empty. To
// benefit from its inclusion, reference the scope definitions you want.
type StmtImport struct {
	interfaces.Textarea

	data *interfaces.Data

	Name  string
	Alias string
}

// String returns a short representation of this statement.
func (obj *StmtImport) String() string {
	return fmt.Sprintf("import(%s)", obj.Name)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtImport) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtImport) Init(data *interfaces.Data) error {
	obj.Textarea.Setup(data)

	if obj.Name == "" {
		return fmt.Errorf("import name is empty")
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *StmtImport) Interpolate() (interfaces.Stmt, error) {
	return &StmtImport{
		Textarea: obj.Textarea,
		data:     obj.data,
		Name:     obj.Name,
		Alias:    obj.Alias,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtImport) Copy() (interfaces.Stmt, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// Nothing special happens in this method, the import magic happens in StmtProg.
func (obj *StmtImport) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Since we always run the imports before anything else in the StmtProg,
	// we don't need to do anything special in here.
	// TODO: If this statement is true, add this in so that imports can be
	// done in the same iteration through StmtProg in SetScope with all of
	// the other statements.

	cons := make(map[interfaces.Node]string)
	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *StmtImport) SetScope(*interfaces.Scope) error { return nil }

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtImport) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	if obj.Name == "" {
		return nil, fmt.Errorf("missing import name")
	}

	return []*interfaces.UnificationInvariant{}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular statement just returns an empty graph.
func (obj *StmtImport) Graph(*interfaces.Env) (*pgraph.Graph, error) {
	return pgraph.NewGraph("import") // empty graph
}

// Output returns the output that this include produces. This output is what is
// used to build the output graph. This only exists for statements. The
// analogous function for expressions is Value. Those Value functions might get
// called by this Output function if they are needed to produce the output. This
// import statement itself produces no output, as it is only used to populate
// the scope so that others can use that to produce values and output.
func (obj *StmtImport) Output(map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	return interfaces.EmptyOutput(), nil
}

// StmtComment is a representation of a comment. It is currently unused. It
// probably makes sense to make a third kind of Node (not a Stmt or an Expr) so
// that comments can still be part of the AST (for eventual automatic code
// formatting) but so that they can exist anywhere in the code. Currently these
// are dropped by the lexer.
type StmtComment struct {
	interfaces.Textarea

	Value string
}

// String returns a short representation of this statement.
func (obj *StmtComment) String() string {
	return fmt.Sprintf("comment(%s)", obj.Value)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *StmtComment) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *StmtComment) Init(data *interfaces.Data) error {
	//obj.data = data
	obj.Textarea.Setup(data)

	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it simply returns itself, as no interpolation is possible.
func (obj *StmtComment) Interpolate() (interfaces.Stmt, error) {
	return &StmtComment{
		Value: obj.Value,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *StmtComment) Copy() (interfaces.Stmt, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *StmtComment) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)
	return graph, cons, nil
}

// SetScope does nothing for this struct, because it has no child nodes, and it
// does not need to know about the parent scope.
func (obj *StmtComment) SetScope(*interfaces.Scope) error { return nil }

// TypeCheck returns the list of invariants that this node produces. It does so
// recursively on any children elements that exist in the AST, and returns the
// collection to the caller. It calls TypeCheck for child statements, and
// Infer/Check for child expressions.
func (obj *StmtComment) TypeCheck() ([]*interfaces.UnificationInvariant, error) {
	return []*interfaces.UnificationInvariant{}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular graph does nothing clever.
func (obj *StmtComment) Graph(*interfaces.Env) (*pgraph.Graph, error) {
	return pgraph.NewGraph("comment")
}

// Output for the comment statement produces no output.
func (obj *StmtComment) Output(map[interfaces.Func]types.Value) (*interfaces.Output, error) {
	return interfaces.EmptyOutput(), nil
}

// ExprBool is a representation of a boolean.
type ExprBool struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	V bool
}

// String returns a short representation of this expression.
func (obj *ExprBool) String() string { return fmt.Sprintf("bool(%t)", obj.V) }

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprBool) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprBool) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it simply returns itself, as no interpolation is possible.
func (obj *ExprBool) Interpolate() (interfaces.Expr, error) {
	return &ExprBool{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		V:        obj.V,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprBool) Copy() (interfaces.Expr, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprBool) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)
	return graph, cons, nil
}

// SetScope does nothing for this struct, because it has no child nodes, and it
// does not need to know about the parent scope. It does however store it for
// later possible use.
func (obj *ExprBool) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope
	return nil
}

// SetType will make no changes if called here. It will error if anything other
// than a Bool is passed in, and doesn't need to be called for this expr to
// work.
func (obj *ExprBool) SetType(typ *types.Type) error { return types.TypeBool.Cmp(typ) }

// Type returns the type of this expression. This method always returns Bool
// here.
func (obj *ExprBool) Type() (*types.Type, error) { return types.TypeBool, nil }

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprBool) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	return types.TypeBool, []*interfaces.UnificationInvariant{
		{
			Node:   obj,
			Expr:   obj,
			Expect: types.TypeBool,
			Actual: types.TypeBool,
		},
	}, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprBool) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprBool) Func() (interfaces.Func, error) {
	return &structs.ConstFunc{
		Textarea: obj.Textarea,

		Value: &types.BoolValue{V: obj.V},
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it.
func (obj *ExprBool) Graph(*interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("bool")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)
	return graph, function, nil
}

// SetValue for a bool expression is always populated statically, and does not
// ever receive any incoming values (no incoming edges) so this should never be
// called. It has been implemented for uniformity.
func (obj *ExprBool) SetValue(value types.Value) error {
	if err := types.TypeBool.Cmp(value.Type()); err != nil {
		return err
	}
	// XXX: should we compare the incoming value with the stored value?
	obj.V = value.Bool()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This particular value is always known since it is a constant.
func (obj *ExprBool) Value() (types.Value, error) {
	return &types.BoolValue{
		V: obj.V,
	}, nil
}

// ExprStr is a representation of a string.
type ExprStr struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	V string // value of this string
}

// String returns a short representation of this expression.
func (obj *ExprStr) String() string { return fmt.Sprintf("str(%s)", strconv.Quote(obj.V)) }

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprStr) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprStr) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it attempts to expand the string if there are any internal variables
// which need interpolation. If any are found, it returns a larger AST which has
// a function which returns a string as its root. Otherwise it returns itself.
func (obj *ExprStr) Interpolate() (interfaces.Expr, error) {
	data := &interfaces.Data{
		// TODO: add missing fields here if/when needed
		Fs:       obj.data.Fs,
		FsURI:    obj.data.FsURI,
		Base:     obj.data.Base,
		Files:    obj.data.Files,
		Imports:  obj.data.Imports,
		Metadata: obj.data.Metadata,
		Modules:  obj.data.Modules,

		LexParser:       obj.data.LexParser,
		Downloader:      obj.data.Downloader,
		StrInterpolater: obj.data.StrInterpolater,
		SourceFinder:    obj.data.SourceFinder,
		//World: obj.data.World, // TODO: do we need this?

		Prefix: obj.data.Prefix,
		Debug:  obj.data.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.data.Logf("interpolate: "+format, v...)
		},
	}

	result, err := obj.data.StrInterpolater(obj.V, &obj.Textarea, data)
	if err != nil {
		return nil, err
	}
	// a nil result means unchanged, string didn't need interpolating done
	if result == nil { // we still copy since Interpolate always "copies"
		return &ExprStr{
			Textarea: obj.Textarea,
			data:     obj.data,
			scope:    obj.scope,
			V:        obj.V,
		}, nil
	}
	// we got something, overwrite the existing static str
	// ensure str, to avoid a pass-through list in a simple interpolation
	if err := result.SetType(types.TypeStr); err != nil {
		return nil, errwrap.Wrapf(err, "interpolated string expected a different type")
	}
	return result, nil // replacement
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprStr) Copy() (interfaces.Expr, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
// This Ordering method runs *after* the Interpolate method, so if this
// originally would have expanded into a bigger AST, but the time Ordering runs,
// this is only used on a raw string expression. As a result, it doesn't need to
// build a map of consumed nodes, because none are consumed. The returned graph
// is empty!
func (obj *ExprStr) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)
	return graph, cons, nil
}

// SetScope does nothing for this struct, because it has no child nodes, and it
// does not need to know about the parent scope. It does however store it for
// later possible use.
func (obj *ExprStr) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope
	return nil
}

// SetType will make no changes if called here. It will error if anything other
// than an Str is passed in, and doesn't need to be called for this expr to
// work.
func (obj *ExprStr) SetType(typ *types.Type) error { return types.TypeStr.Cmp(typ) }

// Type returns the type of this expression. This method always returns Str
// here.
func (obj *ExprStr) Type() (*types.Type, error) { return types.TypeStr, nil }

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprStr) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	return types.TypeStr, []*interfaces.UnificationInvariant{
		{
			Node:   obj,
			Expr:   obj,
			Expect: types.TypeStr,
			Actual: types.TypeStr,
		},
	}, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprStr) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprStr) Func() (interfaces.Func, error) {
	return &structs.ConstFunc{
		Textarea: obj.Textarea,

		Value: &types.StrValue{V: obj.V},
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it.
func (obj *ExprStr) Graph(*interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("str")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)
	return graph, function, nil
}

// SetValue for an str expression is always populated statically, and does not
// ever receive any incoming values (no incoming edges) so this should never be
// called. It has been implemented for uniformity.
func (obj *ExprStr) SetValue(value types.Value) error {
	if err := types.TypeStr.Cmp(value.Type()); err != nil {
		return err
	}
	obj.V = value.Str()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This particular value is always known since it is a constant.
func (obj *ExprStr) Value() (types.Value, error) {
	return &types.StrValue{
		V: obj.V,
	}, nil
}

// ExprInt is a representation of an int.
type ExprInt struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	V int64
}

// String returns a short representation of this expression.
func (obj *ExprInt) String() string { return fmt.Sprintf("int(%d)", obj.V) }

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprInt) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprInt) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it simply returns itself, as no interpolation is possible.
func (obj *ExprInt) Interpolate() (interfaces.Expr, error) {
	return &ExprInt{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		V:        obj.V,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprInt) Copy() (interfaces.Expr, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprInt) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)
	return graph, cons, nil
}

// SetScope does nothing for this struct, because it has no child nodes, and it
// does not need to know about the parent scope. It does however store it for
// later possible use.
func (obj *ExprInt) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope
	return nil
}

// SetType will make no changes if called here. It will error if anything other
// than an Int is passed in, and doesn't need to be called for this expr to
// work.
func (obj *ExprInt) SetType(typ *types.Type) error { return types.TypeInt.Cmp(typ) }

// Type returns the type of this expression. This method always returns Int
// here.
func (obj *ExprInt) Type() (*types.Type, error) { return types.TypeInt, nil }

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprInt) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	return types.TypeInt, []*interfaces.UnificationInvariant{
		{
			Node:   obj,
			Expr:   obj,
			Expect: types.TypeInt,
			Actual: types.TypeInt,
		},
	}, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprInt) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprInt) Func() (interfaces.Func, error) {
	return &structs.ConstFunc{
		Textarea: obj.Textarea,

		Value: &types.IntValue{V: obj.V},
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it.
func (obj *ExprInt) Graph(*interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("int")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)
	return graph, function, nil
}

// SetValue for an int expression is always populated statically, and does not
// ever receive any incoming values (no incoming edges) so this should never be
// called. It has been implemented for uniformity.
func (obj *ExprInt) SetValue(value types.Value) error {
	if err := types.TypeInt.Cmp(value.Type()); err != nil {
		return err
	}
	obj.V = value.Int()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This particular value is always known since it is a constant.
func (obj *ExprInt) Value() (types.Value, error) {
	return &types.IntValue{
		V: obj.V,
	}, nil
}

// ExprFloat is a representation of a float.
type ExprFloat struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later

	V float64
}

// String returns a short representation of this expression.
func (obj *ExprFloat) String() string {
	return fmt.Sprintf("float(%g)", obj.V) // TODO: %f instead?
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprFloat) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprFloat) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it simply returns itself, as no interpolation is possible.
func (obj *ExprFloat) Interpolate() (interfaces.Expr, error) {
	return &ExprFloat{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		V:        obj.V,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprFloat) Copy() (interfaces.Expr, error) {
	return obj, nil // always static
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprFloat) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)
	return graph, cons, nil
}

// SetScope does nothing for this struct, because it has no child nodes, and it
// does not need to know about the parent scope. It does however store it for
// later possible use.
func (obj *ExprFloat) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope
	return nil
}

// SetType will make no changes if called here. It will error if anything other
// than a Float is passed in, and doesn't need to be called for this expr to
// work.
func (obj *ExprFloat) SetType(typ *types.Type) error { return types.TypeFloat.Cmp(typ) }

// Type returns the type of this expression. This method always returns Float
// here.
func (obj *ExprFloat) Type() (*types.Type, error) { return types.TypeFloat, nil }

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprFloat) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	return types.TypeFloat, []*interfaces.UnificationInvariant{
		{
			Node:   obj,
			Expr:   obj,
			Expect: types.TypeFloat,
			Actual: types.TypeFloat,
		},
	}, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprFloat) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprFloat) Func() (interfaces.Func, error) {
	return &structs.ConstFunc{
		Textarea: obj.Textarea,

		Value: &types.FloatValue{V: obj.V},
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it.
func (obj *ExprFloat) Graph(*interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("float")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)
	return graph, function, nil
}

// SetValue for a float expression is always populated statically, and does not
// ever receive any incoming values (no incoming edges) so this should never be
// called. It has been implemented for uniformity.
func (obj *ExprFloat) SetValue(value types.Value) error {
	if err := types.TypeFloat.Cmp(value.Type()); err != nil {
		return err
	}
	obj.V = value.Float()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This particular value is always known since it is a constant.
func (obj *ExprFloat) Value() (types.Value, error) {
	return &types.FloatValue{
		V: obj.V,
	}, nil
}

// ExprList is a representation of a list.
type ExprList struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	//Elements []*ExprListElement
	Elements []interfaces.Expr
}

// String returns a short representation of this expression.
func (obj *ExprList) String() string {
	var s []string
	for _, x := range obj.Elements {
		s = append(s, x.String())
	}
	return fmt.Sprintf("list(%s)", strings.Join(s, ", "))
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprList) Apply(fn func(interfaces.Node) error) error {
	for _, x := range obj.Elements {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprList) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	for _, x := range obj.Elements {
		if err := x.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprList) Interpolate() (interfaces.Expr, error) {
	elements := []interfaces.Expr{}
	for _, x := range obj.Elements {
		interpolated, err := x.Interpolate()
		if err != nil {
			return nil, err
		}
		elements = append(elements, interpolated)
	}
	return &ExprList{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Elements: elements,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprList) Copy() (interfaces.Expr, error) {
	copied := false
	elements := []interfaces.Expr{}
	for _, x := range obj.Elements {
		cp, err := x.Copy()
		if err != nil {
			return nil, err
		}
		if cp != x { // must have been copied, or pointer would be same
			copied = true
		}
		elements = append(elements, cp)
	}

	if !copied { // it's static
		return obj, nil
	}
	return &ExprList{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Elements: elements,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprList) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)

	for _, node := range obj.Elements {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "exprlistelement"}
		graph.AddEdge(node, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprlist"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *ExprList) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope

	for _, x := range obj.Elements {
		if err := x.SetScope(scope, sctx); err != nil {
			return err
		}
	}
	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprList) SetType(typ *types.Type) error {
	// TODO: should we ensure this is set to a KindList ?
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprList) Type() (*types.Type, error) {
	var typ *types.Type
	var err error
	for i, expr := range obj.Elements {
		etyp, e := expr.Type()
		if e != nil {
			err = errwrap.Wrapf(e, "list index `%d` did not return a type", i)
			break
		}
		if typ == nil {
			typ = etyp
		}
		if e := typ.Cmp(etyp); e != nil {
			err = errwrap.Wrapf(e, "list elements have different types")
			break
		}
	}
	if err == nil && obj.typ == nil && len(obj.Elements) > 0 {
		return &types.Type{ // speculate!
			Kind: types.KindList,
			Val:  typ,
		}, nil
	}

	if obj.typ == nil {
		if err != nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, err.Error())
		}
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprList) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	// Same unification var because all values in the list have same type.
	typ := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}
	typExpr := &types.Type{
		Kind: types.KindList,
		Val:  typ,
	}

	for _, x := range obj.Elements {
		invars, err := x.Check(typ) // typ of the list element
		if err != nil {
			return nil, nil, err
		}
		invariants = append(invariants, invars...)
	}

	// Every infer call must have this section, because expr var needs this.
	typType := typExpr
	//if obj.typ == nil { // optional says sam
	//	obj.typ = typExpr // sam says we could unconditionally do this
	//}
	if obj.typ != nil {
		typType = obj.typ
	}
	// This must be added even if redundant, so that we collect the obj ptr.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typExpr, // This is the type that we return.
		Actual: typType,
	}
	invariants = append(invariants, invar)

	return typExpr, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprList) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprList) Func() (interfaces.Func, error) {
	typ, err := obj.Type()
	if err != nil {
		return nil, err
	}

	// composite func (list, map, struct)
	return &structs.CompositeFunc{
		Textarea: obj.Textarea,

		Type: typ,
		Len:  len(obj.Elements),
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it, and
// the edges from all of the child graphs to this.
func (obj *ExprList) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("list")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)

	// each list element needs to point to the final list expression
	for index, x := range obj.Elements { // list elements in order
		g, f, err := x.Graph(env)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)

		fieldName := fmt.Sprintf("%d", index) // argNames as integers!
		edge := &interfaces.FuncEdge{Args: []string{fieldName}}
		graph.AddEdge(f, function, edge) // element -> list
	}

	return graph, function, nil
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child elements (the list elements) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprList) SetValue(value types.Value) error {
	if err := obj.typ.Cmp(value.Type()); err != nil {
		return err
	}
	// noop!
	//obj.V = value
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprList) Value() (types.Value, error) {
	values := []types.Value{}
	var typ *types.Type

	for i, expr := range obj.Elements {
		etyp, err := expr.Type()
		if err != nil {
			return nil, errwrap.Wrapf(err, "list index `%d` did not return a type", i)
		}
		if typ == nil {
			typ = etyp
		}
		if err := typ.Cmp(etyp); err != nil {
			return nil, errwrap.Wrapf(err, "list elements have different types")
		}

		value, err := expr.Value()
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, fmt.Errorf("value for list index `%d` was nil", i)
		}
		values = append(values, value)
	}
	if len(obj.Elements) > 0 {
		t := &types.Type{
			Kind: types.KindList,
			Val:  typ,
		}
		// Run SetType to ensure type is consistent with what we found,
		// which is an easy way to ensure the Cmp passes as expected...
		if err := obj.SetType(t); err != nil {
			return nil, errwrap.Wrapf(err, "type did not match expected!")
		}
	}

	return &types.ListValue{
		T: obj.typ,
		V: values,
	}, nil
}

// ExprMap is a representation of a (dictionary) map.
type ExprMap struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	KVs []*ExprMapKV
}

// String returns a short representation of this expression.
func (obj *ExprMap) String() string {
	var s []string
	for _, x := range obj.KVs {
		s = append(s, fmt.Sprintf("%s: %s", x.Key.String(), x.Val.String()))
	}
	return fmt.Sprintf("map(%s)", strings.Join(s, ", "))
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprMap) Apply(fn func(interfaces.Node) error) error {
	for _, x := range obj.KVs {
		if err := x.Key.Apply(fn); err != nil {
			return err
		}
		if err := x.Val.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprMap) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	// XXX: Can we check that there aren't any duplicate keys? Can we Cmp?
	for _, x := range obj.KVs {
		if err := x.Key.Init(data); err != nil {
			return err
		}
		if err := x.Val.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprMap) Interpolate() (interfaces.Expr, error) {
	kvs := []*ExprMapKV{}
	for _, x := range obj.KVs {
		interpolatedKey, err := x.Key.Interpolate()
		if err != nil {
			return nil, err
		}
		interpolatedVal, err := x.Val.Interpolate()
		if err != nil {
			return nil, err
		}
		kv := &ExprMapKV{
			Key: interpolatedKey,
			Val: interpolatedVal,
		}
		kvs = append(kvs, kv)
	}
	return &ExprMap{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		KVs:      kvs,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprMap) Copy() (interfaces.Expr, error) {
	copied := false
	kvs := []*ExprMapKV{}
	for _, x := range obj.KVs {
		copiedKV := false
		copyKey, err := x.Key.Copy()
		if err != nil {
			return nil, err
		}
		// must have been copied, or pointer would be same
		if copyKey != x.Key {
			copiedKV = true
		}
		copyVal, err := x.Val.Copy()
		if err != nil {
			return nil, err
		}
		if copyVal != x.Val {
			copiedKV = true
		}
		kv := &ExprMapKV{
			Key: copyKey,
			Val: copyVal,
		}
		if copiedKV {
			copied = true
		} else {
			kv = x // don't re-package it unnecessarily!
		}
		kvs = append(kvs, kv)
	}

	if !copied { // it's static
		return obj, nil
	}
	return &ExprMap{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		KVs:      kvs,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprMap) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)

	for _, node := range obj.KVs {
		g1, c1, err := node.Key.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g1) // add in the child graph

		// additional constraint...
		edge1 := &pgraph.SimpleEdge{Name: "exprmapkey"}
		graph.AddEdge(node.Key, obj, edge1) // prod -> cons

		for k, v := range c1 { // c1 is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprmapkey"}
			graph.AddEdge(n, k, edge)
		}

		g2, c2, err := node.Val.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g2) // add in the child graph

		// additional constraint...
		edge2 := &pgraph.SimpleEdge{Name: "exprmapval"}
		graph.AddEdge(node.Val, obj, edge2) // prod -> cons

		for k, v := range c2 { // c2 is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprmapval"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *ExprMap) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope

	for _, x := range obj.KVs {
		if err := x.Key.SetScope(scope, sctx); err != nil {
			return err
		}
		if err := x.Val.SetScope(scope, sctx); err != nil {
			return err
		}
	}
	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprMap) SetType(typ *types.Type) error {
	// TODO: should we ensure this is set to a KindMap ?
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprMap) Type() (*types.Type, error) {
	var ktyp, vtyp *types.Type
	var err error
	for i, x := range obj.KVs {
		// keys
		kt, e := x.Key.Type()
		if e != nil {
			err = errwrap.Wrapf(e, "map key, index `%d` did not return a type", i)
			break
		}
		if ktyp == nil {
			ktyp = kt
		}
		if e := ktyp.Cmp(kt); e != nil {
			err = errwrap.Wrapf(e, "key elements have different types")
			break
		}

		// vals
		vt, e := x.Val.Type()
		if e != nil {
			err = errwrap.Wrapf(e, "map val, index `%d` did not return a type", i)
			break
		}
		if vtyp == nil {
			vtyp = vt
		}
		if e := vtyp.Cmp(vt); e != nil {
			err = errwrap.Wrapf(e, "val elements have different types")
			break
		}
	}
	if err == nil && obj.typ == nil && len(obj.KVs) > 0 {
		return &types.Type{ // speculate!
			Kind: types.KindMap,
			Key:  ktyp,
			Val:  vtyp,
		}, nil
	}

	if obj.typ == nil {
		if err != nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, err.Error())
		}
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprMap) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	// Same unification var because all key/val's in the map have same type.
	ktyp := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}
	vtyp := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?2
	}
	typExpr := &types.Type{
		Kind: types.KindMap,
		Key:  ktyp,
		Val:  vtyp,
	}

	for _, x := range obj.KVs {
		keyInvars, err := x.Key.Check(ktyp) // typ of the map key
		if err != nil {
			return nil, nil, err
		}
		invariants = append(invariants, keyInvars...)

		valInvars, err := x.Val.Check(vtyp) // typ of the map val
		if err != nil {
			return nil, nil, err
		}
		invariants = append(invariants, valInvars...)
	}

	// Every infer call must have this section, because expr var needs this.
	typType := typExpr
	//if obj.typ == nil { // optional says sam
	//	obj.typ = typExpr // sam says we could unconditionally do this
	//}
	if obj.typ != nil {
		typType = obj.typ
	}
	// This must be added even if redundant, so that we collect the obj ptr.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typExpr, // This is the type that we return.
		Actual: typType,
	}
	invariants = append(invariants, invar)

	return typExpr, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprMap) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprMap) Func() (interfaces.Func, error) {
	typ, err := obj.Type()
	if err != nil {
		return nil, err
	}

	// composite func (list, map, struct)
	return &structs.CompositeFunc{
		Textarea: obj.Textarea,

		Type: typ, // the key/val types are known via this type
		Len:  len(obj.KVs),
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it, and
// the edges from all of the child graphs to this.
func (obj *ExprMap) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("map")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)

	// each map key value pair needs to point to the final map expression
	for index, x := range obj.KVs { // map fields in order
		g, f, err := x.Key.Graph(env)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)

		// do the key names ever change? -- yes
		fieldName := fmt.Sprintf("key:%d", index) // stringify map key
		edge := &interfaces.FuncEdge{Args: []string{fieldName}}
		graph.AddEdge(f, function, edge) // key -> map
	}

	// each map key value pair needs to point to the final map expression
	for index, x := range obj.KVs { // map fields in order
		g, f, err := x.Val.Graph(env)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)

		fieldName := fmt.Sprintf("val:%d", index) // stringify map val
		edge := &interfaces.FuncEdge{Args: []string{fieldName}}
		graph.AddEdge(f, function, edge) // val -> map
	}

	return graph, function, nil
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child key/value's (the map elements) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprMap) SetValue(value types.Value) error {
	if err := obj.typ.Cmp(value.Type()); err != nil {
		return err
	}
	// noop!
	//obj.V = value
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprMap) Value() (types.Value, error) {
	kvs := make(map[types.Value]types.Value)
	var ktyp, vtyp *types.Type

	for i, x := range obj.KVs {
		// keys
		kt, err := x.Key.Type()
		if err != nil {
			return nil, errwrap.Wrapf(err, "map key, index `%d` did not return a type", i)
		}
		if ktyp == nil {
			ktyp = kt
		}
		if err := ktyp.Cmp(kt); err != nil {
			return nil, errwrap.Wrapf(err, "key elements have different types")
		}

		key, err := x.Key.Value()
		if err != nil {
			return nil, err
		}
		if key == nil {
			return nil, fmt.Errorf("key for map index `%d` was nil", i)
		}

		// vals
		vt, err := x.Val.Type()
		if err != nil {
			return nil, errwrap.Wrapf(err, "map val, index `%d` did not return a type", i)
		}
		if vtyp == nil {
			vtyp = vt
		}
		if err := vtyp.Cmp(vt); err != nil {
			return nil, errwrap.Wrapf(err, "val elements have different types")
		}

		val, err := x.Val.Value()
		if err != nil {
			return nil, err
		}
		if val == nil {
			return nil, fmt.Errorf("val for map index `%d` was nil", i)
		}

		kvs[key] = val // add to map
	}
	if len(obj.KVs) > 0 {
		t := &types.Type{
			Kind: types.KindMap,
			Key:  ktyp,
			Val:  vtyp,
		}
		// Run SetType to ensure type is consistent with what we found,
		// which is an easy way to ensure the Cmp passes as expected...
		if err := obj.SetType(t); err != nil {
			return nil, errwrap.Wrapf(err, "type did not match expected!")
		}
	}

	return &types.MapValue{
		T: obj.typ,
		V: kvs,
	}, nil
}

// ExprMapKV represents a key and value pair in a (dictionary) map. This does
// not satisfy the Expr interface.
type ExprMapKV struct {
	interfaces.Textarea

	Key interfaces.Expr // keys can be strings, int's, etc...
	Val interfaces.Expr
}

// ExprStruct is a representation of a struct.
type ExprStruct struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	Fields []*ExprStructField // the list (fields) are intentionally ordered!
}

// String returns a short representation of this expression.
func (obj *ExprStruct) String() string {
	var s []string
	for _, x := range obj.Fields {
		s = append(s, fmt.Sprintf("%s: %s", x.Name, x.Value.String()))
	}
	return fmt.Sprintf("struct(%s)", strings.Join(s, "; "))
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprStruct) Apply(fn func(interfaces.Node) error) error {
	for _, x := range obj.Fields {
		if err := x.Value.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprStruct) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	fields := make(map[string]struct{})
	for _, x := range obj.Fields {
		// Validate field names and ensure no duplicates!
		if _, exists := fields[x.Name]; exists {
			return fmt.Errorf("duplicate struct field name of: `%s`", x.Name)
		}
		fields[x.Name] = struct{}{}

		if err := x.Value.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprStruct) Interpolate() (interfaces.Expr, error) {
	fields := []*ExprStructField{}
	for _, x := range obj.Fields {
		interpolated, err := x.Value.Interpolate()
		if err != nil {
			return nil, err
		}
		field := &ExprStructField{
			Name:  x.Name, // don't interpolate the key
			Value: interpolated,
		}
		fields = append(fields, field)
	}
	return &ExprStruct{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Fields:   fields,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprStruct) Copy() (interfaces.Expr, error) {
	copied := false
	fields := []*ExprStructField{}
	for _, x := range obj.Fields {
		cp, err := x.Value.Copy()
		if err != nil {
			return nil, err
		}
		// must have been copied, or pointer would be same
		if cp != x.Value {
			copied = true
		}

		field := &ExprStructField{
			Name:  x.Name,
			Value: cp,
		}
		fields = append(fields, field)
	}

	if !copied { // it's static
		return obj, nil
	}
	return &ExprStruct{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Fields:   fields,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprStruct) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	cons := make(map[interfaces.Node]string)

	for _, node := range obj.Fields {
		g, c, err := node.Value.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "exprstructfield"}
		graph.AddEdge(node.Value, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprstruct"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *ExprStruct) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope

	for _, x := range obj.Fields {
		if err := x.Value.SetScope(scope, sctx); err != nil {
			return err
		}
	}
	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprStruct) SetType(typ *types.Type) error {
	// TODO: should we ensure this is set to a KindStruct ?
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprStruct) Type() (*types.Type, error) {
	var m = make(map[string]*types.Type)
	ord := []string{}
	var err error
	for i, x := range obj.Fields {
		// vals
		t, e := x.Value.Type()
		if e != nil {
			err = errwrap.Wrapf(e, "field val, index `%d` did not return a type", i)
			break
		}
		if _, exists := m[x.Name]; exists {
			err = fmt.Errorf("struct type field index `%d` already exists", i)
			break
		}
		m[x.Name] = t
		ord = append(ord, x.Name)
	}
	if err == nil && obj.typ == nil && len(obj.Fields) > 0 {
		return &types.Type{ // speculate!
			Kind: types.KindStruct,
			Map:  m,
			Ord:  ord, // assume same order as fields
		}, nil
	}

	if obj.typ == nil {
		if err != nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, err.Error())
		}
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprStruct) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	m := make(map[string]*types.Type)
	ord := []string{}

	// Different unification var for each field in the struct.
	for _, x := range obj.Fields {
		typ := &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}

		m[x.Name] = typ
		ord = append(ord, x.Name)

		invars, err := x.Value.Check(typ) // typ of the struct field
		if err != nil {
			return nil, nil, err
		}
		invariants = append(invariants, invars...)
	}

	typExpr := &types.Type{
		Kind: types.KindStruct,
		Map:  m,
		Ord:  ord,
	}

	// Every infer call must have this section, because expr var needs this.
	typType := typExpr
	//if obj.typ == nil { // optional says sam
	//	obj.typ = typExpr // sam says we could unconditionally do this
	//}
	if obj.typ != nil {
		typType = obj.typ
	}
	// This must be added even if redundant, so that we collect the obj ptr.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typExpr, // This is the type that we return.
		Actual: typType,
	}
	invariants = append(invariants, invar)

	return typExpr, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprStruct) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns the reactive stream of values that this expression produces.
func (obj *ExprStruct) Func() (interfaces.Func, error) {
	typ, err := obj.Type()
	if err != nil {
		return nil, err
	}

	// composite func (list, map, struct)
	return &structs.CompositeFunc{
		Textarea: obj.Textarea,

		Type: typ,
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it, and
// the edges from all of the child graphs to this.
func (obj *ExprStruct) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("struct")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(function)

	// each struct field needs to point to the final struct expression
	for _, x := range obj.Fields { // struct fields in order
		g, f, err := x.Value.Graph(env)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)

		fieldName := x.Name
		edge := &interfaces.FuncEdge{Args: []string{fieldName}}
		graph.AddEdge(f, function, edge) // field -> struct
	}

	return graph, function, nil
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the struct elements) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprStruct) SetValue(value types.Value) error {
	if err := obj.typ.Cmp(value.Type()); err != nil {
		return err
	}
	// noop!
	//obj.V = value
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprStruct) Value() (types.Value, error) {
	fields := make(map[string]types.Value)
	typ := &types.Type{
		Kind: types.KindStruct,
		Map:  make(map[string]*types.Type),
		//Ord:  obj.typ.Ord, // assume same order
	}
	ord := []string{} // can't use obj.typ b/c it can be nil during speculation

	for i, x := range obj.Fields {
		// vals
		t, err := x.Value.Type()
		if err != nil {
			return nil, errwrap.Wrapf(err, "field val, index `%d` did not return a type", i)
		}
		if _, exists := typ.Map[x.Name]; exists {
			return nil, fmt.Errorf("struct type field index `%d` already exists", i)
		}
		typ.Map[x.Name] = t

		val, err := x.Value.Value()
		if err != nil {
			return nil, err
		}
		if val == nil {
			return nil, fmt.Errorf("val for field index `%d` was nil", i)
		}

		if _, exists := fields[x.Name]; exists {
			return nil, fmt.Errorf("struct field index `%d` already exists", i)
		}
		fields[x.Name] = val // add to map
		ord = append(ord, x.Name)
	}
	typ.Ord = ord
	if len(obj.Fields) > 0 {
		// Run SetType to ensure type is consistent with what we found,
		// which is an easy way to ensure the Cmp passes as expected...
		if err := obj.SetType(typ); err != nil {
			return nil, errwrap.Wrapf(err, "type did not match expected!")
		}
	}

	return &types.StructValue{
		T: obj.typ,
		V: fields,
	}, nil
}

// ExprStructField represents a name value pair in a struct field. This does not
// satisfy the Expr interface.
type ExprStructField struct {
	interfaces.Textarea

	Name  string
	Value interfaces.Expr
}

// ExprFunc is a representation of a function value. This is not a function
// call, that is represented by ExprCall.
//
// There are several kinds of functions which can be represented:
// 1. The contents of a StmtFunc (set Args, Return, and Body)
// 2. A lambda function (also set Args, Return, and Body)
// 3. A stateful built-in function (set Function)
// 4. A pure built-in function (set Values to a singleton)
// 5. A pure polymorphic built-in function (set Values to a list)
type ExprFunc struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	// Title is a friendly-name to use for identifying the function. It can
	// be used in debugging and error-handling. It is not required. It is
	// *not* called Name, because that could get confused with the Name
	// field in ExprCall and similar nodes.
	Title string

	// Args are the list of args that were used when defining the function.
	// This can include a string name and a type, however the type might be
	// absent here.
	Args []*interfaces.Arg

	// One ExprParam is created for each parameter, and the ExprVars which
	// refer to those parameters are set to point to the corresponding
	// ExprParam.
	params []*ExprParam

	// Return is the return type of the function if it was defined.
	Return *types.Type // return type if specified
	// Body is the contents of the function. It can be any expression.
	Body interfaces.Expr

	// Function is the built implementation of the function interface as
	// represented by the top-level function API.
	Function func() interfaces.Func // store like this to build on demand!
	function interfaces.Func        // store the built version here...

	// Values represents a list of simple functions. This means this can be
	// polymorphic if more than one was specified!
	Values []*types.FuncValue

	// XXX: is this necessary?
	//V func(interfaces.Txn, []pgraph.Vertex) (pgraph.Vertex, error)
}

// String returns a short representation of this expression.
func (obj *ExprFunc) String() string {
	if len(obj.Values) == 1 {
		if obj.Title != "" {
			return fmt.Sprintf("func() { <built-in:%s (simple)> }", obj.Title)
		}
		return "func() { <built-in (simple)> }"
	} else if len(obj.Values) > 0 {
		if obj.Title != "" {
			return fmt.Sprintf("func() { <built-in:%s (simple, poly)> }", obj.Title)
		}
		return "func() { <built-in (simple, poly)> }"
	}
	if obj.Function != nil {
		if obj.Title != "" {
			return fmt.Sprintf("func() { <built-in:%s> }", obj.Title)
		}
		return "func() { <built-in> }"
	}
	if obj.Body == nil {
		panic("function expression was not built correctly")
	}

	var a []string
	for _, x := range obj.Args {
		a = append(a, fmt.Sprintf("%s", x.String()))
	}
	args := strings.Join(a, ", ")
	s := fmt.Sprintf("func(%s)", args)
	if obj.Title != "" {
		s = fmt.Sprintf("func:%s(%s)", obj.Title, args) // overwrite!
	}
	if obj.Return != nil {
		s += fmt.Sprintf(" %s", obj.Return.String())
	}
	s += fmt.Sprintf(" { %s }", obj.Body.String())
	return s
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprFunc) Apply(fn func(interfaces.Node) error) error {
	if obj.Body != nil {
		if err := obj.Body.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprFunc) Init(data *interfaces.Data) error {
	obj.data = data // TODO: why is this sometimes nil?
	obj.Textarea.Setup(data)

	// validate that we're using *only* one correct representation
	a := obj.Body != nil
	b := obj.Function != nil
	c := len(obj.Values) > 0
	if (a && b || b && c) || !a && !b && !c {
		return fmt.Errorf("function expression was not built correctly")
	}

	if obj.Body != nil {
		if err := obj.Body.Init(data); err != nil {
			return err
		}
	}

	if obj.Function != nil {
		if obj.function != nil { // check for double Init!
			// programming error!
			return fmt.Errorf("func is being re-built")
		}
		obj.function = obj.Function() // build it
		// pass in some data to the function
		// TODO: do we want to pass in the full obj.data instead ?
		if dataFunc, ok := obj.function.(interfaces.DataFunc); ok {
			dataFunc.SetData(&interfaces.FuncData{
				Fs:    obj.data.Fs,
				FsURI: obj.data.FsURI,
				Base:  obj.data.Base,
			})
		}
	}

	if len(obj.Values) > 0 {
		typs := []*types.Type{}
		for _, f := range obj.Values {
			if f.T == nil {
				return fmt.Errorf("func contains a nil type signature")
			}
			typs = append(typs, f.T)
		}
		if err := langUtil.HasDuplicateTypes(typs); err != nil {
			return errwrap.Wrapf(err, "func list contains a duplicate signature")
		}
	}

	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it simply returns itself, as no interpolation is possible.
func (obj *ExprFunc) Interpolate() (interfaces.Expr, error) {
	var body interfaces.Expr
	if obj.Body != nil {
		var err error
		body, err = obj.Body.Interpolate()
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not interpolate Body")
		}
	}

	args := obj.Args
	if obj.Args == nil {
		args = []*interfaces.Arg{}
	}

	return &ExprFunc{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Title:    obj.Title,
		Args:     args,
		params:   obj.params,
		Return:   obj.Return,
		Body:     body,
		Function: obj.Function,
		function: obj.function,
		Values:   obj.Values,
		//V:        obj.V,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
// All the constants aren't copied, because we don't want to duplicate them
// unnecessarily in the function graph. For example, an static integer will not
// ever change, where as a function value (expr) might get used with two
// different signatures depending on the caller.
func (obj *ExprFunc) Copy() (interfaces.Expr, error) {
	// I think we want to copy anything in the Expr tree that has at least
	// one input... Eg: we DON'T want to copy an ExprStr but we DO want to
	// copy an ExprVar because it gets an input edge.
	copied := false
	var body interfaces.Expr
	if obj.Body != nil {
		var err error
		//body, err = obj.Body.Interpolate() // an inefficient copy works!
		body, err = obj.Body.Copy()
		if err != nil {
			return nil, err
		}
		// must have been copied, or pointer would be same
		if body != obj.Body {
			copied = true
		}
	}

	var function interfaces.Func
	if obj.Function != nil {
		// We sometimes copy the ExprFunc because we're using the same
		// one in two places, and it might have a different type and
		// type unification needs to solve for it in more than one way.
		// It also turns out that some functions such as the struct
		// lookup function store information that they learned during
		// `FuncInfer`, and as a result, if we re-build this, then we
		// lose that information and the function can then fail during
		// `Build`. As a result, those functions can implement a `Copy`
		// method which we will use instead, so they can preserve any
		// internal state that they would like to keep.
		copyableFunc, isCopyableFunc := obj.function.(interfaces.CopyableFunc)
		if obj.function == nil || !isCopyableFunc {
			function = obj.Function() // force re-build a new pointer here!
		} else {
			// is copyable!
			function = copyableFunc.Copy()
		}

		// restore the type we previously set in SetType()
		if obj.typ != nil {
			buildableFn, ok := function.(interfaces.BuildableFunc) // is it statically polymorphic?
			if ok {
				newTyp, err := buildableFn.Build(obj.typ)
				if err != nil {
					return nil, err // don't wrap, err is ok
				}
				// Cmp doesn't compare arg names. Check it's compatible...
				if err := obj.typ.Cmp(newTyp); err != nil {
					return nil, errwrap.Wrapf(err, "incompatible type")
				}
			}
		}
		// pass in some data to the function
		// TODO: do we want to pass in the full obj.data instead ?
		if dataFunc, ok := function.(interfaces.DataFunc); ok {
			dataFunc.SetData(&interfaces.FuncData{
				Fs:    obj.data.Fs,
				FsURI: obj.data.FsURI,
				Base:  obj.data.Base,
			})
		}
		copied = true
	}

	if len(obj.Values) > 0 {
		// copied = true // XXX: add this if anyone isn't static?
	}

	// We want to allow static functions, although we have to be careful...
	// Doing this for static functions causes us to hit a strange case in
	// the SetScope function for ExprCall... Investigate if we find a bug...
	if !copied { // it's static
		return obj, nil
	}
	return &ExprFunc{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope, // TODO: copy?
		typ:      obj.typ,
		Title:    obj.Title,
		Args:     obj.Args,
		params:   obj.params, // don't copy says sam!
		Return:   obj.Return,
		Body:     body, // definitely copy
		Function: obj.Function,
		function: function,
		Values:   obj.Values, // XXX: do we need to force rebuild these?
		//V:        obj.V,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprFunc) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	prod := make(map[string]interfaces.Node)
	for _, arg := range obj.Args {
		uid := varOrderingPrefix + arg.Name // ordering id
		//node, exists := produces[uid]
		//if exists {
		//	edge := &pgraph.SimpleEdge{Name: "stmtexprfuncarg"}
		//	graph.AddEdge(node, obj, edge) // prod -> cons
		//}
		prod[uid] = &ExprParam{Name: arg.Name} // placeholder
	}

	newProduces := CopyNodeMapping(produces) // don't modify the input map!

	// Overwrite anything in this scope with the shadowed parent variable!
	for key, val := range prod {
		newProduces[key] = val // copy, and overwrite (shadow) any parent var
	}

	cons := make(map[interfaces.Node]string)

	if obj.Body != nil {
		g, c, err := obj.Body.Ordering(newProduces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "exprfuncbody"}
		graph.AddEdge(obj.Body, obj, edge) // prod -> cons

		cons = c
	}

	// The consumes which have already been matched to one of our produces
	// must not be also matched to a produce from our caller. Is that clear?
	newCons := make(map[interfaces.Node]string) // don't modify the input map!
	for k, v := range cons {
		if _, exists := prod[v]; exists {
			continue
		}
		newCons[k] = v // "remaining" values from cons
	}

	return graph, newCons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *ExprFunc) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope // store for later

	if obj.Body != nil {
		sctxBody := make(map[string]interfaces.Expr)
		for k, v := range sctx {
			sctxBody[k] = v
		}

		// add the parameters to the context (sctx) for the body
		// make a list as long as obj.Args
		obj.params = make([]*ExprParam, len(obj.Args))
		for i, arg := range obj.Args {
			param := newExprParam(
				arg.Name,
				arg.Type,
			)
			obj.params[i] = param
			sctxBody[arg.Name] = param
		}

		if err := obj.Body.SetScope(scope, sctxBody); err != nil {
			return errwrap.Wrapf(err, "failed to set scope on function body")
		}
	}

	if obj.Function != nil {
		// TODO: if interfaces.Func grows a SetScope method do it here
	}
	if len(obj.Values) > 0 {
		// TODO: if *types.FuncValue grows a SetScope method do it here
	}

	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprFunc) SetType(typ *types.Type) error {
	if obj.Body != nil {
		// FIXME: check that it's compatible with Args/Body/Return
	}

	// TODO: should we ensure this is set to a KindFunc ?
	if obj.Function != nil {
		// is it buildable? (formerly statically polymorphic)
		buildableFn, ok := obj.function.(interfaces.BuildableFunc)
		if ok {
			newTyp, err := buildableFn.Build(typ)
			if err != nil {
				return err // don't wrap, err is ok
			}
			// Cmp doesn't compare arg names.
			typ = newTyp // check it's compatible down below...
		} else {
			// Even if it's not a buildable, we'd like to use the
			// real arg names of that function, in case they don't
			// get passed through type unification somehow...
			// (There can be an AST bug that this would prevent.)
			sig := obj.function.Info().Sig
			if sig == nil {
				return fmt.Errorf("could not read nil expr func sig")
			}
			typ = sig // check it's compatible down below...
		}
	}

	if len(obj.Values) > 0 {
		// search for the compatible type
		_, err := langUtil.FnMatch(typ, obj.Values)
		if err != nil {
			return errwrap.Wrapf(err, "could not build values func")
		}
		// TODO: build the function here for later use if that is wanted
		//fn := obj.Values[index].Copy()
		//fn.T = typ.Copy() // overwrites any contained "variant" type
	}

	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression. It will attempt to speculate on the
// type if it can be determined statically before type unification.
func (obj *ExprFunc) Type() (*types.Type, error) {
	if len(obj.Values) == 1 {
		// speculative, type is known statically
		if typ := obj.Values[0].Type(); !typ.HasVariant() && obj.typ == nil {
			return typ, nil
		}

		if obj.typ == nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
		}
		return obj.typ, nil

	} else if len(obj.Values) > 0 {
		// there's nothing we can do to speculate at this time
		if obj.typ == nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
		}
		return obj.typ, nil
	}

	if obj.Function != nil {
		if obj.function == nil {
			// TODO: should we return ErrTypeCurrentlyUnknown instead?
			panic("unexpected empty function")
			//return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
		}
		sig := obj.function.Info().Sig
		if sig != nil && !sig.HasVariant() && obj.typ == nil { // type is now known statically
			return sig, nil
		}

		if obj.typ == nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
		}
		return obj.typ, nil
	}

	var m = make(map[string]*types.Type)
	ord := []string{}
	var err error
	for i, arg := range obj.Args {
		if _, exists := m[arg.Name]; exists {
			err = fmt.Errorf("func arg index `%d` already exists", i)
			break
		}
		if arg.Type == nil {
			err = fmt.Errorf("func arg type `%s` at index `%d` is unknown", arg.Name, i)
			break
		}
		m[arg.Name] = arg.Type
		ord = append(ord, arg.Name)
	}

	rtyp, e := obj.Body.Type()
	if e != nil {
		// TODO: do we want to include this speculative snippet below?
		// function return type cannot be determined...
		if obj.Return == nil {
			e := errwrap.Wrapf(e, "body/return type is unknown")
			err = errwrap.Append(err, e)
		} else {
			// probably unnecessary except for speculative execution
			// because there is an invariant to determine this type!
			rtyp = obj.Return // bonus, happens to be known
		}
	}

	if err == nil && obj.typ == nil { // type is now known statically
		return &types.Type{
			Kind: types.KindFunc,
			Map:  m,
			Ord:  ord,
			Out:  rtyp,
		}, nil
	}

	if obj.typ == nil {
		if err != nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, err.Error())
		}
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprFunc) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	if i, j := len(obj.Args), len(obj.params); i != j {
		// programming error?
		if obj.Title == "" {
			return nil, nil, fmt.Errorf("func args and params mismatch %d != %d", i, j)
		}
		return nil, nil, fmt.Errorf("func `%s` args and params mismatch %d != %d", obj.Title, i, j)
	}

	m := make(map[string]*types.Type)
	ord := []string{}
	var out *types.Type

	// This obj.Args stuff is only used for the obj.Body lambda case.
	for i, arg := range obj.Args {
		typArg := arg.Type // maybe it's nil
		if arg.Type == nil {
			typArg = &types.Type{
				Kind: types.KindUnification,
				Uni:  types.NewElem(), // unification variable, eg: ?1
			}
		}

		invars, err := obj.params[i].Check(typArg)
		if err != nil {
			return nil, nil, err
		}
		invariants = append(invariants, invars...)

		m[arg.Name] = typArg
		ord = append(ord, arg.Name)
	}

	out = obj.Return
	if obj.Return == nil {
		out = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}

	if obj.Body != nil {
		invars, err := obj.Body.Check(out) // typ of the func body
		if err != nil {
			return nil, nil, err
		}
		invariants = append(invariants, invars...)
	}

	typExpr := &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  out,
	}

	if obj.Function != nil {
		// Don't call obj.function.(interfaces.InferableFunc).Infer here
		// because we wouldn't have information about how we call it
		// anyways. This happens in ExprCall instead. We only need to
		// ensure this ExprFunc returns a valid unification variable.
		typExpr = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}
	}

	//if len(obj.Values) > 0
	for _, fn := range obj.Values {
		_ = fn
		panic("not implemented") // XXX: not implemented!
	}

	// Every infer call must have this section, because expr var needs this.
	typType := typExpr
	//if obj.typ == nil { // optional says sam
	//	obj.typ = typExpr // sam says we could unconditionally do this
	//}
	if obj.typ != nil {
		typType = obj.typ
	}
	// This must be added even if redundant, so that we collect the obj ptr.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typExpr, // This is the type that we return.
		Actual: typType,
	}
	invariants = append(invariants, invar)

	return typExpr, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprFunc) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it.
func (obj *ExprFunc) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	// This implementation produces a graph with a single node of in-degree
	// zero which outputs a single FuncValue. The FuncValue is a closure, in
	// that it holds both a lambda body and a captured environment. This
	// environment, which we receive from the caller, gives information
	// about the variables declared _outside_ of the lambda, at the time the
	// lambda is returned.
	//
	// Each time the FuncValue is called, it produces a separate graph, the
	// subgraph which computes the lambda's output value from the lambda's
	// argument values. The nodes created for that subgraph have a shorter
	// life span than the nodes in the captured environment.

	//graph, err := pgraph.NewGraph("func")
	//if err != nil {
	//	return nil, nil, err
	//}
	//function, err := obj.Func()
	//if err != nil {
	//	return nil, nil, err
	//}
	//graph.AddVertex(function)

	var funcValueFunc interfaces.Func
	if obj.Body != nil {
		f := func(ctx context.Context, args []types.Value) (types.Value, error) {
			// XXX: Find a way to exercise this function if possible.
			//return nil, funcs.ErrCantSpeculate
			return nil, fmt.Errorf("not implemented")
		}
		v := func(innerTxn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
			// Extend the environment with the arguments.
			extendedEnv := env.Copy() // TODO: Should we copy?
			for i := range obj.Args {
				if args[i] == nil {
					return nil, fmt.Errorf("programming error")
				}
				param := obj.params[i]
				//extendedEnv.Variables[arg.Name] = args[i]
				//extendedEnv.Variables[param.envKey] = args[i]
				extendedEnv.Variables[param.envKey] = &interfaces.FuncSingleton{
					MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
						f := args[i]
						g, err := pgraph.NewGraph("g")
						if err != nil {
							return nil, nil, err
						}
						g.AddVertex(f)
						return g, f, nil
					},
				}

			}

			// Create a subgraph from the lambda's body, instantiating the
			// lambda's parameters with the args and the other variables
			// with the nodes in the captured environment.
			subgraph, bodyFunc, err := obj.Body.Graph(extendedEnv)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not create the lambda body's subgraph")
			}

			innerTxn.AddGraph(subgraph)

			return bodyFunc, nil
		}
		funcValueFunc = structs.FuncValueToConstFunc(&full.FuncValue{
			V: v,
			F: f,
			T: obj.typ,
		})
	} else if obj.Function != nil {
		// Build this "callable" version in case it's available and we
		// can use that directly. We don't need to copy it because we
		// expect anything that is Callable to be stateless, and so it
		// can use the same function call for every instantiation of it.
		var f interfaces.FuncSig
		callableFunc, ok := obj.function.(interfaces.CallableFunc)
		if ok {
			// XXX: this might be dead code, how do we exercise it?
			// If the function is callable then the surrounding
			// ExprCall will produce a graph containing this func
			// instead of calling ExprFunc.Graph().
			f = callableFunc.Call
		}
		v := func(txn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
			// Copy obj.function so that the underlying ExprFunc.function gets
			// refreshed with a new ExprFunc.Function() call. Otherwise, multiple
			// calls to this function will share the same Func.
			exprCopy, err := obj.Copy()
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not copy expression")
			}
			funcExprCopy, ok := exprCopy.(*ExprFunc)
			if !ok {
				// programming error
				return nil, errwrap.Wrapf(err, "ExprFunc.Copy() does not produce an ExprFunc")
			}
			valueTransformingFunc := funcExprCopy.function
			txn.AddVertex(valueTransformingFunc)
			for i, arg := range args {
				argName := obj.typ.Ord[i]
				txn.AddEdge(arg, valueTransformingFunc, &interfaces.FuncEdge{
					Args: []string{argName},
				})
			}
			return valueTransformingFunc, nil
		}

		// obj.function is a node which transforms input values into
		// an output value, but we need to construct a node which takes no
		// inputs and produces a FuncValue, so we need to wrap it.
		funcValueFunc = structs.FuncValueToConstFunc(&full.FuncValue{
			V: v,
			F: f,
			T: obj.typ,
		})
	} else /* len(obj.Values) > 0 */ {
		index, err := langUtil.FnMatch(obj.typ, obj.Values)
		if err != nil {
			// programming error
			// since type checking succeeded at this point, there should only be one match
			return nil, nil, errwrap.Wrapf(err, "multiple matches found")
		}
		simpleFn := obj.Values[index]
		simpleFn.T = obj.typ

		funcValueFunc = structs.SimpleFnToConstFunc(fmt.Sprintf("title: %s", obj.Title), simpleFn)
	}

	outerGraph, err := pgraph.NewGraph("ExprFunc")
	if err != nil {
		return nil, nil, err
	}
	outerGraph.AddVertex(funcValueFunc)
	return outerGraph, funcValueFunc, nil
}

// SetValue for a func expression is always populated statically, and does not
// ever receive any incoming values (no incoming edges) so this should never be
// called. It has been implemented for uniformity.
func (obj *ExprFunc) SetValue(value types.Value) error {
	// We don't need to do anything because no resource has a function field and
	// so nobody is going to call Value().

	//if err := obj.typ.Cmp(value.Type()); err != nil {
	//	return err
	//}
	//// FIXME: is this part necessary?
	//funcValue, worked := value.(*full.FuncValue)
	//if !worked {
	//	return fmt.Errorf("expected a FuncValue")
	//}
	//obj.V = funcValue.V
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This particular value is always known since it is a constant.
func (obj *ExprFunc) Value() (types.Value, error) {
	// Don't panic because we call Value speculatively for partial values!
	//return nil, fmt.Errorf("error: ExprFunc does not store its latest value because resources don't yet have function fields")

	if obj.Body != nil {
		// We can only return a Value if we know the value of all the
		// ExprParams. We don't have an environment, so this is only
		// possible if there are no ExprParams at all.
		// XXX: If we add in EnvValue as an arg, can we change this up?
		if err := checkParamScope(obj, make(map[interfaces.Expr]struct{})); err != nil {
			// return the sentinel value
			return nil, funcs.ErrCantSpeculate
		}

		f := func(ctx context.Context, args []types.Value) (types.Value, error) {
			// TODO: make TestAstFunc1/shape8.txtar better...
			//extendedValueEnv := interfaces.EmptyValueEnv() // TODO: add me?
			//for _, x := range obj.Args {
			//	extendedValueEnv[???] = ???
			//}

			// XXX: Find a way to exercise this function if possible.
			// chained-returned-funcs.txtar will error if we use:
			//return nil, fmt.Errorf("not implemented")
			return nil, funcs.ErrCantSpeculate
		}
		v := func(innerTxn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
			// There are no ExprParams, so we start with the empty environment.
			// Extend that environment with the arguments.
			extendedEnv := interfaces.EmptyEnv()
			//extendedEnv := make(map[string]interfaces.Func)
			for i := range obj.Args {
				if args[i] == nil {
					// XXX: speculation error?
					return nil, fmt.Errorf("programming error?")
				}
				if len(obj.params) <= i {
					// XXX: speculation error?
					return nil, fmt.Errorf("programming error?")
				}
				param := obj.params[i]
				if param == nil || param.envKey == nil {
					// XXX: speculation error?
					return nil, fmt.Errorf("programming error?")
				}

				extendedEnv.Variables[param.envKey] = &interfaces.FuncSingleton{
					MakeFunc: func() (*pgraph.Graph, interfaces.Func, error) {
						f := args[i]
						g, err := pgraph.NewGraph("g")
						if err != nil {
							return nil, nil, err
						}
						g.AddVertex(f)
						return g, f, nil
					},
				}
			}

			// Create a subgraph from the lambda's body, instantiating the
			// lambda's parameters with the args and the other variables
			// with the nodes in the captured environment.
			subgraph, bodyFunc, err := obj.Body.Graph(extendedEnv)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not create the lambda body's subgraph")
			}

			innerTxn.AddGraph(subgraph)

			return bodyFunc, nil
		}

		return &full.FuncValue{
			V: v,
			F: f,
			T: obj.typ,
		}, nil

	} else if obj.Function != nil {
		copyFunc := func() interfaces.Func {
			copyableFunc, isCopyableFunc := obj.function.(interfaces.CopyableFunc)
			if obj.function == nil || !isCopyableFunc {
				return obj.Function() // force re-build a new pointer here!
			}

			// is copyable!
			return copyableFunc.Copy()
		}

		// Instead of passing in the obj.function, we instead pass in a
		// builder function so that this can use that inside of the
		// *full.FuncValue implementation to make new functions when it
		// gets called. We'll need more than one so they're not the same
		// pointer!
		return structs.FuncToFullFuncValue(copyFunc, obj.typ), nil
	}
	// else if /* len(obj.Values) > 0 */

	// XXX: It's unclear if the below code in this function is correct or
	// even tested.

	// polymorphic case: figure out which one has the correct type and wrap
	// it in a full.FuncValue.

	index, err := langUtil.FnMatch(obj.typ, obj.Values)
	if err != nil {
		// programming error
		// since type checking succeeded at this point, there should only be one match
		return nil, errwrap.Wrapf(err, "multiple matches found")
	}

	simpleFn := obj.Values[index] // *types.FuncValue
	simpleFn.T = obj.typ          // ensure the precise type is set/known

	return &full.FuncValue{
		V: nil,        // XXX: do we need to implement this too?
		F: simpleFn.V, // XXX: is this correct?
		T: obj.typ,
	}, nil
}

// ExprCall is a representation of a function call. This does not represent the
// declaration or implementation of a new function value. This struct has an
// analogous symmetry with ExprVar.
type ExprCall struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	expr interfaces.Expr // copy of what we're calling
	orig *ExprCall       // original pointer to this

	V types.Value // stored result (set with SetValue)

	// Name of the function to be called. We look for it in the scope.
	Name string

	// Args are the list of inputs to this function.
	Args []interfaces.Expr // list of args in parsed order

	// Var specifies whether the function being called is a lambda in a var.
	Var bool

	// Anon is an *ExprFunc which is used if we are calling anonymously. If
	// this is specified, Name must be the empty string.
	Anon interfaces.Expr
}

// String returns a short representation of this expression.
func (obj *ExprCall) String() string {
	var s []string
	for _, x := range obj.Args {
		s = append(s, fmt.Sprintf("%s", x.String()))
	}
	name := obj.Name
	if obj.Name == "" && obj.Anon != nil {
		name = "<anon>"
	}
	return fmt.Sprintf("call:%s(%s)", name, strings.Join(s, ", "))
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprCall) Apply(fn func(interfaces.Node) error) error {
	for _, x := range obj.Args {
		if err := x.Apply(fn); err != nil {
			return err
		}
	}
	if obj.Anon != nil {
		if err := obj.Anon.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprCall) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if obj.Name == "" && obj.Anon == nil {
		return fmt.Errorf("missing call name")
	}

	for _, x := range obj.Args {
		if err := x.Init(data); err != nil {
			return err
		}
	}
	if obj.Anon != nil {
		if err := obj.Anon.Init(data); err != nil {
			return err
		}
	}
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprCall) Interpolate() (interfaces.Expr, error) {
	args := []interfaces.Expr{}
	for _, x := range obj.Args {
		interpolated, err := x.Interpolate()
		if err != nil {
			return nil, err
		}
		args = append(args, interpolated)
	}
	var anon interfaces.Expr
	if obj.Anon != nil {
		f, err := obj.Anon.Interpolate()
		if err != nil {
			return nil, err
		}
		anon = f
	}

	orig := obj
	if obj.orig != nil { // preserve the original pointer (the identifier!)
		orig = obj.orig
	}

	return &ExprCall{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		// XXX: Copy copies this, do we want to here as well? (or maybe
		// we want to do it here, but not in Copy?)
		expr: obj.expr,
		orig: orig,
		V:    obj.V,
		Name: obj.Name,
		Args: args,
		Var:  obj.Var,
		Anon: anon,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprCall) Copy() (interfaces.Expr, error) {
	copied := false
	copiedArgs := false
	args := []interfaces.Expr{}
	for _, x := range obj.Args {
		cp, err := x.Copy()
		if err != nil {
			return nil, err
		}
		if cp != x { // must have been copied, or pointer would be same
			copiedArgs = true
		}
		args = append(args, cp)
	}
	if copiedArgs {
		copied = true
	} else {
		args = obj.Args // don't re-package it unnecessarily!
	}

	var anon interfaces.Expr
	if obj.Anon != nil {
		cp, err := obj.Anon.Copy()
		if err != nil {
			return nil, err
		}
		if cp != obj.Anon { // must have been copied, or pointer would be same
			copied = true
		}
		anon = cp
	}

	var err error
	var expr interfaces.Expr
	if obj.expr != nil {
		expr, err = obj.expr.Copy()
		if err != nil {
			return nil, err
		}
		if expr != obj.expr {
			copied = true
		}
	}

	// TODO: is this necessary? (I doubt it even gets used.)
	orig := obj
	if obj.orig != nil { // preserve the original pointer (the identifier!)
		orig = obj.orig
		copied = true // TODO: is this what we want?
	}

	// FIXME: do we want to allow a static ExprCall ?
	if !copied { // it's static
		return obj, nil
	}
	return &ExprCall{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		expr:     expr, // it seems that we need to copy this for it to work
		orig:     orig,
		V:        obj.V,
		Name:     obj.Name,
		Args:     args,
		Var:      obj.Var,
		Anon:     anon,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprCall) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	if obj.Name == "" && obj.Anon == nil {
		return nil, nil, fmt.Errorf("missing call name")
	}
	uid := funcOrderingPrefix + obj.Name // ordering id
	if obj.Var {                         // lambda
		uid = varOrderingPrefix + obj.Name // ordering id
	}

	node, exists := produces[uid]
	if exists {
		edge := &pgraph.SimpleEdge{Name: "exprcallname1"}
		graph.AddEdge(node, obj, edge) // prod -> cons
	}

	// equivalent to: strings.Contains(obj.Name, interfaces.ModuleSep)
	if split := strings.Split(obj.Name, interfaces.ModuleSep); len(split) > 1 {
		// we contain a dot
		uid = scopedOrderingPrefix + split[0] // just the first prefix

		// TODO: do we also want this second edge??
		node, exists := produces[uid]
		if exists {
			edge := &pgraph.SimpleEdge{Name: "exprcallname2"}
			graph.AddEdge(node, obj, edge) // prod -> cons
		}
	}
	// It's okay to replace the normal `func` or `var` prefix, because we
	// have the fancier `scoped:` prefix which matches more generally...

	// TODO: we _can_ produce two uid's here, is it okay we only offer one?
	cons := make(map[interfaces.Node]string)
	cons[obj] = uid

	for _, node := range obj.Args {
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraint...
		edge := &pgraph.SimpleEdge{Name: "exprcallargs1"}
		graph.AddEdge(node, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprcallargs2"}
			graph.AddEdge(n, k, edge)
		}
	}

	if obj.Anon != nil {
		g, c, err := obj.Anon.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraints...
		edge := &pgraph.SimpleEdge{Name: "exprcallanon1"}
		graph.AddEdge(obj.Anon, obj, edge) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprcallanon2"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to. This particular function has been
// heavily optimized to work correctly with calling functions with the correct
// args. Edit cautiously and with extensive testing.
func (obj *ExprCall) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope
	if obj.data.Debug {
		obj.data.Logf("call: %s(%t): scope: variables: %+v", obj.Name, obj.Var, obj.scope.Variables)
		obj.data.Logf("call: %s(%t): scope: functions: %+v", obj.Name, obj.Var, obj.scope.Functions)
	}

	// scope-check the arguments
	for _, x := range obj.Args {
		if err := x.SetScope(scope, sctx); err != nil {
			return err
		}
	}

	if obj.Anon != nil {
		if err := obj.Anon.SetScope(scope, sctx); err != nil {
			return err
		}
	}

	var prefixedName string
	var target interfaces.Expr
	if obj.Var {
		// The call looks like $f().
		prefixedName = interfaces.VarPrefix + obj.Name
		if f, exists := sctx[obj.Name]; exists {
			// $f refers to a parameter bound by an enclosing lambda definition.
			target = f
		} else {
			f, exists := obj.scope.Variables[obj.Name]
			if !exists {
				if obj.data.Debug || true { // TODO: leave this on permanently?
					lambdaScopeFeedback(obj.scope, obj.data.Logf)
				}
				err := fmt.Errorf("lambda `$%s` does not exist in this scope", prefixedName)
				return interfaces.HighlightHelper(obj, obj.data.Logf, err)
			}
			target = f
		}
	} else if obj.Name == "" && obj.Anon != nil {
		// The call looks like <anon>().

		target = obj.Anon
	} else {
		// The call looks like f().
		prefixedName = obj.Name
		f, exists := obj.scope.Functions[obj.Name]
		if !exists {
			if obj.data.Debug || true { // TODO: leave this on permanently?
				functionScopeFeedback(obj.scope, obj.data.Logf)
			}
			err := fmt.Errorf("func `%s` does not exist in this scope", prefixedName)
			return interfaces.HighlightHelper(obj, obj.data.Logf, err)
		}
		target = f
	}

	// NOTE: We previously used a findExprPoly helper function here.
	if polymorphicTarget, isExprPoly := target.(*ExprPoly); isExprPoly {
		// This function call refers to a polymorphic function
		// expression. Those expressions can be instantiated at
		// different types in different parts of the program, so that
		// the definition we found has a "polymorphic" type.
		//
		// This particular call is one of the parts of the program which
		// uses the polymorphic expression as a single, "monomorphic"
		// type. We make a copy of the definition, and later each copy
		// will be type-checked separately.
		monomorphicTarget, err := polymorphicTarget.Definition.Copy()
		if err != nil {
			return errwrap.Wrapf(err, "could not copy the function definition `%s`", prefixedName)
		}

		// This call now has the only reference to monomorphicTarget, so
		// it is our responsibility to scope-check it.
		if err := monomorphicTarget.SetScope(scope, sctx); err != nil {
			return errwrap.Wrapf(err, "scope-checking the function definition `%s`", prefixedName)
		}

		if obj.data.Debug {
			obj.data.Logf("call $%s(): set scope: func pointer: %p (polymorphic) -> %p (copy)", prefixedName, &polymorphicTarget, &monomorphicTarget)
		}

		obj.expr = monomorphicTarget
	} else {
		// This call refers to a monomorphic expression which has
		// already been scope-checked, so we don't need to scope-check
		// it again.
		obj.expr = target
	}

	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error. Remember that
// for this function expression, the type is the *return type* of the function,
// not the full type of the function signature.
func (obj *ExprCall) SetType(typ *types.Type) error {
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set

	// XXX: Do we need to do something to obj.Anon ?

	return nil
}

// Type returns the type of this expression, which is the return type of the
// function call.
func (obj *ExprCall) Type() (*types.Type, error) {

	// XXX: If we have the function statically in obj.Anon, run this?

	if obj.expr == nil {
		// possible programming error
		return nil, fmt.Errorf("call doesn't contain an expr pointer yet")
	}

	// function specific code follows...
	exprFunc, isFn := obj.expr.(*ExprFunc)
	if !isFn {
		if obj.typ == nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
		}
		return obj.typ, nil
	}

	sig, err := exprFunc.Type()
	if err != nil {
		return nil, err
	}
	if typ := sig.Out; typ != nil && !typ.HasVariant() && obj.typ == nil {
		return typ, nil // speculate!
	}

	// speculate if a partial return type is known
	if exprFunc.Body != nil {
		if exprFunc.Return != nil && obj.typ == nil {
			return exprFunc.Return, nil
		}

		if typ, err := exprFunc.Body.Type(); err == nil && obj.typ == nil {
			return typ, nil
		}
	}

	if exprFunc.Function != nil {
		// is it buildable? (formerly statically polymorphic)
		_, isBuildable := exprFunc.function.(interfaces.BuildableFunc)
		if !isBuildable && obj.typ == nil {
			if info := exprFunc.function.Info(); info != nil {
				if sig := info.Sig; sig != nil {
					if typ := sig.Out; typ != nil && !typ.HasVariant() {
						return typ, nil // speculate!
					}
				}
			}
		}
		// TODO: we could also check if a truly polymorphic type has
		// consistent return values across all possibilities available
	}

	//if len(exprFunc.Values) > 0
	// check to see if we have a unique return type
	for _, fn := range exprFunc.Values {
		typ := fn.Type()
		if typ == nil || typ.Out == nil {
			continue // skip, not available yet
		}
		if obj.typ == nil {
			return typ, nil
		}
	}

	if obj.typ == nil {
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// getPartials is a helper function to aid in building partial types and values.
// Remember that it's not legal to run many of the normal methods like .String()
// on a partial type.
func (obj *ExprCall) getPartials(fn *ExprFunc) (*types.Type, []types.Value, error) {
	argGen := func(x int) (string, error) {
		// assume (incorrectly?) for now...
		return util.NumToAlpha(x), nil
	}
	if fn.Function != nil {
		namedArgsFn, ok := fn.function.(interfaces.NamedArgsFunc) // are the args named?
		if ok {
			argGen = namedArgsFn.ArgGen // func(int) string
		}
	}

	// build partial type and partial input values to aid in filtering...
	mapped := make(map[string]*types.Type)
	argNames := []string{}
	//partialValues := []types.Value{}
	partialValues := make([]types.Value, len(obj.Args))
	for i, arg := range obj.Args {
		name, err := argGen(i) // get the Nth arg name
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "error getting arg #%d for func `%s`", i, obj.Name)
		}
		if name == "" {
			// possible programming error
			return nil, nil, fmt.Errorf("can't get arg #%d for func `%s`", i, obj.Name)
		}
		//mapped[name] = nil // unknown type
		argNames = append(argNames, name)
		//partialValues = append(partialValues, nil) // placeholder value

		// optimization: if type/value is already known, specify it now!
		var err1, err2 error
		// NOTE: This _can_ return unification variables now. Is it ok?
		mapped[name], err1 = arg.Type()      // nil type on error
		partialValues[i], err2 = arg.Value() // nil value on error
		if err1 == nil && err2 == nil && mapped[name].Cmp(partialValues[i].Type()) != nil {
			// This can happen when we statically find an issue like
			// a printf scenario where it's wrong statically...
			t1 := mapped[name]
			t2 := partialValues[i].Type()
			return nil, nil, fmt.Errorf("type/value inconsistent at arg #%d for func `%s`: %v != %v", i, obj.Name, t1, t2)
		}
	}

	out, err := obj.Type() // do we know the return type yet?
	if err != nil {
		out = nil // just to make sure...
	}
	// partial type can have some type components that are nil!
	// this means they are not yet known at this time...
	partialType := &types.Type{
		Kind: types.KindFunc,
		Map:  mapped,
		Ord:  argNames,
		Out:  out, // possibly nil
	}

	return partialType, partialValues, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprCall) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	if obj.expr == nil {
		// possible programming error
		return nil, nil, fmt.Errorf("call doesn't contain an expr pointer yet")
	}

	invariants := []*interfaces.UnificationInvariant{}

	mapped := make(map[string]*types.Type)
	ordered := []string{}
	var typExpr *types.Type // out

	// Look at what kind of function we are calling...
	callee := trueCallee(obj.expr)
	exprFunc, isFn := callee.(*ExprFunc)

	argGen := func(x int) (string, error) {
		// assume (incorrectly?) for now...
		return util.NumToAlpha(x), nil
	}
	if isFn && exprFunc.Function != nil {
		namedArgsFn, ok := exprFunc.function.(interfaces.NamedArgsFunc) // are the args named?
		if ok {
			argGen = namedArgsFn.ArgGen // func(int) string
		}
	}

	for i, arg := range obj.Args { // []interfaces.Expr
		name, err := argGen(i) // get the Nth arg name
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "error getting arg name #%d for func `%s`", i, obj.Name)
		}
		if name == "" {
			// possible programming error
			return nil, nil, fmt.Errorf("can't get arg name #%d for func `%s`", i, obj.Name)
		}

		typ, invars, err := arg.Infer()
		if err != nil {
			return nil, nil, err
		}
		// Equivalent:
		//typ := &types.Type{
		//	Kind: types.KindUnification,
		//	Uni:  types.NewElem(), // unification variable, eg: ?1
		//}
		//invars, err := arg.Check(typ) // typ of the arg
		//if err != nil {
		//	return nil, nil, err
		//}
		invariants = append(invariants, invars...)

		mapped[name] = typ
		ordered = append(ordered, name)
	}

	typExpr = &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}

	typFunc := &types.Type{
		Kind: types.KindFunc,
		Map:  mapped,
		Ord:  ordered,
		Out:  typExpr,
	}

	// Every infer call must have this section, because expr var needs this.
	typType := typExpr
	//if obj.typ == nil { // optional says sam
	//	obj.typ = typExpr // sam says we could unconditionally do this
	//}
	if obj.typ != nil {
		typType = obj.typ
	}
	// This must be added even if redundant, so that we collect the obj ptr.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typExpr, // This is the type that we return.
		Actual: typType,
	}
	invariants = append(invariants, invar)

	// We run this Check for all cases. (So refactor it to here.)
	invars, err := obj.expr.Check(typFunc)
	if err != nil {
		return nil, nil, err
	}
	invariants = append(invariants, invars...)

	if !isFn {
		// legacy case (does this even happen?)
		return typExpr, invariants, nil
	}

	// Just get ExprFunc.Check to figure it out...
	//if exprFunc.Body != nil {}

	if exprFunc.Function != nil {
		var typFn *types.Type
		fn := exprFunc.function // instantiated copy of exprFunc.Function
		// is it inferable? (formerly statically polymorphic)
		inferableFn, ok := fn.(interfaces.InferableFunc)
		if info := fn.Info(); !ok && info != nil && info.Sig != nil {
			if info.Sig.HasVariant() { // XXX: legacy, remove me
				// XXX: Look up the obj.Title for obj.expr instead?
				return nil, nil, fmt.Errorf("func `%s` contains a variant: %s", obj.Name, info.Sig)
			}

			// It's important that we copy the type signature, since
			// it may otherwise get used in more than one place for
			// type unification when in fact there should be two or
			// more different solutions if it's polymorphic and used
			// more than once. We could be more careful when passing
			// this in here, but it's simple and safe to just always
			// do this copy. Sam prefers this approach.
			typFn = info.Sig.Copy()

		} else if ok {
			partialType, partialValues, err := obj.getPartials(exprFunc)
			if err != nil {
				return nil, nil, err
			}

			// We just run the Infer() method of the ExprFunc if it
			// happens to have one. Get the list of Invariants, and
			// return them directly.
			typ, invars, err := inferableFn.FuncInfer(partialType, partialValues)
			if err != nil {
				return nil, nil, errwrap.Wrapf(err, "func `%s` infer error", exprFunc.Title)
			}
			invariants = append(invariants, invars...)
			if typ == nil { // should get a sig, not a nil!
				// programming error
				return nil, nil, fmt.Errorf("func `%s` infer type was nil", exprFunc.Title)
			}

			// It's important that we copy the type signature here.
			// See the above comment which explains the reasoning.
			typFn = typ.Copy()

		} else {
			// programming error
			return nil, nil, fmt.Errorf("incorrectly built `%s` function", exprFunc.Title)
		}

		invar := &interfaces.UnificationInvariant{
			Node:   obj,
			Expr:   obj.expr, // this should NOT be obj
			Expect: typFunc,  // TODO: are these two reversed here?
			Actual: typFn,
		}
		invariants = append(invariants, invar)

		// TODO: Do we need to link obj.expr to exprFunc, eg:
		//invar2 := &interfaces.UnificationInvariant{
		//	Expr:   exprFunc, // trueCallee variant
		//	Expect: typFunc,
		//	Actual: typFn,
		//}
		//invariants = append(invariants, invar2)
	}

	// if len(exprFunc.Values) > 0
	for _, fn := range exprFunc.Values {
		_ = fn
		panic("not implemented") // XXX: not implemented!
	}

	return typExpr, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprCall) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it, and
// the edges from all of the child graphs to this.
func (obj *ExprCall) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	if obj.expr == nil {
		// possible programming error
		return nil, nil, fmt.Errorf("call doesn't contain an expr pointer yet")
	}

	graph, err := pgraph.NewGraph("call")
	if err != nil {
		return nil, nil, err
	}

	ftyp, err := obj.expr.Type()
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "could not get the type of the function")
	}

	// Loop over the arguments, add them to the graph, but do _not_ connect them
	// to the function vertex. Instead, each time the call vertex (which we
	// create below) receives a FuncValue from the function node, it creates the
	// corresponding subgraph and connects these arguments to it.
	var argFuncs []interfaces.Func
	for i, arg := range obj.Args {
		argGraph, argFunc, err := arg.Graph(env)
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "could not get graph for arg %d", i)
		}
		graph.AddGraph(argGraph)
		argFuncs = append(argFuncs, argFunc)
	}

	// Speculate early, in an attempt to get a simpler graph shape.
	//exprFunc, ok := obj.expr.(*ExprFunc)
	// XXX: Does this need to be .Pure for it to be allowed?
	//canSpeculate := !ok || exprFunc.function == nil || (exprFunc.function.Info().Fast && exprFunc.function.Info().Spec)
	canSpeculate := true // XXX: use the magic Info fields?
	exprValue, err := obj.expr.Value()
	exprFuncValue, ok := exprValue.(*full.FuncValue)
	if err == nil && ok && canSpeculate {
		txn := (&txn.GraphTxn{
			GraphAPI: (&txn.Graph{
				Debug: obj.data.Debug,
				Logf: func(format string, v ...interface{}) {
					obj.data.Logf(format, v...)
				},
			}).Init(),
			Lock:     func() {},
			Unlock:   func() {},
			RefCount: (&ref.Count{}).Init(),
		}).Init()
		txn.AddGraph(graph) // add all of the graphs so far...

		outputFunc, err := exprFuncValue.CallWithFuncs(txn, argFuncs)
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "could not construct the static graph for a function call")
		}
		txn.AddVertex(outputFunc)

		if err := txn.Commit(); err != nil { // Must Commit after txn.AddGraph(...)
			return nil, nil, err
		}

		return txn.Graph(), outputFunc, nil
	} else if err != nil && ok && canSpeculate && err != funcs.ErrCantSpeculate {
		// This is a permanent error, not a temporary speculation error.
		//return nil, nil, err // XXX: Consider adding this...
	}

	// Find the vertex which produces the FuncValue.
	g, funcValueFunc, err := obj.funcValueFunc(env)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g)

	// Add a vertex for the call itself.
	edgeName := structs.CallFuncArgNameFunction
	callFunc := &structs.CallFunc{
		Textarea: obj.Textarea,

		Type:        obj.typ,
		FuncType:    ftyp,
		EdgeName:    edgeName,
		ArgVertices: argFuncs,
	}
	graph.AddVertex(callFunc)
	graph.AddEdge(funcValueFunc, callFunc, &interfaces.FuncEdge{
		Args: []string{edgeName},
	})

	return graph, callFunc, nil
}

// funcValueFunc is a helper function to make the code more readable. This was
// some very hard logic to get right for each case, and it eventually simplifies
// down to two cases after refactoring.
// TODO: Maybe future refactoring can improve this even more!
func (obj *ExprCall) funcValueFunc(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	_, isParam := obj.expr.(*ExprParam)
	exprIterated, isIterated := obj.expr.(*ExprIterated)

	if !isIterated || obj.Var {
		// XXX: isIterated and !obj.Var -- seems to never happen
		//if (isParam || isIterated) && !obj.Var // better replacement?
		if isParam && !obj.Var {
			return nil, nil, fmt.Errorf("programming error")
		}
		// XXX: AFAICT we can *always* use the real env here. Ask Sam!
		useEnv := interfaces.EmptyEnv()
		if (isParam || isIterated) && obj.Var {
			useEnv = env
		}
		// If isParam, the function being called is a parameter from the
		// surrounding function. We should be able to find this
		// parameter in the environment.

		// If obj.Var, then the function being called is a top-level
		// definition. The parameters which are visible at this use site
		// must not be visible at the definition site, so we pass an
		// empty environment. Sam was confused about this but apparently
		// it works. He thinks that the reason it works must be in
		// ExprFunc where we must be capturing the Env somehow. See:
		// watsam1.mcl and watsam2.mcl for more examples.

		// If else, the function being called is a top-level definition.
		// (Which is NOT inside of a for loop.) The parameters which are
		// visible at this use site must not be visible at the
		// definition site, so we pass the captured environment. Sam is
		// VERY confused about this case.
		//
		//capturedEnv, exists := env.Functions[obj.Name]
		//if !exists {
		//	return nil, nil, fmt.Errorf("programming error with `%s`", obj.Name)
		//}
		//useEnv = capturedEnv
		// But then we decided to not use this env there after all...

		return obj.expr.Graph(useEnv)
	}

	// This is: isIterated && !obj.Var

	// The ExprPoly has been unwrapped to produce a fresh ExprIterated which
	// was stored in obj.expr therefore we don't want to look up obj.expr in
	// env.Functions because we would not find this fresh copy of the
	// ExprIterated. Instead we recover the ExprPoly and look up that
	// ExprPoly in env.Functions.
	expr, exists := obj.scope.Functions[obj.Name]
	if !exists {
		// XXX: Improve this error message.
		return nil, nil, fmt.Errorf("unspecified error with: %s", obj.Name)
	}

	exprPoly, ok := expr.(*ExprPoly)
	if !ok {
		// XXX: Improve this error message.
		return nil, nil, fmt.Errorf("unspecified error with: %s", obj.Name)
	}

	// The function being called is a top-level definition inside a for
	// loop. The parameters which are visible at this use site must not be
	// visible at the definition site, so we pass the captured environment.
	// Sam is not confused ANYMORE about this case.
	capturedEnv, exists := env.Functions[exprPoly]
	if !exists {
		// XXX: Improve this error message.
		return nil, nil, fmt.Errorf("unspecified error with: %s", obj.Name)
	}

	g, f, err := exprIterated.Definition.Graph(capturedEnv)
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "could not get the graph for the expr pointer")
	}

	return g, f, nil

	// NOTE: If `isIterated && obj.Var` is now handled in the above "else"!
}

// SetValue here is used to store the result of the last computation of this
// expression node after it has received all the required input values. This
// value is cached and can be retrieved by calling Value.
func (obj *ExprCall) SetValue(value types.Value) error {
	if err := obj.typ.Cmp(value.Type()); err != nil {
		return err
	}
	obj.V = value // XXX: is this useful or a good idea?
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// It is often unlikely that this kind of speculative execution finds something.
// This particular implementation will run a function if all of the needed
// values are known. This is necessary for getting the efficient graph shape of
// ExprCall.
func (obj *ExprCall) Value() (types.Value, error) {
	if obj.V != nil { // XXX: is this useful or a good idea?
		return obj.V, nil
	}

	if obj.expr == nil {
		return nil, fmt.Errorf("func value does not yet exist")
	}

	// Speculatively call Value() on obj.expr and each arg.
	// XXX: Should we check obj.expr.(*ExprFunc).Info.Pure here ?
	value, err := obj.expr.Value() // speculative
	if err != nil {
		return nil, err
	}

	funcValue, ok := value.(*full.FuncValue)
	if !ok {
		return nil, fmt.Errorf("not a func value")
	}

	args := []types.Value{}
	for _, arg := range obj.Args { // []interfaces.Expr
		a, err := arg.Value() // speculative
		if err != nil {
			return nil, err
		}
		args = append(args, a)
	}

	// We now have a *full.FuncValue and a []types.Value. We can't call the
	// existing:
	//	Call(..., []interfaces.Func) interfaces.Func` method on the
	// FuncValue, we need a speculative:
	//	Call(..., []types.Value) types.Value
	// method.
	return funcValue.CallWithValues(context.TODO(), args)
}

// ExprVar is a representation of a variable lookup. It returns the expression
// that that variable refers to.
type ExprVar struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	Name string // name of the variable
}

// String returns a short representation of this expression.
func (obj *ExprVar) String() string { return fmt.Sprintf("var(%s)", obj.Name) }

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprVar) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprVar) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	return langUtil.ValidateVarName(obj.Name)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
// Here it returns itself, since variable names cannot be interpolated. We don't
// support variable, variables or anything crazy like that.
func (obj *ExprVar) Interpolate() (interfaces.Expr, error) {
	return &ExprVar{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Name:     obj.Name,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
// This intentionally returns a copy, because if a function (usually a lambda)
// that is used more than once, contains this variable, we will want each
// instantiation of it to be unique, otherwise they will be the same pointer,
// and they won't be able to have different values.
func (obj *ExprVar) Copy() (interfaces.Expr, error) {
	return &ExprVar{
		Textarea: obj.Textarea,
		data:     obj.data,
		scope:    obj.scope,
		typ:      obj.typ,
		Name:     obj.Name,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprVar) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	if obj.Name == "" {
		return nil, nil, fmt.Errorf("missing var name")
	}
	uid := varOrderingPrefix + obj.Name // ordering id

	node, exists := produces[uid]
	if exists {
		edge := &pgraph.SimpleEdge{Name: "exprvar1"}
		graph.AddEdge(node, obj, edge) // prod -> cons
	}

	// equivalent to: strings.Contains(obj.Name, interfaces.ModuleSep)
	if split := strings.Split(obj.Name, interfaces.ModuleSep); len(split) > 1 {
		// we contain a dot
		uid = scopedOrderingPrefix + split[0] // just the first prefix

		// TODO: do we also want this second edge??
		node, exists := produces[uid]
		if exists {
			edge := &pgraph.SimpleEdge{Name: "exprvar2"}
			graph.AddEdge(node, obj, edge) // prod -> cons
		}
	}
	// It's okay to replace the normal `var` prefix, because we have the
	// fancier `scoped:` prefix which matches more generally...

	// TODO: we _can_ produce two uid's here, is it okay we only offer one?
	cons := make(map[interfaces.Node]string)
	cons[obj] = uid

	return graph, cons, nil
}

// SetScope stores the scope for use in this resource.
func (obj *ExprVar) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	obj.scope = interfaces.EmptyScope()
	if scope != nil {
		obj.scope = scope.Copy() // XXX: Sam says we probably don't need to copy this.
	}

	if monomorphicTarget, exists := sctx[obj.Name]; exists {
		// This ExprVar refers to a parameter bound by an enclosing
		// lambda definition.
		obj.scope.Variables[obj.Name] = monomorphicTarget

		// There is no need to scope-check the target, it's just a
		// an ExprParam with no internal references.
		return nil
	}

	target, exists := obj.scope.Variables[obj.Name]
	if !exists {
		if obj.data.Debug || true { // TODO: leave this on permanently?
			variableScopeFeedback(obj.scope, obj.data.Logf)
		}
		err := fmt.Errorf("var `$%s` does not exist in this scope", obj.Name)
		return interfaces.HighlightHelper(obj, obj.data.Logf, err)
	}

	obj.scope.Variables[obj.Name] = target

	// This ExprVar refers to a top-level definition which has already been
	// scope-checked, so we don't need to scope-check it again.
	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprVar) SetType(typ *types.Type) error {
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprVar) Type() (*types.Type, error) {
	// TODO: should this look more like Type() in ExprCall or vice-versa?

	if obj.scope == nil { // avoid a possible nil panic if we speculate here
		if obj.typ == nil {
			return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
		}
		return obj.typ, nil
	}

	// Return the type if it is already known statically... It is useful for
	// type unification to have some extra info early.
	expr, exists := obj.scope.Variables[obj.Name]
	// If !exists, just ignore the error for now since this is speculation!
	// This logic simplifies down to just this!
	if exists && obj.typ == nil {
		return expr.Type()
	}

	if obj.typ == nil {
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer. This Infer is an exception to that pattern.
func (obj *ExprVar) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	// lookup value from scope
	expr, exists := obj.scope.Variables[obj.Name]
	if !exists {
		err := fmt.Errorf("var `%s` does not exist in this scope", obj.Name)
		return nil, nil, interfaces.HighlightHelper(obj, obj.data.Logf, err)
	}

	// This child call to Infer is an outlier to the common pattern where
	// "Infer does not call Infer". We really want the indirection here.

	typ, invariants, err := expr.Infer() // this is usually a top level expr
	if err != nil {
		return nil, nil, err
	}

	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typ,
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return typ, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprVar) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This returns a graph with a single vertex (itself) in it, and
// the edges from all of the child graphs to this. The child graph in this case
// is the graph which is obtained from the bound expression. The edge points
// from that expression to this vertex. The function used for this vertex is a
// simple placeholder which sucks incoming values in and passes them on. This is
// important for filling the logical requirements of the graph type checker, and
// to avoid duplicating production of the incoming input value from the bound
// expression.
func (obj *ExprVar) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	// New "env" based methodology. One day we will use this for everything,
	// and not use the scope in the same way. Sam hacking!
	//targetFunc, exists := env.Variables[obj.Name]
	//if exists {
	//	if targetFunc == nil {
	//		panic("BUG")
	//	}
	//	graph, err := pgraph.NewGraph("ExprVar")
	//	if err != nil {
	//		return nil, nil, err
	//	}
	//	graph.AddVertex(targetFunc)
	//	return graph, targetFunc, nil
	//}

	// Leave this remainder here for now...

	// Delegate to the targetExpr.
	targetExpr, exists := obj.scope.Variables[obj.Name]
	if !exists {
		return nil, nil, fmt.Errorf("scope is missing %s", obj.Name)
	}

	// The variable points to a top-level expression.
	return targetExpr.Graph(env)
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the dest lookup expr) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprVar) SetValue(value types.Value) error {
	if err := obj.typ.Cmp(value.Type()); err != nil {
		return err
	}
	// noop!
	//obj.V = value
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This returns the value this variable points to. It is able to do so because
// it can lookup in the previous set scope which expression this points to, and
// then it can call Value on that expression.
func (obj *ExprVar) Value() (types.Value, error) {
	if obj.scope == nil { // avoid a possible nil panic if we speculate here
		return nil, errwrap.Wrapf(interfaces.ErrValueCurrentlyUnknown, obj.String())
	}

	expr, exists := obj.scope.Variables[obj.Name]
	if !exists {
		return nil, fmt.Errorf("var `%s` does not exist in scope", obj.Name)
	}
	return expr.Value() // recurse
}

// ExprParam represents a parameter to a function.
type ExprParam struct {
	typ *types.Type

	Name string // name of the parameter

	envKey interfaces.Expr
}

// String returns a short representation of this expression.
func (obj *ExprParam) String() string {
	return fmt.Sprintf("param(%s)", obj.Name)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprParam) Apply(fn func(interfaces.Node) error) error { return fn(obj) }

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprParam) Init(*interfaces.Data) error {
	return langUtil.ValidateVarName(obj.Name)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprParam) Interpolate() (interfaces.Expr, error) {
	expr := &ExprParam{
		typ:  obj.typ,
		Name: obj.Name,
	}
	expr.envKey = expr
	return expr, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
// This intentionally returns a copy, because if a function (usually a lambda)
// that is used more than once, contains this variable, we will want each
// instantiation of it to be unique, otherwise they will be the same pointer,
// and they won't be able to have different values.
func (obj *ExprParam) Copy() (interfaces.Expr, error) {
	return &ExprParam{
		typ:    obj.typ,
		Name:   obj.Name,
		envKey: obj.envKey, // don't copy
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprParam) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	if obj.Name == "" {
		return nil, nil, fmt.Errorf("missing param name")
	}
	uid := paramOrderingPrefix + obj.Name // ordering id

	cons := make(map[interfaces.Node]string)
	cons[obj] = uid

	node, exists := produces[uid]
	if exists {
		edge := &pgraph.SimpleEdge{Name: "exprparam"}
		graph.AddEdge(node, obj, edge) // prod -> cons
	}

	return graph, cons, nil
}

// SetScope stores the scope for use in this resource.
func (obj *ExprParam) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	obj.envKey = obj // XXX: not being used, we use newExprParam for now
	// ExprParam doesn't have a scope, because it is the node to which a
	// ExprVar can point to, so it doesn't point to anything itself.
	return nil
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprParam) SetType(typ *types.Type) error {
	if obj.typ != nil {
		if obj.typ.Cmp(typ) == nil { // if not set, ensure it doesn't change
			return nil
		}

		// Redundant: just as expensive as running UnifyCmp below and it
		// would fail in that case since we did the above Cmp anyways...
		//if !obj.typ.HasUni() {
		//	return err // err from above obj.Typ
		//}

		// Here, obj.typ might be a unification variable, so if we're
		// setting it to overwrite it, we need to at least make sure
		// that it's compatible.
		if err := unificationUtil.UnifyCmp(obj.typ, typ); err != nil {
			return err
		}
		//obj.typ = typ // fallthrough below and set
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprParam) Type() (*types.Type, error) {
	// Return the type if it is already known statically... It is useful for
	// type unification to have some extra info early.
	if obj.typ == nil {
		return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
	}
	return obj.typ, nil
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer. This Infer returns a quasi-equivalent to my
// ExprAny invariant idea.
func (obj *ExprParam) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	// We know this has to be something, but we don't know what. Return
	// anything, just like my ExprAny invariant would have.
	typ := obj.typ
	if obj.typ == nil { // XXX: is this correct?
		typ = &types.Type{
			Kind: types.KindUnification,
			Uni:  types.NewElem(), // unification variable, eg: ?1
		}

		// XXX: Every time we call ExprParam.Infer it is generating a
		// new unification variable... So we want ?1 the first time, ?2
		// the second... but we never get ?1 solved... SO we want to
		// cache this so it only happens once I think.
		obj.typ = typ // cache for now

		// This adds the obj ptr, so it's seen as an expr that we need to solve.
		invar := &interfaces.UnificationInvariant{
			Node:   obj,
			Expr:   obj,
			Expect: typ,
			Actual: typ,
		}
		invariants = append(invariants, invar)
	}

	return typ, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprParam) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might.
func (obj *ExprParam) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	fSingleton, exists := env.Variables[obj.envKey]
	if !exists {
		return nil, nil, fmt.Errorf("could not find `%s` in env for ExprParam", obj.Name)
	}

	return fSingleton.GraphFunc()
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the dest lookup expr) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprParam) SetValue(value types.Value) error {
	// ignored, as we don't support ExprParam.Value()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprParam) Value() (types.Value, error) {
	// XXX: if value env is an arg in Expr.Value(...)
	//value, exists := valueEnv[obj]
	//if exists {
	//	return value, nil
	//}
	return nil, fmt.Errorf("no value for ExprParam")
}

// ExprIterated tags an Expr to indicate that we want to use the env instead of
// the scope in Graph() because this variable was defined inside a for loop. We
// create a new ExprIterated which wraps an Expr and indicates that we want to
// use an iteration-specific value instead of the wrapped Expr. It delegates to
// the Expr for SetScope() and Infer(), and looks up in the env for Graph(), the
// same as ExprParam.Graph(), and panics if any later phase is called.
type ExprIterated struct {
	// Name is the name (Ident) of the StmtBind.
	Name string

	// Definition is the wrapped expression.
	Definition interfaces.Expr

	envKey interfaces.Expr
}

// String returns a short representation of this expression.
func (obj *ExprIterated) String() string {
	return fmt.Sprintf("iterated(%v %s)", obj.Definition, obj.Name)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprIterated) Apply(fn func(interfaces.Node) error) error {
	if obj.Definition != nil {
		if err := obj.Definition.Apply(fn); err != nil {
			return err
		}
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprIterated) Init(*interfaces.Data) error {
	if obj.Name == "" {
		return fmt.Errorf("empty name for ExprIterated")
	}
	if obj.Definition == nil {
		return fmt.Errorf("empty Definition for ExprIterated")
	}
	return nil
	//return langUtil.ValidateVarName(obj.Name) XXX: Should we add this?
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprIterated) Interpolate() (interfaces.Expr, error) {
	expr := &ExprIterated{
		Name:       obj.Name,
		Definition: obj.Definition,
		// TODO: Should we copy envKey ?
	}
	expr.envKey = expr
	return expr, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
// This intentionally returns a copy, because if a function (usually a lambda)
// that is used more than once, contains this variable, we will want each
// instantiation of it to be unique, otherwise they will be the same pointer,
// and they won't be able to have different values.
func (obj *ExprIterated) Copy() (interfaces.Expr, error) {
	return &ExprIterated{
		Name:       obj.Name,
		Definition: obj.Definition, // XXX: Should we copy this?
		envKey:     obj.envKey,     // don't copy
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprIterated) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	return obj.Definition.Ordering(produces) // Also do this.
	// XXX: Do we need to add our own graph too? Sam says yes, maybe.
	//
	//graph, err := pgraph.NewGraph("ordering")
	//if err != nil {
	//	return nil, nil, err
	//}
	//graph.AddVertex(obj)
	//
	//if obj.Name == "" {
	//	return nil, nil, fmt.Errorf("missing param name")
	//}
	//uid := paramOrderingPrefix + obj.Name // ordering id
	//
	//cons := make(map[interfaces.Node]string)
	//cons[obj] = uid
	//
	//node, exists := produces[uid]
	//if exists {
	//	edge := &pgraph.SimpleEdge{Name: "ExprIterated"}
	//	graph.AddEdge(node, obj, edge) // prod -> cons
	//}
	//
	//return graph, cons, nil
}

// SetScope stores the scope for use in this resource.
func (obj *ExprIterated) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	// When we copy, the pointer of the obj changes, so we save it here now,
	// so that we can use it later in the env lookup!
	obj.envKey = obj // XXX: not being used we use newExprIterated for now
	return obj.Definition.SetScope(scope, sctx)
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprIterated) SetType(typ *types.Type) error {
	return obj.Definition.SetType(typ)
}

// Type returns the type of this expression.
func (obj *ExprIterated) Type() (*types.Type, error) {
	return obj.Definition.Type()
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer. This Infer returns a quasi-equivalent to my
// ExprAny invariant idea.
func (obj *ExprIterated) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	typ, invariants, err := obj.Definition.Infer()
	if err != nil {
		return nil, nil, err
	}

	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	invar := &interfaces.UnificationInvariant{
		Expr:   obj,
		Expect: typ,
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return typ, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprIterated) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might.
func (obj *ExprIterated) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	fSingleton, exists := env.Variables[obj.envKey]
	if !exists {
		return nil, nil, fmt.Errorf("could not find `%s` in env for ExprIterated", obj.Name)
	}

	return fSingleton.GraphFunc()
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the dest lookup expr) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprIterated) SetValue(value types.Value) error {
	// ignored, as we don't support ExprIterated.Value()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprIterated) Value() (types.Value, error) {
	return nil, fmt.Errorf("no value for ExprIterated")
}

// ExprPoly is a polymorphic expression that is a definition that can be used in
// multiple places with different types. We must copy the definition at each
// call site in order for the type checker to find a different type at each call
// site. We create this copy inside SetScope, at which point we also recursively
// call SetScope on the copy. We must be careful to use the scope captured at
// the definition site, not the scope which is available at the call site.
type ExprPoly struct {
	Definition interfaces.Expr // The definition.
}

// String returns a short representation of this expression.
func (obj *ExprPoly) String() string {
	return fmt.Sprintf("polymorphic(%s)", obj.Definition.String())
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprPoly) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Definition.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprPoly) Init(data *interfaces.Data) error {
	return obj.Definition.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprPoly) Interpolate() (interfaces.Expr, error) {
	definition, err := obj.Definition.Interpolate()
	if err != nil {
		return nil, err
	}

	return &ExprPoly{
		Definition: definition,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
// This implementation intentionally does not copy anything, because the
// Definition is already intended to be copied at each use site.
func (obj *ExprPoly) Copy() (interfaces.Expr, error) {
	return obj, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprPoly) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	return obj.Definition.Ordering(produces)
}

// SetScope stores the scope for use in this resource.
func (obj *ExprPoly) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	panic("ExprPoly.SetScope(): should not happen, ExprVar should replace ExprPoly with a copy of its definition before calling SetScope")
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprPoly) SetType(typ *types.Type) error {
	panic("ExprPoly.SetType(): should not happen, all ExprPoly expressions should be gone by the time type-checking starts")
}

// Type returns the type of this expression.
func (obj *ExprPoly) Type() (*types.Type, error) {
	return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer. This Infer should never be called.
func (obj *ExprPoly) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	panic("ExprPoly.Infer(): should not happen, all ExprPoly expressions should be gone by the time type-checking starts")
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprPoly) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might.
func (obj *ExprPoly) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	panic("ExprPoly.Graph(): should not happen, all ExprPoly expressions should be gone by the time type-checking starts")
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the dest lookup expr) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprPoly) SetValue(value types.Value) error {
	// ignored, as we don't support ExprPoly.Value()
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprPoly) Value() (types.Value, error) {
	return nil, fmt.Errorf("no value for ExprPoly")
}

// ExprTopLevel is intended to wrap top-level definitions. It captures the
// variables which are in scope at the the top-level, so that when use sites
// call ExprTopLevel.SetScope() with the variables which are in scope at the use
// site, ExprTopLevel can automatically correct this by using the variables
// which are in scope at the definition site.
type ExprTopLevel struct {
	Definition    interfaces.Expr   // The definition.
	CapturedScope *interfaces.Scope // The scope at the definition site.
}

// String returns a short representation of this expression.
func (obj *ExprTopLevel) String() string {
	return fmt.Sprintf("topLevel(%s)", obj.Definition.String())
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprTopLevel) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Definition.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprTopLevel) Init(data *interfaces.Data) error {
	return obj.Definition.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprTopLevel) Interpolate() (interfaces.Expr, error) {
	definition, err := obj.Definition.Interpolate()
	if err != nil {
		return nil, err
	}

	return &ExprTopLevel{
		Definition:    definition,
		CapturedScope: obj.CapturedScope,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprTopLevel) Copy() (interfaces.Expr, error) {
	definition, err := obj.Definition.Copy()
	if err != nil {
		return nil, err
	}

	return &ExprTopLevel{
		Definition:    definition,
		CapturedScope: obj.CapturedScope,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprTopLevel) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraint: We know the Definition has to be satisfied before
	// this ExprTopLevel expression itself can be used, since ExprTopLevel
	// delegates to the Definition.
	edge := &pgraph.SimpleEdge{Name: "exprtoplevel"}
	graph.AddEdge(obj.Definition, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Definition.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "exprtopleveldefinition"}
		graph.AddEdge(n, k, edge)
	}

	return graph, cons, nil
}

// SetScope stores the scope for use in this resource.
func (obj *ExprTopLevel) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	// Use the scope captured at the definition site. The parameters from
	// functions enclosing the use site are not visible at the top-level either,
	// so we must clear sctx.
	return obj.Definition.SetScope(obj.CapturedScope, make(map[string]interfaces.Expr))
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprTopLevel) SetType(typ *types.Type) error {
	return obj.Definition.SetType(typ)
}

// Type returns the type of this expression.
func (obj *ExprTopLevel) Type() (*types.Type, error) {
	return obj.Definition.Type()
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer. This Infer is an exception to that pattern.
func (obj *ExprTopLevel) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	typ, invariants, err := obj.Definition.Infer()
	if err != nil {
		return nil, nil, err
	}

	// This adds the obj ptr, so it's seen as an expr that we need to solve.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typ,
		Actual: typ,
	}
	invariants = append(invariants, invar)

	return typ, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprTopLevel) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might.
func (obj *ExprTopLevel) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	return obj.Definition.Graph(env)
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the dest lookup expr) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprTopLevel) SetValue(value types.Value) error {
	return obj.Definition.SetValue(value)
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprTopLevel) Value() (types.Value, error) {
	return obj.Definition.Value()
}

// ExprSingleton is intended to wrap top-level variable definitions. It ensures
// that a single Func is created even if multiple use sites call
// ExprSingleton.Graph().
type ExprSingleton struct {
	Definition interfaces.Expr

	singletonType  *types.Type
	singletonGraph *pgraph.Graph
	singletonFunc  interfaces.Func
	mutex          *sync.Mutex // protects singletonGraph and singletonFunc
}

// String returns a short representation of this expression.
func (obj *ExprSingleton) String() string {
	return fmt.Sprintf("singleton(%s)", obj.Definition.String())
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprSingleton) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Definition.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprSingleton) Init(data *interfaces.Data) error {
	obj.mutex = &sync.Mutex{}
	return obj.Definition.Init(data)
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprSingleton) Interpolate() (interfaces.Expr, error) {
	definition, err := obj.Definition.Interpolate()
	if err != nil {
		return nil, err
	}

	return &ExprSingleton{
		Definition:     definition,
		singletonType:  nil, // each copy should have its own Type
		singletonGraph: nil, // each copy should have its own Graph
		singletonFunc:  nil, // each copy should have its own Func
		mutex:          &sync.Mutex{},
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprSingleton) Copy() (interfaces.Expr, error) {
	definition, err := obj.Definition.Copy()
	if err != nil {
		return nil, err
	}

	return &ExprSingleton{
		Definition:     definition,
		singletonType:  nil, // each copy should have its own Type
		singletonGraph: nil, // each copy should have its own Graph
		singletonFunc:  nil, // each copy should have its own Func
		mutex:          &sync.Mutex{},
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprSingleton) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraint: We know the Definition has to be satisfied before
	// this ExprSingleton expression itself can be used, since ExprSingleton
	// delegates to the Definition.
	edge := &pgraph.SimpleEdge{Name: "exprsingleton"}
	graph.AddEdge(obj.Definition, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Definition.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "exprsingletondefinition"}
		graph.AddEdge(n, k, edge)
	}

	return graph, cons, nil
}

// SetScope stores the scope for use in this resource.
func (obj *ExprSingleton) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	return obj.Definition.SetScope(scope, sctx)
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprSingleton) SetType(typ *types.Type) error {
	return obj.Definition.SetType(typ)
}

// Type returns the type of this expression.
func (obj *ExprSingleton) Type() (*types.Type, error) {
	return obj.Definition.Type()
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer. This Infer is an exception to that pattern.
func (obj *ExprSingleton) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	// shouldn't run in parallel...
	//obj.mutex.Lock()
	//defer obj.mutex.Unlock()

	if obj.singletonType == nil {
		typ, invariants, err := obj.Definition.Infer()
		if err != nil {
			return nil, nil, err
		}
		obj.singletonType = typ

		// This adds the obj ptr, so it's seen as an expr that we need
		// to solve.
		invar := &interfaces.UnificationInvariant{
			Node:   obj,
			Expr:   obj,
			Expect: typ,
			Actual: typ,
		}
		invariants = append(invariants, invar)

		return obj.singletonType, invariants, nil
	}

	// We only need to return the invariants the first time, as done above!
	return obj.singletonType, []*interfaces.UnificationInvariant{}, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprSingleton) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might.
func (obj *ExprSingleton) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.singletonFunc == nil {
		g, f, err := obj.Definition.Graph(env)
		if err != nil {
			return nil, nil, err
		}
		obj.singletonGraph = g
		obj.singletonFunc = f
		return g, f, nil
	}

	return obj.singletonGraph, obj.singletonFunc, nil
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the dest lookup expr) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprSingleton) SetValue(value types.Value) error {
	return obj.Definition.SetValue(value)
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
func (obj *ExprSingleton) Value() (types.Value, error) {
	return obj.Definition.Value()
}

// ExprIf represents an if expression which *must* have both branches, and which
// returns a value. As a result, it has a type. This is different from a StmtIf,
// which does not need to have both branches, and which does not return a value.
type ExprIf struct {
	interfaces.Textarea

	data  *interfaces.Data
	scope *interfaces.Scope // store for referencing this later
	typ   *types.Type

	Condition  interfaces.Expr
	ThenBranch interfaces.Expr // could be an ExprBranch
	ElseBranch interfaces.Expr // could be an ExprBranch
}

// String returns a short representation of this expression.
func (obj *ExprIf) String() string {
	condition := obj.Condition.String()
	thenBranch := obj.ThenBranch.String()
	elseBranch := obj.ElseBranch.String()
	return fmt.Sprintf("if( %s ) { %s } else { %s }", condition, thenBranch, elseBranch)
}

// Apply is a general purpose iterator method that operates on any AST node. It
// is not used as the primary AST traversal function because it is less readable
// and easy to reason about than manually implementing traversal for each node.
// Nevertheless, it is a useful facility for operations that might only apply to
// a select number of node types, since they won't need extra noop iterators...
func (obj *ExprIf) Apply(fn func(interfaces.Node) error) error {
	if err := obj.Condition.Apply(fn); err != nil {
		return err
	}
	if err := obj.ThenBranch.Apply(fn); err != nil {
		return err
	}
	if err := obj.ElseBranch.Apply(fn); err != nil {
		return err
	}
	return fn(obj)
}

// Init initializes this branch of the AST, and returns an error if it fails to
// validate.
func (obj *ExprIf) Init(data *interfaces.Data) error {
	obj.data = data
	obj.Textarea.Setup(data)

	if err := obj.Condition.Init(data); err != nil {
		return err
	}
	if err := obj.ThenBranch.Init(data); err != nil {
		return err
	}
	if err := obj.ElseBranch.Init(data); err != nil {
		return err
	}

	// no errors
	return nil
}

// Interpolate returns a new node (aka a copy) once it has been expanded. This
// generally increases the size of the AST when it is used. It calls Interpolate
// on any child elements and builds the new node with those new node contents.
func (obj *ExprIf) Interpolate() (interfaces.Expr, error) {
	condition, err := obj.Condition.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate Condition")
	}
	thenBranch, err := obj.ThenBranch.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate ThenBranch")
	}
	elseBranch, err := obj.ElseBranch.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate ElseBranch")
	}
	return &ExprIf{
		Textarea:   obj.Textarea,
		data:       obj.data,
		scope:      obj.scope,
		typ:        obj.typ,
		Condition:  condition,
		ThenBranch: thenBranch,
		ElseBranch: elseBranch,
	}, nil
}

// Copy returns a light copy of this struct. Anything static will not be copied.
func (obj *ExprIf) Copy() (interfaces.Expr, error) {
	copied := false
	condition, err := obj.Condition.Copy()
	if err != nil {
		return nil, err
	}
	// must have been copied, or pointer would be same
	if condition != obj.Condition {
		copied = true
	}
	thenBranch, err := obj.ThenBranch.Copy()
	if err != nil {
		return nil, err
	}
	if thenBranch != obj.ThenBranch {
		copied = true
	}
	elseBranch, err := obj.ElseBranch.Copy()
	if err != nil {
		return nil, err
	}
	if elseBranch != obj.ElseBranch {
		copied = true
	}

	if !copied { // it's static
		return obj, nil
	}
	return &ExprIf{
		Textarea:   obj.Textarea,
		data:       obj.data,
		scope:      obj.scope,
		typ:        obj.typ,
		Condition:  condition,
		ThenBranch: thenBranch,
		ElseBranch: elseBranch,
	}, nil
}

// Ordering returns a graph of the scope ordering that represents the data flow.
// This can be used in SetScope so that it knows the correct order to run it in.
func (obj *ExprIf) Ordering(produces map[string]interfaces.Node) (*pgraph.Graph, map[interfaces.Node]string, error) {
	graph, err := pgraph.NewGraph("ordering")
	if err != nil {
		return nil, nil, err
	}
	graph.AddVertex(obj)

	// Additional constraints: We know the condition has to be satisfied
	// before this if expression itself can be used, since we depend on that
	// value.
	edge := &pgraph.SimpleEdge{Name: "exprif"}
	graph.AddEdge(obj.Condition, obj, edge) // prod -> cons

	cons := make(map[interfaces.Node]string)

	g, c, err := obj.Condition.Ordering(produces)
	if err != nil {
		return nil, nil, err
	}
	graph.AddGraph(g) // add in the child graph

	for k, v := range c { // c is consumes
		x, exists := cons[k]
		if exists && v != x {
			return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
		}
		cons[k] = v // add to map

		n, exists := produces[v]
		if !exists {
			continue
		}
		edge := &pgraph.SimpleEdge{Name: "exprifcondition"}
		graph.AddEdge(n, k, edge)
	}

	// don't put obj.Condition here because this adds an extra edge to it!
	nodes := []interfaces.Expr{obj.ThenBranch, obj.ElseBranch}

	for _, node := range nodes { // "dry"
		g, c, err := node.Ordering(produces)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g) // add in the child graph

		// additional constraints...
		edge1 := &pgraph.SimpleEdge{Name: "exprifbranch1"}
		graph.AddEdge(obj.Condition, node, edge1) // prod -> cons
		edge2 := &pgraph.SimpleEdge{Name: "exprifbranchcondition"}
		graph.AddEdge(node, obj, edge2) // prod -> cons

		for k, v := range c { // c is consumes
			x, exists := cons[k]
			if exists && v != x {
				return nil, nil, fmt.Errorf("consumed value is different, got `%+v`, expected `%+v`", x, v)
			}
			cons[k] = v // add to map

			n, exists := produces[v]
			if !exists {
				continue
			}
			edge := &pgraph.SimpleEdge{Name: "exprifbranch2"}
			graph.AddEdge(n, k, edge)
		}
	}

	return graph, cons, nil
}

// SetScope stores the scope for later use in this resource and its children,
// which it propagates this downwards to.
func (obj *ExprIf) SetScope(scope *interfaces.Scope, sctx map[string]interfaces.Expr) error {
	if scope == nil {
		scope = interfaces.EmptyScope()
	}
	obj.scope = scope
	if err := obj.ThenBranch.SetScope(scope, sctx); err != nil {
		return err
	}
	if err := obj.ElseBranch.SetScope(scope, sctx); err != nil {
		return err
	}
	return obj.Condition.SetScope(scope, sctx)
}

// SetType is used to set the type of this expression once it is known. This
// usually happens during type unification, but it can also happen during
// parsing if a type is specified explicitly. Since types are static and don't
// change on expressions, if you attempt to set a different type than what has
// previously been set (when not initially known) this will error.
func (obj *ExprIf) SetType(typ *types.Type) error {
	if obj.typ != nil {
		return obj.typ.Cmp(typ) // if not set, ensure it doesn't change
	}
	obj.typ = typ // set
	return nil
}

// Type returns the type of this expression.
func (obj *ExprIf) Type() (*types.Type, error) {
	if obj.typ != nil {
		return obj.typ, nil
	}

	var typ *types.Type
	testAndSet := func(t *types.Type) error {
		if t == nil {
			return nil // skip
		}
		if typ == nil {
			return nil // it's ok
		}

		if typ.Cmp(t) != nil {
			return fmt.Errorf("inconsistent branch")
		}
		typ = t // save

		return nil
	}

	if obj.ThenBranch != nil {
		if t, err := obj.ThenBranch.Type(); err != nil {
			if err := testAndSet(t); err != nil {
				return nil, err
			}
		}
	}
	if obj.ElseBranch != nil {
		if t, err := obj.ElseBranch.Type(); err != nil {
			if err := testAndSet(t); err != nil {
				return nil, err
			}
		}
	}

	if typ != nil {
		return typ, nil
	}
	return nil, errwrap.Wrapf(interfaces.ErrTypeCurrentlyUnknown, obj.String())
}

// Infer returns the type of itself and a collection of invariants. The returned
// type may contain unification variables. It collects the invariants by calling
// Check on its children expressions. In making those calls, it passes in the
// known type for that child to get it to "Check" it. When the type is not
// known, it should create a new unification variable to pass in to the child
// Check calls. Infer usually only calls Check on things inside of it, and often
// does not call another Infer.
func (obj *ExprIf) Infer() (*types.Type, []*interfaces.UnificationInvariant, error) {
	invariants := []*interfaces.UnificationInvariant{}

	conditionInvars, err := obj.Condition.Check(types.TypeBool) // bool, yes!
	if err != nil {
		return nil, nil, err
	}
	invariants = append(invariants, conditionInvars...)

	// Same unification var because both branches must have the same type.
	typExpr := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}

	thenInvars, err := obj.ThenBranch.Check(typExpr)
	if err != nil {
		return nil, nil, err
	}
	invariants = append(invariants, thenInvars...)

	elseInvars, err := obj.ElseBranch.Check(typExpr)
	if err != nil {
		return nil, nil, err
	}
	invariants = append(invariants, elseInvars...)

	// Every infer call must have this section, because expr var needs this.
	typType := typExpr
	//if obj.typ == nil { // optional says sam
	//	obj.typ = typExpr // sam says we could unconditionally do this
	//}
	if obj.typ != nil {
		typType = obj.typ
	}
	// This must be added even if redundant, so that we collect the obj ptr.
	invar := &interfaces.UnificationInvariant{
		Node:   obj,
		Expr:   obj,
		Expect: typExpr, // This is the type that we return.
		Actual: typType,
	}
	invariants = append(invariants, invar)

	return typExpr, invariants, nil
}

// Check is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func (obj *ExprIf) Check(typ *types.Type) ([]*interfaces.UnificationInvariant, error) {
	return interfaces.GenericCheck(obj, typ)
}

// Func returns a function which returns the correct branch based on the ever
// changing conditional boolean input.
func (obj *ExprIf) Func() (interfaces.Func, error) {
	typ, err := obj.Type()
	if err != nil {
		return nil, err
	}

	return &structs.IfFunc{
		Textarea: obj.Textarea,

		Type: typ, // this is the output type of the expression
	}, nil
}

// Graph returns the reactive function graph which is expressed by this node. It
// includes any vertices produced by this node, and the appropriate edges to any
// vertices that are produced by its children. Nodes which fulfill the Expr
// interface directly produce vertices (and possible children) where as nodes
// that fulfill the Stmt interface do not produces vertices, where as their
// children might. This particular if expression doesn't do anything clever here
// other than adding in both branches of the graph. Since we're functional, this
// shouldn't have any ill effects.
// XXX: is this completely true if we're running technically impure, but safe
// built-in functions on both branches? Can we turn off half of this?
func (obj *ExprIf) Graph(env *interfaces.Env) (*pgraph.Graph, interfaces.Func, error) {
	graph, err := pgraph.NewGraph("if")
	if err != nil {
		return nil, nil, err
	}
	function, err := obj.Func()
	if err != nil {
		return nil, nil, err
	}

	exprs := map[string]interfaces.Expr{
		"c": obj.Condition,
		"a": obj.ThenBranch,
		"b": obj.ElseBranch,
	}
	for _, argName := range []string{"c", "a", "b"} { // deterministic order
		x := exprs[argName]
		g, f, err := x.Graph(env)
		if err != nil {
			return nil, nil, err
		}
		graph.AddGraph(g)

		edge := &interfaces.FuncEdge{Args: []string{argName}}
		graph.AddEdge(f, function, edge) // branch -> if
	}

	return graph, function, nil
}

// SetValue here is a no-op, because algorithmically when this is called from
// the func engine, the child fields (the branches expr's) will have had this
// done to them first, and as such when we try and retrieve the set value from
// this expression by calling `Value`, it will build it from scratch!
func (obj *ExprIf) SetValue(value types.Value) error {
	if err := obj.typ.Cmp(value.Type()); err != nil {
		return err
	}
	// noop!
	//obj.V = value
	return nil
}

// Value returns the value of this expression in our type system. This will
// usually only be valid once the engine has run and values have been produced.
// This might get called speculatively (early) during unification to learn more.
// This particular expression evaluates the condition and returns the correct
// branch's value accordingly.
func (obj *ExprIf) Value() (types.Value, error) {
	boolValue, err := obj.Condition.Value()
	if err != nil {
		return nil, err
	}
	if boolValue.Bool() { // must not panic
		return obj.ThenBranch.Value()
	}
	return obj.ElseBranch.Value()
}
