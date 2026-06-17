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

package coretest

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// OneStreamFuncName is the name this function is registered as.
	OneStreamFuncName = "one_stream"

	// arg names...
	oneStreamArgNameCount = "count"

	repeatableString = "x" // whatever
)

func init() {
	funcs.ModuleRegister(ModuleName, OneStreamFuncName, func() interfaces.Func { return &OneStreamFunc{} })
}

// OneStreamFunc is a func that takes a single input, and returns a single value
// once. It is used to test that we don't send too many values or any values too
// early.
type OneStreamFunc struct {
	interfaces.Textarea

	init *interfaces.Init

	input chan int64

	once  bool  // did we send the string?
	count int64 // last value
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *OneStreamFunc) String() string {
	return OneStreamFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *OneStreamFunc) ArgGen(index int) (string, error) {
	seq := []string{oneStreamArgNameCount}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *OneStreamFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *OneStreamFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s int) str", oneStreamArgNameCount)),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *OneStreamFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan int64)
	obj.init.Logf("Init of `%s` @ %p", OneStreamFuncName, obj)
	return nil
}

// Stream verifies that the static input is received exactly once.
func (obj *OneStreamFunc) Stream(ctx context.Context) error {
	obj.init.Logf("Stream of `%s` @ %p", OneStreamFuncName, obj)
	for {
		select {
		case count, ok := <-obj.input:
			if !ok {
				return nil
			}

			if obj.once {
				return fmt.Errorf("you can only pass in a single input, got: %d, previously: %d", count, obj.count)
			}

			obj.count = count // store for later
			obj.once = true

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *OneStreamFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("expected one arg")
	}

	count := args[0].Int()
	if count < 0 {
		return nil, fmt.Errorf("cannot use a negative count")
	}

	select {
	case obj.input <- count:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &types.StrValue{V: strings.Repeat(repeatableString, int(count))}, nil
}
