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
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/data"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("fwattr", func() engine.Res { return &FWAttrRes{} })

	if !strings.HasPrefix(FWAttrDir, "/") {
		panic("the FWAttrConfDir does not start with a slash")
	}
	if !strings.HasSuffix(FWAttrDir, "/") {
		panic("the FWAttrConfDir does not end with a slash")
	}
}

const (
	// FWAttrDir is the directory to read/write firmware attributes to/from.
	FWAttrDir = "/sys/class/firmware-attributes/"

	// fwattrSkip is a sentinel value that we use in a few places.
	fwattrSkip = "_skip"
)

// FWAttrRes is a resource for interacting with the kernel firmware attributes
// API. This resource will automatically use the correct "value" for a key when
// more than one is possible for that particular vendor. If you have a mapping
// that is not in our database, please send a patch.
//
// Please note that on some platforms such as Lenovo (thinklmi), there is an
// architectural limitation that prevents more than 48 attributes from being set
// before presumably needing to reboot.
//
// https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-class-firmware-attributes
//
// XXX: We could count the number of changes, and error before we hit 48+?
// Apparently if we go over, you need to manually enter the BIOS to clear the
// error. Of course we can never know if the user edited these elsewhere.
type FWAttrRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Driver is the interface that is supported. Available options can be
	// found in the /sys/class/firmware-attributes/ directory. Common values
	// include "thinklmi" (lenovo), "dell-wmi-sysman" (dell) and more. If
	// you do not specify this, then we will attempt to determine it
	// automatically, however if zero or more than one option exists, then
	// this will error.
	Driver string `lang:"driver" yaml:"driver"`

	// Key is the name of the field to modify. If this is not set we use the
	// Name field. This is case sensitive.
	Key string `lang:"key" yaml:"key"`

	// Value is the string value to set. Make sure you specify it in the
	// same format that the kernel offers it as to avoid automation
	// "flapping" or errors. You can test this by writing a value to the
	// /sys/class/firmware-attributes/<driver>/<key>/current_value path with
	// `echo foo >` and seeing if it works without erroring. You must not
	// include the trailing newline which is present all values.
	//
	// TODO: When resources eventually support proper type unification, let
	// this also be an int or a bool, and for booleans, map them using our
	// json file to the correct string value of "Enabled" (for those keys
	// of the boolean variety).
	Value string `lang:"value" yaml:"value"`

	// Check (which defaults to true) turns off the validation that runs
	// before we attempt to change a setting. This should only be used in
	// rare exceptions where you have an old/buggy version of firmware that
	// has illogical data presented through the kernel API. For example, one
	// version of BootOrder on thinklmi had type "enumeration" and
	// "possible_values" of: `HDD0;HDD1;HDD2;...` but took a "current_value"
	// of `HDD0:HDD1:HDD2:...` which would be illogical. Newer versions just
	// have type "string".
	Check bool `lang:"check" yaml:"check"`

	// Strict means that we won't use the "quirks" alternate values mapping
	// if the exact key is not available. It should not be harmful to keep
	// this off, but it would be useful to find which old values are being
	// used long after legacy firmware has been deprecated.
	Strict bool `lang:"strict" yaml:"strict"`

	// Skip let's you turn this resource into a "noop" if the key doesn't
	// exist. This should ideally not be used because a typo would
	// effectively make this resource ineffective. As a result, if you use
	// this, it will emit a warning. This option is very useful, because you
	// can add a more general "configuration set" of values to all of your
	// machine, without having to match them precisely, and this won't cause
	// errors if one of them has an old version of a BIOS without that
	// feature. This will also skip if the attributes API doesn't exist on
	// this machine.
	Skip bool `lang:"skip" yaml:"skip"`

	skip   bool    // are we actually skipping this particular name?
	driver *string // cache
	value  *string // actual value to use
}

// getDriver gets the driver that we should use.
// TODO: add a mutex if this gets called concurrently!
func (obj *FWAttrRes) getDriver() (string, error) {
	if obj.driver != nil { // cache
		return *obj.driver, nil
	}

	files, err := os.ReadDir(FWAttrDir)
	if err != nil && os.IsNotExist(err) && obj.Skip { // TODO: could be a different skip
		return fwattrSkip, nil
	} else if err != nil {
		return "", err
	}

	name := ""
	for _, x := range files {
		n := x.Name() // no slashes
		if n == obj.Driver {
			obj.driver = &n        // cache
			return obj.Driver, nil // found
		}
		name = n // store
	}

	if len(files) == 1 {
		obj.driver = &name // cache
		return name, nil
	}

	return "", fmt.Errorf("could not pick driver, got: %d", len(files))
}

