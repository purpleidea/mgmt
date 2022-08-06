// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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
	"context"
	"fmt"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/hetznercloud/hcloud-go/hcloud"
)

const (
	// HetznerStateUndefined leaves the state undefined by default. This state
	// is always treated as converged. Changes to other params are only applied
	// when the server is in a state that is compatible with the operations
	// needed to make that change.
	HetznerStateUndefined = ""
	// HetznerStateExists indicates that the server must exist, without
	// differentiation between `off`, `running` or any transient states.
	// If the server was absent, a new server is created in `off` state, with
	// one exception: if the last observed state before a rebuild was `running`
	// or `starting`, rebuildServer will set the new server to `running`.
	HetznerStateExists = "exists"
	// HetznerStateRunning indicates that the server must be powered on. If the
	// server was absent, a new server is created in `running` state.
	HetznerStateRunning = "running"
	// HetznerStateOff indicates that the server must be powered off. If the
	// server was absent, a new server is created in `off` state.
	HetznerStateOff = "off"
	// HetznerStateAbsent indicates that the server must be deleted/absent. If
	// the server already existed, it is deleted. Note that this deletion is
	// always executed if the `absent` state is explicitly specified!
	HetznerStateAbsent = "absent"

	// HetznerAllowRebuildError blocks any server rebuild requests in CheckApply
	// and exits with an error. These rebuild requests occur when other resource
	// params require a destructive rebuild to reach resource convergence. The
	// error option is used by default to prevent unexpected server deletions.
	HetznerAllowRebuildError = ""
	// HetznerAllowRebuildIgnore blocks any server rebuild requests in
	// CheckApply, but does not throw any errors. Instead, CheckApply must skip
	// this rebuild, and continue further steps if possible. Use this option to
	// prevent unexpected server deletions, without disrupting the mcl script.
	HetznerAllowRebuildIgnore = "ignore"
	// HetznerAllowRebuildIfNeeded allows server rebuilds within CheckApply.
	// This is needed when the specified serverspecs are not (yet) aligned with
	// the active instance. Use this option only if you are sure that you are
	// not destroying any critical data or services!
	HetznerAllowRebuildIfNeeded = "ifneeded"

	// HetznerServerRescueDisabled disables rescue mode by default.
	HetznerServerRescueDisabled = ""
	// HetznerServerRescueTypeLinux32 is used to enable rescue mode with a
	// linux32 image type.
	HetznerServerRescueTypeLinux32 = "linux32"
	// HetznerServerRescueTypeLinux64 is used to enable rescue mode with a
	// linux64 image type.
	HetznerServerRescueTypeLinux64 = "linux64"
	// HetznerServerRescueTypeFreeBSD64 is used to enable rescue mode with a
	// freebsd64 image type.
	HetznerServerRescueTypeFreeBSD64 = "freebsd64"

	// HetznerPollLimit sets a lower limit on polling interval in seconds.
	// Since the Hetzner API supports requests at up to 3600 requests per hour,
	// this limit is set to prevent rate limit errors in long term operation.
	// NOTE: polling the same Hetzner project from multiple clients will require
	// a larger polling interval to prevent the same rate limit error, since
	// these requests all add to the query count of their shared project. It is
	// recommended to use a polling interval of at least N seconds, with N the
	// number of active hetzner:vm instances of the same project.
	// NOTE: high rates of change to other params will require additional API
	// queries at CheckApply. Increase the polling interval again to prevent
	// rate limit errors if frequent updates are expected.
	HetznerPollLimit = 1

	// HetznerWaitIntervalLimit sets a lower limit on wait intervals in seconds.
	// High request rates are allowed, but risk causing rate limit errors.
	HetznerWaitIntervalLimit = 0

	// HetznerWaitIntervalDefault sets a default wait interval in seconds.
	// NOTE: use larger intervals when using many resources under the same
	// Hetzner project, or when expecting consistently high rates of change to
	// other resource parameters.
	HetznerWaitIntervalDefault = 5

	// HetznerWaitTimeoutDefault sets a default timeout limit in seconds.
	HetznerWaitTimeoutDefault = 60 * 5
)

func init() {
	engine.RegisterResource("hetzner:vm", func() engine.Res { return &HetznerVMRes{} })
}

