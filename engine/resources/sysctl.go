// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package resources

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("sysctl", func() engine.Res { return &SysctlRes{} })

	if !strings.HasPrefix(SysctlConfDir, "/") {
		panic("the SysctlConfDir does not start with a slash")
	}
	if !strings.HasSuffix(SysctlConfDir, "/") {
		panic("the SysctlConfDir does not end with a slash")
	}
}

const (
	// SysctlConfDir is the directory to store persistent sysctl files in.
	SysctlConfDir = "/etc/sysctl.d/"

	// SysctlConfPrefix is the prefix we prepend to any automatically chosen
	// filename that we put in the /etc/sysctl.d/ directory.
	// TODO: What prefix should we use if any?
	SysctlConfPrefix = "99-"
)

// SysctlRes is a resource for setting kernel parameters.
// TODO: Add a sysctl:clean resource that removes any unmanaged files from
// /etc/sysctl.d/ and optionally blanks out the stock /etc/sysctl.conf file too.
type SysctlRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Value is the string value to set. Make sure you specify it in the
	// same format that the kernel parses it as to avoid automation
	// "flapping". You can test this by writing a value to the correct
	// /proc/sys/ path entry with `echo foo >` and then reading it back out
	// and seeing what the "parsed" correct format is. You must not include
	// the trailing newline which is present in the readback for all values.
	Value string `lang:"value" yaml:"value"`

	// Runtime specifies whether this value should be set immediately. It
	// defaults to true. If this is not set, then the value must be set in a
	// file and the machine will have to reboot for the setting to take
	// effect.
	Runtime bool `lang:"runtime" yaml:"runtime"`

	// Persist specifies whether this value should be stored on disk where
	// it will persist across reboots. It defaults to true. Keep in mind,
	// that if this is not used, but `Runtime` is true, then the value will
	// be restored anyways if `mgmt` runs on boot, which may be what you
	// want anyways.
	Persist bool `lang:"persist" yaml:"persist"`

	// Filename is the full path for the persistence file which is usually
	// read on boot. We usually use entries in the /etc/sysctl.d/ directory.
	// By convention, they end in .conf and start with a numeric prefix and
	// a dash. For example: /etc/sysctl.d/10-dmesg.conf for example. If this
	// is omitted, the filename will be chosen automatically.
	Filename string `lang:"path" yaml:"path"`
}

// toPath converts our name into the magic kernel path. It does not validate
// that the name is in a valid format.
func (obj *SysctlRes) toPath() string {
	return path.Join("/proc/sys/", strings.ReplaceAll(obj.Name(), ".", "/"))
}

// getFilename returns the filename of the config that we would use if we're
// setting one. This does not look at the is persistent aspect.
func (obj *SysctlRes) getFilename() string {
	if obj.Filename != "" {
		return obj.Filename
	}
	return SysctlConfDir + SysctlConfPrefix + obj.Name() + ".conf"
}

// Default returns some sensible defaults for this resource.
func (obj *SysctlRes) Default() engine.Res {
	return &SysctlRes{
		Runtime: true,
		Persist: true,
	}
}

