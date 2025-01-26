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

package corelocal

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// PoolFuncName is the name this function is registered as.
	PoolFuncName = "pool"

	// arg names...
	absPathArgNameNamespace = "namespace"
	absPathArgNameUID       = "uid"
	absPathArgNameConfig    = "config"
)

func init() {
	// TODO: Add a "world" version of this function that picks a value from
	// the global "world" pool that all nodes share.
	funcs.ModuleRegister(ModuleName, PoolFuncName, func() interfaces.Func { return &PoolFunc{} }) // must register the func and name
}

// PoolFunc is a function that returns a unique integer from a pool of numbers.
// Within a given namespace, it returns the same integer for a given name. It is
// a simple mechanism to allocate numbers to different inputs when we don't have
// a hashing alternative. It does not allocate zero.
type PoolFunc struct {
	init *interfaces.Init
	data *interfaces.FuncData
	last types.Value // last value received to use for diff
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *PoolFunc) String() string {
	return PoolFuncName
}

// SetData is used by the language to pass our function some code-level context.
func (obj *PoolFunc) SetData(data *interfaces.FuncData) {
	obj.data = data
}

// ArgGen returns the Nth arg name for this function.
func (obj *PoolFunc) ArgGen(index int) (string, error) {
	seq := []string{absPathArgNameNamespace, absPathArgNameUID, absPathArgNameConfig}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *PoolFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *PoolFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str, %s str) int", absPathArgNameNamespace, absPathArgNameUID)),
		// TODO: add an optional config arg
		//Sig: types.NewType(fmt.Sprintf("func(%s str, %s str, %s struct{}) int", absPathArgNameNamespace, absPathArgNameUID, absPathArgNameConfig)),
	}
}

// Init runs some startup code for this function.
func (obj *PoolFunc) Init(init *interfaces.Init) error {
	obj.init = init
	if obj.data == nil {
		// programming error
		return fmt.Errorf("missing function data")
	}
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *PoolFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	var value types.Value
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

			args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			if err != nil {
				return err
			}
			result, err := obj.Call(ctx, args)
			if err != nil {
				return err
			}
			value = result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- value:
		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *PoolFunc) Call(ctx context.Context, input []types.Value) (types.Value, error) {
	// Validation of these inputs happens in the Local API which does it.
	namespace := input[0].Str()
	uid := input[1].Str()
	// TODO: pass in config
	//config := input[2].???()

	result, err := obj.init.Local.Pool(ctx, namespace, uid, nil)
	if err != nil {
		return nil, err
	}
	return &types.IntValue{
		V: int64(result),
	}, nil
}
