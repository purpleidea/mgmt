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

package coreiter

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	// XXX: rename to map once our parser sees a function name and not a type
	funcs.ModuleRegister(ModuleName, "xmap", func() interfaces.Func { return &MapFunc{} }) // must register the func and name
}

const (
	argNameInputs   = "inputs"
	argNameFunction = "function"
)

// MapFunc is the standard map iterator function that applies a function to each
// element in a list. It returns a list with the same number of elements as the
// input list. There is no requirement that the element output type be the same
// as the input element type. This implements the signature:
// `func(inputs []T1, function func(T1) T2) []T2` instead of the alternate with
// the two input args swapped, because while the latter is more common with
// languages that support partial function application, the former variant that
// we implemented is much more readable when using an inline lambda.
// TODO: should we extend this to support iterating over map's and structs, or
// should that be a different function? I think a different function is best.
type MapFunc struct {
	Type  *types.Type // this is the type of the elements in our input list
	RType *types.Type // this is the type of the elements in our output list

	init *interfaces.Init
	last types.Value // last value received to use for diff

	inputs   types.Value
	function func([]types.Value) (types.Value, error)

	result types.Value // last calculated output

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *MapFunc) ArgGen(index int) (string, error) {
	seq := []string{argNameInputs, argNameFunction} // inverted for pretty!
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
	s := fmt.Sprintf("func(%s %s, %s %s) %s", argNameInputs, tI, argNameFunction, tF, tO)
	typ := types.NewType(s) // yay!

	// TODO: type check that the partialValues are compatible

	return []*types.Type{typ}, nil // solved!
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *MapFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return fmt.Errorf("the map needs exactly two args")
	}
	if typ.Map == nil {
		return fmt.Errorf("the map is nil")
	}

	tInputs, exists := typ.Map[typ.Ord[0]]
	if !exists || tInputs == nil {
		return fmt.Errorf("first argument was missing")
	}
	tFunction, exists := typ.Map[typ.Ord[1]]
	if !exists || tFunction == nil {
		return fmt.Errorf("second argument was missing")
	}

	if tInputs.Kind != types.KindList {
		return fmt.Errorf("first argument must be of kind list")
	}
	if tFunction.Kind != types.KindFunc {
		return fmt.Errorf("second argument must be of kind func")
	}

	if typ.Out == nil {
		return fmt.Errorf("return type must be specified")
	}
	if typ.Out.Kind != types.KindList {
		return fmt.Errorf("return argument must be a list")
	}

	if len(tFunction.Ord) != 1 {
		return fmt.Errorf("the functions map needs exactly one arg")
	}
	if tFunction.Map == nil {
		return fmt.Errorf("the functions map is nil")
	}
	tArg, exists := tFunction.Map[tFunction.Ord[0]]
	if !exists || tArg == nil {
		return fmt.Errorf("the functions first argument was missing")
	}
	if err := tArg.Cmp(tInputs.Val); err != nil {
		return errwrap.Wrapf(err, "the functions arg type must match the input list contents type")
	}

	if tFunction.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if err := tFunction.Out.Cmp(typ.Out.Val); err != nil {
		return errwrap.Wrapf(err, "return type of function must match returned list contents type")
	}

	obj.Type = tInputs.Val    // or tArg
	obj.RType = tFunction.Out // or typ.Out.Val
	return nil
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
	tF := types.NewType(fmt.Sprintf("func(%s) %s", tIi.String(), tOi.String()))

	s := fmt.Sprintf("func(%s %s, %s %s) %s", argNameInputs, tI, argNameFunction, tF, tO)
	typ := types.NewType(s) // yay!

	return &interfaces.Info{
		Pure: false, // TODO: what if the input function isn't pure?
		Memo: false,
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *MapFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *MapFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
	rtyp := types.NewType(fmt.Sprintf("[]%s", obj.RType.String()))
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back
				continue             // no more inputs, but don't return!
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			function := input.Struct()[argNameFunction].Func() // func([]Value) (Value, error)
			//if function == obj.function { // TODO: how can we cmp?
			//	continue // nothing changed
			//}
			obj.function = function

			inputs := input.Struct()[argNameInputs]
			if obj.inputs != nil && obj.inputs.Cmp(inputs) == nil {
				continue // nothing changed
			}
			obj.inputs = inputs

			// run the function on each index
			output := []types.Value{}
			for ix, v := range inputs.List() { // []Value
				args := []types.Value{v} // only one input arg!
				x, err := function(args)
				if err != nil {
					return errwrap.Wrapf(err, "error running map function on index %d", ix)
				}

				output = append(output, x)
			}
			result := &types.ListValue{
				V: output,
				T: rtyp,
			}

			if obj.result != nil && obj.result.Cmp(result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *MapFunc) Close() error {
	close(obj.closeChan)
	return nil
}