// HetznerVMRes is a Hetzner cloud resource (1). It connects with the cloud API
// using the hcloud-go package provided by Hetzner. The API token for a new
// project must be generated manually, via the cloud console (2), before this
// resource can establish a connection with the API. One Hetzner resource
// represents one server instance, and multiple instances can be registered
// under the same project. A resource in the `absent` state only exists as a
// local mcl struct, and does not exist as server instance on Hetzner's side.
// NOTE: the Hetzner cloud console must be used to create a new project,
// generate the corresponding API token, and initialize the desired SSH keys.
// All registered SSH keys are used when creating a server, and a subset of
// those can be enabled for rescue mode via the `serverrescuekeys` param.
// NOTE: complete and up-to-date serverconfig options must be requested from the
// Hetzner API, but hcloud-go-getopts (3) provides a static reference.
// NOTE: this resources requires polling, via the `Meta:poll` param. The Hetzner
// API imposes a maximum rate of 3600 requests per hour that must be taken into
// account for intensive and/or long term operations. When running N hetzner:vm
// resources under the same Hetzner project, it is recommended to use a polling
// interval of at least N seconds. High rates of change to other params will
// require additional API requests at CheckApply. When frequent param updates
// are expected for long term operations, it is reommended to increase the
// polling interval again to prevent rate limit errors.
// NOTE: running multiple concurrent mcl scripts on the same resource might
// cause unexpected behavior in the API or the resource state. Use with care.
// TODO: build tests for hetzner:vm? But hcloud-go has no mocking package.
// 1) https://docs.hetzner.cloud/
// 2) https://console.hetzner.cloud/
// 3) https://github.com/jefmasereel/hcloud-go-getopts
type HetznerVMRes struct {
	traits.Base
	init *engine.Init

	// APIToken specifies the unique API token corresponding to a Hetzner
	// project. Keep this token private! It provides full access to this
	// project, so a leaked token will be vulnerable to abuse. Read it from
	// a local file or the mgmt deploy, or provide it directly as a string.
	// NOTE: It must be generated manually via https://console.hetzner.cloud/.
	// NOTE: This token is usually a 64 character alphanumeric string.
	APIToken string `lang:"apitoken"`

	// State specifies the desired state of the server instance. The supported
	// options are `` (undefined), `absent`, `exists`, `off` and `running`.
	// HetznerStateUndefined (``) leaves the state undefined by default.
	// HetznerStateExists (`exists`) indicates that the server must exist.
	// HetznerStateAbsent (`absent`) indicates that the server must not exist.
	// HetznerStateRunning (`running`) tells the server it must be powered on.
	// HetznerStateOff (`off`) tells the server it must be powered off.
	// NOTE: any other inputs will not pass Validate and result in an error.
	// NOTE: setting the state of a live server to `absent` will delete all data
	// and services that are located on that instance! Use with caution.
	State string `lang:"state"`

	// AllowRebuild provides flexible protection against unexpected server
	// rebuilds. Any changes to the `servertype`, `datacenter` or `image` params
	// require a destructive rebuild, which deletes all data on that server.
	// The user must explicitly allow these operations with AllowRebuild.
	// Choose from three options: `ifneeded` allows all rebuilds that are needed
	// by CheckApply to meet the specified params. `ignore` disables these
	// rebuilds, but continues without error. The default option (``) disables
	// always returns an error when CheckApply requests a rebuild.
	// NOTE: Soft updates related to power and rescue mode are always allowed,
	// because they are only required for explicit changes to resource fields.
	// TODO: add AllowReboot if any indirect poweroffs are ever implemented.
	AllowRebuild string `lang:"allowrebuild"`

	// ServerType determines the machine type as defined by Hetzner. A complete
	// and up-to-date list of options must be requested from the Hetzner API,
	// but hcloud-go-getopts (url) provides a static reference. Basic servertype
	// options include `cx11`, `cx21`, `cx31` etc.
	// NOTE: make sure to check the price of the selected servertype! The listed
	// examples are usually very cheap, but never free. Price and availability
	// can also be dependent on the selected datacenter.
	// https://github.com/JefMasereel/hcloud-go-getopts/
	// TODO: set some kind of cost-based protection policy?
	ServerType string `lang:"servertype"`

	// Datacenter determines where the resource is hosted.  A complete and
	// up-to-date list of options must be requested from the Hetzner API, but
	// hcloud-go-getopts (url) provides a static reference. The datacenter
	// options include `nbg1-dc3`, `fsn1-dc14`, `hel1-dc2` etc.
	// https://github.com/JefMasereel/hcloud-go-getopts/
	Datacenter string `lang:"datacenter"`

	// Image determines the operating system to be installed. A complete and
	// up-to-date list of options must be requested from the Hetzner API, but
	// hcloud-go-getopts (url) provides a static reference. The image type
	// options include `centos-7`, `ubuntu-18.04`, `debian-10` etc.
	// https://github.com/JefMasereel/hcloud-go-getopts/
	Image string `lang:"image"`

	// UserData can be used to run commands on the server instance at creation.
	// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html.
	UserData string `lang:"userdata"`

	// ServerRescueMode specifies the image type used when enabling rescue mode.
	// The supported image types are `linux32`, `linux64` and `freebsd64`.
	// Alternatively, leave this string empty to disable rescue mode (default).
	// Other input values will not pass Validate and result in an error.
	// NOTE: rescue mode can not be enabled if the server is absent.
	// NOTE: Rescue mode can be used to log into the server over SSH and access
	// the disks when the normal OS has trouble booting on its own.
	ServerRescueMode string `lang:"serverrescuemode"`

	// ServerRescueSSHKeys can be used to select a subset of keys that should be
	// enabled for rescue mode operations over SSH. From all SSH keys known to
	// the project client, choose a subset of keys by name, as an array of
	// strings. New keys must first be added manually via the cloud console.
	// An error is thrown if a given keyname is not recognized by the client.
	// NOTE: live changes to this keylist while rescue mode is already enabled
	// are not (yet) detected or applied by CheckApply.
	// TODO: improve ssh key handling at checkApplyRescueMode and serverRebuild.
	ServerRescueSSHKeys []string `lang:"serverrescuekeys"`

	// WaitInterval is the interval in seconds that is used when waiting for
	// transient states to converge between intermediate operations. A zero
	// value causes the waiter to run without delays (burst requests). Although
	// such burst requests are allowed, it is recommended to use a wait interval
	// that keeps the total request rate under 3600 requests per hour. Take
	// these factors into account: polling rate `Meta:poll`, number of active
	// resources under the same Hetzner project, and the expected rate of param
	// updates. This will help to prevent rate limit errors.
	WaitInterval uint32 `lang:"waitinterval"`

	// WaitTimeout will cancel wait loops if they do not exit cleanly before
	// the expected time in seconds, in order to detect defective loops and
	// avoid unnecessary consumption of computational resources.
	WaitTimeout uint32 `lang:"waittimeout"`

	// client is required for hcloud-go to interact with the Hetzner API.
	client *hcloud.Client

	// server is a local copy of the server object returned by hcloud-go. If
	// this is nil, the server is considered to be absent. Otherwise, this
	// struct describes the properties of the server instance as registered with
	// Hetzner at the time of the update request.
	server *hcloud.Server

	// serverconfig is a local copy of the serverCreateOpts struct generated
	// with hcloud-go. This struct is dependent on the ServerType, Datacenter,
	// Image and State params. These must be chosen from the valid options
	// provided by Hetzner, see details on https://docs.hetzner.cloud/.
	serverconfig hcloud.ServerCreateOpts

	// lastObservedState is a local copy of the last observed state of the
	// resource. This is used to determine the startAfterCreate option during
	// server rebuilds when the state is `` (undefined).
	lastObservedState hcloud.ServerStatus

	// rescueKeys is a local copy of the array of SSH key values to be enabled
	// in rescue mode, after formatting for direct use with hcloud-go.
	rescueKeys []*hcloud.SSHKey

	// rescueImage is a local copy of the image type used when rescue mode was
	// enabled the last time, to give checkapplyrescuemode a static reference.
	rescueImage hcloud.ServerRescueType
}

