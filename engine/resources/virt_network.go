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

//go:build !novirt

package resources

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"

	libvirt "libvirt.org/go/libvirt"       // gitlab.com/libvirt/libvirt-go-module
	libvirtxml "libvirt.org/go/libvirtxml" // gitlab.com/libvirt/libvirt-go-xml-module
)

func init() {
	engine.RegisterResource("virt:network", func() engine.Res { return &VirtNetworkRes{} })
	engine.RegisterResource("virt:network:host", func() engine.Res { return &VirtNetworkHostRes{} })
	engine.RegisterResource("virt:network:range", func() engine.Res { return &VirtNetworkRangeRes{} })
}

const (
	// VirtNetworkStateUp ensures the network is up. Whether it is defined
	// or not depends on the "transient" property.
	VirtNetworkStateUp = "up"

	// VirtNetworkStateDown ensures the network is down. Whether it is
	// defined or not depends on the "transient" property.
	VirtNetworkStateDown = "down"

	// VirtNetworkStartupEnabled ensures the network starts on boot.
	VirtNetworkStartupEnabled = "enabled"

	// VirtNetworkStartupDisabled ensures the network doesn't start on boot.
	VirtNetworkStartupDisabled = "disabled"

	// virtNetworkDefaultMode is the forward mode used when none is given.
	virtNetworkDefaultMode = "nat"

	// virtNetworkDefaultDevice is the bridge name used when none is given.
	virtNetworkDefaultDevice = "virbr0"
)

// VirtNetworkRes is a libvirt network resource. The name is used as the network
// name. This autogroups VirtNetworkHost and VirtNetworkRange resources into it
// to define the static host allocations and the dynamic DHCP pool respectively.
//
// Base network fields are reconciled through the inactive config and applied
// live by restarting the network when needed. DHCP hosts and ranges are
// reconciled against both views so live-only and config-only drift can be
// corrected without a restart.
//
// NOTE: Every network can have a "live" (active) config and an "on disk"
// (inactive) config. If you attempt to manage a network which was adversarially
// built to have different config between those two situations, then we could
// possibly miss changing the active one. Once your network restarts, this
// should resolve itself as long as the adversary stops making sneaky changes.
// This is due to the fact that getting events for live edits is not always
// possible, and also because if it turns out to be very expensive to constantly
// update both types of config then in the future we'll be more simplistic here.
type VirtNetworkRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // TODO: add autoedge support
	traits.Groupable // can have VirtNetworkHostRes and more, grouped into it

	init *engine.Init

	// URI is the libvirt connection URI, eg: `qemu:///session`.
	URI string `lang:"uri" yaml:"uri"`

	// State is the desired network state. One of "up", "down" or "".
	State string `lang:"state" yaml:"state"`

	// Transient is whether the network is defined (false) or undefined
	// (true). The libvirt API docs call this "persistent".
	Transient bool `lang:"transient" yaml:"transient"`

	// Startup specifies what should happen on startup. Values can be:
	// enabled, disabled, and undefined (empty string).
	Startup string `lang:"startup" yaml:"startup"`

	// UUID of the network. If unspecified, libvirt picks one when the
	// network is first defined.
	// TODO: Store the random value in $vardir, and use it if this is empty?
	UUID string `lang:"uuid" yaml:"uuid"`

	// Device is the name of the Linux network device we use for this
	// network. If absent this defaults to virbr0.
	Device string `lang:"device" yaml:"device"`

	// Mode is the forward mode for this device. Currently only "nat" is
	// supported. Currently this defaults to "nat".
	// TODO: Add other modes and check if this resource API makes sense.
	Mode string `lang:"mode" yaml:"mode"`

	// Mac is the bridge mac address in lower case and separated with
	// colons.
	Mac string `lang:"mac" yaml:"mac"`

	// IP is the (router) IPv4 address with the CIDR suffix that we use for
	// the network.
	IP string `lang:"ip" yaml:"ip"`

	// PurgeHosts removes DHCP host entries that are present in libvirt but
	// not declared as grouped virt:network:host resources.
	PurgeHosts bool `lang:"purge_hosts" yaml:"purge_hosts"`

	// PurgeRanges removes DHCP range entries that are present in libvirt
	// but not declared as grouped virt:network:range resources.
	PurgeRanges bool `lang:"purge_ranges" yaml:"purge_ranges"`

	// Auth points to the libvirt credentials to use if any are necessary.
	Auth *VirtAuth `lang:"auth" yaml:"auth"`

	ipv4Addr net.IP // XXX: port to netip.Addr
	ipv4Mask net.IPMask
	ipv4Net  *net.IPNet // used to check that grouped host IPs are in-subnet

	// These are all cached pointers and values used by CheckApply and its
	// child helpers. They are all set at the top of CheckApply and are only
	// valid for the lifetime of that call. They must not be used elsewhere
	// as that could race!
	conn    *libvirt.Connect
	network *libvirt.Network

	// TODO: add in ipv6 support here or in a separate resource?
}

// getDevice returns the desired device.
func (obj *VirtNetworkRes) getDevice() string {
	if obj.Device != "" {
		return obj.Device
	}
	return virtNetworkDefaultDevice
}

// getMode returns the desired mode.
func (obj *VirtNetworkRes) getMode() string {
	if obj.Mode != "" {
		return obj.Mode
	}
	return virtNetworkDefaultMode
}

// Default returns some sensible defaults for this resource.
func (obj *VirtNetworkRes) Default() engine.Res {
	return &VirtNetworkRes{}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *VirtNetworkRes) Validate() error {
	if obj.State != "" && obj.State != VirtNetworkStateUp && obj.State != VirtNetworkStateDown {
		return fmt.Errorf("the State is invalid")
	}

	if obj.Startup != VirtNetworkStartupEnabled && obj.Startup != VirtNetworkStartupDisabled && obj.Startup != "" {
		return fmt.Errorf("startup must be either `enabled` or `disabled` or undefined")
	}

	if obj.Transient && obj.Startup != "" {
		return fmt.Errorf("can't specify startup when machine is transient")
	}

	if obj.UUID != "" {
		if _, err := uuid.Parse(obj.UUID); err != nil {
			return errwrap.Wrapf(err, "invalid UUID")
		}
	}

	if obj.getDevice() == "" {
		return fmt.Errorf("the Device is empty")
	}

	if mode := obj.getMode(); mode != "nat" {
		// XXX: add more modes?
		return fmt.Errorf("the Mode must be `nat` (others are not yet supported)")
	}

	// Transient==true + State=="down" means we teardown. There's nothing to
	// define or reconcile, so the base fields aren't meaningful. Every
	// other combination needs them including Transient==true + State==""
	// against a pre-existing network, where we still need to reconcile
	// grouped hosts and ranges via the live XML. We might be only editing
	// those running values but not managing any other aspects of the state.
	if obj.Transient && obj.State == VirtNetworkStateDown {
		return nil
	}

	if obj.Mac == "" {
		return fmt.Errorf("the Mac is empty")
	}
	if obj.IP == "" {
		return fmt.Errorf("the IP is empty")
	}

	if hw, err := net.ParseMAC(obj.Mac); err != nil {
		return errwrap.Wrapf(err, "invalid mac address")

	} else if s := hw.String(); s != obj.Mac {
		// should be all lowercase, for example
		return fmt.Errorf("the mac address is not in the canonical format of: %s", s)
	}

	ipv4Addr, _, err := net.ParseCIDR(obj.IP)
	if err != nil {
		return errwrap.Wrapf(err, "invalid IP/CIDR address")
	}
	if ipv4Addr.To4() == nil {
		return fmt.Errorf("only IPv4 is currently supported")
	}

	// NOTE: the validation of hosts and ranges happens in Init during
	// runtime since we haven't yet autogrouped at this point.

	return nil
}

