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
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"
)

func init() {
	gob.Register(&PkgRes{})
}

// PkgRes is a package resource for packagekit.
type PkgRes struct {
	BaseRes          `yaml:",inline"`
	State            string `yaml:"state"`            // state: installed, uninstalled, newest, <version>
	AllowUntrusted   bool   `yaml:"allowuntrusted"`   // allow untrusted packages to be installed?
	AllowNonFree     bool   `yaml:"allownonfree"`     // allow nonfree packages to be found?
	AllowUnsupported bool   `yaml:"allowunsupported"` // allow unsupported packages to be found?
	//bus              *Conn    // pk bus connection
	fileList []string // FIXME: update if pkg changes
}

// NewPkgRes is a constructor for this resource. It also calls Init() for you.
func NewPkgRes(name, state string, allowuntrusted, allownonfree, allowunsupported bool) *PkgRes {
	obj := &PkgRes{
		BaseRes: BaseRes{
			Name: name,
		},
		State:            state,
		AllowUntrusted:   allowuntrusted,
		AllowNonFree:     allownonfree,
		AllowUnsupported: allowunsupported,
	}
	obj.Init()
	return obj
}

// Init runs some startup code for this resource.
func (obj *PkgRes) Init() {
	obj.BaseRes.kind = "Pkg"
	obj.BaseRes.Init() // call base init, b/c we're overriding

	bus := NewBus()
	if bus == nil {
		log.Fatal("Can't connect to PackageKit bus.")
	}
	defer bus.Close()

	result, err := obj.pkgMappingHelper(bus)
	if err != nil {
		// FIXME: return error?
		log.Fatalf("The pkgMappingHelper failed with: %v.", err)
		return
	}

	data, ok := result[obj.Name] // lookup single package (init does just one)
	// package doesn't exist, this is an error!
	if !ok || !data.Found {
		// FIXME: return error?
		log.Fatalf("Can't find package named '%s'.", obj.Name)
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

// Validate checks if the resource data structure was populated correctly.
func (obj *PkgRes) Validate() bool {
	if obj.State == "" {
		return false
	}

	return true
}

// Watch is the primary listener for this resource and it outputs events.
// It uses the PackageKit UpdatesChanged signal to watch for changes.
// TODO: https://github.com/hughsie/PackageKit/issues/109
// TODO: https://github.com/hughsie/PackageKit/issues/110
func (obj *PkgRes) Watch(processChan chan Event, delay time.Duration) error {
	if obj.IsWatching() {
		return nil
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	var doSend func() (bool, error) // lol, golang doesn't support recursive lambdas
	doSend = func() (bool, error) {
		resp := NewResp()
		processChan <- Event{eventNil, resp, "", true} // trigger process
		select {
		case e := <-resp: // wait for the ACK()
			if e != nil { // we got a NACK
				return true, e // exit with error
			}

		case event := <-obj.events:
			// NOTE: this code should match the similar code below!
			cuuid.SetConverged(false)
			if exit, send := obj.ReadEvent(&event); exit {
				return true, nil // exit, without error
			} else if send {
				return doSend() // recurse
			}
		}
		return false, nil // return, no error or exit signal
	}

	// if a retry-delay was requested, wait, but don't block our events!
	if delay > 0 {
		var pendingSendEvent bool
		timer := time.NewTimer(delay)
	Loop:
		for {
			select {
			case <-timer.C: // the wait is over
				break Loop // critical

			case event := <-obj.events:
				// NOTE: this code should match the similar code below!
				cuuid.SetConverged(false)
				if exit, send := obj.ReadEvent(&event); exit {
					return nil // exit
				} else if send {
					// NOTE: see long comment in the file resource
					//if exit, err := doSend(); exit || err != nil {
					//	return err // we exit or bubble up a NACK...
					//}
					pendingSendEvent = true // all events are identical for now...
				}
			}
		}
		timer.Stop() // it's nice to cleanup
		log.Printf("%s[%s]: Delay expired!", obj.Kind(), obj.GetName())
		if pendingSendEvent { // TODO: should this become a list in the future?
			if exit, err := doSend(); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}

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
			log.Printf("%v: Watching...", obj.fmtNames(obj.getNames()))
		}

		obj.SetState(resStateWatching) // reset
		select {
		case event := <-ch:
			cuuid.SetConverged(false)

			// FIXME: ask packagekit for info on what packages changed
			if DEBUG {
				log.Printf("%v: Event: %v", obj.fmtNames(obj.getNames()), event.Name)
			}

			// since the chan is buffered, remove any supplemental
			// events since they would just be duplicates anyways!
			for len(ch) > 0 { // we can detect pending count here!
				<-ch // discard
			}

			send = true
			dirty = true

		case event := <-obj.events:
			cuuid.SetConverged(false)
			if exit, send = obj.ReadEvent(&event); exit {
				return nil // exit
			}
			dirty = false // these events don't invalidate state

		case <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!
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
			if exit, err := doSend(); exit || err != nil {
				return err // we exit or bubble up a NACK...
			}
		}
	}
}

// get list of names when grouped or not
func (obj *PkgRes) getNames() []string {
	if g := obj.GetGroup(); len(g) > 0 { // grouped elements
		names := []string{obj.GetName()}
		for _, x := range g {
			pkg, ok := x.(*PkgRes) // convert from Res
			if ok {
				names = append(names, pkg.Name)
			}
		}
		return names
	}
	return []string{obj.GetName()}
}

// pretty print for header values
func (obj *PkgRes) fmtNames(names []string) string {
	if len(obj.GetGroup()) > 0 { // grouped elements
		return fmt.Sprintf("%v[autogroup:(%v)]", obj.Kind(), strings.Join(names, ","))
	}
	return fmt.Sprintf("%v[%v]", obj.Kind(), obj.GetName())
}

func (obj *PkgRes) groupMappingHelper() map[string]string {
	var result = make(map[string]string)
	if g := obj.GetGroup(); len(g) > 0 { // add any grouped elements
		for _, x := range g {
			pkg, ok := x.(*PkgRes) // convert from Res
			if !ok {
				log.Fatalf("Grouped member %v is not a %v", x, obj.Kind())
			}
			result[pkg.Name] = pkg.State
		}
	}
	return result
}

func (obj *PkgRes) pkgMappingHelper(bus *Conn) (map[string]*PkPackageIDActionData, error) {
	packageMap := obj.groupMappingHelper() // get the grouped values
	packageMap[obj.Name] = obj.State       // key is pkg name, value is pkg state
	var filter uint64                      // initializes at the "zero" value of 0
	filter += PK_FILTER_ENUM_ARCH          // always search in our arch (optional!)
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
	return result, nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *PkgRes) CheckApply(apply bool) (checkok bool, err error) {
	log.Printf("%v: CheckApply(%t)", obj.fmtNames(obj.getNames()), apply)

	if obj.State == "" { // TODO: Validate() should replace this check!
		log.Fatalf("%v: Package state is undefined!", obj.fmtNames(obj.getNames()))
	}

	if obj.isStateOK { // cache the state
		return true, nil
	}

	bus := NewBus()
	if bus == nil {
		return false, errors.New("Can't connect to PackageKit bus.")
	}
	defer bus.Close()

	result, err := obj.pkgMappingHelper(bus)
	if err != nil {
		return false, fmt.Errorf("The pkgMappingHelper failed with: %v.", err)
	}

	packageMap := obj.groupMappingHelper() // map[string]string
	packageList := []string{obj.Name}
	packageList = append(packageList, StrMapKeys(packageMap)...)
	//stateList := []string{obj.State}
	//stateList = append(stateList, StrMapValues(packageMap)...)

	// TODO: at the moment, all the states are the same, but
	// eventually we might be able to drop this constraint!
	states, err := FilterState(result, packageList, obj.State)
	if err != nil {
		return false, fmt.Errorf("The FilterState method failed with: %v.", err)
	}
	data, _ := result[obj.Name] // if above didn't error, we won't either!
	validState := BoolMapTrue(BoolMapValues(states))

	// obj.State == "installed" || "uninstalled" || "newest" || "4.2-1.fc23"
	switch obj.State {
	case "installed":
		fallthrough
	case "uninstalled":
		fallthrough
	case "newest":
		if validState {
			obj.isStateOK = true // reset
			return true, nil     // state is correct, exit!
		}
	default: // version string
		if obj.State == data.Version && data.Version != "" {
			obj.isStateOK = true // reset
			return true, nil
		}
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%v: Apply", obj.fmtNames(obj.getNames()))
	readyPackages, err := FilterPackageState(result, packageList, obj.State)
	if err != nil {
		return false, err // fail
	}
	// these are the packages that actually need their states applied!
	applyPackages := StrFilterElementsInList(readyPackages, packageList)
	packageIDs, _ := FilterPackageIDs(result, applyPackages) // would be same err as above

	var transactionFlags uint64 // initializes at the "zero" value of 0
	if !obj.AllowUntrusted {    // allow
		transactionFlags += PK_TRANSACTION_FLAG_ENUM_ONLY_TRUSTED
	}
	// apply correct state!
	log.Printf("%v: Set: %v...", obj.fmtNames(StrListIntersection(applyPackages, obj.getNames())), obj.State)
	switch obj.State {
	case "uninstalled": // run remove
		// NOTE: packageID is different than when installed, because now
		// it has the "installed" flag added to the data portion if it!!
		err = bus.RemovePackages(packageIDs, transactionFlags)

	case "newest": // TODO: isn't this the same operation as install, below?
		err = bus.UpdatePackages(packageIDs, transactionFlags)

	case "installed":
		fallthrough // same method as for "set specific version", below
	default: // version string
		err = bus.InstallPackages(packageIDs, transactionFlags)
	}
	if err != nil {
		return false, err // fail
	}
	log.Printf("%v: Set: %v success!", obj.fmtNames(StrListIntersection(applyPackages, obj.getNames())), obj.State)
	obj.isStateOK = true // reset
	return false, nil    // success
}

// PkgUUID is the UUID struct for PkgRes.
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

// PkgResAutoEdges holds the state of the auto edge generator.
type PkgResAutoEdges struct {
	fileList   []string
	svcUUIDs   []ResUUID
	testIsNext bool   // safety
	name       string // saved data from PkgRes obj
	kind       string
}

// Next returns the next automatic edge.
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

// Test gets results of the earlier Next() call, & returns if we should continue!
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

// AutoEdges produces an object which generates a minimal pkg file optimization
// sequence of edges.
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

// GetUUIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *PkgRes) GetUUIDs() []ResUUID {
	x := &PkgUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		name:     obj.Name,
		state:    obj.State,
	}
	result := []ResUUID{x}
	return result
}

// GroupCmp returns whether two resources can be grouped together or not.
// can these two resources be merged ?
// (aka does this resource support doing so?)
// will resource allow itself to be grouped _into_ this obj?
func (obj *PkgRes) GroupCmp(r Res) bool {
	res, ok := r.(*PkgRes)
	if !ok {
		return false
	}
	objStateIsVersion := (obj.State != "installed" && obj.State != "uninstalled" && obj.State != "newest") // must be a ver. string
	resStateIsVersion := (res.State != "installed" && res.State != "uninstalled" && res.State != "newest") // must be a ver. string
	if objStateIsVersion || resStateIsVersion {
		// can't merge specific version checks atm
		return false
	}
	// FIXME: keep it simple for now, only merge same states
	if obj.State != res.State {
		return false
	}
	return true
}

// Compare two resources and return if they are equivalent.
func (obj *PkgRes) Compare(res Res) bool {
	switch res.(type) {
	case *PkgRes:
		res := res.(*PkgRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}

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
