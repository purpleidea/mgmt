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

package resources

import (
	"encoding/gob"
	"fmt"
	"log"
	"strings"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/recwatch"

	errwrap "github.com/pkg/errors"
	// FIXME: we vendor go/augeas because master requires augeas 1.6.0
	// and libaugeas-dev-1.6.0 is not yet available in a PPA.
	"honnef.co/go/augeas"
)

func init() {
	gob.Register(&AugeasRes{})
}

// AugeasRes is a resource that enables you to use the augeas resource.
// Currently only allows you to change simple files (e.g sshd_config).
type AugeasRes struct {
	BaseRes `yaml:",inline"`

	// File is the path to the file targetted by this resource.
	// If specified, mgmt will watch that file.
	File string `yaml:"file"`

	// Lens is the lens  used by this resource. If specified, mgmt
	// will lower the augeas overhead by only loeading that lens.
	Lens string `yaml:"lens"`

	// Sets is a list of changes that will be applied to the file, in the form of
	// ["path", "value"]. mgmt will run augeas.Get() before augeas.Set(), to
	// prevent changing the file when it is not needed.
	// XXX: this should be a []AugeasSet or something similar where an
	// AugeasSet is struct AugeasSet { "Path" string "Value" string (with the YAML annotations}
	Sets [][]string `yaml:"sets"`

	recWatcher *recwatch.RecWatcher // used to watch the changed files
}

// NewAugeasRes is a constructor for this resource. It also calls Init() for you.
func NewAugeasRes(name string) (*AugeasRes, error) {
	obj := &AugeasRes{
		BaseRes: BaseRes{
			Name: name,
		},
	}
	return obj, obj.Init()
}

// Default returns some sensible defaults for this resource.
func (obj *AugeasRes) Default() Res {
	return &AugeasRes{}
}

// Validate if the params passed in are valid data.
func (obj *AugeasRes) Validate() error {
	if !strings.HasPrefix(obj.File, "/") {
		return fmt.Errorf("File should start with a slash.")
	}
	if obj.Lens != "" && !strings.HasSuffix(obj.Lens, ".lns") {
		return fmt.Errorf("Lens should have a .lns suffix.")
	}
	if (obj.Lens == "") != (obj.File == "") {
		return fmt.Errorf("File and Lens must be specified together.")
	}
	return obj.BaseRes.Validate()
}

// Init initiates the resource.
func (obj *AugeasRes) Init() error {
	obj.BaseRes.kind = "Augeas"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
// Taken from the File resource.
// FIXME: DRY - This is taken from the file resource
func (obj *AugeasRes) Watch(processChan chan *event.Event) error {
	var err error
	obj.recWatcher, err = recwatch.NewRecWatcher(obj.File, false)
	if err != nil {
		return err
	}
	defer obj.recWatcher.Close()

	// notify engine that we're running
	if err := obj.Running(processChan); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error

	for {
		if obj.debug {
			log.Printf("%s[%s]: Watching: %s", obj.Kind(), obj.GetName(), obj.File) // attempting to watch...
		}

		select {
		case event, ok := <-obj.recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s[%s] watcher error", obj.Kind(), obj.GetName())
			}
			if obj.debug { // don't access event.Body if event.Error isn't nil
				log.Printf("%s[%s]: Event(%s): %v", obj.Kind(), obj.GetName(), event.Body.Name, event.Body.Op)
			}
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
			//obj.StateOK(false) // dirty // these events don't invalidate state
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event(processChan)
		}
	}
}

