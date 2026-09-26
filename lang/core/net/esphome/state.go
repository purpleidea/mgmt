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
	// stateArgNameEndpoint is the name of the first arg, which is the name
	// of the esphome:endpoint resource that describes the device.
	stateArgNameEndpoint = "endpoint"

	// stateArgNameID is the name of the second arg, which is the object_id
	// of the entity on the device.
	stateArgNameID = "id"
)

func init() {
	funcs.ModuleRegister(registeredModule, esphomeUtil.DomainBinarySensor, func() interfaces.Func {
		return &StateFunc{Domain: esphomeUtil.DomainBinarySensor}
	})
	funcs.ModuleRegister(registeredModule, esphomeUtil.DomainSensor, func() interfaces.Func {
		return &StateFunc{Domain: esphomeUtil.DomainSensor}
	})
	funcs.ModuleRegister(registeredModule, esphomeUtil.DomainTextSensor, func() interfaces.Func {
		return &StateFunc{Domain: esphomeUtil.DomainTextSensor}
	})
}

var _ interfaces.StreamableFunc = &StateFunc{} // ensure it meets this expectation

// stateArgs are the two input values that select which entity we stream.
type stateArgs struct {
	endpoint string
	id       string
}

// StateFunc is a function that streams the live state of one entity of an
// esphome device. It takes the endpoint (the name of the esphome:endpoint
// resource that describes the device) and the exact entity name or legacy
// object_id. The registered variant selects the entity domain and the return
// type: a binary_sensor returns bool, a sensor returns float, and a text_sensor
// returns str. A gpio input pin shows up as a binary_sensor entity, so that
// variant is how you read gpio inputs with events. Until the endpoint resource
// has published its connection info, and until the device has reported the
// entity, this returns the zero value of the type, and it also does so if the
// device reports the state as missing.
type StateFunc struct {
	interfaces.Textarea

	// Domain is the entity domain that this function instance reads. It
	// determines the return type, and it is set at registration time.
	Domain string

	init *interfaces.Init

	input chan *stateArgs // stream of inputs
	args  *stateArgs      // the active args

	session *esphomeUtil.Session // only used by Stream
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *StateFunc) String() string {
	return ModuleName + "." + obj.Domain
}

// ArgGen returns the Nth arg name for this function.
func (obj *StateFunc) ArgGen(index int) (string, error) {
	seq := []string{stateArgNameEndpoint, stateArgNameID}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// outKind is a helper to return the return type of this function variant.
func (obj *StateFunc) outKind() string {
	switch obj.Domain {
	case esphomeUtil.DomainBinarySensor:
		return "bool"
	case esphomeUtil.DomainSensor:
		return "float"
	case esphomeUtil.DomainTextSensor:
		return "str"
	}
	return "" // invalid, caught by Validate
}

// sig is a helper to return the static type signature of this function.
func (obj *StateFunc) sig() *types.Type {
	// func(endpoint str, id str) bool (or float, or str)
	return types.NewType(fmt.Sprintf(
		"func(%s str, %s str) %s",
		stateArgNameEndpoint,
		stateArgNameID,
		obj.outKind(),
	))
}

// Validate makes sure we've built our struct properly.
func (obj *StateFunc) Validate() error {
	if obj.outKind() == "" {
		return fmt.Errorf("unsupported domain: %s", obj.Domain)
	}
	return nil
}

// Info returns some static info about itself.
func (obj *StateFunc) Info() *interfaces.Info {
	var sig *types.Type
	if obj.Validate() == nil { // avoid panic in NewType
		sig = obj.sig()
	}
	return &interfaces.Info{
		Pure: false, // depends on the local API and the device
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *StateFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan *stateArgs)
	return nil
}

// Copy is implemented so that the Domain value is not lost if we copy this
// function.
func (obj *StateFunc) Copy() interfaces.Func {
	return &StateFunc{
		Textarea: obj.Textarea,
		Domain:   obj.Domain,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Stream returns the changing values that this func has over time. It sets up a
// watch on the bridge (so we hear about the endpoint resource publishing,
// changing, or unpublishing its connection info) and a watch on the shared
// session (so we hear about entity states changing), and sends an event on
// either one.
func (obj *StateFunc) Stream(ctx context.Context) error {
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
		case args, ok := <-obj.input:
			if !ok {
				obj.input = nil // don't infinite loop back
				return fmt.Errorf("unexpected close")
			}

			if obj.args != nil && *obj.args == *args {
				continue // nothing changed
			}

			// We don't support changing the args over time, since
			// the watches are set up against a single endpoint.
			if obj.args == nil {
				obj.args = args // store it
				var err error
				// Don't send a value right away, wait for the
				// first watch startup events to get one!
				bridgeChan, err = obj.init.Local.BridgeWatch(ctx, esphomeUtil.BridgeNamespace, args.endpoint)
				if err != nil {
					return err
				}
				obj.session = esphomeUtil.SessionReserve(args.endpoint)
				sessionChan, err = obj.session.Watch(ctx)
				if err != nil {
					return err
				}
				continue // we get values on the watch chans, not here!
			}

			// *obj.args != *args
			return fmt.Errorf("can't change args, previously: `%s`, `%s`", obj.args.endpoint, obj.args.id)

		case _, ok := <-bridgeChan:
			if !ok { // closed
				return nil
			}

			// Pass whatever the endpoint resource published (or nil
			// if it unpublished) into the shared session.
			val, err := obj.init.Local.BridgeGet(ctx, esphomeUtil.BridgeNamespace, obj.args.endpoint)
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
func (obj *StateFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("not enough args")
	}
	endpoint := args[0].Str()
	if endpoint == "" {
		return nil, fmt.Errorf("can't use an empty endpoint")
	}
	id := args[1].Str()
	if id == "" {
		return nil, fmt.Errorf("can't use an empty id")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	select {
	case obj.input <- &stateArgs{endpoint: endpoint, id: id}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// This is cheap in-memory refcounting, and it shares the session that
	// our Stream (and everyone else naming this endpoint) holds open.
	session := esphomeUtil.SessionReserve(endpoint)
	defer session.Release()

	state := session.State(id)
	if state == nil || state.Missing || state.Domain != obj.Domain {
		// Nothing has happened yet, the entity is unknown, or it has
		// no valid reading: return the zero value for now.
		switch obj.Domain {
		case esphomeUtil.DomainBinarySensor:
			return &types.BoolValue{V: false}, nil
		case esphomeUtil.DomainSensor:
			return &types.FloatValue{V: 0}, nil
		case esphomeUtil.DomainTextSensor:
			return &types.StrValue{V: ""}, nil
		}
		return nil, fmt.Errorf("unsupported domain: %s", obj.Domain)
	}

	switch obj.Domain {
	case esphomeUtil.DomainBinarySensor:
		return &types.BoolValue{V: state.Bool}, nil
	case esphomeUtil.DomainSensor:
		return &types.FloatValue{V: state.Float}, nil
	case esphomeUtil.DomainTextSensor:
		return &types.StrValue{V: state.Str}, nil
	}
	return nil, fmt.Errorf("unsupported domain: %s", obj.Domain)
}
