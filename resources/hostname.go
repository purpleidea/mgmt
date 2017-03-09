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
	"errors"
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/util"

	"github.com/godbus/dbus"
	errwrap "github.com/pkg/errors"
)

// ErrResourceInsufficientParameters is returned when the configuration of the resource
// is insufficient for the resource to do any useful work.
var ErrResourceInsufficientParameters = errors.New(
	"Insufficient parameters for this resource")

func init() {
	gob.Register(&HostnameRes{})
}

const (
	hostname1Path  = "/org/freedesktop/hostname1"
	hostname1Iface = "org.freedesktop.hostname1"
	dbusAddMatch   = "org.freedesktop.DBus.AddMatch"
)

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
	BaseRes           `yaml:",inline"`
	Hostname          string `yaml:"hostname"`
	PrettyHostname    string `yaml:"pretty_hostname"`
	StaticHostname    string `yaml:"static_hostname"`
	TransientHostname string `yaml:"transient_hostname"`

	conn *dbus.Conn
}

// Default returns some sensible defaults for this resource.
func (obj *HostnameRes) Default() Res {
	return &HostnameRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *HostnameRes) Validate() error {
	if obj.PrettyHostname == "" && obj.StaticHostname == "" && obj.TransientHostname == "" {
		return ErrResourceInsufficientParameters
	}
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *HostnameRes) Init() error {
	obj.BaseRes.kind = "hostname"
	if obj.PrettyHostname == "" {
		obj.PrettyHostname = obj.Hostname
	}
	if obj.StaticHostname == "" {
		obj.StaticHostname = obj.Hostname
	}
	if obj.TransientHostname == "" {
		obj.TransientHostname = obj.Hostname
	}
	return obj.BaseRes.Init() // call base init, b/c we're overriding
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
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?

	for {
		select {
		case <-signals:
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, _ := obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
			send = true
			obj.StateOK(false) // dirty
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

func updateHostnameProperty(object dbus.BusObject, expectedValue, property, setterName string, apply bool) (checkOK bool, err error) {
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
	log.Printf("Changing %s: %s => %s", property, propertyValue, expectedValue)
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
		propertyCheckOK, err := updateHostnameProperty(hostnameObject, obj.PrettyHostname, "PrettyHostname", "SetPrettyHostname", apply)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && propertyCheckOK
	}
	if obj.StaticHostname != "" {
		propertyCheckOK, err := updateHostnameProperty(hostnameObject, obj.StaticHostname, "StaticHostname", "SetStaticHostname", apply)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && propertyCheckOK
	}
	if obj.TransientHostname != "" {
		propertyCheckOK, err := updateHostnameProperty(hostnameObject, obj.TransientHostname, "Hostname", "SetHostname", apply)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && propertyCheckOK
	}

	return checkOK, nil
}

// HostnameUID is the UID struct for HostnameRes.
type HostnameUID struct {
	BaseUID
	name              string
	prettyHostname    string
	staticHostname    string
	transientHostname string
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *HostnameRes) AutoEdges() AutoEdge {
	return nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *HostnameRes) UIDs() []ResUID {
	x := &HostnameUID{
		BaseUID:           BaseUID{name: obj.GetName(), kind: obj.Kind()},
		name:              obj.Name,
		prettyHostname:    obj.PrettyHostname,
		staticHostname:    obj.StaticHostname,
		transientHostname: obj.TransientHostname,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *HostnameRes) GroupCmp(r Res) bool {
	return false
}

// Compare two resources and return if they are equivalent.
func (obj *HostnameRes) Compare(res Res) bool {
	switch res := res.(type) {
	// we can only compare HostnameRes to others of the same resource
	case *HostnameRes:
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}
		if obj.Name != res.Name {
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
	default:
		return false
	}
	return true
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
