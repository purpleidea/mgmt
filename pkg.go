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

package main

import (
	//"packagekit" // TODO
	"errors"
	"fmt"
	"log"
	"strings"
)

type PkgRes struct {
	BaseRes          `yaml:",inline"`
	State            string `yaml:"state"`            // state: installed, uninstalled, newest, <version>
	AllowUntrusted   bool   `yaml:"allowuntrusted"`   // allow untrusted packages to be installed?
	AllowNonFree     bool   `yaml:"allownonfree"`     // allow nonfree packages to be found?
	AllowUnsupported bool   `yaml:"allowunsupported"` // allow unsupported packages to be found?
}

// helper function for creating new pkg resources that calls Init()
func NewPkgRes(name, state string, allowuntrusted, allownonfree, allowunsupported bool) *PkgRes {
	obj := &PkgRes{
		BaseRes: BaseRes{
			Name:   name,
			events: make(chan Event),
			vertex: nil,
		},
		State:            state,
		AllowUntrusted:   allowuntrusted,
		AllowNonFree:     allownonfree,
		AllowUnsupported: allowunsupported,
	}
	obj.Init()
	return obj
}

func (obj *PkgRes) Init() {
	obj.BaseRes.kind = "Pkg"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

func (obj *PkgRes) Kind() string {
	return "Pkg"
}

func (obj *PkgRes) Validate() bool {

	if obj.State == "" {
		return false
	}

	return true
}

// use UpdatesChanged signal to watch for changes
// TODO: https://github.com/hughsie/PackageKit/issues/109
// TODO: https://github.com/hughsie/PackageKit/issues/110
func (obj *PkgRes) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	bus := NewBus()
	if bus == nil {
		log.Fatal("Can't connect to PackageKit bus.")
	}
	defer bus.Close()

	ch, err := bus.WatchChanges()
	if err != nil {
		log.Fatalf("Error adding signal match: %v", err)
	}

	var send = false // send event?
	var exit = false
	var dirty = false

	for {
		if DEBUG {
			log.Printf("Pkg[%v]: Watching...", obj.GetName())
		}

		obj.SetState(resStateWatching) // reset
		select {
		case event := <-ch:

			// FIXME: ask packagekit for info on what packages changed
			if DEBUG {
				log.Printf("Pkg[%v]: Event: %v", obj.GetName(), event.Name)
			}

			// since the chan is buffered, remove any supplemental
			// events since they would just be duplicates anyways!
			for len(ch) > 0 { // we can detect pending count here!
				<-ch // discard
			}

			obj.SetConvergedState(resConvergedNil)
			send = true
			dirty = true

		case event := <-obj.events:
			obj.SetConvergedState(resConvergedNil)
			if exit, send = obj.ReadEvent(&event); exit {
				return // exit
			}
			//dirty = false // these events don't invalidate state

		case _ = <-TimeAfterOrBlock(obj.ctimeout):
			obj.SetConvergedState(resConvergedTimeout)
			obj.converged <- true
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// only invalid state on certain types of events
			if dirty {
				dirty = false
				obj.isStateOK = false // something made state dirty
			}
			Process(obj) // XXX: rename this function
		}
	}
}

