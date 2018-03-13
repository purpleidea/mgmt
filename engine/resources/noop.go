// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

func init() {
	engine.RegisterResource("noop", func() engine.Res { return &NoopRes{} })
}

// NoopRes is a no-op resource that does nothing.
type NoopRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Groupable
	traits.Refreshable

	init *engine.Init

	Comment string `lang:"comment" yaml:"comment"` // extra field for example purposes
}

// Default returns some sensible defaults for this resource.
func (obj *NoopRes) Default() engine.Res {
	return &NoopRes{}
}

// Validate if the params passed in are valid data.
func (obj *NoopRes) Validate() error {
	return nil
}

// Init runs some startup code for this resource.
func (obj *NoopRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *NoopRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *NoopRes) Watch() error {
	// notify engine that we're running
	if err := obj.init.Running(); err != nil {
		return err // exit if requested
	}

	var send = false // send event?
	for {
		select {
		case event, ok := <-obj.init.Events:
			if !ok {
				return nil
			}
			if err := obj.init.Read(event); err != nil {
				return err
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			if err := obj.init.Event(); err != nil {
				return err // exit if requested
			}
		}
	}
}

// CheckApply method for Noop resource. Does nothing, returns happy!
func (obj *NoopRes) CheckApply(apply bool) (checkOK bool, err error) {
	if obj.init.Refresh() {
		obj.init.Logf("received a notification!")
	}
	return true, nil // state is always okay
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *NoopRes) Cmp(r engine.Res) error {
	// we can only compare NoopRes to others of the same resource kind
	res, ok := r.(*NoopRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Comment != res.Comment {
		return fmt.Errorf("the Comment differs")
	}

	return nil
}

// NoopUID is the UID struct for NoopRes.
type NoopUID struct {
	engine.BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *NoopRes) UIDs() []engine.ResUID {
	x := &NoopUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *NoopRes) GroupCmp(r engine.GroupableRes) error {
	_, ok := r.(*NoopRes)
	if !ok {
		// NOTE: technically we could group a noop into any other
		// resource, if that resource knew how to handle it, although,
		// since the mechanics of inter-kind resource grouping are
		// tricky, avoid doing this until there's a good reason.
		return fmt.Errorf("resource is not the same kind")
	}
	return nil // noop resources can always be grouped together!
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *NoopRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes NoopRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*NoopRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to NoopRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = NoopRes(raw) // restore from indirection with type conversion!
	return nil
}
