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

// Package packagekit provides an interface to interact with packagekit.
// See: https://www.freedesktop.org/software/PackageKit/gtk-doc/index.html for
// more information.
package packagekit

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/purpleidea/mgmt/util"

	"github.com/godbus/dbus"
	errwrap "github.com/pkg/errors"
)

// global tweaks of verbosity and code path
const (
	Paranoid = false // enable if you see any ghosts
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
	PkFilterEnumUnknown        uint64 = 1 << iota // "unknown"
	PkFilterEnumNone                              // "none"
	PkFilterEnumInstalled                         // "installed"
	PkFilterEnumNotInstalled                      // "~installed"
	PkFilterEnumDevelopment                       // "devel"
	PkFilterEnumNotDevelopment                    // "~devel"
	PkFilterEnumGui                               // "gui"
	PkFilterEnumNotGui                            // "~gui"
	PkFilterEnumFree                              // "free"
	PkFilterEnumNotFree                           // "~free"
	PkFilterEnumVisible                           // "visible"
	PkFilterEnumNotVisible                        // "~visible"
	PkFilterEnumSupported                         // "supported"
	PkFilterEnumNotSupported                      // "~supported"
	PkFilterEnumBasename                          // "basename"
	PkFilterEnumNotBasename                       // "~basename"
	PkFilterEnumNewest                            // "newest"
	PkFilterEnumNotNewest                         // "~newest"
	PkFilterEnumArch                              // "arch"
	PkFilterEnumNotArch                           // "~arch"
	PkFilterEnumSource                            // "source"
	PkFilterEnumNotSource                         // "~source"
	PkFilterEnumCollections                       // "collections"
	PkFilterEnumNotCollections                    // "~collections"
	PkFilterEnumApplication                       // "application"
	PkFilterEnumNotApplication                    // "~application"
	PkFilterEnumDownloaded                        // "downloaded"
	PkFilterEnumNotDownloaded                     // "~downloaded"
)

// constants from packagekit c library.
const ( //static const PkEnumMatch enum_transaction_flag[]
	PkTransactionFlagEnumNone           uint64 = 1 << iota // "none"
	PkTransactionFlagEnumOnlyTrusted                       // "only-trusted"
	PkTransactionFlagEnumSimulate                          // "simulate"
	PkTransactionFlagEnumOnlyDownload                      // "only-download"
	PkTransactionFlagEnumAllowReinstall                    // "allow-reinstall"
	PkTransactionFlagEnumJustReinstall                     // "just-reinstall"
	PkTransactionFlagEnumAllowDowngrade                    // "allow-downgrade"
)

// constants from packagekit c library.
const ( //typedef enum
	PkInfoEnumUnknown uint64 = 1 << iota
	PkInfoEnumInstalled
	PkInfoEnumAvailable
	PkInfoEnumLow
	PkInfoEnumEnhancement
	PkInfoEnumNormal
	PkInfoEnumBugfix
	PkInfoEnumImportant
	PkInfoEnumSecurity
	PkInfoEnumBlocked
	PkInfoEnumDownloading
	PkInfoEnumUpdating
	PkInfoEnumInstalling
	PkInfoEnumRemoving
	PkInfoEnumCleanup
	PkInfoEnumObsoleting
	PkInfoEnumCollectionInstalled
	PkInfoEnumCollectionAvailable
	PkInfoEnumFinished
	PkInfoEnumReinstalling
	PkInfoEnumDowngrading
	PkInfoEnumPreparing
	PkInfoEnumDecompressing
	PkInfoEnumUntrusted
	PkInfoEnumTrusted
	PkInfoEnumUnavailable
	PkInfoEnumLast
)

