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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"

	"github.com/godbus/dbus"
	errwrap "github.com/pkg/errors"
)

func init() {
	engine.RegisterResource("hostname", func() engine.Res { return &HostnameRes{} })
}

const (
	hostname1Path  = "/org/freedesktop/hostname1"
	hostname1Iface = "org.freedesktop.hostname1"
	dbusAddMatch   = "org.freedesktop.DBus.AddMatch"
)

// ErrResourceInsufficientParameters is returned when the configuration of the
// resource is insufficient for the resource to do any useful work.
var ErrResourceInsufficientParameters = errors.New("insufficient parameters for this resource")

// HostnameRes is a resource that allows setting and watching the hostname.
//
// StaticHostname is the one configured in /etc/hostname or a similar file.
// It is chosen by the local user. It is not always in sync with the current
// host name as returned by the gethostname() system call.
//
// TransientHostname is the one configured via the kernel's sethostbyname().
// It can be different from the static hostname in case DHCP or mDNS have been
// configured to change the name based on network information.
//
// PrettyHostname is a free-form UTF8 host name for presentation to the user.
//
// Hostname is the fallback value for all 3 fields above, if only Hostname is
// specified, it will set all 3 fields to this value.
type HostnameRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	Hostname          string `yaml:"hostname"`
	PrettyHostname    string `yaml:"pretty_hostname"`
	StaticHostname    string `yaml:"static_hostname"`
	TransientHostname string `yaml:"transient_hostname"`

	conn *dbus.Conn
}

// Default returns some sensible defaults for this resource.
func (obj *HostnameRes) Default() engine.Res {
	return &HostnameRes{}
}

// Validate if the params passed in are valid data.
func (obj *HostnameRes) Validate() error {
	if obj.PrettyHostname == "" && obj.StaticHostname == "" && obj.TransientHostname == "" {
		return ErrResourceInsufficientParameters
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *HostnameRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	if obj.PrettyHostname == "" {
		obj.PrettyHostname = obj.Hostname
	}
	if obj.StaticHostname == "" {
		obj.StaticHostname = obj.Hostname
	}
	if obj.TransientHostname == "" {
		obj.TransientHostname = obj.Hostname
	}
	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *HostnameRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *HostnameRes) Watch() error {
	// if we share the bus with others, we will get each others messages!!
	bus, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return errwrap.Wrap(err, "Failed to connect to bus")
	}
	defer bus.Close()
	callResult := bus.BusObject().Call(
		"org.freedesktop.DBus.AddMatch", 0,
		fmt.Sprintf("type='signal',path='%s',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged'", hostname1Path))
	if callResult.Err != nil {
		return errwrap.Wrap(callResult.Err, "Failed to subscribe to DBus events for hostname1")
	}

	signals := make(chan *dbus.Signal, 10) // closed by dbus package
	bus.Signal(signals)

	// notify engine that we're running
	if err := obj.init.Running(); err != nil {
		return err // exit if requested
	}

	var send = false // send event?
	for {
		select {
		case <-signals:
			send = true
			obj.init.Dirty() // dirty

		case event, ok := <-obj.init.Events:
			if !ok {
				return nil
			}
			if err := obj.init.Read(event); err != nil {
				return err
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			if err := obj.init.Event(); err != nil {
				return err // exit if requested
			}
		}
	}
}

func (obj *HostnameRes) updateHostnameProperty(object dbus.BusObject, expectedValue, property, setterName string, apply bool) (checkOK bool, err error) {
	propertyObject, err := object.GetProperty("org.freedesktop.hostname1." + property)
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get org.freedesktop.hostname1.%s", property)
	}
	if propertyObject.Value() == nil {
		return false, errwrap.Errorf("Unexpected nil value received when reading property %s", property)
	}

	propertyValue, ok := propertyObject.Value().(string)
	if !ok {
		return false, fmt.Errorf("Received unexpected type as %s value, expected string got '%T'", property, propertyValue)
	}

	// expected value and actual value match => checkOk
	if propertyValue == expectedValue {
		return true, nil
	}

	// nothing to do anymore
	if !apply {
		return false, nil
	}

	// attempting to apply the changes
	obj.init.Logf("Changing %s: %s => %s", property, propertyValue, expectedValue)
	if err := object.Call("org.freedesktop.hostname1."+setterName, 0, expectedValue, false).Err; err != nil {
		return false, errwrap.Wrapf(err, "failed to call org.freedesktop.hostname1.%s", setterName)
	}

	// all good changes should now be applied again
	return false, nil
}

// CheckApply method for Hostname resource.
func (obj *HostnameRes) CheckApply(apply bool) (checkOK bool, err error) {
	conn, err := util.SystemBusPrivateUsable()
	if err != nil {
		return false, errwrap.Wrap(err, "Failed to connect to the private system bus")
	}
	defer conn.Close()

	hostnameObject := conn.Object(hostname1Iface, hostname1Path)

	checkOK = true
	if obj.PrettyHostname != "" {
		propertyCheckOK, err := obj.updateHostnameProperty(hostnameObject, obj.PrettyHostname, "PrettyHostname", "SetPrettyHostname", apply)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && propertyCheckOK
	}
	if obj.StaticHostname != "" {
		propertyCheckOK, err := obj.updateHostnameProperty(hostnameObject, obj.StaticHostname, "StaticHostname", "SetStaticHostname", apply)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && propertyCheckOK
	}
	if obj.TransientHostname != "" {
		propertyCheckOK, err := obj.updateHostnameProperty(hostnameObject, obj.TransientHostname, "Hostname", "SetHostname", apply)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && propertyCheckOK
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HostnameRes) Cmp(r engine.Res) error {
	if !obj.Compare(r) {
		return fmt.Errorf("did not compare")
	}
	return nil
}

// Compare two resources and return if they are equivalent.
func (obj *HostnameRes) Compare(r engine.Res) bool {
	// we can only compare HostnameRes to others of the same resource kind
	res, ok := r.(*HostnameRes)
	if !ok {
		return false
	}

	if obj.PrettyHostname != res.PrettyHostname {
		return false
	}
	if obj.StaticHostname != res.StaticHostname {
		return false
	}
	if obj.TransientHostname != res.TransientHostname {
		return false
	}

	return true
}

// HostnameUID is the UID struct for HostnameRes.
type HostnameUID struct {
	engine.BaseUID

	name              string
	prettyHostname    string
	staticHostname    string
	transientHostname string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *HostnameRes) UIDs() []engine.ResUID {
	x := &HostnameUID{
		BaseUID:           engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:              obj.Name(),
		prettyHostname:    obj.PrettyHostname,
		staticHostname:    obj.StaticHostname,
		transientHostname: obj.TransientHostname,
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *HostnameRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HostnameRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*HostnameRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HostnameRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HostnameRes(raw) // restore from indirection with type conversion!
	return nil
}
