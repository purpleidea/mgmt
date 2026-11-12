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
	"fmt"

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
		//Textarea: ???, // XXX: add me!

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

			// XXX: do we need to use the `out` arg here?
			// XXX: eg: via .Graph(txn, args) if it's a GraphFunc
			//if graphFunc, ok := wrappedFunc.(interfaces.GraphFunc); ok {
			//	graphFunc.Graph(txn, args)
			//}

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
func FuncToFullFuncValue(makeFunc func() interfaces.Func, typ *types.Type, textarea interfaces.Textarea) *full.FuncValue {

	v := func(txn interfaces.Txn, args []interfaces.Func) (interfaces.Func, error) {
		valueTransformingFunc := makeFunc() // do this once here
		if tf, ok := valueTransformingFunc.(interfaces.TextareaSettable); ok {
			tf.SetTextarea(textarea)
		}
		if buildableFunc, ok := valueTransformingFunc.(interfaces.BuildableFunc); ok {
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

		if graphFunc, ok := valueTransformingFunc.(interfaces.GraphFunc); ok {
			return graphFunc.Graph(txn, args)
		}

		return valueTransformingFunc, nil
	}

	var f interfaces.FuncSig
	fn := makeFunc()
	//if _, ok := fn.(interfaces.StreamableFunc); !ok { // XXX: is this what we want now?
	f = fn.Call
	//}

	// This has the "V" implementation and the simpler "F" implementation
	// which can occasionally be used if the interfaces.Func supports that!
	return &full.FuncValue{
		V: v,
		F: f,
		T: typ,
	}
}

// AddRuntimeShape adds the vertices which a function that builds a subgraph at
// runtime (eg: iter.map) needs around it to the txn, along with their edges.
// Such a function must never add an edge from itself with its own txn, since
// that edge would hold a reference on it which only its own Cleanup could
// release, and so it would never get removed from the graph. Instead, for each
// of the indexes, this adds a vertex which passes that arg along after the
// function has run, so that the subgraph can read it from there. It returns
// those, in the same order, along with the vertex which outputs the value of
// the function, which is where the subgraph must send its value. They form a
// chain from the function to the output, each one taking the one before it as
// its dummy arg, so that none of them is ever a sink. The typ is the signature
// of the function, and the args are the vertices which have an edge to it for
// each of its args. See interfaces.GraphFunc.
func AddRuntimeShape(txn interfaces.Txn, f interfaces.Func, typ *types.Type, args []interfaces.Func, indexes ...int) ([]interfaces.Func, interfaces.Func) {
	inputs := []interfaces.Func{}
	prev, prevType := f, types.TypeNil // the function outputs a dummy nil
	for _, i := range indexes {
		t := typ.Map[typ.Ord[i]]
		input := &OutputFunc{
			Name:      "funcSubgraphInput",
			EdgeName:  OutputFuncArgName,
			Type:      t,
			DummyType: prevType,
		}
		txn.AddEdge(args[i], input, &interfaces.FuncEdge{
			Args: []string{OutputFuncArgName},
		})
		txn.AddEdge(prev, input, &interfaces.FuncEdge{
			Args: []string{OutputFuncDummyArgName},
		})
		inputs = append(inputs, input)
		prev, prevType = input, t
	}

	output := &OutputFunc{
		Name:      "funcSubgraphOutput",
		EdgeName:  OutputFuncArgName,
		Type:      typ.Out, // the return type, not the func type
		DummyType: prevType,
	}
	txn.AddEdge(prev, output, &interfaces.FuncEdge{
		Args: []string{OutputFuncDummyArgName},
	})

	return inputs, output
}

// StmtsToValue wraps the statements which a func built at runtime (eg: the
// active branch of an if statement, or the body of each iteration of a for
// loop) into an opaque value, so that they travel along with the table to the
// Output of the statement which needs them. Use ValueToStmts to get them back.
func StmtsToValue(stmts []interfaces.Stmt) types.Value {
	return &types.OpaqueValue{Data: stmts}
}

// ValueToStmts returns the statements which StmtsToValue wrapped in this value.
// It errors if this value didn't come from there.
func ValueToStmts(v types.Value) ([]interfaces.Stmt, error) {
	x, ok := v.(*types.OpaqueValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value: %v", v)
	}
	stmts, ok := x.Data.([]interfaces.Stmt)
	if !ok {
		return nil, fmt.Errorf("unexpected data: %T", x.Data)
	}
	return stmts, nil
}
