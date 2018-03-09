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

// DOCS: https://godoc.org/github.com/coreos/go-systemd/dbus

package resources

import (
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/util"

	systemd "github.com/coreos/go-systemd/dbus" // change namespace
	systemdUtil "github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus" // namespace collides with systemd wrapper
	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("svc", func() Res { return &SvcRes{} })
}

// SvcRes is a service resource for systemd units.
type SvcRes struct {
	BaseRes `yaml:",inline"`
	State   string `yaml:"state"`   // state: running, stopped, undefined
	Startup string `yaml:"startup"` // enabled, disabled, undefined
	Session bool   `yaml:"session"` // user session (true) or system?
}

// Default returns some sensible defaults for this resource.
func (obj *SvcRes) Default() Res {
	return &SvcRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *SvcRes) Validate() error {
	if obj.State != "running" && obj.State != "stopped" && obj.State != "" {
		return fmt.Errorf("state must be either `running` or `stopped` or undefined")
	}
	if obj.Startup != "enabled" && obj.Startup != "disabled" && obj.Startup != "" {
		return fmt.Errorf("startup must be either `enabled` or `disabled` or undefined")
	}
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *SvcRes) Init() error {
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *SvcRes) Watch() error {
	// obj.Name: svc name
	if !systemdUtil.IsRunningSystemd() {
		return fmt.Errorf("systemd is not running")
	}

	var conn *systemd.Conn
	var err error
	if obj.Session {
		conn, err = systemd.NewUserConnection() // user session
	} else {
		// we want NewSystemConnection but New falls back to this
		conn, err = systemd.New() // needs root access
	}
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to systemd")
	}
	defer conn.Close()

	// if we share the bus with others, we will get each others messages!!
	bus, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}

	// XXX: will this detect new units?
	bus.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.freedesktop.systemd1.Manager',member='Reloading'")
	buschan := make(chan *dbus.Signal, 10)
	bus.Signal(buschan)

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var svc = fmt.Sprintf("%s.service", obj.Name) // systemd name
	var send = false                              // send event?
	var exit *error
	var invalid = false              // does the svc exist or not?
	var previous bool                // previous invalid value
	set := conn.NewSubscriptionSet() // no error should be returned
	subChannel, subErrors := set.Subscribe()
	var activeSet = false

	for {
		// XXX: watch for an event for new units...
		// XXX: detect if startup enabled/disabled value changes...

		previous = invalid
		invalid = false

		// firstly, does svc even exist or not?
		loadstate, err := conn.GetUnitProperty(svc, "LoadState")
		if err != nil {
			log.Printf("Failed to get property: %v", err)
			invalid = true
		}

		if !invalid {
			var notFound = (loadstate.Value == dbus.MakeVariant("not-found"))
			if notFound { // XXX: in the loop we'll handle changes better...
				log.Printf("Failed to find svc: %s", svc)
				invalid = true // XXX: ?
			}
		}

		if previous != invalid { // if invalid changed, send signal
			send = true
			obj.StateOK(false) // dirty
		}

		if invalid {
			log.Printf("Waiting for: %s", svc) // waiting for svc to appear...
			if activeSet {
				activeSet = false
				set.Remove(svc) // no return value should ever occur
			}

			select {
			case <-buschan: // XXX: wait for new units event to unstick
				// loop so that we can see the changed invalid signal
				log.Printf("Svc[%s]->DaemonReload()", svc)

			case event := <-obj.Events():
				if exit, send = obj.ReadEvent(event); exit != nil {
					return *exit // exit
				}
			}
		} else {
			if !activeSet {
				activeSet = true
				set.Add(svc) // no return value should ever occur
			}

			log.Printf("Watching: %s", svc) // attempting to watch...
			select {
			case event := <-subChannel:

				log.Printf("Svc event: %+v", event)
				// NOTE: the value returned is a map for some reason...
				if event[svc] != nil {
					// event[svc].ActiveState is not nil

					switch event[svc].ActiveState {
					case "active":
						log.Printf("Svc[%s]->Started", svc)
					case "inactive":
						log.Printf("Svc[%s]->Stopped", svc)
					case "reloading":
						log.Printf("Svc[%s]->Reloading", svc)
					case "failed":
						log.Printf("Svc[%s]->Failed", svc)
					case "deactivating":
						log.Printf("Svc[%s]->Deactivating", svc)
					default:
						return fmt.Errorf("Unknown svc state: %s", event[svc].ActiveState)
					}
				} else {
					// svc stopped (and ActiveState is nil...)
					log.Printf("Svc[%s]->Stopped", svc)
				}
				send = true
				obj.StateOK(false) // dirty

			case err := <-subErrors:
				return errwrap.Wrapf(err, "unknown %s error", obj)

			case event := <-obj.Events():
				if exit, send = obj.ReadEvent(event); exit != nil {
					return *exit // exit
				}
			}
		}

		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *SvcRes) CheckApply(apply bool) (checkOK bool, err error) {
	if !systemdUtil.IsRunningSystemd() {
		return false, fmt.Errorf("systemd is not running")
	}

	var conn *systemd.Conn
	if obj.Session {
		conn, err = systemd.NewUserConnection() // user session
	} else {
		// we want NewSystemConnection but New falls back to this
		conn, err = systemd.New() // needs root access
	}
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to connect to systemd")
	}
	defer conn.Close()

	var svc = fmt.Sprintf("%s.service", obj.Name) // systemd name

	loadstate, err := conn.GetUnitProperty(svc, "LoadState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get load state")
	}

	// NOTE: we have to compare variants with other variants, they are really strings...
	var notFound = (loadstate.Value == dbus.MakeVariant("not-found"))
	if notFound {
		return false, errwrap.Wrapf(err, "failed to find svc: %s", svc)
	}

	// XXX: check svc "enabled at boot" or not status...

	//conn.GetUnitProperties(svc)
	activestate, err := conn.GetUnitProperty(svc, "ActiveState")
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get active state")
	}

	var running = (activestate.Value == dbus.MakeVariant("active"))
	var stateOK = ((obj.State == "") || (obj.State == "running" && running) || (obj.State == "stopped" && !running))
	var startupOK = true        // XXX: DETECT AND SET
	var refresh = obj.Refresh() // do we have a pending reload to apply?

	if stateOK && startupOK && !refresh {
		return true, nil // we are in the correct state
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%s: Apply", obj)
	var files = []string{svc} // the svc represented in a list
	if obj.Startup == "enabled" {
		_, _, err = conn.EnableUnitFiles(files, false, true)

	} else if obj.Startup == "disabled" {
		_, err = conn.DisableUnitFiles(files, false)
	}

	if err != nil {
		return false, errwrap.Wrapf(err, "unable to change startup status")
	}

	// XXX: do we need to use a buffered channel here?
	result := make(chan string, 1) // catch result information

	if obj.State == "running" {
		_, err = conn.StartUnit(svc, "fail", result)
		if err != nil {
			return false, errwrap.Wrapf(err, "failed to start unit")
		}
		if refresh {
			log.Printf("%s: Skipping reload, due to pending start", obj)
		}
		refresh = false // we did a start, so a reload is not needed
	} else if obj.State == "stopped" {
		_, err = conn.StopUnit(svc, "fail", result)
		if err != nil {
			return false, errwrap.Wrapf(err, "failed to stop unit")
		}
		if refresh {
			log.Printf("%s: Skipping reload, due to pending stop", obj)
		}
		refresh = false // we did a stop, so a reload is not needed
	}

	status := <-result
	if &status == nil {
		return false, fmt.Errorf("systemd service action result is nil")
	}
	if status != "done" {
		return false, fmt.Errorf("unknown systemd return string: %v", status)
	}

	if refresh { // we need to reload the service
		// XXX: run a svc reload here!
		log.Printf("%s: Reloading...", obj)
	}

	// XXX: also set enabled on boot

	return false, nil // success
}

