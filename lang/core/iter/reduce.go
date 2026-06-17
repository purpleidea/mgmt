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
	// ReduceFuncName is the name this function is registered as.
	ReduceFuncName = "reduce"

	// arg names...
	reduceArgNameInitval  = "initval"
	reduceArgNameInputs   = "inputs"
	reduceArgNameFunction = "function"

	reduceArgNameFunctionArgX = "x" // arg name can vary over time
	reduceArgNameFunctionArgY = "y" // arg name can vary over time
)

func init() {
	funcs.ModuleRegister(ModuleName, ReduceFuncName, func() interfaces.Func { return &ReduceFunc{} }) // must register the func and name
}

var _ interfaces.InferableFunc = &ReduceFunc{} // ensure it meets this expectation

// ReduceFunc is the standard reduce iterator function that runs a function on
// the first pair of elements in a list and then runs the function again with
// the output of the first computation and the next element. At the end of this
// chain, the final result is returned. This is the standard "left fold"
// variant, if you want the "right fold" variant, you can invert the input list.
// If the list only has one element, then that is returned directly without
// running the computation. If the input list is empty than one of two
// possibilities exist. If you've used the two argument variant of this
// function, then the function will error. If you've used the three argument
// variant, then the function will return the "initial value" element. This
// value is also used for the first pair-wise function element computation if it
// exists. This function is sometimes known as "fold". This implements the
// signature: `func(inputs []?1, function func(?1, ?1) ?1) ?1` or:
// `func(initval ?1, inputs []?1, function func(?1, ?1) ?1) ?1` instead of the
// alternate scenarios with the arguments in the reverse order, because while
// the latter are more common with languages that support partial function
// application, the former variants that we implemented are much more readable
// when using an inline lambda.
type ReduceFunc struct {
	interfaces.Textarea

	Type *types.Type // this is the type of the elements in our input list

	init *interfaces.Init
	last types.Value // last input list received to use for diff

	lastFuncValue       *full.FuncValue // remember the last function value
	lastInputListLength int             // remember the last input list length

	listType *types.Type
	three    *bool // true means 3 args, false means 2, nil means unbuilt

	argFuncs   []interfaces.Func
	outputFunc interfaces.Func
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ReduceFunc) String() string {
	return ReduceFuncName
}

// helper
func (obj *ReduceFunc) sig(count ...int) *types.Type {
	if len(count) == 0 && obj.three == nil { // we don't know which to build yet
		return nil
	}
	three := false // size three?
	if obj.three != nil {
		three = *obj.three
	} else if count[0] == 3 {
		three = true
	}

	// For the 2 arg version, if length of inputs == 0, we error.
	// func(inputs []?1, function func(?1, ?1) ?1) ?1

	// For the 3 arg version, if length of inputs == 0, we return initval.
	// func(initval ?1, inputs []?1, function func(?1, ?1) ?1) ?1

	typ := "?1"
	if obj.Type != nil {
		typ = obj.Type.String()
	}

	// type of the function
	tF := fmt.Sprintf("func(%s %s, %s %s) %s", reduceArgNameFunctionArgX, typ, reduceArgNameFunctionArgY, typ, typ)

	// 3 args
	s := fmt.Sprintf("func(%s %s, %s []%s, %s %s) %s", reduceArgNameInitval, typ, reduceArgNameInputs, typ, reduceArgNameFunction, tF, typ)

	if !three { // 2 args
		s = fmt.Sprintf("func(%s []%s, %s %s) %s", reduceArgNameInputs, typ, reduceArgNameFunction, tF, typ)
	}

	return types.NewType(s) // yay!
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *ReduceFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	l := len(partialType.Map)
	if l < 2 || l > 3 {
		return nil, nil, fmt.Errorf("must have either two or three args")
	}

	return obj.sig(l), []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ReduceFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	l := len(typ.Ord)
	if l != 2 && l != 3 {
		return nil, fmt.Errorf("must have either two or three args")
	}

	// TODO: Do we need to be extra careful and check that this matches?
	// unificationUtil.UnifyCmp(typ, obj.sig()) != nil {}

	obj.Type = typ.Out // extract element type

	b := l == 3
	obj.three = &b // we have the initial value variant

	return obj.sig(l), nil
}

