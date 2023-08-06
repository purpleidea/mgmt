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

package coreiter

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/fancyfunc"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// MapFuncName is the name this function is registered as.
	MapFuncName = "map"

	// arg names...
	mapArgNameInputs   = "inputs"
	mapArgNameFunction = "function"
)

func init() {
	funcs.ModuleRegister(ModuleName, MapFuncName, func() interfaces.Func { return &MapFunc{} }) // must register the func and name
}

var _ interfaces.PolyFunc = &MapFunc{} // ensure it meets this expectation

// MapFunc is the standard map iterator function that applies a function to each
// element in a list. It returns a list with the same number of elements as the
// input list. There is no requirement that the element output type be the same
// as the input element type. This implements the signature: `func(inputs []T1,
// function func(T1) T2) []T2` instead of the alternate with the two input args
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

	lastFuncValue       *fancyfunc.FuncValue // remember the last function value
	lastInputListLength int                  // remember the last input list length

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

// Unify returns the list of invariants that this func produces.
func (obj *MapFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(inputs []T1, function func(T1) T2) []T2

	inputsName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}
	functionName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	dummyArgList := &interfaces.ExprAny{} // corresponds to the input list
	dummyArgFunc := &interfaces.ExprAny{} // corresponds to the input func
	dummyOutList := &interfaces.ExprAny{} // corresponds to the output list

	t1Expr := &interfaces.ExprAny{} // corresponds to the t1 type
	t2Expr := &interfaces.ExprAny{} // corresponds to the t2 type

	invar = &interfaces.EqualityWrapListInvariant{
		Expr1:    dummyArgList,
		Expr2Val: t1Expr,
	}
	invariants = append(invariants, invar)

	invar = &interfaces.EqualityWrapListInvariant{
		Expr1:    dummyOutList,
		Expr2Val: t2Expr,
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{inputsName, functionName}
	mapped[inputsName] = dummyArgList
	mapped[functionName] = dummyArgFunc

	invar = &interfaces.EqualityWrapFuncInvariant{
		Expr1:    expr, // maps directly to us!
		Expr2Map: mapped,
		Expr2Ord: ordered,
		Expr2Out: dummyOutList,
	}
	invariants = append(invariants, invar)

	// relationship between t1 and t2
	argName := util.NumToAlpha(0) // XXX: does the arg name matter?
	invar = &interfaces.EqualityWrapFuncInvariant{
		Expr1: dummyArgFunc,
		Expr2Map: map[string]interfaces.Expr{
			argName: t1Expr,
		},
		Expr2Ord: []string{argName},
		Expr2Out: t2Expr,
	}
	invariants = append(invariants, invar)

	// generator function
	fn := func(fnInvariants []interfaces.Invariant, solved map[interfaces.Expr]*types.Type) ([]interfaces.Invariant, error) {
		for _, invariant := range fnInvariants {
			// search for this special type of invariant
			cfavInvar, ok := invariant.(*interfaces.CallFuncArgsValueInvariant)
			if !ok {
				continue
			}
			// did we find the mapping from us to ExprCall ?
			if cfavInvar.Func != expr {
				continue
			}
			// cfavInvar.Expr is the ExprCall! (the return pointer)
			// cfavInvar.Args are the args that ExprCall uses!
			if l := len(cfavInvar.Args); l != 2 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}
			// we must have exactly two args

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// add the relationship to the returned value
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Expr,
				Expr2: dummyOutList,
			}
			invariants = append(invariants, invar)

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyArgList,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyArgFunc,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityWrapListInvariant{
				Expr1:    cfavInvar.Args[0],
				Expr2Val: t1Expr,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityWrapListInvariant{
				Expr1:    cfavInvar.Expr,
				Expr2Val: t2Expr,
			}
			invariants = append(invariants, invar)

			var t1, t2 *types.Type                       // as seen in our sig's
			var foundArgName string = util.NumToAlpha(0) // XXX: is this a hack?

			// validateArg0 checks: inputs []T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}
				if typ.Kind != types.KindList {
					return fmt.Errorf("input type must be of kind list")
				}
				if typ.Val == nil { // TODO: is this okay to add?
					return nil // unknown so far
				}
				if t1 == nil { // t1 is not yet known, so done!
					t1 = typ.Val // learn!
					return nil
				}
				//if err := typ.Val.Cmp(t1); err != nil {
				//	return errwrap.Wrapf(err, "input type was inconsistent")
				//}
				//return nil
				return errwrap.Wrapf(typ.Val.Cmp(t1), "input type was inconsistent")
			}

			// validateArg1 checks: func(T1) T2
			validateArg1 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}
				if typ.Kind != types.KindFunc {
					return fmt.Errorf("input type must be of kind func")
				}
				if len(typ.Map) != 1 || len(typ.Ord) != 1 {
					return fmt.Errorf("input type func must have only one input arg")
				}
				arg, exists := typ.Map[typ.Ord[0]]
				if !exists {
					// programming error
					return fmt.Errorf("input type func first arg is missing")
				}

				if t1 != nil {
					if err := arg.Cmp(t1); err != nil {
						return errwrap.Wrapf(err, "input type func arg was inconsistent")
					}
				}
				if t2 != nil {
					if err := typ.Out.Cmp(t2); err != nil {
						return errwrap.Wrapf(err, "input type func output was inconsistent")
					}
				}

				// in case they weren't set already
				t1 = arg
				t2 = typ.Out
				foundArgName = typ.Ord[0] // we found a name!
				return nil
			}

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first input arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first input arg type is inconsistent")
				}
			}
			// XXX: since we might not yet have association to this
			// expression (dummyArgList) yet, we could consider
			// returning some of the invariants and a new generator
			// and hoping we get a hit on this one the next time.
			if typ, exists := solved[dummyArgList]; exists { // alternate way to lookup type
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first input arg type is inconsistent")
				}
			}

			if typ, err := cfavInvar.Args[1].Type(); err == nil { // is it known?
				// this sets t1 and t2 on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second input arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[1]]; exists { // alternate way to lookup type
				// this sets t1 and t2 on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second input arg type is inconsistent")
				}
			}
			// XXX: since we might not yet have association to this
			// expression (dummyArgFunc) yet, we could consider
			// returning some of the invariants and a new generator
			// and hoping we get a hit on this one the next time.
			if typ, exists := solved[dummyArgFunc]; exists { // alternate way to lookup type
				// this sets t1 and t2 on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second input arg type is inconsistent")
				}
			}

			// XXX: look for t1 and t2 in other places?

			if t1 != nil {
				invar = &interfaces.EqualsInvariant{
					Expr: t1Expr,
					Type: t1,
				}
				invariants = append(invariants, invar)
			}

			if t1 != nil && t2 != nil {
				// TODO: if the argName matters, do it here...
				_ = foundArgName
				//argName := foundArgName // XXX: is this a hack?
				//mapped := make(map[string]interfaces.Expr)
				//ordered := []string{argName}
				//mapped[argName] = t1Expr
				//invar = &interfaces.EqualityWrapFuncInvariant{
				//	Expr1:    dummyArgFunc,
				//	Expr2Map: mapped,
				//	Expr2Ord: ordered,
				//	Expr2Out: t2Expr,
				//}
				//invariants = append(invariants, invar)
			}

			// note, currently, we can't learn t2 without t1
			if t2 != nil {
				invar = &interfaces.EqualsInvariant{
					Expr: t2Expr,
					Type: t2,
				}
				invariants = append(invariants, invar)
			}

			// We need to require this knowledge to continue!
			if t1 == nil || t2 == nil {
				return nil, fmt.Errorf("not enough known about function signature")
			}

			// TODO: do we return this relationship with ExprCall?
			invar = &interfaces.EqualityWrapCallInvariant{
				// TODO: should Expr1 and Expr2 be reversed???
				Expr1: cfavInvar.Expr,
				//Expr2Func: cfavInvar.Func, // same as below
				Expr2Func: expr,
			}
			invariants = append(invariants, invar)

			// TODO: are there any other invariants we should build?
			return invariants, nil // generator return
		}
		// We couldn't tell the solver anything it didn't already know!
		return nil, fmt.Errorf("couldn't generate new invariants")
	}
	invar = &interfaces.GeneratorInvariant{
		Func: fn,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *MapFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// XXX: double check that this works with `func([]int, func(int) str) []str` (when types change!)
	// TODO: look at partialValues to gleam type information?
	if partialType == nil {
		return nil, fmt.Errorf("zero type information given")
	}
	if partialType.Kind != types.KindFunc {
		return nil, fmt.Errorf("partial type must be of kind func")
	}

	// If we figure out both of these two types, we'll know the full type...
	var t1 *types.Type // type
	var t2 *types.Type // rtype

	// Look at the returned "out" type if it's known.
	if tOut := partialType.Out; tOut != nil {
		if tOut.Kind != types.KindList {
			return nil, fmt.Errorf("partial out type must be of kind list")
		}
		t2 = tOut.Val // found (if not nil)
	}

	ord := partialType.Ord
	if partialType.Map != nil {
		// TODO: is it okay to assume this?
		//if len(ord) == 0 {
		//	return nil, fmt.Errorf("must have two args in func")
		//}
		if len(ord) != 2 {
			return nil, fmt.Errorf("must have two args in func")
		}

		if tInputs, exists := partialType.Map[ord[0]]; exists && tInputs != nil {
			if tInputs.Kind != types.KindList {
				return nil, fmt.Errorf("first input arg must be of kind list")
			}
			t1 = tInputs.Val // found (if not nil)
		}

		if tFunction, exists := partialType.Map[ord[1]]; exists && tFunction != nil {
			if tFunction.Kind != types.KindFunc {
				return nil, fmt.Errorf("second input arg must be a func")
			}

			fOrd := tFunction.Ord
			if fMap := tFunction.Map; fMap != nil {
				if len(fOrd) != 1 {
					return nil, fmt.Errorf("second input arg func, must have only one arg")
				}
				if fIn, exists := fMap[fOrd[0]]; exists && fIn != nil {
					if err := fIn.Cmp(t1); t1 != nil && err != nil {
						return nil, errwrap.Wrapf(err, "first arg function in type is inconsistent")
					}
					t1 = fIn // found
				}
			}

			if fOut := tFunction.Out; fOut != nil {
				if err := fOut.Cmp(t2); t2 != nil && err != nil {
					return nil, errwrap.Wrapf(err, "second arg function out type is inconsistent")
				}
				t2 = fOut // found
			}
		}
	}

	if t1 == nil || t2 == nil {
		return nil, fmt.Errorf("not enough type information given")
	}
	tI := types.NewType(fmt.Sprintf("[]%s", t1.String())) // in
	tO := types.NewType(fmt.Sprintf("[]%s", t2.String())) // out
	tF := types.NewType(fmt.Sprintf("func(%s) %s", t1.String(), t2.String()))
	s := fmt.Sprintf("func(%s %s, %s %s) %s", mapArgNameInputs, tI, mapArgNameFunction, tF, tO)
	typ := types.NewType(s) // yay!

	// TODO: type check that the partialValues are compatible

	return []*types.Type{typ}, nil // solved!
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
	sig := obj.sig() // helper

	return &interfaces.Info{
		Pure: false, // TODO: what if the input function isn't pure?
		Memo: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// helper
func (obj *MapFunc) sig() *types.Type {
	// TODO: what do we put if this is unknown?
	tIi := types.TypeVariant
	if obj.Type != nil {
		tIi = obj.Type
	}
	tI := types.NewType(fmt.Sprintf("[]%s", tIi.String())) // type of 2nd arg

	tOi := types.TypeVariant
	if obj.RType != nil {
		tOi = obj.RType
	}
	tO := types.NewType(fmt.Sprintf("[]%s", tOi.String())) // return type

	// type of 1st arg (the function)
	tF := types.NewType(fmt.Sprintf("func(%s %s) %s", "name-which-can-vary-over-time", tIi.String(), tOi.String()))

	s := fmt.Sprintf("func(%s %s, %s %s) %s", mapArgNameInputs, tI, mapArgNameFunction, tF, tO)
	return types.NewType(s) // yay!
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
	subgraphInput := &simple.ChannelBasedSourceFunc{
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

			// XXX: double check this passes through function changes
			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			value, exists := input.Struct()[mapArgNameFunction]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			newFuncValue, ok := value.(*fancyfunc.FuncValue)
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
	// 'newFuncValue' to each, then combines the 'n' outputs back into a list.
	//
	// Here is what the subgraph looks like:
	//
	// digraph {
	//   "subgraphInput" -> "inputElemFunc0"
	//   "subgraphInput" -> "inputElemFunc1"
	//   "subgraphInput" -> "inputElemFunc2"
	//
	//   "inputElemFunc0" -> "outputElemFunc0"
	//   "inputElemFunc1" -> "outputElemFunc1"
	//   "inputElemFunc2" -> "outputElemFunc2"
	//
	//   "outputElemFunc0" -> "outputListFunc"
	//   "outputElemFunc1" -> "outputListFunc"
	//   "outputElemFunc1" -> "outputListFunc"
	//
	//   "outputListFunc" -> "subgraphOutput"
	// }

	const channelBasedSinkFuncArgNameEdgeName = simple.ChannelBasedSinkFuncArgName // XXX: not sure if the specific name matters.

	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	// create the new subgraph

	obj.outputChan = make(chan types.Value)
	subgraphOutput := &simple.ChannelBasedSinkFunc{
		Name:     "subgraphOutput",
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
	outputListFunc := simple.SimpleFnToDirectFunc(
		"mapOutputList",
		&types.SimpleFn{
			V: func(args []types.Value) (types.Value, error) {
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
		inputElemFunc := simple.SimpleFnToDirectFunc(
			fmt.Sprintf("mapInputElem[%d]", i),
			&types.SimpleFn{
				V: func(args []types.Value) (types.Value, error) {
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

		outputElemFunc, err := obj.lastFuncValue.Call(obj.init.Txn, []interfaces.Func{inputElemFunc})
		if err != nil {
			return errwrap.Wrapf(err, "could not call obj.lastFuncValue.Call()")
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
