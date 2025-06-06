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
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ForFuncName is the unique name identifier for this function.
	ForFuncName = "for"

	// ForFuncArgNameList is the name for the edge which connects the input
	// list to CallFunc.
	ForFuncArgNameList = "list"
)

// ForFunc receives a list from upstream. We iterate over the received list to
// build a subgraph that processes each element, and in doing so we get a larger
// function graph. This is rebuilt as necessary if the input list changes.
type ForFunc struct {
	interfaces.Textarea

	IndexType *types.Type
	ValueType *types.Type

	EdgeName string // name of the edge used

	AppendToIterBody func(innerTxn interfaces.Txn, index int, value interfaces.Func) error
	ClearIterBody    func(length int)

	init *interfaces.Init

	lastInputListLength int // remember the last input list length
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ForFunc) String() string {
	return ForFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *ForFunc) Validate() error {
	if obj.IndexType == nil {
		return fmt.Errorf("must specify a type")
	}
	if obj.ValueType == nil {
		return fmt.Errorf("must specify a type")
	}

	// TODO: maybe we can remove this if we use this for core functions...
	if obj.EdgeName == "" {
		return fmt.Errorf("must specify an edge name")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *ForFunc) Info() *interfaces.Info {
	var typ *types.Type

	if obj.IndexType != nil && obj.ValueType != nil { // don't panic if called speculatively
		// XXX: Improve function engine so it can return no value?
		//typ = types.NewType(fmt.Sprintf("func(%s []%s)", obj.EdgeName, obj.ValueType)) // returns nothing
		// XXX: Temporary float type to prove we're dropping the output since we don't use it.
		typ = types.NewType(fmt.Sprintf("func(%s []%s) float", obj.EdgeName, obj.ValueType))
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this composite function.
func (obj *ForFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.lastInputListLength = -1
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *ForFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes

	// A Func to send input lists to the subgraph. The Txn.Erase() call
	// ensures that this Func is not removed when the subgraph is recreated,
	// so that the function graph can propagate the last list we received to
	// the subgraph.
	inputChan := make(chan types.Value)
	subgraphInput := &ChannelBasedSourceFunc{
		Name:   "subgraphInput",
		Source: obj,
		Chan:   inputChan,
		Type:   obj.listType(),
	}
	obj.init.Txn.AddVertex(subgraphInput)
	if err := obj.init.Txn.Commit(); err != nil {
		return errwrap.Wrapf(err, "commit error in Stream")
	}
	obj.init.Txn.Erase() // prevent the next Reverse() from removing subgraphInput
	defer func() {
		close(inputChan)
		obj.init.Txn.Reverse()
		obj.init.Txn.DeleteVertex(subgraphInput)
		obj.init.Txn.Commit()
	}()

	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // block looping back here
				//canReceiveMoreListValues = false
				// We don't ever shutdown here, since even if we
				// don't get more lists, that last list value is
				// still propagating inside of the subgraph and
				// so we don't want to shutdown since that would
				// reverse the txn which we only do at the very
				// end on graph shutdown.
				continue
			}

			forList, exists := input.Struct()[obj.EdgeName]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			// If the length of the input list has changed, then we
			// need to replace the subgraph with a new one that has
			// that many "tentacles". Basically the shape of the
			// graph depends on the length of the list. If we get a
			// brand new list where each value is different, but
			// the length is the same, then we can just flow new
			// values into the list and we don't need to change the
			// graph shape! Changing the graph shape is more
			// expensive, so we don't do it when not necessary.
			n := len(forList.List())

			//if forList.Cmp(obj.lastForList) != nil // don't!
			if n != obj.lastInputListLength {
				//obj.lastForList = forList
				obj.lastInputListLength = n
				// replaceSubGraph uses the above two values
				if err := obj.replaceSubGraph(subgraphInput); err != nil {
					return errwrap.Wrapf(err, "could not replace subgraph")
				}
			}

			// send the new input list to the subgraph
			select {
			case inputChan <- forList:
			case <-ctx.Done():
				return nil
			}

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- &types.FloatValue{
			V: 42.0, // XXX: temporary
		}:
		case <-ctx.Done():
			return nil
		}
	}
}

func (obj *ForFunc) replaceSubGraph(subgraphInput interfaces.Func) error {
	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	obj.ClearIterBody(obj.lastInputListLength) // XXX: pass in size?

	for i := 0; i < obj.lastInputListLength; i++ {
		i := i
		argName := "forInputList"

		inputElemFunc := SimpleFnToDirectFunc(
			fmt.Sprintf("forInputElem[%d]", i),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("inputElemFunc: expected a single argument")
					}
					arg := args[0]

					list, ok := arg.(*types.ListValue)
					if !ok {
						return nil, fmt.Errorf("inputElemFunc: expected a ListValue argument")
					}

					return list.List()[i], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s) %s", argName, obj.listType(), obj.ValueType)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFunc)

		obj.init.Txn.AddEdge(subgraphInput, inputElemFunc, &interfaces.FuncEdge{
			Args: []string{argName},
		})

		if err := obj.AppendToIterBody(obj.init.Txn, i, inputElemFunc); err != nil {
			return errwrap.Wrapf(err, "could not call AppendToIterBody()")
		}
	}

	return obj.init.Txn.Commit()
}

func (obj *ForFunc) listType() *types.Type {
	return types.NewType(fmt.Sprintf("[]%s", obj.ValueType))
}