// getKey is a helper function to return the key we're using.
func (obj *FWAttrRes) getKey() string {
	if obj.Key != "" {
		return obj.Key
	}

	// TODO: consider parsing name in some sort of driver:key style instead?
	return obj.Name()
}

// toPath converts our key into the magic kernel path. It does not validate that
// the key is in a valid format. This must not be called before getDriver was at
// least called once, because we depend on the cached value and that the error
// path was used elsewhere first. This points to the "current_value" file.
func (obj *FWAttrRes) toPath() string {
	driver, err := obj.getDriver()
	if err != nil {
		// programming error
		panic("driver lookup error")
	}

	return path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "current_value")
}

// getType reads the type of the value. Known types are:
//
// * enumeration: a set of pre-defined valid values
// * integer: a range of numerical values
// * string: what you expect
//
// on hp systems you can also have:
//
// * ordered-list: a set of ordered list valid values
//
// This does not validate if the string is sensible or not.
func (obj *FWAttrRes) getType() (string, error) {
	driver, err := obj.getDriver()
	if err != nil {
		// programming error
		panic("driver lookup error")
	}

	p := path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "type")

	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return "", err

	} else if err != nil {
		if obj.Skip {
			return fwattrSkip, nil
		}

		// the path should already exist (this driver is broken?)
		return "", errwrap.Wrapf(err, "unexpected error with: %s", p)
	}

	return strings.TrimSuffix(string(b), "\n"), nil
}

