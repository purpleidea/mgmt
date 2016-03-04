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

// DOCS: https://godoc.org/github.com/coreos/go-systemd/dbus

package main

import (
	"errors"
	"fmt"
	systemd "github.com/coreos/go-systemd/dbus" // change namespace
	systemdUtil "github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus" // namespace collides with systemd wrapper
	"log"
)

type SvcRes struct {
	BaseRes `yaml:",inline"`
	State   string `yaml:"state"`   // state: running, stopped, undefined
	Startup string `yaml:"startup"` // enabled, disabled, undefined
}

func NewSvcRes(name, state, startup string) *SvcRes {
	obj := &SvcRes{
		BaseRes: BaseRes{
			Name: name,
		},
		State:   state,
		Startup: startup,
	}
	obj.Init()
	return obj
}

func (obj *SvcRes) Init() {
	obj.BaseRes.kind = "Svc"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

func (obj *SvcRes) Validate() bool {
	if obj.State != "running" && obj.State != "stopped" && obj.State != "" {
		return false
	}
	if obj.Startup != "enabled" && obj.Startup != "disabled" && obj.Startup != "" {
		return false
	}
	return true
}

// Service watcher
func (obj *SvcRes) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	// obj.Name: svc name
	//vertex := obj.GetVertex()         // stored with SetVertex
	if !systemdUtil.IsRunningSystemd() {
		log.Fatal("Systemd is not running.")
	}

	conn, err := systemd.NewSystemdConnection() // needs root access
	if err != nil {
		log.Fatal("Failed to connect to systemd: ", err)
	}
	defer conn.Close()

	// if we share the bus with others, we will get each others messages!!
	bus, err := SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		log.Fatal("Failed to connect to bus: ", err)
	}

	// XXX: will this detect new units?
	bus.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.freedesktop.systemd1.Manager',member='Reloading'")
	buschan := make(chan *dbus.Signal, 10)
	bus.Signal(buschan)

	var svc = fmt.Sprintf("%v.service", obj.Name) // systemd name
	var send = false                              // send event?
	var exit = false
	var dirty = false
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
				log.Printf("Failed to find svc: %v", svc)
				invalid = true // XXX ?
			}
		}

		if previous != invalid { // if invalid changed, send signal
			send = true
			dirty = true
		}

		if invalid {
			log.Printf("Waiting for: %v", svc) // waiting for svc to appear...
			if activeSet {
				activeSet = false
				set.Remove(svc) // no return value should ever occur
			}

			obj.SetState(resStateWatching) // reset
			select {
			case _ = <-buschan: // XXX wait for new units event to unstick
				obj.SetConvergedState(resConvergedNil)
				// loop so that we can see the changed invalid signal
				log.Printf("Svc[%v]->DaemonReload()", svc)

			case event := <-obj.events:
				obj.SetConvergedState(resConvergedNil)
				if exit, send = obj.ReadEvent(&event); exit {
					return // exit
				}
				if event.GetActivity() {
					dirty = true
				}

			case _ = <-TimeAfterOrBlock(obj.ctimeout):
				obj.SetConvergedState(resConvergedTimeout)
				obj.converged <- true
				continue
			}
		} else {
			if !activeSet {
				activeSet = true
				set.Add(svc) // no return value should ever occur
			}

			log.Printf("Watching: %v", svc) // attempting to watch...
			obj.SetState(resStateWatching)  // reset
			select {
			case event := <-subChannel:

				log.Printf("Svc event: %+v", event)
				// NOTE: the value returned is a map for some reason...
				if event[svc] != nil {
					// event[svc].ActiveState is not nil
					if event[svc].ActiveState == "active" {
						log.Printf("Svc[%v]->Started()", svc)
					} else if event[svc].ActiveState == "inactive" {
						log.Printf("Svc[%v]->Stopped!()", svc)
					} else {
						log.Fatal("Unknown svc state: ", event[svc].ActiveState)
					}
				} else {
					// svc stopped (and ActiveState is nil...)
					log.Printf("Svc[%v]->Stopped", svc)
				}
				send = true
				dirty = true

			case err := <-subErrors:
				obj.SetConvergedState(resConvergedNil) // XXX ?
				log.Println("error:", err)
				log.Fatal(err)
				//vertex.events <- fmt.Sprintf("svc: %v", "error") // XXX: how should we handle errors?

			case event := <-obj.events:
				obj.SetConvergedState(resConvergedNil)
				if exit, send = obj.ReadEvent(&event); exit {
					return // exit
				}
				if event.GetActivity() {
					dirty = true
				}
			}
		}

		if send {
			send = false
			if dirty {
				dirty = false
				obj.isStateOK = false // something made state dirty
			}
			Process(obj) // XXX: rename this function
		}

	}
}

