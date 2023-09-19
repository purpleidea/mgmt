// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

			value, exists := input.Struct()[obj.EdgeName]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			if obj.last != nil && value.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = value // store so we can send after this select

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
