// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	systemdDbus "github.com/coreos/go-systemd/dbus"
	machined "github.com/coreos/go-systemd/machine1"
	systemdUtil "github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus"
)

const (
	running           = "running"
	stopped           = "stopped"
	dbusMachine1Iface = "org.freedesktop.machine1.Manager"
	machineNew        = dbusMachine1Iface + ".MachineNew"
	machineRemoved    = dbusMachine1Iface + ".MachineRemoved"
	nspawnServiceTmpl = "systemd-nspawn@%s"
)

func init() {
	engine.RegisterResource("nspawn", func() engine.Res { return &NspawnRes{} })
}

// NspawnRes is an nspawn container resource.
type NspawnRes struct {
	traits.Base // add the base methods without re-implementation
	//traits.Groupable // TODO: this would be quite useful for this resource
	traits.Refreshable // needed because we embed a svc res

	init *engine.Init

	State string `yaml:"state"`
	// We're using the svc resource to start and stop the machine because
	// that's what machinectl does. We're not using svc.Watch because then we
	// would have two watches potentially racing each other and producing
	// potentially unexpected results. We get everything we need to monitor
	// the machine state changes from the org.freedesktop.machine1 object.
	svc *SvcRes
}

// Default returns some sensible defaults for this resource.
func (obj *NspawnRes) Default() engine.Res {
	return &NspawnRes{
		State: running,
	}
}

// makeComposite creates a pointer to a SvcRes. The pointer is used to validate
// and initialize the nested svc.
func (obj *NspawnRes) makeComposite() (*SvcRes, error) {
	res, err := engine.NewNamedResource("svc", fmt.Sprintf(nspawnServiceTmpl, obj.Name()))
	if err != nil {
		return nil, err
	}
	svc := res.(*SvcRes)
	svc.State = obj.State
	return svc, nil
}

// Validate if the params passed in are valid data.
func (obj *NspawnRes) Validate() error {
	if len(obj.Name()) > 64 {
		return fmt.Errorf("name must be 64 characters or less")
	}
	// check if systemd version is higher than 231 to allow non-alphanumeric
	// machine names, as previous versions would error in such cases
	ver, err := systemdVersion()
	if err != nil {
		return err
	}
	if ver < 231 {
		for _, char := range obj.Name() {
			if !unicode.IsLetter(char) && !unicode.IsNumber(char) {
				return fmt.Errorf("name must only contain alphanumeric characters for systemd versions < 231")
			}
		}
	}

	if obj.State != running && obj.State != stopped {
		return fmt.Errorf("invalid state: %s", obj.State)
	}

	svc, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in validate")
	}
	if err := svc.Validate(); err != nil { // composite resource
		return errwrap.Wrapf(err, "validate failed for embedded svc: %s", svc)
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *NspawnRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	svc, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in init")
	}
	obj.svc = svc
	// TODO: we could build a new init that adds a prefix to the logger...
	return obj.svc.Init(init)
}

// Close is run by the engine to clean up after the resource is done.
func (obj *NspawnRes) Close() error {
	if obj.svc != nil {
		return obj.svc.Close()
	}
	return nil
}

