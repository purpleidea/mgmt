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
	"log"
)

func init() {
	RegisterResource("print", func() Res { return &PrintRes{} })
}

// PrintRes is a resource that is useful for printing a message to the screen.
// It will also display a message when it receives a notification. It supports
// automatic grouping.
type PrintRes struct {
	BaseRes `lang:"" yaml:",inline"`

	Msg string `lang:"msg" yaml:"msg"` // the message to display
}

// Default returns some sensible defaults for this resource.
func (obj *PrintRes) Default() Res {
	return &PrintRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *PrintRes) Validate() error {
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *PrintRes) Init() error {
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *PrintRes) Watch() error {
	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error
	for {
		select {
		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply method for Print resource. Does nothing, returns happy!
func (obj *PrintRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: CheckApply: %t", obj, apply)
	if val, exists := obj.Recv["Msg"]; exists && val.Changed {
		// if we received on Msg, and it changed, log message
		log.Printf("CheckApply: Received `Msg` of: %s", obj.Msg)
	}

	if obj.Refresh() {
		log.Printf("%s: Received a notification!", obj)
	}
	log.Printf("%s: Msg: %s", obj, obj.Msg)
	if g := obj.GetGroup(); len(g) > 0 { // add any grouped elements
		for _, x := range g {
			print, ok := x.(*PrintRes) // convert from Res
			if !ok {
				log.Fatalf("grouped member %v is not a %s", x, obj.GetKind())
			}
			log.Printf("%s: Msg: %s", print, print.Msg)
		}
	}
	return true, nil // state is always okay
}

// PrintUID is the UID struct for PrintRes.
type PrintUID struct {
	BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *PrintRes) UIDs() []ResUID {
	x := &PrintUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *PrintRes) GroupCmp(r Res) bool {
	_, ok := r.(*PrintRes)
	if !ok {
		return false
	}
	return true // grouped together if we were asked to
}

// Compare two resources and return if they are equivalent.
func (obj *PrintRes) Compare(r Res) bool {
	// we can only compare PrintRes to others of the same resource kind
	res, ok := r.(*PrintRes)
	if !ok {
		return false
	}
	// calling base Compare is probably unneeded for the print res, but do it
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}
	if obj.Name != res.Name {
		return false
	}

	if obj.Msg != res.Msg {
		return false
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
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
