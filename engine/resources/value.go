// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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
	// This is similar to the KV resource, but it makes sense not to merge.

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

	// Store specifies that we should store this value locally in our cache,
	// so that if we have a cold startup, that it comes back instantly, even
	// before we might get the current value from send/recv. This does not
	// override any value passed in directly as a field parameter. This is
	// mostly useful if we're using the value.get() function, and we don't
	// want an initial stale value for a subsequent run. Remember that
	// functions run before resources do!
	//
	// At the moment, this is permanently enabled until someone has a good
	// reason why we wouldn't want it on.
	//Store bool

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
	received := false
	different := false
	checkOK := false
	if val, exists := obj.init.Recv()["any"]; exists && val.Changed {
		// if we received on Any, and it changed, invalidate the cache!
		obj.init.Logf("CheckApply: received on `any`")
		obj.isSet = true // we received something
		obj.cachedAny = obj.Any
		received = true // we'll always need to send below when we recv
	}

	// TODO: can we ever return before `if !apply` because "state is okay"?

	if val, err := obj.init.Local.ValueGet(ctx, obj.Name()); err != nil {
		return false, err

	} else if obj.cachedAny == nil {
		different = !(val == nil)
		if !different && !received {
			//return true, nil // no values, state is okay
			checkOK = true
		}

	} else if different = !reflect.DeepEqual(val, *obj.cachedAny); !different && !received {
		//return true, nil // same values, state is okay
		// If we return early here, then we won't run the Send/Recv and
		// we'll get an engine error like:
		// `could not SendRecv: received nil value from: value[hello]`
		// so instead, make sure we always send/recv at the end.
		// XXX: verify this is a reasonable behaviour of the send/recv
		// engine; that is, that we require to always send on each
		// CheckApply. It seems it might be sensible that if our state
		// doesn't change, we shouldn't need to re-send.
		checkOK = true
	}

	if !apply { // XXX: does this break send/recv if we end early?
		return checkOK, nil
	}

	if different { // don't cause unnecessary events!
		if obj.cachedAny == nil {
			// pass nil to delete!
			if err := obj.init.Local.ValueSet(ctx, obj.Name(), nil); err != nil {
				return false, err
			}
		} else {
			if err := obj.init.Local.ValueSet(ctx, obj.Name(), *obj.cachedAny); err != nil {
				return false, err
			}
		}
	}

	// send
	//if obj.cachedAny != nil { // TODO: okay to send if value got removed too?
	if err := obj.init.Send(&ValueSends{
		Any: obj.cachedAny,
	}); err != nil {
		return false, err
	}
	//}

	return checkOK, nil
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

	//if obj.Store != res.Store {
	//	return fmt.Errorf("the Store field differs")
	//}

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