// Watch for state changes and sends a message to the bus if there is a change.
func (obj *NspawnRes) Watch() error {
	// this resource depends on systemd to ensure that it's running
	if !systemdUtil.IsRunningSystemd() {
		return fmt.Errorf("systemd is not running")
	}

	// create a private message bus
	bus, err := util.SystemBusPrivateUsable()
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}
	defer bus.Close()

	// add a match rule to match messages going through the message bus
	args := fmt.Sprintf("type='signal',interface='%s',eavesdrop='true'", dbusMachine1Iface)
	if call := bus.BusObject().Call(engineUtil.DBusAddMatch, 0, args); call.Err != nil {
		return err
	}
	defer bus.BusObject().Call(engineUtil.DBusRemoveMatch, 0, args) // ignore the error

	busChan := make(chan *dbus.Signal)
	defer close(busChan)
	bus.Signal(busChan)
	defer bus.RemoveSignal(busChan) // not needed here, but nice for symmetry

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		select {
		case event := <-busChan:
			// process org.freedesktop.machine1 events for this resource's name
			if event.Body[0] == obj.Name() {
				obj.init.Logf("Event received: %v", event.Name)
				if event.Name == machineNew {
					obj.init.Logf("Machine started")
				} else if event.Name == machineRemoved {
					obj.init.Logf("Machine stopped")
				} else {
					return fmt.Errorf("unknown event: %s", event.Name)
				}
				send = true
			}

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

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *NspawnRes) CheckApply(apply bool) (bool, error) {
	// this resource depends on systemd to ensure that it's running
	if !systemdUtil.IsRunningSystemd() {
		return false, errors.New("systemd is not running")
	}

	// connect to org.freedesktop.machine1.Manager
	conn, err := machined.New()
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to connect to dbus")
	}

	// compare the current state with the desired state and perform the
	// appropriate action
	var exists = true
	properties, err := conn.DescribeMachine(obj.Name())
	if err != nil {
		if err, ok := err.(dbus.Error); ok && err.Name !=
			"org.freedesktop.machine1.NoSuchMachine" {
			return false, err
		}
		exists = false
		// if we could not successfully get the properties because
		// there's no such machine the machine is stopped
		// error if we need the image ignore if we don't
		if _, err = conn.GetImage(obj.Name()); err != nil && obj.State != stopped {
			return false, fmt.Errorf(
				"no machine nor image named '%s'",
				obj.Name())
		}
	}
	if obj.init.Debug {
		obj.init.Logf("properties: %v", properties)
	}
	// if the machine doesn't exist and is supposed to
	// be stopped or the state matches we're done
	if !exists && obj.State == stopped || properties["State"] == obj.State {
		if obj.init.Debug {
			obj.init.Logf("CheckApply() in valid state")
		}
		return true, nil
	}

	// end of state checking. if we're here, checkOK is false
	if !apply {
		return false, nil
	}

	obj.init.Logf("CheckApply() applying '%s' state", obj.State)
	// use the embedded svc to apply the correct state
	if _, err := obj.svc.CheckApply(apply); err != nil {
		return false, errwrap.Wrapf(err, "nested svc failed")
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *NspawnRes) Cmp(r engine.Res) error {
	// we can only compare NspawnRes to others of the same resource kind
	res, ok := r.(*NspawnRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	// TODO: why is res.svc ever nil?
	if (obj.svc == nil) != (res.svc == nil) { // xor
		return fmt.Errorf("the svc differs")
	}
	if obj.svc != nil && res.svc != nil {
		if err := obj.svc.Cmp(res.svc); err != nil {
			return errwrap.Wrapf(err, "the svc differs")
		}
	}

	return nil
}

// NspawnUID is a unique resource identifier.
type NspawnUID struct {
	// NOTE: There is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	engine.BaseUID

	name string // the machine name
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *NspawnUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*NspawnUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one although some resources can return multiple.
func (obj *NspawnRes) UIDs() []engine.ResUID {
	x := &NspawnUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(), // svc name
	}
	return append([]engine.ResUID{x}, obj.svc.UIDs()...)
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *NspawnRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes NspawnRes // indirection to avoid infinite recursion

	def := obj.Default()        // get the default
	res, ok := def.(*NspawnRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to NspawnRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = NspawnRes(raw) // restore from indirection with type conversion!
	return nil
}

// systemdVersion uses dbus to check which version of systemd is installed.
func systemdVersion() (uint16, error) {
	// check if systemd is running
	if !systemdUtil.IsRunningSystemd() {
		return 0, fmt.Errorf("systemd is not running")
	}
	bus, err := systemdDbus.NewSystemdConnection()
	if err != nil {
		return 0, errwrap.Wrapf(err, "failed to connect to bus")
	}
	defer bus.Close()
	// get the systemd version
	verString, err := bus.GetManagerProperty("Version")
	if err != nil {
		return 0, errwrap.Wrapf(err, "could not get version property")
	}
	// lose the surrounding quotes
	verNumString, err := strconv.Unquote(verString)
	if err != nil {
		return 0, errwrap.Wrapf(err, "error unquoting version number")
	}
	// trim possible version suffix like in "242.19-1"
	verNum := strings.Split(verNumString, ".")[0]
	// cast to uint16
	ver, err := strconv.ParseUint(verNum, 10, 16)
	if err != nil {
		return 0, errwrap.Wrapf(err, "error casting systemd version number")
	}
	return uint16(ver), nil
}
