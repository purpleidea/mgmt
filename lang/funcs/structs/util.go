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

package structs

import (
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/types/full"
)

// In the following set of conversion functions, a "constant" Func is a node
// with in-degree zero which always outputs the same function value, while a
// "direct" Func is a node with one upstream node for each of the function's
// arguments.

// FuncValueToConstFunc transforms a *full.FuncValue into an interfaces.Func
// which is implemented by &ConstFunc{}.
func FuncValueToConstFunc(fv *full.FuncValue) interfaces.Func {
	return &ConstFunc{
		Value:    fv,
		NameHint: "FuncValue",
	}
}

// SimpleFnToDirectFunc transforms a name and *types.FuncValue into an
// interfaces.Func which is implemented by &simple.WrappedFunc{}.
func SimpleFnToDirectFunc(name string, fv *types.FuncValue) interfaces.Func {
	return &simple.WrappedFunc{
		Name: name,
		Fn:   fv,
	}
}

// SimpleFnToFuncValue transforms a name and *types.FuncValue into a
// *full.FuncValue.
func SimpleFnToFuncValue(name string, fv *types.FuncValue) *full.FuncValue {
	return &full.FuncValue{
		V: func(txn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
			wrappedFunc := SimpleFnToDirectFunc(name, fv)
			txn.AddVertex(wrappedFunc)
			for i, arg := range args {
				argName := fv.T.Ord[i]
				txn.AddEdge(arg, wrappedFunc, &interfaces.FuncEdge{
					Args: []string{argName},
				})
			}
			return wrappedFunc, nil
		},
		T: fv.T,
	}
}

// SimpleFnToConstFunc transforms a name and *types.FuncValue into an
// interfaces.Func which is implemented by FuncValueToConstFunc and
// SimpleFnToFuncValue.
func SimpleFnToConstFunc(name string, fv *types.FuncValue) interfaces.Func {
	return FuncValueToConstFunc(SimpleFnToFuncValue(name, fv))
}

// FuncToFullFuncValue creates a *full.FuncValue which adds the given
// interfaces.Func to the graph. Note that this means the *full.FuncValue
// can only be called once.
func FuncToFullFuncValue(valueTransformingFunc interfaces.Func, typ *types.Type) *full.FuncValue {
	return &full.FuncValue{
		V: func(txn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
			for i, arg := range args {
				argName := typ.Ord[i]
				txn.AddEdge(arg, valueTransformingFunc, &interfaces.FuncEdge{
					Args: []string{argName},
				})
			}
			return valueTransformingFunc, nil
		},
		T: typ,
	}
}
