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
	"path"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// VarDirFunctionsPrefix is the prefix we prepend to any VarDir path
	// request, so that if the local.VarDir API is used elsewhere, it
	// doesn't conflict with what we're doing here.
	VarDirFunctionsPrefix = "functions/"

	// VarDirFuncName is the name this function is registered as.
	VarDirFuncName = "vardir"

	// arg names...
	absPathArgNamePath = "path"
)

func init() {
	// TODO: Add a function named UniqVarDir which returns a path within a
	// private namespace that is unique to the caller of the function. IOW,
	// we probably want to prepend the obj.data.Base path onto the path
	// before we call local.VarDir().
	funcs.ModuleRegister(ModuleName, VarDirFuncName, func() interfaces.Func { return &VarDirFunc{} }) // must register the func and name
}

// VarDirFunc is a function that returns the absolute, full path in the deploy
// from an input path that is relative to the calling file. If you pass it an
// empty string, you'll just get the absolute deploy directory path that you're
// in.
type VarDirFunc struct {
	init *interfaces.Init
	data *interfaces.FuncData
	last types.Value // last value received to use for diff

	path *string // the active path
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *VarDirFunc) String() string {
	return VarDirFuncName
}

// SetData is used by the language to pass our function some code-level context.
func (obj *VarDirFunc) SetData(data *interfaces.FuncData) {
	obj.data = data
}

// ArgGen returns the Nth arg name for this function.
func (obj *VarDirFunc) ArgGen(index int) (string, error) {
	seq := []string{absPathArgNamePath}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *VarDirFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *VarDirFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) str", absPathArgNamePath)),
	}
}

// Init runs some startup code for this function.
func (obj *VarDirFunc) Init(init *interfaces.Init) error {
	obj.init = init
	if obj.data == nil {
		// programming error
		return fmt.Errorf("missing function data")
	}
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *VarDirFunc) Stream(ctx context.Context) error {
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
func (obj *VarDirFunc) Call(ctx context.Context, input []types.Value) (types.Value, error) {
	reldir := input[0].Str()
	if strings.HasPrefix(reldir, "/") {
		return nil, fmt.Errorf("path must be relative")
	}
	if !strings.HasSuffix(reldir, "/") {
		return nil, fmt.Errorf("path must be a dir")
	}
	// NOTE: The above checks ensure we don't get either "" or "/" as input!

	p := fmt.Sprintf("%s/", path.Join(VarDirFunctionsPrefix, reldir))

	result, err := obj.init.Local.VarDir(ctx, p)
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: result,
	}, nil
}
