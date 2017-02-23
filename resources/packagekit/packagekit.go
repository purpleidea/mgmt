// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

// Package packagekit provides an interface to interact with packagekit.
// See: https://www.freedesktop.org/software/PackageKit/gtk-doc/index.html for
// more information.
package packagekit

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/purpleidea/mgmt/util"

	"github.com/godbus/dbus"
)

// global tweaks of verbosity and code path
const (
	PK_DEBUG = false
	PARANOID = false // enable if you see any ghosts
)

// constants which might need to be tweaked or which contain special dbus strings.
const (
	// FIXME: if PkBufferSize is too low, install seems to drop signals
	PkBufferSize = 1000
	// TODO: the PkSignalTimeout value might be too low
	PkSignalPackageTimeout = 60 // 60 seconds, arbitrary
	PkSignalDestroyTimeout = 15 // 15 seconds, arbitrary
	PkPath                 = "/org/freedesktop/PackageKit"
	PkIface                = "org.freedesktop.PackageKit"
	PkIfaceTransaction     = PkIface + ".Transaction"
	dbusAddMatch           = "org.freedesktop.DBus.AddMatch"
)

var (
	// PkArchMap contains the mapping from PackageKit arch to GOARCH.
	// GOARCH's: 386, amd64, arm, arm64, mips64, mips64le, ppc64, ppc64le
	PkArchMap = map[string]string{ // map of PackageKit arch to GOARCH
		// TODO: add more values
		// noarch
		"noarch": "ANY", // special value "ANY" (noarch as seen in Fedora)
		"all":    "ANY", // special value "ANY" ('all' as seen in Debian)
		// fedora
		"x86_64":  "amd64",
		"aarch64": "arm64",
		// debian, from: https://www.debian.org/ports/
		"amd64": "amd64",
		"arm64": "arm64",
		"i386":  "386",
		"i486":  "386",
		"i586":  "386",
		"i686":  "386",
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

// constants from packagekit c library.
const ( //static const PkEnumMatch enum_transaction_flag[]
	PK_TRANSACTION_FLAG_ENUM_NONE            uint64 = 1 << iota // "none"
	PK_TRANSACTION_FLAG_ENUM_ONLY_TRUSTED                       // "only-trusted"
	PK_TRANSACTION_FLAG_ENUM_SIMULATE                           // "simulate"
	PK_TRANSACTION_FLAG_ENUM_ONLY_DOWNLOAD                      // "only-download"
	PK_TRANSACTION_FLAG_ENUM_ALLOW_REINSTALL                    // "allow-reinstall"
	PK_TRANSACTION_FLAG_ENUM_JUST_REINSTALL                     // "just-reinstall"
	PK_TRANSACTION_FLAG_ENUM_ALLOW_DOWNGRADE                    // "allow-downgrade"
)

// constants from packagekit c library.
const ( //typedef enum
	PK_INFO_ENUM_UNKNOWN uint64 = 1 << iota
	PK_INFO_ENUM_INSTALLED
	PK_INFO_ENUM_AVAILABLE
	PK_INFO_ENUM_LOW
	PK_INFO_ENUM_ENHANCEMENT
	PK_INFO_ENUM_NORMAL
	PK_INFO_ENUM_BUGFIX
	PK_INFO_ENUM_IMPORTANT
	PK_INFO_ENUM_SECURITY
	PK_INFO_ENUM_BLOCKED
	PK_INFO_ENUM_DOWNLOADING
	PK_INFO_ENUM_UPDATING
	PK_INFO_ENUM_INSTALLING
	PK_INFO_ENUM_REMOVING
	PK_INFO_ENUM_CLEANUP
	PK_INFO_ENUM_OBSOLETING
	PK_INFO_ENUM_COLLECTION_INSTALLED
	PK_INFO_ENUM_COLLECTION_AVAILABLE
	PK_INFO_ENUM_FINISHED
	PK_INFO_ENUM_REINSTALLING
	PK_INFO_ENUM_DOWNGRADING
	PK_INFO_ENUM_PREPARING
	PK_INFO_ENUM_DECOMPRESSING
	PK_INFO_ENUM_UNTRUSTED
	PK_INFO_ENUM_TRUSTED
	PK_INFO_ENUM_UNAVAILABLE
	PK_INFO_ENUM_LAST
)

// Conn is a wrapper struct so we can pass bus connection around in the struct.
type Conn struct {
	conn *dbus.Conn
}

// PkPackageIDActionData is a struct that is returned by PackagesToPackageIDs in the map values.
type PkPackageIDActionData struct {
	Found     bool
	Installed bool
	Version   string
	PackageID string
	Newest    bool
}

// NewBus returns a new bus connection.
func NewBus() *Conn {
	// if we share the bus with others, we will get each others messages!!
	bus, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return nil
	}
	return &Conn{
		conn: bus,
	}
}

// GetBus gets the dbus connection object.
func (bus *Conn) GetBus() *dbus.Conn {
	return bus.conn
}

// Close closes the dbus connection object.
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
	// The caller has to make sure that ch is sufficiently buffered; if a
	// message arrives when a write to c is not possible, it is discarded!
	// This can be disastrous if we're waiting for a "Finished" signal!
	bus.GetBus().Signal(ch)
	return nil
}

