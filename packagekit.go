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

// DOCS: https://www.freedesktop.org/software/PackageKit/gtk-doc/index.html

//package packagekit // TODO
package main

import (
	"errors"
	"fmt"
	"github.com/godbus/dbus"
	"log"
	"strings"
)

const (
	PK_DEBUG = false
	PARANOID = false // enable if you see any ghosts
)

const (
	PkBufferSize       = 100
	PkPath             = "/org/freedesktop/PackageKit"
	PkIface            = "org.freedesktop.PackageKit"
	PkIfaceTransaction = PkIface + ".Transaction"
	dbusAddMatch       = "org.freedesktop.DBus.AddMatch"
)

var (
	PkArchMap = map[string]string{ // map of PackageKit arch to GOARCH
		"x86_64": "amd64",
		// TODO: add more values
	}
)

//type enum_filter uint64
// https://github.com/hughsie/PackageKit/blob/master/lib/packagekit-glib2/pk-enum.c
const ( //static const PkEnumMatch enum_filter[]
	PK_FILTER_ENUM_UNKNOWN         uint64 = 1 << iota // "unknown"
	PK_FILTER_ENUM_NONE                               // "none"
	PK_FILTER_ENUM_INSTALLED                          // "installed"
	PK_FILTER_ENUM_NOT_INSTALLED                      // "~installed"
	PK_FILTER_ENUM_DEVELOPMENT                        // "devel"
	PK_FILTER_ENUM_NOT_DEVELOPMENT                    // "~devel"
	PK_FILTER_ENUM_GUI                                // "gui"
	PK_FILTER_ENUM_NOT_GUI                            // "~gui"
	PK_FILTER_ENUM_FREE                               // "free"
	PK_FILTER_ENUM_NOT_FREE                           // "~free"
	PK_FILTER_ENUM_VISIBLE                            // "visible"
	PK_FILTER_ENUM_NOT_VISIBLE                        // "~visible"
	PK_FILTER_ENUM_SUPPORTED                          // "supported"
	PK_FILTER_ENUM_NOT_SUPPORTED                      // "~supported"
	PK_FILTER_ENUM_BASENAME                           // "basename"
	PK_FILTER_ENUM_NOT_BASENAME                       // "~basename"
	PK_FILTER_ENUM_NEWEST                             // "newest"
	PK_FILTER_ENUM_NOT_NEWEST                         // "~newest"
	PK_FILTER_ENUM_ARCH                               // "arch"
	PK_FILTER_ENUM_NOT_ARCH                           // "~arch"
	PK_FILTER_ENUM_SOURCE                             // "source"
	PK_FILTER_ENUM_NOT_SOURCE                         // "~source"
	PK_FILTER_ENUM_COLLECTIONS                        // "collections"
	PK_FILTER_ENUM_NOT_COLLECTIONS                    // "~collections"
	PK_FILTER_ENUM_APPLICATION                        // "application"
	PK_FILTER_ENUM_NOT_APPLICATION                    // "~application"
	PK_FILTER_ENUM_DOWNLOADED                         // "downloaded"
	PK_FILTER_ENUM_NOT_DOWNLOADED                     // "~downloaded"
)

const ( //static const PkEnumMatch enum_transaction_flag[]
	PK_TRANSACTION_FLAG_ENUM_NONE            uint64 = 1 << iota // "none"
	PK_TRANSACTION_FLAG_ENUM_ONLY_TRUSTED                       // "only-trusted"
	PK_TRANSACTION_FLAG_ENUM_SIMULATE                           // "simulate"
	PK_TRANSACTION_FLAG_ENUM_ONLY_DOWNLOAD                      // "only-download"
	PK_TRANSACTION_FLAG_ENUM_ALLOW_REINSTALL                    // "allow-reinstall"
	PK_TRANSACTION_FLAG_ENUM_JUST_REINSTALL                     // "just-reinstall"
	PK_TRANSACTION_FLAG_ENUM_ALLOW_DOWNGRADE                    // "allow-downgrade"
)

// wrapper struct so we can pass bus connection around in the struct
type Conn struct {
	conn *dbus.Conn
}

// get a new bus connection
func NewBus() *Conn {
	// if we share the bus with others, we will get each others messages!!
	bus, err := SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return nil
	}
	return &Conn{
		conn: bus,
	}
}

// get the dbus connection object
func (bus *Conn) GetBus() *dbus.Conn {
	return bus.conn
}

// close the dbus connection object
func (bus *Conn) Close() error {
	return bus.conn.Close()
}

