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
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
)

func init() {
	engine.RegisterResource("fwupd:remote", func() engine.Res { return &FwupdRemoteRes{} })
}

// FwupdRemoteRes is a resource that manages one of the firmware sources of the
// fwupd daemon, over its D-Bus API. The name is the remote id, eg: "lvfs", as
// shown by the GetRemotes API (or `fwupdmgr get-remotes`). Besides toggling
// whether a remote is enabled, it can also keep the remote's metadata fresh:
// the daemon never downloads metadata on its own (most distros ship a
// fwupd-refresh.timer systemd unit for that), so with RefreshAge set, this
// resource downloads the metadata and its signature itself (in pure golang) and
// hands them to the daemon for verification and import. When new metadata
// lands, the daemon emits its Changed signal, which wakes up any fwupd:device
// and fwupd:system resources so they can converge onto newly published
// releases.
type FwupdRemoteRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Enabled specifies whether the daemon should use this remote. If it
	// is unspecified, then the enabled state is left as-is, which is
	// useful if you only want metadata refreshing.
	Enabled *bool `lang:"enabled" yaml:"enabled"`

	// RefreshAge is how stale we let this remote's metadata get before we
	// download it again, eg: "24h". It parses as a golang duration. If it
	// is empty, then we never refresh, which is what you want for local
	// (directory kind) remotes such as the built-in fwupd-tests one, whose
	// metadata can't go stale. Refreshing a disabled remote is an error.
	RefreshAge string `lang:"refresh_age" yaml:"refresh_age"`
}

// getRefreshAge parses the RefreshAge field. A zero duration means that the
// refreshing behaviour is disabled.
func (obj *FwupdRemoteRes) getRefreshAge() (time.Duration, error) {
	if obj.RefreshAge == "" {
		return 0, nil
	}
	age, err := time.ParseDuration(obj.RefreshAge)
	if err != nil {
		return 0, errwrap.Wrapf(err, "invalid RefreshAge")
	}
	if age <= 0 {
		return 0, fmt.Errorf("the RefreshAge must be positive")
	}
	return age, nil
}

// Default returns some sensible defaults for this resource.
func (obj *FwupdRemoteRes) Default() engine.Res {
	return &FwupdRemoteRes{}
}

// Validate if the params passed in are valid data.
func (obj *FwupdRemoteRes) Validate() error {
	if obj.Name() == "" {
		return fmt.Errorf("need a remote id as the Name")
	}
	age, err := obj.getRefreshAge()
	if err != nil {
		return err
	}
	if age > 0 && obj.Enabled != nil && !*obj.Enabled {
		return fmt.Errorf("can't refresh a remote we're disabling")
	}
	if age == 0 && obj.Enabled == nil {
		return fmt.Errorf("nothing to manage, set Enabled or RefreshAge")
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *FwupdRemoteRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *FwupdRemoteRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. The
// daemon emits its Changed signal when a remote is reconfigured or when its
// metadata is imported, so we filter out the more chatty per-device signals.
// Since metadata goes stale silently with the passage of time, we also tick
// periodically when RefreshAge is used, so that CheckApply gets a chance to
// re-measure the age. We tick at half the age, which bounds the worst case
// staleness at one and a half times the requested age.
func (obj *FwupdRemoteRes) Watch(ctx context.Context) error {
	age, err := obj.getRefreshAge()
	if err != nil {
		return err // programming error, Validate checked this already
	}
	var tick <-chan time.Time // nil unless we have a time based component
	if age > 0 {
		ticker := time.NewTicker(age / 2) // check often so not as stale
		defer ticker.Stop()
		tick = ticker.C
	}

	filter := func(signal *dbus.Signal) bool {
		return signal.Name == fwupdDBusInterface+".Changed"
	}
	return fwupdWatch(ctx, obj.init, tick, filter)
}

// CheckApply method for the FwupdRemote resource.
func (obj *FwupdRemoteRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	client, err := newFwupdClient()
	if err != nil {
		return false, err
	}
	defer client.Close()

	remotes, err := client.Remotes(ctx)
	if err != nil {
		return false, err
	}
	var remote *fwupdRemote
	for _, r := range remotes {
		if r.RemoteID == obj.Name() {
			remote = r
			break
		}
	}
	if remote == nil {
		return false, fmt.Errorf("remote %s was not found", obj.Name())
	}

	checkOK := true

	if obj.Enabled != nil && remote.Enabled != *obj.Enabled {
		if !apply {
			return false, nil
		}
		value := "false"
		if *obj.Enabled {
			value = "true"
		}
		if err := client.ModifyRemote(ctx, remote.RemoteID, "Enabled", value); err != nil {
			return false, err
		}
		obj.init.Logf("enabled: %s", value)
		remote.Enabled = *obj.Enabled
		checkOK = false
	}

	age, err := obj.getRefreshAge()
	if err != nil {
		return false, err // programming error, Validate checked this already
	}
	// A zero ModificationTime means the metadata was never downloaded, and
	// makes this age astronomical, which refreshes it, as it should.
	if age > 0 && remote.Enabled && time.Since(time.Unix(remote.ModificationTime, 0)) > age {
		if !apply {
			return false, nil
		}
		if err := obj.refresh(ctx, client, remote); err != nil {
			return false, err
		}
		obj.init.Logf("refreshed metadata")
		checkOK = false
	}

	return checkOK, nil
}

// refresh downloads the metadata for this remote along with its detached
// signature, and hands both to the daemon, which verifies and imports them.
func (obj *FwupdRemoteRes) refresh(ctx context.Context, client *fwupdClient, remote *fwupdRemote) error {
	if remote.MetadataURI == "" {
		return fmt.Errorf("remote %s has no metadata uri", remote.RemoteID)
	}

	data, err := fwupdDownload(ctx, remote.MetadataURI)
	if err != nil {
		return err
	}
	defer data.Close()

	signature, err := fwupdDownload(ctx, remote.MetadataURI+fwupdMetadataSignatureSuffix)
	if err != nil {
		return err
	}
	defer signature.Close()

	return client.UpdateMetadata(ctx, remote.RemoteID, data, signature)
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *FwupdRemoteRes) Cmp(r engine.Res) error {
	// we can only compare FwupdRemoteRes to others of the same resource kind
	res, ok := r.(*FwupdRemoteRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if err := engineUtil.BoolPtrCmp(obj.Enabled, res.Enabled); err != nil {
		return errwrap.Wrapf(err, "the Enabled differs")
	}
	if obj.RefreshAge != res.RefreshAge {
		return fmt.Errorf("the RefreshAge differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *FwupdRemoteRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FwupdRemoteRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*FwupdRemoteRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FwupdRemoteRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FwupdRemoteRes(raw) // restore from indirection with type conversion!
	return nil
}