// WatchChanges gets a signal anytime an event happens.
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
						log.Printf("PackageKit: Woops: Event: %+v", event)
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

// CreateTransaction creates and returns a transaction path.
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

// ResolvePackages runs the PackageKit Resolve method and returns the result.
func (bus *Conn) ResolvePackages(packages []string, filter uint64) ([]string, error) {
	packageIDs := []string{}
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
				log.Printf("PackageKit: Woops: Signal.Path: %+v", signal.Path)
				continue loop
			}

			if signal.Name == FmtTransactionMethod("Package") {
				//pkg_int, ok := signal.Body[0].(int)
				packageID, ok := signal.Body[1].(string)
				// format is: name;version;arch;data
				if !ok {
					continue loop
				}
				//comment, ok := signal.Body[2].(string)
				for _, p := range packageIDs {
					if packageID == p {
						continue loop // duplicate!
					}
				}
				packageIDs = append(packageIDs, packageID)
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return []string{}, fmt.Errorf("PackageKit: Error: %v", signal.Body)
			}
		}
	}
	return packageIDs, nil
}

// IsInstalledList queries a list of packages to see if they are installed.
func (bus *Conn) IsInstalledList(packages []string) ([]bool, error) {
	var filter uint64             // initializes at the "zero" value of 0
	filter += PK_FILTER_ENUM_ARCH // always search in our arch
	packageIDs, e := bus.ResolvePackages(packages, filter)
	if e != nil {
		return nil, fmt.Errorf("ResolvePackages error: %v", e)
	}

	var m = make(map[string]int)
	for _, packageID := range packageIDs {
		s := strings.Split(packageID, ";")
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

// IsInstalled returns if a package is installed.
// TODO: this could be optimized by making the resolve call directly
func (bus *Conn) IsInstalled(pkg string) (bool, error) {
	p, e := bus.IsInstalledList([]string{pkg})
	if len(p) != 1 {
		return false, e
	}
	return p[0], nil
}

// InstallPackages installs a list of packages by packageID.
func (bus *Conn) InstallPackages(packageIDs []string, transactionFlags uint64) error {

	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := bus.CreateTransaction() // emits Destroy on close
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("InstallPackages"), 0, transactionFlags, packageIDs)
	if call.Err != nil {
		return call.Err
	}
	timeout := -1 // disabled initially
	finished := false
loop:
	for {
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Printf("PackageKit: Woops: Signal.Path: %+v", signal.Path)
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return fmt.Errorf("PackageKit: Error: %v", signal.Body)
			} else if signal.Name == FmtTransactionMethod("Package") {
				// a package was installed...
				// only start the timer once we're here...
				timeout = PkSignalPackageTimeout
			} else if signal.Name == FmtTransactionMethod("Finished") {
				finished = true
				timeout = PkSignalDestroyTimeout // wait a bit
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				return nil // success
			} else {
				return fmt.Errorf("PackageKit: Error: %v", signal.Body)
			}
		case <-util.TimeAfterOrBlock(timeout):
			if finished {
				log.Println("PackageKit: Timeout: InstallPackages: Waiting for 'Destroy'")
				return nil // got tired of waiting for Destroy
			}
			return fmt.Errorf("PackageKit: Timeout: InstallPackages: %v", strings.Join(packageIDs, ", "))
		}
	}
}

