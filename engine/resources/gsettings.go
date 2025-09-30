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
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"reflect"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("gsettings", func() engine.Res { return &GsettingsRes{} })
}

const (
	gsettingsTmpl = "gsettings@%s"
)

// GsettingsRes is a resource for setting dconf values through gsettings. The
// ideal scenario is that this runs as the same user that wants settings set.
// This should be done by a local user-specific mgmt daemon. As a special case,
// we can run as root (or anyone with permission) which launches a subprocess
// which setuid/setgid's to that user to run the needed operations. To specify
// the schema and key, set the resource name as "schema key" (separated by a
// single space character) or use the parameters.
type GsettingsRes struct {
	// XXX: add a dbus version of this-- it will require running as the user
	// directly since in that scenario we can't spawn a process of the right
	// uid/gid, and if we set either of those we would interfere with all of
	// the normal mgmt stuff running inside this process.

	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Schema is the schema to use in. This can be schema:path if the schema
	// doesn't have a fixed path. See the `gsettings` manual for more info.
	Schema string `lang:"schema" yaml:"schema"`

	// Key is the key to set.
	Key string `lang:"key" yaml:"key"`

	// Type is the type value to set. This can be "bool", "str", "int", or
	// "custom".
	// XXX: add support for [][]str and so on...
	Type string `lang:"type" yaml:"type"`

	// Value is the value to set. It is interface{} because it can hold any
	// value type.
	// XXX: Add resource unification to this key
	Value interface{} `lang:"value" yaml:"value"`

	// User is the (optional) user to use to execute the command. It is used
	// for any command being run.
	User string `lang:"user" yaml:"user"`

	// Group is the (optional) group to use to execute the command. It is
	// used for any command being run.
	Group string `lang:"group" yaml:"group"`

	// XXX: We should have a "once" functionality if this param is set true.
	// XXX: Basically it would change that field once, and store a "tag"
	// file to say it was done.
	// XXX: Maybe that should be a metaparam called Once that works anywhere.
	// XXX: Maybe there should be a way to reset the "once" tag too...
	//Once string `lang:"once" yaml:"once"`

	// We're using the exec resource to build the resources because it's all
	// done through exec.
	exec *ExecRes
}

// Default returns some sensible defaults for this resource.
func (obj *GsettingsRes) Default() engine.Res {
	return &GsettingsRes{}
}

// parse is a helper to pull out the correct schema and key to use.
func (obj *GsettingsRes) parse() (string, string, error) {
	schema := obj.Schema
	key := obj.Key

	sp := strings.Split(obj.Name(), " ")
	if len(sp) == 2 && obj.Schema == "" && obj.Key == "" {
		schema = sp[0]
		key = sp[1]
	}

	if schema == "" {
		return "", "", fmt.Errorf("empty schema")
	}
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}

	return schema, key, nil
}

// value is a helper to pull out the value in the correct format to use.
func (obj *GsettingsRes) value() (string, error) {
	if obj.Type == "bool" {
		v, ok := obj.Value.(bool)
		if !ok {
			return "", fmt.Errorf("invalid bool")
		}
		if v {
			return "true", nil
		}
		return "false", nil
	}

	if obj.Type == "str" {
		v, ok := obj.Value.(string)
		if !ok {
			return "", fmt.Errorf("invalid str")
		}
		return v, nil
	}

	if obj.Type == "int" {
		v, ok := obj.Value.(int)
		if !ok {
			return "", fmt.Errorf("invalid int")
		}
		return strconv.Itoa(v), nil
	}

	if obj.Type == "custom" {
		v, ok := obj.Value.(string)
		if !ok {
			return "", fmt.Errorf("invalid custom")
		}
		return v, nil
	}

	// XXX: add proper type parsing

	return "", fmt.Errorf("invalid type: %s", obj.Type)
}

// uid is a helper to get the correct uid.
func (obj *GsettingsRes) uid() (int, error) {
	uid := obj.User // something or empty
	if obj.User == "" {
		u, err := user.Current()
		if err != nil {
			return -1, err
		}

		uid = u.Uid
	}

	out, err := engineUtil.GetUID(uid)
	if err != nil {
		return -1, errwrap.Wrapf(err, "error looking up uid for %s", uid)
	}
	return out, nil
}

