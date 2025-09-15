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

	argFuncs   []interfaces.Func
	outputFunc interfaces.Func
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
//
// NOTE: The expression signature is shown here, but the actual "signature" of
// this in the function graph returns the "dummy" value because we do the same
// this that we do with ExprCall for example. That means that this function is
// one of very few where the actual expr signature is different from the func!
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

	// TODO: Do we need to be extra careful and check that this matches?
	// unificationUtil.UnifyCmp(typ, obj.sig()) != nil {}

	obj.Type = tInputs.Val    // or tArg
	obj.RType = tFunction.Out // or typ.Out.Val

	return obj.sig(), nil
}

// SetShape tells the function about some special graph engine pointers.
func (obj *MapFunc) SetShape(argFuncs []interfaces.Func, outputFunc interfaces.Func) {
	obj.argFuncs = argFuncs
	obj.outputFunc = outputFunc
}

// Validate tells us if the input struct takes a valid form.
func (obj *MapFunc) Validate() error {
	if obj.Type == nil || obj.RType == nil {
		return fmt.Errorf("type is not yet known")
	}

	if obj.argFuncs == nil || obj.outputFunc == nil {
		return fmt.Errorf("function did not receive shape information")
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
		Spec: false,     // must be false with the current graph shape code
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
	//	"outputListFunc" -> "funcSubgraphOutput"
	// }

	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	// create the new subgraph

	// XXX: Should we move creation of funcSubgraphOutput into Init() ?
	funcSubgraphOutput := &structs.OutputFunc{ // the new graph shape thing!
		//Textarea: obj.Textarea,
		Name:     "funcSubgraphOutput",
		Type:     obj.sig().Out,
		EdgeName: structs.OutputFuncArgName,
	}
	obj.init.Txn.AddVertex(funcSubgraphOutput)
	obj.init.Txn.AddEdge(funcSubgraphOutput, obj.outputFunc, &interfaces.FuncEdge{Args: []string{structs.OutputFuncArgName}}) // "out"

	// XXX: hack add this edge that I thought would happen in call.go
	obj.init.Txn.AddEdge(obj, funcSubgraphOutput, &interfaces.FuncEdge{Args: []string{structs.OutputFuncDummyArgName}}) // "dummy"

	argNameInputList := "inputList"

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

	edge := &interfaces.FuncEdge{Args: []string{structs.OutputFuncArgName}} // "out"
	obj.init.Txn.AddVertex(outputListFunc)
	obj.init.Txn.AddEdge(outputListFunc, funcSubgraphOutput, edge)

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

					// Extract the correct list element.
					return list.List()[i], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s) %s", argNameInputList, obj.inputListType, obj.Type)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFunc)

		outputElemFunc, err := obj.lastFuncValue.CallWithFuncs(obj.init.Txn, []interfaces.Func{inputElemFunc}, funcSubgraphOutput)
		if err != nil {
			return errwrap.Wrapf(err, "could not call obj.lastFuncValue.CallWithFuncs()")
		}

		obj.init.Txn.AddEdge(subgraphInput, inputElemFunc, &interfaces.FuncEdge{
			Args: []string{argNameInputList},
		})
		obj.init.Txn.AddEdge(outputElemFunc, outputListFunc, &interfaces.FuncEdge{
			Args: []string{fmt.Sprintf("outputElem%d", i)},
		})
	}

	return obj.init.Txn.Commit()
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *MapFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("not enough args")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	// Need this before we can *really* run this properly.
	if len(obj.argFuncs) != 2 {
		return nil, funcs.ErrCantSpeculate
		//return nil, fmt.Errorf("unexpected input arg length")
	}

	newInputList := args[0]
	value := args[1]
	newFuncValue, ok := value.(*full.FuncValue)
	if !ok {
		return nil, fmt.Errorf("programming error, can't convert to *FuncValue")
	}

	a := obj.last != nil && newInputList.Cmp(obj.last) == nil
	b := obj.lastFuncValue != nil && newFuncValue == obj.lastFuncValue
	if a && b {
		return types.NewNil(), nil // dummy value
	}
	obj.last = newInputList // store for next
	obj.lastFuncValue = newFuncValue

	// Every time the FuncValue or the length of the list changes, recreate
	// the subgraph, by calling the FuncValue N times on N nodes, each of
	// which extracts one of the N values in the list. If the contents of
	// the list change (BUT NOT THE LENGTH) then it's okay to use the
	// existing graph, because the shape is the same!

	n := len(newInputList.List())

	c := n == obj.lastInputListLength
	if b && c {
		return types.NewNil(), nil // dummy value
	}
	obj.lastInputListLength = n

	// If we have a new function or the length of the input list has
	// changed, then we need to replace the subgraph with a new one that
	// uses the new function the correct number of times.

	subgraphInput := obj.argFuncs[0]

	// replaceSubGraph uses the above two values
	if err := obj.replaceSubGraph(subgraphInput); err != nil {
		return nil, errwrap.Wrapf(err, "could not replace subgraph")
	}

	return nil, interfaces.ErrInterrupt
}

// Cleanup runs after that function was removed from the graph.
func (obj *MapFunc) Cleanup(ctx context.Context) error {
	obj.init.Txn.Reverse()
	//obj.init.Txn.DeleteVertex(subgraphInput) // XXX: should we delete it?
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
