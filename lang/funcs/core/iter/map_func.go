// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	// XXX: rename to map once our parser sees a function name and not a type
	funcs.ModuleRegister(ModuleName, "xmap", func() interfaces.Func { return &MapFunc{} }) // must register the func and name
}

const (
	argNameFunction = "function"
	argNameInputs   = "inputs"
)

// MapFunc is the standard map iterator function that applies a function to each
// element in a list. It returns a list with the same number of elements as the
// input list. There is no requirement that the element output type be the same
// as the input element type.
// TODO: should we extend this to support iterating over map's and structs, or
// should that be a different function? I think a different function is best.
type MapFunc struct {
	Type  *types.Type // this is the type of the elements in our input list
	RType *types.Type // this is the type of the elements in our output list

	init *interfaces.Init
	last types.Value // last value received to use for diff

	function func([]types.Value) (types.Value, error)
	inputs   types.Value

	result types.Value // last calculated output

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *MapFunc) ArgGen(index int) (string, error) {
	seq := []string{argNameFunction, argNameInputs}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *MapFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
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

		if tInputs, exists := partialType.Map[ord[1]]; exists && tInputs != nil {
			if tInputs.Kind != types.KindList {
				return nil, fmt.Errorf("second input arg must be of kind list")
			}
			t1 = tInputs.Val // found (if not nil)
		}

		if tFunction, exists := partialType.Map[ord[0]]; exists && tFunction != nil {
			if tFunction.Kind != types.KindFunc {
				return nil, fmt.Errorf("first input arg must be a func")
			}

			fOrd := tFunction.Ord
			if fMap := tFunction.Map; fMap != nil {
				if len(fOrd) != 1 {
					return nil, fmt.Errorf("first input arg func, must have only one arg")
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
					return nil, errwrap.Wrapf(err, "first arg function out type is inconsistent")
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
	s := fmt.Sprintf("func(%s %s, %s %s) %s", argNameFunction, tF, argNameInputs, tI, tO)
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

	tFunction, exists := typ.Map[typ.Ord[0]]
	if !exists || tFunction == nil {
		return fmt.Errorf("first argument was missing")
	}
	tInputs, exists := typ.Map[typ.Ord[1]]
	if !exists || tInputs == nil {
		return fmt.Errorf("second argument was missing")
	}

	if tFunction.Kind != types.KindFunc {
		return fmt.Errorf("first argument must be of kind func")
	}
	if tInputs.Kind != types.KindList {
		return fmt.Errorf("second argument must be of kind list")
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

	s := fmt.Sprintf("func(%s %s, %s %s) %s", argNameFunction, tF, argNameInputs, tI, tO)
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