// RemovePackages removes a list of packages by packageID.
func (bus *Conn) RemovePackages(packageIDs []string, transactionFlags uint64) error {

	var allowDeps = true                          // TODO: configurable
	var autoremove = false                        // unsupported on GNU/Linux
	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := bus.CreateTransaction() // emits Destroy on close
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("RemovePackages"), 0, transactionFlags, packageIDs, allowDeps, autoremove)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Printf("PackageKit: Woops: Signal.Path: %+v", signal.Path)
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return fmt.Errorf("PackageKit: Error: %v", signal.Body)
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
				return fmt.Errorf("PackageKit: Error: %v", signal.Body)
			}
		}
	}
	return nil
}

// UpdatePackages updates a list of packages to versions that are specified.
func (bus *Conn) UpdatePackages(packageIDs []string, transactionFlags uint64) error {
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := bus.CreateTransaction()
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("UpdatePackages"), 0, transactionFlags, packageIDs)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Printf("PackageKit: Woops: Signal.Path: %+v", signal.Path)
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return fmt.Errorf("PackageKit: Error: %v", signal.Body)
			} else if signal.Name == FmtTransactionMethod("Package") {
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return fmt.Errorf("PackageKit: Error: %v", signal.Body)
			}
		}
	}
	return nil
}

// GetFilesByPackageID gets the list of files that are contained inside a list of packageIDs.
func (bus *Conn) GetFilesByPackageID(packageIDs []string) (files map[string][]string, err error) {
	// NOTE: the maximum number of files in an RPM is 52116 in Fedora 23
	// https://gist.github.com/purpleidea/b98e60dcd449e1ac3b8a
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := bus.CreateTransaction()
	if err != nil {
		return
	}

	var signals = []string{"Files", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("GetFiles"), 0, packageIDs)
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
				log.Printf("PackageKit: Woops: Signal.Path: %+v", signal.Path)
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				err = fmt.Errorf("PackageKit: Error: %v", signal.Body)
				return

				// one signal returned per packageID found...
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
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				err = fmt.Errorf("PackageKit: Error: %v", signal.Body)
				return
			}
		}
	}
	return
}

// GetUpdates gets a list of packages that are installed and which can be updated, mod filter.
func (bus *Conn) GetUpdates(filter uint64) ([]string, error) {
	if PK_DEBUG {
		log.Println("PackageKit: GetUpdates()")
	}
	packageIDs := []string{}
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := bus.CreateTransaction()
	if err != nil {
		return nil, err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress" ?
	bus.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	obj := bus.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := obj.Call(FmtTransactionMethod("GetUpdates"), 0, filter)
	if call.Err != nil {
		return nil, call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				log.Printf("PackageKit: Woops: Signal.Path: %+v", signal.Path)
				continue loop
			}

			if signal.Name == FmtTransactionMethod("ErrorCode") {
				return nil, fmt.Errorf("PackageKit: Error: %v", signal.Body)
			} else if signal.Name == FmtTransactionMethod("Package") {

				//pkg_int, ok := signal.Body[0].(int)
				packageID, ok := signal.Body[1].(string)
				// format is: name;version;arch;data
				if !ok {
					continue loop
				}
				//comment, ok := signal.Body[2].(string)
				for _, p := range packageIDs { // optional?
					if packageID == p {
						continue loop // duplicate!
					}
				}
				packageIDs = append(packageIDs, packageID)
			} else if signal.Name == FmtTransactionMethod("Finished") {
				// TODO: should we wait for the Destroy signal?
				break loop
			} else if signal.Name == FmtTransactionMethod("Destroy") {
				// should already be broken
				break loop
			} else {
				return nil, fmt.Errorf("PackageKit: Error: %v", signal.Body)
			}
		}
	}
	return packageIDs, nil
}