// makeComposite creates a pointer to a ExecRes. The pointer is used to validate
// and initialize the nested exec.
func (obj *GsettingsRes) makeComposite() (*ExecRes, error) {
	cmd, err := exec.LookPath("gsettings")
	if err != nil {
		return nil, err
	}

	schema, key, err := obj.parse()
	if err != nil {
		return nil, err
	}
	val, err := obj.value()
	if err != nil {
		return nil, err
	}
	uid, err := obj.uid()
	if err != nil {
		return nil, err
	}

	res, err := engine.NewNamedResource("exec", fmt.Sprintf(gsettingsTmpl, obj.Name()))
	if err != nil {
		return nil, err
	}
	exec := res.(*ExecRes)

	exec.Cmd = cmd
	exec.Args = []string{
		"set",
		schema,
		key,
		val,
	}
	exec.Cwd = "/"

	exec.IfCmd = fmt.Sprintf("%s get %s %s", cmd, schema, key)
	exec.IfCwd = "/"
	expected := val + "\n" // value comes with a trailing newline
	exec.IfEquals = &expected

	exec.WatchCmd = fmt.Sprintf("%s monitor %s %s", cmd, schema, key)
	exec.WatchCwd = "/"

	exec.User = obj.User
	exec.Group = obj.Group

	exec.Env = map[string]string{
		// Either of these will work, so we'll include both for fun.
		"DBUS_SESSION_BUS_ADDRESS": fmt.Sprintf("unix:path=/run/user/%d/bus", uid),
		"XDG_RUNTIME_DIR":          fmt.Sprintf("/run/user/%d/", uid),
	}
	//exec.Timeout = ? // TODO: should we have a timeout to prevent blocking?

	return exec, nil
}

// Validate reports any problems with the struct definition.
func (obj *GsettingsRes) Validate() error {
	if _, _, err := obj.parse(); err != nil {
		return err
	}
	// validation of obj.Type happens in this function.
	if _, err := obj.value(); err != nil {
		return err
	}

	exec, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in validate")
	}
	if err := exec.Validate(); err != nil { // composite resource
		return errwrap.Wrapf(err, "validate failed for embedded exec: %s", exec)
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *GsettingsRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	exec, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in init")
	}
	obj.exec = exec

	newInit := obj.init.Copy()
	newInit.Send = func(interface{}) error { // override so exec can't send
		return nil
	}
	newInit.Logf = func(format string, v ...interface{}) {
		//if format == "cmd out empty!" {
		//	return
		//}
		//obj.init.Logf("exec: "+format, v...)
	}

	return obj.exec.Init(newInit)
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *GsettingsRes) Cleanup() error {
	if obj.exec != nil {
		return obj.exec.Cleanup()
	}
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *GsettingsRes) Watch(ctx context.Context) error {
	return obj.exec.Watch(ctx)
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *GsettingsRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	obj.init.Logf("%s", obj.exec.IfCmd) // "gsettings get"

	checkOK, err := obj.exec.CheckApply(ctx, apply)
	if err != nil {
		return checkOK, err
	}

	if !checkOK {
		// "gsettings set"
		obj.init.Logf("%s %s", obj.exec.Cmd, strings.Join(obj.exec.Args, " "))
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *GsettingsRes) Cmp(r engine.Res) error {
	// we can only compare GsettingsRes to others of the same resource kind
	res, ok := r.(*GsettingsRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Schema != res.Schema {
		return fmt.Errorf("the Schema differs")
	}
	if obj.Key != res.Key {
		return fmt.Errorf("the Key differs")
	}
	if obj.Type != res.Type {
		return fmt.Errorf("the Type differs")
	}

	//if obj.Value != res.Value {
	//	return fmt.Errorf("the Value differs")
	//}
	if !reflect.DeepEqual(obj.Value, res.Value) {
		return fmt.Errorf("the Value field differs")
	}

	if obj.User != res.User {
		return fmt.Errorf("the User differs")
	}
	if obj.Group != res.Group {
		return fmt.Errorf("the Group differs")
	}

	// TODO: why is res.exec ever nil?
	if (obj.exec == nil) != (res.exec == nil) { // xor
		return fmt.Errorf("the exec differs")
	}
	if obj.exec != nil && res.exec != nil {
		if err := obj.exec.Cmp(res.exec); err != nil {
			return errwrap.Wrapf(err, "the exec differs")
		}
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *GsettingsRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes GsettingsRes // indirection to avoid infinite recursion

	def := obj.Default()           // get the default
	res, ok := def.(*GsettingsRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to GsettingsRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = GsettingsRes(raw) // restore from indirection with type conversion!
	return nil
}