// Init runs some startup code for this resource.
func (obj *VirtNetworkRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// Nothing to init after this because all we want to do is nuke it!
	if obj.Transient && obj.State == VirtNetworkStateDown {
		return nil
	}

	// NOTE: If we don't Init anything that's autogrouped, then it won't
	// even get an Init call on it.
	// TODO: should we do this in the engine? Do we want to decide it here?
	for _, res := range obj.GetGroup() { // grouped elements
		if err := res.Init(init); err != nil {
			return errwrap.Wrapf(err, "autogrouped Init failed")
		}
	}

	ipv4Addr, ipv4Net, err := net.ParseCIDR(obj.IP)
	if err != nil {
		return errwrap.Wrapf(err, "unexpected invalid IP/CIDR address")
	}
	if ipv4Addr.To4() == nil {
		return fmt.Errorf("unexpectedly missing an IPv4 address")
	}

	obj.ipv4Addr = ipv4Addr.To4()
	obj.ipv4Mask = ipv4Net.Mask
	obj.ipv4Net = ipv4Net

	if err := validateUsableIPv4("network IP", obj.ipv4Addr, obj.ipv4Net); err != nil {
		return err
	}

	// Walk the grouped data to detect duplicate macs/ips/ranges, and check
	// that each child address belongs to the parent network.
	reservedMacs := map[string]engine.Res{}
	reservedIPs := map[string]engine.Res{
		obj.ipv4Addr.String(): obj, // self
	}
	hosts := []*VirtNetworkHostRes{}
	ranges := []*VirtNetworkRangeRes{}
	for _, x := range obj.GetGroup() { // grouped elements
		switch res := x.(type) {
		case *VirtNetworkHostRes:
			hosts = append(hosts, res)

		case *VirtNetworkRangeRes:
			ranges = append(ranges, res)

		default:
			return fmt.Errorf("unexpected grouped type(%T): %v", res, res)
		}
	}

	for _, res := range hosts {
		if r, exists := reservedMacs[res.Mac]; exists {
			return fmt.Errorf("res %s already reserved mac: %s", r, res.Mac)
		}
		reservedMacs[res.Mac] = res

		ipKey := res.ipv4Addr.String()
		if r, exists := reservedIPs[ipKey]; exists {
			return fmt.Errorf("res %s already reserved ip: %s", r, ipKey)
		}
		reservedIPs[ipKey] = res

		if !bytes.Equal(res.ipv4Mask, obj.ipv4Mask) {
			return fmt.Errorf("host %s mask %s does not match network mask %s", res.Name(), net.IP(res.ipv4Mask), net.IP(obj.ipv4Mask))
		}
		if err := validateUsableIPv4(fmt.Sprintf("host %s ip", res.Name()), res.ipv4Addr, obj.ipv4Net); err != nil {
			return err
		}
	}

	for i, res := range ranges {
		if bytes.Compare(res.startAddr, res.endAddr) > 0 {
			return fmt.Errorf("range %s start %s is greater than end %s", res.Name(), res.startAddr, res.endAddr)
		}
		if err := validateUsableIPv4(fmt.Sprintf("range %s start", res.Name()), res.startAddr, obj.ipv4Net); err != nil {
			return err
		}
		if err := validateUsableIPv4(fmt.Sprintf("range %s end", res.Name()), res.endAddr, obj.ipv4Net); err != nil {
			return err
		}
		for ipKey, r := range reservedIPs {
			if ip := net.ParseIP(ipKey).To4(); ip != nil && ipv4Between(ip, res.startAddr, res.endAddr) {
				return fmt.Errorf("range %s contains reserved ip %s from res %s", res.Name(), ip, r)
			}
		}
		for _, existing := range ranges[:i] {
			if ipv4RangesOverlap(existing.startAddr, existing.endAddr, res.startAddr, res.endAddr) {
				return fmt.Errorf("range %s overlaps with range %s", res.Name(), existing.Name())
			}
		}
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *VirtNetworkRes) Cleanup() error {
	return nil
}

// networkXMLDir returns the on-disk directory that libvirt uses to store
// persistent network definitions for the connection URI.
func (obj *VirtNetworkRes) networkXMLDir(uri string) (string, error) {
	if uri == "" {
		return "", fmt.Errorf("empty URI")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "qemu" {
		// XXX: support other schemes?
		return "", fmt.Errorf("scheme must be qemu")
	}

	switch u.Path {
	case "/system":
		return "/etc/libvirt/qemu/networks/", nil
	case "/session":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config/libvirt/qemu/networks") + "/", nil
	}
	return "", fmt.Errorf("unknown path: %s", u.Path)
}

// Watch is the primary listener for this resource and it outputs events.
// XXX: Can we get events from live host/range edits?
func (obj *VirtNetworkRes) Watch(ctx context.Context) error {
	// separate connection from CheckApply
	conn, _, err := obj.Auth.Connect(obj.URI)
	if err != nil {
		return errwrap.Wrapf(err, "connection to libvirt failed")
	}
	defer conn.Close()
	watchURI := obj.URI
	if uri, err := conn.GetURI(); err == nil {
		watchURI = uri
	}

	// innerCtx is cancelled either by the parent ctx or by libvirtd closing
	// the connection (via the close callback below).
	innerCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)

	if err := conn.RegisterCloseCallback(func(c *libvirt.Connect, reason libvirt.ConnectCloseReason) {
		cancel(fmt.Errorf("libvirt connection closed: reason=%d", reason))
	}); err != nil {
		return errwrap.Wrapf(err, "RegisterCloseCallback failed")
	}
	defer conn.UnregisterCloseCallback()

	netChan := make(chan libvirt.NetworkEventLifecycleType)
	callback := func(c *libvirt.Connect, n *libvirt.Network, ev *libvirt.NetworkEventLifecycle) {
		netName, err := n.GetName()
		if err != nil || netName != obj.Name() {
			// XXX: should we shutdown on error here?
			return
		}
		select {
		case netChan <- ev.Event: // send
		case <-innerCtx.Done():
		}
	}

	// Passing nil means we get events for *all* networks. We filter by name
	// in the callback above.
	callbackID, err := conn.NetworkEventLifecycleRegister(nil, callback)
	if err != nil {
		return errwrap.Wrapf(err, "NetworkEventLifecycleRegister failed")
	}
	defer conn.NetworkEventDeregister(callbackID)

	// Watch files for persistent network XML changes made through libvirt.
	// (`virsh net-update --config`, `virsh net-edit`, `virsh net-define`)
	// Raw edits to libvirt's private XML files can wake us too, but libvirt
	// may still serve the old in-memory definition.
	dir, err := obj.networkXMLDir(watchURI)
	if err != nil {
		return err
	}
	recWatcher, err := recwatch.NewRecWatcher(dir, false)
	if err != nil {
		return err
	}
	defer recWatcher.Close()
	recChan := recWatcher.Events()

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case event, ok := <-netChan:
			if !ok {
				netChan = nil
				continue
			}
			if obj.init.Debug {
				obj.init.Logf("network event: %v", event)
			}

		case event, ok := <-recChan:
			if !ok {
				recChan = nil
				continue
			}
			if err := event.Error; err != nil {
				obj.init.Logf("file watcher error: %v", err)
				recChan = nil
				continue
			}
			if obj.init.Debug {
				obj.init.Logf("file event(%s): %v", event.Body.Name, event.Body.Op)
			}

		case <-innerCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err() // parent shutdown
			}
			return context.Cause(innerCtx) // libvirtd disconnected
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply reconciles the libvirt network with the desired state. The base
// network XML is rendered from the parent fields plus any grouped child
// resources. For already defined networks we use the incremental
// network.Update() API for DHCP entries, so we don't need to bring the network
// down to converge those entries.
func (obj *VirtNetworkRes) CheckApply(ctx context.Context, apply bool) (checkOK bool, reterr error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	// We cache a lot of values in this function, to be used by the child
	// CheckApply helpers.

	// XXX: How expensive is it to connect and disconnect each CheckApply?
	// XXX: Since this probably doesn't run often, maybe we save memory?
	conn, _, err := obj.Auth.Connect(obj.URI)
	if err != nil {
		return false, errwrap.Wrapf(err, "connection to libvirt failed")
	}
	obj.conn = conn // cache the connection for child helpers
	defer func() {
		if _, err := obj.conn.Close(); err != nil {
			checkOK = false
			reterr = errwrap.Append(reterr, errwrap.Wrapf(err, "connection close failed"))
		}
		obj.conn = nil
	}()

	obj.network = nil // reset
	network, err := obj.conn.LookupNetworkByName(obj.Name())
	if err != nil && !isNotFound(err) {
		return false, errwrap.Wrapf(err, "could not lookup network")
	}
	exists := !isNotFound(err)
	if exists {
		obj.network = network
		defer func() {
			// don't touch obj.network since that gets reused below!
			if err := network.Free(); err != nil {
				checkOK = false
				reterr = errwrap.Append(reterr, errwrap.Wrapf(err, "network free failed"))
			}
			network = nil
		}()
	}
	// if !exists, it means it's undefined and not "up" either

	checkOK = true

	if c, err := obj.removeCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
	}

	addCheckOK := true
	n, c, err := obj.addCheckApply(ctx, apply)
	if err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
		addCheckOK = false
	}
	if n != nil {
		obj.network = n // populate!
		defer func() {
			// don't touch obj.network since that gets reused below!
			if err := n.Free(); err != nil {
				checkOK = false
				reterr = errwrap.Append(reterr, errwrap.Wrapf(err, "network free failed"))
			}
			n = nil
		}()
	}

	// NOTE: doing this operation again should be redundant...
	if addCheckOK { // skip if addCheckApply already built something!
		n, c, err := obj.baseCheckApply(ctx, apply)
		if err != nil {
			return false, err
		} else if !c {
			//if !apply { // short-circuit and end early?
			//	return false, nil
			//}
			checkOK = false
		}
		if n != nil { // apply && !checkOK
			obj.network = n // populate!
			defer func() {
				// don't touch obj.network since that gets reused below!
				if err := n.Free(); err != nil {
					checkOK = false
					reterr = errwrap.Append(reterr, errwrap.Wrapf(err, "network free failed"))
				}
				n = nil
			}()
		}
	}

	// performs hostsCheckApply and rangeCheckApply for both live and config
	if c, err := obj.groupedCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
	}

	if c, err := obj.activeCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
	}

	if c, err := obj.startupCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
	}

	return checkOK, nil
}