func (obj *SvcRes) CheckApply(apply bool) (stateok bool, err error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.isStateOK { // cache the state
		return true, nil
	}

	if !systemdUtil.IsRunningSystemd() {
		return false, errors.New("Systemd is not running.")
	}

	conn, err := systemd.NewSystemdConnection() // needs root access
	if err != nil {
		return false, errors.New(fmt.Sprintf("Failed to connect to systemd: %v", err))
	}
	defer conn.Close()

	var svc = fmt.Sprintf("%v.service", obj.Name) // systemd name

	loadstate, err := conn.GetUnitProperty(svc, "LoadState")
	if err != nil {
		return false, errors.New(fmt.Sprintf("Failed to get load state: %v", err))
	}

	// NOTE: we have to compare variants with other variants, they are really strings...
	var notFound = (loadstate.Value == dbus.MakeVariant("not-found"))
	if notFound {
		return false, errors.New(fmt.Sprintf("Failed to find svc: %v", svc))
	}

	// XXX: check svc "enabled at boot" or not status...

	//conn.GetUnitProperties(svc)
	activestate, err := conn.GetUnitProperty(svc, "ActiveState")
	if err != nil {
		return false, errors.New(fmt.Sprintf("Failed to get active state: %v", err))
	}

	var running = (activestate.Value == dbus.MakeVariant("active"))
	var stateOK = ((obj.State == "") || (obj.State == "running" && running) || (obj.State == "stopped" && !running))
	var startupOK = true // XXX DETECT AND SET

	if stateOK && startupOK {
		return true, nil // we are in the correct state
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%v[%v]: Apply", obj.Kind(), obj.GetName())
	var files = []string{svc} // the svc represented in a list
	if obj.Startup == "enabled" {
		_, _, err = conn.EnableUnitFiles(files, false, true)

	} else if obj.Startup == "disabled" {
		_, err = conn.DisableUnitFiles(files, false)
	}

	if err != nil {
		return false, errors.New(fmt.Sprintf("Unable to change startup status: %v", err))
	}

	// XXX: do we need to use a buffered channel here?
	result := make(chan string, 1) // catch result information

	if obj.State == "running" {
		_, err = conn.StartUnit(svc, "fail", result)
		if err != nil {
			return false, errors.New(fmt.Sprintf("Failed to start unit: %v", err))
		}
	} else if obj.State == "stopped" {
		_, err = conn.StopUnit(svc, "fail", result)
		if err != nil {
			return false, errors.New(fmt.Sprintf("Failed to stop unit: %v", err))
		}
	}

	status := <-result
	if &status == nil {
		return false, errors.New("Systemd service action result is nil")
	}
	if status != "done" {
		return false, errors.New(fmt.Sprintf("Unknown systemd return string: %v", status))
	}

	// XXX: also set enabled on boot

	return false, nil // success
}

type SvcUUID struct {
	BaseUUID
}

// if and only if they are equivalent, return true
// if they are not equivalent, return false
func (obj *SvcUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*SvcUUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

func (obj *SvcRes) AutoEdges() AutoEdge {
	// TODO: add auto edges to the files that provide the service files,
	// which might come from a pkg resource perhaps!
	return nil
}

// include all params to make a unique identification of this object
func (obj *SvcRes) GetUUIDs() []ResUUID {
	x := &SvcUUID{BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()}}
	return []ResUUID{x}

}

func (obj *SvcRes) Compare(res Res) bool {
	switch res.(type) {
	case *SvcRes:
		res := res.(*SvcRes)
		if obj.Name != res.Name {
			return false
		}
		if obj.State != res.State {
			return false
		}
		if obj.Startup != res.Startup {
			return false
		}
	default:
		return false
	}
	return true
}