// PackagesToPackageIDs is a helper function that *might* be generally useful
// outside mgmt. The packageMap input has the package names as keys and
// requested states as values. These states can be: installed, uninstalled,
// newest or a requested version str.
func (bus *Conn) PackagesToPackageIDs(packageMap map[string]string, filter uint64) (map[string]*PkPackageIDActionData, error) {
	count := 0
	packages := make([]string, len(packageMap))
	for k := range packageMap { // lol, golang has no hash.keys() function!
		packages[count] = k
		count++
	}

	if !(filter&PK_FILTER_ENUM_ARCH == PK_FILTER_ENUM_ARCH) {
		filter += PK_FILTER_ENUM_ARCH // always search in our arch
	}

	if PK_DEBUG {
		log.Printf("PackageKit: PackagesToPackageIDs(): %v", strings.Join(packages, ", "))
	}
	resolved, e := bus.ResolvePackages(packages, filter)
	if e != nil {
		return nil, fmt.Errorf("Resolve error: %v", e)
	}

	found := make([]bool, count) // default false
	installed := make([]bool, count)
	version := make([]string, count)
	usePackageID := make([]string, count)
	newest := make([]bool, count) // default true
	for i := range newest {
		newest[i] = true // assume, for now
	}
	var index int

	for _, packageID := range resolved {
		index = -1
		//log.Printf("* %v", packageID)
		// format is: name;version;arch;data
		s := strings.Split(packageID, ";")
		//if len(s) != 4 { continue } // this would be a bug!
		pkg, ver, arch, data := s[0], s[1], s[2], s[3]
		// we might need to allow some of this, eg: i386 .deb on amd64
		if !IsMyArch(arch) {
			continue
		}

		for i := range packages { // find pkg if it exists
			if pkg == packages[i] {
				index = i
			}
		}
		if index == -1 { // can't find what we're looking for
			continue
		}
		state := packageMap[pkg] // lookup the requested state/version
		if state == "" {
			return nil, fmt.Errorf("Empty package state for %v", pkg)
		}
		found[index] = true
		stateIsVersion := (state != "installed" && state != "uninstalled" && state != "newest") // must be a ver. string

		if stateIsVersion {
			if state == ver && ver != "" { // we match what we want...
				usePackageID[index] = packageID
			}
		}

		if FlagInData("installed", data) {
			installed[index] = true
			version[index] = ver
			// state of "uninstalled" matched during CheckApply, and
			// states of "installed" and "newest" for fileList
			if !stateIsVersion {
				usePackageID[index] = packageID // save for later
			}
		} else { // not installed...
			if !stateIsVersion {
				// if there is more than one result, eg: there
				// is the old and newest version of a package,
				// then this section can run more than once...
				// in that case, don't worry, we'll choose the
				// right value in the "updates" section below!
				usePackageID[index] = packageID
			}
		}
	}

	// we can't determine which packages are "newest", without searching
	// for each one individually, so instead we check if any updates need
	// to be done, and if so, anything that needs updating isn't newest!
	// if something isn't installed, we can't verify it with this method
	// FIXME: https://github.com/hughsie/PackageKit/issues/116
	updates, e := bus.GetUpdates(filter)
	if e != nil {
		return nil, fmt.Errorf("Updates error: %v", e)
	}
	for _, packageID := range updates {
		//log.Printf("* %v", packageID)
		// format is: name;version;arch;data
		s := strings.Split(packageID, ";")
		//if len(s) != 4 { continue } // this would be a bug!
		pkg, _, _, _ := s[0], s[1], s[2], s[3]
		for index := range packages { // find pkg if it exists
			if pkg == packages[index] {
				state := packageMap[pkg] // lookup
				newest[index] = false
				if state == "installed" || state == "newest" {
					// fix up in case above wasn't correct!
					usePackageID[index] = packageID
				}
				break
			}
		}
	}

	// skip if the "newest" filter was used, otherwise we might need fixing
	// this check is for packages that need to verify their "newest" status
	// we need to know this so we can install the correct newest packageID!
	recursion := make(map[string]*PkPackageIDActionData)
	if !(filter&PK_FILTER_ENUM_NEWEST == PK_FILTER_ENUM_NEWEST) {
		checkPackages := []string{}
		filteredPackageMap := make(map[string]string)
		for index, pkg := range packages {
			state := packageMap[pkg]               // lookup the requested state/version
			if !found[index] || installed[index] { // skip these, they're okay
				continue
			}
			if !(state == "newest" || state == "installed") {
				continue
			}

			checkPackages = append(checkPackages, pkg)
			filteredPackageMap[pkg] = packageMap[pkg] // check me!
		}

		// we _could_ do a second resolve and then parse like this...
		//resolved, e := bus.ResolvePackages(..., filter+PK_FILTER_ENUM_NEWEST)
		// but that's basically what recursion here could do too!
		if len(checkPackages) > 0 {
			if PK_DEBUG {
				log.Printf("PackageKit: PackagesToPackageIDs(): Recurse: %v", strings.Join(checkPackages, ", "))
			}
			recursion, e = bus.PackagesToPackageIDs(filteredPackageMap, filter+PK_FILTER_ENUM_NEWEST)
			if e != nil {
				return nil, fmt.Errorf("Recursion error: %v", e)
			}
		}
	}

	// fix up and build result format
	result := make(map[string]*PkPackageIDActionData)
	for index, pkg := range packages {

		if !found[index] || !installed[index] {
			newest[index] = false // make the results more logical!
		}

		// prefer recursion results if present
		if lookup, ok := recursion[pkg]; ok {
			result[pkg] = lookup
		} else {
			result[pkg] = &PkPackageIDActionData{
				Found:     found[index],
				Installed: installed[index],
				Version:   version[index],
				PackageID: usePackageID[index],
				Newest:    newest[index],
			}
		}
	}

	return result, nil
}

