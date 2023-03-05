// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

//package structs
package ast

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/fancyfunc"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// CallFuncName is the unique name identifier for this function.
	CallFuncName = "call"
)

// CallFunc is a function that takes in a function and all the args, and passes
// through the results of running the function call.
type CallFunc struct {
	Type     *types.Type // this is the type of the var's value that we hold
	FuncType *types.Type
	Edge     string // name of the edge used (typically starts with: `call:`)
	//Func interfaces.Func // this isn't actually used in the Stream :/
	//Fn *types.FuncValue // pass in the actual function instead of Edge

	// Indexed specifies that args are accessed by index instead of name.
	// This is currently unused.
	Indexed     bool
	ArgVertices []interfaces.Func

	init          *interfaces.Init
	reversibleTxn *interfaces.ReversibleTxn

	closeChan chan struct{}
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *CallFunc) String() string {
	return CallFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *CallFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	if obj.FuncType == nil {
		return fmt.Errorf("must specify a func type")
	}
	// TODO: maybe we can remove this if we use this for core functions...
	if obj.Edge == "" {
		return fmt.Errorf("must specify an edge name")
	}
	typ := obj.FuncType
	// we only care about the output type of calling our func
	if err := obj.Type.Cmp(typ.Out); err != nil {
		return errwrap.Wrapf(err, "call expr type must match func out type")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *CallFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		typ = &types.Type{
			Kind: types.KindFunc, // function type
			Map:  make(map[string]*types.Type),
			Ord:  []string{},
			Out:  obj.Type, // this is the output type for the expression
		}

		sig := obj.FuncType
		if obj.Edge != "" {
			typ.Map[obj.Edge] = sig // we get a function in
			typ.Ord = append(typ.Ord, obj.Edge)
		}

		// add any incoming args
		for _, key := range sig.Ord { // sig.Out, not sig!
			typ.Map[key] = sig.Map[key]
			typ.Ord = append(typ.Ord, key)
		}
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this composite function.
func (obj *CallFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.reversibleTxn = &interfaces.ReversibleTxn{
		InnerTxn: init.Txn,
	}
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *CallFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes

	// Create a channel to receive the output of the subgraph.
	outChan := make(chan types.Value)

	// TODO: detect when the subgraph will no longer emit any new values and
	// inform the for loop below so it can stop when both the input FuncValues
	// have stopped coming in and the subgraph has stopped emitting values.
	subgraphOutputToChannel := &ExprFunc{
		Title: "subgraphOutputToChannel",
		Values: []*types.SimpleFn{
			{
				V: func(args []types.Value) (types.Value, error) {
					outputValue := args[0]
					outChan <- outputValue
					return nil, nil
				},
				T: types.NewType(fmt.Sprintf("func(outputValue %s)", obj.Type)),
			},
		},
	}
	// TODO: call Init() on every node we create

	// Use obj.init.Txn instead of obj.txn because we don't want to
	// reverse this operation when the subgraph changes.
	obj.init.Txn.AddVertex(subgraphOutputToChannel)
	obj.init.Txn.Commit()

	defer func() {
		obj.init.Txn.DeleteVertex(subgraphOutputToChannel)
		obj.init.Txn.Commit()
	}()

	// Every time the FuncValue changes, recreate the subgraph by calling the FuncValue.
	var prevFn *fancyfunc.FuncValue
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				// The graph cannot change anymore, but values can still flow
				// through it, e.g. if the function is a lambda and one of the
				// captured variables (not an argument) changes. Therefore, we
				// must not close the output channel yet.
			}

			st := input.(*types.StructValue) // must be!

			// get the function
			fnValue, exists := st.Lookup("fn")
			if !exists {
				return fmt.Errorf("missing function")
			}
			fn, isFuncValue := fnValue.(*fancyfunc.FuncValue)
			if !isFuncValue {
				return fmt.Errorf("function is not a FuncValue")
			}

			if fn != prevFn {
				// The function changed, so we need to recreate the subgraph.
				obj.reversibleTxn.Reset()
				outVertex, err := fn.Call(obj.reversibleTxn, obj.ArgVertices)
				if err != nil {
					return nil
				}

				// attach the output vertex to the output channel
				obj.reversibleTxn.AddEdge(outVertex, subgraphOutputToChannel, &pgraph.SimpleEdge{
					Name: "subgraphOutput",
				})
				obj.reversibleTxn.Commit()
			}

		case output, ok := <-outChan:
			if !ok {
				panic("We don't yet have any logic to handle the case where the subgraph stops emitting values.")
			}

			obj.init.Output <- output

		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *CallFunc) Close() error {
	close(obj.closeChan)
	return nil
}
