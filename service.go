// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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

// NOTE: docs are found at: https://godoc.org/github.com/coreos/go-systemd/dbus

package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	systemd "github.com/coreos/go-systemd/dbus" // change namespace
	"github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus" // namespace collides with systemd wrapper
	"log"
)

type ServiceType struct {
	uuid    string
	Type    string      // always "service"
	Name    string      // name variable
	Events  chan string // FIXME: eventually a struct for the event?
	State   string      // state: running, stopped
	Startup string      // enabled, disabled, undefined
}

func NewServiceType(name, state, startup string) *ServiceType {
	return &ServiceType{
		uuid:    uuid.New(),
		Type:    "service",
		Name:    name,
		Events:  make(chan string, 1), // XXX: chan size?
		State:   state,
		Startup: startup,
	}
}

// Service watcher
func (obj ServiceType) Watch(v *Vertex) {
	// obj.Name: service name

	if !util.IsRunningSystemd() {
		log.Fatal("Systemd is not running.")
	}

	conn, err := systemd.NewSystemdConnection() // needs root access
	if err != nil {
		log.Fatal("Failed to connect to systemd: ", err)
	}
	defer conn.Close()

	bus, err := dbus.SystemBus()
	if err != nil {
		log.Fatal("Failed to connect to bus: %v\n", err)
	}

	// XXX: will this detect new units?
	bus.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='org.freedesktop.systemd1.Manager',member='Reloading'")
	buschan := make(chan *dbus.Signal, 10)
	bus.Signal(buschan)

	var service = fmt.Sprintf("%v.service", obj.Name) // systemd name
	var send = false                                  // send event?
	var invalid = false                               // does the service exist or not?
	var previous bool                                 // previous invalid value
	set := conn.NewSubscriptionSet()                  // no error should be returned
	subChannel, subErrors := set.Subscribe()
	var activeSet = false

	for {
		// XXX: watch for an event for new units...
		// XXX: detect if startup enabled/disabled value changes...

		previous = invalid
		invalid = false

		// firstly, does service even exist or not?
		loadstate, err := conn.GetUnitProperty(service, "LoadState")
		if err != nil {
			log.Printf("Failed to get property: %v\n", err)
			invalid = true
		}

		if !invalid {
			var notFound = (loadstate.Value == dbus.MakeVariant("not-found"))
			if notFound { // XXX: in the loop we'll handle changes better...
				log.Printf("Failed to find service: %v\n", service)
				invalid = true // XXX ?
			}
		}

		if previous != invalid { // if invalid changed, send signal
			send = true
		}

		if invalid {
			log.Printf("Waiting for: %v\n", service) // waiting for service to appear...
			if activeSet {
				activeSet = false
				set.Remove(service) // no return value should ever occur
			}

			select {
			case _ = <-buschan: // XXX wait for new units event to unstick
				// loop so that we can see the changed invalid signal
				log.Printf("Service[%v]->DaemonReload()\n", service)

			case exit := <-obj.Events:
				if exit == "exit" {
					return
				} else {
					log.Fatal("Unknown event: %v\n", exit)
				}
			}
		} else {
			if !activeSet {
				activeSet = true
				set.Add(service) // no return value should ever occur
			}

			log.Printf("Watching: %v\n", service) // attempting to watch...
			select {
			case event := <-subChannel:

				log.Printf("Service event: %+v\n", event)
				// NOTE: the value returned is a map for some reason...
				if event[service] != nil {
					// event[service].ActiveState is not nil
					if event[service].ActiveState == "active" {
						log.Printf("Service[%v]->Started()\n", service)
					} else if event[service].ActiveState == "inactive" {
						log.Printf("Service[%v]->Stopped!()\n", service)
					} else {
						log.Fatal("Unknown service state: ", event[service].ActiveState)
					}
				} else {
					// service stopped (and ActiveState is nil...)
					log.Printf("Service[%v]->Stopped\n", service)
				}
				send = true

			case err := <-subErrors:
				log.Println("error:", err)
				log.Fatal(err)
				v.Events <- fmt.Sprintf("service: %v", "error")

			case exit := <-obj.Events:
				if exit == "exit" {
					return
				} else {
					log.Fatal("Unknown event: %v\n", exit)
				}
			}
		}

		if send {
			send = false
			//log.Println("Sending event!")
			v.Events <- fmt.Sprintf("service(%v): %v", obj.Name, "event!") // FIXME: use struct
		}
	}
}

func (obj ServiceType) Exit() bool {
	obj.Events <- "exit"
	return true
}

func (obj ServiceType) StateOK() bool {

	if !util.IsRunningSystemd() {
		log.Fatal("Systemd is not running.")
	}

	conn, err := systemd.NewSystemdConnection() // needs root access
	if err != nil {
		log.Fatal("Failed to connect to systemd: ", err)
	}
	defer conn.Close()

	var service = fmt.Sprintf("%v.service", obj.Name) // systemd name

	loadstate, err := conn.GetUnitProperty(service, "LoadState")
	if err != nil {
		log.Printf("Failed to get load state: %v\n", err)
		return false
	}

	// NOTE: we have to compare variants with other variants, they are really strings...
	var notFound = (loadstate.Value == dbus.MakeVariant("not-found"))
	if notFound {
		log.Printf("Failed to find service: %v\n", service)
		return false
	}

	// XXX: check service "enabled at boot" or not status...

	//conn.GetUnitProperties(service)
	activestate, err := conn.GetUnitProperty(service, "ActiveState")
	if err != nil {
		log.Fatal("Failed to get active state: ", err)
	}

	var running = (activestate.Value == dbus.MakeVariant("active"))

	if obj.State == "running" {
		if !running {
			return false // we are in the wrong state
		}
	} else if obj.State == "stopped" {
		if running {
			return false
		}
	} else {
		log.Fatal("Unknown state: ", obj.State)
	}

	return true // all is good, no state change needed
}

func (obj ServiceType) Apply() bool {
	fmt.Printf("Apply->%v[%v]\n", obj.Type, obj.Name)

	if !util.IsRunningSystemd() {
		log.Fatal("Systemd is not running.")
	}

	conn, err := systemd.NewSystemdConnection() // needs root access
	if err != nil {
		log.Fatal("Failed to connect to systemd: ", err)
	}
	defer conn.Close()

	var service = fmt.Sprintf("%v.service", obj.Name) // systemd name
	var files = []string{service}                     // the service represented in a list
	if obj.Startup == "enabled" {
		_, _, err = conn.EnableUnitFiles(files, false, true)

	} else if obj.Startup == "disabled" {
		_, err = conn.DisableUnitFiles(files, false)
	} else {
		err = nil
	}
	if err != nil {
		log.Printf("Unable to change startup status: %v\n", err)
		return false
	}

	result := make(chan string, 1) // catch result information

	if obj.State == "running" {
		_, err := conn.StartUnit(service, "fail", result)
		if err != nil {
			log.Fatal("Failed to start unit: ", err)
			return false
		}
	} else if obj.State == "stopped" {
		_, err = conn.StopUnit(service, "fail", result)
		if err != nil {
			log.Fatal("Failed to stop unit: ", err)
			return false
		}
	} else {
		log.Fatal("Unknown state: ", obj.State)
	}

	status := <-result
	if &status == nil {
		log.Fatal("Result is nil")
		return false
	}
	if status != "done" {
		log.Fatal("Unknown return string: ", status)
		return false
	}

	// XXX: also set enabled on boot

	return true
}
