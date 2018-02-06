// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"log"
	"strconv"
	"unicode"

	"github.com/purpleidea/mgmt/util"

	systemdDbus "github.com/coreos/go-systemd/dbus"
	machined "github.com/coreos/go-systemd/machine1"
	systemdUtil "github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus"
	errwrap "github.com/pkg/errors"
)

const (
	running           = "running"
	stopped           = "stopped"
	dbusInterface     = "org.freedesktop.machine1.Manager"
	machineNew        = "org.freedesktop.machine1.Manager.MachineNew"
	machineRemoved    = "org.freedesktop.machine1.Manager.MachineRemoved"
	nspawnServiceTmpl = "systemd-nspawn@%s"
)

func init() {
	RegisterResource("nspawn", func() Res { return &NspawnRes{} })
}

// NspawnRes is an nspawn container resource.
type NspawnRes struct {
	BaseRes `yaml:",inline"`
	State   string `yaml:"state"`
	// We're using the svc resource to start and stop the machine because
	// that's what machinectl does. We're not using svc.Watch because then we
	// would have two watches potentially racing each other and producing
	// potentially unexpected results. We get everything we need to monitor
	// the machine state changes from the org.freedesktop.machine1 object.
	svc *SvcRes
}

// Default returns some sensible defaults for this resource.
func (obj *NspawnRes) Default() Res {
	return &NspawnRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
		State: running,
	}
}

// makeComposite creates a pointer to a SvcRes. The pointer is used to
// validate and initialize the nested svc.
func (obj *NspawnRes) makeComposite() (*SvcRes, error) {
	res, err := NewNamedResource("svc", fmt.Sprintf(nspawnServiceTmpl, obj.GetName()))
	if err != nil {
		return nil, err
	}
	svc := res.(*SvcRes)
	svc.State = obj.State
	return svc, nil
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
	verNum, err := strconv.Unquote(verString)
	if err != nil {
		return 0, errwrap.Wrapf(err, "error unquoting version number")
	}
	// cast to uint16
	ver, err := strconv.ParseUint(verNum, 10, 16)
	if err != nil {
		return 0, errwrap.Wrapf(err, "error casting systemd version number")
	}
	return uint16(ver), nil
}

// Validate if the params passed in are valid data.
func (obj *NspawnRes) Validate() error {
	if len(obj.GetName()) > 64 {
		return fmt.Errorf("name must be 64 characters or less")
	}
	// check if systemd version is higher than 231 to allow non-alphanumeric
	// machine names, as previous versions would error in such cases
	ver, err := systemdVersion()
	if err != nil {
		return err
	}
	if ver < 231 {
		for _, char := range obj.GetName() {
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
		return errwrap.Wrapf(err, "validate failed for embedded svc: %s", obj.svc)
	}
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *NspawnRes) Init() error {
	svc, err := obj.makeComposite()
	if err != nil {
		return errwrap.Wrapf(err, "makeComposite failed in init")
	}
	obj.svc = svc
	if err := obj.svc.Init(); err != nil {
		return err
	}
	return obj.BaseRes.Init()
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

	// add a match rule to match messages going through the message bus
	call := bus.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		fmt.Sprintf("type='signal',interface='%s',eavesdrop='true'",
			dbusInterface))
	// <-call.Done
	if err := call.Err; err != nil {
		return err
	}
	// TODO: verify that implementation doesn't deadlock if there are unread
	// messages left in the channel
	busChan := make(chan *dbus.Signal, 10)
	bus.Signal(busChan)

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false
	var exit *error

	defer close(busChan)
	defer bus.Close()
	defer bus.RemoveSignal(busChan)
	for {
		select {
		case event := <-busChan:
			// process org.freedesktop.machine1 events for this resource's name
			if event.Body[0] == obj.GetName() {
				log.Printf("%s: Event received: %v", obj, event.Name)
				if event.Name == machineNew {
					log.Printf("%s: Machine started", obj)
				} else if event.Name == machineRemoved {
					log.Printf("%s: Machine stopped", obj)
				} else {
					return fmt.Errorf("unknown event: %s", event.Name)
				}
				send = true
				obj.StateOK(false) // dirty
			}

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *NspawnRes) CheckApply(apply bool) (checkOK bool, err error) {
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
	properties, err := conn.DescribeMachine(obj.GetName())
	if err != nil {
		if err, ok := err.(dbus.Error); ok && err.Name !=
			"org.freedesktop.machine1.NoSuchMachine" {
			return false, err
		}
		exists = false
		// if we could not successfully get the properties because
		// there's no such machine the machine is stopped
		// error if we need the image ignore if we don't
		if _, err = conn.GetImage(obj.GetName()); err != nil && obj.State != stopped {
			return false, fmt.Errorf(
				"no machine nor image named '%s'",
				obj.GetName())
		}
	}
	if obj.debug {
		log.Printf("%s: properties: %v", obj, properties)
	}
	// if the machine doesn't exist and is supposed to
	// be stopped or the state matches we're done
	if !exists && obj.State == stopped || properties["State"] == obj.State {
		if obj.debug {
			log.Printf("%s: CheckApply() in valid state", obj)
		}
		return true, nil
	}

	// end of state checking. if we're here, checkOK is false
	if !apply {
		return false, nil
	}

	log.Printf("%s: CheckApply() applying '%s' state", obj, obj.State)
	// use the embedded svc to apply the correct state
	if _, err := obj.svc.CheckApply(apply); err != nil {
		return false, errwrap.Wrapf(err, "nested svc failed")
	}

	return false, nil
}

// NspawnUID is a unique resource identifier.
type NspawnUID struct {
	// NOTE: There is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	BaseUID
	name string // the machine name
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *NspawnUID) IFF(uid ResUID) bool {
	res, ok := uid.(*NspawnUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one although some resources can return multiple.
func (obj *NspawnRes) UIDs() []ResUID {
	x := &NspawnUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name, // svc name
	}
	return append([]ResUID{x}, obj.svc.UIDs()...)
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *NspawnRes) GroupCmp(r Res) bool {
	_, ok := r.(*NspawnRes)
	if !ok {
		return false
	}
	// TODO: this would be quite useful for this resource!
	return false
}

// Compare two resources and return if they are equivalent.
func (obj *NspawnRes) Compare(r Res) bool {
	// we can only compare NspawnRes to others of the same resource kind
	res, ok := r.(*NspawnRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) {
		return false
	}
	if obj.Name != res.Name {
		return false
	}
	if obj.State != res.State {
		return false
	}

	// TODO: why is res.svc ever nil?
	if (obj.svc == nil) != (res.svc == nil) { // xor
		return false
	}
	if obj.svc != nil && res.svc != nil {
		if !obj.svc.Compare(res.svc) {
			return false
		}
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
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
