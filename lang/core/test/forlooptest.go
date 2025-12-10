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
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ForLoopFuncName is the name this function is registered as.
	ForLoopFuncName = "forloop"
)

func init() {
	funcs.ModuleRegister(ModuleName, ForLoopFuncName, func() interfaces.Func { return &ForLoopFunc{} }) // must register the func and name
}

// ForLoopFunc is a function that is used for testing.
type ForLoopFunc struct {
	init *interfaces.Init

	count int
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ForLoopFunc) String() string {
	return ForLoopFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ForLoopFunc) ArgGen(index int) (string, error) {
	seq := []string{}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ForLoopFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ForLoopFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // maybe false because the output changes
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() []str"),
	}
}

// Init runs some startup code for this function.
func (obj *ForLoopFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ForLoopFunc) Stream(ctx context.Context) error {
	//defer close(obj.input)  // if we close, this is a race with the sender

	startup := make(chan struct{})
	close(startup)

	for {
		select {
		case <-startup:
			startup = nil
			// send an initial event

		case <-time.After(5 * time.Second):

		case <-ctx.Done():
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil { // send event
			return err
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ForLoopFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	l := types.NewList(types.NewType("[]str"))

	if obj.count == 0 {
		if err := l.Add(&types.StrValue{V: "foo"}); err != nil {
			return nil, err
		}
		obj.count++
		return l, nil
	}

	if err := l.Add(&types.StrValue{V: "bar"}); err != nil {
		return nil, err
	}

	return l, nil
}

// Cleanup runs after that function was removed from the graph.
func (obj *ForLoopFunc) Cleanup(ctx context.Context) error {
	return nil
}

// Done is a message from the engine to tell us that no more Call's are coming.
func (obj *ForLoopFunc) Done() error {
	return nil
}
