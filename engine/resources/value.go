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

package resources

import (
	"context"
	"fmt"
	"reflect"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

func init() {
	engine.RegisterResource("value", func() engine.Res { return &ValueRes{} })
}

// ValueRes is a no-op resource that accepts a value normally or via send/recv
// and it sends it via send/recv as well.
//
// XXX: intermediate chained values being used for send/recv must have a
// temporary placeholder value set or we'll get an invalid value error. This can
// be fixed eventually when we expand the resource API. See the Default method
// of this resource for more information.
type ValueRes struct {
	traits.Base // add the base methods without re-implementation

	//traits.Groupable // TODO: this is doable, but probably not very useful
	traits.Sendable
	traits.Recvable

	init *engine.Init

	// Any is an arbitrary value to store in this resource. It can also be
	// sent via send/recv and received by the same mechanism as well. The
	// received value overwrites this value for the lifetime of the
	// resource. It is interface{} because it can hold any type. It has
	// pointer because it is only set if an actual value exists.
	Any *interface{} `lang:"any" yaml:"any"`

	cachedAny *interface{}
	isSet     bool
}

// Default returns some sensible defaults for this resource.
func (obj *ValueRes) Default() engine.Res {
	// XXX: once we have a "SetType" style method for unifying this resource
	// with the correct type for the interface{} fields, we should put the
	// zero values of those types for those fields here... This will allow
	// send/recv to not require an empty placeholder to type check.
	return &ValueRes{
		Any: nil, // XXX: use the zero value of the actual chosen type
	}
}

// Validate if the params passed in are valid data.
func (obj *ValueRes) Validate() error {
	return nil
}

// Init runs some startup code for this resource.
func (obj *ValueRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *ValueRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ValueRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply method for Value resource. Does nothing, returns happy!
func (obj *ValueRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// NOTE: all send/recv change notifications *must* be processed before
	// there is a possibility of failure in CheckApply. This is because if
	// we fail (and possibly run again) the subsequent send->recv transfer
	// might not have a new value to copy, and therefore we won't see this
	// notification of change. Therefore, it is important to process these
	// promptly, if they must not be lost, such as for cache invalidation.
	if !obj.isSet {
		obj.cachedAny = obj.Any // store anything we have if any
	}
	if val, exists := obj.init.Recv()["Any"]; exists && val.Changed {
		// if we received on Any, and it changed, invalidate the cache!
		obj.init.Logf("CheckApply: received on `Any`")
		obj.isSet = true // we received something
		obj.cachedAny = obj.Any
	}

	// send
	if obj.cachedAny != nil {
		if err := obj.init.Send(&ValueSends{
			Any: obj.cachedAny,
		}); err != nil {
			return false, err
		}
	}

	return true, nil // state is always okay
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *ValueRes) Cmp(r engine.Res) error {
	// we can only compare ValueRes to others of the same resource kind
	res, ok := r.(*ValueRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if !reflect.DeepEqual(obj.Any, res.Any) {
		return fmt.Errorf("the Any field differs")
	}

	return nil
}

// ValueSends is the struct of data which is sent after a successful Apply.
type ValueSends struct {
	// Any is the generated value being sent. It is interface{} because it
	// can hold any type. It has pointer because it is only set if an actual
	// value is actually being sent.
	Any *interface{} `lang:"any"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *ValueRes) Sends() interface{} {
	return &ValueSends{
		Any: nil,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *ValueRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes ValueRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*ValueRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to ValueRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = ValueRes(raw) // restore from indirection with type conversion!
	return nil
}
