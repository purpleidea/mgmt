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
	// FilterFuncName is the name this function is registered as.
	FilterFuncName = "filter"

	// arg names...
	filterArgNameInputs   = "inputs"
	filterArgNameFunction = "function"

	filterArgNameArgName = "name-which-can-vary-over-time" // XXX: weird but ok
)

func init() {
	funcs.ModuleRegister(ModuleName, FilterFuncName, func() interfaces.Func { return &FilterFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &FilterFunc{} // ensure it meets this expectation

// FilterFunc is the standard filter iterator function that runs a function on
// each element in a list. That function must return true to keep the element,
// or false otherwise. The function then returns a list with the subset of kept
// elements from the input list. This implements the signature:
// `func(inputs []?1, function func(?1) bool) []?1` instead of the alternate
// with the two input args swapped, because while the latter is more common with
// languages that support partial function application, the former variant that
// we implemented is much more readable when using an inline lambda.
type FilterFunc struct {
	Type *types.Type // this is the type of the elements in our input list

	init *interfaces.Init
	last types.Value // last value received to use for diff

	lastFuncValue       *full.FuncValue // remember the last function value
	lastInputListLength int             // remember the last input list length

	listType *types.Type

	// outputChan is an initially-nil channel from which we receive output
	// lists from the subgraph. This channel is reset when the subgraph is
	// recreated.
	outputChan chan types.Value
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *FilterFunc) String() string {
	return FilterFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *FilterFunc) ArgGen(index int) (string, error) {
	seq := []string{filterArgNameInputs, filterArgNameFunction} // inverted for pretty!
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *FilterFunc) sig() *types.Type {
	// func(inputs []?1, function func(?1) bool) []?1
	typ := "?1"
	if obj.Type != nil {
		typ = obj.Type.String()
	}
	tList := fmt.Sprintf("[]%s", typ) // type of 1st arg

	// type of 2nd arg (the function)
	tF := fmt.Sprintf("func(%s %s) bool", filterArgNameArgName, typ)

	s := fmt.Sprintf("func(%s %s, %s %s) %s", filterArgNameInputs, tList, filterArgNameFunction, tF, tList)
	return types.NewType(s) // yay!
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *FilterFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...

	// TODO: Do we need to be extra careful and check that this matches?
	// unificationUtil.UnifyCmp(typ, obj.sig()) != nil {}

	obj.Type = typ.Out.Val // extract list type

	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *FilterFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("type is not yet known")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *FilterFunc) Info() *interfaces.Info {
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
func (obj *FilterFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.lastFuncValue = nil
	obj.lastInputListLength = -1

	obj.listType = types.NewType(fmt.Sprintf("[]%s", obj.Type))

	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *FilterFunc) Stream(ctx context.Context) error {
	// Every time the FuncValue or the length of the list changes, recreate
	// the subgraph, by calling the FuncValue N times on N nodes, each of
	// which extracts one of the N values in the list.

	defer close(obj.init.Output) // the sender closes

	// A Func to send input lists to the subgraph. The Txn.Erase() call
	// ensures that this Func is not removed when the subgraph is recreated,
	// so that the function graph can propagate the last list we received to
	// the subgraph.
	inputChan := make(chan types.Value)
	subgraphInput := &structs.ChannelBasedSourceFunc{
		Name:   "subgraphInput",
		Source: obj,
		Chan:   inputChan,
		Type:   obj.listType,
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

			value, exists := input.Struct()[filterArgNameFunction]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			newFuncValue, ok := value.(*full.FuncValue)
			if !ok {
				return fmt.Errorf("programming error, can't convert to *FuncValue")
			}

			newInputList, exists := input.Struct()[filterArgNameInputs]
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

func (obj *FilterFunc) replaceSubGraph(subgraphInput interfaces.Func) error {
	// replaceSubGraph creates a subgraph which first splits the input list
	// into 'n' nodes. Then it applies 'newFuncValue' to each, and sends
	// that value (a bool) along with that input value to a function which
	// combines the values into a struct. One struct (for each input list
	// index) gets passed to the filterOutputList function which then
	// combines the 'n' outputs which have a corresponding `true` value for
	// the 'b' key, back into a filtered list. That combiner struct also has
	// a 'v' key with the original value so we can put it back into the
	// output list if selected.
	//
	// Here is what the subgraph looks like:
	//
	// digraph {
	//	"subgraphInput" -> "filterInputElem0"
	//	"subgraphInput" -> "filterInputElem1"
	//	"subgraphInput" -> "filterInputElem2"
	//
	//	"filterInputElem0" -> "outputElemFunc0"
	//	"filterInputElem1" -> "outputElemFunc1"
	//	"filterInputElem2" -> "outputElemFunc2"
	//
	//	"filterInputElem0" -> "filterCombineList0"
	//	"filterInputElem1" -> "filterCombineList1"
	//	"filterInputElem2" -> "filterCombineList2"
	//
	//	"outputElemFunc0" -> "filterCombineList0"
	//	"outputElemFunc1" -> "filterCombineList1"
	//	"outputElemFunc2" -> "filterCombineList2"
	//
	//	"filterCombineList0" -> "outputListFunc"
	//	"filterCombineList1" -> "outputListFunc"
	//	"filterCombineList2" -> "outputListFunc"
	//	"outputListFunc" -> "subgraphOutput"
	// }
	const channelBasedSinkFuncArgNameEdgeName = structs.ChannelBasedSinkFuncArgName // XXX: not sure if the specific name matters.

	// We pack the value pairs into structs that look like this...
	structType := types.NewType(fmt.Sprintf("struct{v %s; b bool}", obj.Type.String()))
	getArgName := func(i int) string {
		return fmt.Sprintf("outputElem%d", i)
	}
	argNameInputList := "inputList"

	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	// create the new subgraph
	obj.outputChan = make(chan types.Value)
	subgraphOutput := &structs.ChannelBasedSinkFunc{
		Name:     "subgraphOutput",
		Target:   obj,
		EdgeName: channelBasedSinkFuncArgNameEdgeName,
		Chan:     obj.outputChan,
		Type:     obj.listType,
	}
	obj.init.Txn.AddVertex(subgraphOutput)

	m := make(map[string]*types.Type)
	ord := []string{}
	for i := 0; i < obj.lastInputListLength; i++ {
		argName := getArgName(i)
		m[argName] = structType // each arg is a struct of value and bool
		ord = append(ord, argName)
	}
	outTyp := &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  obj.listType,
	}

	outputListFunc := structs.SimpleFnToDirectFunc(
		"filterOutputList",
		&types.FuncValue{
			V: func(_ context.Context, args []types.Value) (types.Value, error) {
				newValues := []types.Value{}
				for _, arg := range args {
					st := arg.Struct() // map[string]types.Value
					b, exists := st["b"]
					if !exists {
						return nil, fmt.Errorf("missing struct field")
					}
					v, exists := st["v"]
					if !exists {
						return nil, fmt.Errorf("missing struct field")
					}
					if !b.Bool() { // filtered out!
						continue
					}
					newValues = append(newValues, v)
				}

				return &types.ListValue{
					V: newValues,
					T: obj.listType, // output list type
				}, nil
			},
			T: outTyp,
		},
	)

	obj.init.Txn.AddVertex(outputListFunc)
	obj.init.Txn.AddEdge(outputListFunc, subgraphOutput, &interfaces.FuncEdge{
		Args: []string{channelBasedSinkFuncArgNameEdgeName},
	})

	for i := 0; i < obj.lastInputListLength; i++ {
		i := i
		inputElemFunc := structs.SimpleFnToDirectFunc(
			fmt.Sprintf("filterInputElem[%d]", i),
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

					// Extract the correct list element.
					return list.List()[i], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s) %s", argNameInputList, obj.listType, obj.Type)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFunc)

		outputElemFunc, err := obj.lastFuncValue.CallWithFuncs(obj.init.Txn, []interfaces.Func{inputElemFunc})
		if err != nil {
			return errwrap.Wrapf(err, "could not call obj.lastFuncValue.CallWithFuncs()")
		}

		obj.init.Txn.AddEdge(subgraphInput, inputElemFunc, &interfaces.FuncEdge{
			Args: []string{argNameInputList},
		})

		combinerValueElem := fmt.Sprintf("combinerValueElem%d", i)
		combinerBoolElem := fmt.Sprintf("combinerBoolElem%d", i)
		combinerTyp := types.NewType(fmt.Sprintf("func(%s %s, %s bool) %s", combinerValueElem, obj.Type.String(), combinerBoolElem, structType))

		combinerFunc := structs.SimpleFnToDirectFunc(
			fmt.Sprintf("filterCombineList[%d]", i),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 2 {
						// programming error
						return nil, fmt.Errorf("expected two args")
					}
					return &types.StructValue{
						T: structType,
						V: map[string]types.Value{
							"v": args[0],
							"b": args[1],
						},
					}, nil
				},
				T: combinerTyp,
			},
		)

		obj.init.Txn.AddEdge(inputElemFunc, combinerFunc, &interfaces.FuncEdge{
			Args: []string{combinerValueElem},
		})
		obj.init.Txn.AddEdge(outputElemFunc, combinerFunc, &interfaces.FuncEdge{
			Args: []string{combinerBoolElem},
		})

		obj.init.Txn.AddEdge(combinerFunc, outputListFunc, &interfaces.FuncEdge{
			Args: []string{getArgName(i)},
		})
	}

	return obj.init.Txn.Commit()
}

// Copy is implemented so that the type value is not lost if we copy this
// function.
func (obj *FilterFunc) Copy() interfaces.Func {
	return &FilterFunc{
		Type: obj.Type, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}