// checkApplySet runs CheckApply for one element of the AugeasRes.Set
func (obj *AugeasRes) checkApplySet(apply bool, ag *augeas.Augeas, set []string) (checkOK bool, err error) {
	checkOK = true
	path, value := set[0], set[1]
	fullpath := fmt.Sprintf("/files/%v/%v", obj.File, path)

	if getValue, err := ag.Get(fullpath); err != nil || value != getValue {
		// note: augeas.Get throws an error if the path does not exist.
		// Thus we need to change the value if there is an error or if
		// the values do not match.
		if !apply {
			// If noop, we can return here directly. We return with
			// nil even if err is not nil because it does not mean
			// there is an error.
			return false, nil
		}
		checkOK = false
	}

	// XXX wat? -- reverse this so that we return early if this is the case...
	// XXX: eg if ag.Get ... == nil {
	//	return true, nil // be explicit, the named return values aren't as clear.
	//}
	if checkOK {
		return
	}

	if err = ag.Set(fullpath, value); err != nil {
		return false, errwrap.Wrapf(err, "augeas: error while setting value")
	}

	return false, nil
}

// CheckApply method for Augeas resource.
func (obj *AugeasRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s[%s]: CheckApply: %s", obj.Kind(), obj.GetName(), obj.File)
	// By default we do not set any option to augeas, we use the defaults.
	opts := augeas.None
	if obj.Lens != "" {
		// if the lens is specified, we can speed up augeas by not
		// loading everything. Without this option, augeas will try to
		// read all the files it knows in the complete filesystem.
		// e.g. to change /etc/ssh/sshd_config, it would load /etc/hosts, /etc/ntpd.conf, etc...
		opts = augeas.NoModlAutoload
	}

	// Initiate augeas
	ag, err := augeas.New("/", "", opts)
	if err != nil {
		return false, errwrap.Wrapf(err, "augeas: error while initializing")
	}
	defer ag.Close()

	checkOK = true

	if obj.Lens != "" {
		// If the lens is set, load the lens for the file we want to edit.
		// We pick Xmgmt, as this name will not collide with any other lens name.
		// We do not pick Mgmt as in the future there might be an Mgmt lens.
		// https://github.com/hercules-team/augeas/wiki/Loading-specific-files
		if err = ag.Set("/augeas/load/Xmgmt/lens", obj.Lens); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error while initializing lens")
		}
		if err = ag.Set("/augeas/load/Xmgmt/incl", obj.File); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error while initializing incl")
		}
		if err = ag.Load(); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error while loading")
		}
	}

	for _, set := range obj.Sets {
		if setCheckOK, err := obj.checkApplySet(apply, &ag, set); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error during CheckApply of one Set")
		} else if !setCheckOK {
			checkOK = false
		}
	}

	if checkOK || !apply {
		return
	}

	log.Printf("%s[%s]: changes needed, saving", obj.Kind(), obj.GetName())
	if err = ag.Save(); err != nil {
		return false, errwrap.Wrapf(err, "augeas: error while saving augeas values")
	}

	// XXX: recurse? not a good idea. Instead test for file existence manually.
	// FIXME: Workaround for https://github.com/dominikh/go-augeas/issues/13
	// To be fixed upstream.
	var newCheckOK bool
	newCheckOK, err = obj.CheckApply(false)
	if err != nil {
		return false, errwrap.Wrapf(err, "augeas: error while saving augeas values")
	}
	if !newCheckOK {
		return false, errwrap.New("augeas: new values were not set correctly")
	}

	return
}

// AugeasUID is the UID struct for AugeasRes.
type AugeasUID struct {
	BaseUID
	name string
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *AugeasRes) AutoEdges() AutoEdge {
	return nil
}

// UIDs includes all params to make a unique identification of this object.
func (obj *AugeasRes) UIDs() []ResUID {
	x := &AugeasUID{
		BaseUID: BaseUID{name: obj.GetName(), kind: obj.Kind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *AugeasRes) GroupCmp(r Res) bool {
	return false // Augeas commands can not be grouped together.
}

// Compare two resources and return if they are equivalent.
func (obj *AugeasRes) Compare(res Res) bool {
	switch res.(type) {
	// we can only compare AugeasRes to others of the same resource
	case *AugeasRes:
		res := res.(*AugeasRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}
		if obj.Name != res.Name {
			return false
		}
	default:
		return false
	}
	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *AugeasRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes AugeasRes // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*AugeasRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to AugeasRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = AugeasRes(raw) // restore from indirection with type conversion!
	return nil
}
