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

package util

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus"
)

const (
	// DBusInterface is the dbus interface that contains genereal methods.
	DBusInterface = "org.freedesktop.DBus"
	// DBusAddMatch is the dbus method to receive a subset of dbus broadcast
	// signals.
	DBusAddMatch = DBusInterface + ".AddMatch"
	// DBusRemoveMatch is the dbus method to remove a previously defined
	// AddMatch rule.
	DBusRemoveMatch = DBusInterface + ".RemoveMatch"
	// DBusSystemd1Path is the base systemd1 path.
	DBusSystemd1Path = "/org/freedesktop/systemd1"
	// DBusSystemd1Iface is the base systemd1 interface.
	DBusSystemd1Iface = "org.freedesktop.systemd1"
	// DBusSystemd1ManagerIface is the systemd manager interface used for
	// interfacing with systemd units.
	DBusSystemd1ManagerIface = DBusSystemd1Iface + ".Manager"
	// DBusRestartUnit is the dbus method for restarting systemd units.
	DBusRestartUnit = DBusSystemd1ManagerIface + ".RestartUnit"
	// DBusStopUnit is the dbus method for stopping systemd units.
	DBusStopUnit = DBusSystemd1ManagerIface + ".StopUnit"
	// DBusSignalJobRemoved is the name of the dbus signal that produces a
	// message when a dbus job is done (or has errored.)
	DBusSignalJobRemoved = "JobRemoved"
)

// ResPathUID returns a unique resource UID based on its name and kind. It's
// safe to use as a token in a path, and as a result has no slashes in it.
func ResPathUID(res engine.Res) string {
	// res.Name() is NOT sufficiently unique to use as a UID here, because:
	// a name of: /tmp/mgmt/foo is /tmp-mgmt-foo and
	// a name of: /tmp/mgmt-foo -> /tmp-mgmt-foo if we replace slashes.
	// As a result, we base64 encode (but without slashes).
	name := strings.ReplaceAll(res.Name(), "/", "-")
	if os.PathSeparator != '/' { // lol windows?
		name = strings.ReplaceAll(name, string(os.PathSeparator), "-")
	}
	b := []byte(res.Name())
	encoded := base64.URLEncoding.EncodeToString(b)
	// Add the safe name on so that it's easier to identify by name...
	return fmt.Sprintf("%s-%s+%s", res.Kind(), encoded, name)
}

// ResToB64 encodes a resource to a base64 encoded string (after serialization).
func ResToB64(res engine.Res) (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(&res) // pass with &
	if err != nil {
		return "", errwrap.Wrapf(err, "gob failed to encode")
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// B64ToRes decodes a resource from a base64 encoded string (after
// deserialization).
func B64ToRes(str string) (engine.Res, error) {
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, errwrap.Wrapf(err, "base64 failed to decode")
	}
	b := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b)
	if err := d.Decode(&output); err != nil { // pass with &
		return nil, errwrap.Wrapf(err, "gob failed to decode")
	}
	res, ok := output.(engine.Res)
	if !ok {
		return nil, fmt.Errorf("output `%v` is not a Res", output)
	}
	return res, nil
}

// GetUID returns the UID of an user. It supports an UID or an username. Caller
// should first check user is not empty. It will return an error if it can't
// lookup the UID or username.
func GetUID(username string) (int, error) {
	userObj, err := user.LookupId(username)
	if err == nil {
		return strconv.Atoi(userObj.Uid)
	}

	userObj, err = user.Lookup(username)
	if err == nil {
		return strconv.Atoi(userObj.Uid)
	}

	return -1, errwrap.Wrapf(err, "user lookup error (%s)", username)
}

// GetGID returns the GID of a group. It supports a GID or a group name. Caller
// should first check group is not empty. It will return an error if it can't
// lookup the GID or group name.
func GetGID(group string) (int, error) {
	groupObj, err := user.LookupGroupId(group)
	if err == nil {
		return strconv.Atoi(groupObj.Gid)
	}

	groupObj, err = user.LookupGroup(group)
	if err == nil {
		return strconv.Atoi(groupObj.Gid)
	}

	return -1, errwrap.Wrapf(err, "group lookup error (%s)", group)
}

// RestartUnit resarts the given dbus unit and waits for it to finish starting.
func RestartUnit(ctx context.Context, conn *dbus.Conn, unit string) error {
	return unitStateAction(ctx, conn, unit, DBusRestartUnit)
}

// StopUnit stops the given dbus unit and waits for it to finish stopping.
func StopUnit(ctx context.Context, conn *dbus.Conn, unit string) error {
	return unitStateAction(ctx, conn, unit, DBusStopUnit)
}

// unitStateAction is a helper function to perform state actions on systemd
// units. It waits for the requested job to be complete before it returns.
func unitStateAction(ctx context.Context, conn *dbus.Conn, unit, action string) error {
	// Add a dbus rule to watch the systemd1 JobRemoved signal, used to wait
	// until the job completes.
	args := []string{
		"type='signal'",
		fmt.Sprintf("path='%s'", DBusSystemd1Path),
		fmt.Sprintf("interface='%s'", DBusSystemd1ManagerIface),
		fmt.Sprintf("member='%s'", DBusSignalJobRemoved),
		fmt.Sprintf("arg2='%s'", unit),
	}
	// match dbus messages
	if call := conn.BusObject().Call(DBusAddMatch, 0, strings.Join(args, ",")); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error creating dbus call")
	}
	defer conn.BusObject().Call(DBusRemoveMatch, 0, args) // ignore the error

	// channel for godbus signal
	ch := make(chan *dbus.Signal)
	defer close(ch)
	// subscribe the channel to the signal
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	// perform requested action on specified unit
	sd1 := conn.Object(DBusSystemd1Iface, dbus.ObjectPath(DBusSystemd1Path))
	if call := sd1.Call(action, 0, unit, "fail"); call.Err != nil {
		return errwrap.Wrapf(call.Err, "error stopping unit: %s", unit)
	}

	// wait for the job to be removed, indicating completion
	select {
	case event, ok := <-ch:
		if !ok {
			return fmt.Errorf("channel closed unexpectedly")
		}
		if event.Body[3] != "done" {
			return fmt.Errorf("unexpected job status: %s", event.Body[3])
		}
	case <-ctx.Done():
		return fmt.Errorf("action %s on %s failed due to context timeout", action, unit)
	}
	return nil
}

// autoEdgeCombiner holds the state of the auto edge generator.
type autoEdgeCombiner struct {
	ae  []engine.AutoEdge
	ptr int
}

// Next returns the next automatic edge.
func (obj *autoEdgeCombiner) Next() []engine.ResUID {
	if len(obj.ae) <= obj.ptr {
		panic("shouldn't be called anymore!")
	}
	return obj.ae[obj.ptr].Next() // return the next edge
}

// Test takes the output of the last call to Next() and outputs true if we
// should continue.
func (obj *autoEdgeCombiner) Test(input []bool) bool {
	if !obj.ae[obj.ptr].Test(input) {
		obj.ptr++ // match found, on to the next
	}
	return len(obj.ae) > obj.ptr // are there any auto edges left?
}

// AutoEdgeCombiner takes any number of AutoEdge structs, and combines them into
// a single one, so that the logic from each one can be built separately, and
// then combined using this utility. This makes implementing different AutoEdge
// generators much easier. This respects the Next() and Test() API, and ratchets
// through each AutoEdge entry until they have all run their course.
func AutoEdgeCombiner(ae ...engine.AutoEdge) (engine.AutoEdge, error) {
	return &autoEdgeCombiner{
		ae: ae,
	}, nil
}
