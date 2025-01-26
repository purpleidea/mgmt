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

package coresys

import (
	"context"
	"fmt"

	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"

	"github.com/godbus/dbus/v5"
)

const (
	// HostnameFuncName is the name this fact is registered as. It's still a
	// Func Name because this is the name space the fact is actually using.
	HostnameFuncName = "hostname"

	hostname1Path       = "/org/freedesktop/hostname1"
	hostname1Iface      = "org.freedesktop.hostname1"
	dbusPropertiesIface = "org.freedesktop.DBus.Properties"
)

func init() {
	facts.ModuleRegister(ModuleName, HostnameFuncName, func() facts.Fact { return &HostnameFact{} }) // must register the fact and name
}

// HostnameFact is a function that returns the hostname.
// TODO: support hostnames that change in the future.
type HostnameFact struct {
	init *facts.Init
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *HostnameFact) String() string {
	return HostnameFuncName
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal facts that users can use directly.
//func (obj *HostnameFact) Validate() error {
//	return nil
//}

// Info returns some static info about itself.
func (obj *HostnameFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType("str"),
	}
}

// Init runs some startup code for this fact.
func (obj *HostnameFact) Init(init *facts.Init) error {
	obj.init = init
	return nil
}

// Stream returns the single value that this fact has, and then closes.
func (obj *HostnameFact) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // signal that we're done sending

	recurse := false // single file
	recWatcher, err := recwatch.NewRecWatcher("/etc/hostname", recurse)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	// if we share the bus with others, we will get each others messages!!
	bus, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}
	defer bus.Close()
	// watch the PropertiesChanged signal on the hostname1 dbus path
	args := fmt.Sprintf(
		"type='signal', path='%s', interface='%s', member='PropertiesChanged'",
		hostname1Path,
		dbusPropertiesIface,
	)
	if call := bus.BusObject().Call(engineUtil.DBusAddMatch, 0, args); call.Err != nil {
		return errwrap.Wrapf(call.Err, "failed to subscribe to DBus events for hostname1")
	}
	defer bus.BusObject().Call(engineUtil.DBusRemoveMatch, 0, args) // ignore the error

	signals := make(chan *dbus.Signal, 10) // closed by dbus package
	bus.Signal(signals)

	// streams must generate an initial event on startup
	startChan := make(chan struct{}) // start signal
	close(startChan)                 // kick it off!

	for {
		select {
		case <-startChan: // kick the loop once at start
			startChan = nil // disable

		case _, ok := <-signals:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}

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

		// NOTE: We ask the actual machine instead of using obj.init.Hostname
		value, err := obj.Call(ctx)
		if err != nil {
			return err
		}

		select {
		case obj.init.Output <- value:
			// pass
		case <-ctx.Done():
			return nil
		}
	}
}

// Call returns the result of this function.
func (obj *HostnameFact) Call(ctx context.Context) (types.Value, error) {
	conn, err := util.SystemBusPrivateUsable()
	if err != nil {
		return nil, errwrap.Wrapf(err, "failed to connect to the private system bus")
	}
	defer conn.Close()

	hostnameObject := conn.Object(hostname1Iface, hostname1Path)

	h, err := obj.getHostnameProperty(hostnameObject, "Hostname")
	if err != nil {
		return nil, err
	}

	return &types.StrValue{
		V: h,
	}, nil
}

func (obj *HostnameFact) getHostnameProperty(object dbus.BusObject, property string) (string, error) {
	propertyObject, err := object.GetProperty("org.freedesktop.hostname1." + property)
	if err != nil {
		return "", errwrap.Wrapf(err, "failed to get org.freedesktop.hostname1.%s", property)
	}
	if propertyObject.Value() == nil {
		return "", fmt.Errorf("unexpected nil value received when reading property %s", property)
	}

	propertyValue, ok := propertyObject.Value().(string)
	if !ok {
		return "", fmt.Errorf("received unexpected type as %s value, expected string got '%T'", property, propertyValue)
	}

	// expected value and actual value match => checkOk
	return propertyValue, nil
}
