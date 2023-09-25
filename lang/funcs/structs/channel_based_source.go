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

// ChannelBasedSourceFunc is a Func which receives values from a golang channel
// and emits them to the downstream nodes.
type ChannelBasedSourceFunc struct {
	Name   string
	Source interfaces.Func // for drawing dashed edges in the Graphviz visualization

	Chan chan types.Value
	Type *types.Type

	init *interfaces.Init
	last types.Value // last value received to use for diff
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ChannelBasedSourceFunc) String() string {
	return "ChannelBasedSourceFunc"
}

// ArgGen returns the Nth arg name for this function.
func (obj *ChannelBasedSourceFunc) ArgGen(index int) (string, error) {
	return "", fmt.Errorf("the ChannelBasedSourceFunc doesn't have any arguments")
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ChannelBasedSourceFunc) Validate() error {
	if obj.Chan == nil {
		return fmt.Errorf("the Chan was not set")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *ChannelBasedSourceFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func() %s", obj.Type)),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ChannelBasedSourceFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ChannelBasedSourceFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes

	for {
		select {
		case input, ok := <-obj.Chan:
			if !ok {
				return nil // can't output any more
			}

			//if obj.last != nil && input.Cmp(obj.last) == nil {
			//	continue // value didn't change, skip it
			//}
			obj.last = input // store so we can send after this select

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.last: // send
		case <-ctx.Done():
			return nil
		}
	}
}
