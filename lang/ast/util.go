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

package ast

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/funcs/simplepoly"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// FuncPrefixToFunctionsScope is a helper function to return the functions
// portion of the scope from a function prefix lookup. Basically this wraps the
// implementation in the Func interface in the *ExprFunc struct.
func FuncPrefixToFunctionsScope(prefix string) map[string]interfaces.Expr {
	fns := funcs.LookupPrefix(prefix) // map[string]func() interfaces.Func
	exprs := make(map[string]interfaces.Expr)
	for name, f := range fns {

		x := f() // inspect
		// We can pass in Fns []*types.FuncValue for the simple and
		// simplepoly API's and avoid the double wrapping from the
		// simple/simplepoly API's to the main function api and back.
		if st, ok := x.(*simple.WrappedFunc); simple.DirectInterface && ok {
			fn := &ExprFunc{
				Title: name,

				Values: []*types.FuncValue{st.Fn}, // just one!
			}
			// XXX: should we run fn.SetType(st.Fn.Type()) ?
			exprs[name] = fn
			continue
		} else if st, ok := x.(*simplepoly.WrappedFunc); simplepoly.DirectInterface && ok {
			fn := &ExprFunc{
				Title: name,

				Values: st.Fns,
			}
			exprs[name] = fn
			continue
		}

		fn := &ExprFunc{
			Title: name,
			// We need to pass in the constructor function, because
			// we'll need more than one copy of this function if it
			// is used in more than one place so we can build more.
			Function: f, // func() interfaces.Func
		}
		exprs[name] = fn
	}
	return exprs
}

// VarPrefixToVariablesScope is a helper function to return the variables
// portion of the scope from a variable prefix lookup. Basically this is useful
// to pull out a portion of the variables we've defined by API.
func VarPrefixToVariablesScope(prefix string) map[string]interfaces.Expr {
	fns := vars.LookupPrefix(prefix) // map[string]func() interfaces.Var
	exprs := make(map[string]interfaces.Expr)
	for name, f := range fns {
		x := f() // inspect
		expr, err := ValueToExpr(x)
		if err != nil {
			panic(fmt.Sprintf("could not build expr: %+v", err))
		}
		exprs[name] = expr
	}
	return exprs
}

// MergeExprMaps merges the two maps of Expr's, and errors if any overwriting
// would occur. If any prefix string is specified, that is added to the keys of
// the second "extra" map before doing the merge. This doesn't change the input
// maps.
func MergeExprMaps(m, extra map[string]interfaces.Expr, prefix ...string) (map[string]interfaces.Expr, error) {
	p := strings.Join(prefix, "") // hack to have prefix be optional

	result := map[string]interfaces.Expr{}
	for k, v := range m {
		result[k] = v // copy
	}

	for k, v := range extra {
		name := p + k
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate variable: %s", name)
		}
		result[name] = v
	}

	return result, nil
}

// ValueToExpr converts a Value into the equivalent Expr.
// FIXME: Add some tests for this function.
func ValueToExpr(val types.Value) (interfaces.Expr, error) {
	var expr interfaces.Expr

	switch x := val.(type) {
	case *types.BoolValue:
		expr = &ExprBool{
			V: x.Bool(),
		}

	case *types.StrValue:
		expr = &ExprStr{
			V: x.Str(),
		}

	case *types.IntValue:
		expr = &ExprInt{
			V: x.Int(),
		}

	case *types.FloatValue:
		expr = &ExprFloat{
			V: x.Float(),
		}

	case *types.ListValue:
		exprs := []interfaces.Expr{}

		for _, v := range x.List() {
			e, err := ValueToExpr(v)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, e)
		}

		expr = &ExprList{
			Elements: exprs,
		}

	case *types.MapValue:
		kvs := []*ExprMapKV{}

		for k, v := range x.Map() {
			kx, err := ValueToExpr(k)
			if err != nil {
				return nil, err
			}
			vx, err := ValueToExpr(v)
			if err != nil {
				return nil, err
			}
			kv := &ExprMapKV{
				Key: kx,
				Val: vx,
			}
			kvs = append(kvs, kv)
		}

		expr = &ExprMap{
			KVs: kvs,
		}

	case *types.StructValue:
		fields := []*ExprStructField{}

		for k, v := range x.Struct() {
			fx, err := ValueToExpr(v)
			if err != nil {
				return nil, err
			}
			field := &ExprStructField{
				Name:  k,
				Value: fx,
			}
			fields = append(fields, field)
		}

		expr = &ExprStruct{
			Fields: fields,
		}

	case *types.FuncValue:
		// TODO: this particular case is particularly untested!
		expr = &ExprFunc{
			Title: "<func from ValueToExpr>", // TODO: change this?
			// TODO: symmetrically, it would have used x.Func() here
			Values: []*types.FuncValue{
				x, // just one!
			},
		}

	case *types.VariantValue:
		// TODO: should this be allowed, or should we unwrap them?
		return nil, fmt.Errorf("variant values are not supported")

	default:
		return nil, fmt.Errorf("unknown type (%T) for value: %+v", val, val)
	}

	return expr, expr.SetType(val.Type())
}

// CollectFiles collects all the files used in the AST. You will see more files
// based on how many compiling steps have run. In general, this is useful for
// collecting all the files needed to store in our file system for a deploy.
func CollectFiles(ast interfaces.Stmt) ([]string, error) {
	// collect the list of files
	fileList := []string{}
	fn := func(node interfaces.Node) error {
		// redundant check for example purposes
		stmt, ok := node.(interfaces.Stmt)
		if !ok {
			return nil
		}
		body, ok := stmt.(*StmtProg)
		if !ok {
			return nil
		}
		// collect into global
		fileList = append(fileList, body.importFiles...)
		return nil
	}
	if err := ast.Apply(fn); err != nil {
		return nil, errwrap.Wrapf(err, "can't retrieve paths")
	}
	return fileList, nil
}