// isValidValue determines if a value is a valid possibility for this key. We do
// this check early to catch errors at the Validate step, rather than during
// CheckApply which is well into deeper runtime.
func (obj *FWAttrRes) isValidValue() (bool, error) {
	driver, err := obj.getDriver()
	if err != nil {
		// programming error
		panic("driver lookup error")
	}

	if !obj.Check {
		return true, nil // exceptionally, we skip validation!
	}

	typ, err := obj.getType()
	if err != nil {
		return false, err
	}

	switch typ {
	case "enumeration":
		// For thinklmi BootOrder/possible_values I've seen (excerpt):
		// HDD0;HDD1;PXEBOOT;USBHDD;OtherHDD;NVMe0;LENOVOCLOUD;NODEV
		// However BootOrder/current_value contains these colon join as:
		// USBHDD:USBCD:USBFDD:NVMe0 which means our validation is bad.
		p := path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "possible_values")
		b, err := os.ReadFile(p)
		if err != nil && !os.IsNotExist(err) {
			// system or permissions error?
			return false, err

		} else if err != nil {
			// the path should exist (this driver is broken?)
			return false, errwrap.Wrapf(err, "unexpected error with: %s", p)
		}
		sp := strings.Split(strings.TrimSuffix(string(b), "\n"), ";") // semicolon is the separator
		if util.StrInList(obj.Value, sp) {                            // is valid?
			obj.value = &obj.Value
			return true, nil
		}

		if obj.Strict {
			return false, nil
		}
		// If the value wasn't valid, before returning false, first see
		// if there's a validate alternate mapping available. This is
		// because some vendors have incompatible values with the same
		// meaning across different firmware versions. Attempt to pick
		// the correct alternate if we can.
		kvv, err := decodeFWAttrJSON(driver)
		if err != nil {
			return false, err
		}

		if kvv == nil {
			return false, nil
		}
		vv, exists := kvv[obj.getKey()]
		if !exists {
			return false, nil
		}
		vs, exists := vv[obj.Value]
		if !exists {
			return false, nil
		}

		for _, v := range vs {
			if util.StrInList(v, sp) { // is valid?
				obj.value = &v
				return true, nil
			}
		}

		return false, nil

	case "integer":
		i, err := strconv.Atoi(obj.Value)
		if err != nil {
			return false, errwrap.Wrapf(err, "value is not an int")
		}

		scalar_min := -1
		scalar_max := -1

		// If these min or max files are missing, there's no limit...
		if b, err := os.ReadFile(path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "min_value")); err != nil && !os.IsNotExist(err) {
			// system or permissions error?
			return false, err

		} else if err == nil {
			min, err := strconv.Atoi(strings.TrimSuffix(string(b), "\n"))
			if err != nil {
				return false, errwrap.Wrapf(err, "can't read min_value")
			}
			if i < min {
				return false, nil // too short
			}
			scalar_min = min
		}

		if b, err := os.ReadFile(path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "max_value")); err != nil && !os.IsNotExist(err) {
			// system or permissions error?
			return false, err

		} else if err == nil {
			max, err := strconv.Atoi(strings.TrimSuffix(string(b), "\n"))
			if err != nil {
				return false, errwrap.Wrapf(err, "can't read max_value")
			}
			if i > max {
				return false, nil // too long
			}
			scalar_max = max
		}

		if b, err := os.ReadFile(path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "scalar_increment")); err != nil && !os.IsNotExist(err) {
			// system or permissions error?
			return false, err

		} else if err == nil {
			if scalar_min == -1 {
				// TODO: or do we assume we centre on zero?
				//scalar_min = 0 // for centering, not checking
				return false, fmt.Errorf("can't verify scalar_increment without min_value")
			}

			scalar_increment, err := strconv.Atoi(strings.TrimSuffix(string(b), "\n"))
			if err != nil {
				return false, errwrap.Wrapf(err, "can't read scalar_increment")
			}

			// XXX: if you see this message, please let us know!
			obj.init.Logf("how do we validate against scalar_increment for %s", obj.getKey())
			obj.init.Logf("scalar_increment: %v", scalar_increment)
			return isValidScalarIncrement(i, scalar_min, scalar_max, scalar_increment), nil
		}

		obj.value = &obj.Value // store
		return true, nil       // must be fine

	case "string":
		// NOTE: I have seen a "possible_values" file with type "string"
		// for thinklmi AlarmTime and UserDefinedAlarmTime. The contents
		// were "HH/MM/SS" which means we can't check against that value
		// or it wouldn't work of course. Current value is "00:00:00".

		// If these min or max files are missing, there's no limit...
		if b, err := os.ReadFile(path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "min_length")); err != nil && !os.IsNotExist(err) {
			// system or permissions error?
			return false, err

		} else if err == nil {
			min, err := strconv.Atoi(strings.TrimSuffix(string(b), "\n"))
			if err != nil {
				return false, errwrap.Wrapf(err, "can't read min_length")
			}
			if len(obj.Value) < min {
				return false, nil // too short
			}
		}

		if b, err := os.ReadFile(path.Join(FWAttrDir, driver, "attributes", obj.getKey(), "max_length")); err != nil && !os.IsNotExist(err) {
			// system or permissions error?
			return false, err

		} else if err == nil {
			max, err := strconv.Atoi(strings.TrimSuffix(string(b), "\n"))
			if err != nil {
				return false, errwrap.Wrapf(err, "can't read max_length")
			}
			if len(obj.Value) > max {
				return false, nil // too long
			}
		}

		obj.value = &obj.Value // store
		return true, nil       // must be fine

	case "ordered-list":
		// XXX: I need HP hardware to test this.
		obj.init.Logf("please send the developer some hardware if you want support here")
		return false, fmt.Errorf("type %s is not implemented", typ)

	case fwattrSkip: // skip mode, not a real type
		//obj.value = &obj.Value // store
		return true, nil

	default:
		return false, fmt.Errorf("unexpected type: %s", typ)
	}
}

// Default returns some sensible defaults for this resource.
func (obj *FWAttrRes) Default() engine.Res {
	return &FWAttrRes{
		Check: true,
	}
}

