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

package resources

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/lang/interfaces"
)

func init() {
	// It starts with an underscore so a user can't add it manually.
	engine.RegisterResource(interfaces.PanicResKind, func() engine.Res { return &PanicRes{} })
}

// PanicRes is a no-op resource that does nothing as quietly as possible. One of
// these will be added the graph if you use the panic function. (Even when it is
// in a non-panic mode.) This is possibly the simplest resource that exists, and
// in fact, every time it is used, it will always have the same "name" value. It
// is only used so that there is a valid destination for the panic function.
type PanicRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init
}

// Default returns some sensible defaults for this resource.
func (obj *PanicRes) Default() engine.Res {
	return &PanicRes{}
}

// Validate if the params passed in are valid data.
func (obj *PanicRes) Validate() error {
	return nil
}

// Init runs some startup code for this resource.
func (obj *PanicRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *PanicRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *PanicRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply method for Panic resource. Does nothing, returns happy!
func (obj *PanicRes) CheckApply(context.Context, bool) (bool, error) {
	return true, nil // state is always okay
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *PanicRes) Cmp(r engine.Res) error {
	// we can only compare PanicRes to others of the same resource kind
	_, ok := r.(*PanicRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *PanicRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes PanicRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*PanicRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to PanicRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = PanicRes(raw) // restore from indirection with type conversion!
	return nil
}
