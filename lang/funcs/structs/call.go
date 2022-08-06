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
	"github.com/purpleidea/mgmt/util/errwrap"
)

// CallFunc is a function that takes in a function and all the args, and passes
// through the results of running the function call.
type CallFunc struct {
	Type     *types.Type // this is the type of the var's value that we hold
	FuncType *types.Type
	Edge     string // name of the edge used (typically starts with: `call:`)
	//Func interfaces.Func // this isn't actually used in the Stream :/
	//Fn *types.FuncValue // pass in the actual function instead of Edge

	// Indexed specifies that args are accessed by index instead of name.
	// This is currently unused.
	Indexed bool

	init   *interfaces.Init
	last   types.Value // last value received to use for diff
	result types.Value // last calculated output

	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly.
func (obj *CallFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	if obj.FuncType == nil {
		return fmt.Errorf("must specify a func type")
	}
	// TODO: maybe we can remove this if we use this for core functions...
	if obj.Edge == "" {
		return fmt.Errorf("must specify an edge name")
	}
	typ := obj.FuncType
	// we only care about the output type of calling our func
	if err := obj.Type.Cmp(typ.Out); err != nil {
		return errwrap.Wrapf(err, "call expr type must match func out type")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *CallFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		typ = &types.Type{
			Kind: types.KindFunc, // function type
			Map:  make(map[string]*types.Type),
			Ord:  []string{},
			Out:  obj.Type, // this is the output type for the expression
		}

		sig := obj.FuncType
		if obj.Edge != "" {
			typ.Map[obj.Edge] = sig // we get a function in
			typ.Ord = append(typ.Ord, obj.Edge)
		}

		// add any incoming args
		for _, key := range sig.Ord { // sig.Out, not sig!
			typ.Map[key] = sig.Map[key]
			typ.Ord = append(typ.Ord, key)
		}
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this composite function.
func (obj *CallFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *CallFunc) Stream() error {
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

			st := input.(*types.StructValue) // must be!

			// get the function
			fn, exists := st.Lookup(obj.Edge)
			if !exists {
				return fmt.Errorf("missing expected input argument `%s`", obj.Edge)
			}

			// get the arguments to call the function
			args := []types.Value{}
			typ := obj.FuncType
			for ix, key := range typ.Ord { // sig!
				if obj.Indexed {
					key = fmt.Sprintf("%d", ix)
				}
				value, exists := st.Lookup(key)
				// TODO: replace with:
				//value, exists := st.Lookup(fmt.Sprintf("arg:%s", key))
				if !exists {
					return fmt.Errorf("missing expected input argument `%s`", key)
				}
				args = append(args, value)
			}

			// actually call it
			result, err := fn.(*types.FuncValue).Call(args)
			if err != nil {
				return errwrap.Wrapf(err, "error calling function")
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
func (obj *CallFunc) Close() error {
	close(obj.closeChan)
	return nil
}