// removeCheckApply destroys the network (if active) and (if persistent) it
// undefines it.
func (obj *VirtNetworkRes) removeCheckApply(ctx context.Context, apply bool) (bool, error) {

	// Easy case, we want it gone, and it doesn't exist.
	if obj.network == nil && obj.State == VirtNetworkStateDown && obj.Transient {
		return true, nil
	}

	if obj.network == nil { // must not apply to me
		// TODO: old logic here, but is it ever useful to do it this way?
		// no need to check apply here since we aren't changing anything
		//return false, nil // something will have to be done by someone
		return true, nil
	}

	// For these cases, we do nothing in this CheckApply function variant.
	//if active && obj.State != VirtNetworkStateDown {}
	//if persistent && !obj.Transient {}

	// Read both flags up front. Destroying a transient network leaves
	// obj.network as a stale handle, so a later IsPersistent on it would
	// fail with VIR_ERR_NO_NETWORK. :(
	active, err := obj.network.IsActive()
	if err != nil {
		return false, errwrap.Wrapf(err, "network.IsActive failed")
	}
	persistent, err := obj.network.IsPersistent()
	if err != nil {
		return false, errwrap.Wrapf(err, "network.IsPersistent failed")
	}

	checkOK := true

	if active && obj.State == VirtNetworkStateDown {
		if !apply {
			return false, nil
		}
		checkOK = false

		if err := obj.network.Destroy(); err != nil {
			return false, errwrap.Wrapf(err, "network.Destroy failed")
		}
		obj.init.Logf("network destroyed")

		active = false // update for accounting below...
	}

	if persistent && obj.Transient {
		if !apply {
			return false, nil
		}
		checkOK = false

		if err := obj.network.Undefine(); err != nil {
			return false, errwrap.Wrapf(err, "network.Undefine failed")
		}
		obj.init.Logf("network undefined")

		persistent = false // update for accounting below...
	}

	if !active && !persistent {
		obj.network = nil // cleanup for future callers
	}

	return checkOK, nil
}