// internal helper to add signal matches to the bus, should only be called once
func (bus *Conn) matchSignal(ch chan *dbus.Signal, path dbus.ObjectPath, iface string, signals []string) error {
	if PK_DEBUG {
		log.Printf("PackageKit: matchSignal(%v, %v, %v, %v)", ch, path, iface, signals)
	}
	// eg: gdbus monitor --system --dest org.freedesktop.PackageKit --object-path /org/freedesktop/PackageKit | grep <signal>
	var call *dbus.Call
	// TODO: if we make this call many times, we seem to receive signals
	// that many times... Maybe this should be an object singleton?
	obj := bus.GetBus().BusObject()
	pathStr := fmt.Sprintf("%s", path)
	if len(signals) == 0 {
		call = obj.Call(dbusAddMatch, 0, "type='signal',path='"+pathStr+"',interface='"+iface+"'")
	} else {
		for _, signal := range signals {
			call = obj.Call(dbusAddMatch, 0, "type='signal',path='"+pathStr+"',interface='"+iface+"',member='"+signal+"'")
			if call.Err != nil {
				break
			}
		}
	}
	if call.Err != nil {
		return call.Err
	}
	if PK_DEBUG {
		log.Println("PackageKit: matchSignal(): Added!")
	}
	// The caller has to make sure that ch is sufficiently buffered; if a
	// message arrives when a write to c is not possible, it is discarded!
	// This can be disastrous if we're waiting for a "Finished" signal!
	bus.GetBus().Signal(ch)
	if PK_DEBUG {
		log.Println("PackageKit: matchSignal(): Success!")
	}
	return nil
}

// get a signal anytime an event happens
func (bus *Conn) WatchChanges() (chan *dbus.Signal, error) {
	ch := make(chan *dbus.Signal, PkBufferSize)
	// NOTE: the TransactionListChanged signal fires much more frequently,
	// but with much less specificity. If we're missing events, report the
	// issue upstream! The UpdatesChanged signal is what hughsie suggested
	var signal = "UpdatesChanged"
	err := bus.matchSignal(ch, PkPath, PkIface, []string{signal})
	if err != nil {
		return nil, err
	}
	if PARANOID { // TODO: this filtering might not be necessary anymore...
		// try to handle the filtering inside this function!
		rch := make(chan *dbus.Signal)
		go func() {
		loop:
			for {
				select {
				case event := <-ch:
					// "A receive from a closed channel returns the
					// zero value immediately": if i get nil here,
					// it means the channel was closed by someone!!
					if event == nil { // shared bus issue?
						log.Println("PackageKit: Hrm, channel was closed!")
						break loop // TODO: continue?
					}
					// i think this was caused by using the shared
					// bus, but we might as well leave it in for now
					if event.Path != PkPath || event.Name != fmt.Sprintf("%s.%s", PkIface, signal) {
						log.Println("PackageKit: Some wires have been crossed!")
						continue
					}
					rch <- event // forward...
				}
			}
			defer close(ch)
		}()
		return rch, nil
	}
	return ch, nil
}

// create and return a transaction path
func (bus *Conn) CreateTransaction() (dbus.ObjectPath, error) {
	if PK_DEBUG {
		log.Println("PackageKit: CreateTransaction()")
	}
	var interfacePath dbus.ObjectPath
	obj := bus.GetBus().Object(PkIface, PkPath)
	call := obj.Call(fmt.Sprintf("%s.CreateTransaction", PkIface), 0).Store(&interfacePath)
	if call != nil {
		return "", call
	}
	if PK_DEBUG {
		log.Printf("PackageKit: CreateTransaction(): %v", interfacePath)
	}
	return interfacePath, nil
}

func (bus *Conn) ResolvePackages(packages []string, filter uint64) ([]string, error) {
	packageIds := []string{}
	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := bus.CreateTransaction() // emits Destroy on close
	if err != nil {
		return []string{}, err
	}

	// add signal matches for Package and Finished which will always be last
	var signals = []string{"Package", "Finished", "Error", "Destroy"}
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)
	if PK_DEBUG {
		log.Printf("PackageKit: ResolvePackages(): Object(%v, %v)", PkIface, interfacePath)
	}
	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("Resolve"), 0, filter, packages)
	if PK_DEBUG {
		log.Println("PackageKit: ResolvePackages(): Call: Success!")
	}
	if call.Err != nil {
		return []string{}, call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if PK_DEBUG {
				log.Printf("PackageKit: ResolvePackages(): Signal: %+v", signal)
			}
			if signal.Path != interfacePath {
				log.Println("PackageKit: Some wires have been crossed!")
				continue loop
			}

			if signal.Name == FmtTransactionMethod("Package") {
				//pkg_int, ok := signal.Body[0].(int)
				packageId, ok := signal.Body[1].(string)
				// format is: name;version;arch;data
				if !ok {
					continue loop
				}
				//comment, ok := signal.Body[2].(string)
				for _, p := range packageIds {
					if packageId == p {
						continue loop // duplicate!
					}
				}
				packageIds = append(packageIds, packageId)
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return []string{}, errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			}
		}
	}
	return packageIds, nil
}

