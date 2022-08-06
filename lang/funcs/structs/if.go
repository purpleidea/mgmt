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

package structs

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// IfFunc is a function that passes through the value of the correct branch
// based on the conditional value it gets.
type IfFunc struct {
	Type *types.Type // this is the type of the if expression output we hold

	init   *interfaces.Init
	last   types.Value // last value received to use for diff
	result types.Value // last calculated output

	closeChan chan struct{}
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
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *IfFunc) Stream() error {
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

			var result types.Value

			if input.Struct()["c"].Bool() {
				result = input.Struct()["a"] // true branch
			} else {
				result = input.Struct()["b"] // false branch
			}

			// skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
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
func (obj *IfFunc) Close() error {
	close(obj.closeChan)
	return nil
}
