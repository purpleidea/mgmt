// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package simple

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// DirectInterface specifies whether we should use the direct function
	// API or not. If we don't use it, then these simple functions are
	// wrapped with the struct below.
	DirectInterface = false // XXX: fix any bugs and set to true!
)

// RegisteredFuncs maps a function name to the corresponding static, pure func.
var RegisteredFuncs = make(map[string]*types.FuncValue) // must initialize

// Register registers a simple, static, pure function. It is easier to use than
// the raw function API, but also limits you to simple, static, pure functions.
func Register(name string, fn *types.FuncValue) {
	if _, exists := RegisteredFuncs[name]; exists {
		panic(fmt.Sprintf("a simple func named %s is already registered", name))
	}
	if fn == nil {
		panic(fmt.Sprintf("simple func %s contains no function body", name))
	}
	if fn.T == nil {
		panic(fmt.Sprintf("simple func %s contains a nil type signature", name))
	}
	if fn.T.Kind != types.KindFunc {
		panic(fmt.Sprintf("simple func %s must be of kind func", name))
	}
	if fn.T.HasVariant() {
		panic(fmt.Sprintf("simple func %s contains a variant type signature", name))
	}

	RegisteredFuncs[name] = fn // store a copy for ourselves

	// register a copy in the main function database
	funcs.Register(name, func() interfaces.Func { return &WrappedFunc{Name: name, Fn: fn} })
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module. This is a helper function.
func ModuleRegister(module, name string, fn *types.FuncValue) {
	Register(module+funcs.ModuleSep+name, fn)
}

// WrappedFunc is a scaffolding function struct which fulfills the boiler-plate
// for the function API, but that can run a very simple, static, pure function.
type WrappedFunc struct {
	Name string

	Fn *types.FuncValue

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *WrappedFunc) String() string {
	return fmt.Sprintf("%s @ %p", obj.Name, obj) // be more unique!
}

// ArgGen returns the Nth arg name for this function.
func (obj *WrappedFunc) ArgGen(index int) (string, error) {
	typ := obj.Fn.Type()
	if typ.Kind != types.KindFunc {
		return "", fmt.Errorf("expected %s, got %s", types.KindFunc, typ.Kind)
	}
	seq := typ.Ord
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *WrappedFunc) Validate() error {
	if obj.Fn == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *WrappedFunc) Info() *interfaces.Info {
	var typ *types.Type
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
func (obj *WrappedFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *WrappedFunc) Stream(ctx context.Context) error {
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

			// Use the existing implementation instead of this one
			// which can't handle this case at the moment.
			//args, err := interfaces.StructToCallableArgs(input)
			args := []types.Value{}
			for _, name := range obj.Fn.Type().Ord {
				x := input.Struct()[name]
				args = append(args, x)
			}

			result, err := obj.Call(args)
			if err != nil {
				return err
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

// Call means this function implements the CallableFunc interface and can be
// called statically if we want to do it speculatively or from a resource.
func (obj *WrappedFunc) Call(args []types.Value) (types.Value, error) {
	result, err := obj.Fn.Call(args) // (Value, error)
	if err != nil {
		return nil, errwrap.Wrapf(err, "simple function errored")
	}
	return result, err
}
