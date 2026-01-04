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

	argFuncs   []interfaces.Func
	outputFunc interfaces.Func
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
//
// NOTE: The expression signature is shown here, but the actual "signature" of
// this in the function graph returns the "dummy" value because we do the same
// this that we do with ExprCall for example. That means that this function is
// one of very few where the actual expr signature is different from the func!
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
	if err := tFunction.Out.Cmp(types.TypeBool); err != nil {
		return nil, errwrap.Wrapf(err, "return type of function must be a bool")
	}

	// TODO: Do we need to be extra careful and check that this matches?
	// unificationUtil.UnifyCmp(typ, obj.sig()) != nil {}

	obj.Type = typ.Out.Val // extract list type

	return obj.sig(), nil
}

// SetShape tells the function about some special graph engine pointers.
func (obj *FilterFunc) SetShape(argFuncs []interfaces.Func, outputFunc interfaces.Func) {
	obj.argFuncs = argFuncs
	obj.outputFunc = outputFunc
}

// Validate tells us if the input struct takes a valid form.
func (obj *FilterFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("type is not yet known")
	}

	if obj.argFuncs == nil || obj.outputFunc == nil {
		return fmt.Errorf("function did not receive shape information")
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
		Spec: false,     // must be false with the current graph shape code
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

	// We pack the value pairs into structs that look like this...
	structType := types.NewType(fmt.Sprintf("struct{v %s; b bool}", obj.Type.String()))
	getArgName := func(i int) string {
		return fmt.Sprintf("outputElem%d", i)
	}
	argNameInputList := "inputList"

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

	edge := &interfaces.FuncEdge{Args: []string{structs.OutputFuncArgName}} // "out"
	obj.init.Txn.AddVertex(outputListFunc)
	obj.init.Txn.AddEdge(outputListFunc, funcSubgraphOutput, edge)

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
					valuesList := list.List()
					if l := len(valuesList); i >= l {
						// programming error?
						return nil, fmt.Errorf("index %d out of range with length %d", i, l)
					}
					return valuesList[i], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s) %s", argNameInputList, obj.listType, obj.Type)),
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

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *FilterFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
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
func (obj *FilterFunc) Cleanup(ctx context.Context) error {
	obj.init.Txn.Reverse()
	//obj.init.Txn.DeleteVertex(subgraphInput) // XXX: should we delete it?
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
