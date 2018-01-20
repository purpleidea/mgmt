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
	"time"
)

func init() {
	RegisterResource("timer", func() Res { return &TimerRes{} })
}

// TimerRes is a timer resource for time based events. It outputs an event every
// interval seconds.
type TimerRes struct {
	BaseRes  `yaml:",inline"`
	Interval uint32 `yaml:"interval"` // interval between runs in seconds

	ticker *time.Ticker
}

// TimerUID is the UID struct for TimerRes.
type TimerUID struct {
	BaseUID
	name string
}

// Default returns some sensible defaults for this resource.
func (obj *TimerRes) Default() Res {
	return &TimerRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate the params that are passed to TimerRes.
func (obj *TimerRes) Validate() error {
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *TimerRes) Init() error {
	return obj.BaseRes.Init() // call base init, b/c we're overrriding
}

// newTicker creates a new ticker
func (obj *TimerRes) newTicker() *time.Ticker {
	return time.NewTicker(time.Duration(obj.Interval) * time.Second)
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TimerRes) Watch() error {
	// create a time.Ticker for the given interval
	obj.ticker = obj.newTicker()
	defer obj.ticker.Stop()

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false

	for {
		select {
		case <-obj.ticker.C: // received the timer event
			send = true
			log.Printf("%s: received tick", obj)

		case event := <-obj.Events():
			if exit, _ := obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply method for Timer resource. Triggers a timer reset on notify.
func (obj *TimerRes) CheckApply(apply bool) (bool, error) {
	// because there are no checks to run, this resource has a less
	// traditional pattern than what is seen in most resources...
	if !obj.Refresh() { // this works for apply || !apply
		return true, nil // state is always okay if no refresh to do
	} else if !apply { // we had a refresh to do
		return false, nil // therefore state is wrong
	}

	// reset the timer since apply && refresh
	obj.ticker.Stop()
	obj.ticker = obj.newTicker()
	return false, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *TimerRes) UIDs() []ResUID {
	x := &TimerUID{
		BaseUID: BaseUID{
			Name: obj.GetName(),
			Kind: obj.GetKind(),
		},
		name: obj.Name,
	}
	return []ResUID{x}
}

// Compare two resources and return if they are equivalent.
func (obj *TimerRes) Compare(r Res) bool {
	// we can only compare TimerRes to others of the same resource kind
	res, ok := r.(*TimerRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) {
		return false
	}
	if obj.Name != res.Name {
		return false
	}

	if obj.Interval != res.Interval {
		return false
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *TimerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TimerRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*TimerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TimerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TimerRes(raw) // restore from indirection with type conversion!
	return nil
}
