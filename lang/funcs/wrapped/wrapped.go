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

package wrapped

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

var _ interfaces.Func = &Func{} // ensure it meets this expectation

// Func is a wrapped scaffolding function struct which fulfills the boiler-plate
// for the function API, but that can run a very simple, static, pure, function.
// It can be wrapped by other structs that support polymorphism in various ways.
type Func struct {
	//*docsUtil.Metadata // This should NOT happen here, the parents do it.

	// Name is a unique string name for the function.
	Name string

	// Type is the type of the function. It can include unification
	// variables when this struct is wrapped in one that can build this out.
	Type *types.Type

	// Fn is the concrete version of our chosen function.
	Fn *types.FuncValue

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *Func) String() string {
	return fmt.Sprintf("%s @ %p", obj.Name, obj) // be more unique!
}

// ArgGen returns the Nth arg name for this function.
func (obj *Func) ArgGen(index int) (string, error) {
	// If the user specified just a ?1 here, then this might panic if we
	// wanted to determine the arg length at compile time.
	seq := obj.Type.Ord
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *Func) Validate() error {
	if obj.Fn == nil { // build must be run first
		return fmt.Errorf("func has not been built")
	}
	if obj.Fn.T == nil {
		return fmt.Errorf("func type must not be nil")
	}
	if obj.Fn.T.Kind != types.KindFunc {
		return fmt.Errorf("func must be a kind of func")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *Func) Info() *interfaces.Info {
	var typ *types.Type
	// For speculation we still need to return a type with unification vars.
	if obj.Type != nil { // && !obj.Type.HasUni() // always return something
		typ = obj.Type
	}
	if obj.Fn != nil { // don't panic if called speculatively
		typ = obj.Fn.Type()
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: should this be something we specify here?
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *Func) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *Func) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				if len(obj.Fn.Type().Ord) > 0 {
					return nil // can't output any more
				}
				// no inputs were expected, pass through once
			}
			if ok {
				//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
				//	return errwrap.Wrapf(err, "wrong function input")
				//}

				if obj.last != nil && input.Cmp(obj.last) == nil {
					continue // value didn't change, skip it
				}
				obj.last = input // store for next
			}

			values := []types.Value{}
			for _, name := range obj.Fn.Type().Ord {
				x := input.Struct()[name]
				values = append(values, x)
			}

			if obj.init.Debug {
				obj.init.Logf("Calling function with: %+v", values)
			}
			result, err := obj.Fn.Call(ctx, values) // (Value, error)
			if err != nil {
				if obj.init.Debug {
					obj.init.Logf("Function returned error: %+v", err)
				}
				return err
			}
			if obj.init.Debug {
				obj.init.Logf("Function returned with: %+v", result)
			}

			// TODO: do we want obj.result to be a pointer instead?
			if obj.result == result {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			if len(obj.Fn.Type().Ord) == 0 {
				return nil // no more values, we're a pure func
			}
		case <-ctx.Done():
			return nil
		}
	}
}