// Default returns some conservative defaults for this resource.
func (obj *HetznerVMRes) Default() engine.Res {
	return &HetznerVMRes{
		State:        HetznerStateUndefined,
		AllowRebuild: HetznerAllowRebuildError,
		WaitInterval: HetznerWaitIntervalDefault,
		WaitTimeout:  HetznerWaitTimeoutDefault,
	}
}

// Validate if the given param values are valid.
func (obj *HetznerVMRes) Validate() error {

	// check for empty token
	if obj.APIToken == "" {
		return fmt.Errorf("empty token string")
	}

	// validate state param
	switch obj.State {
	case HetznerStateRunning, HetznerStateOff, HetznerStateAbsent:
		// Valid: the server is in a well defined steady state.
	case HetznerStateExists:
		// Valid: the server exists (on, off or transient state).
	case HetznerStateUndefined:
		// Valid: the server state is left undefined (default).
	default:
		return fmt.Errorf("invalid state: %s", obj.State)
	}

	// validate allowrebuild
	switch obj.AllowRebuild {
	case HetznerAllowRebuildError, HetznerAllowRebuildIgnore, HetznerAllowRebuildIfNeeded:
		// ok
	default:
		return fmt.Errorf("invalid allowrebuild: %s", obj.AllowRebuild)
	}

	// validate rescue mode parameters
	switch obj.ServerRescueMode {
	case HetznerServerRescueTypeLinux32, HetznerServerRescueTypeLinux64, HetznerServerRescueTypeFreeBSD64:
		// valid options for rescue mode image
	case HetznerServerRescueDisabled:
		// valid option to disable rescue mode
	default:
		return fmt.Errorf("invalid serverrescuemode: %s", obj.ServerRescueMode)
	}

	// validate time params
	if obj.MetaParams().Poll < HetznerPollLimit {
		return fmt.Errorf("invalid polling interval (minimum %d s)", HetznerPollLimit)
	}
	if obj.WaitInterval < HetznerWaitIntervalLimit {
		return fmt.Errorf("invalid wait interval (minimum %d)", HetznerWaitIntervalLimit)
	}

	return nil
}

// Init runs some startup code for this resource: initialize hcloud-go client,
// and then build some internal flags from the given public fields.
func (obj *HetznerVMRes) Init(init *engine.Init) error {

	// save init struct
	obj.init = init

	// initialize hcloud-go client
	obj.client = hcloud.NewClient(
		hcloud.WithToken(obj.APIToken),
		hcloud.WithApplication(obj.init.Program, obj.init.Version),
		// TODO: hcloud.WithEndpoint(),
		// TODO: hcloud.WithDebugWriter(),
	)

	// warn user about AllowRebuild setting
	switch obj.AllowRebuild {
	case HetznerAllowRebuildError:
		obj.init.Logf("warning: server rebuild requests will be blocked with error")
	case HetznerAllowRebuildIgnore:
		obj.init.Logf("warning: server rebuild requests will be skipped without error")
	case HetznerAllowRebuildIfNeeded:
		obj.init.Logf("warning: server rebuild requests will be applied without error")
	}

	// warn user about late serverconfig validation
	obj.init.Logf("warning: serverconfig options will only be validated during checkapply")

	// warn user about timing requirements
	obj.init.Logf("warning: Meta:poll must always be greater or equal than %d seconds", HetznerPollLimit)
	obj.init.Logf("warning: waitinterval must always be greater or equal than %d seconds", HetznerWaitIntervalLimit)

	return nil
}

// Close deletes the authentication info before closing the resource.
func (obj *HetznerVMRes) Close() error {
	obj.APIToken = ""
	obj.client = nil
	return nil
}

// Watch is not implemented for this resource, since the Hetzner API does not
// provide any event streams. Instead, always use polling.
// NOTE: HetznerPollLimit sets an explicit minimum on the polling interval.
func (obj *HetznerVMRes) Watch() error {
	return fmt.Errorf("invalid Watch call: requires poll metaparam")
}

// CheckApply checks the resource state and determines what needs to happen for
// the HetznerVM resource to converge. It only applies the necessary changes if
// the bool apply is true. If the resource requires changes, CheckApply returns
// false regardless of the apply value, true otherwise. Any errors that might
// occur are wrapped and returned.
// NOTE: all functions that push changes to the Hetzner instance run a waitUntil
// call with the appropriate exit condition before returning, such that the
// requested operation is confirmed before continuing. This ensures that the
// `server` struct always contains up-to-date info of the live instance.
// NOTE: this last assumption might still fail in case the same resource
// instance is managed by multiple running mgmt instances!
// TODO: possible to ensure safe concurrency?
func (obj *HetznerVMRes) CheckApply(apply bool) (bool, error) {
	checkOK := true
	ctx := context.TODO()
	// Request up-to-date server info from the API.
	if err := obj.getServerUpdate(ctx); err != nil {
		return false, errwrap.Wrapf(err, "getServerUpdate failed")
	}
	// Try to get the server in the correct state (if it is not already there).
	// NOTE: in case of undefined state, this always returns (true, nil).
	if c, err := obj.checkApplyServerState(ctx, apply); err != nil {
		return false, errwrap.Wrapf(err, "checkApplyServerState failed")
	} else if !c {
		checkOK = false
	}
	// If the intended state was not reached, exit here.
	// NOTE: this prevents unnecessary checks and operations.
	// NOTE: undefined state will pass! Further steps are applied if possible.
	if stateOK, err := obj.serverStateConverged(); err != nil {
		return false, errwrap.Wrapf(err, "serverStateConverged failed")
	} else if !stateOK {
		return false, nil
	}
	// Changes in cpu, location and/or image require a server rebuild.
	// NOTE: these changes are only applied if the server exists.
	if c, err := obj.checkApplyServerRebuild(ctx, apply); err != nil {
		return false, errwrap.Wrapf(err, "checkApplyServerRebuild failed")
	} else if !c {
		checkOK = false
	}
	// Changes in rescue mode can be made without a destructive rebuild.
	// NOTE: these changes are only applied if the server is running.
	if c, err := obj.checkApplyRescueMode(ctx, apply); err != nil {
		return false, errwrap.Wrapf(err, "checkApplyRescueMode failed")
	} else if !c {
		checkOK = false
	}
	return checkOK, nil
}

