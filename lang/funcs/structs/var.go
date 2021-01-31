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

package structs

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	//"github.com/purpleidea/mgmt/util/errwrap"
)

// VarFunc is a function that passes through a function that came from a bind
// lookup. It exists so that the reactive function engine type checks correctly.
type VarFunc struct {
	Type *types.Type // this is the type of the var's value that we hold
	Edge string      // name of the edge used
	//Func interfaces.Func // this isn't actually used in the Stream :/

	init   *interfaces.Init
	last   types.Value // last value received to use for diff
	result types.Value // last calculated output

	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly.
func (obj *VarFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	if obj.Edge == "" {
		return fmt.Errorf("must specify an edge name")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *VarFunc) Info() *interfaces.Info {
	typ := &types.Type{
		Kind: types.KindFunc, // function type
		Map:  map[string]*types.Type{obj.Edge: obj.Type},
		Ord:  []string{obj.Edge},
		Out:  obj.Type, // this is the output type for the expression
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this composite function.
func (obj *VarFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *VarFunc) Stream() error {
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
			st := input.(*types.StructValue) // must be!
			value, exists := st.Lookup(obj.Edge)
			if !exists {
				return fmt.Errorf("missing expected input argument `%s`", obj.Edge)
			}
			result = value

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
func (obj *VarFunc) Close() error {
	close(obj.closeChan)
	return nil
}