func (obj *PkgRes) CheckApply(apply bool) (stateok bool, err error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.State == "" { // TODO: Validate() should replace this check!
		log.Fatalf("%v[%v]: Package state is undefined!", obj.Kind(), obj.GetName())
	}

	if obj.isStateOK { // cache the state
		return true, nil
	}

	bus := NewBus()
	if bus == nil {
		return false, errors.New("Can't connect to PackageKit bus.")
	}
	defer bus.Close()

	var packages = []string{obj.Name}
	var filter uint64 = 0
	filter += PK_FILTER_ENUM_ARCH // always search in our arch
	// we're requesting latest version, or to narrow down install choices!
	if obj.State == "newest" || obj.State == "installed" {
		// if we add this, we'll still see older packages if installed
		filter += PK_FILTER_ENUM_NEWEST // only search for newest packages
	}
	if !obj.AllowNonFree {
		filter += PK_FILTER_ENUM_FREE
	}
	if !obj.AllowUnsupported {
		filter += PK_FILTER_ENUM_SUPPORTED
	}
	if DEBUG {
		log.Printf("Pkg[%v]: ResolvePackages: %v", obj.GetName(), strings.Join(packages, ", "))
	}
	resolved, e := bus.ResolvePackages(packages, filter)
	if e != nil {
		return false, errors.New(fmt.Sprintf("Resolve error: %v", e))
	}

	var found = false
	var installed = false
	var version = ""
	var newest = true // assume, for now
	var usePackageId = ""
	for _, packageId := range resolved {
		//log.Printf("* %v", packageId)
		// format is: name;version;arch;data
		s := strings.Split(packageId, ";")
		//if len(s) != 4 { continue } // this would be a bug!
		pkg, ver, _, data := s[0], s[1], s[2], s[3]
		//arch := s[2] // TODO: double check match on arch?
		if pkg != obj.Name { // not what we're looking for
			continue
		}
		found = true
		if obj.State != "installed" && obj.State != "uninstalled" && obj.State != "newest" { // must be a ver. string
			if obj.State == ver && ver != "" { // we match what we want...
				usePackageId = packageId
			}
		}
		if FlagInData("installed", data) {
			installed = true
			version = ver
			if obj.State == "uninstalled" {
				usePackageId = packageId // save for later
			}
		} else { // not installed...
			if obj.State == "installed" || obj.State == "newest" {
				usePackageId = packageId
			}
		}

		// if the first iteration didn't contain the installed package,
		// then since the NEWEST filter was on, we're not the newest!
		if !installed {
			newest = false
		}
	}

	// package doesn't exist, this is an error!
	if !found {
		return false, errors.New(fmt.Sprintf("Can't find package named '%s'.", obj.Name))
	}

	//obj.State == "installed" || "uninstalled" || "newest" || "4.2-1.fc23"
	switch obj.State {
	case "installed":
		if installed {
			return true, nil // state is correct, exit!
		}
	case "uninstalled":
		if !installed {
			return true, nil
		}
	case "newest":
		if newest {
			return true, nil
		}
	default: // version string
		if obj.State == version && version != "" {
			return true, nil
		}
	}

	if usePackageId == "" {
		return false, errors.New("Can't find package id to use.")
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%v[%v]: Apply", obj.Kind(), obj.GetName())
	packageList := []string{usePackageId}
	var transactionFlags uint64 = 0
	if !obj.AllowUntrusted { // allow
		transactionFlags += PK_TRANSACTION_FLAG_ENUM_ONLY_TRUSTED
	}
	// apply correct state!
	log.Printf("%v[%v]: Set: %v...", obj.Kind(), obj.GetName(), obj.State)
	switch obj.State {
	case "uninstalled": // run remove
		// NOTE: packageId is different than when installed, because now
		// it has the "installed" flag added to the data portion if it!!
		err = bus.RemovePackages(packageList, transactionFlags)

	case "newest": // TODO: isn't this the same operation as install, below?
		err = bus.UpdatePackages(packageList, transactionFlags)

	case "installed":
		fallthrough // same method as for "set specific version", below
	default: // version string
		err = bus.InstallPackages(packageList, transactionFlags)
	}
	if err != nil {
		return false, err // fail
	}
	log.Printf("%v[%v]: Set: %v success!", obj.Kind(), obj.GetName(), obj.State)
	return false, nil // success
}

func (obj *PkgRes) AutoEdges() AutoEdge {
	return nil
}

func (obj *PkgRes) Compare(res Res) bool {
	switch res.(type) {
	case *PkgRes:
		res := res.(*PkgRes)
		if obj.Name != res.Name {
			return false
		}
		if obj.State != res.State {
			return false
		}
		if obj.AllowUntrusted != res.AllowUntrusted {
			return false
		}
		if obj.AllowNonFree != res.AllowNonFree {
			return false
		}
		if obj.AllowUnsupported != res.AllowUnsupported {
			return false
		}
	default:
		return false
	}
	return true
}
