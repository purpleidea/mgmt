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
	// ZipFuncName is the name this function is registered as.
	ZipFuncName = "zip"

	// arg names...
	zipArgNameInputs1  = "inputs1"
	zipArgNameInputs2  = "inputs2"
	zipArgNameFunction = "function"

	zipArgNameFunctionArgX = "x" // arg name can vary over time
	zipArgNameFunctionArgY = "y" // arg name can vary over time
)

func init() {
	funcs.ModuleRegister(ModuleName, ZipFuncName, func() interfaces.Func { return &ZipFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &ZipFunc{} // ensure it meets this expectation

// ZipFunc is the standard zip iterator function that walks two input lists in
// lockstep, applying a two-argument function to each pair of elements, and
// returning the resulting list. If the two lists differ in length, the output
// is truncated to the shorter of the two, which matches the behaviour of the
// standard zip in most languages. There is no requirement that the two input
// element types or the output element type match. This implements the signature
// `func(inputs1 []?1, inputs2 []?2, function func(?1, ?2) ?3) []?3` instead of
// the alternate with the input args at the end, because while the latter is
// more common with languages that support partial function application, the
// former variant that we implemented is much more readable when using an inline
// lambda.
type ZipFunc struct {
	interfaces.Textarea

	Type1 *types.Type // element type of the first input list
	Type2 *types.Type // element type of the second input list
	RType *types.Type // element type of the output list

	init  *interfaces.Init
	last1 types.Value // last value received for inputs1, to use for diff
	last2 types.Value // last value received for inputs2, to use for diff

	lastFuncValue *full.FuncValue // remember the last function value
	lastN         int             // remember the last min(len1, len2)

	inputList1Type *types.Type
	inputList2Type *types.Type
	outputListType *types.Type

	argFuncs   []interfaces.Func
	outputFunc interfaces.Func
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ZipFunc) String() string {
	return ZipFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ZipFunc) ArgGen(index int) (string, error) {
	seq := []string{zipArgNameInputs1, zipArgNameInputs2, zipArgNameFunction}
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
func (obj *ZipFunc) sig() *types.Type {
	// func(inputs1 []?1, inputs2 []?2, function func(?1, ?2) ?3) []?3
	t1 := "?1"
	if obj.Type1 != nil {
		t1 = obj.Type1.String()
	}
	t2 := "?2"
	if obj.Type2 != nil {
		t2 = obj.Type2.String()
	}
	tR := "?3"
	if obj.RType != nil {
		tR = obj.RType.String()
	}

	tList1 := fmt.Sprintf("[]%s", t1) // type of 1st arg
	tList2 := fmt.Sprintf("[]%s", t2) // type of 2nd arg
	tOut := fmt.Sprintf("[]%s", tR)   // return type

	// type of 3rd arg (the function)
	tF := fmt.Sprintf("func(%s %s, %s %s) %s", zipArgNameFunctionArgX, t1, zipArgNameFunctionArgY, t2, tR)

	s := fmt.Sprintf("func(%s %s, %s %s, %s %s) %s",
		zipArgNameInputs1, tList1,
		zipArgNameInputs2, tList2,
		zipArgNameFunction, tF,
		tOut,
	)
	return types.NewType(s) // yay!
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ZipFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 3 {
		return nil, fmt.Errorf("the zip needs exactly three args")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("the map is nil")
	}

	tInputs1, exists := typ.Map[typ.Ord[0]]
	if !exists || tInputs1 == nil {
		return nil, fmt.Errorf("first argument was missing")
	}
	tInputs2, exists := typ.Map[typ.Ord[1]]
	if !exists || tInputs2 == nil {
		return nil, fmt.Errorf("second argument was missing")
	}
	tFunction, exists := typ.Map[typ.Ord[2]]
	if !exists || tFunction == nil {
		return nil, fmt.Errorf("third argument was missing")
	}

	if tInputs1.Kind != types.KindList {
		return nil, fmt.Errorf("first argument must be of kind list")
	}
	if tInputs2.Kind != types.KindList {
		return nil, fmt.Errorf("second argument must be of kind list")
	}
	if tFunction.Kind != types.KindFunc {
		return nil, fmt.Errorf("third argument must be of kind func")
	}

	if typ.Out == nil {
		return nil, fmt.Errorf("return type must be specified")
	}
	if typ.Out.Kind != types.KindList {
		return nil, fmt.Errorf("return argument must be a list")
	}

	if len(tFunction.Ord) != 2 {
		return nil, fmt.Errorf("the function needs exactly two args")
	}
	if tFunction.Map == nil {
		return nil, fmt.Errorf("the function map is nil")
	}
	tArg1, exists := tFunction.Map[tFunction.Ord[0]]
	if !exists || tArg1 == nil {
		return nil, fmt.Errorf("the function's first argument was missing")
	}
	tArg2, exists := tFunction.Map[tFunction.Ord[1]]
	if !exists || tArg2 == nil {
		return nil, fmt.Errorf("the function's second argument was missing")
	}
	if err := tArg1.Cmp(tInputs1.Val); err != nil {
		return nil, errwrap.Wrapf(err, "the function's first arg type must match the first input list contents type")
	}
	if err := tArg2.Cmp(tInputs2.Val); err != nil {
		return nil, errwrap.Wrapf(err, "the function's second arg type must match the second input list contents type")
	}

	if tFunction.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if err := tFunction.Out.Cmp(typ.Out.Val); err != nil {
		return nil, errwrap.Wrapf(err, "return type of function must match returned list contents type")
	}

	// TODO: Do we need to be extra careful and check that this matches?
	// unificationUtil.UnifyCmp(typ, obj.sig()) != nil {}

	obj.Type1 = tInputs1.Val  // or tArg1
	obj.Type2 = tInputs2.Val  // or tArg2
	obj.RType = tFunction.Out // or typ.Out.Val

	return obj.sig(), nil
}

// SetShape tells the function about some special graph engine pointers.
func (obj *ZipFunc) SetShape(argFuncs []interfaces.Func, outputFunc interfaces.Func) {
	obj.argFuncs = argFuncs
	obj.outputFunc = outputFunc
}

// Validate tells us if the input struct takes a valid form.
func (obj *ZipFunc) Validate() error {
	if obj.Type1 == nil || obj.Type2 == nil || obj.RType == nil {
		return fmt.Errorf("type is not yet known")
	}

	if obj.argFuncs == nil || obj.outputFunc == nil {
		return fmt.Errorf("function did not receive shape information")
	}

	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ZipFunc) Info() *interfaces.Info {
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
func (obj *ZipFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.lastFuncValue = nil
	obj.lastN = -1

	obj.inputList1Type = types.NewType(fmt.Sprintf("[]%s", obj.Type1))
	obj.inputList2Type = types.NewType(fmt.Sprintf("[]%s", obj.Type2))
	obj.outputListType = types.NewType(fmt.Sprintf("[]%s", obj.RType))

	return nil
}

func (obj *ZipFunc) replaceSubGraph(subgraphInputs1, subgraphInputs2 interfaces.Func) error {
	// Build a subgraph that splits each of the two input lists into 'n'
	// per-index extractor nodes (where 'n' is the lesser of the two list
	// lengths), pairs them up by index through 'newFuncValue', and then
	// stitches the 'n' results back into one output list.
	//
	// Each per-index extractor is gated on the parent's dummy output (the
	// "zip -> ..." edges) so that, on a list shrink, this function gets a
	// chance to rebuild the subgraph before any stale extractor reads off
	// the end of the new shorter list.
	//
	// Here is what the subgraph looks like for n == 3:
	//
	// digraph {
	//	"subgraphInputs1" -> "zipInputElem1[0]"
	//	"subgraphInputs1" -> "zipInputElem1[1]"
	//	"subgraphInputs1" -> "zipInputElem1[2]"
	//
	//	"subgraphInputs2" -> "zipInputElem2[0]"
	//	"subgraphInputs2" -> "zipInputElem2[1]"
	//	"subgraphInputs2" -> "zipInputElem2[2]"
	//
	//	"zip" -> "zipInputElem1[0]" # dummy
	//	"zip" -> "zipInputElem1[1]" # dummy
	//	"zip" -> "zipInputElem1[2]" # dummy
	//	"zip" -> "zipInputElem2[0]" # dummy
	//	"zip" -> "zipInputElem2[1]" # dummy
	//	"zip" -> "zipInputElem2[2]" # dummy
	//
	//	"zipInputElem1[0]" -> "fn0" # x
	//	"zipInputElem2[0]" -> "fn0" # y
	//	"zipInputElem1[1]" -> "fn1" # x
	//	"zipInputElem2[1]" -> "fn1" # y
	//	"zipInputElem1[2]" -> "fn2" # x
	//	"zipInputElem2[2]" -> "fn2" # y
	//
	//	"fn0" -> "outputListFunc" # outputElem0
	//	"fn1" -> "outputListFunc" # outputElem1
	//	"fn2" -> "outputListFunc" # outputElem2
	//
	//	"outputListFunc" -> "subgraphOutput"
	// }

	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	// create the new subgraph

	argNameInputList := "inputList"
	argNameInputDummy := structs.OutputFuncDummyArgName

	m := make(map[string]*types.Type)
	ord := []string{}
	for i := 0; i < obj.lastN; i++ {
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
		"zipOutputList",
		&types.FuncValue{
			V: func(_ context.Context, args []types.Value) (types.Value, error) {
				return &types.ListValue{
					V: args,
					T: obj.outputListType,
				}, nil
			},
			T: typ,
		},
	)

	edge := &interfaces.FuncEdge{Args: []string{structs.OutputFuncArgName}} // "out"
	obj.init.Txn.AddVertex(outputListFunc)
	obj.init.Txn.AddEdge(outputListFunc, obj.outputFunc, edge)

	for i := 0; i < obj.lastN; i++ {
		inputElem1Func := structs.SimpleFnToDirectFunc(
			fmt.Sprintf("zipInputElem1[%d]", i),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 2 {
						return nil, fmt.Errorf("inputElem1Func: expected two arguments")
					}
					list, ok := args[0].(*types.ListValue)
					if !ok {
						return nil, fmt.Errorf("inputElem1Func: expected a ListValue argument")
					}
					valuesList := list.List()
					if l := len(valuesList); i >= l {
						return nil, fmt.Errorf("index %d out of range with length %d", i, l)
					}
					return valuesList[i], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s, %s nil) %s", argNameInputList, obj.inputList1Type, argNameInputDummy, obj.Type1)),
			},
		)
		obj.init.Txn.AddVertex(inputElem1Func)

		inputElem2Func := structs.SimpleFnToDirectFunc(
			fmt.Sprintf("zipInputElem2[%d]", i),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 2 {
						return nil, fmt.Errorf("inputElem2Func: expected two arguments")
					}
					list, ok := args[0].(*types.ListValue)
					if !ok {
						return nil, fmt.Errorf("inputElem2Func: expected a ListValue argument")
					}
					valuesList := list.List()
					if l := len(valuesList); i >= l {
						return nil, fmt.Errorf("index %d out of range with length %d", i, l)
					}
					return valuesList[i], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s, %s nil) %s", argNameInputList, obj.inputList2Type, argNameInputDummy, obj.Type2)),
			},
		)
		obj.init.Txn.AddVertex(inputElem2Func)

		outputElemFunc, err := obj.lastFuncValue.CallWithFuncs(obj.init.Txn, []interfaces.Func{inputElem1Func, inputElem2Func})
		if err != nil {
			return errwrap.Wrapf(err, "could not call obj.lastFuncValue.CallWithFuncs()")
		}

		obj.init.Txn.AddEdge(subgraphInputs1, inputElem1Func, &interfaces.FuncEdge{
			Args: []string{argNameInputList},
		})
		obj.init.Txn.AddEdge(obj, inputElem1Func, &interfaces.FuncEdge{
			Args: []string{argNameInputDummy},
		})

		obj.init.Txn.AddEdge(subgraphInputs2, inputElem2Func, &interfaces.FuncEdge{
			Args: []string{argNameInputList},
		})
		obj.init.Txn.AddEdge(obj, inputElem2Func, &interfaces.FuncEdge{
			Args: []string{argNameInputDummy},
		})

		obj.init.Txn.AddEdge(outputElemFunc, outputListFunc, &interfaces.FuncEdge{
			Args: []string{fmt.Sprintf("outputElem%d", i)},
		})
	}

	return obj.init.Txn.Commit()
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ZipFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("not enough args")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	// Need this before we can *really* run this properly.
	if len(obj.argFuncs) != 3 {
		return nil, funcs.ErrCantSpeculate
		//return nil, fmt.Errorf("unexpected input arg length")
	}

	newInputList1 := args[0]
	newInputList2 := args[1]
	value := args[2]
	newFuncValue, ok := value.(*full.FuncValue)
	if !ok {
		return nil, fmt.Errorf("programming error, can't convert to *FuncValue")
	}

	a1 := obj.last1 != nil && newInputList1.Cmp(obj.last1) == nil
	a2 := obj.last2 != nil && newInputList2.Cmp(obj.last2) == nil
	b := obj.lastFuncValue != nil && newFuncValue == obj.lastFuncValue
	if a1 && a2 && b {
		return types.NewNil(), nil // dummy value
	}
	obj.last1 = newInputList1
	obj.last2 = newInputList2
	obj.lastFuncValue = newFuncValue

	// Every time the FuncValue or the effective length (min of the two
	// input list lengths) changes, recreate the subgraph. If only the
	// contents change but the effective length stays the same, the existing
	// graph is correct: the per-index extractor nodes will pick up the new
	// contents and re-run the chain.

	n1 := len(newInputList1.List())
	n2 := len(newInputList2.List())
	n := n1
	if n2 < n {
		n = n2
	}

	c := n == obj.lastN
	if b && c {
		return types.NewNil(), nil // dummy value
	}
	obj.lastN = n

	// If we have a new function or the effective length has changed, then
	// we need to replace the subgraph with a new one that uses the new
	// function the correct number of times.

	subgraphInputs1 := obj.argFuncs[0]
	subgraphInputs2 := obj.argFuncs[1]

	// replaceSubGraph uses the above values
	if err := obj.replaceSubGraph(subgraphInputs1, subgraphInputs2); err != nil {
		return nil, errwrap.Wrapf(err, "could not replace subgraph")
	}

	return nil, interfaces.ErrInterrupt
}

// Cleanup runs after that function was removed from the graph.
func (obj *ZipFunc) Cleanup(ctx context.Context) error {
	if err := obj.init.Txn.Reverse(); err != nil {
		return err
	}
	return obj.init.Txn.Commit()
}

// Copy is implemented so that the type values are not lost if we copy this
// function.
func (obj *ZipFunc) Copy() interfaces.Func {
	return &ZipFunc{
		Textarea: obj.Textarea,

		Type1: obj.Type1, // don't copy because we use this after unification
		Type2: obj.Type2, // don't copy because we use this after unification
		RType: obj.RType, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}
