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

package structs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ConstFuncName is the unique name identifier for this function.
	ConstFuncName = "const"
)

// ConstFunc is a function that returns the constant value passed to Value.
type ConstFunc struct {
	Value    types.Value
	NameHint string

	init *interfaces.Init
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ConstFunc) String() string {
	if obj.NameHint != "" {
		return obj.NameHint
	}
	if obj.Value != nil && obj.Value.Type() != nil {
		return fmt.Sprintf("%s: %s(%s)", ConstFuncName, obj.Value.Type().String(), obj.Value.String())
	}
	return ConstFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *ConstFunc) Validate() error {
	if obj.Value == nil {
		return fmt.Errorf("must specify `Value` input")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *ConstFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Value != nil { // don't panic if called speculatively
		if t := obj.Value.Type(); t != nil {
			typ = types.NewType(fmt.Sprintf("func() %s", t.String()))
		}
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(), // XXX: implement this and check .Err in engine!
	}
}

// Init runs some startup code for this const.
func (obj *ConstFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the single value that this const has, and then closes.
func (obj *ConstFunc) Stream(ctx context.Context) error {
	value, err := obj.Call(ctx, nil)
	if err != nil {
		return err
	}
	select {
	case obj.init.Output <- value:
		// pass
	case <-ctx.Done():
		return nil
	}
	close(obj.init.Output) // signal that we're done sending
	return nil
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ConstFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.Value == nil {
		return nil, fmt.Errorf("no value available from const")
	}
	return obj.Value, nil
}