// Validate reports any problems with the struct definition.
func (obj *SysctlRes) Validate() error {
	if strings.Contains(obj.Name(), "/") {
		// We do this to avoid having two resources which "fight" with
		// each other by using the alternative representation.
		return fmt.Errorf("name contains slashes, use the dotted representation")
	}
	if strings.Contains(obj.Name(), "=") {
		return fmt.Errorf("name contains equals sign, this is illegal")
	}
	if strings.HasPrefix(obj.Name(), ".") || strings.HasSuffix(obj.Name(), ".") {
		return fmt.Errorf("name contains leading or trailing periods")
	}
	if obj.Name() != strings.TrimSpace(obj.Name()) {
		return fmt.Errorf("name has leading or trailing whitespace")
	}

	if obj.Value == "" {
		return fmt.Errorf("value is empty")
	}
	if strings.TrimSpace(obj.Value) != obj.Value {
		return fmt.Errorf("value contains leading or trailing whitespace")
	}

	// TODO: We could probably relax this check I suppose.
	if !obj.Runtime && !obj.Persist {
		return fmt.Errorf("you must either set the value at runtime or you must persist")
	}

	// Parse the Name() and see if it's a valid path under /proc/sys/ dir.
	if _, err := os.Stat(obj.toPath()); err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return errwrap.Wrapf(err, "unknown stat error")

	} else if err != nil {
		// TODO: Could there be a kernel that doesn't show this path?
		return fmt.Errorf("name is not a valid kernel path: %s", obj.toPath())
	}

	if obj.Persist && !strings.HasSuffix(obj.getFilename(), ".conf") {
		return fmt.Errorf("filename must end with .conf")
	}
	if obj.Persist && !strings.HasPrefix(obj.getFilename(), "/") {
		return fmt.Errorf("filename must be absolute and start with slash")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *SysctlRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *SysctlRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// one watches the on disk filename if it creates one, as well as the runtime
// value the kernel has stored!
func (obj *SysctlRes) Watch(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	recurse := false // single file

	var events1, events2 chan recwatch.Event

	if obj.Runtime {
		recWatcher, err := recwatch.NewRecWatcher(obj.toPath(), recurse)
		if err != nil {
			return err
		}
		defer recWatcher.Close()
		events1 = recWatcher.Events()
	}

	if obj.Persist {
		recWatcher, err := recwatch.NewRecWatcher(obj.getFilename(), recurse)
		if err != nil {
			return err
		}
		defer recWatcher.Close()
		events2 = recWatcher.Events()
	}

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		select {
		case event, ok := <-events1:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return err
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}
			send = true

		case event, ok := <-events2:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return err
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}
			send = true

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *SysctlRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	checkOK := true

	// TODO: If there any reason to do one of these before the other? At
	// least right now, if the runtime change causes a kernel panic, the
	// machine will have a better chance of coming back online without the
	// persisted value being stored.

	if c, err := obj.runtimeCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	if c, err := obj.persistCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil // w00t
}

// runtimeCheckApply checks the runtime value in the kernel, and modifies it if
// needed.
func (obj *SysctlRes) runtimeCheckApply(ctx context.Context, apply bool) (bool, error) {
	if !obj.Runtime {
		return true, nil
	}

	// Clean off any whitespace it comes with and then always add a newline.
	expected := []byte(obj.Value + "\n")

	b, err := os.ReadFile(obj.toPath())
	if err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return false, nil
	}
	if err == nil && bytes.Equal(expected, b) {
		return true, nil // we match!
	}

	// Down here, file does not exist or does not match...

	if !apply {
		return false, nil
	}

	if err := os.WriteFile(obj.toPath(), expected, 0644); err != nil {
		return false, err
	}

	obj.init.Logf("runtime `%s` to: %s\n", obj.Value, obj.toPath())

	return false, err
}

// persistCheckApply checks the on-disk value for the kernel, and modifies it if
// needed.
func (obj *SysctlRes) persistCheckApply(ctx context.Context, apply bool) (bool, error) {
	if !obj.Persist {
		return true, nil
	}

	// Clean off any whitespace and put it in the standard format.
	// TODO: Should we add a "last managed by mgmt on $date" line ?
	s := fmt.Sprintf("%s = %s\n", obj.Name(), obj.Value)
	expected := []byte(s)

	b, err := os.ReadFile(obj.getFilename())
	if err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return false, nil
	}
	if err == nil && bytes.Equal(expected, b) {
		return true, nil // we match!
	}

	// Down here, file does not exist or does not match...

	if !apply {
		return false, nil
	}

	if err := os.WriteFile(obj.getFilename(), expected, 0644); err != nil {
		return false, err
	}

	obj.init.Logf("persist `%s` to: %s\n", obj.Value, obj.getFilename())

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *SysctlRes) Cmp(r engine.Res) error {
	// we can only compare SysctlRes to others of the same resource kind
	res, ok := r.(*SysctlRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Value != res.Value {
		return fmt.Errorf("the Value differs")
	}

	if obj.Runtime != res.Runtime {
		return fmt.Errorf("the Runtime value differs")
	}
	if obj.Persist != res.Persist {
		return fmt.Errorf("the Persist value differs")
	}

	// TODO: We could compare the actual resultant Filename if we're using
	// it, even if it comes from different representations, eg: specified vs
	// chosen automatically. If they don't differ, it's fine with us!
	if obj.Filename != res.Filename {
		return fmt.Errorf("the contents of Filename differ")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *SysctlRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes SysctlRes // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*SysctlRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to SysctlRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = SysctlRes(raw) // restore from indirection with type conversion!
	return nil
}
