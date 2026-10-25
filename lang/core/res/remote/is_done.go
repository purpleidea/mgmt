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
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	coreres "github.com/purpleidea/mgmt/lang/core/res"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// IsDoneFuncName is the name this function is registered as.
	IsDoneFuncName = "is_done"

	// arg names...
	isDoneArgNameName = "name"
)

func init() {
	funcs.ModuleRegister(coreres.ModuleName+"/"+ModuleName, IsDoneFuncName, func() interfaces.Func { return &IsDoneFunc{} })
}

var _ interfaces.StreamableFunc = &IsDoneFunc{}

// IsDoneFunc is a function which returns true if the child of the remote
// resource with this name, run *from* this host, told us that it was done, by
// using the remote:done resource. If our remote resource receives a done from a
// child remote:done resource, we'll ack that, and store that result as a bool
// which this function can read. This is true whether that child succeeded or
// failed, since either way, it's done. It stays true, even across restarts,
// until the ack flag is cleared with the clear param of the remote resource.
type IsDoneFunc struct {
	interfaces.Textarea

	init  *interfaces.Init
	world engine.RemoteWorld

	input chan string // stream of inputs
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *IsDoneFunc) String() string {
	return IsDoneFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *IsDoneFunc) ArgGen(index int) (string, error) {
	seq := []string{isDoneArgNameName}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *IsDoneFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *IsDoneFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // depends on what's in the world
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) bool", isDoneArgNameName)),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *IsDoneFunc) Init(init *interfaces.Init) error {
	obj.init = init
	world, ok := obj.init.World.(engine.RemoteWorld)
	if !ok {
		return fmt.Errorf("world backend does not support the RemoteWorld interface")
	}
	obj.world = world
	obj.input = make(chan string)
	return nil
}

// Stream returns the changing values that this func has over time. It watches
// the ack record for the most recent name that Call was run with.
func (obj *IsDoneFunc) Stream(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops any watch when we return

	var name *string                 // the name that we're watching
	var watchChan chan error         // nil until we have a name
	cancelWatch := func() {}         // stops the current watch
	defer func() { cancelWatch() }() // the current one, not the first one

	for {
		select {
		case n := <-obj.input:
			if name != nil && *name == n {
				continue // nothing changed
			}
			name = &n

			// Stop the previous watch. Its goroutine exits once its
			// context is cancelled so it's safe to stop reading it.
			cancelWatch()
			watchCtx, watchCancel := context.WithCancel(ctx)
			cancelWatch = watchCancel
			var err error
			watchChan, err = obj.world.RemoteAckWatch(watchCtx, obj.init.Hostname, n)
			if err != nil {
				if ctx.Err() != nil { // shutdown, not a failure
					return ctx.Err()
				}
				return errwrap.Wrapf(err, "can't watch the ack record of %s", n)
			}
			// The watch sends a startup event, which makes the next
			// Call read the value again, so nothing can be missed
			// between the read in this Call and the watch starting.
			continue

		case err, ok := <-watchChan:
			if !ok {
				// A watch only closes without sending an error
				// first if its ctx was cancelled. We only
				// cancel a previous watch after we've stopped
				// reading it.
				if ctx.Err() != nil {
					return nil // we're shutting down
				}
				return fmt.Errorf("the ack watch of %s closed", *name) // should not happen
			}
			if err != nil {
				return errwrap.Wrapf(err, "the ack watch of %s failed", *name)
			}

			// send event now...

		case <-ctx.Done():
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *IsDoneFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	name := args[0].Str()
	if name == "" {
		return nil, fmt.Errorf("can't use an empty name")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	select {
	case obj.input <- name:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	val, err := obj.world.RemoteAckGet(ctx, obj.init.Hostname, name)
	if err != nil {
		return nil, err
	}

	return &types.BoolValue{
		V: val != nil,
	}, nil
}
