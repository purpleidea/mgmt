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

// Package interpolate contains the string interpolation parser and associated
// structs and code.
package interpolate

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/operators"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/hashicorp/hil"
	hilast "github.com/hashicorp/hil/ast"
)

const (
	// UseHilInterpolation specifies that we use the legacy Hil interpolate.
	// This can't properly escape a $ in the standard way. It's here in case
	// someone wants to play with it and examine how the AST stuff worked...
	UseHilInterpolation = false

	// UseOptimizedConcat uses a simpler to unify concat operator instead of
	// the normal + operator which uses fancy polymorphic type unification.
	UseOptimizedConcat = true
)

// StrInterpolate interpolates a string and returns the representative AST. If
// there was nothing to interpolate, this returns (nil, nil).
func StrInterpolate(str string, textarea *interfaces.Textarea, data *interfaces.Data) (interfaces.Expr, error) {
	if data.Debug {
		data.Logf("interpolating: %s", str)
	}

	if UseHilInterpolation {
		return HilInterpolate(str, textarea, data)
	}
	return RagelInterpolate(str, textarea, data)
}

// RagelInterpolate interpolates a string and returns the representative AST. It
// uses the ragel parser to perform the string interpolation. If there was
// nothing to interpolate, this returns (nil, nil).
func RagelInterpolate(str string, textarea *interfaces.Textarea, data *interfaces.Data) (interfaces.Expr, error) {
	sequence, err := Parse(str)
	if err != nil {
		return nil, errwrap.Wrapf(err, "parser failed")
	}

	// If we didn't find anything of value, we got an empty string...
	if len(sequence) == 0 && str == "" { // be doubly sure...
		return nil, nil // pass through, nothing changed
	}

	if len(sequence) == 1 {
		if x, ok := sequence[0].(Literal); ok && x.Value == str {
			return nil, nil // pass through, nothing changed
		}
	}

	exprs := []interfaces.Expr{}
	for _, term := range sequence {

		switch t := term.(type) {
		case Literal:
			expr := &ast.ExprStr{
				Textarea: *textarea, // XXX: until we re-calculate
				V:        t.Value,
			}
			exprs = append(exprs, expr)

		case Variable:
			expr := &ast.ExprVar{
				Textarea: *textarea, // XXX: until we re-calculate
				Name:     t.Name,
			}
			exprs = append(exprs, expr)
		default:
			return nil, fmt.Errorf("unknown term (%T): %+v", t, t)
		}
	}

	// The parser produces non-optimal results where two strings are next to
	// each other, when they could be statically combined together.
	simplified, err := simplifyExprList(exprs)
	if err != nil {
		return nil, errwrap.Wrapf(err, "expr list simplify failed")
	}

	result, err := concatExprListIntoCall(simplified)
	if err != nil {
		return nil, errwrap.Wrapf(err, "concat expr list failed")
	}

	return result, errwrap.Wrapf(result.Init(data), "init failed")
}

// HilInterpolate interpolates a string and returns the representative AST. This
// particular implementation uses the hashicorp hil library and syntax to do so.
func HilInterpolate(str string, textarea *interfaces.Textarea, data *interfaces.Data) (interfaces.Expr, error) {
	var line, column int = -1, -1
	var filename string
	if textarea != nil {
		startLine, startColumn := textarea.Pos() // zero based
		line = startLine
		column = startColumn
		filename = textarea.Filename() // TODO: .Path() instead?
	}
	hilPos := hilast.Pos{
		Line:     line,
		Column:   column,
		Filename: filename,
	}
	// should not error on plain strings
	tree, err := hil.ParseWithPosition(str, hilPos)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't parse string interpolation: `%s`", str)
	}
	if data.Debug {
		data.Logf("tree: %+v", tree)
	}

	transformData := &interfaces.Data{
		// TODO: add missing fields here if/when needed
		Fs:       data.Fs,
		FsURI:    data.FsURI,
		Base:     data.Base,
		Files:    data.Files,
		Imports:  data.Imports,
		Metadata: data.Metadata,
		Modules:  data.Modules,

		LexParser:       data.LexParser,
		Downloader:      data.Downloader,
		StrInterpolater: data.StrInterpolater,
		SourceFinder:    data.SourceFinder,
		//World: data.World, // TODO: do we need this?

		Prefix: data.Prefix,
		Debug:  data.Debug,
		Logf: func(format string, v ...interface{}) {
			data.Logf("transform: "+format, v...)
		},
	}
	result, err := hilTransform(tree, transformData)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error running AST map: `%s`", str)
	}
	if data.Debug {
		data.Logf("transform: %+v", result)
	}

	// make sure to run the Init on the new expression
	return result, errwrap.Wrapf(result.Init(data), "init failed")
}