// addCheckApply defines and/or creates the network from scratch if it's absent.
// It returns an updated libvirt Network struct if it creates one. This *must*
// be Free()'d by the caller if it is returned.
func (obj *VirtNetworkRes) addCheckApply(ctx context.Context, apply bool) (*libvirt.Network, bool, error) {

	if obj.network != nil { // we only do define/create operations
		return nil, true, nil // checkOK is true, network is nil!
	}

	// can't be transient and not "up" here or there's nothing to do!
	if obj.Transient && obj.State != VirtNetworkStateUp {
		return nil, true, nil // checkOK is true, network is nil!
	}

	if !apply {
		return nil, false, nil // apply is false, network is nil!
	}

	xml, err := obj.buildNetworkXML()
	if err != nil {
		return nil, false, errwrap.Wrapf(err, "could not build network XML")
	}

	// Here are the different things to do if !exists...
	//   up and transient  -> virNetworkCreateXML
	//   up and persistent -> virNetworkDefineXML + virNetworkCreate
	// down and persistent -> virNetworkDefineXML

	if obj.Transient && obj.State == VirtNetworkStateUp {
		network, err := obj.conn.NetworkCreateXML(xml)
		if err != nil {
			return nil, false, errwrap.Wrapf(err, "NetworkCreateXML failed")
		}
		//defer network.Free() // freed by caller!

		obj.init.Logf("network created (defined and started)")
		return network, false, nil
	}

	if obj.Transient {
		// unexpected case, can't be transient and not "up" here!
		return nil, false, fmt.Errorf("programming error")
	}

	// build the persistent xml
	network, err := obj.conn.NetworkDefineXML(xml)
	if err != nil {
		return nil, false, errwrap.Wrapf(err, "NetworkDefineXML failed")
	}
	//defer network.Free() // freed by caller!
	obj.init.Logf("network defined")

	if obj.State != VirtNetworkStateUp { // done early, nothing else to do
		return network, false, nil
	}

	if err := network.Create(); err != nil {
		defer network.Free() // on error scenarios only!
		return nil, false, errwrap.Wrapf(err, "network.Create failed")
	}
	obj.init.Logf("network started")

	return network, false, nil
}

// baseCheckApply diffs the desired non-host/range fields against inactive XML
// for persistent networks, or live XML for transient networks. If any differ,
// only the base fields we manage get rewritten. Persistent networks are
// redefined and restarted when active. Transient networks are destroyed and
// created again because these core fields cannot be mutated live. It returns an
// updated libvirt Network struct if it creates one. This *must* be Free()'d by
// the caller if it is returned.
func (obj *VirtNetworkRes) baseCheckApply(ctx context.Context, apply bool) (*libvirt.Network, bool, error) {

	if obj.network == nil {
		// must have been a remove scenario (or a bug)
		return nil, true, nil // checkOK is true, network is nil!
	}

	// can't be transient and not "up" here or there's nothing to do!
	if obj.Transient && obj.State != VirtNetworkStateUp {
		return nil, true, nil // checkOK is true, network is nil!
	}

	// Here are the different things to do if !exists...
	//   up and transient  -> virNetworkCreateXML
	//   up and persistent -> virNetworkDefineXML + virNetworkCreate
	// down and persistent -> virNetworkDefineXML
	// down and transient  -> (nothing)

	flags := libvirt.NETWORK_XML_INACTIVE
	if obj.Transient {
		flags = libvirt.NetworkXMLFlags(0)
	}
	current := &libvirtxml.Network{}

	// Fetch the persistent XML for persistent networks, or the live XML for
	// transient networks. It's not our fault if an out-of-band thing
	// modifies the world. If the active network differs from the inactive
	// config, then someone was playing games and we can't really help you.
	desc, err := obj.network.GetXMLDesc(flags)
	if err != nil {
		return nil, false, errwrap.Wrapf(err, "network.GetXMLDesc failed")
	}
	if err := current.Unmarshal(desc); err != nil { // store in current
		return nil, false, errwrap.Wrapf(err, "could not unmarshal network XML")
	}

	idx, err := obj.findIPv4Index(current)
	if err != nil {
		return nil, false, err
	}
	if idx >= 0 && obj.baseMatch(current, idx) {
		return nil, true, nil // checkOK is true, network is nil!
	}

	if !apply {
		return nil, false, nil // apply is false, network is nil!
	}

	if obj.UUID != "" {
		current.UUID = obj.UUID
	}
	if current.Forward == nil {
		current.Forward = &libvirtxml.NetworkForward{}
	}
	current.Forward.Mode = obj.getMode()
	if current.Bridge == nil {
		current.Bridge = &libvirtxml.NetworkBridge{}
	}
	current.Bridge.Name = obj.getDevice()
	if current.MAC == nil {
		current.MAC = &libvirtxml.NetworkMAC{}
	}
	current.MAC.Address = obj.Mac
	if idx < 0 {
		current.IPs = append(current.IPs, libvirtxml.NetworkIP{})
		idx = len(current.IPs) - 1
	}
	current.IPs[idx].Address = obj.ipv4Addr.String()
	current.IPs[idx].Netmask = net.IP(obj.ipv4Mask).String()

	xml, err := current.Marshal()
	if err != nil {
		return nil, false, errwrap.Wrapf(err, "could not marshal desired XML")
	}

	// If we're transient, then just destroy and re-create.
	if obj.Transient {
		if err := obj.network.Destroy(); err != nil {
			return nil, false, errwrap.Wrapf(err, "network.Destroy failed during base CheckApply")
		}

		network, err := obj.conn.NetworkCreateXML(xml)
		if err != nil {
			return nil, false, errwrap.Wrapf(err, "NetworkCreateXML failed during update")
		}
		obj.init.Logf("network recreated (base fields)")

		return network, false, nil
	}

	network, err := obj.conn.NetworkDefineXML(xml)
	if err != nil {
		return nil, false, errwrap.Wrapf(err, "NetworkDefineXML failed during update")
	}
	//defer network.Free() // freed by caller!
	obj.init.Logf("network redefined (base fields)")

	// Base changes only take effect on the live network after a restart,
	// because NetworkDefineXML only touches the persistent definition.
	active, err := obj.network.IsActive()
	if err != nil {
		defer network.Free() // on error scenarios only!
		return nil, false, errwrap.Wrapf(err, "network.IsActive failed")
	}
	if !active {
		return network, false, nil
	}

	if err := obj.network.Destroy(); err != nil {
		defer network.Free() // on error scenarios only!
		return nil, false, errwrap.Wrapf(err, "network.Destroy failed during base CheckApply")
	}

	if obj.State == VirtNetworkStateDown {
		obj.init.Logf("network stopped to apply base changes")
		return network, false, nil
	}

	if err := obj.network.Create(); err != nil {
		defer network.Free() // on error scenarios only!
		return nil, false, errwrap.Wrapf(err, "network.Create failed during base CheckApply")
	}
	obj.init.Logf("network restarted to apply base changes")

	return network, false, nil
}