func (bus *Conn) IsInstalledList(packages []string) ([]bool, error) {
	var filter uint64 = 0
	filter += PK_FILTER_ENUM_ARCH // always search in our arch
	packageIds, e := bus.ResolvePackages(packages, filter)
	if e != nil {
		return nil, errors.New(fmt.Sprintf("ResolvePackages error: %v", e))
	}

	var m map[string]int = make(map[string]int)
	for _, packageId := range packageIds {
		s := strings.Split(packageId, ";")
		//if len(s) != 4 { continue } // this would be a bug!
		pkg := s[0]
		flags := strings.Split(s[3], ":")
		for _, f := range flags {
			if f == "installed" {
				if _, exists := m[pkg]; !exists {
					m[pkg] = 0
				}
				m[pkg]++ // if we see pkg installed, increment
				break
			}
		}
	}

	var r []bool
	for _, p := range packages {
		if value, exists := m[p]; exists {
			r = append(r, value > 0) // at least 1 means installed
		} else {
			r = append(r, false)
		}
	}
	return r, nil
}

// is package installed ?
// TODO: this could be optimized by making the resolve call directly
func (bus *Conn) IsInstalled(pkg string) (bool, error) {
	p, e := bus.IsInstalledList([]string{pkg})
	if len(p) != 1 {
		return false, e
	}
	return p[0], nil
}

// install list of packages by packageId
func (bus *Conn) InstallPackages(packageIds []string, transactionFlags uint64) error {

	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := bus.CreateTransaction() // emits Destroy on close
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("InstallPackages"), 0, transactionFlags, packageIds)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Println("PackageKit: Some wires have been crossed!")
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			} else if signal.Name == FmtTransactionMethod("Package") {
				// a package was installed...
				continue loop
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			}
		}
	}
	return nil
}

// remove list of packages
func (bus *Conn) RemovePackages(packageIds []string, transactionFlags uint64) error {

	var allowDeps bool = true                     // TODO: configurable
	var autoremove bool = false                   // unsupported on GNU/Linux
	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := bus.CreateTransaction() // emits Destroy on close
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("RemovePackages"), 0, transactionFlags, packageIds, allowDeps, autoremove)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Println("PackageKit: Some wires have been crossed!")
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			} else if signal.Name == FmtTransactionMethod("Package") {
				// a package was installed...
				continue loop
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			}
		}
	}
	return nil
}

// update list of packages to versions that are specified
func (bus *Conn) UpdatePackages(packageIds []string, transactionFlags uint64) error {
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := bus.CreateTransaction()
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("UpdatePackages"), 0, transactionFlags, packageIds)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Println("PackageKit: Some wires have been crossed!")
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			} else if signal.Name == FmtTransactionMethod("Package") {
				// a package was installed...
				continue loop
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
			}
		}
	}
	return nil
}

// get the list of files that are contained inside a list of packageids
func (bus *Conn) GetFilesByPackageId(packageIds []string) (files map[string][]string, err error) {
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := bus.CreateTransaction()
	if err != nil {
		return
	}

	var signals = []string{"Files", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("GetFiles"), 0, packageIds)
	if call.Err != nil {
		err = call.Err
		return
	}
	files = make(map[string][]string)
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:

			if signal.Path != interfacePath {
				log.Println("PackageKit: Some wires have been crossed!")
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				err = errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
				return

				// one signal returned per packageId found...
			} else if signal.Name == FmtTransactionMethod("Files") {
				if len(signal.Body) != 2 { // bad data
					continue loop
				}
				var ok bool
				var key string
				var fileList []string
				if key, ok = signal.Body[0].(string); !ok {
					continue loop
				}
				if fileList, ok = signal.Body[1].([]string); !ok {
					continue loop // failed conversion
				}
				files[key] = fileList // build up map

				continue loop
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				err = errors.New(fmt.Sprintf("PackageKit error: %v", signal.Body))
				return
			}
		}
	}
	return
}

// does flag exist inside data portion of packageId field?
func FlagInData(flag, data string) bool {
	flags := strings.Split(data, ":")
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

// builds the transaction method string
func FmtTransactionMethod(method string) string {
	return fmt.Sprintf("%s.%s", PkIfaceTransaction, method)
}