// checkApplyServerState tries to get the server in the correct state. If it is
// already there (converged), no changes are applied. In case of the undefined
// state, this function immediately returns (true, nil). Otherwise, it powers
// the server on/off, creates a new instance, or deletes the existing one as
// needed to reach the specified state.
// NOTE: the output arguments follow the rules of CheckApply: If the resource
// requires changes, CheckApply returns false regardless of the apply value,
// true otherwise. Any errors that might occur are wrapped and returned.
func (obj *HetznerVMRes) checkApplyServerState(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("checkApplyServerState(apply: %t)", apply)
	}
	// Exit immediately if the server state is undefined.
	if obj.State == HetznerStateUndefined {
		return true, nil
	}
	// Make sure the server exists as intended before further checks.
	serverCreationRequired := false
	if obj.server == nil {
		// The server doesn't exist as intended (state = `absent`).
		if obj.State == HetznerStateAbsent {
			return true, nil
		}
		// Otherwise, the server should exist, but doesn't (yet).
		serverCreationRequired = true
		if !apply {
			return false, nil
		}
		// Request the creation of a new server.
		if err := obj.createServer(ctx); err != nil {
			return false, errwrap.Wrapf(err, "createServer failed")
		}
	}
	// If the resource only needs to exist, exit here.
	if obj.State == HetznerStateExists {
		return !serverCreationRequired, nil
	}
	// Otherwise, continue if/once the resource is in a steady state.
	if err := obj.waitUntil(ctx, obj.serverInSteadyState); err != nil {
		return false, errwrap.Wrapf(err, "waitUntil(serverInSteadyState) exited early")
	}
	// If the state has already converged, exit here.
	stateConverged, err := obj.serverStateConverged()
	if err != nil {
		return false, errwrap.Wrapf(err, "serverStateConverged failed")
	}
	if checkOK := (stateConverged && !serverCreationRequired); stateConverged {
		return checkOK, nil
	}
	// Otherwise, the server is in a steady state, but not the right one.
	if !apply {
		return false, nil
	}
	// Apply the necessary changes to get to the specified state.
	switch obj.State {
	case HetznerStateRunning:
		if err := obj.powerServerOn(ctx); err != nil {
			return false, errwrap.Wrapf(err, "powerServerOn failed")
		}
	case HetznerStateOff:
		if err := obj.powerServerOff(ctx); err != nil {
			return false, errwrap.Wrapf(err, "powerServerOff failed")
		}
	case HetznerStateAbsent:
		if err := obj.deleteServer(ctx); err != nil {
			return false, errwrap.Wrapf(err, "deleteServer failed")
		}
	default:
		return false, fmt.Errorf("invalid state: %s", obj.State)
	}
	// All required state changes were applied without error.
	return false, nil
}

// checkApplyServerRebuild checks the servertype, datacenter and image values of
// the live instance, and tries to rebuild the server when that is required to
// match the specified params.
// NOTE: AllowRebuild protects the user against unexpected server deletions.
// NOTE: the output arguments follow the rules of CheckApply: If the resource
// requires changes, CheckApply returns false regardless of the apply value,
// true otherwise. Any errors that might occur are wrapped and returned.
func (obj *HetznerVMRes) checkApplyServerRebuild(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("checkApplyServerRebuild(apply: %t)", apply)
	}
	// Exit immediately if the server does not exist.
	if obj.server == nil {
		return false, nil
	}
	// Compare ServerType, Datacenter and Image params.
	specsOK, err := obj.cmpServerSpecs()
	if err != nil {
		return false, errwrap.Wrapf(err, "cmpServerSpecs failed")
	}
	if specsOK {
		return true, nil
	}
	if !apply {
		return false, nil
	}
	// Rebuild the server to meet specs (if AllowRebuild passes).
	// NOTE: if `undefined`, this tries to match the last observed state.
	if err := obj.rebuildServer(ctx); err != nil {
		return false, errwrap.Wrapf(err, "rebuildServer failed")
	}
	return false, nil
}

// checkApplyRescueMode checks if the rescue mode is enabled (or disabled) as
// intended, and tries to disable (or enable) the rescue mode if needed to meet
// the specified parameters. When enabling rescue mode, the SSH keys specified
// by ServerRescueSSHKeys are validated and enabled for rescue login over SSH.
// NOTE: rescue mode changes require steady state (`off` or `running`).
// NOTE: the output arguments follow the rules of CheckApply: If the resource
// requires changes, CheckApply returns false regardless of the apply value,
// true otherwise. Any errors that might occur are wrapped and returned.
// NOTE: switching image type in ServerRescueMode triggers this checkapply, but
// dynamic changes to the SSH keys are not yet supported.
// TODO: add `undefined` option for HetznerServerRescueMode? default?
// TODO: add support for rescue login via root password?
func (obj *HetznerVMRes) checkApplyRescueMode(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("checkApplyRescueMode(apply: %t)", apply)
	}
	// Exit immediately if the server is absent.
	// NOTE: an absent server is treated as rescue mode `disabled`.
	if obj.server == nil {
		if obj.ServerRescueMode == HetznerServerRescueDisabled {
			return true, nil
		}
		return false, nil
	}
	// Exit if the server is not in a steady state (`running` or `off`).
	// NOTE: otherwise the `server is locked` when trying to enable or disable.
	stateOK, err := obj.serverInSteadyState()
	if err != nil {
		return false, errwrap.Wrapf(err, "serverInSteadyState failed")
	}
	if !stateOK {
		return false, nil
	}
	// Exit if rescue mode is already in the intended configuration.
	// TODO: add check for ssh keys? Only checking rescueImage.
	rescueModeOK, err := obj.rescueModeConverged()
	if err != nil {
		return false, errwrap.Wrapf(err, "rescueModeConverged failed")
	}
	if rescueModeOK {
		return true, nil
	}
	if !apply {
		return false, nil
	}
	// Disable rescue mode to match specs, or to re-enable with new image type.
	if err := obj.disableRescueMode(ctx); err != nil {
		return false, errwrap.Wrapf(err, "disableRescueMode failed")
	}
	// Enable rescue mode if specified.
	if obj.ServerRescueMode != HetznerServerRescueDisabled {
		if err := obj.enableRescueMode(ctx); err != nil {
			return false, errwrap.Wrapf(err, "enableRescueMode failed")
		}
	}
	return false, nil
}