// SetShape tells the function about some special graph engine pointers.
func (obj *ReduceFunc) SetShape(argFuncs []interfaces.Func, outputFunc interfaces.Func) {
	obj.argFuncs = argFuncs
	obj.outputFunc = outputFunc
}

// Validate tells us if the input struct takes a valid form.
func (obj *ReduceFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("type is not yet known")
	}
	if obj.three == nil {
		return fmt.Errorf("variant is not yet known")
	}

	if obj.argFuncs == nil || obj.outputFunc == nil {
		return fmt.Errorf("function did not receive shape information")
	}

	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ReduceFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // TODO: what if the input function isn't pure?
		Memo: false,
		Fast: false,
		Spec: false,     // must be false with the current graph shape code
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ReduceFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.lastFuncValue = nil
	obj.lastInputListLength = -1

	obj.listType = types.NewType(fmt.Sprintf("[]%s", obj.Type))

	return nil
}

func (obj *ReduceFunc) replaceSubGraph(subgraphInputs interfaces.Func, subgraphInitval interfaces.Func) error {
	// Build a chain of function calls that performs a left fold over the
	// input list. Each per-index node extracts one element from the list,
	// and the function value is invoked once per pair, threading the
	// running accumulator through the chain. This shape lets all the
	// per-index reads happen in parallel, while the actual function calls
	// remain a sequential chain.
	//
	// Without an initial value (2-arg variant), with input list
	// [a, b, c, d]:
	//
	// digraph {
	//	"subgraphInputs" -> "reduceInputElem0"
	//	"subgraphInputs" -> "reduceInputElem1"
	//	"subgraphInputs" -> "reduceInputElem2"
	//	"subgraphInputs" -> "reduceInputElem3"
	//
	//	"reduce" -> "reduceInputElem0" # dummy
	//	"reduce" -> "reduceInputElem1" # dummy
	//	"reduce" -> "reduceInputElem2" # dummy
	//	"reduce" -> "reduceInputElem3" # dummy
	//
	//	"reduceInputElem0" -> "fn0" # x
	//	"reduceInputElem1" -> "fn0" # y
	//	"fn0"              -> "fn1" # x
	//	"reduceInputElem2" -> "fn1" # y
	//	"fn1"              -> "fn2" # x
	//	"reduceInputElem3" -> "fn2" # y
	//	"fn2"              -> "subgraphOutput" # out
	// }
	//
	// With an initial value (3-arg variant), the chain starts with initval
	// instead of the first list element:
	//
	//	"subgraphInitval"  -> "fn0" # x
	//	"reduceInputElem0" -> "fn0" # y
	//	"fn0"              -> "fn1" # x
	//	"reduceInputElem1" -> "fn1" # y
	//	...

	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	// create the new subgraph

	argNameInputList := "inputList"
	argNameInputDummy := structs.OutputFuncDummyArgName
	edgeOut := &interfaces.FuncEdge{Args: []string{structs.OutputFuncArgName}} // "out"

	n := obj.lastInputListLength
	three := obj.three != nil && *obj.three

	// 3-arg empty list: pass the initial value straight to the output.
	if three && n == 0 {
		obj.init.Txn.AddEdge(subgraphInitval, obj.outputFunc, edgeOut)
		return obj.init.Txn.Commit()
	}

	// 2-arg empty list: there's no value to return, so emit a runtime
	// error. We can't error from Call directly because Call may run
	// speculatively, and the user-visible failure should come from
	// evaluating the graph.
	if !three && n == 0 {
		errFunc := structs.SimpleFnToDirectFunc(
			"reduceError",
			&types.FuncValue{
				V: func(_ context.Context, _ []types.Value) (types.Value, error) {
					return nil, fmt.Errorf("cannot reduce an empty list without an initial value")
				},
				T: types.NewType(fmt.Sprintf("func() %s", obj.Type.String())),
			},
		)
		obj.init.Txn.AddVertex(errFunc)
		obj.init.Txn.AddEdge(errFunc, obj.outputFunc, edgeOut)
		return obj.init.Txn.Commit()
	}

	// Build the per-index extractor nodes. Each is gated on the parent
	// reduce dummy output so that a list shrink can't race a stale
	// per-index node into reading off the end of the new list.
	inputElemFuncs := make([]interfaces.Func, n)
	for i := 0; i < n; i++ {
		inputElemFunc := structs.SimpleFnToDirectFunc(
			fmt.Sprintf("reduceInputElem[%d]", i),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 2 {
						return nil, fmt.Errorf("inputElemFunc: expected two arguments")
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
				T: types.NewType(fmt.Sprintf("func(%s %s, %s nil) %s", argNameInputList, obj.listType, argNameInputDummy, obj.Type)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFunc)

		obj.init.Txn.AddEdge(subgraphInputs, inputElemFunc, &interfaces.FuncEdge{
			Args: []string{argNameInputList},
		})
		obj.init.Txn.AddEdge(obj, inputElemFunc, &interfaces.FuncEdge{
			Args: []string{argNameInputDummy},
		})

		inputElemFuncs[i] = inputElemFunc
	}

	// Pick the seed of the chain and the index of the first list element
	// that still needs to be folded in.
	var current interfaces.Func
	startIdx := 0
	if three {
		current = subgraphInitval
	} else {
		// 2-arg with at least one element (n == 0 was handled above).
		current = inputElemFuncs[0]
		startIdx = 1
	}

	// Build the chain of pair-wise function calls.
	for i := startIdx; i < n; i++ {
		next, err := obj.lastFuncValue.CallWithFuncs(obj.init.Txn, []interfaces.Func{current, inputElemFuncs[i]})
		if err != nil {
			return errwrap.Wrapf(err, "could not call obj.lastFuncValue.CallWithFuncs()")
		}
		current = next
	}

	// Hook the final value of the chain up to the shared output. For the
	// 2-arg n == 1 and 3-arg n == 0 single-value cases, there's no chain
	// and current points directly at the seed, which is exactly what we
	// want to emit.
	obj.init.Txn.AddEdge(current, obj.outputFunc, edgeOut)

	return obj.init.Txn.Commit()
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ReduceFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.three == nil {
		// programming error
		return nil, fmt.Errorf("not built correctly")
	}
	three := *obj.three

	expected := 2
	if three {
		expected = 3
	}
	if len(args) < expected {
		return nil, fmt.Errorf("not enough args")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	// Need this before we can *really* run this properly.
	if len(obj.argFuncs) != expected {
		return nil, funcs.ErrCantSpeculate
		//return nil, fmt.Errorf("unexpected input arg length")
	}

	var newInputList types.Value
	var fnVal types.Value
	if three {
		// args: initval, inputs, function
		newInputList = args[1]
		fnVal = args[2]
	} else {
		// args: inputs, function
		newInputList = args[0]
		fnVal = args[1]
	}

	newFuncValue, ok := fnVal.(*full.FuncValue)
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
	// the subgraph. If the contents of the list change (BUT NOT THE LENGTH)
	// then it's okay to keep the existing graph: the per-index extractor
	// nodes will pick up the new contents and re-run the chain. Note that
	// changes to the initial value (3-arg variant) also propagate via the
	// subgraphInitval edge without needing a rebuild.

	n := len(newInputList.List())

	c := n == obj.lastInputListLength
	if b && c {
		return types.NewNil(), nil // dummy value
	}
	obj.lastInputListLength = n

	// Resolve which arg func is which. SetShape gave us the upstream Funcs
	// in declaration order: 2-arg is [inputs, function]; 3-arg is
	// [initval, inputs, function].
	var subgraphInputs interfaces.Func
	var subgraphInitval interfaces.Func
	if three {
		subgraphInitval = obj.argFuncs[0]
		subgraphInputs = obj.argFuncs[1]
	} else {
		subgraphInputs = obj.argFuncs[0]
	}

	// replaceSubGraph uses the above values
	if err := obj.replaceSubGraph(subgraphInputs, subgraphInitval); err != nil {
		return nil, errwrap.Wrapf(err, "could not replace subgraph")
	}

	return nil, interfaces.ErrInterrupt
}

// Cleanup runs after that function was removed from the graph.
func (obj *ReduceFunc) Cleanup(ctx context.Context) error {
	if err := obj.init.Txn.Reverse(); err != nil {
		return err
	}
	return obj.init.Txn.Commit()
}

// Copy is implemented so that the type values are not lost if we copy this
// function.
func (obj *ReduceFunc) Copy() interfaces.Func {
	var three *bool
	if obj.three != nil {
		b := *obj.three
		three = &b
	}
	return &ReduceFunc{
		Textarea: obj.Textarea,

		Type:  obj.Type, // don't copy because we use this after unification
		three: three,    // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}
