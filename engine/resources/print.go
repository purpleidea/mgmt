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
)

func init() {
	engine.RegisterResource("print", func() engine.Res { return &PrintRes{} })
}

// PrintRes is a resource that is useful for printing a message to the screen.
// It will also display a message when it receives a notification. It supports
// automatic grouping. When it displays a "different" message that what it had
// done previously, this is considered a state change. As a result it can even
// send a notification, and it will also internally see such a change as a new
// Watch event! This is mostly for consistency with the other resources and is
// not expected to be especially useful for anything other than learning mgmt!
type PrintRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Groupable
	traits.Recvable
	traits.Refreshable

	init *engine.Init

	// Msg is the message to display.
	Msg string `lang:"msg" yaml:"msg"`

	// RefreshOnly is an option that causes the message to be printed only
	// when notified by another resource. When set to true, this resource
	// cannot be autogrouped.
	RefreshOnly bool `lang:"refresh_only" yaml:"refresh_only"`

	last *string       // last printed value
	evch chan struct{} // a message got printed (changed) event
}

// Default returns some sensible defaults for this resource.
func (obj *PrintRes) Default() engine.Res {
	return &PrintRes{}
}

// Validate if the params passed in are valid data.
func (obj *PrintRes) Validate() error {
	return nil
}

// Init runs some startup code for this resource.
func (obj *PrintRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.evch = make(chan struct{}) // TODO: should it buffer to a size of 1?

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *PrintRes) Cleanup() error {
	close(obj.evch)
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *PrintRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-obj.evch:
			// event!

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply method for Print resource. Does nothing, returns happy!
func (obj *PrintRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if val, exists := obj.init.Recv()["msg"]; exists && val.Changed {
		// if we received on Msg, and it changed, log message
		obj.init.Logf("received `msg` of: %s", obj.Msg)
	}

	refresh := obj.init.Refresh()
	// Might as well always print this, since this res is kind of for debug.
	if refresh {
		obj.init.Logf("received refresh notification!")
	}

	changed := obj.last == nil || obj.Msg != *obj.last // did the message change since last run?
	last := obj.Msg                                    // make a copy of the current message
	obj.last = &last                                   // store the current message

	// We output a message if it changed and we're not in RefreshOnly mode,
	// or if we are in RefreshOnly mode and we received a refresh.
	display := (changed && !obj.RefreshOnly) || (refresh && obj.RefreshOnly)

	// add any grouped elements
	g := obj.GetGroup()
	for _, x := range g {
		print, ok := x.(*PrintRes) // convert from Res
		if !ok {
			// programming error
			panic(fmt.Sprintf("grouped member %v is not a %s", x, obj.Kind()))
		}
		if print.RefreshOnly {
			// programming error
			panic(fmt.Sprintf("grouped member %v should not be merged", x))
		}
		if len(print.GetGroup()) > 0 {
			// programming error
			panic(fmt.Sprintf("grouped member %v has nested autogrouping", x))
		}

		changed := print.last == nil || print.Msg != *print.last // did the message change since last run?
		last := print.Msg                                        // make a copy of the current message
		print.last = &last                                       // store the current message
		if changed {
			// arbitrary: if anything changes, display them all...
			display = true
		}
	}

	if !apply && display {
		return false, nil // technically we shouldn't write in noop mode
	}

	if apply && !display {
		return true, nil // done early, nothing to do!
	}

	// TODO: Our logf system should have a mechanism to lock/unlock so that
	// we could group all of this printing together with the same indent.
	if obj.Msg == "" {
		obj.init.Logf("<empty>")
	} else {
		obj.init.Logf("Msg: %s", obj.Msg)
	}
	for _, x := range g {
		print := x.(*PrintRes) // already safe
		if print.Msg == "" {
			obj.init.Logf("%s: <empty>", print)
		} else {
			obj.init.Logf("%s: Msg: %s", print, print.Msg)
		}
	}

	// What a peculiar resource after all! It turns out if we always return
	// (true, nil) then we will never send a refresh notification, so to be
	// consistent with users experimenting with that, we've got to actually
	// "apply" the state, which for this resource means "print the message"
	// which must then cause Watch to see that the state changed internally!
	select {
	case obj.evch <- struct{}{}:
		// send

	case <-ctx.Done():
		return false, ctx.Err()
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *PrintRes) Cmp(r engine.Res) error {
	// we can only compare PrintRes to others of the same resource kind
	res, ok := r.(*PrintRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Msg != res.Msg {
		return fmt.Errorf("the Msg differs")
	}
	if obj.RefreshOnly != res.RefreshOnly {
		return fmt.Errorf("the RefreshOnly differs")
	}

	return nil
}

// PrintUID is the UID struct for PrintRes.
type PrintUID struct {
	engine.BaseUID

	name string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *PrintRes) UIDs() []engine.ResUID {
	x := &PrintUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *PrintRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*PrintRes)
	if !ok {
		return fmt.Errorf("resource is not the same kind")
	}
	// we don't group if it's RefreshOnly: only the notifier may trigger
	if obj.RefreshOnly || res.RefreshOnly {
		return fmt.Errorf("resource uses RefreshOnly, it cannot be merged")
	}
	return nil // grouped together if we were asked to
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *PrintRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes PrintRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*PrintRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to PrintRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = PrintRes(raw) // restore from indirection with type conversion!
	return nil
}