// SvcUID is the UID struct for SvcRes.
type SvcUID struct {
	// NOTE: there is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	BaseUID
	name string // the svc name
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *SvcUID) IFF(uid ResUID) bool {
	res, ok := uid.(*SvcUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// SvcResAutoEdges holds the state of the auto edge generator.
type SvcResAutoEdges struct {
	data    []ResUID
	pointer int
	found   bool
}

// Next returns the next automatic edge.
func (obj *SvcResAutoEdges) Next() []ResUID {
	if obj.found {
		log.Fatal("shouldn't be called anymore!")
	}
	if len(obj.data) == 0 { // check length for rare scenarios
		return nil
	}
	value := obj.data[obj.pointer]
	obj.pointer++
	return []ResUID{value} // we return one, even though api supports N
}

// Test gets results of the earlier Next() call, & returns if we should continue!
func (obj *SvcResAutoEdges) Test(input []bool) bool {
	// if there aren't any more remaining
	if len(obj.data) <= obj.pointer {
		return false
	}
	if obj.found { // already found, done!
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		log.Fatal("expecting a single value")
	}
	if input[0] { // if a match is found, we're done!
		obj.found = true // no more to find!
		return false
	}
	return true // keep going
}

// AutoEdges returns the AutoEdge interface. In this case the systemd units.
func (obj *SvcRes) AutoEdges() (AutoEdge, error) {
	var data []ResUID
	svcFiles := []string{
		fmt.Sprintf("/etc/systemd/system/%s.service", obj.Name),     // takes precedence
		fmt.Sprintf("/usr/lib/systemd/system/%s.service", obj.Name), // pkg default
	}
	for _, x := range svcFiles {
		var reversed = true
		data = append(data, &FileUID{
			BaseUID: BaseUID{
				Name:     obj.GetName(),
				Kind:     obj.GetKind(),
				Reversed: &reversed,
			},
			path: x, // what matters
		})
	}
	return &FileResAutoEdges{
		data:    data,
		pointer: 0,
		found:   false,
	}, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *SvcRes) UIDs() []ResUID {
	x := &SvcUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name, // svc name
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *SvcRes) GroupCmp(r Res) bool {
	_, ok := r.(*SvcRes)
	if !ok {
		return false
	}
	// TODO: depending on if the systemd service api allows batching, we
	// might be able to build this, although not sure how useful it is...
	// it might just eliminate parallelism be bunching up the graph
	return false // not possible atm
}

// Compare two resources and return if they are equivalent.
func (obj *SvcRes) Compare(r Res) bool {
	// we can only compare SvcRes to others of the same resource kind
	res, ok := r.(*SvcRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}
	if obj.Name != res.Name {
		return false
	}

	if obj.State != res.State {
		return false
	}
	if obj.Startup != res.Startup {
		return false
	}
	if obj.Session != res.Session {
		return false
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *SvcRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes SvcRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*SvcRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to SvcRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = SvcRes(raw) // restore from indirection with type conversion!
	return nil
}