// getServerUpdate pings the Hetzner API for up-to-date server info.
// NOTE: if obj.server is nil, the server is considered to be in `absent` state.
func (obj *HetznerVMRes) getServerUpdate(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("getServerUpdate()")
	}
	server, _, err := obj.client.Server.GetByName(ctx, obj.Name())
	if err != nil {
		return errwrap.Wrapf(err, "failed serverupdate request")
	}
	obj.server = server
	return nil
}

// Cmp compares two resource structs. Returns nil if the comparison holds true,
// otherwise an error is thrown to identify the difference.
func (obj *HetznerVMRes) Cmp(r engine.Res) error {
	// check if empty
	if obj == nil && r == nil {
		return nil
	}
	if (obj == nil) != (r == nil) {
		return fmt.Errorf("one resource is empty")
	}
	// compare types
	res, ok := r.(*HetznerVMRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	// compare resource fields
	if obj.APIToken != res.APIToken {
		return fmt.Errorf("apitoken differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("state differs")
	}
	if obj.AllowRebuild != res.AllowRebuild {
		return fmt.Errorf("allowrebuild differs")
	}
	if obj.ServerType != res.ServerType {
		return fmt.Errorf("servertype differs")
	}
	if obj.Datacenter != res.Datacenter {
		return fmt.Errorf("datacenter differs")
	}
	if obj.Image != res.Image {
		return fmt.Errorf("image differs")
	}
	if obj.UserData != res.UserData {
		return fmt.Errorf("userdata differs")
	}
	if obj.ServerRescueMode != res.ServerRescueMode {
		return fmt.Errorf("serverrescuemode differs")
	}
	// TODO: more robust comparison of keylists
	for i, key := range obj.ServerRescueSSHKeys {
		if key != res.ServerRescueSSHKeys[i] {
			return fmt.Errorf("serverrescuekeys differ")
		}
	}
	if obj.WaitInterval != res.WaitInterval {
		return fmt.Errorf("waitinterval differs")
	}
	if obj.WaitTimeout != res.WaitTimeout {
		return fmt.Errorf("waittimeout differs")
	}
	return nil
}

// cmpServerSpecs compares the server specifications between the local mcl
// struct HetznerVMRes and the corresponding server instance. Returns true if
// ServerType, Datacenter and Image match. Returns an error if the server is
// absent.
func (obj *HetznerVMRes) cmpServerSpecs() (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("cmpServerSpecs()")
	}
	if obj.server == nil {
		return false, fmt.Errorf("server is unavailable")
	}
	if obj.ServerType != obj.server.ServerType.Name {
		return false, nil
	}
	if obj.Datacenter != obj.server.Datacenter.Name {
		return false, nil
	}
	if obj.Image != obj.server.Image.Name {
		return false, nil
	}
	return true, nil
}

// powerServerOn requests a poweron for the specified server, then waits until
// the new `running` state is confirmed. Returns an error if the specified
// server is absent, or if waitUntil exits early due to timeout, context
// cancellation or another error.
func (obj *HetznerVMRes) powerServerOn(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("powerServerOn()")
	}
	if obj.server == nil {
		return fmt.Errorf("server is unavailable")
	}
	if _, _, err := obj.client.Server.Poweron(ctx, obj.server); err != nil {
		return errwrap.Wrapf(err, "client.Server.Poweron failed")
	}
	// Wait until the poweron is confirmed, error otherwise.
	if err := obj.waitUntil(ctx, obj.serverStateIs(HetznerStateRunning)); err != nil {
		return errwrap.Wrapf(err, "waitUntil(serverStateIs(Running)) exited early")
	}
	return nil
}

// powerServerOff requests a poweroff for the specified server, then waits until
// the new `off` state is confirmed. Returns an error if the specified server is
// absent, or if waitUntil exits early due to timeout, context cancellation or
// another error.
func (obj *HetznerVMRes) powerServerOff(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("powerServerOff()")
	}
	if obj.server == nil {
		return fmt.Errorf("server is unavailable")
	}
	if _, _, err := obj.client.Server.Poweroff(ctx, obj.server); err != nil {
		return errwrap.Wrapf(err, "client.Server.Poweroff failed")
	}
	// Wait until the poweroff is confirmed, error otherwise.
	if err := obj.waitUntil(ctx, obj.serverStateIs(HetznerStateOff)); err != nil {
		return errwrap.Wrapf(err, "waitUntil(serverStateIs(Off)) exited early")
	}
	return nil
}

// createServer checks if the servername does not already exists, builds the
// serverconfig in hcloud-go format from resource params, requests a server
// creation with that configuration, and waits until the creation is confirmed.
// Errors occur when the server exists already, the client fails, or the wait
// step exits early due context cancellation, client failure or timeout.
// NOTE: the startAfterCreate option is used to reach `running` state faster for
// two cases. When the state is specified as `running`, or when the state is ``
// (undefined) and the last observed serverstatus was `running` or `starting`.
func (obj *HetznerVMRes) createServer(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("createServer()")
	}
	if obj.server != nil {
		return fmt.Errorf("server already exists")
	}
	if err := obj.getServerConfig(ctx); err != nil {
		return errwrap.Wrapf(err, "getServerConfig failed")
	}
	if obj.serverconfig.SSHKeys == nil {
		obj.init.Logf("warning: no ssh keys registered for server creation")
	}
	if _, _, err := obj.client.Server.Create(ctx, obj.serverconfig); err != nil {
		return errwrap.Wrapf(err, "client.server.create failed")
	}
	if err := obj.waitUntil(ctx, obj.serverStateIs(HetznerStateExists)); err != nil {
		return errwrap.Wrapf(err, "waitUntil(serverExists) exited early")
	}
	return nil
}

