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
	// OutputFuncArgName is the name for the edge which connects
	// the input value to OutputFunc.
	OutputFuncArgName = "out"

	// OutputFuncDummyArgName is the name for the edge which is used as the
	// dummy.
	OutputFuncDummyArgName = "dummy"
)

// OutputFunc is a Func which receives values from upstream nodes and emits them
// downstream. It accepts (and ignores) a "dummy" arg as well.
type OutputFunc struct {
	interfaces.Textarea // XXX: Do we want this here for this func as well ?

	Name     string
	EdgeName string
	Type     *types.Type

	init *interfaces.Init
	last types.Value // last value received to use for diff
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *OutputFunc) String() string {
	return obj.Name
}

// ArgGen returns the Nth arg name for this function.
func (obj *OutputFunc) ArgGen(index int) (string, error) {
	seq := []string{obj.EdgeName, OutputFuncDummyArgName}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *OutputFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *OutputFunc) Info() *interfaces.Info {
	// contains "dummy" return type
	s := fmt.Sprintf("func(%s %s, %s nil) %s", obj.EdgeName, obj.Type, OutputFuncDummyArgName, obj.Type)
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Sig:  types.NewType(s),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *OutputFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *OutputFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("programming error, can't find edge")
	}
	// Send the useful input arg, not the dummy arg.
	return args[0], nil
}
