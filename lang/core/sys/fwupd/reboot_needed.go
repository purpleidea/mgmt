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

package corefwupd

import (
	"context"
	"fmt"

	coresys "github.com/purpleidea/mgmt/lang/core/sys"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
)

const (
	// RebootNeededFuncName is the name this func is registered as.
	RebootNeededFuncName = "reboot_needed"

	// dbusName is the well-known bus name of the fwupd daemon. We speak to
	// it directly, we do not depend on the engine resources.
	dbusName = "org.freedesktop.fwupd"

	// dbusPath is the singleton object path of the fwupd daemon.
	dbusPath = dbus.ObjectPath("/")

	// dbusInterface is where all the fwupd methods and signals are found.
	dbusInterface = "org.freedesktop.fwupd"

	// dbusAddMatch is the standard method to subscribe to signals.
	dbusAddMatch = "org.freedesktop.DBus.AddMatch"

	// updateStateNeedsReboot is the FwupdUpdateState value which tells us
	// an update was deployed and is waiting for a reboot to apply.
	updateStateNeedsReboot = uint32(4)
)

func init() {
	funcs.ModuleRegister(coresys.ModuleName+"/"+ModuleName, RebootNeededFuncName, func() interfaces.Func { return &RebootNeeded{} })
}

// RebootNeeded is a func which returns true when at least one device has a
// firmware update deployed and is waiting for a reboot to apply it. It is the
// equivalent of `fwupdmgr check-reboot-needed`, and it pairs well with the
// fwupd:device and fwupd:system resources, which deploy such updates but never
// reboot for you. This is reactive: it re-evaluates from the fwupd daemon's
// D-Bus signals whenever any device changes state, so no polling is involved.
// On machines without a usable fwupd daemon this returns false, since no
// firmware update can possibly be waiting on a reboot there.
type RebootNeeded struct {
	interfaces.Textarea

	init *interfaces.Init
}

// String returns a simple name for this func. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *RebootNeeded) String() string {
	return RebootNeededFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *RebootNeeded) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *RebootNeeded) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant funcs can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() bool"),
	}
}

// Init runs some startup code for this func.
func (obj *RebootNeeded) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream starts a mainloop and runs Event when it's time to Call() again. We
// subscribe to the fwupd daemon signals, which fire whenever a device changes
// state, notably at the end of an install when a device starts waiting for a
// reboot, and after a reboot when it stops.
func (obj *RebootNeeded) Stream(ctx context.Context) error {
	bus, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		// XXX: After we merge <|> error instead we let the user choose.
		// XXX: Make sure to check if it's an error about fwupd missing.
		// No system bus means no fwupd daemon, and our value is a
		// constant false, so emit it once and settle down forever.
		obj.init.Logf("no system bus: %v", err)
		if err := obj.init.Event(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		}
	}
	defer bus.Close()

	args := fmt.Sprintf("type='signal', interface='%s'", dbusInterface)
	if call := bus.BusObject().Call(dbusAddMatch, 0, args); call.Err != nil {
		return errwrap.Wrapf(call.Err, "failed to subscribe to the fwupd signals")
	}

	signals := make(chan *dbus.Signal, 10) // closed by dbus package
	bus.Signal(signals)

	// streams must generate an initial event on startup
	startChan := make(chan struct{}) // start signal
	close(startChan)                 // kick it off!

	for {
		select {
		case <-startChan:
			startChan = nil // disable

		case _, ok := <-signals:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}

		case <-ctx.Done():
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// Call this func and return the value if it is possible to do so at this time.
func (obj *RebootNeeded) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	needed, err := rebootNeeded(ctx)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not check the device states")
	}
	return &types.BoolValue{V: needed}, nil
}

// rebootNeeded asks the fwupd daemon if any device has an update deployed which
// is waiting for a reboot to apply. If there is no usable daemon at all, then
// the answer is an error-free false.
func rebootNeeded(ctx context.Context) (bool, error) {
	bus, err := util.SystemBusPrivateUsable()
	if err != nil {
		// XXX: See above comment about <|>
		return false, nil // no bus, no fwupd, no pending firmware
	}
	defer bus.Close()

	raw := []map[string]dbus.Variant{}
	call := bus.Object(dbusName, dbusPath).CallWithContext(ctx, dbusInterface+".GetDevices", 0)
	if err := call.Store(&raw); err != nil {
		if _, ok := err.(dbus.Error); ok {
			return false, nil // no daemon to activate, same as no bus
		}
		return false, err
	}

	for _, device := range raw {
		variant, exists := device["UpdateState"]
		if !exists {
			continue
		}
		state, ok := variant.Value().(uint32)
		if !ok {
			continue
		}
		if state == updateStateNeedsReboot {
			return true, nil
		}
	}

	return false, nil
}