// deleteServer checks if the server is available from the client, requests a
// server deletion from the API, waits for confirmation and then returns. It
// returns an error when the server is already absent or something fails.
// Context cancellation allows a clean exit when needed.
// NOTE: a direct deleteServer call is never blocked. Use with caution.
func (obj *HetznerVMRes) deleteServer(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("deleteServer()")
	}
	if obj.server == nil {
		return fmt.Errorf("server is already unavailable")
	}
	if _, err := obj.client.Server.Delete(ctx, obj.server); err != nil {
		return errwrap.Wrapf(err, "client.server.delete failed")
	}
	if err := obj.waitUntil(ctx, obj.serverStateIs(HetznerStateAbsent)); err != nil {
		return errwrap.Wrapf(err, "waitUntil(serverStateIs(Absent)) exited early")
	}
	return nil
}

// rebuildServer deletes the current server instance and creates a new one, in
// accordance with the provided resource specifications. If the state is ``
// (undefined), this function tries to match the last observed state of the live
// instance. If that last observed state is `absent`, rebuild returns nil
// without creating a new server. Otherwise, the server must exist, and absence
// will result in an error.
// NOTE: AllowRebuild protects the user against unexpected server deletions:
// AllowRebuildError blocks deletion with error, AllowRebuildIgnore blocks
// deletion without error, and HetznerAllowRebuildIfNeeded allows deletion.
func (obj *HetznerVMRes) rebuildServer(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("rebuildServer()")
	}
	// Exit immediately if the server is absent.
	if obj.server == nil {
		// Leave undefined server as is, rebuild if/when it becomes available.
		if obj.State == HetznerStateUndefined {
			return nil
		}
		// Otherwise there is no reason to allow absence.
		return fmt.Errorf("server is unavailable")
	}
	// Exit if rebuild is not allowed.
	if obj.AllowRebuild == HetznerAllowRebuildError {
		// exit without applying changes, throw error
		return fmt.Errorf("server rebuild blocked, requires deletion")
	}
	if obj.AllowRebuild == HetznerAllowRebuildIgnore {
		// exit without applying changes, but no error
		return nil
	}
	// If the server exists but is undefined, save a temporary copy of the last
	// observed state. This will be used to create the appropriate serverconfig.
	if obj.State == HetznerStateUndefined {
		obj.lastObservedState = obj.server.Status
	}
	// Rebuild.
	if err := obj.deleteServer(ctx); err != nil {
		return errwrap.Wrapf(err, "deleteServer failed")
	}
	if err := obj.createServer(ctx); err != nil {
		return errwrap.Wrapf(err, "createServer failed")
	}
	return nil
}

// getServerConfig builds a serverconfig struct based on the given resource
// parameters, such that this serverconfig can be used to create a new server
// instance that matches the specified parameters. Errors can occur if the
// params used to construct serverconfig contain invalid arguments, or if the
// client fails.
// NOTE: the startAfterCreate option is used to reach `running` state faster for
// two cases. When the state is specified as `running`, or when the state is ``
// (undefined) and the last observed serverstatus was `running` or `starting`.
// TODO: add option to define Location xor Datacenter (never both!).
func (obj *HetznerVMRes) getServerConfig(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("getServerConfig()")
	}
	// default, volumes not supported (yet)
	// TODO: add support for volume selection?
	automount := false
	// poweron at creation to reach `running` faster
	startAfterCreate := false
	if obj.State == HetznerStateRunning {
		startAfterCreate = true
	}
	if obj.State == HetznerStateUndefined {
		switch obj.lastObservedState {
		case hcloud.ServerStatusRunning, hcloud.ServerStatusStarting:
			startAfterCreate = true
		default:
			// leave powered off
		}
	}
	// collect serverconfig elements
	serverType, _, err := obj.client.ServerType.GetByName(ctx, obj.ServerType)
	if err != nil {
		return errwrap.Wrapf(err, "failed to collect ServerType struct")
	}
	image, _, err := obj.client.Image.GetByName(ctx, obj.Image)
	if err != nil {
		return errwrap.Wrapf(err, "failed to collect Image struct")
	}
	datacenter, _, err := obj.client.Datacenter.GetByName(ctx, obj.Datacenter)
	if err != nil {
		return errwrap.Wrapf(err, "failed to collect Datacenter struct")
	}
	// TODO: add more flexible key selection
	keylist, err := obj.client.SSHKey.All(ctx)
	if err != nil {
		return errwrap.Wrapf(err, "failed to collect SSHKey array")
	}
	// NOTE: GetByName will return nil in case the given name is unknown.
	if serverType == nil {
		return fmt.Errorf("unknown servertype: %s", obj.ServerType)
	}
	if image == nil {
		return fmt.Errorf("unknown image: %s", obj.Image)
	}
	if datacenter == nil {
		return fmt.Errorf("unknown datacenter: %s", obj.Datacenter)
	}
	// build serverconfig from given specs & defaults
	obj.serverconfig = hcloud.ServerCreateOpts{
		Name:             obj.Name(),        // string
		ServerType:       serverType,        // *ServerType
		Image:            image,             // *Image
		SSHKeys:          keylist,           // []*SSHKey
		Location:         nil,               // *Location
		Datacenter:       datacenter,        // *Datacenter
		UserData:         obj.UserData,      // string
		StartAfterCreate: &startAfterCreate, // *bool
		Labels:           nil,               // map[string]string
		Automount:        &automount,        // *bool
		Volumes:          nil,               // []*Volume
		Networks:         nil,               // []*Network
		Firewalls:        nil,               // []*ServerCreateFirewall
		PlacementGroup:   nil,               // *PlacementGroup
	}
	// hcloud-go provides basic validation, but this can still miss problems!
	// TODO: add tests? If issues come up, add checks to Validate.
	if err := hcloud.ServerCreateOpts.Validate(obj.serverconfig); err != nil {
		return errwrap.Wrapf(err, "invalid serverconfig")
	}
	return nil
}

