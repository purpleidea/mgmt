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
	// ChannelBasedSinkFuncArgName is the name for the edge which connects
	// the input value to ChannelBasedSinkFunc.
	ChannelBasedSinkFuncArgName = "channelBasedSinkFuncArg"
)

// ChannelBasedSinkFunc is a Func which receives values from upstream nodes and
// emits them to a golang channel.
type ChannelBasedSinkFunc struct {
	Name     string
	EdgeName string
	Target   interfaces.Func // for drawing dashed edges in the Graphviz visualization

	Chan chan types.Value
	Type *types.Type

	init *interfaces.Init
	last types.Value // last value received to use for diff
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ChannelBasedSinkFunc) String() string {
	return obj.Name
}

// ArgGen returns the Nth arg name for this function.
func (obj *ChannelBasedSinkFunc) ArgGen(index int) (string, error) {
	if index != 0 {
		return "", fmt.Errorf("the ChannelBasedSinkFunc only has one argument")
	}
	return obj.EdgeName, nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ChannelBasedSinkFunc) Validate() error {
	if obj.Chan == nil {
		return fmt.Errorf("the Chan was not set")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *ChannelBasedSinkFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s %s) %s", obj.EdgeName, obj.Type, obj.Type)),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ChannelBasedSinkFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ChannelBasedSinkFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	defer close(obj.Chan)        // the sender closes

	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}

			args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			if err != nil {
				return err
			}

			result, err := obj.Call(ctx, args) // get the value...
			if err != nil {
				return err
			}

			if obj.last != nil && result.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = result // store so we can send after this select

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.Chan <- obj.last: // send
		case <-ctx.Done():
			return nil
		}

		// Also send the value downstream. If we don't, then when we
		// close the Output channel, the function engine is going to
		// complain that we closed that channel without sending it any
		// value.
		select {
		case obj.init.Output <- obj.last: // send
		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
// XXX: Is is correct to implement this here for this particular function?
func (obj *ChannelBasedSinkFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("programming error, can't find edge")
	}
	return args[0], nil
}
