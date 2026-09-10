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

package corenetesphome

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	esphomeUtil "github.com/purpleidea/mgmt/util/esphome"
)

const (
	// ConnectedFuncName is the name this function is registered as.
	ConnectedFuncName = "connected"

	// connectedArgNameEndpoint is the name of the only arg.
	connectedArgNameEndpoint = "endpoint"
)

func init() {
	funcs.ModuleRegister(registeredModule, ConnectedFuncName, func() interfaces.Func { return &ConnectedFunc{} })
}

var _ interfaces.StreamableFunc = &ConnectedFunc{} // ensure it meets this expectation

// ConnectedFunc is a function that returns whether we consider the named
// esphome device healthy. It takes the endpoint (the name of the
// esphome:endpoint resource that describes the device) and returns false until
// that resource has published its connection info and the device is reachable.
// With a persistent connection (interval zero) it means the connection is
// currently up, and with a polling endpoint it means the most recent poll
// succeeded.
type ConnectedFunc struct {
	interfaces.Textarea

	init *interfaces.Init

	input    chan string // stream of inputs
	endpoint *string     // the active endpoint

	session *esphomeUtil.Session // only used by Stream
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ConnectedFunc) String() string {
	return ModuleName + "." + ConnectedFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ConnectedFunc) ArgGen(index int) (string, error) {
	seq := []string{connectedArgNameEndpoint}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// sig is a helper to return the static type signature of this function.
func (obj *ConnectedFunc) sig() *types.Type {
	// func(endpoint str) bool
	return types.NewType(fmt.Sprintf("func(%s str) bool", connectedArgNameEndpoint))
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ConnectedFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ConnectedFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // depends on the local API and the device
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  obj.sig(),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ConnectedFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan string)
	return nil
}

// Copy is implemented so that the type value is not lost if we copy this
// function.
func (obj *ConnectedFunc) Copy() interfaces.Func {
	return &ConnectedFunc{
		Textarea: obj.Textarea,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Stream returns the changing values that this func has over time.
func (obj *ConnectedFunc) Stream(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // important so that we cleanup the watches when exiting
	defer func() {
		if obj.session != nil {
			obj.session.Release()
			obj.session = nil
		}
	}()

	var bridgeChan, sessionChan chan struct{} // nil chans block forever
	for {
		select {
		case endpoint, ok := <-obj.input:
			if !ok {
				obj.input = nil // don't infinite loop back
				return fmt.Errorf("unexpected close")
			}

			if obj.endpoint != nil && *obj.endpoint == endpoint {
				continue // nothing changed
			}

			// We don't support changing the endpoint over time,
			// since the watches are set up against a single one.
			if obj.endpoint == nil {
				obj.endpoint = &endpoint // store it
				var err error
				// Don't send a value right away, wait for the
				// first watch startup events to get one!
				bridgeChan, err = obj.init.Local.BridgeWatch(ctx, esphomeUtil.BridgeNamespace, endpoint)
				if err != nil {
					return err
				}
				obj.session = esphomeUtil.SessionReserve(endpoint)
				sessionChan, err = obj.session.Watch(ctx)
				if err != nil {
					return err
				}
				continue // we get values on the watch chans, not here!
			}

			// *obj.endpoint != endpoint
			return fmt.Errorf("can't change endpoint, previously: `%s`", *obj.endpoint)

		case _, ok := <-bridgeChan:
			if !ok { // closed
				return nil
			}

			// Pass whatever the endpoint resource published (or nil
			// if it unpublished) into the shared session.
			val, err := obj.init.Local.BridgeGet(ctx, esphomeUtil.BridgeNamespace, *obj.endpoint)
			if err != nil {
				return err
			}
			info, _ := val.(*esphomeUtil.ConnInfo) // nil is fine
			obj.session.Configure(info)

			if err := obj.init.Event(ctx); err != nil { // send event
				return err
			}

		case _, ok := <-sessionChan:
			if !ok { // closed
				return nil
			}

			if err := obj.init.Event(ctx); err != nil { // send event
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ConnectedFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	endpoint := args[0].Str()
	if endpoint == "" {
		return nil, fmt.Errorf("can't use an empty endpoint")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	select {
	case obj.input <- endpoint:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// This is cheap in-memory refcounting, and it shares the session that
	// our Stream (and everyone else naming this endpoint) holds open.
	session := esphomeUtil.SessionReserve(endpoint)
	defer session.Release()

	return &types.BoolValue{V: session.Connected()}, nil
}