// groupedCheckApply runs the groupCheckApply a maximum of two times. Once for
// live and once for the inactive config, whichever are necessary.
func (obj *VirtNetworkRes) groupedCheckApply(ctx context.Context, apply bool) (bool, error) {

	if obj.network == nil {
		// must have been a remove scenario (or a bug)
		return true, nil // checkOK is true, network is nil!
	}

	// Reconcile against whatever states are defined. A transient running
	// network (e.g. one we just undefined) has no inactive XML, but its
	// live hosts/ranges are still ours to manage.

	checkOK := true

	persistent, err := obj.network.IsPersistent()
	if err != nil {
		return false, errwrap.Wrapf(err, "network.IsPersistent failed")
	}
	if persistent {
		flags := libvirt.NETWORK_XML_INACTIVE
		if c, err := obj.groupCheckApply(ctx, apply, flags); err != nil {
			return false, err
		} else if !c {
			//if !apply { // short-circuit and end early?
			//	return false, nil
			//}
			checkOK = false
		}
	}

	active, err := obj.network.IsActive()
	if err != nil {
		return false, errwrap.Wrapf(err, "network.IsActive failed")
	}
	if active {
		flags := libvirt.NetworkXMLFlags(0)
		if c, err := obj.groupCheckApply(ctx, apply, flags); err != nil {
			return false, err
		} else if !c {
			//if !apply { // short-circuit and end early?
			//	return false, nil
			//}
			checkOK = false
		}
	}

	return checkOK, nil
}

// groupCheckApply performs CheckApply of all of the grouped resources. It takes
// a flag to determine if it should do it to the live or inactive config parts.
func (obj *VirtNetworkRes) groupCheckApply(ctx context.Context, apply bool, flags libvirt.NetworkXMLFlags) (bool, error) {

	current := &libvirtxml.Network{}

	// Fetch the persistent XML once and hand it to each helper so they see
	// a snapshot of the state. It's not our fault if an out-of-band thing
	// modifies the world. If the active network differs from the inactive
	// config, then someone was playing games and we can't really help you.
	// We'll try to reconcile this by checking both sides, but that's sad.
	desc, err := obj.network.GetXMLDesc(flags)
	if err != nil {
		return false, errwrap.Wrapf(err, "network.GetXMLDesc failed")
	}
	if err := current.Unmarshal(desc); err != nil { // store in current
		return false, errwrap.Wrapf(err, "could not unmarshal network XML")
	}

	// XXX: can we ever combine the two flags?
	var innerFlags libvirt.NetworkUpdateFlags
	switch flags {
	case libvirt.NETWORK_XML_INACTIVE:
		innerFlags = libvirt.NETWORK_UPDATE_AFFECT_CONFIG
	case libvirt.NetworkXMLFlags(0):
		innerFlags = libvirt.NETWORK_UPDATE_AFFECT_LIVE
	default:
		return false, fmt.Errorf("unsupported network XML flags: %d", flags)
	}
	checkOK := true

	if c, err := obj.hostsCheckApply(ctx, apply, current, innerFlags); err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
	}

	if c, err := obj.rangeCheckApply(ctx, apply, current, innerFlags); err != nil {
		return false, err
	} else if !c {
		//if !apply { // short-circuit and end early?
		//	return false, nil
		//}
		checkOK = false
	}

	return checkOK, nil
}

// hostsCheckApply uses the incremental network.Update() API to add, modify and
// delete DHCP host entries so that the network matches the set of grouped
// VirtNetworkHostRes children.
func (obj *VirtNetworkRes) hostsCheckApply(ctx context.Context, apply bool, current *libvirtxml.Network, flags libvirt.NetworkUpdateFlags) (bool, error) {
	currentHosts := map[string]libvirtxml.NetworkDHCPHost{}
	idx, err := obj.findIPv4Index(current)
	if err != nil {
		return false, err
	}
	if idx >= 0 && current.IPs[idx].DHCP != nil {
		for _, h := range current.IPs[idx].DHCP.Hosts {
			currentHosts[h.MAC] = h
		}
	}

	desiredHosts := map[string]libvirtxml.NetworkDHCPHost{}
	for _, x := range obj.GetGroup() {
		res, ok := x.(*VirtNetworkHostRes)
		if !ok {
			continue
		}
		h := res.toDHCPHost()
		desiredHosts[h.MAC] = h
	}

	checkOK := true

	// add or modify
	for mac, want := range desiredHosts {
		got, exists := currentHosts[mac]
		if exists && hostsEqual(got, want) {
			continue
		}
		if !apply {
			return false, nil
		}
		checkOK = false
		xml, err := want.Marshal()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not marshal DHCP host XML")
		}
		cmd := libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST
		if exists {
			cmd = libvirt.NETWORK_UPDATE_COMMAND_MODIFY
		}

		// which parent element, if there are multiple parents of the
		// same type (e.g. which <ip> element when modifying a
		// <dhcp>/<host> element), or "-1" for "don't care" or
		// "automatically find appropriate one".
		parentIndex := idx
		if err := obj.network.Update(cmd, libvirt.NETWORK_SECTION_IP_DHCP_HOST, parentIndex, xml, flags); err != nil {
			return false, errwrap.Wrapf(err, "network.Update (add/modify) failed for mac %s", mac)
		}
		if exists {
			obj.init.Logf("dhcp host modified: %s", mac)
		} else {
			obj.init.Logf("dhcp host added: %s", mac)
		}
	}

	// delete
	if !obj.PurgeHosts { // done early
		return checkOK, nil
	}

	for mac, got := range currentHosts {
		if _, exists := desiredHosts[mac]; exists {
			continue
		}
		if !apply {
			return false, nil
		}
		checkOK = false
		xml, err := got.Marshal()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not marshal DHCP host XML")
		}

		parentIndex := idx
		if err := obj.network.Update(libvirt.NETWORK_UPDATE_COMMAND_DELETE, libvirt.NETWORK_SECTION_IP_DHCP_HOST, parentIndex, xml, flags); err != nil {
			return false, errwrap.Wrapf(err, "network.Update (delete) failed for mac %s", mac)
		}
		obj.init.Logf("dhcp host deleted: %s", mac)
	}

	return checkOK, nil
}