// hilTransform returns the AST equivalent of the hil AST.
func hilTransform(root hilast.Node, data *interfaces.Data) (interfaces.Expr, error) {
	switch node := root.(type) {
	case *hilast.Output: // common root node
		if data.Debug {
			data.Logf("got output type: %+v", node)
		}

		if len(node.Exprs) == 0 {
			return nil, fmt.Errorf("no expressions found")
		}
		if len(node.Exprs) == 1 {
			return hilTransform(node.Exprs[0], data)
		}

		// assumes len > 1
		args := []interfaces.Expr{}
		for _, n := range node.Exprs {
			expr, err := hilTransform(n, data)
			if err != nil {
				return nil, errwrap.Wrapf(err, "root failed")
			}
			args = append(args, expr)
		}

		// XXX: i think we should be adding these args together, instead
		// of grouping for example...
		result, err := concatExprListIntoCall(args)
		if err != nil {
			return nil, errwrap.Wrapf(err, "function grouping failed")
		}
		return result, nil

	case *hilast.Call:
		if data.Debug {
			data.Logf("got function type: %+v", node)
		}
		args := []interfaces.Expr{}
		for _, n := range node.Args {
			arg, err := hilTransform(n, data)
			if err != nil {
				return nil, fmt.Errorf("call failed: %+v", err)
			}
			args = append(args, arg)
		}

		return &ast.ExprCall{
			Name: node.Func, // name
			Args: args,
		}, nil

	case *hilast.LiteralNode: // string, int, etc...
		if data.Debug {
			data.Logf("got literal type: %+v", node)
		}

		switch node.Typex {
		case hilast.TypeBool:
			return &ast.ExprBool{
				V: node.Value.(bool),
			}, nil

		case hilast.TypeString:
			return &ast.ExprStr{
				V: node.Value.(string),
			}, nil

		case hilast.TypeInt:
			return &ast.ExprInt{
				// node.Value is an int stored as an interface
				V: int64(node.Value.(int)),
			}, nil

		case hilast.TypeFloat:
			return &ast.ExprFloat{
				V: node.Value.(float64),
			}, nil

		// TODO: should we handle these too?
		//case hilast.TypeList:
		//case hilast.TypeMap:

		default:
			return nil, fmt.Errorf("unmatched type: %T", node)
		}

	case *hilast.VariableAccess: // variable lookup
		if data.Debug {
			data.Logf("got variable access type: %+v", node)
		}
		return &ast.ExprVar{
			Name: node.Name,
		}, nil

	//case *hilast.Index:
	//	if va, ok := node.Target.(*hilast.VariableAccess); ok {
	//		v, err := NewInterpolatedVariable(va.Name)
	//		if err != nil {
	//			resultErr = err
	//			return n
	//		}
	//		result = append(result, v)
	//	}
	//	if va, ok := node.Key.(*hilast.VariableAccess); ok {
	//		v, err := NewInterpolatedVariable(va.Name)
	//		if err != nil {
	//			resultErr = err
	//			return n
	//		}
	//		result = append(result, v)
	//	}

	default:
		return nil, fmt.Errorf("unmatched type: %+v", node)
	}
}

// concatExprListIntoCall takes a list of expressions, and combines them into an
// expression which ultimately concatenates them all together with a + operator.
// TODO: this assumes they're all strings, do we need to watch out for int's?
func concatExprListIntoCall(exprs []interfaces.Expr) (interfaces.Expr, error) {
	if len(exprs) == 0 {
		return nil, fmt.Errorf("empty list")
	}

	operator := &ast.ExprStr{
		V: "+", // for PLUS this is a `+` character
	}

	if len(exprs) == 1 {
		return exprs[0], nil // just return self
	}
	//if len(exprs) == 1 {
	//	arg := exprs[0]
	//	emptyStr := &ast.ExprStr{
	//		V: "", // empty str
	//	}
	//	return &ast.ExprCall{
	//		Name: operators.OperatorFuncName, // concatenate the two strings with + operator
	//		Args: []interfaces.Expr{
	//			operator, // operator first
	//			arg,      // string arg
	//			emptyStr,
	//		},
	//	}, nil
	//}

	head, tail := exprs[0], exprs[1:]

	grouped, err := concatExprListIntoCall(tail)
	if err != nil {
		return nil, err
	}

	// Faster variant, but doesn't allow potential future more exotic string
	// interpolation which would need a more expressive plus operator. I do
	// not think we'll ever need that, but leave it in for now as a const.
	if UseOptimizedConcat {
		return &ast.ExprCall{
			// NOTE: if we don't set the data field we need Init() called on it!
			Name: funcs.ConcatFuncName, // concatenate the two strings with concat function
			Args: []interfaces.Expr{
				head,    // string arg
				grouped, // nested function call which returns a string
			},
		}, nil
	}

	return &ast.ExprCall{
		// NOTE: if we don't set the data field we need Init() called on it!
		Name: operators.OperatorFuncName, // concatenate the two strings with + operator
		Args: []interfaces.Expr{
			operator, // operator first
			head,     // string arg
			grouped,  // nested function call which returns a string
		},
	}, nil
}

// simplifyExprList takes a list of *ExprStr and *ExprVar and groups the
// sequential *ExprStr's together. If you pass it a list of Expr's that contains
// a different type of Expr, then this will error.
func simplifyExprList(exprs []interfaces.Expr) ([]interfaces.Expr, error) {
	last := false
	result := []interfaces.Expr{}

	for _, x := range exprs {
		switch v := x.(type) {
		case *ast.ExprStr:
			if !last {
				last = true
				result = append(result, x)
				continue
			}

			// combine!
			expr := result[len(result)-1] // there has to be at least one
			str, ok := expr.(*ast.ExprStr)
			if !ok {
				// programming error
				return nil, fmt.Errorf("unexpected type (%T)", expr)
			}
			str.V += v.V // combine!
			//last = true // redundant, it's already true
			// ... and don't append, we've combined!

		case *ast.ExprVar:
			last = false // the next one can't combine with me
			result = append(result, x)

		default:
			return nil, fmt.Errorf("unsupported type (%T)", x)
		}
	}

	return result, nil
}
