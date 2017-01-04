// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"encoding/gob"
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/event"
)

func init() {
	gob.Register(&NoopRes{})
}

// NoopRes is a no-op resource that does nothing.
type NoopRes struct {
	BaseRes `yaml:",inline"`
	Comment string `yaml:"comment"` // extra field for example purposes
}

// NewNoopRes is a constructor for this resource. It also calls Init() for you.
func NewNoopRes(name string) (*NoopRes, error) {
	obj := &NoopRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Comment: "",
	}
	return obj, obj.Init()
}

// Default returns some sensible defaults for this resource.
func (obj *NoopRes) Default() Res {
	return &NoopRes{}
}

// Validate if the params passed in are valid data.
func (obj *NoopRes) Validate() error {
	return nil
}

// Init runs some startup code for this resource.
func (obj *NoopRes) Init() error {
	obj.BaseRes.kind = "Noop"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *NoopRes) Watch(processChan chan *event.Event) error {
	cuid := obj.ConvergerUID() // get the converger uid used to report status

	// notify engine that we're running
	if err := obj.Running(processChan); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error
	for {
		obj.SetState(ResStateWatching) // reset
		select {
		case event := <-obj.Events():
			cuid.SetConverged(false)
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}

		case <-cuid.ConvergedTimer():
			cuid.SetConverged(true) // converged!
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event(processChan)
		}
	}
}

// CheckApply method for Noop resource. Does nothing, returns happy!
func (obj *NoopRes) CheckApply(apply bool) (checkOK bool, err error) {
	if obj.Refresh() {
		log.Printf("%s[%s]: Received a notification!", obj.Kind(), obj.GetName())
	}
	return true, nil // state is always okay
}

// NoopUID is the UID struct for NoopRes.
type NoopUID struct {
	BaseUID
	name string
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *NoopRes) AutoEdges() AutoEdge {
	return nil
}

// GetUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *NoopRes) GetUIDs() []ResUID {
	x := &NoopUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *NoopRes) GroupCmp(r Res) bool {
	_, ok := r.(*NoopRes)
	if !ok {
		// NOTE: technically we could group a noop into any other
		// resource, if that resource knew how to handle it, although,
		// since the mechanics of inter-kind resource grouping are
		// tricky, avoid doing this until there's a good reason.
		return false
	}
	return true // noop resources can always be grouped together!
}

// Compare two resources and return if they are equivalent.
func (obj *NoopRes) Compare(res Res) bool {
	switch res.(type) {
	// we can only compare NoopRes to others of the same resource
	case *NoopRes:
		res := res.(*NoopRes)
		// calling base Compare is unneeded for the noop res
		//if !obj.BaseRes.Compare(res) { // call base Compare
		//	return false
		//}
		if obj.Name != res.Name {
			return false
		}
	default:
		return false
	}
	return true
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
