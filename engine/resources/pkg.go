// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
	"path"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources/packagekit"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("pkg", func() engine.Res { return &PkgRes{} })
}

const (
	// PkgStateInstalled is the string that represents that the package
	// should be installed.
	PkgStateInstalled = "installed"

	// PkgStateUninstalled is the string that represents that the package
	// should be uninstalled.
	PkgStateUninstalled = "uninstalled"

	// PkgStateNewest is the string that represents that the package should
	// be installed in the newest available version.
	PkgStateNewest = "newest"
)

// PkgRes is a package resource for packagekit.
type PkgRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Groupable

	init *engine.Init

	State            string `yaml:"state"`            // state: installed, uninstalled, newest, <version>
	AllowUntrusted   bool   `yaml:"allowuntrusted"`   // allow untrusted packages to be installed?
	AllowNonFree     bool   `yaml:"allownonfree"`     // allow nonfree packages to be found?
	AllowUnsupported bool   `yaml:"allowunsupported"` // allow unsupported packages to be found?
	//bus              *packagekit.Conn    // pk bus connection
	fileList []string // FIXME: update if pkg changes
}

// Default returns some sensible defaults for this resource.
func (obj *PkgRes) Default() engine.Res {
	return &PkgRes{
		State: PkgStateInstalled, // this *is* preferable to "newest"
	}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *PkgRes) Validate() error {
	if obj.State == "" {
		return fmt.Errorf("state cannot be empty")
	}
	if obj.State == "latest" {
		return fmt.Errorf("state is invalid, did you mean `newest` ?")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *PkgRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	if obj.fileList == nil {
		if err := obj.populateFileList(); err != nil {
			return errwrap.Wrapf(err, "error populating file list in init")
		}
	}

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *PkgRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
// It uses the PackageKit UpdatesChanged signal to watch for changes.
// TODO: https://github.com/hughsie/PackageKit/issues/109
// TODO: https://github.com/hughsie/PackageKit/issues/110
func (obj *PkgRes) Watch() error {
	bus := packagekit.NewBus()
	if bus == nil {
		return fmt.Errorf("can't connect to PackageKit bus")
	}
	defer bus.Close()
	bus.Debug = obj.init.Debug
	bus.Logf = func(format string, v ...interface{}) {
		obj.init.Logf("packagekit: "+format, v...)
	}

	ch, err := bus.WatchChanges()
	if err != nil {
		return errwrap.Wrapf(err, "error adding signal match")
	}

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("%s: Watching...", obj.fmtNames(obj.getNames()))
		}

		select {
		case event := <-ch:
			// FIXME: ask packagekit for info on what packages changed
			if obj.init.Debug {
				obj.init.Logf("Event(%s): %s", event.Name, obj.fmtNames(obj.getNames()))
			}

			// since the chan is buffered, remove any supplemental
			// events since they would just be duplicates anyways!
			for len(ch) > 0 { // we can detect pending count here!
				<-ch // discard
			}

			send = true

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// get list of names when grouped or not
func (obj *PkgRes) getNames() []string {
	if g := obj.GetGroup(); len(g) > 0 { // grouped elements
		names := []string{obj.Name()}
		for _, x := range g {
			pkg, ok := x.(*PkgRes) // convert from Res
			if ok {
				names = append(names, pkg.Name())
			}
		}
		return names
	}
	return []string{obj.Name()}
}

// pretty print for header values
func (obj *PkgRes) fmtNames(names []string) string {
	if len(obj.GetGroup()) > 0 { // grouped elements
		return fmt.Sprintf("%s[autogroup:(%s)]", obj.Kind(), strings.Join(names, ","))
	}
	return obj.String()
}

func (obj *PkgRes) groupMappingHelper() map[string]string {
	var result = make(map[string]string)
	if g := obj.GetGroup(); len(g) > 0 { // add any grouped elements
		for _, x := range g {
			pkg, ok := x.(*PkgRes) // convert from Res
			if !ok {
				panic(fmt.Sprintf("grouped member %v is not a %s", x, obj.Kind()))
			}
			result[pkg.Name()] = pkg.State
		}
	}
	return result
}

func (obj *PkgRes) pkgMappingHelper(bus *packagekit.Conn) (map[string]*packagekit.PkPackageIDActionData, error) {
	packageMap := obj.groupMappingHelper() // get the grouped values
	packageMap[obj.Name()] = obj.State     // key is pkg name, value is pkg state
	var filter uint64                      // initializes at the "zero" value of 0
	filter += packagekit.PkFilterEnumArch  // always search in our arch (optional!)
	// we're requesting newest version, or to narrow down install choices!
	if obj.State == PkgStateNewest || obj.State == PkgStateInstalled {
		// if we add this, we'll still see older packages if installed
		// this is an optimization, and is *optional*, this logic is
		// handled inside of PackagesToPackageIDs now automatically!
		filter += packagekit.PkFilterEnumNewest // only search for newest packages
	}
	if !obj.AllowNonFree {
		filter += packagekit.PkFilterEnumFree
	}
	if !obj.AllowUnsupported {
		filter += packagekit.PkFilterEnumSupported
	}
	result, err := bus.PackagesToPackageIDs(packageMap, filter)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't run PackagesToPackageIDs")
	}
	return result, nil
}

// populateFileList fills in the fileList structure with what is in the package.
// TODO: should this work properly if pkg has been autogrouped ?
func (obj *PkgRes) populateFileList() error {

	bus := packagekit.NewBus()
	if bus == nil {
		return fmt.Errorf("can't connect to PackageKit bus")
	}
	defer bus.Close()
	if obj.init != nil {
		bus.Debug = obj.init.Debug
		bus.Logf = func(format string, v ...interface{}) {
			obj.init.Logf("packagekit: "+format, v...)
		}
	}

	result, err := obj.pkgMappingHelper(bus)
	if err != nil {
		return errwrap.Wrapf(err, "the pkgMappingHelper failed")
	}

	data, ok := result[obj.Name()] // lookup single package (init does just one)
	// package doesn't exist, this is an error!
	if !ok || !data.Found {
		return fmt.Errorf("can't find package named '%s'", obj.Name())
	}
	if data.PackageID == "" {
		// this can happen if you specify a bad version like "latest"
		return fmt.Errorf("empty PackageID found for '%s'", obj.Name())
	}

	packageIDs := []string{data.PackageID} // just one for now
	filesMap, err := bus.GetFilesByPackageID(packageIDs)
	if err != nil {
		return errwrap.Wrapf(err, "can't run GetFilesByPackageID")
	}
	if files, ok := filesMap[data.PackageID]; ok {
		obj.fileList = util.DirifyFileList(files, false)
	}

	return nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *PkgRes) CheckApply(apply bool) (bool, error) {
	obj.init.Logf("Check: %s", obj.fmtNames(obj.getNames()))

	bus := packagekit.NewBus()
	if bus == nil {
		return false, fmt.Errorf("can't connect to PackageKit bus")
	}
	defer bus.Close()
	bus.Debug = obj.init.Debug
	bus.Logf = func(format string, v ...interface{}) {
		obj.init.Logf("packagekit: "+format, v...)
	}

	result, err := obj.pkgMappingHelper(bus)
	if err != nil {
		return false, errwrap.Wrapf(err, "the pkgMappingHelper failed")
	}

	packageMap := obj.groupMappingHelper() // map[string]string
	packageList := []string{obj.Name()}
	packageList = append(packageList, util.StrMapKeys(packageMap)...)
	//stateList := []string{obj.State}
	//stateList = append(stateList, util.StrMapValues(packageMap)...)

	// TODO: at the moment, all the states are the same, but
	// eventually we might be able to drop this constraint!
	states, err := packagekit.FilterState(result, packageList, obj.State)
	if err != nil {
		return false, errwrap.Wrapf(err, "the FilterState method failed")
	}
	data, _ := result[obj.Name()] // if above didn't error, we won't either!
	validState := util.BoolMapTrue(util.BoolMapValues(states))

	// obj.State == PkgStateInstalled || PkgStateUninstalled || PkgStateNewest || "4.2-1.fc23"
	switch obj.State {
	case PkgStateInstalled:
		fallthrough
	case PkgStateUninstalled:
		fallthrough
	case PkgStateNewest:
		if validState {
			return true, nil // state is correct, exit!
		}
	default: // version string
		if obj.State == data.Version && data.Version != "" {
			return true, nil
		}
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	obj.init.Logf("Apply: %s", obj.fmtNames(obj.getNames()))
	readyPackages, err := packagekit.FilterPackageState(result, packageList, obj.State)
	if err != nil {
		return false, err // fail
	}
	// these are the packages that actually need their states applied!
	applyPackages := util.StrFilterElementsInList(readyPackages, packageList)
	packageIDs, _ := packagekit.FilterPackageIDs(result, applyPackages) // would be same err as above

	var transactionFlags uint64 // initializes at the "zero" value of 0
	if !obj.AllowUntrusted {    // allow
		transactionFlags += packagekit.PkTransactionFlagEnumOnlyTrusted
	}
	// apply correct state!
	obj.init.Logf("Set(%s): %s...", obj.State, obj.fmtNames(util.StrListIntersection(applyPackages, obj.getNames())))
	switch obj.State {
	case PkgStateUninstalled: // run remove
		// NOTE: packageID is different than when installed, because now
		// it has the "installed" flag added to the data portion of it!!
		err = bus.RemovePackages(packageIDs, transactionFlags)

	case PkgStateNewest: // TODO: isn't this the same operation as install, below?
		err = bus.UpdatePackages(packageIDs, transactionFlags)

	case PkgStateInstalled:
		fallthrough // same method as for "set specific version", below
	default: // version string
		err = bus.InstallPackages(packageIDs, transactionFlags)
	}
	if err != nil {
		return false, err // fail
	}
	obj.init.Logf("Set(%s) success: %s", obj.State, obj.fmtNames(util.StrListIntersection(applyPackages, obj.getNames())))
	return false, nil // success
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *PkgRes) Cmp(r engine.Res) error {
	// we can only compare PkgRes to others of the same resource kind
	res, ok := r.(*PkgRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.State != res.State {
		return fmt.Errorf("state differs: %s vs %s", obj.State, res.State)
	}

	return obj.Adapts(res)
}

// Adapts compares two resources and returns an error if they are not able to be
// equivalently output compatible.
func (obj *PkgRes) Adapts(r engine.CompatibleRes) error {
	res, ok := r.(*PkgRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.State != res.State {
		e := fmt.Errorf("state differs in an incompatible way: %s vs %s", obj.State, res.State)
		if obj.State == PkgStateUninstalled || res.State == PkgStateUninstalled {
			return e
		}
		if stateIsVersion(obj.State) || stateIsVersion(res.State) {
			return e
		}
		// one must be installed, and the other must be "newest"
	}

	if obj.AllowUntrusted != res.AllowUntrusted {
		return fmt.Errorf("allowuntrusted differs: %t vs %t", obj.AllowUntrusted, res.AllowUntrusted)
	}
	if obj.AllowNonFree != res.AllowNonFree {
		return fmt.Errorf("allownonfree differs: %t vs %t", obj.AllowNonFree, res.AllowNonFree)
	}
	if obj.AllowUnsupported != res.AllowUnsupported {
		return fmt.Errorf("allowunsupported differs: %t vs %t", obj.AllowUnsupported, res.AllowUnsupported)
	}

	return nil
}

// Merge returns the best equivalent of the two resources. They must satisfy the
// Adapts test for this to work.
func (obj *PkgRes) Merge(r engine.CompatibleRes) (engine.CompatibleRes, error) {
	res, ok := r.(*PkgRes)
	if !ok {
		return nil, fmt.Errorf("res is not the same kind")
	}

	if err := obj.Adapts(r); err != nil {
		return nil, errwrap.Wrapf(err, "can't merge resources that aren't compatible")
	}

	// modify the copy, not the original
	x, err := engine.ResCopy(obj) // don't call our .Copy() directly!
	if err != nil {
		return nil, err
	}
	result, ok := x.(*PkgRes)
	if !ok {
		// bug!
		return nil, fmt.Errorf("res is not the same kind")
	}

	// if these two were compatible then if they're not identical, then one
	// must be PkgStateNewest and the other is PkgStateInstalled, so we
	// upgrade to the best common denominator
	if obj.State != res.State {
		result.State = PkgStateNewest
	}

	return result, nil
}

// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
// TODO: should this copy internal state?
func (obj *PkgRes) Copy() engine.CopyableRes {
	return &PkgRes{
		State:            obj.State,
		AllowUntrusted:   obj.AllowUntrusted,
		AllowNonFree:     obj.AllowNonFree,
		AllowUnsupported: obj.AllowUnsupported,
	}
}

// PkgUID is the main UID struct for PkgRes.
type PkgUID struct {
	engine.BaseUID
	name  string // pkg name
	state string // pkg state or "version"
}

// PkgFileUID is the UID struct for PkgRes files.
type PkgFileUID struct {
	engine.BaseUID
	path string // path of the file
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *PkgUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*PkgUID)
	if !ok {
		return false
	}
	// FIXME: match on obj.state vs. res.state ?
	return obj.name == res.name
}

// PkgResAutoEdges holds the state of the auto edge generator.
type PkgResAutoEdges struct {
	fileList   []string
	svcUIDs    []engine.ResUID
	testIsNext bool   // safety
	name       string // saved data from PkgRes obj
	kind       string
}

// Next returns the next automatic edge.
func (obj *PkgResAutoEdges) Next() []engine.ResUID {
	if obj.testIsNext {
		panic("expecting a call to Test()")
	}
	obj.testIsNext = true // set after all the errors paths are past

	// first return any matching svcUIDs
	if x := obj.svcUIDs; len(x) > 0 {
		return x
	}

	var result []engine.ResUID
	// return UID's for whatever is in obj.fileList
	for _, x := range obj.fileList {
		var reversed = false // cheat by passing a pointer
		result = append(result, &FileUID{
			BaseUID: engine.BaseUID{
				Name:     obj.name,
				Kind:     obj.kind,
				Reversed: &reversed,
			},
			path: x, // what matters
		}) // build list
	}
	return result
}

// Test gets results of the earlier Next() call, & returns if we should continue!
func (obj *PkgResAutoEdges) Test(input []bool) bool {
	if !obj.testIsNext {
		panic("expecting a call to Next()")
	}

	// ack the svcUID's...
	if x := obj.svcUIDs; len(x) > 0 {
		if y := len(x); y != len(input) {
			panic(fmt.Sprintf("expecting %d value(s)", y))
		}
		obj.svcUIDs = []engine.ResUID{} // empty
		obj.testIsNext = false
		return true
	}

	count := len(obj.fileList)
	if count != len(input) {
		panic(fmt.Sprintf("expecting %d value(s)", count))
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
		dir := util.Dirname(obj.fileList[i]) // dirname of /foo/ should be /
		dirs[i] = dir
		if input[i] {
			done = append(done, dir)
		}
	}
	nodupes := util.StrRemoveDuplicatesInList(dirs)                // remove duplicates
	nodones := util.StrFilterElementsInList(done, nodupes)         // filter out done
	noempty := util.StrFilterElementsInList([]string{""}, nodones) // remove the "" from /
	obj.fileList = util.RemoveCommonFilePrefixes(noempty)          // magic

	if len(obj.fileList) == 0 { // nothing more, don't continue
		return false
	}
	return true // continue, there are more files!
}

// AutoEdges produces an object which generates a minimal pkg file optimization
// sequence of edges.
func (obj *PkgRes) AutoEdges() (engine.AutoEdge, error) {
	// in contrast with the FileRes AutoEdges() function which contains
	// more of the mechanics, most of the AutoEdge mechanics for the PkgRes
	// are contained in the Test() method! This design is completely okay!

	if obj.fileList == nil {
		if err := obj.populateFileList(); err != nil {
			return nil, errwrap.Wrapf(err, "error populating file list for automatic edges")
		}
	}

	// add matches for any svc resources found in pkg definition!
	var svcUIDs []engine.ResUID
	for _, x := range ReturnSvcInFileList(obj.fileList) {
		var reversed = false
		svcUIDs = append(svcUIDs, &SvcUID{
			BaseUID: engine.BaseUID{
				Name:     obj.Name(),
				Kind:     obj.Kind(),
				Reversed: &reversed,
			},
			name: x, // the svc name itself in the SvcUID object!
		}) // build list
	}

	return &PkgResAutoEdges{
		fileList:   util.RemoveCommonFilePrefixes(obj.fileList), // clean start!
		svcUIDs:    svcUIDs,
		testIsNext: false,      // start with Next() call
		name:       obj.Name(), // save data for PkgResAutoEdges obj
		kind:       obj.Kind(),
	}, nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *PkgRes) UIDs() []engine.ResUID {
	x := &PkgUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
		state:   obj.State,
	}
	result := []engine.ResUID{x}

	for _, y := range obj.fileList {
		y := &PkgFileUID{
			BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
			path:    y,
		}
		result = append(result, y)
	}
	return result
}

// GroupCmp returns whether two resources can be grouped together or not.
// Can these two resources be merged, aka, does this resource support doing so?
// Will resource allow itself to be grouped _into_ this obj?
func (obj *PkgRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*PkgRes)
	if !ok {
		return fmt.Errorf("resource is not the same kind")
	}
	// TODO: what should we do about the empty string?
	if stateIsVersion(obj.State) || stateIsVersion(res.State) {
		// can't merge specific version checks atm
		return fmt.Errorf("resource uses a version string")
	}
	// FIXME: keep it simple for now, only merge same states
	if obj.State != res.State {
		return fmt.Errorf("resource is of a different state")
	}
	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *PkgRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes PkgRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*PkgRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to PkgRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = PkgRes(raw) // restore from indirection with type conversion!
	return nil
}

// ReturnSvcInFileList returns a list of svc names for matches like: `/usr/lib/systemd/system/*.service`.
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
		if s := strings.TrimSuffix(basename, ".service"); !util.StrInList(s, result) {
			result = append(result, s)
		}
	}
	return result
}

// stateIsVersion is a simple test to see if the state string is an existing
// well-known flag.
// TODO: what should we do about the empty string?
func stateIsVersion(state string) bool {
	return (state != PkgStateInstalled && state != PkgStateUninstalled && state != PkgStateNewest) // must be a ver. string
}