// rangeCheckApply uses the incremental network.Update() API to add and delete
// DHCP range entries so that the network matches the set of grouped
// VirtNetworkRangeRes children. Ranges have no natural identifier other than
// start+end, so a "modify" is just delete+add.
func (obj *VirtNetworkRes) rangeCheckApply(ctx context.Context, apply bool, current *libvirtxml.Network, flags libvirt.NetworkUpdateFlags) (bool, error) {
	rangeKey := func(r libvirtxml.NetworkDHCPRange) string {
		return r.Start + "-" + r.End
	}

	currentRanges := map[string]libvirtxml.NetworkDHCPRange{}
	idx, err := obj.findIPv4Index(current)
	if err != nil {
		return false, err
	}
	if idx >= 0 && current.IPs[idx].DHCP != nil {
		for _, r := range current.IPs[idx].DHCP.Ranges {
			currentRanges[rangeKey(r)] = r
		}
	}

	desiredRanges := map[string]libvirtxml.NetworkDHCPRange{}
	for _, x := range obj.GetGroup() {
		res, ok := x.(*VirtNetworkRangeRes)
		if !ok {
			continue
		}
		r := res.toDHCPRange()
		desiredRanges[rangeKey(r)] = r
	}

	checkOK := true

	// add missing
	for key, want := range desiredRanges {
		if _, exists := currentRanges[key]; exists {
			continue
		}
		if !apply {
			return false, nil
		}
		checkOK = false

		xml, err := want.Marshal()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not marshal DHCP range XML")
		}

		parentIndex := idx
		if err := obj.network.Update(libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST, libvirt.NETWORK_SECTION_IP_DHCP_RANGE, parentIndex, xml, flags); err != nil {
			return false, errwrap.Wrapf(err, "network.Update (add) failed for range %s", key)
		}
		obj.init.Logf("dhcp range added: %s", key)
	}

	// delete extra
	if !obj.PurgeRanges { // done early
		return checkOK, nil
	}

	for key, got := range currentRanges {
		if _, exists := desiredRanges[key]; exists {
			continue
		}
		if !apply {
			return false, nil
		}
		checkOK = false

		xml, err := got.Marshal()
		if err != nil {
			return false, errwrap.Wrapf(err, "could not marshal DHCP range XML")
		}

		parentIndex := idx
		if err := obj.network.Update(libvirt.NETWORK_UPDATE_COMMAND_DELETE, libvirt.NETWORK_SECTION_IP_DHCP_RANGE, parentIndex, xml, flags); err != nil {
			return false, errwrap.Wrapf(err, "network.Update (delete) failed for range %s", key)
		}
		obj.init.Logf("dhcp range deleted: %s", key)
	}

	return checkOK, nil
}