// enableRescueMode tries to enable rescue mode for the specified server, then
// waits until the operation is confirmed. Returns an error if the server is not
// in steady state, if an intermediate API request fails, if waitUntil exits
// early or in case of context cancellation.
// NOTE: the EnableRescue request requires steady state (`off` or `running`).
func (obj *HetznerVMRes) enableRescueMode(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("enableRescueMode()")
	}
	// Exit immediately if the server is absent.
	if obj.server == nil {
		return fmt.Errorf("server is unavailable")
	}
	// Exit if rescue mode is already enabled.
	if obj.server.RescueEnabled {
		return nil
	}
	// Exit if the server is not in a steady state (`running` or `off`).
	// NOTE: otherwise the `server is locked` when trying to enable.
	stateOK, err := obj.serverInSteadyState()
	if err != nil {
		return errwrap.Wrapf(err, "serverInSteadyState failed")
	}
	if !stateOK {
		return fmt.Errorf("state must be `running` or `off` (now: %s)", obj.server.Status)
	}
	// Format rescueImage and rescueKeys, then enable rescue mode.
	// NOTE: rescueImage and rescueKeys also provide a checkapply reference.
	obj.rescueImage = hcloud.ServerRescueType(obj.ServerRescueMode)
	if err := obj.getRescueKeys(ctx); err != nil {
		return errwrap.Wrapf(err, "getRescueKeys failed")
	}
	opts := hcloud.ServerEnableRescueOpts{
		Type:    obj.rescueImage,
		SSHKeys: obj.rescueKeys,
	}
	if _, _, err := obj.client.Server.EnableRescue(ctx, obj.server, opts); err != nil {
		return errwrap.Wrapf(err, "client.Server.EnableRescue failed")
	}
	// NOTE: EnableRescue returns a root password, but this is ignored in favor
	// of connecting to the server in rescue mode over SSH.
	// TODO: add support for password login? SSH usually ok.

	// Wait until the rescue enable is confirmed.
	if err := obj.waitUntil(ctx, obj.rescueModeEnabled); err != nil {
		return errwrap.Wrapf(err, "waitUntil(rescueModeEnabled) exited early")
	}
	return nil
}

// disableRescueMode tries to disable rescue mode for the specified server, then
// waits until the operation is confirmed. It returns early if the rescue mode
// is already disabled. Returns an error if an intermediate API request fails,
// if waitUntil exits early, or in case of context cancellation.
// NOTE: an absent server is treated as a disabled serverrescuemode.
// NOTE: the DisableRescue request requires steady state (`off` or `running`).
func (obj *HetznerVMRes) disableRescueMode(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("disableRescueMode()")
	}
	// Exit immediately if rescue mode is already disabled.
	if obj.server == nil {
		return nil
	}
	if !obj.server.RescueEnabled {
		return nil
	}
	// Exit if the server is not in a steady state (`running` or `off`).
	// NOTE: otherwise the `server is locked` when trying to enable.
	stateOK, err := obj.serverInSteadyState()
	if err != nil {
		return errwrap.Wrapf(err, "serverInSteadyState failed")
	}
	if !stateOK {
		return fmt.Errorf("state must be `running` or `off` (now: %s)", obj.server.Status)
	}
	// Disable rescue mode.
	if _, _, err := obj.client.Server.DisableRescue(ctx, obj.server); err != nil {
		return errwrap.Wrapf(err, "client.Server.EnableRescue failed")
	}
	// Wait until the rescue disable is confirmed.
	if err := obj.waitUntil(ctx, obj.rescueModeDisabled); err != nil {
		return errwrap.Wrapf(err, "waitUntil(rescueModeDisabled) exited early")
	}
	return nil
}

// getRescueKeys builds a list of keys to be enabled for rescue mode over SSH.
// ServerRescueSSHKeys provides the selected keys as []string by name. The
// corresponding data is collected with the Hetzner client (if valid). The
// resulting keylist is formatted as []*hcloud.SSHKey for use with hcloud, and
// saved for later use in private field rescueKeys.
// TODO: standardize this so that it can also be used for serverconfig keys.
func (obj *HetznerVMRes) getRescueKeys(ctx context.Context) error {
	if obj.init.Debug {
		obj.init.Logf("getRescueKeys()")
	}
	var keylist []*hcloud.SSHKey
	for _, keyname := range obj.ServerRescueSSHKeys {
		key, _, err := obj.client.SSHKey.GetByName(ctx, keyname)
		if err != nil {
			return errwrap.Wrapf(err, "SSHKey GetByName(%s) failed", keyname)
		}
		if key == nil {
			return fmt.Errorf("unknown keyname: %s", keyname)
		}
		if obj.init.Debug {
			obj.init.Logf("appending known key: %s", keyname)
		}
		keylist = append(keylist, key)
	}
	obj.rescueKeys = keylist
	return nil
}

