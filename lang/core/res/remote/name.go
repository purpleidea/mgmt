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

package coreresremote

import (
	"context"

	coreres "github.com/purpleidea/mgmt/lang/core/res"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// NameFuncName is the name this function is registered as.
	NameFuncName = "name"
)

func init() {
	funcs.ModuleRegister(coreres.ModuleName+"/"+ModuleName, NameFuncName, func() interfaces.Func { return &NameFunc{} })
}

// NameFunc is a function that returns the name of the remote resource which
// started this process, if it was started by one. If it was not, then this
// returns the empty string. This is useful in a deploy with more than one
// remote resource, in combination with the `parent` function, so that each
// child can consider running different resources depending on who its caller is
// and (perhaps) which remote in that caller ran it. While you can use this to
// get the name to use for the remote:done resource, it's preferable to set it
// statically so that it is chosen by logical association, rather than by
// dynamic execution. (In other words, it's recommended to avoid using this
// function to get the name value to use for a remote:done resource.) This is
// because the name determines which "branch" of the parent code ran this
// resource, which also acts like a logical safety check, because using the
// wrong name would result in an error. By using this function result as the
// name, you would always match, and so it may mask a possible logic mistake.
type NameFunc struct {
	interfaces.Textarea

	init *interfaces.Init
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *NameFunc) String() string {
	return NameFuncName
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *NameFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *NameFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // depends on how this process was started
		Memo: false,
		Fast: true,
		Spec: false,
		Sig:  types.NewType("func() str"),
	}
}

// Init runs some startup code for this function.
func (obj *NameFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *NameFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}
	remote := obj.init.Local.Remote

	if remote == nil { // not a remote child
		return &types.StrValue{V: ""}, nil
	}

	return &types.StrValue{
		V: remote.Name,
	}, nil
}
