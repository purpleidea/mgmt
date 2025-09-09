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
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	funcs.ModuleRegister(ModuleName, RangeFuncName, func() interfaces.Func { return &RangeFunc{} })
}

const (
	// RangeFuncName is the name this function is registered as.
	RangeFuncName = "range"
)

var _ interfaces.BuildableFunc = &RangeFunc{}

// RangeFunc is a function that ranges over elements on a list according to
// three possible inputs: start, stop, and step. At least one input is needed,
// and in that case it's mapped to be the stop argument. Start is used for the
// function to build lists which start from a chosen number, and step to filter
// its contents to a subset of all the numbers between start and stop. This
// function only takes ints as inputs, and outputs a list of ints.
type RangeFunc struct {
	Type *types.Type

	init   *interfaces.Init
	last   types.Value // used to store the last known value of the function
	result types.Value // used to store the result of the function
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *RangeFunc) String() string {
	return RangeFuncName
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *RangeFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// This function only takes ints as inputs, and outputs a list of ints.

	l := len(partialValues)
	if l < 1 || l > 3 {
		return nil, nil, fmt.Errorf("function must have between 1 and 3 args")
	}

	var typ *types.Type
	if l == 1 {
		// we only have the stop argument
		typ = types.NewType("func(int) []int")
	}

	if l == 2 {
		// we have start and stop arguments
		typ = types.NewType("func(int, int) []int")
	}

	if l == 3 {
		// we have all the arguments
		typ = types.NewType("func(int, int, int) []int")
	}

	return typ, []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *RangeFunc) Build(typ *types.Type) (*types.Type, error) {
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("must be of kind func")
	}

	if len(typ.Ord) < 1 || len(typ.Ord) > 3 {
		return nil, fmt.Errorf("the range function needs one to three args")
	}

	// check each of the args
	for i, v := range typ.Ord {
		tI, exists := typ.Map[v]
		if !exists || tI == nil { // sanity check for existence of arg
			return nil, fmt.Errorf("argument number %d is missing", i)
		}
		if tI.Cmp(types.TypeInt) != nil { // checking arg type
			return nil, fmt.Errorf("input type is not int")
		}
	}

	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}

	if typ.Out.Cmp(types.NewType("[]int")) != nil {
		return nil, fmt.Errorf("return type of function must be a list of ints")
	}

	obj.Type = typ.Copy() // this is to store the type of return value

	return obj.Type, nil
}

// Copy is implemented so that the obj.Type value is not lost if we copy this
// function.
func (obj *RangeFunc) Copy() interfaces.Func {
	return &RangeFunc{
		Type: obj.Type, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}

// Validate tells us if the input struct takes a valid form.
func (obj *RangeFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data
func (obj *RangeFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: true,
		Fast: true,
		Spec: true,
		Sig:  obj.Type,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *RangeFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Call returns the result of this function.
func (obj *RangeFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) == 1 { // we only have stop, assume start is 0 and step is 1
		return obj.loop(ctx, 0, args[0].Int(), 1)
	}
	if len(args) == 2 { // we have start and stop, assume step is 1
		return obj.loop(ctx, args[0].Int(), args[1].Int(), 1)
	}
	if len(args) == 3 { // we have all the args
		return obj.loop(ctx, args[0].Int(), args[1].Int(), args[2].Int())
	}

	return nil, fmt.Errorf("error calling the loop function")
}

// loop is the private helper function that calculates the range according to
// the inputs provided.
func (obj *RangeFunc) loop(ctx context.Context, start, stop, step int64) (types.Value, error) {
	if step == 0 {
		return nil, fmt.Errorf("step value cannot be 0")
	}

	if step > 0 && start >= stop {
		// empty since step is positive and start > stop
		return types.NewType("[]int").New(), nil

	}

	if step < 0 && start <= stop {
		// empty since step is negative and start < stop
		return types.NewType("[]int").New(), nil
	}

	result := []types.Value{}

	if step > 0 {
		for i := start; i < stop; i += step {
			result = append(result, &types.IntValue{V: i})
		}
	} else {
		for i := start; i > stop; i += step {
			result = append(result, &types.IntValue{V: i})
		}
	}

	return &types.ListValue{
		T: types.NewType("[]int"),
		V: result,
	}, nil
}