// Validate reports any problems with the struct definition.
func (obj *FWAttrRes) Validate() error {
	// TODO: is it safe to use at obj.Skip for this getDriver section too?
	// TODO: maybe code elsewhere will turn it into an empty string issue?
	// TODO: we don't want to cause a write-to-wrong-path security bug...
	if driver, err := obj.getDriver(); err != nil {
		return err
	} else if driver == "" { // at runtime we must have chosen a value here
		return fmt.Errorf("driver must not be empty")
	} else if strings.Contains(driver, "/") {
		return fmt.Errorf("driver contains slashes")
	}

	if key := obj.getKey(); key == "" {
		return fmt.Errorf("missing key")
	} else if strings.Contains(key, "/") {
		return fmt.Errorf("key contains slashes")
	}

	// Parse the key and see if it's a valid path.
	if _, err := os.Stat(obj.toPath()); err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return errwrap.Wrapf(err, "unknown stat error")

	} else if err != nil && !obj.Skip {
		return fmt.Errorf("invalid kernel path: %s", obj.toPath())
	}

	// check that value is allowed for this key
	if b, err := obj.isValidValue(); err != nil {
		return err
	} else if !b {
		return fmt.Errorf("value %s is invalid for %s", obj.Value, obj.getKey())
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *FWAttrRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	if _, err := os.Stat(obj.toPath()); err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return errwrap.Wrapf(err, "unknown stat error")

	} else if err != nil && !obj.Skip {
		return fmt.Errorf("invalid kernel path: %s", obj.toPath())

	} else if err != nil && obj.Skip {
		obj.skip = true
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *FWAttrRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// one watches the on disk filename if it creates one, as well as the runtime
// value the kernel has stored!
func (obj *FWAttrRes) Watch(ctx context.Context) error {
	if obj.skip {

		driver, err := obj.getDriver()
		if err != nil {
			// programming error
			panic("driver lookup error")
		}
		if driver == fwattrSkip {
			obj.init.Logf("warning: skip mode: %s not present", FWAttrDir)
		} else {
			obj.init.Logf("warning: skip mode: this key is ineffective")
		}

		obj.init.Running() // when started, notify engine that we're running

		select {
		case <-ctx.Done(): // closed by the engine to signal shutdown
		}

		return nil
	}

	recurse := false
	recWatcher, err := recwatch.NewRecWatcher(obj.toPath(), recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case event, ok := <-recWatcher.Events():
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if err := event.Error; err != nil {
				return err
			}
			if obj.init.Debug { // don't access event.Body if event.Error isn't nil
				obj.init.Logf("event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *FWAttrRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.skip {
		return true, nil
	}

	if obj.value == nil {
		// must be set by calling isValidValue method
		panic("programming error")
	}

	expected := []byte(*obj.value + "\n") // always add a newline

	b, err := os.ReadFile(obj.toPath())
	if err != nil && !os.IsNotExist(err) {
		// system or permissions error?
		return false, err

	} else if err != nil {
		// the path should already exist, we checked in Validate()
		return false, errwrap.Wrapf(err, "unexpected error with: %s", obj.toPath())
	}

	if err == nil && bytes.Equal(expected, b) {
		//if obj.init.Debug {
		//	obj.init.Logf("value matches")
		//}
		return true, nil // we match!
	}

	// Down here, file does not match...

	if !apply {
		return false, nil
	}

	if err := os.WriteFile(obj.toPath(), expected, 0600); err != nil {
		return false, err
	}

	if *obj.value == obj.Value {
		obj.init.Logf("wrote `%s` to: %s\n", *obj.value, obj.getKey())
	} else {
		obj.init.Logf("wrote (modified) `%s` to: %s\n", *obj.value, obj.getKey())
	}

	return false, err
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *FWAttrRes) Cmp(r engine.Res) error {
	// we can only compare FWAttrRes to others of the same resource kind
	res, ok := r.(*FWAttrRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Driver != res.Driver {
		return fmt.Errorf("the Driver differs")
	}
	if obj.Key != res.Key {
		return fmt.Errorf("the Key differs")
	}
	if obj.Value != res.Value {
		return fmt.Errorf("the Value differs")
	}

	if obj.Check != res.Check {
		return fmt.Errorf("the Check differs")
	}
	if obj.Strict != res.Strict {
		return fmt.Errorf("the Strict differs")
	}
	if obj.Skip != res.Skip {
		return fmt.Errorf("the Skip differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *FWAttrRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FWAttrRes // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*FWAttrRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FWAttrRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FWAttrRes(raw) // restore from indirection with type conversion!
	return nil
}

// isValidScalarIncrement checks if a value is within [min, max] and if it is a
// valid stepping. This is for the scalar increment field.
// XXX: I'm not sure if I'm interpreting the scalar_increment stuff correctly.
func isValidScalarIncrement(value, min, max, step int) bool {
	if value < min || (max != -1 && value > max) {
		return false
	}
	return (value-min)%step == 0
}

// KeyValueValues is a map from fwattr name to value to alternate values for
// that value. In other words, for the attribute name "SecureBoot", it can have
// a value named "Enabled" which has a list of alternate aliases on different
// firmware, which is ["Enable"].
type KeyValueValues map[string]map[string][]string

// decodeFWAttrJSON pulls the mapping out of the json data.
func decodeFWAttrJSON(driver string) (KeyValueValues, error) {
	// First we decode into a map of raw JSON to get the driver keys.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data.FWAttr, &root); err != nil {
		return nil, err
	}

	raw, exists := root[driver]
	if !exists { // key doesn't exist
		return nil, nil
	}

	// Now decode only the key we want.
	var kvv KeyValueValues
	if err := json.Unmarshal(raw, &kvv); err != nil {
		return nil, err
	}

	return kvv, nil
}
