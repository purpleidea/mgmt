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

package coreiter

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/structs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/types/full"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// MapFuncName is the name this function is registered as.
	MapFuncName = "map"

	// arg names...
	mapArgNameInputs   = "inputs"
	mapArgNameFunction = "function"

	mapArgNameArgName = "name-which-can-vary-over-time" // XXX: weird but ok
)

func init() {
	funcs.ModuleRegister(ModuleName, MapFuncName, func() interfaces.Func { return &MapFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &MapFunc{} // ensure it meets this expectation

// MapFunc is the standard map iterator function that applies a function to each
// element in a list. It returns a list with the same number of elements as the
// input list. There is no requirement that the element output type be the same
// as the input element type. This implements the signature: `func(inputs []?1,
// function func(?1) ?2) []?2` instead of the alternate with the two input args
// swapped, because while the latter is more common with languages that support
// partial function application, the former variant that we implemented is much
// more readable when using an inline lambda.
// TODO: should we extend this to support iterating over map's and structs, or
// should that be a different function? I think a different function is best.
type MapFunc struct {
	Type  *types.Type // this is the type of the elements in our input list
	RType *types.Type // this is the type of the elements in our output list

	init *interfaces.Init
	last types.Value // last value received to use for diff

	lastFuncValue       *full.FuncValue // remember the last function value
	lastInputListLength int             // remember the last input list length

	inputListType  *types.Type
	outputListType *types.Type

	// outputChan is an initially-nil channel from which we receive output
	// lists from the subgraph. This channel is reset when the subgraph is
	// recreated.
	outputChan chan types.Value
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *MapFunc) String() string {
	return MapFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *MapFunc) ArgGen(index int) (string, error) {
	seq := []string{mapArgNameInputs, mapArgNameFunction} // inverted for pretty!
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *MapFunc) sig() *types.Type {
	// func(inputs []?1, function func(?1) ?2) []?2
	tIi := "?1"
	if obj.Type != nil {
		tIi = obj.Type.String()
	}
	tI := fmt.Sprintf("[]%s", tIi) // type of 1st arg

	tOi := "?2"
	if obj.RType != nil {
		tOi = obj.RType.String()
	}
	tO := fmt.Sprintf("[]%s", tOi) // return type

	// type of 2nd arg (the function)
	tF := fmt.Sprintf("func(%s %s) %s", mapArgNameArgName, tIi, tOi)

	s := fmt.Sprintf("func(%s %s, %s %s) %s", mapArgNameInputs, tI, mapArgNameFunction, tF, tO)
	return types.NewType(s) // yay!
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *MapFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the map needs exactly two args")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("the map is nil")
	}

	tInputs, exists := typ.Map[typ.Ord[0]]
	if !exists || tInputs == nil {
		return nil, fmt.Errorf("first argument was missing")
	}
	tFunction, exists := typ.Map[typ.Ord[1]]
	if !exists || tFunction == nil {
		return nil, fmt.Errorf("second argument was missing")
	}

	if tInputs.Kind != types.KindList {
		return nil, fmt.Errorf("first argument must be of kind list")
	}
	if tFunction.Kind != types.KindFunc {
		return nil, fmt.Errorf("second argument must be of kind func")
	}

	if typ.Out == nil {
		return nil, fmt.Errorf("return type must be specified")
	}
	if typ.Out.Kind != types.KindList {
		return nil, fmt.Errorf("return argument must be a list")
	}

	if len(tFunction.Ord) != 1 {
		return nil, fmt.Errorf("the functions map needs exactly one arg")
	}
	if tFunction.Map == nil {
		return nil, fmt.Errorf("the functions map is nil")
	}
	tArg, exists := tFunction.Map[tFunction.Ord[0]]
	if !exists || tArg == nil {
		return nil, fmt.Errorf("the functions first argument was missing")
	}
	if err := tArg.Cmp(tInputs.Val); err != nil {
		return nil, errwrap.Wrapf(err, "the functions arg type must match the input list contents type")
	}

	if tFunction.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if err := tFunction.Out.Cmp(typ.Out.Val); err != nil {
		return nil, errwrap.Wrapf(err, "return type of function must match returned list contents type")
	}

	obj.Type = tInputs.Val    // or tArg
	obj.RType = tFunction.Out // or typ.Out.Val

	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *MapFunc) Validate() error {
	if obj.Type == nil || obj.RType == nil {
		return fmt.Errorf("type is not yet known")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *MapFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // XXX: what if the input function isn't pure?
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *MapFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.lastFuncValue = nil
	obj.lastInputListLength = -1

	obj.inputListType = types.NewType(fmt.Sprintf("[]%s", obj.Type))
	obj.outputListType = types.NewType(fmt.Sprintf("[]%s", obj.RType))

	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *MapFunc) Stream(ctx context.Context) error {
	// Every time the FuncValue or the length of the list changes, recreate the
	// subgraph, by calling the FuncValue N times on N nodes, each of which
	// extracts one of the N values in the list.

	defer close(obj.init.Output) // the sender closes

	// A Func to send input lists to the subgraph. The Txn.Erase() call ensures
	// that this Func is not removed when the subgraph is recreated, so that the
	// function graph can propagate the last list we received to the subgraph.
	inputChan := make(chan types.Value)
	subgraphInput := &structs.ChannelBasedSourceFunc{
		Name:   "subgraphInput",
		Source: obj,
		Chan:   inputChan,
		Type:   obj.inputListType,
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

	obj.outputChan = nil

	canReceiveMoreFuncValuesOrInputLists := true
	canReceiveMoreOutputLists := true
	for {

		if !canReceiveMoreFuncValuesOrInputLists && !canReceiveMoreOutputLists {
			//break
			return nil
		}

		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // block looping back here
				canReceiveMoreFuncValuesOrInputLists = false
				continue
			}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			value, exists := input.Struct()[mapArgNameFunction]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			newFuncValue, ok := value.(*full.FuncValue)
			if !ok {
				return fmt.Errorf("programming error, can't convert to *FuncValue")
			}

			newInputList, exists := input.Struct()[mapArgNameInputs]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			// If we have a new function or the length of the input
			// list has changed, then we need to replace the
			// subgraph with a new one that uses the new function
			// the correct number of times.

			// It's important to have this compare step to avoid
			// redundant graph replacements which slow things down,
			// but also cause the engine to lock, which can preempt
			// the process scheduler, which can cause duplicate or
			// unnecessary re-sending of values here, which causes
			// the whole process to repeat ad-nauseum.
			n := len(newInputList.List())
			if newFuncValue != obj.lastFuncValue || n != obj.lastInputListLength {
				obj.lastFuncValue = newFuncValue
				obj.lastInputListLength = n
				// replaceSubGraph uses the above two values
				if err := obj.replaceSubGraph(subgraphInput); err != nil {
					return errwrap.Wrapf(err, "could not replace subgraph")
				}
				canReceiveMoreOutputLists = true
			}

			// send the new input list to the subgraph
			select {
			case inputChan <- newInputList:
			case <-ctx.Done():
				return nil
			}

		case outputList, ok := <-obj.outputChan:
			// send the new output list downstream
			if !ok {
				obj.outputChan = nil
				canReceiveMoreOutputLists = false
				continue
			}

			select {
			case obj.init.Output <- outputList:
			case <-ctx.Done():
				return nil
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (obj *MapFunc) replaceSubGraph(subgraphInput interfaces.Func) error {
	// Create a subgraph which splits the input list into 'n' nodes, applies
	// 'newFuncValue' to each, then combines the 'n' outputs back into a
	// list.
	//
	// Here is what the subgraph looks like:
	//
	// digraph {
	//	"subgraphInput" -> "inputElemFunc0"
	//	"subgraphInput" -> "inputElemFunc1"
	//	"subgraphInput" -> "inputElemFunc2"
	//
	//	"inputElemFunc0" -> "outputElemFunc0"
	//	"inputElemFunc1" -> "outputElemFunc1"
	//	"inputElemFunc2" -> "outputElemFunc2"
	//
	//	"outputElem0" -> "outputListFunc"
	//	"outputElem1" -> "outputListFunc"
	//	"outputElem2" -> "outputListFunc"
	//
	//	"outputListFunc" -> "mapSubgraphOutput"
	// }

	const channelBasedSinkFuncArgNameEdgeName = structs.ChannelBasedSinkFuncArgName // XXX: not sure if the specific name matters.

	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	// create the new subgraph

	obj.outputChan = make(chan types.Value)
	subgraphOutput := &structs.ChannelBasedSinkFunc{
		Name:     "mapSubgraphOutput",
		Target:   obj,
		EdgeName: channelBasedSinkFuncArgNameEdgeName,
		Chan:     obj.outputChan,
		Type:     obj.outputListType,
	}
	obj.init.Txn.AddVertex(subgraphOutput)

	m := make(map[string]*types.Type)
	ord := []string{}
	for i := 0; i < obj.lastInputListLength; i++ {
		argName := fmt.Sprintf("outputElem%d", i)
		m[argName] = obj.RType
		ord = append(ord, argName)
	}
	typ := &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  obj.outputListType,
	}
	outputListFunc := structs.SimpleFnToDirectFunc(
		"mapOutputList",
		&types.FuncValue{
			V: func(_ context.Context, args []types.Value) (types.Value, error) {
				listValue := &types.ListValue{
					V: args,
					T: obj.outputListType,
				}

				return listValue, nil
			},
			T: typ,
		},
	)

	obj.init.Txn.AddVertex(outputListFunc)
	obj.init.Txn.AddEdge(outputListFunc, subgraphOutput, &interfaces.FuncEdge{
		Args: []string{channelBasedSinkFuncArgNameEdgeName},
	})

	for i := 0; i < obj.lastInputListLength; i++ {
		i := i
		inputElemFunc := structs.SimpleFnToDirectFunc(
			fmt.Sprintf("mapInputElem[%d]", i),
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
				T: types.NewType(fmt.Sprintf("func(inputList %s) %s", obj.inputListType, obj.Type)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFunc)

		outputElemFunc, err := obj.lastFuncValue.CallWithFuncs(obj.init.Txn, []interfaces.Func{inputElemFunc})
		if err != nil {
			return errwrap.Wrapf(err, "could not call obj.lastFuncValue.CallWithFuncs()")
		}

		obj.init.Txn.AddEdge(subgraphInput, inputElemFunc, &interfaces.FuncEdge{
			Args: []string{"inputList"},
		})
		obj.init.Txn.AddEdge(outputElemFunc, outputListFunc, &interfaces.FuncEdge{
			Args: []string{fmt.Sprintf("outputElem%d", i)},
		})
	}

	return obj.init.Txn.Commit()
}

// Copy is implemented so that the type values are not lost if we copy this
// function.
func (obj *MapFunc) Copy() interfaces.Func {
	return &MapFunc{
		Type:  obj.Type,  // don't copy because we use this after unification
		RType: obj.RType, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}
