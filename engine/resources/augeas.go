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

// +build !noaugeas

package resources

import (
	"fmt"
	"os"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util/errwrap"

	// FIXME: we vendor go/augeas because master requires augeas 1.6.0
	// and libaugeas-dev-1.6.0 is not yet available in a PPA.
	"honnef.co/go/augeas"
)

const (
	// NS is a namespace for augeas operations
	NS = "Xmgmt"
)

func init() {
	engine.RegisterResource("augeas", func() engine.Res { return &AugeasRes{} })
}

// AugeasRes is a resource that enables you to use the augeas resource.
// Currently only allows you to change simple files (e.g sshd_config).
type AugeasRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// File is the path to the file targeted by this resource.
	File string `yaml:"file"`

	// Lens is the lens used by this resource. If specified, mgmt
	// will lower the augeas overhead by only loading that lens.
	Lens string `yaml:"lens"`

	// Sets is a list of changes that will be applied to the file, in the form of
	// ["path", "value"]. mgmt will run augeas.Get() before augeas.Set(), to
	// prevent changing the file when it is not needed.
	Sets []*AugeasSet `yaml:"sets"`

	recWatcher *recwatch.RecWatcher // used to watch the changed files
}

// AugeasSet represents a key/value pair of settings to be applied.
type AugeasSet struct {
	Path  string `yaml:"path"`  // The relative path to the value to be changed.
	Value string `yaml:"value"` // The value to be set on the given Path.
}

// Cmp compares this set with another one.
func (obj *AugeasSet) Cmp(set *AugeasSet) error {
	if obj == nil && set == nil {
		return nil
	}
	if obj == nil && set != nil {
		return fmt.Errorf("can't compare nil set to set")
	}
	if obj != nil && set == nil {
		return fmt.Errorf("can't compare set to nil set")
	}

	if obj.Path != set.Path {
		return fmt.Errorf("the Path values differ")
	}
	if obj.Value != set.Value {
		return fmt.Errorf("the Value values differ")
	}

	return nil
}

// Default returns some sensible defaults for this resource.
func (obj *AugeasRes) Default() engine.Res {
	return &AugeasRes{}
}

// Validate if the params passed in are valid data.
func (obj *AugeasRes) Validate() error {
	if !strings.HasPrefix(obj.File, "/") {
		return fmt.Errorf("the File param should start with a slash")
	}
	if obj.Lens != "" && !strings.HasSuffix(obj.Lens, ".lns") {
		return fmt.Errorf("the Lens param should have a .lns suffix")
	}
	if (obj.Lens == "") != (obj.File == "") {
		return fmt.Errorf("the File and Lens params must be specified together")
	}
	return nil
}

// Init initializes the resource.
func (obj *AugeasRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *AugeasRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// was taken from the File resource.
// FIXME: DRY - This is taken from the file resource
func (obj *AugeasRes) Watch() error {
	var err error
	obj.recWatcher, err = recwatch.NewRecWatcher(obj.File, false)
	if err != nil {
		return err
	}
	defer obj.recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("Watching: %s", obj.File) // attempting to watch...
		}

		select {
		case event, ok := <-obj.recWatcher.Events():
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s watcher error", obj)
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("Event(%s): %v", event.Body.Name, event.Body.Op)
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

// checkApplySet runs CheckApply for one element of the AugeasRes.Set
func (obj *AugeasRes) checkApplySet(apply bool, ag *augeas.Augeas, set *AugeasSet) (bool, error) {
	fullpath := fmt.Sprintf("/files/%v/%v", obj.File, set.Path)

	// We do not check for errors because errors are also thrown when
	// the path does not exist.
	if getValue, _ := ag.Get(fullpath); set.Value == getValue {
		// The value is what we expect, return directly
		return true, nil
	}

	if !apply {
		// If noop, we can return here directly. We return with
		// nil even if err is not nil because it does not mean
		// there is an error.
		return false, nil
	}

	if err := ag.Set(fullpath, set.Value); err != nil {
		return false, errwrap.Wrapf(err, "augeas: error while setting value")
	}

	return false, nil
}

// CheckApply method for Augeas resource.
func (obj *AugeasRes) CheckApply(apply bool) (bool, error) {
	obj.init.Logf("CheckApply: %s", obj.File)
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

	if obj.Lens != "" {
		// If the lens is set, load the lens for the file we want to edit.
		// We pick Xmgmt, as this name will not collide with any other lens name.
		// We do not pick Mgmt as in the future there might be an Mgmt lens.
		// https://github.com/hercules-team/augeas/wiki/Loading-specific-files
		if err = ag.Set(fmt.Sprintf("/augeas/load/%s/lens", NS), obj.Lens); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error while initializing lens")
		}
		if err = ag.Set(fmt.Sprintf("/augeas/load/%s/incl", NS), obj.File); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error while initializing incl")
		}
		if err = ag.Load(); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error while loading")
		}
	}

	checkOK := true
	for _, set := range obj.Sets {
		if setCheckOK, err := obj.checkApplySet(apply, &ag, set); err != nil {
			return false, errwrap.Wrapf(err, "augeas: error during CheckApply of one Set")
		} else if !setCheckOK {
			checkOK = false
		}
	}

	// If the state is correct or we can't apply, return early.
	if checkOK || !apply {
		return checkOK, nil
	}

	obj.init.Logf("changes needed, saving")
	if err = ag.Save(); err != nil {
		return false, errwrap.Wrapf(err, "augeas: error while saving augeas values")
	}

	// FIXME: Workaround for https://github.com/dominikh/go-augeas/issues/13
	// To be fixed upstream.
	if obj.File != "" {
		if _, err := os.Stat(obj.File); os.IsNotExist(err) {
			return false, errwrap.Wrapf(err, "augeas: error: file does not exist")
		}
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *AugeasRes) Cmp(r engine.Res) error {
	// we can only compare to others of the same resource kind
	res, ok := r.(*AugeasRes)
	if !ok {
		return fmt.Errorf("resource is not the same kind")
	}

	if obj.File != res.File {
		return fmt.Errorf("the File params differ")
	}
	if obj.Lens != res.Lens {
		return fmt.Errorf("the Lens params differ")
	}

	if len(obj.Sets) != len(res.Sets) {
		return fmt.Errorf("the length of the two Sets params differs")
	}
	for i := 0; i < len(obj.Sets); i++ {
		if err := obj.Sets[i].Cmp(res.Sets[i]); err != nil {
			return errwrap.Wrapf(err, "the Sets item at index %d differs", i)
		}
	}

	return nil
}

// AugeasUID is the UID struct for AugeasRes.
type AugeasUID struct {
	engine.BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object.
func (obj *AugeasRes) UIDs() []engine.ResUID {
	x := &AugeasUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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
