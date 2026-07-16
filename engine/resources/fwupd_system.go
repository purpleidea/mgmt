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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("fwupd:system", func() engine.Res { return &FwupdSystemRes{} })
}

const (
	// FwupdSystemStateNewest is the only supported state, and means every
	// updatable device should track the newest release that the metadata
	// knows about for it.
	FwupdSystemStateNewest = "newest"
)

// FwupdSystemRes is a resource that keeps the firmware of every updatable
// device on this machine at the newest available release, using the fwupd
// daemon over its D-Bus API. It is the declarative equivalent of running
// `fwupdmgr update` continuously: whenever refreshed metadata reveals a newer
// release for any device, this converges towards it. The name of this resource
// is irrelevant. Devices whose updates only apply after a reboot get their
// update deployed, and the sys/fwupd.reboot_needed() function can then tell you
// that a reboot would help; this resource never reboots for you.
//
// This resource gets real events over D-Bus, so no polling is needed: the
// daemon notifies us when devices appear, disappear or change, and when the
// remote metadata is updated.
type FwupdSystemRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	init *engine.Init

	// State is the desired firmware state for every device. The only valid
	// value is currently "newest".
	State string `lang:"state" yaml:"state"`

	// Exclude is a list of devices (GUID or daemon device-id) to leave
	// alone. Use this for devices you'd rather manage explicitly with
	// fwupd:device, or not at all.
	Exclude []string `lang:"exclude" yaml:"exclude"`
}

// Default returns some sensible defaults for this resource.
func (obj *FwupdSystemRes) Default() engine.Res {
	// TODO: Should we default the state to FwupdSystemStateNewest or not?
	return &FwupdSystemRes{}
}

// Validate if the params passed in are valid data.
func (obj *FwupdSystemRes) Validate() error {
	if obj.State != FwupdSystemStateNewest {
		return fmt.Errorf("the State must be %s", FwupdSystemStateNewest)
	}
	for _, id := range obj.Exclude {
		if id == "" {
			return fmt.Errorf("an Exclude entry is empty")
		}
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *FwupdSystemRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *FwupdSystemRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. It
// subscribes to all the fwupd daemon signals, since any device event or any
// metadata refresh could mean there is a new upgrade for us to apply.
func (obj *FwupdSystemRes) Watch(ctx context.Context) error {
	return fwupdWatch(ctx, obj.init, nil, nil)
}

// isExcluded returns true if this device is on our Exclude list.
func (obj *FwupdSystemRes) isExcluded(device *fwupdDevice) bool {
	for _, id := range obj.Exclude {
		if device.Matches(id) {
			return true
		}
	}
	return false
}

// CheckApply method for the FwupdSystem resource.
func (obj *FwupdSystemRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	client, err := newFwupdClient()
	if err != nil {
		return false, err
	}
	defer client.Close()

	devices, err := client.Devices(ctx)
	if err != nil {
		return false, err
	}

	checkOK := true
	for _, device := range devices {
		if !device.IsUpdatable() || obj.isExcluded(device) {
			continue
		}
		// An update was already deployed onto this device and it is
		// waiting for a reboot; nothing more we can do for it now.
		if device.UpdateState == fwupdUpdateStateNeedsReboot {
			continue
		}

		// The daemon errors on devices with no metadata at all; for a
		// whole-system sweep that simply means nothing to do here.
		releases, err := client.Releases(ctx, device.DeviceID)
		if err != nil {
			if obj.init.Debug {
				obj.init.Logf("skipping %s: %v", device.Name, err)
			}
			continue
		}

		// The daemon returns these newest first, and it also computes
		// the upgrade comparisons for us, since only it knows each
		// device's version format.
		var release *fwupdRelease
		for _, r := range releases {
			if r.IsUpgrade() {
				release = r
				break
			}
		}
		if release == nil {
			continue // this device is already at its newest
		}

		checkOK = false
		if !apply {
			return false, nil
		}

		// upgrades need no options
		if err := fwupdInstall(ctx, client, device, release, nil); err != nil {
			return false, errwrap.Wrapf(err, "could not upgrade %s", device.Name)
		}
		obj.init.Logf("%s: %s -> %s", device.Name, device.Version, release.Version)
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *FwupdSystemRes) Cmp(r engine.Res) error {
	// we can only compare FwupdSystemRes to others of the same resource kind
	res, ok := r.(*FwupdSystemRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if err := engineUtil.StrListCmp(obj.Exclude, res.Exclude); err != nil {
		return errwrap.Wrapf(err, "the Exclude differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *FwupdSystemRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FwupdSystemRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*FwupdSystemRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FwupdSystemRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FwupdSystemRes(raw) // restore from indirection with type conversion!
	return nil
}

// AutoEdges returns the AutoEdge interface. Every fwupd:remote resource in the
// graph should happen before us, since any one of them could be the source of
// the releases that we want to install.
func (obj *FwupdSystemRes) AutoEdges(ctx context.Context) (engine.AutoEdge, error) {
	return fwupdConsumerAutoEdges(ctx, obj.Name(), obj.Kind())
}

// FwupdSystemUID is the UID struct for FwupdSystemRes.
type FwupdSystemUID struct {
	engine.BaseUID
}

// IFF aka if and only if they are equivalent, return true. If not, false. Any
// two fwupd:system resources are equivalent, since the name is irrelevant and
// they all manage the same set of devices.
func (obj *FwupdSystemUID) IFF(uid engine.ResUID) bool {
	_, ok := uid.(*FwupdSystemUID)
	return ok
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *FwupdSystemRes) UIDs() []engine.ResUID {
	x := &FwupdSystemUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
	}
	return []engine.ResUID{x}
}
