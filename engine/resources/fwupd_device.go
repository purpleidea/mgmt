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
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
)

func init() {
	engine.RegisterResource("fwupd:device", func() engine.Res { return &FwupdDeviceRes{} })
}

// FwupdDeviceRes is a resource that pins the firmware of a single device to an
// exact version, using the fwupd daemon over its D-Bus API. This is the same
// API that the fwupdmgr cli uses, so no external tools are involved, and the
// LVFS (https://fwupd.org/) is simply the default remote that firmware and
// metadata get downloaded from. If the device is at a different version, the
// matching release is looked up in the daemon's metadata, downloaded, checksum
// verified, and handed to the daemon to flash. The daemon independently
// verifies payload signatures. Some devices only apply a deployed update after
// a reboot; this resource does NOT reboot for you, but the
// sys/fwupd.reboot_needed() function can tell you when one would help.
//
// This resource gets real events over D-Bus, so no polling is needed: the
// daemon notifies us when devices appear, disappear or change, and when the
// remote metadata is updated.
type FwupdDeviceRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	init *engine.Init

	// Device is the GUID or the daemon device-id of the device to manage,
	// as shown by the GetDevices API (or `fwupdmgr get-devices`). If not
	// specified, we use the Name.
	Device string `lang:"device" yaml:"device"`

	// Version is the exact firmware version we want on the device. If a
	// release with this version can't be found in the metadata of any
	// enabled remote, this errors.
	Version string `lang:"version" yaml:"version"`

	// AllowDowngrade permits flashing a release that is older than what is
	// currently on the device. Without this, needing a downgrade to
	// converge is an error.
	AllowDowngrade bool `lang:"allow_downgrade" yaml:"allow_downgrade"`
}

// getDevice returns the device identifier we want to manage. If the Device
// field is set, we use that, otherwise we use the Name.
func (obj *FwupdDeviceRes) getDevice() string {
	if obj.Device != "" {
		return obj.Device
	}
	return obj.Name()
}

// Default returns some sensible defaults for this resource.
func (obj *FwupdDeviceRes) Default() engine.Res {
	return &FwupdDeviceRes{}
}

// Validate if the params passed in are valid data.
func (obj *FwupdDeviceRes) Validate() error {
	if obj.getDevice() == "" {
		return fmt.Errorf("need a Device")
	}
	if obj.Version == "" {
		return fmt.Errorf("the Version is empty")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *FwupdDeviceRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *FwupdDeviceRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. It
// subscribes to the fwupd daemon signals, and filters out events about other
// devices where possible.
func (obj *FwupdDeviceRes) Watch(ctx context.Context) error {
	filter := func(signal *dbus.Signal) bool {
		return fwupdSignalMatchesDevice(signal, obj.getDevice())
	}
	return fwupdWatch(ctx, obj.init, nil, filter)
}

// CheckApply method for the FwupdDevice resource.
func (obj *FwupdDeviceRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	client, err := newFwupdClient()
	if err != nil {
		return false, err
	}
	defer client.Close()

	device, err := client.Device(ctx, obj.getDevice())
	if err != nil {
		return false, err
	}

	if device.Version == obj.Version {
		return true, nil // we're at the correct version
	}

	// An update was already deployed and is waiting for a reboot to apply.
	// There is nothing useful we can flash until that happens, so we count
	// this as converged. After the reboot we re-evaluate from scratch.
	if device.UpdateState == fwupdUpdateStateNeedsReboot {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	release, err := obj.findRelease(ctx, client, device)
	if err != nil {
		return false, err
	}
	if release.IsDowngrade() && !obj.AllowDowngrade {
		return false, fmt.Errorf("device %s is at %s, going to %s is a downgrade, set AllowDowngrade if you're sure", device.DeviceID, device.Version, obj.Version)
	}

	options := map[string]bool{
		"allow-older": obj.AllowDowngrade,
	}
	if err := fwupdInstall(ctx, client, device, release, options); err != nil {
		return false, err
	}
	obj.init.Logf("%s: %s -> %s", device.Name, device.Version, release.Version)

	return false, nil
}

// findRelease looks for the release that matches our expected version, in the
// metadata that the daemon has for this device.
func (obj *FwupdDeviceRes) findRelease(ctx context.Context, client *fwupdClient, device *fwupdDevice) (*fwupdRelease, error) {
	releases, err := client.Releases(ctx, device.DeviceID)
	if err != nil {
		return nil, errwrap.Wrapf(err, "no releases for device %s, is the remote enabled and refreshed?", device.DeviceID)
	}
	versions := []string{}
	for _, release := range releases {
		if release.Version == obj.Version {
			return release, nil
		}
		versions = append(versions, release.Version)
	}
	return nil, fmt.Errorf("version %s was not found for device %s, available: %s", obj.Version, device.DeviceID, strings.Join(versions, ", "))
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *FwupdDeviceRes) Cmp(r engine.Res) error {
	// we can only compare FwupdDeviceRes to others of the same resource kind
	res, ok := r.(*FwupdDeviceRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Device != res.Device {
		return fmt.Errorf("the Device differs")
	}
	if obj.Version != res.Version {
		return fmt.Errorf("the Version differs")
	}
	if obj.AllowDowngrade != res.AllowDowngrade {
		return fmt.Errorf("the AllowDowngrade differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *FwupdDeviceRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FwupdDeviceRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*FwupdDeviceRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FwupdDeviceRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FwupdDeviceRes(raw) // restore from indirection with type conversion!
	return nil
}

// AutoEdges returns the AutoEdge interface. Every fwupd:remote resource in the
// graph should happen before us, since any one of them could be the source of
// the release that we want to install.
func (obj *FwupdDeviceRes) AutoEdges(ctx context.Context) (engine.AutoEdge, error) {
	return fwupdConsumerAutoEdges(ctx, obj.Name(), obj.Kind())
}

// FwupdDeviceUID is the UID struct for FwupdDeviceRes.
type FwupdDeviceUID struct {
	engine.BaseUID

	device string // the device guid or daemon device-id
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *FwupdDeviceUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*FwupdDeviceUID)
	if !ok {
		return false
	}
	return obj.device == res.device
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *FwupdDeviceRes) UIDs() []engine.ResUID {
	x := &FwupdDeviceUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		device:  obj.getDevice(),
	}
	return []engine.ResUID{x}
}
