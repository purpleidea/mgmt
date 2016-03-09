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
	"path"
	"strings"
)

type PkgRes struct {
	BaseRes          `yaml:",inline"`
	State            string `yaml:"state"`            // state: installed, uninstalled, newest, <version>
	AllowUntrusted   bool   `yaml:"allowuntrusted"`   // allow untrusted packages to be installed?
	AllowNonFree     bool   `yaml:"allownonfree"`     // allow nonfree packages to be found?
	AllowUnsupported bool   `yaml:"allowunsupported"` // allow unsupported packages to be found?
	//bus              *Conn    // pk bus connection
	fileList []string // FIXME: update if pkg changes
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

	bus := NewBus()
	if bus == nil {
		log.Fatal("Can't connect to PackageKit bus.")
	}
	defer bus.Close()

	data, err := obj.PkgMappingHelper(bus)
	if err != nil {
		// FIXME: return error?
		log.Fatalf("The PkgMappingHelper failed with: %v.", err)
		return
	}

	packageIDs := []string{data.PackageID} // just one for now
	filesMap, err := bus.GetFilesByPackageID(packageIDs)
	if err != nil {
		// FIXME: return error?
		log.Fatalf("Can't run GetFilesByPackageID: %v", err)
		return
	}
	if files, ok := filesMap[data.PackageID]; ok {
		obj.fileList = DirifyFileList(files, false)
	}
}