// FilterPackageIDs returns a list of packageIDs which match the set of package names in packages.
func FilterPackageIDs(m map[string]*PkPackageIDActionData, packages []string) ([]string, error) {
	result := []string{}
	for _, k := range packages {
		obj, ok := m[k] // lookup single package
		// package doesn't exist, this is an error!
		if !ok || !obj.Found || obj.PackageID == "" {
			return nil, fmt.Errorf("can't find package named '%s'", k)
		}
		result = append(result, obj.PackageID)
	}
	return result, nil
}

// FilterState returns a map of whether each package queried matches the particular state.
func FilterState(m map[string]*PkPackageIDActionData, packages []string, state string) (result map[string]bool, err error) {
	result = make(map[string]bool)
	pkgs := []string{} // bad pkgs that don't have a bool state
	for _, k := range packages {
		obj, ok := m[k] // lookup single package
		// package doesn't exist, this is an error!
		if !ok || !obj.Found {
			return nil, fmt.Errorf("can't find package named '%s'", k)
		}
		var b bool
		if state == "installed" {
			b = obj.Installed
		} else if state == "uninstalled" {
			b = !obj.Installed
		} else if state == "newest" {
			b = obj.Newest
		} else {
			// we can't filter "version" state in this function
			pkgs = append(pkgs, k)
			continue
		}
		result[k] = b // save
	}
	if len(pkgs) > 0 {
		err = fmt.Errorf("can't filter non-boolean state on: %v", strings.Join(pkgs, ","))
	}
	return result, err
}

// FilterPackageState returns all packages that are in package and match the specific state.
func FilterPackageState(m map[string]*PkPackageIDActionData, packages []string, state string) (result []string, err error) {
	result = []string{}
	for _, k := range packages {
		obj, ok := m[k] // lookup single package
		// package doesn't exist, this is an error!
		if !ok || !obj.Found {
			return nil, fmt.Errorf("can't find package named '%s'", k)
		}
		b := false
		if state == "installed" && obj.Installed {
			b = true
		} else if state == "uninstalled" && !obj.Installed {
			b = true
		} else if state == "newest" && obj.Newest {
			b = true
		} else if state == obj.Version {
			b = true
		}
		if b {
			result = append(result, k)
		}
	}
	return result, err
}

// FlagInData asks whether a flag exists inside the data portion of a packageID field?
func FlagInData(flag, data string) bool {
	flags := strings.Split(data, ":")
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

// FmtTransactionMethod builds the transaction method string properly.
func FmtTransactionMethod(method string) string {
	return fmt.Sprintf("%s.%s", PkIfaceTransaction, method)
}

// IsMyArch determines if a PackageKit architecture matches the current os arch.
func IsMyArch(arch string) bool {
	goarch, ok := PkArchMap[arch]
	if !ok {
		// if you get this error, please update the PkArchMap const
		log.Fatalf("PackageKit: Arch '%v', not found!", arch)
	}
	if goarch == "ANY" { // special value that corresponds to noarch
		return true
	}
	return goarch == runtime.GOARCH
}