// waitUntil provides a general function that waits until the provided exit
// condition is satisfied. It retries every WaitInterval until the condition is
// satisfied. It can exit early in case the WaitTimeout is reached, the context
// is cancelled or an error occurs. Otherwise it returns nil once the condition
// is satisfied. The exit condition must check a well-defined condition for the
// resource, and return true if satisfied, false otherwise. The condition must
// check its logic without API requests, so no context is needed.
func (obj *HetznerVMRes) waitUntil(ctx context.Context, condition func() (bool, error)) error {
	if obj.init.Debug {
		obj.init.Logf("waitUntil()")
	}
	timeout := time.After(time.Duration(obj.WaitTimeout) * time.Second)
	for {
		// Get up-to-date serverinfo.
		if err := obj.getServerUpdate(ctx); err != nil {
			return errwrap.Wrapf(err, "failed serverupdate request")
		}
		// Check if the provided exit condition is satisfied.
		conditionSatisfied, err := condition()
		if err != nil {
			return errwrap.Wrapf(err, "failed to confirm exit condition")
		}
		if conditionSatisfied {
			return nil
		}
		// Retry every WaitInterval until the exit condition is satisfied.
		// Can exit early by timeout, context cancellation or an error.
		select {
		case <-time.After(time.Duration(obj.WaitInterval) * time.Second):
			// retry confirmation
		case <-timeout:
			return fmt.Errorf("timeout: exit condition not confirmed after %d seconds", obj.WaitTimeout)
		case <-ctx.Done():
			return errwrap.Wrapf(ctx.Err(), "wait interrupted by context")
		}
	}
}

// serverStateConverged checks if the target server is in the desired state.
// Returns true if the client confirms that the state is `exists`, `running`,
// `off` or `absent` as intended. An undefined state `` always returns true.
// Otherwise, this function returns false. Invalid states result in an error.
func (obj *HetznerVMRes) serverStateConverged() (converged bool, err error) {
	if obj.init.Debug {
		obj.init.Logf("serverStateConverged()")
	}
	// always return true if undefined
	if obj.State == HetznerStateUndefined {
		return true, nil
	}
	// return true if absent as intended
	if obj.server == nil {
		if obj.State == HetznerStateAbsent {
			return true, nil
		}
		return false, nil
	}
	// convergence cases if the server exists
	switch obj.State {
	case HetznerStateAbsent:
		// false, nil
	case HetznerStateExists:
		converged = true
	case HetznerStateRunning:
		converged = (obj.server.Status == hcloud.ServerStatusRunning)
	case HetznerStateOff:
		converged = (obj.server.Status == hcloud.ServerStatusOff)
	default:
		err = fmt.Errorf("invalid state: %s", obj.State)
	}
	return converged, err
}

// serverInSteadyState returns true if the server is in one of the two known
// steady states, i.e. `running` or `off`, and false otherwise. Any other states
// are either transients or `absent`, so it is safe to return false without
// errors and try again later if needed.
func (obj *HetznerVMRes) serverInSteadyState() (steady bool, err error) {
	if obj.init.Debug {
		obj.init.Logf("serverInSteadyState()")
	}
	if obj.server == nil {
		return false, nil
	}
	switch obj.server.Status {
	case hcloud.ServerStatusRunning, hcloud.ServerStatusOff:
		return true, nil
	default:
		return false, nil
	}
}

// rescueModeEnabled returns true if rescue mode is enabled, false otherwise.
func (obj *HetznerVMRes) rescueModeEnabled() (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("rescueModeEnabled()")
	}
	if obj.server == nil {
		return false, nil
	}
	if obj.server.RescueEnabled {
		return true, nil
	}
	return false, nil
}

// rescueModeDisabled returns true if rescue mode is disabled, false otherwise.
// Server absence is also considered to `disable` rescue mode, and returns true.
func (obj *HetznerVMRes) rescueModeDisabled() (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("rescueModeDisabled()")
	}
	if obj.server == nil {
		return true, nil
	}
	if obj.server.RescueEnabled {
		return false, nil
	}
	return true, nil
}

// rescueModeConverged returns true if the server's rescue mode is enabled or
// disabled as intended, false otherwise. Absence is treated as a valid case of
// disabled rescue mode. An error can only occur for invalid rescue images.
// TODO: review checks for image and ssh keys.
func (obj *HetznerVMRes) rescueModeConverged() (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("rescueModeConverged()")
	}
	// check server existence
	if obj.server == nil {
		if obj.ServerRescueMode == HetznerServerRescueDisabled {
			return true, nil
		}
		return false, nil
	}
	// check rescue mode
	switch obj.ServerRescueMode {
	case HetznerServerRescueDisabled:
		// check if disabled as intended
		if obj.server.RescueEnabled {
			return false, nil
		}
	case HetznerServerRescueTypeLinux32, HetznerServerRescueTypeLinux64, HetznerServerRescueTypeFreeBSD64:
		// check if enabled as intended
		if !obj.server.RescueEnabled {
			return false, nil
		}
		// check if the last used image type matches specs
		// TODO: reference logic needs review
		if obj.rescueImage != hcloud.ServerRescueType(obj.ServerRescueMode) {
			return false, nil
		}
		// check if the last used keyset matches specs
		// TODO: compare rescueKeys with ServerRescueSSHKeys?
	default:
		return false, fmt.Errorf("invalid ServerRescueMode: %s", obj.ServerRescueMode)
	}
	return true, nil
}

// serverStateIs returns a function that can be used with waitUntil. When this
// function is called, it returns true if the server status matches the state
// specified as input argument, false otherwise. It also returns false if the
// state argument is not supported. The supported states are `absent`, `exists`,
// `running`, `off` and `` (undefined). Other inputs will result in an error.
// NOTE: hcloud states like ServerStatusUnknown and ServerStatusDeleting are
// also considered to be valid for state `exists`. This is important to take
// into account when rewriting or adjusting any logic using this function.
func (obj *HetznerVMRes) serverStateIs(state string) func() (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("serverStateIs(%s)", state)
	}
	return func() (bool, error) {
		// Undefined state is always true.
		if state == HetznerStateUndefined {
			return true, nil
		}
		// Exit if the server is absent.
		if obj.server == nil {
			if state == HetznerStateAbsent {
				return true, nil
			}
			return false, nil
		}
		// The server exists, but in the right state?
		switch state {
		case HetznerStateAbsent:
			return false, nil
		case HetznerStateExists:
			return true, nil
		case HetznerStateRunning:
			if obj.server.Status == hcloud.ServerStatusRunning {
				return true, nil
			}
		case HetznerStateOff:
			if obj.server.Status == hcloud.ServerStatusOff {
				return true, nil
			}
		default:
			return false, fmt.Errorf("unsupported state: %s", state)
		}
		return false, nil
	}
}
