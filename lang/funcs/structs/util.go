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

package structs

import (
	"github.com/purpleidea/mgmt/lang/funcs/wrapped"
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
	var typ *types.Type
	if fv != nil { // TODO: is this necessary?
		typ = fv.T
	}
	return &wrapped.Func{
		Name: name,
		Type: typ, // TODO: is this needed?
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
		F: nil, // unused
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
// interfaces.Func to the graph. Note that this means the *full.FuncValue can
// only be called once.
func FuncToFullFuncValue(makeFunc func() interfaces.Func, typ *types.Type) *full.FuncValue {

	v := func(txn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
		valueTransformingFunc := makeFunc() // do this once here
		buildableFunc, ok := valueTransformingFunc.(interfaces.BuildableFunc)
		if ok {
			// Set the type in case it's not already done.
			if _, err := buildableFunc.Build(typ); err != nil {
				// programming error?
				return nil, err
			}
		}
		for i, arg := range args {
			argName := typ.Ord[i]
			txn.AddEdge(arg, valueTransformingFunc, &interfaces.FuncEdge{
				Args: []string{argName},
			})
		}
		return valueTransformingFunc, nil
	}

	var f interfaces.FuncSig
	callableFunc, ok := makeFunc().(interfaces.CallableFunc)
	if ok {
		f = callableFunc.Call
	}

	// This has the "V" implementation and the simpler "F" implementation
	// which can occasionally be used if the interfaces.Func supports that!
	return &full.FuncValue{
		V: v,
		F: f,
		T: typ,
	}
}
