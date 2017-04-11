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

package resources

import (
	"encoding/gob"
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/resources/packagekit"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("pkg", func() Res { return &PkgRes{} })
	gob.Register(&PkgRes{})
}

// PkgRes is a package resource for packagekit.
type PkgRes struct {
	BaseRes          `yaml:",inline"`
	State            string `yaml:"state"`            // state: installed, uninstalled, newest, <version>
	AllowUntrusted   bool   `yaml:"allowuntrusted"`   // allow untrusted packages to be installed?
	AllowNonFree     bool   `yaml:"allownonfree"`     // allow nonfree packages to be found?
	AllowUnsupported bool   `yaml:"allowunsupported"` // allow unsupported packages to be found?
	//bus              *packagekit.Conn    // pk bus connection
	fileList []string // FIXME: update if pkg changes
}

// Default returns some sensible defaults for this resource.
func (obj *PkgRes) Default() Res {
	return &PkgRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
		State: "installed", // i think this is preferable to "latest"
	}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *PkgRes) Validate() error {
	if obj.State == "" {
		return fmt.Errorf("state cannot be empty")
	}

	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *PkgRes) Init() error {
	obj.BaseRes.Kind = "pkg"
	if err := obj.BaseRes.Init(); err != nil { // call base init, b/c we're overriding
		return err
	}

	bus := packagekit.NewBus()
	if bus == nil {
		return fmt.Errorf("can't connect to PackageKit bus")
	}
	defer bus.Close()

	result, err := obj.pkgMappingHelper(bus)
	if err != nil {
		return errwrap.Wrapf(err, "the pkgMappingHelper failed")
	}

	data, ok := result[obj.Name] // lookup single package (init does just one)
	// package doesn't exist, this is an error!
	if !ok || !data.Found {
		return fmt.Errorf("can't find package named '%s'", obj.Name)
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

	ch, err := bus.WatchChanges()
	if err != nil {
		return errwrap.Wrapf(err, "error adding signal match")
	}

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error

	for {
		if obj.debug {
			log.Printf("%s: Watching...", obj.fmtNames(obj.getNames()))
		}

		select {
		case event := <-ch:
			// FIXME: ask packagekit for info on what packages changed
			if obj.debug {
				log.Printf("%s: Event: %v", obj.fmtNames(obj.getNames()), event.Name)
			}

			// since the chan is buffered, remove any supplemental
			// events since they would just be duplicates anyways!
			for len(ch) > 0 { // we can detect pending count here!
				<-ch // discard
			}

			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
			//obj.StateOK(false) // these events don't invalidate state

		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
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
		return fmt.Sprintf("%s[autogroup:(%v)]", obj.GetKind(), strings.Join(names, ","))
	}
	return fmt.Sprintf("%s[%s]", obj.GetKind(), obj.GetName())
}

func (obj *PkgRes) groupMappingHelper() map[string]string {
	var result = make(map[string]string)
	if g := obj.GetGroup(); len(g) > 0 { // add any grouped elements
		for _, x := range g {
			pkg, ok := x.(*PkgRes) // convert from Res
			if !ok {
				log.Fatalf("grouped member %v is not a %s", x, obj.GetKind())
			}
			result[pkg.Name] = pkg.State
		}
	}
	return result
}

func (obj *PkgRes) pkgMappingHelper(bus *packagekit.Conn) (map[string]*packagekit.PkPackageIDActionData, error) {
	packageMap := obj.groupMappingHelper()   // get the grouped values
	packageMap[obj.Name] = obj.State         // key is pkg name, value is pkg state
	var filter uint64                        // initializes at the "zero" value of 0
	filter += packagekit.PK_FILTER_ENUM_ARCH // always search in our arch (optional!)
	// we're requesting latest version, or to narrow down install choices!
	if obj.State == "newest" || obj.State == "installed" {
		// if we add this, we'll still see older packages if installed
		// this is an optimization, and is *optional*, this logic is
		// handled inside of PackagesToPackageIDs now automatically!
		filter += packagekit.PK_FILTER_ENUM_NEWEST // only search for newest packages
	}
	if !obj.AllowNonFree {
		filter += packagekit.PK_FILTER_ENUM_FREE
	}
	if !obj.AllowUnsupported {
		filter += packagekit.PK_FILTER_ENUM_SUPPORTED
	}
	result, err := bus.PackagesToPackageIDs(packageMap, filter)
	if err != nil {
		return nil, errwrap.Wrapf(err, "Can't run PackagesToPackageIDs")
	}
	return result, nil
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *PkgRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: Check", obj.fmtNames(obj.getNames()))

	bus := packagekit.NewBus()
	if bus == nil {
		return false, fmt.Errorf("can't connect to PackageKit bus")
	}
	defer bus.Close()

	result, err := obj.pkgMappingHelper(bus)
	if err != nil {
		return false, errwrap.Wrapf(err, "the pkgMappingHelper failed")
	}

	packageMap := obj.groupMappingHelper() // map[string]string
	packageList := []string{obj.Name}
	packageList = append(packageList, util.StrMapKeys(packageMap)...)
	//stateList := []string{obj.State}
	//stateList = append(stateList, util.StrMapValues(packageMap)...)

	// TODO: at the moment, all the states are the same, but
	// eventually we might be able to drop this constraint!
	states, err := packagekit.FilterState(result, packageList, obj.State)
	if err != nil {
		return false, errwrap.Wrapf(err, "the FilterState method failed")
	}
	data, _ := result[obj.Name] // if above didn't error, we won't either!
	validState := util.BoolMapTrue(util.BoolMapValues(states))

	// obj.State == "installed" || "uninstalled" || "newest" || "4.2-1.fc23"
	switch obj.State {
	case "installed":
		fallthrough
	case "uninstalled":
		fallthrough
	case "newest":
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
	log.Printf("%s: Apply", obj.fmtNames(obj.getNames()))
	readyPackages, err := packagekit.FilterPackageState(result, packageList, obj.State)
	if err != nil {
		return false, err // fail
	}
	// these are the packages that actually need their states applied!
	applyPackages := util.StrFilterElementsInList(readyPackages, packageList)
	packageIDs, _ := packagekit.FilterPackageIDs(result, applyPackages) // would be same err as above

	var transactionFlags uint64 // initializes at the "zero" value of 0
	if !obj.AllowUntrusted {    // allow
		transactionFlags += packagekit.PK_TRANSACTION_FLAG_ENUM_ONLY_TRUSTED
	}
	// apply correct state!
	log.Printf("%s: Set: %v...", obj.fmtNames(util.StrListIntersection(applyPackages, obj.getNames())), obj.State)
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
	log.Printf("%s: Set: %v success!", obj.fmtNames(util.StrListIntersection(applyPackages, obj.getNames())), obj.State)
	return false, nil // success
}

// PkgUID is the UID struct for PkgRes.
type PkgUID struct {
	BaseUID
	name  string // pkg name
	state string // pkg state or "version"
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *PkgUID) IFF(uid ResUID) bool {
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
	svcUIDs    []ResUID
	testIsNext bool   // safety
	name       string // saved data from PkgRes obj
	kind       string
}

// Next returns the next automatic edge.
func (obj *PkgResAutoEdges) Next() []ResUID {
	if obj.testIsNext {
		log.Fatal("expecting a call to Test()")
	}
	obj.testIsNext = true // set after all the errors paths are past

	// first return any matching svcUIDs
	if x := obj.svcUIDs; len(x) > 0 {
		return x
	}

	var result []ResUID
	// return UID's for whatever is in obj.fileList
	for _, x := range obj.fileList {
		var reversed = false // cheat by passing a pointer
		result = append(result, &FileUID{
			BaseUID: BaseUID{
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
		log.Fatal("expecting a call to Next()")
	}

	// ack the svcUID's...
	if x := obj.svcUIDs; len(x) > 0 {
		if y := len(x); y != len(input) {
			log.Fatalf("expecting %d value(s)", y)
		}
		obj.svcUIDs = []ResUID{} // empty
		obj.testIsNext = false
		return true
	}

	count := len(obj.fileList)
	if count != len(input) {
		log.Fatalf("expecting %d value(s)", count)
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
func (obj *PkgRes) AutoEdges() AutoEdge {
	// in contrast with the FileRes AutoEdges() function which contains
	// more of the mechanics, most of the AutoEdge mechanics for the PkgRes
	// is contained in the Test() method! This design is completely okay!

	// add matches for any svc resources found in pkg definition!
	var svcUIDs []ResUID
	for _, x := range ReturnSvcInFileList(obj.fileList) {
		var reversed = false
		svcUIDs = append(svcUIDs, &SvcUID{
			BaseUID: BaseUID{
				Name:     obj.GetName(),
				Kind:     obj.GetKind(),
				Reversed: &reversed,
			},
			name: x, // the svc name itself in the SvcUID object!
		}) // build list
	}

	return &PkgResAutoEdges{
		fileList:   util.RemoveCommonFilePrefixes(obj.fileList), // clean start!
		svcUIDs:    svcUIDs,
		testIsNext: false,         // start with Next() call
		name:       obj.GetName(), // save data for PkgResAutoEdges obj
		kind:       obj.GetKind(),
	}
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *PkgRes) UIDs() []ResUID {
	x := &PkgUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
		state:   obj.State,
	}
	result := []ResUID{x}
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
