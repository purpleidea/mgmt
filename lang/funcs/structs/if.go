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

package structs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// IfFuncName is the unique name identifier for this function.
	IfFuncName = "if"
)

// IfFunc is a function that passes through the value of the correct branch
// based on the conditional value it gets.
type IfFunc struct {
	interfaces.Textarea

	Type *types.Type // this is the type of the if expression output we hold

	init   *interfaces.Init
	last   types.Value // last value received to use for diff
	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *IfFunc) String() string {
	return IfFuncName
}

// Validate tells us if the input struct takes a valid form.
func (obj *IfFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *IfFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		typ = &types.Type{
			Kind: types.KindFunc, // function type
			Map: map[string]*types.Type{
				"c": types.TypeBool, // conditional must be a boolean
				"a": obj.Type,       // true branch must be this type
				"b": obj.Type,       // false branch must be this type too
			},
			Ord: []string{"c", "a", "b"}, // conditional, and two branches
			Out: obj.Type,                // result type must match
		}
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this if expression function.
func (obj *IfFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *IfFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}
			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			if err != nil {
				return err
			}

			result, err := obj.Call(ctx, args) // get the value...
			if err != nil {
				return err
			}

			// skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
// XXX: Is it correct to implement this here for this particular function?
func (obj *IfFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.Info() == nil {
		return nil, fmt.Errorf("info is empty")
	}
	if obj.Info().Sig == nil {
		return nil, fmt.Errorf("sig is empty")
	}
	if i, j := len(args), len(obj.Info().Sig.Ord); i != j {
		return nil, fmt.Errorf("arg length doesn't match, got %d, exp: %d", i, j)
	}

	if c := args[0].Bool(); c {
		return args[1], nil // true branch
	}
	return args[2], nil
}
