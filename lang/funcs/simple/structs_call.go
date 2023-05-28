// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

// package structs
package simple

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/fancyfunc"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// CallFuncName is the unique name identifier for this function.
	CallFuncName = "call"

	// Suggestion for how to name the edge which connects the input function to CallFunc.
	CallFuncArgNameFunction = "fn"
)

// CallFunc receives a function from upstream, but not the arguments. Instead,
// the Funcs which emit those arguments must be specified at construction time.
// The arguments are connected to the received FuncValues in such a way that
// CallFunc emits the result of applying the function to the arguments.
type CallFunc struct {
	Type     *types.Type // the type of the result of applying the function
	FuncType *types.Type // the type of the function
	EdgeName string

	ArgVertices []interfaces.Func

	init *interfaces.Init

	lastFuncValue *fancyfunc.FuncValue // remember the last function value
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
	typ := obj.FuncType
	// we only care about the output type of calling our func
	if err := obj.Type.Cmp(typ.Out); err != nil {
		return errwrap.Wrapf(err, "call expr type must match func out type")
	}
	if len(obj.ArgVertices) != len(typ.Ord) {
		return fmt.Errorf("number of arg Funcs must match number of func args in the type")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *CallFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil && obj.FuncType != nil { // don't panic if called speculatively
		typ = types.NewType(fmt.Sprintf("func(%s %s) %s", obj.EdgeName, obj.FuncType, obj.Type))
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
	obj.lastFuncValue = nil
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *CallFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes

	// An initially-closed channel from which we receive output lists from the
	// subgraph. This channel is reset when the subgraph is recreated.
	var outputChan chan types.Value = nil

	// Create a subgraph which looks as follows. Most of the nodes are elided
	// because we don't know which nodes the FuncValues will create.
	//
	// digraph {
	//   ArgVertices[0] -> ...
	//   ArgVertices[1] -> ...
	//   ArgVertices[2] -> ...
	//
	//   outputFunc -> "subgraphOutput"
	// }
	replaceSubGraph := func(
		newFuncValue *fancyfunc.FuncValue,
	) error {
		// delete the old subgraph
		obj.init.Txn.Reverse() // XXX: or Clear?

		// create the new subgraph

		outputFunc, err := newFuncValue.Call(obj.init.Txn, obj.ArgVertices)
		if err != nil {
			return errwrap.Wrapf(err, "could not call newFuncValue.Call()")
		}

		edgeName := ChannelBasedSinkFuncArgName
		outputChan = make(chan types.Value)
		subgraphOutput := &ChannelBasedSinkFunc{
			Name:     "subgraphOutput",
			EdgeName: edgeName,
			Chan:     outputChan,
			Type:     obj.Type,
		}
		obj.init.Txn.AddVertex(subgraphOutput)
		obj.init.Txn.AddEdge(outputFunc, subgraphOutput, &interfaces.FuncEdge{Args: []string{edgeName}})

		obj.init.Txn.Commit()

		return nil
	}
	defer func() {
		obj.init.Txn.Reverse()
	}()

	canReceiveMoreFuncValues := true
	canReceiveMoreOutputValues := true
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				canReceiveMoreFuncValues = false
			} else {
				newFuncValue := input.Struct()[obj.EdgeName].(*fancyfunc.FuncValue)

				// If we have a new function, then we need to replace the
				// subgraph with a new one that uses the new function.
				if newFuncValue != obj.lastFuncValue {
					if err := replaceSubGraph(newFuncValue); err != nil {
						return errwrap.Wrapf(err, "could not replace subgraph")
					}
					canReceiveMoreOutputValues = true
					obj.lastFuncValue = newFuncValue
				}
			}

		case outputValue, ok := <-outputChan:
			// send the new output value downstream
			if !ok {
				canReceiveMoreOutputValues = false

				// prevent the next loop iteration from trying to receive from a
				// closed channel
				outputChan = nil
			} else {
				select {
				case obj.init.Output <- outputValue:
				case <-ctx.Done():
					return nil
				}
			}

		case <-ctx.Done():
			return nil
		}

		if !canReceiveMoreFuncValues && !canReceiveMoreOutputValues {
			return nil
		}
	}
}
