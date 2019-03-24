// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package lang // TODO: move this into a sub package of lang/$name?

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/hashicorp/hil"
	hilast "github.com/hashicorp/hil/ast"
)

// Pos represents a position in the code.
// TODO: consider expanding with range characteristics.
type Pos struct {
	Line     int    // line number starting at 1
	Column   int    // column number starting at 1
	Filename string // optional source filename, if known
}

// InterpolateInfo contains some information passed around during interpolation.
// TODO: rename to Info if this is moved to its own package.
type InterpolateInfo struct {
	// Prefix used for path namespacing if required.
	Prefix string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// InterpolateStr interpolates a string and returns the representative AST. This
// particular implementation uses the hashicorp hil library and syntax to do so.
func InterpolateStr(str string, pos *Pos, info *InterpolateInfo) (interfaces.Expr, error) {
	if info.Debug {
		info.Logf("interpolating: %s", str)
	}
	var line, column int = -1, -1
	var filename string
	if pos != nil {
		line = pos.Line
		column = pos.Column
		filename = pos.Filename
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
	if info.Debug {
		info.Logf("tree: %+v", tree)
	}

	transformInfo := &InterpolateInfo{
		Prefix: info.Prefix,
		Debug:  info.Debug,
		Logf: func(format string, v ...interface{}) {
			info.Logf("transform: "+format, v...)
		},
	}
	result, err := hilTransform(tree, transformInfo)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error running AST map: `%s`", str)
	}
	if info.Debug {
		info.Logf("transform: %+v", result)
	}

	// make sure to run the Init on the new expression
	return result, errwrap.Wrapf(result.Init(&interfaces.Data{
		Debug: info.Debug,
		Logf:  info.Logf,
	}), "init failed")
}

// hilTransform returns the AST equivalent of the hil AST.
func hilTransform(root hilast.Node, info *InterpolateInfo) (interfaces.Expr, error) {
	switch node := root.(type) {
	case *hilast.Output: // common root node
		if info.Debug {
			info.Logf("got output type: %+v", node)
		}

		if len(node.Exprs) == 0 {
			return nil, fmt.Errorf("no expressions found")
		}
		if len(node.Exprs) == 1 {
			return hilTransform(node.Exprs[0], info)
		}

		// assumes len > 1
		args := []interfaces.Expr{}
		for _, n := range node.Exprs {
			expr, err := hilTransform(n, info)
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
		if info.Debug {
			info.Logf("got function type: %+v", node)
		}
		args := []interfaces.Expr{}
		for _, n := range node.Args {
			arg, err := hilTransform(n, info)
			if err != nil {
				return nil, fmt.Errorf("call failed: %+v", err)
			}
			args = append(args, arg)
		}

		return &ExprCall{
			Name: node.Func, // name
			Args: args,
		}, nil

	case *hilast.LiteralNode: // string, int, etc...
		if info.Debug {
			info.Logf("got literal type: %+v", node)
		}

		switch node.Typex {
		case hilast.TypeBool:
			return &ExprBool{
				V: node.Value.(bool),
			}, nil

		case hilast.TypeString:
			return &ExprStr{
				V: node.Value.(string),
			}, nil

		case hilast.TypeInt:
			return &ExprInt{
				// node.Value is an int stored as an interface
				V: int64(node.Value.(int)),
			}, nil

		case hilast.TypeFloat:
			return &ExprFloat{
				V: node.Value.(float64),
			}, nil

		// TODO: should we handle these too?
		//case hilast.TypeList:
		//case hilast.TypeMap:

		default:
			return nil, fmt.Errorf("unmatched type: %T", node)
		}

	case *hilast.VariableAccess: // variable lookup
		if info.Debug {
			info.Logf("got variable access type: %+v", node)
		}
		return &ExprVar{
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

	operator := &ExprStr{
		V: "+", // for PLUS this is a `+` character
	}

	if len(exprs) == 1 {
		return exprs[0], nil // just return self
	}
	//if len(exprs) == 1 {
	//	arg := exprs[0]
	//	emptyStr := &ExprStr{
	//		V: "", // empty str
	//	}
	//	return &ExprCall{
	//		Name: operatorFuncName, // concatenate the two strings with + operator
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

	return &ExprCall{
		Name: operatorFuncName, // concatenate the two strings with + operator
		Args: []interfaces.Expr{
			operator, // operator first
			head,     // string arg
			grouped,  // nested function call which returns a string
		},
	}, nil
}