// Conn is a wrapper struct so we can pass bus connection around in the struct.
type Conn struct {
	conn *dbus.Conn

	Debug bool
	Logf  func(format string, v ...interface{})
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
func (obj *Conn) GetBus() *dbus.Conn {
	return obj.conn
}

// Close closes the dbus connection object.
func (obj *Conn) Close() error {
	return obj.conn.Close()
}

// internal helper to add signal matches to the bus, should only be called once
func (obj *Conn) matchSignal(ch chan *dbus.Signal, path dbus.ObjectPath, iface string, signals []string) error {
	if obj.Debug {
		obj.Logf("matchSignal(%v, %v, %s, %v)", ch, path, iface, signals)
	}
	// eg: gdbus monitor --system --dest org.freedesktop.PackageKit --object-path /org/freedesktop/PackageKit | grep <signal>
	var call *dbus.Call
	// TODO: if we make this call many times, we seem to receive signals
	// that many times... Maybe this should be an object singleton?
	bus := obj.GetBus().BusObject()
	pathStr := fmt.Sprintf("%s", path)
	if len(signals) == 0 {
		call = bus.Call(dbusAddMatch, 0, "type='signal',path='"+pathStr+"',interface='"+iface+"'")
	} else {
		for _, signal := range signals {
			call = bus.Call(dbusAddMatch, 0, "type='signal',path='"+pathStr+"',interface='"+iface+"',member='"+signal+"'")
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
	obj.GetBus().Signal(ch)
	return nil
}

// WatchChanges gets a signal anytime an event happens.
func (obj *Conn) WatchChanges() (chan *dbus.Signal, error) {
	ch := make(chan *dbus.Signal, PkBufferSize)
	// NOTE: the TransactionListChanged signal fires much more frequently,
	// but with much less specificity. If we're missing events, report the
	// issue upstream! The UpdatesChanged signal is what hughsie suggested
	var signal = "UpdatesChanged"
	err := obj.matchSignal(ch, PkPath, PkIface, []string{signal})
	if err != nil {
		return nil, err
	}
	if Paranoid { // TODO: this filtering might not be necessary anymore...
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
						obj.Logf("Hrm, channel was closed!")
						break loop // TODO: continue?
					}
					// i think this was caused by using the shared
					// bus, but we might as well leave it in for now
					if event.Path != PkPath || event.Name != fmt.Sprintf("%s.%s", PkIface, signal) {
						obj.Logf("Woops: Event: %+v", event)
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
func (obj *Conn) CreateTransaction() (dbus.ObjectPath, error) {
	if obj.Debug {
		obj.Logf("CreateTransaction()")
	}
	var interfacePath dbus.ObjectPath
	bus := obj.GetBus().Object(PkIface, PkPath)
	call := bus.Call(fmt.Sprintf("%s.CreateTransaction", PkIface), 0).Store(&interfacePath)
	if call != nil {
		return "", call
	}
	if obj.Debug {
		obj.Logf("CreateTransaction(): %v", interfacePath)
	}
	return interfacePath, nil
}

// ResolvePackages runs the PackageKit Resolve method and returns the result.
func (obj *Conn) ResolvePackages(packages []string, filter uint64) ([]string, error) {
	packageIDs := []string{}
	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := obj.CreateTransaction() // emits Destroy on close
	if err != nil {
		return []string{}, err
	}

	// add signal matches for Package and Finished which will always be last
	var signals = []string{"Package", "Finished", "Error", "Destroy"}
	obj.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)
	if obj.Debug {
		obj.Logf("ResolvePackages(): Object(%s, %v)", PkIface, interfacePath)
	}
	bus := obj.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := bus.Call(FmtTransactionMethod("Resolve"), 0, filter, packages)
	if obj.Debug {
		obj.Logf("ResolvePackages(): Call: Success!")
	}
	if call.Err != nil {
		return []string{}, call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if obj.Debug {
				obj.Logf("ResolvePackages(): Signal: %+v", signal)
			}
			if signal.Path != interfacePath {
				obj.Logf("Woops: Signal.Path: %+v", signal.Path)
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
func (obj *Conn) IsInstalledList(packages []string) ([]bool, error) {
	var filter uint64          // initializes at the "zero" value of 0
	filter += PkFilterEnumArch // always search in our arch
	packageIDs, e := obj.ResolvePackages(packages, filter)
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
func (obj *Conn) IsInstalled(pkg string) (bool, error) {
	p, e := obj.IsInstalledList([]string{pkg})
	if len(p) != 1 {
		return false, e
	}
	return p[0], nil
}

// InstallPackages installs a list of packages by packageID.
func (obj *Conn) InstallPackages(packageIDs []string, transactionFlags uint64) error {

	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := obj.CreateTransaction() // emits Destroy on close
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	obj.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	bus := obj.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := bus.Call(FmtTransactionMethod("RefreshCache"), 0, false)
	if call.Err != nil {
		return call.Err
	}
	call = bus.Call(FmtTransactionMethod("InstallPackages"), 0, transactionFlags, packageIDs)
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
				obj.Logf("Woops: Signal.Path: %+v", signal.Path)
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
				obj.Logf("Timeout: InstallPackages: Waiting for 'Destroy'")
				return nil // got tired of waiting for Destroy
			}
			return fmt.Errorf("PackageKit: Timeout: InstallPackages: %s", strings.Join(packageIDs, ", "))
		}
	}
}

// RemovePackages removes a list of packages by packageID.
func (obj *Conn) RemovePackages(packageIDs []string, transactionFlags uint64) error {

	var allowDeps = true                          // TODO: configurable
	var autoremove = false                        // unsupported on GNU/Linux
	ch := make(chan *dbus.Signal, PkBufferSize)   // we need to buffer :(
	interfacePath, err := obj.CreateTransaction() // emits Destroy on close
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	obj.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	bus := obj.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := bus.Call(FmtTransactionMethod("RemovePackages"), 0, transactionFlags, packageIDs, allowDeps, autoremove)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				obj.Logf("Woops: Signal.Path: %+v", signal.Path)
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
func (obj *Conn) UpdatePackages(packageIDs []string, transactionFlags uint64) error {
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := obj.CreateTransaction()
	if err != nil {
		return err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	obj.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	bus := obj.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := bus.Call(FmtTransactionMethod("UpdatePackages"), 0, transactionFlags, packageIDs)
	if call.Err != nil {
		return call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				obj.Logf("Woops: Signal.Path: %+v", signal.Path)
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
func (obj *Conn) GetFilesByPackageID(packageIDs []string) (files map[string][]string, err error) {
	// NOTE: the maximum number of files in an RPM is 52116 in Fedora 23
	// https://gist.github.com/purpleidea/b98e60dcd449e1ac3b8a
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := obj.CreateTransaction()
	if err != nil {
		return
	}

	var signals = []string{"Files", "ErrorCode", "Finished", "Destroy"} // "ItemProgress", "Status" ?
	obj.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	bus := obj.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := bus.Call(FmtTransactionMethod("GetFiles"), 0, packageIDs)
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
				obj.Logf("Woops: Signal.Path: %+v", signal.Path)
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
func (obj *Conn) GetUpdates(filter uint64) ([]string, error) {
	if obj.Debug {
		obj.Logf("GetUpdates()")
	}
	packageIDs := []string{}
	ch := make(chan *dbus.Signal, PkBufferSize) // we need to buffer :(
	interfacePath, err := obj.CreateTransaction()
	if err != nil {
		return nil, err
	}

	var signals = []string{"Package", "ErrorCode", "Finished", "Destroy"} // "ItemProgress" ?
	obj.matchSignal(ch, interfacePath, PkIfaceTransaction, signals)

	bus := obj.GetBus().Object(PkIface, interfacePath) // pass in found transaction path
	call := bus.Call(FmtTransactionMethod("GetUpdates"), 0, filter)
	if call.Err != nil {
		return nil, call.Err
	}
loop:
	for {
		// FIXME: add a timeout option to error in case signals are dropped!
		select {
		case signal := <-ch:
			if signal.Path != interfacePath {
				obj.Logf("Woops: Signal.Path: %+v", signal.Path)
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
func (obj *Conn) PackagesToPackageIDs(packageMap map[string]string, filter uint64) (map[string]*PkPackageIDActionData, error) {
	count := 0
	packages := make([]string, len(packageMap))
	for k := range packageMap { // lol, golang has no hash.keys() function!
		packages[count] = k
		count++
	}

	if !(filter&PkFilterEnumArch == PkFilterEnumArch) {
		filter += PkFilterEnumArch // always search in our arch
	}

	if obj.Debug {
		obj.Logf("PackagesToPackageIDs(): %s", strings.Join(packages, ", "))
	}
	resolved, e := obj.ResolvePackages(packages, filter)
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
		//obj.Logf("* %v", packageID)
		// format is: name;version;arch;data
		s := strings.Split(packageID, ";")
		//if len(s) != 4 { continue } // this would be a bug!
		pkg, ver, arch, data := s[0], s[1], s[2], s[3]
		// we might need to allow some of this, eg: i386 .deb on amd64
		b, err := IsMyArch(arch)
		if err != nil {
			return nil, errwrap.Wrapf(err, "arch error")
		} else if !b {
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
	updates, e := obj.GetUpdates(filter)
	if e != nil {
		return nil, fmt.Errorf("Updates error: %v", e)
	}
	for _, packageID := range updates {
		//obj.Logf("* %v", packageID)
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
	if !(filter&PkFilterEnumNewest == PkFilterEnumNewest) {
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
		//resolved, e := obj.ResolvePackages(..., filter+PkFilterEnumNewest)
		// but that's basically what recursion here could do too!
		if len(checkPackages) > 0 {
			if obj.Debug {
				obj.Logf("PackagesToPackageIDs(): Recurse: %s", strings.Join(checkPackages, ", "))
			}
			recursion, e = obj.PackagesToPackageIDs(filteredPackageMap, filter+PkFilterEnumNewest)
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
		p, ok := m[k] // lookup single package
		// package doesn't exist, this is an error!
		if !ok || !p.Found || p.PackageID == "" {
			return nil, fmt.Errorf("can't find package named '%s'", k)
		}
		result = append(result, p.PackageID)
	}
	return result, nil
}

// FilterState returns a map of whether each package queried matches the particular state.
func FilterState(m map[string]*PkPackageIDActionData, packages []string, state string) (result map[string]bool, err error) {
	result = make(map[string]bool)
	pkgs := []string{} // bad pkgs that don't have a bool state
	for _, k := range packages {
		p, ok := m[k] // lookup single package
		// package doesn't exist, this is an error!
		if !ok || !p.Found {
			return nil, fmt.Errorf("can't find package named '%s'", k)
		}
		var b bool
		if state == "installed" {
			b = p.Installed
		} else if state == "uninstalled" {
			b = !p.Installed
		} else if state == "newest" {
			b = p.Newest
		} else {
			// we can't filter "version" state in this function
			pkgs = append(pkgs, k)
			continue
		}
		result[k] = b // save
	}
	if len(pkgs) > 0 {
		err = fmt.Errorf("can't filter non-boolean state on: %s", strings.Join(pkgs, ","))
	}
	return result, err
}

// FilterPackageState returns all packages that are in package and match the specific state.
func FilterPackageState(m map[string]*PkPackageIDActionData, packages []string, state string) (result []string, err error) {
	result = []string{}
	for _, k := range packages {
		p, ok := m[k] // lookup single package
		// package doesn't exist, this is an error!
		if !ok || !p.Found {
			return nil, fmt.Errorf("can't find package named '%s'", k)
		}
		b := false
		if state == "installed" && p.Installed {
			b = true
		} else if state == "uninstalled" && !p.Installed {
			b = true
		} else if state == "newest" && p.Newest {
			b = true
		} else if state == p.Version {
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
func IsMyArch(arch string) (bool, error) {
	goarch, ok := PkArchMap[arch]
	if !ok {
		// if you get this error, please update the PkArchMap const
		return false, fmt.Errorf("arch '%s', not found", arch)
	}
	if goarch == "ANY" { // special value that corresponds to noarch
		return true, nil
	}
	return goarch == runtime.GOARCH, nil
}