// XXX: run this when resource exits
func (obj *PkgRes) Close() {
	//obj.bus.Close()
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

func (obj *PkgRes) PkgMappingHelper(bus *Conn) (*PkPackageIDActionData, error) {

	var packageMap = map[string]string{
		obj.Name: obj.State, // key is pkg name, value is pkg state
	}
	var filter uint64             // initializes at the "zero" value of 0
	filter += PK_FILTER_ENUM_ARCH // always search in our arch (optional!)
	// we're requesting latest version, or to narrow down install choices!
	if obj.State == "newest" || obj.State == "installed" {
		// if we add this, we'll still see older packages if installed
		// this is an optimization, and is *optional*, this logic is
		// handled inside of PackagesToPackageIDs now automatically!
		filter += PK_FILTER_ENUM_NEWEST // only search for newest packages
	}
	if !obj.AllowNonFree {
		filter += PK_FILTER_ENUM_FREE
	}
	if !obj.AllowUnsupported {
		filter += PK_FILTER_ENUM_SUPPORTED
	}
	result, e := bus.PackagesToPackageIDs(packageMap, filter)
	if e != nil {
		return nil, fmt.Errorf("Can't run PackagesToPackageIDs: %v", e)
	}

	data, ok := result[obj.Name] // lookup single package
	// package doesn't exist, this is an error!
	if !ok || !data.Found {
		return nil, fmt.Errorf("Can't find package named '%s'.", obj.Name)
	}

	return data, nil
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

	data, err := obj.PkgMappingHelper(bus)
	if err != nil {
		return false, fmt.Errorf("The PkgMappingHelper failed with: %v.", err)
	}

	// obj.State == "installed" || "uninstalled" || "newest" || "4.2-1.fc23"
	switch obj.State {
	case "installed":
		if data.Installed {
			return true, nil // state is correct, exit!
		}
	case "uninstalled":
		if !data.Installed {
			return true, nil
		}
	case "newest":
		if data.Newest {
			return true, nil
		}
	default: // version string
		if obj.State == data.Version && data.Version != "" {
			return true, nil
		}
	}

	if data.PackageID == "" {
		return false, errors.New("Can't find package id to use.")
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%v[%v]: Apply", obj.Kind(), obj.GetName())
	packageList := []string{data.PackageID}
	var transactionFlags uint64 // initializes at the "zero" value of 0
	if !obj.AllowUntrusted {    // allow
		transactionFlags += PK_TRANSACTION_FLAG_ENUM_ONLY_TRUSTED
	}
	// apply correct state!
	log.Printf("%v[%v]: Set: %v...", obj.Kind(), obj.GetName(), obj.State)
	switch obj.State {
	case "uninstalled": // run remove
		// NOTE: packageID is different than when installed, because now
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

type PkgUUID struct {
	BaseUUID
	name  string // pkg name
	state string // pkg state or "version"
}

// if and only if they are equivalent, return true
// if they are not equivalent, return false
func (obj *PkgUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*PkgUUID)
	if !ok {
		return false
	}
	// FIXME: match on obj.state vs. res.state ?
	return obj.name == res.name
}

type PkgResAutoEdges struct {
	fileList   []string
	svcUUIDs   []ResUUID
	testIsNext bool   // safety
	name       string // saved data from PkgRes obj
	kind       string
}

func (obj *PkgResAutoEdges) Next() []ResUUID {
	if obj.testIsNext {
		log.Fatal("Expecting a call to Test()")
	}
	obj.testIsNext = true // set after all the errors paths are past

	// first return any matching svcUUIDs
	if x := obj.svcUUIDs; len(x) > 0 {
		return x
	}

	var result []ResUUID
	// return UUID's for whatever is in obj.fileList
	for _, x := range obj.fileList {
		var reversed = false // cheat by passing a pointer
		result = append(result, &FileUUID{
			BaseUUID: BaseUUID{
				name:     obj.name,
				kind:     obj.kind,
				reversed: &reversed,
			},
			path: x, // what matters
		}) // build list
	}
	return result
}

func (obj *PkgResAutoEdges) Test(input []bool) bool {
	if !obj.testIsNext {
		log.Fatal("Expecting a call to Next()")
	}

	// ack the svcUUID's...
	if x := obj.svcUUIDs; len(x) > 0 {
		if y := len(x); y != len(input) {
			log.Fatalf("Expecting %d value(s)!", y)
		}
		obj.svcUUIDs = []ResUUID{} // empty
		obj.testIsNext = false
		return true
	}

	count := len(obj.fileList)
	if count != len(input) {
		log.Fatalf("Expecting %d value(s)!", count)
	}
	obj.testIsNext = false // set after all the errors paths are past

	// while i do believe this algorithm generates the *correct* result, i
	// don't know if it does so in the optimal way. improvements welcome!
	// the basic logic is:
	// 0) Next() returns whatever is in fileList
	// 1) Test() computes the dirname of each file, and removes duplicates
	// and dirname's that have been in the path of an ack from input results
	// 2) It then simplifies the list by removing the common path prefixes
	// 3) Lastly, the remaining set of files (dirs) is used as new fileList
	// 4) We then iterate in (0) until the fileList is empty!
	var dirs = make([]string, count)
	done := []string{}
	for i := 0; i < count; i++ {
		dir := Dirname(obj.fileList[i]) // dirname of /foo/ should be /
		dirs[i] = dir
		if input[i] {
			done = append(done, dir)
		}
	}
	nodupes := StrRemoveDuplicatesInList(dirs)                // remove duplicates
	nodones := StrFilterElementsInList(done, nodupes)         // filter out done
	noempty := StrFilterElementsInList([]string{""}, nodones) // remove the "" from /
	obj.fileList = RemoveCommonFilePrefixes(noempty)          // magic

	if len(obj.fileList) == 0 { // nothing more, don't continue
		return false
	}
	return true // continue, there are more files!
}

// produce an object which generates a minimal pkg file optimization sequence
func (obj *PkgRes) AutoEdges() AutoEdge {
	// in contrast with the FileRes AutoEdges() function which contains
	// more of the mechanics, most of the AutoEdge mechanics for the PkgRes
	// is contained in the Test() method! This design is completely okay!

	// add matches for any svc resources found in pkg definition!
	var svcUUIDs []ResUUID
	for _, x := range ReturnSvcInFileList(obj.fileList) {
		var reversed = false
		svcUUIDs = append(svcUUIDs, &SvcUUID{
			BaseUUID: BaseUUID{
				name:     obj.GetName(),
				kind:     obj.Kind(),
				reversed: &reversed,
			},
			name: x, // the svc name itself in the SvcUUID object!
		}) // build list
	}

	return &PkgResAutoEdges{
		fileList:   RemoveCommonFilePrefixes(obj.fileList), // clean start!
		svcUUIDs:   svcUUIDs,
		testIsNext: false,         // start with Next() call
		name:       obj.GetName(), // save data for PkgResAutoEdges obj
		kind:       obj.Kind(),
	}
}

// include all params to make a unique identification of this object
func (obj *PkgRes) GetUUIDs() []ResUUID {
	x := &PkgUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		name:     obj.Name,
		state:    obj.State,
	}
	result := []ResUUID{x}
	return result
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

// return a list of svc names for matches like /usr/lib/systemd/system/*.service
func ReturnSvcInFileList(fileList []string) []string {
	result := []string{}
	for _, x := range fileList {
		dirname, basename := path.Split(path.Clean(x))
		// TODO: do we also want to look for /etc/systemd/system/ ?
		if dirname != "/usr/lib/systemd/system/" {
			continue
		}
		if !strings.HasSuffix(basename, ".service") {
			continue
		}
		if s := strings.TrimSuffix(basename, ".service"); !StrInList(s, result) {
			result = append(result, s)
		}
	}
	return result
}