// activeCheckApply reconciles the network's active (started/stopped) state.
func (obj *VirtNetworkRes) activeCheckApply(ctx context.Context, apply bool) (bool, error) {

	if obj.network == nil { // must not apply to me
		return true, nil
	}

	if obj.State == "" { // undefined
		return true, nil
	}

	// TODO: should we cache this call?
	active, err := obj.network.IsActive()
	if err != nil {
		return false, errwrap.Wrapf(err, "network.IsActive failed")
	}

	if obj.State == VirtNetworkStateUp && active {
		return true, nil
	}
	if obj.State == VirtNetworkStateDown && !active {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	if obj.State == VirtNetworkStateDown {
		if err := obj.network.Destroy(); err != nil {
			return false, errwrap.Wrapf(err, "network.Destroy failed")
		}
		obj.init.Logf("network stopped")
		return false, nil
	}

	if err := obj.network.Create(); err != nil {
		return false, errwrap.Wrapf(err, "network.Create failed")
	}
	obj.init.Logf("network started")
	return false, nil
}

// startupCheckApply reconciles whether or not the network starts on first boot.
func (obj *VirtNetworkRes) startupCheckApply(ctx context.Context, apply bool) (bool, error) {

	if obj.network == nil { // must not apply to me
		return true, nil
	}

	if obj.Startup == "" { // undefined
		return true, nil
	}

	autostart, err := obj.network.GetAutostart()
	if err != nil {
		return false, errwrap.Wrapf(err, "network.GetAutostart failed")
	}

	startup := obj.Startup == VirtNetworkStartupEnabled // convert to bool

	if autostart == startup { // both true or both false, we're done early!
		return true, nil
	}

	if !apply {
		return false, nil
	}

	if err := obj.network.SetAutostart(startup); err != nil {
		return false, errwrap.Wrapf(err, "network.SetAutostart(%t) failed", startup)
	}
	obj.init.Logf("network startup: %t", startup)

	return false, nil
}

// buildNetworkXML renders the desired libvirt network XML from the parent
// fields and any grouped VirtNetworkHostRes / VirtNetworkRangeRes children.
func (obj *VirtNetworkRes) buildNetworkXML() (string, error) {
	hosts := []libvirtxml.NetworkDHCPHost{}
	ranges := []libvirtxml.NetworkDHCPRange{}
	for _, x := range obj.GetGroup() {
		switch res := x.(type) {
		case *VirtNetworkHostRes:
			hosts = append(hosts, res.toDHCPHost())

		case *VirtNetworkRangeRes:
			ranges = append(ranges, res.toDHCPRange())

		default:
			return "", fmt.Errorf("unexpected grouped type(%T): %v", res, res)
		}
	}

	ipBlock := libvirtxml.NetworkIP{
		Address: obj.ipv4Addr.String(),
		Netmask: net.IP(obj.ipv4Mask).String(),
	}
	// Only emit a <dhcp> element when there is something in it. Otherwise
	// libvirt would start dnsmasq for a network that didn't ask for DHCP!
	if len(hosts) > 0 || len(ranges) > 0 {
		ipBlock.DHCP = &libvirtxml.NetworkDHCP{
			Ranges: ranges,
			Hosts:  hosts,
		}
	}

	n := &libvirtxml.Network{
		Name: obj.Name(),
		UUID: obj.UUID,
		Forward: &libvirtxml.NetworkForward{
			Mode: obj.getMode(),
		},
		Bridge: &libvirtxml.NetworkBridge{
			Name: obj.getDevice(),
		},
		MAC: &libvirtxml.NetworkMAC{
			Address: obj.Mac,
		},
		IPs: []libvirtxml.NetworkIP{ipBlock},
	}
	return n.Marshal()
}

// findIPv4Index returns the network IP entry index that this resource owns.
// When the desired address already exists, that entry wins. Otherwise an
// existing network must have at most one IPv4 entry so we can update it without
// guessing.
func (obj *VirtNetworkRes) findIPv4Index(current *libvirtxml.Network) (int, error) {
	candidates := []int{}
	desired := obj.ipv4Addr.String()
	for i, ip := range current.IPs {
		if ip.Address == desired {
			return i, nil
		}
		if networkIPIsIPv4(ip) {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return -1, nil
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return -1, fmt.Errorf("multiple IPv4 network entries exist and none matches desired address %s", desired)
}

// baseMatch reports whether the persistent network's non-host fields match what
// we want. Returns true when no changes are needed.
func (obj *VirtNetworkRes) baseMatch(current *libvirtxml.Network, idx int) bool {
	if obj.UUID != "" && current.UUID != obj.UUID {
		return false
	}
	if current.Forward == nil || current.Forward.Mode != obj.getMode() {
		return false
	}
	if current.Bridge == nil || current.Bridge.Name != obj.getDevice() {
		return false
	}
	if current.MAC == nil || current.MAC.Address != obj.Mac {
		return false
	}
	if idx < 0 || idx >= len(current.IPs) {
		return false
	}
	ip := current.IPs[idx]
	if ip.Address != obj.ipv4Addr.String() {
		return false
	}
	if ip.Netmask != net.IP(obj.ipv4Mask).String() {
		return false
	}
	return true
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *VirtNetworkRes) Cmp(r engine.Res) error {
	// we can only compare VirtNetworkRes to others of the same resource kind
	res, ok := r.(*VirtNetworkRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.URI != res.URI {
		return fmt.Errorf("the URI differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Transient != res.Transient {
		return fmt.Errorf("the Transient differs")
	}
	if obj.Startup != res.Startup {
		return fmt.Errorf("the Startup differs")
	}
	if obj.UUID != res.UUID {
		return fmt.Errorf("the UUID differs")
	}
	if obj.Device != res.Device {
		return fmt.Errorf("the Device differs")
	}
	if obj.Mode != res.Mode {
		return fmt.Errorf("the Mode differs")
	}
	if obj.Mac != res.Mac {
		return fmt.Errorf("the Mac differs")
	}
	if obj.IP != res.IP {
		return fmt.Errorf("the IP differs")
	}
	if obj.PurgeHosts != res.PurgeHosts {
		return fmt.Errorf("the PurgeHosts differs")
	}
	if obj.PurgeRanges != res.PurgeRanges {
		return fmt.Errorf("the PurgeRanges differs")
	}

	if err := obj.Auth.Cmp(res.Auth); err != nil {
		return errwrap.Wrapf(err, "the Auth differs")
	}

	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *VirtNetworkRes) GroupCmp(r engine.GroupableRes) error {
	res1, ok1 := r.(*VirtNetworkHostRes) // different from what we usually do!
	if ok1 {
		// If the virt host resource has the Network field specified,
		// then it must match against our name field if we want it to
		// group with us.
		if res1.Network != "" && res1.Network != obj.Name() {
			return fmt.Errorf("resource groups with a different network name")
		}

		return nil
	}

	res2, ok2 := r.(*VirtNetworkRangeRes) // different from what we usually do!
	if ok2 {
		// If the virt range resource has the Network field specified,
		// then it must match against our name field if we want it to
		// group with us.
		if res2.Network != "" && res2.Network != obj.Name() {
			return fmt.Errorf("resource groups with a different network name")
		}

		return nil
	}

	return fmt.Errorf("resource is not the right kind")
}

// Background is a worker function which is run once per resource kind as long
// as there is at least one of that kind running in the active resource graph.
// The worker function is the generated (returned) function that is used here.
// NOTE: This is not needed for VirtNetworkHostRes and VirtNetworkRangeRes
// because both of those autogroup into this resource which is all that is seen.
func (obj *VirtNetworkRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return libvirtNewBackgroundBool(handle)
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *VirtNetworkRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes VirtNetworkRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*VirtNetworkRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to VirtNetworkRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = VirtNetworkRes(raw) // restore from indirection with type conversion!
	return nil
}

// VirtNetworkHostRes is a representation of a static host assignment in DHCP.
// It groups into a VirtNetworkRes (selected via the Network field, or the only
// virt:network in the graph if Network is unset) and renders as a single
// `<host name='...' mac='...' ip='...'/>` entry under the network's `<dhcp>`
// block.
type VirtNetworkHostRes struct {
	traits.Base // add the base methods without re-implementation
	//traits.Edgeable // XXX: add autoedge support
	traits.Groupable // can be grouped into VirtNetworkRes

	init *engine.Init

	// Network is the name of the virt network resource to group this into.
	// If it is omitted, and there is only a single virt network resource,
	// then it will be grouped into it automatically. If there is more than
	// one main virt network resource being used, then the grouping
	// behaviour is *undefined* when this is not specified, and it is not
	// recommended to leave this blank!
	Network string `lang:"network" yaml:"network"`

	// Hostname is the DHCP hostname rendered as the `name=` attribute on
	// the `<host>` element. If empty, the resource Name() is used as a
	// fallback.
	Hostname string `lang:"hostname" yaml:"hostname"`

	// Mac is the mac address of the host in lower case and separated with
	// colons.
	Mac string `lang:"mac" yaml:"mac"`

	// IP is the IPv4 address with the CIDR suffix. The suffix is required
	// in case we eventually need it somewhere and for consistency with the
	// API of the dhcp:host resource. For example, you might specify
	// 192.0.2.42/24 which represents a mask of 255.255.255.0 that is used.
	IP string `lang:"ip" yaml:"ip"`

	ipv4Addr net.IP // XXX: port to netip.Addr
	ipv4Mask net.IPMask
}

// getHostname returns the hostname to render in the libvirt host entry. We
// prefer the explicit Hostname field, and fall back to the resource Name().
func (obj *VirtNetworkHostRes) getHostname() string {
	if obj.Hostname != "" {
		return obj.Hostname
	}
	return obj.Name()
}

// toDHCPHost returns the libvirtxml struct that we use to render this host into
// the parent network's XML, or to drive a network.Update() call.
func (obj *VirtNetworkHostRes) toDHCPHost() libvirtxml.NetworkDHCPHost {
	return libvirtxml.NetworkDHCPHost{
		MAC:  obj.Mac,
		Name: obj.getHostname(),
		IP:   obj.ipv4Addr.String(),
	}
}

// Default returns some sensible defaults for this resource.
func (obj *VirtNetworkHostRes) Default() engine.Res {
	return &VirtNetworkHostRes{}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *VirtNetworkHostRes) Validate() error {
	if hw, err := net.ParseMAC(obj.Mac); err != nil {
		return errwrap.Wrapf(err, "invalid mac address")

	} else if s := hw.String(); s != obj.Mac {
		// should be all lowercase, for example
		return fmt.Errorf("the mac address is not in the canonical format of: %s", s)
	}

	ipv4Addr, _, err := net.ParseCIDR(obj.IP)
	if err != nil {
		return errwrap.Wrapf(err, "invalid IP/CIDR address")
	}
	if ipv4Addr.To4() == nil {
		return fmt.Errorf("only IPv4 is currently supported")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *VirtNetworkHostRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	ipv4Addr, ipv4Net, err := net.ParseCIDR(obj.IP)
	if err != nil {
		return errwrap.Wrapf(err, "unexpected invalid IP/CIDR address")
	}
	if ipv4Addr.To4() == nil {
		return fmt.Errorf("unexpectedly missing an IPv4 address")
	}

	obj.ipv4Addr = ipv4Addr.To4()
	obj.ipv4Mask = ipv4Net.Mask

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *VirtNetworkHostRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *VirtNetworkHostRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
		return ctx.Err()
	}
}

// CheckApply never has anything to do for this resource, since the parent
// VirtNetworkRes does the work for grouped children.
func (obj *VirtNetworkHostRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *VirtNetworkHostRes) Cmp(r engine.Res) error {
	// we can only compare VirtNetworkHostRes to others of the same resource kind
	res, ok := r.(*VirtNetworkHostRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Network != res.Network {
		return fmt.Errorf("the Network field differs")
	}
	if obj.Hostname != res.Hostname {
		return fmt.Errorf("the Hostname differs")
	}
	if obj.Mac != res.Mac {
		return fmt.Errorf("the Mac differs")
	}
	if obj.IP != res.IP {
		return fmt.Errorf("the IP differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *VirtNetworkHostRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes VirtNetworkHostRes // indirection to avoid infinite recursion

	def := obj.Default()                 // get the default
	res, ok := def.(*VirtNetworkHostRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to VirtNetworkHostRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = VirtNetworkHostRes(raw) // restore from indirection with type conversion!
	return nil
}

// VirtNetworkRangeRes defines a dynamic DHCP address pool. It groups into a
// VirtNetworkRes (selected via the Network field, or the only virt:network in
// the graph if Network is unset) and renders as a single
// `<range start='...' end='...'/>` entry under the network's `<dhcp>` block.
// Any DHCP request from a MAC not covered by a virt:network:host reservation is
// assigned an IP from this pool by libvirt's built-in dnsmasq.
type VirtNetworkRangeRes struct {
	traits.Base // add the base methods without re-implementation
	//traits.Edgeable // XXX: add autoedge support
	traits.Groupable // can be grouped into VirtNetworkRes

	init *engine.Init

	// Network is the name of the virt network resource to group this into.
	// If it is omitted, and there is only a single virt network resource,
	// then it will be grouped into it automatically. If there is more than
	// one main virt network resource being used, then the grouping
	// behaviour is *undefined* when this is not specified, and it is not
	// recommended to leave this blank!
	Network string `lang:"network" yaml:"network"`

	// Start is the first IPv4 address in the dynamic DHCP pool, e.g.
	// "192.168.122.2". The IP must lie within the parent network's subnet.
	Start string `lang:"start" yaml:"start"`

	// End is the last IPv4 address in the dynamic DHCP pool inclusive, e.g.
	// "192.168.122.254". Must lie within the parent network's subnet and
	// must not be lower than Start.
	End string `lang:"end" yaml:"end"`

	startAddr net.IP // XXX: port to netip.Addr
	endAddr   net.IP
}

// toDHCPRange returns the libvirtxml struct that we use to render this range
// into the parent network's XML, or to drive a network.Update() call.
func (obj *VirtNetworkRangeRes) toDHCPRange() libvirtxml.NetworkDHCPRange {
	return libvirtxml.NetworkDHCPRange{
		Start: obj.startAddr.String(),
		End:   obj.endAddr.String(),
	}
}

// Default returns some sensible defaults for this resource.
func (obj *VirtNetworkRangeRes) Default() engine.Res {
	return &VirtNetworkRangeRes{}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *VirtNetworkRangeRes) Validate() error {
	if obj.Start == "" {
		return fmt.Errorf("the Start address is empty")
	}
	if obj.End == "" {
		return fmt.Errorf("the End address is empty")
	}

	start := net.ParseIP(obj.Start)
	if start == nil || start.To4() == nil {
		return fmt.Errorf("the Start is not a valid IPv4 address: %s", obj.Start)
	}
	end := net.ParseIP(obj.End)
	if end == nil || end.To4() == nil {
		return fmt.Errorf("the End is not a valid IPv4 address: %s", obj.End)
	}

	if bytes.Compare(start.To4(), end.To4()) > 0 {
		return fmt.Errorf("the Start (%s) must not be greater than End (%s)", obj.Start, obj.End)
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *VirtNetworkRangeRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.startAddr = net.ParseIP(obj.Start).To4()
	if obj.startAddr == nil {
		return fmt.Errorf("unexpectedly invalid Start address: %s", obj.Start)
	}
	obj.endAddr = net.ParseIP(obj.End).To4()
	if obj.endAddr == nil {
		return fmt.Errorf("unexpectedly invalid End address: %s", obj.End)
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *VirtNetworkRangeRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *VirtNetworkRangeRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
		return ctx.Err()
	}
}

// CheckApply never has anything to do for this resource, since the parent
// VirtNetworkRes does the work for grouped children.
func (obj *VirtNetworkRangeRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *VirtNetworkRangeRes) Cmp(r engine.Res) error {
	// we can only compare VirtNetworkRangeRes to others of the same kind
	res, ok := r.(*VirtNetworkRangeRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Network != res.Network {
		return fmt.Errorf("the Network field differs")
	}
	if obj.Start != res.Start {
		return fmt.Errorf("the Start differs")
	}
	if obj.End != res.End {
		return fmt.Errorf("the End differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *VirtNetworkRangeRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes VirtNetworkRangeRes // indirection to avoid infinite recursion

	def := obj.Default()                  // get the default
	res, ok := def.(*VirtNetworkRangeRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to VirtNetworkRangeRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = VirtNetworkRangeRes(raw) // restore from indirection with type conversion!
	return nil
}
