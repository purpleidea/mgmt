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
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/plugins/allocators"
	"github.com/coredhcp/coredhcp/plugins/allocators/bitmap"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"go4.org/netipx" // extension of unmerged parts from net/netip
)

func init() {
	engine.RegisterResource("dhcp:server", func() engine.Res { return &DHCPServerRes{} })
	engine.RegisterResource("dhcp:host", func() engine.Res { return &DHCPHostRes{} })
	engine.RegisterResource("dhcp:range", func() engine.Res { return &DHCPRangeRes{} })

	if _, err := time.ParseDuration(DHCPDefaultLeaseTime); err != nil {
		panic("invalid duration for DHCPDefaultLeaseTime constant")
	}
}

const (
	// DHCPDefaultLeaseTime is the default lease time used when one was not
	// specified explicitly.
	DHCPDefaultLeaseTime = "10m" // common default from dhcpd
)

// DHCPServerRes is a simple dhcp server resource. It responds to dhcp client
// requests, but does not actually apply any state. The name is used as the
// address to listen on, unless the Address field is specified, and in that case
// it is used instead. The resource can offer up dhcp client leases from any
// number of dhcp:host resources which will get autogrouped into this resource
// at runtime.
//
// This server is not meant as a featureful replacement for the venerable dhcpd,
// but rather as a simple, dynamic, integrated alternative for bootstrapping new
// machines and clusters in an elegant way.
//
// TODO: Add autoedges between the Interface and any identically named NetRes.
type DHCPServerRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // TODO: add autoedge support
	traits.Groupable // can have DHCPHostRes and more, grouped into it

	init *engine.Init

	// Address is the listen address to use for the dhcp server. It is
	// common to use `:67` (the standard) to listen on UDP port 67 on all
	// addresses.
	Address string `lang:"address" yaml:"address"`

	// Interface is interface to bind to. For example `eth0` for the common
	// case. You may leave this field blank to not run any specific binding.
	// XXX: You need to actually specify an interface here at the moment. :(
	// BUG: https://github.com/insomniacslk/dhcp/issues/372
	Interface string `lang:"interface" yaml:"interface"`

	// ServerID is a unique IPv4 identifier for this server as specified in
	// the DHCPv4 protocol. It is almost always the IP address of the DHCP
	// server. If you don't specify this, then we will attempt to determine
	// it from the specified interface. If it is set to the empty string,
	// then this won't be set in the DHCP protocol, and your DHCP server
	// might not work as you intend. Otherwise, if a valid value is
	// specified, then this will be used as long as it validates correctly.
	// Please note that if you attempt to automatically determine this from
	// the specified interface, then this only happens at runtime when the
	// first DHCP request needs this or during CheckApply, either of which
	// could fail if for some reason it is not available.
	ServerID *string `lang:"serverid" yaml:"serverid"`

	// LeaseTime is the default lease duration in a format that is parseable
	// by the golang time.ParseDuration function, for example "60s" or "10m"
	// or "1h42m13s". If it is unspecified, then a default will be used. If
	// the empty string is specified, then no lease time will be set in the
	// DHCP protocol, and your DHCP server might not work as you intend.
	LeaseTime *string `lang:"leasetime" yaml:"leasetime"`

	// DNS represents a list of DNS servers to offer to the DHCP client.
	// XXX: Is it mandatory? https://github.com/insomniacslk/dhcp/issues/359
	DNS []string `lang:"dns" yaml:"dns"`

	// Routers represents a list of routers to offer to the DHCP client. It
	// is most common to only specify one unless you know what you're doing.
	Routers []string `lang:"routers" yaml:"routers"`

	// NBP is the network boot program URL. This is used for the tftp server
	// name and the boot file name. For example, you might use:
	// tftp://192.0.2.13/pxelinux.0 for a common bios, pxe boot setup. Note
	// that the "scheme" prefix is required, and that it's impossible to
	// specify a file that doesn't begin with a leading slash. If you wish
	// to specify a "root less" file (common for legacy tftp setups) then
	// you can use this feature in conjunction with the NBPPath parameter.
	// For DHCPv4, the scheme must be "tftp". This values is used as the
	// default for all dhcp:host resources. You can specify this here, and
	// the NBPPath per-resource and they will successfully combine.
	NBP string `lang:"nbp" yaml:"nbp"`

	// These private fields are ordered in the handler order, the above
	// public fields are ordered in the human logical order.
	leaseTime   time.Duration
	sidMutex    *sync.Mutex // guards the serverID field
	serverID    net.IP      // can be nil
	dnsServers4 []net.IP
	routers4    []net.IP

	//mutex *sync.RWMutex

	// Global pool where allocated resources are tracked.
	//reservedMacs map[string]engine.Res // net.HardwareAddr is not comparable :(
	reservedIPs map[netip.Addr]engine.Res // track which res reserved

	// TODO: add in ipv6 support here or in a separate resource?
}

// Default returns some sensible defaults for this resource.
func (obj *DHCPServerRes) Default() engine.Res {
	return &DHCPServerRes{}
}

// getAddress returns the actual address to use. When Address is not specified,
// we use the Name.
func (obj *DHCPServerRes) getAddress() string {
	if obj.Address != "" {
		return obj.Address
	}
	return obj.Name()
}

// getServerID returns the expected server identifier that we should use, based
// on our obj.ServerID, obj.Interface and Address value. This function should
// only return (nil, nil) if the user requested we skip the server id process.
func (obj *DHCPServerRes) getServerID() (net.IP, error) {
	// First see if the server id was specified explicitly...
	if obj.ServerID != nil {
		id := *obj.ServerID
		if id == "" {
			return nil, nil // skip!
		}
		ip := net.ParseIP(id)
		if ip == nil {
			// We're allowed to fail here, because you can either
			// specify a valid server id, or omit it, but you can
			// never pass in invalid data for no reason at all...
			return nil, fmt.Errorf("the ServerID is not a valid IP: %s", id)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("the ServerID is not a valid v4 address: %s", id)
		}
		return ip, nil
	}

	// Next use the host part of the address if it was specified...
	if host, _, err := net.SplitHostPort(obj.getAddress()); err == nil && host != "" {
		ip := net.ParseIP(host)
		if ip == nil {
			// We're allowed to fail here, because you can either
			// specify a valid server id, or omit it, but you can
			// never pass in invalid data for no reason at all...
			return nil, fmt.Errorf("the Address is not a valid IP: %s", host)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("the Address is not a valid v4 address: %s", host)
		}
		return ip, nil
	}

	// Try and lookup the ServerID automatically if we can, otherwise fail.

	// TODO: is there a way to determine this without Interface being set?
	// NOTE: we could look on the first interface if there is only one, but
	// what if an earlier graph operation created that interface in our run?
	if obj.Interface == "" {
		return nil, fmt.Errorf("can't get ServerID with an empty Interface")
	}

	// From `man dhcpd.conf`:
	// The default value is the first IP address associated with the
	// physical network interface on which the request arrived. The usual
	// case where the server-identifier statement needs to be sent is when a
	// physical interface has more than one IP address...

	iface, err := net.InterfaceByName(obj.Interface) // *net.Interface
	if err != nil {
		return nil, errwrap.Wrapf(err, "error finding interface: %s", obj.Interface)
	}
	if iface == nil {
		return nil, fmt.Errorf("unexpected nil iface")
	}

	a, err := iface.Addrs()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error getting addrs from interface: %s", iface.Name)
	}
	if len(a) == 0 { // add a better error message in this scenario
		return nil, fmt.Errorf("no addrs were found on %s", iface.Name)
	}
	if obj.init.Debug {
		obj.init.Logf("got %d addrs from %s", len(a), iface.Name)
		for _, addr := range a {
			obj.init.Logf("addr: %s", addr.String())
		}
	}

	var ip net.IP
	for _, addr := range a {
		// we're only interested in the strings (not the network)
		// NOTE: it's weird you can't get net.IP directly :/ Probably
		// because "the exact form and meaning of the strings is up to
		// the implementation".
		s := addr.String()

		// Parse by two different methods, since we don't know if it
		// contains a CIDR suffix or not...

		ip = net.ParseIP(s)
		if ip == nil {
			var err error
			ip, _, err = net.ParseCIDR(s)
			if err != nil || ip == nil {
				continue // nothing found
			}
		}

		// If we reached here, we have a potential IP...
		if ip.To4() == nil {
			ip = nil
			continue // take the first ipv4 address, or nothing...
		}

		break // we only care about the first one
	}
	if ip == nil {
		return nil, fmt.Errorf("no valid IPv4 addrs were found")
	}

	return ip, nil
}

// Validate checks if the resource data structure was populated correctly.
func (obj *DHCPServerRes) Validate() error {
	// FIXME: https://github.com/insomniacslk/dhcp/issues/372
	if obj.Interface == "" {
		return fmt.Errorf("the Interface is empty")
	}

	if obj.getAddress() == "" {
		return fmt.Errorf("the Address is empty")
	}

	// Ensure this format is valid for when we parse it later.
	host, _, err := net.SplitHostPort(obj.getAddress())
	if err != nil {
		return errwrap.Wrapf(err, "the Address is in an invalid format: %s", obj.getAddress())
	}
	if host != "" {
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("the Address is not a valid IP: %s", host)
		}
		if ip.To4() == nil {
			return fmt.Errorf("the Address is not a valid v4 address: %s", host)
		}
	}

	// TODO: is there a way to determine this without Interface being set?
	// NOTE: we could look on the first interface if there is only one, but
	// what if an earlier graph operation created that interface in our run?
	if obj.ServerID == nil && (obj.Interface == "" && host == "") {
		return fmt.Errorf("can't determine ServerID automatically without Interface or host Address")
	}

	// We only validate the ServerID if it's specified and not empty.
	// We can't call getServerID because it does run-time dependent checks.
	if obj.ServerID != nil && *obj.ServerID != "" {
		// modified from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/serverid/plugin.go#L101
		serverID := net.ParseIP(*obj.ServerID)
		if serverID == nil {
			return fmt.Errorf("%s is an invalid or empty IP address", *obj.ServerID)
		}
		if serverID.To4() == nil {
			return fmt.Errorf("%s is not a valid IPv4 address", *obj.ServerID)
		}
	}

	// We only validate the LeaseTime if it's specified and not empty.
	if obj.LeaseTime != nil && *obj.LeaseTime != "" {
		// modified from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/leasetime/plugin.go#L49
		_, err := time.ParseDuration(*obj.LeaseTime)
		if err != nil {
			return errwrap.Wrapf(err, "invalid duration: %s", *obj.LeaseTime)
		}
	}

	// Validate the DNS servers.
	// modified from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/dns/plugin.go#L52
	for _, ip := range obj.DNS {
		if dns := net.ParseIP(ip); dns.To4() == nil {
			return fmt.Errorf("expected a DNS server address, got: %s", ip)
		}
	}

	// Validate the routers.
	// modified from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/router/plugin.go#L42
	for _, ip := range obj.Routers {
		if router := net.ParseIP(ip); router.To4() == nil {
			return fmt.Errorf("expected a router address, got: %s", ip)
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *DHCPServerRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// NOTE: If we don't Init anything that's autogrouped, then it won't
	// even get an Init call on it.
	// TODO: should we do this in the engine? Do we want to decide it here?
	for _, res := range obj.GetGroup() { // grouped elements
		if err := res.Init(init); err != nil {
			return errwrap.Wrapf(err, "autogrouped Init failed")
		}
	}

	// Ensure the lease time is valid before we try and use it.
	if obj.LeaseTime == nil || *obj.LeaseTime != "" {
		leaseTime := DHCPDefaultLeaseTime
		if obj.LeaseTime != nil {
			leaseTime = *obj.LeaseTime
		}

		var err error
		if obj.leaseTime, err = time.ParseDuration(leaseTime); err != nil {
			return errwrap.Wrapf(err, "unexpected invalid duration: %s", leaseTime)
		}
	}

	obj.sidMutex = &sync.Mutex{}
	// We can't do this here, because our network might not be up yet, and
	// if this happens in Init, that's before a Net resource might do it!
	//var err error
	//if obj.serverID, err = obj.getServerID(); err != nil {
	//	return errwrap.Wrapf(err, "could not determine the ServerID")
	//}

	obj.dnsServers4 = []net.IP{}
	for _, ip := range obj.DNS {
		dns := net.ParseIP(ip).To4()
		if dns == nil {
			return fmt.Errorf("unexpected invalid DNS server address, got: %s", ip)
		}
		obj.dnsServers4 = append(obj.dnsServers4, dns)
	}

	obj.routers4 = []net.IP{}
	for _, ip := range obj.Routers {
		router := net.ParseIP(ip).To4()
		if router == nil {
			return fmt.Errorf("unexpected invalid router address, got: %s", ip)
		}
		obj.routers4 = append(obj.routers4, router)
	}

	//obj.mutex = &sync.RWMutex{}
	//obj.mutex.RLock()

	//obj.reservedMacs = make(map[string]engine.Res)
	obj.reservedIPs = make(map[netip.Addr]engine.Res)

	for _, x := range obj.GetGroup() { // grouped elements
		switch res := x.(type) { // convert from Res
		case *DHCPHostRes:

			// TODO: reserve res.Mac as well

			addr, ok := netip.AddrFromSlice(res.ipv4Addr) // net.IP -> netip.Addr
			if !ok {
				// programming error
				return fmt.Errorf("could not convert ip: %s", res.ipv4Addr)
			}
			//addr := res.ipv4Addr // TODO: once ported to netip.Addr
			if r, exists := obj.reservedIPs[addr]; exists {
				// TODO: Could we do all of this in Validate() ?
				return fmt.Errorf("res %s already reserved ip: %s", r, addr)
			}

			obj.reservedIPs[addr] = res // reserve!

		case *DHCPRangeRes:
		}
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *DHCPServerRes) Cleanup() error {
	// NOTE: if this ever panics, it might mean the engine is running Close
	// before Watch finishes exiting, which is an engine bug in that code...
	//obj.mutex.RUnlock()
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DHCPServerRes) Watch(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", obj.getAddress()) // *net.UDPAddr
	if err != nil {
		return errwrap.Wrapf(err, "could not resolve address")
	}

	//conn, err := net.ListenUDP("udp", addr)
	//if err != nil {
	//	return errwrap.Wrapf(err, "could not start listener")
	//}
	//defer conn.Close()

	opts := []server4.ServerOpt{}

	// This is the variant for the simple interface as seen in:
	// https://github.com/insomniacslk/dhcp/pull/373
	//logf := func(format string, v ...interface{}) {
	//	obj.init.Logf("dhcpv4: "+format, v...)
	//}
	//logfOpt := server4.WithLogf(logf) // wrap the server logging...
	//opts = append(opts, logfOpt)

	newLogger := &overEngineeredLogger{
		logf: func(format string, v ...interface{}) {
			obj.init.Logf(format, v...)
		},
	}
	logOpt := server4.WithLogger(newLogger)
	opts = append(opts, logOpt)

	server, err := server4.NewServer(obj.Interface, addr, obj.handler4(), opts...)
	if err != nil {
		return errwrap.Wrapf(err, "could not start listener") // it's inside
	}

	obj.init.Running() // when started, notify engine that we're running
	//defer obj.mutex.RLock()
	//obj.mutex.RUnlock() // it's safe to let CheckApply proceed

	var closeError error
	closeSignal := make(chan struct{})

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(closeSignal)

		err := server.Serve() // blocks until Close() is called I hope!
		// TODO: getting this error is probably a bug, please see:
		// https://github.com/insomniacslk/dhcp/issues/376
		isClosing := errors.Is(err, net.ErrClosed)
		if err == nil || isClosing {
			return
		}
		// if this returned on its own, then closeSignal can be used...
		closeError = errwrap.Wrapf(err, "the server errored")
	}()
	defer server.Close()

	startupChan := make(chan struct{})
	close(startupChan) // send one initial signal

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("Looping...")
		}

		select {
		case <-startupChan:
			startupChan = nil
			send = true

		case <-closeSignal: // something shut us down early
			return closeError

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// sidCheckApply runs the server ID cache operation in CheckApply, which can
// help CheckApply fail before the handler runs, so at least we see an error.
func (obj *DHCPServerRes) sidCheckApply(ctx context.Context, apply bool) (bool, error) {
	// Mutex guards the cached obj.serverID value.
	defer obj.sidMutex.Unlock()
	obj.sidMutex.Lock()

	// We've been explicitly asked to skip this handler.
	if obj.ServerID != nil && *obj.ServerID == "" {
		return true, nil
	}

	if obj.serverID == nil { // lookup the server ID and cache it here...
		var err error
		if obj.serverID, err = obj.getServerID(); err != nil {
			obj.init.Logf("could not determine the ServerID during CheckApply")
			return false, err
		}
	}

	return true, nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
// It does however check that certain runtime requirements (such as the Root dir
// existing if one was specified) are fulfilled.
func (obj *DHCPServerRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	//// We don't want the initial CheckApply to return true until the Watch
	//// has started up, so we must block here until that's the case...
	//ch := make(chan struct{})
	//go func() {
	//	// XXX: this goroutine leaks if CheckApply runs right before the
	//	// Watch method exits on error. And this could deadlock it too!
	//	defer close(ch)
	//	defer obj.mutex.Unlock()
	//	obj.mutex.Lock() // can't acquire this lock until Watch startup RUnlock
	//
	//}()
	//select {
	//case <-ch:
	////case <-obj.interruptChan: // TODO: if we ever support InterruptableRes
	//case <-obj.init.DoneCtx.Done(): // closed by the engine to signal shutdown
	//}

	// Cheap runtime validation!
	checkOK := true
	if c, err := obj.sidCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil // almost always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DHCPServerRes) Cmp(r engine.Res) error {
	// we can only compare DHCPServerRes to others of the same resource kind
	res, ok := r.(*DHCPServerRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Address != res.Address {
		return fmt.Errorf("the Address differs")
	}
	if obj.Interface != res.Interface {
		return fmt.Errorf("the Interface differs")
	}

	if (obj.ServerID == nil) != (res.ServerID == nil) { // xor
		return fmt.Errorf("the ServerID differs")
	}
	if obj.ServerID != nil && res.ServerID != nil {
		if *obj.ServerID != *res.ServerID { // compare the strings
			return fmt.Errorf("the contents of ServerID differ")
		}
	}

	// Be sneaky and compare the actual values. Eg: 60s vs 1m are the same!
	objIsEmptyLeaseTime := obj.LeaseTime != nil && *obj.LeaseTime == ""
	resIsEmptyLeaseTime := res.LeaseTime != nil && *res.LeaseTime == ""

	// if objIsEmptyLeaseTime && !resIsEmptyLeaseTime : FAIL
	// if !objIsEmptyLeaseTime && resIsEmptyLeaseTime : FAIL
	if objIsEmptyLeaseTime != resIsEmptyLeaseTime {
		return fmt.Errorf("the LeaseTime differs")
	}

	// if objIsEmptyLeaseTime && resIsEmptyLeaseTime : SKIP
	// if !objIsEmptyLeaseTime && !resIsEmptyLeaseTime : COMPARE
	if !objIsEmptyLeaseTime && !resIsEmptyLeaseTime {
		objLeaseTime := DHCPDefaultLeaseTime
		resLeaseTime := DHCPDefaultLeaseTime
		if obj.LeaseTime != nil {
			objLeaseTime = *obj.LeaseTime
		}
		if res.LeaseTime != nil {
			resLeaseTime = *res.LeaseTime
		}
		d1, err1 := time.ParseDuration(objLeaseTime)
		d2, err2 := time.ParseDuration(resLeaseTime)
		// we can only compare this way if they both parse correctly...
		if err1 == nil && err2 == nil {
			// TODO: should we use Nanoseconds or Seconds instead?
			// NOTE: Seconds are the sane unit for DHCP, and
			// Nanoseconds are the internal representation of the
			// Duration value. LeaseTime option precision is in sec.
			//if d1.Milliseconds() != d2.Milliseconds() {
			if d1.Seconds() != d2.Seconds() {
				return fmt.Errorf("the duration of LeaseTime differs")
			}

		} else if objLeaseTime != resLeaseTime { // plain string cmp
			return fmt.Errorf("the contents of LeaseTime differs")
		}
	}

	if len(obj.DNS) != len(res.DNS) {
		return fmt.Errorf("the number of DNS servers differs")
	}
	for i, x := range obj.DNS {
		if dns := res.DNS[i]; x != dns {
			return fmt.Errorf("the DNS server at index %d differs", i)
		}
	}

	if len(obj.Routers) != len(res.Routers) {
		return fmt.Errorf("the number of routers differs")
	}
	for i, x := range obj.Routers {
		if router := res.Routers[i]; x != router {
			return fmt.Errorf("the router at index %d differs", i)
		}
	}

	if len(obj.NBP) != len(res.NBP) {
		return fmt.Errorf("the NBP field differs")
	}

	return nil
}

// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
// TODO: should this copy internal state?
func (obj *DHCPServerRes) Copy() engine.CopyableRes {
	var serverID *string
	if obj.ServerID != nil {
		s := *obj.ServerID // copy
		serverID = &s
	}
	var leaseTime *string
	if obj.LeaseTime != nil {
		x := *obj.LeaseTime // copy
		leaseTime = &x
	}
	dns := []string{}
	for _, x := range obj.DNS {
		dns = append(dns, x)
	}
	routers := []string{}
	for _, x := range obj.Routers {
		routers = append(routers, x)
	}
	return &DHCPServerRes{
		Address:   obj.Address,
		Interface: obj.Interface,
		ServerID:  serverID,
		LeaseTime: leaseTime,
		DNS:       dns,
		Routers:   routers,
		NBP:       obj.NBP,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *DHCPServerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DHCPServerRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*DHCPServerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DHCPServerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DHCPServerRes(raw) // restore from indirection with type conversion!
	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *DHCPServerRes) GroupCmp(r engine.GroupableRes) error {
	res1, ok1 := r.(*DHCPHostRes) // different from what we usually do!
	if ok1 {
		// If the dhcp host resource has the Server field specified,
		// then it must match against our name field if we want it to
		// group with us.
		if res1.Server != "" && res1.Server != obj.Name() {
			return fmt.Errorf("resource groups with a different server name")
		}

		return nil
	}

	res2, ok2 := r.(*DHCPRangeRes) // different from what we usually do!
	if ok2 {
		// If the dhcp range resource has the Server field specified,
		// then it must match against our name field if we want it to
		// group with us.
		if res2.Server != "" && res2.Server != obj.Name() {
			return fmt.Errorf("resource groups with a different server name")
		}

		return nil
	}

	return fmt.Errorf("resource is not the right kind")
}

// leasetimeHandler4 handles DHCPv4 packets for the leasetime component.
// Modified from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/leasetime/plugin.go#L32
func (obj *DHCPServerRes) leasetimeHandler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	// We've been explicitly asked to skip this handler.
	if obj.LeaseTime != nil && *obj.LeaseTime == "" {
		return resp, false
	}

	if req.OpCode != dhcpv4.OpcodeBootRequest {
		return resp, false
	}
	// Set lease time unless it has already been set.
	if !resp.Options.Has(dhcpv4.OptionIPAddressLeaseTime) {
		resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(obj.leaseTime))
	}

	return resp, false
}

// serverIDHandler4 handles DHCPv4 packets for the serverid component. Modified
// from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/serverid/plugin.go#L78
func (obj *DHCPServerRes) serverIDHandler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	// We've been explicitly asked to skip this handler.
	if obj.ServerID != nil && *obj.ServerID == "" {
		return resp, false
	}

	// Mutex guards the cached obj.serverID value.
	defer obj.sidMutex.Unlock()
	obj.sidMutex.Lock()
	if obj.serverID == nil { // lookup the server ID and cache it here...
		var err error
		if obj.serverID, err = obj.getServerID(); err != nil {
			obj.init.Logf("could not determine the ServerID during runtime")
			return resp, false
		}
	}

	v4ServerID := obj.serverID.To4()
	if v4ServerID == nil { // We already checked this in Validate!
		panic("unexpected nil serverID")
	}

	if req.OpCode != dhcpv4.OpcodeBootRequest {
		if obj.init.Debug {
			obj.init.Logf("not a BootRequest, ignoring")
		}
		return resp, false
	}
	if req.ServerIPAddr != nil &&
		!req.ServerIPAddr.Equal(net.IPv4zero) &&
		!req.ServerIPAddr.Equal(v4ServerID) {
		// This request is not for us, drop it.
		if obj.init.Debug {
			obj.init.Logf("requested server ID does not match this server's ID. Got %v, want %v", req.ServerIPAddr, v4ServerID)
		}
		return nil, true
	}

	resp.ServerIPAddr = make(net.IP, net.IPv4len)
	copy(resp.ServerIPAddr[:], v4ServerID)
	resp.UpdateOption(dhcpv4.OptServerIdentifier(v4ServerID))

	return resp, false
}

// dnsHandler4 handles DHCPv4 packets for the DNS component. Modified from:
// https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/dns/plugin.go#L79
// XXX: Is it mandatory? https://github.com/insomniacslk/dhcp/issues/359
func (obj *DHCPServerRes) dnsHandler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	if len(obj.dnsServers4) == 0 {
		return resp, false // skip it
	}

	if req.IsOptionRequested(dhcpv4.OptionDomainNameServer) {
		resp.Options.Update(dhcpv4.OptDNS(obj.dnsServers4...))
	}
	return resp, false
}

// routerHandler4 handles DHCPv4 packets for the router component. Modified
// from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/router/plugin.go#L61
func (obj *DHCPServerRes) routerHandler4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	if len(obj.routers4) == 0 {
		return resp, false // skip it
	}

	resp.Options.Update(dhcpv4.OptRouter(obj.routers4...))
	return resp, false
}

// handler4 handles all the incoming requests from IPv4 clients.
func (obj *DHCPServerRes) handler4() func(net.PacketConn, net.Addr, *dhcpv4.DHCPv4) {

	// NOTE: this is similar to MainHandler4 in coredhcp. Keep it in sync...
	return func(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
		// req is the incoming message from the dhcp client
		// peer is who we're replying to (often a broadcast address)
		obj.init.Logf("received a DHCPv4 packet from: %s", req.ClientHWAddr.String())
		if obj.init.Debug {
			obj.init.Logf("received from DHCPv4 peer: %s", peer)
			obj.init.Logf("received a DHCPv4 packet: %s", req.Summary())
		}

		var (
			resp, tmp *dhcpv4.DHCPv4
			err       error
			stop      bool
		)
		if req.OpCode != dhcpv4.OpcodeBootRequest {
			obj.init.Logf("handler4: unsupported opcode %d. Only BootRequest (%d) is supported", req.OpCode, dhcpv4.OpcodeBootRequest)
			return
		}
		tmp, err = dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			obj.init.Logf("handler4: failed to build reply: %v", err)
			return
		}
		// D-O-R-A
		switch mt := req.MessageType(); mt {
		case dhcpv4.MessageTypeDiscover:
			tmp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
		case dhcpv4.MessageTypeRequest:
			tmp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
		case dhcpv4.MessageTypeDecline:
			// If mask is not set, some DHCP clients will DECLINE.
			obj.init.Logf("handler4: Unhandled decline message: %+v", req)
			return
		default:
			obj.init.Logf("handler4: Unhandled message type: %v", mt)
			return
		}

		handlers := []handler.Handler4{}

		// These are the core handlers from our own server struct. The
		// order of these matters in theory, and possibly in practice.
		handlers = append(handlers, obj.leasetimeHandler4)
		handlers = append(handlers, obj.serverIDHandler4)
		handlers = append(handlers, obj.dnsHandler4)
		handlers = append(handlers, obj.routerHandler4)
		//handlers = append(handlers, obj.netmaskHandler4) // in host

		// These handlers arrive from autogrouping other resources in.
		hostHandlers := []handler.Handler4{}
		rangeHandlers := []handler.Handler4{}

		// Look through the autogrouped resources!
		// TODO: can we improve performance by only searching here once?
		for _, x := range obj.GetGroup() { // grouped elements
			if obj.init.Debug {
				obj.init.Logf("Got grouped resource: %s", x.String())
			}

			// TODO: any kind of filtering could go here...

			switch res := x.(type) { // convert from Res
			case *DHCPHostRes:
				// NOTE: this is not changed if we do send/recv.
				// If we want to support that, we need to patch!
				data := &HostData{
					NBP: obj.NBP,
				}
				h, err := res.handler4(data)
				if err != nil {
					// This should rarely error, so a log
					// message and skipping the handler is
					// good enough here.
					obj.init.Logf("%s: invalid handler: %v", x.String(), err)
					continue
				}
				hostHandlers = append(hostHandlers, h)

			case *DHCPRangeRes:
				// TODO: should all handlers have the same signature?
				data := &HostData{
					NBP: obj.NBP,
				}
				h, err := res.handler4(data)
				if err != nil {
					// This should rarely error, so a log
					// message and skipping the handler is
					// good enough here.
					obj.init.Logf("%s: invalid handler: %v", x.String(), err)
					continue
				}
				rangeHandlers = append(rangeHandlers, h)

			default:
				continue
			}
		}

		// NOTE: the order of these plugins matter, but we'll just
		// hardcode something for now. It could be configurable later.

		for _, h := range hostHandlers {
			handlers = append(handlers, h)
		}
		for _, h := range rangeHandlers {
			handlers = append(handlers, h)
		}

		resp = tmp
		for _, handler := range handlers {
			resp, stop = handler(req, resp)
			if stop {
				break
			}
		}

		if resp != nil {
			if obj.init.Debug {
				// NOTE: This is very useful for debugging!
				obj.init.Logf("sending a DHCPv4 packet: %s", resp.Summary())
			}
			var peer net.Addr
			if !req.GatewayIPAddr.IsUnspecified() {
				// TODO: make RFC8357 compliant
				peer = &net.UDPAddr{IP: req.GatewayIPAddr, Port: dhcpv4.ServerPort}
			} else if resp.MessageType() == dhcpv4.MessageTypeNak {
				peer = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
			} else if !req.ClientIPAddr.IsUnspecified() {
				peer = &net.UDPAddr{IP: req.ClientIPAddr, Port: dhcpv4.ClientPort}
			} else if req.IsBroadcast() {
				peer = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
			} else {
				// FIXME: we're supposed to unicast to a specific *L2* address, and an L3
				// address that's not yet assigned.
				// I don't know how to do that with this API...
				//peer = &net.UDPAddr{IP: resp.YourIPAddr, Port: dhcpv4.ClientPort}
				obj.init.Logf("handler4: Cannot handle non-broadcast-capable unspecified peers in an RFC-compliant way. Response will be broadcast")

				peer = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
			}

			if _, err := conn.WriteTo(resp.ToBytes(), peer); err != nil {
				obj.init.Logf("handler4: conn.Write to %v failed: %v", peer, err)
			}

		} else {
			obj.init.Logf("handler4: dropping request because response is nil")
		}

		return // department of redundancy department
	}
}

// DHCPHostRes is a representation of a static host assignment in DHCP.
type DHCPHostRes struct {
	traits.Base // add the base methods without re-implementation
	//traits.Edgeable // XXX: add autoedge support
	traits.Groupable // can be grouped into DHCPServerRes

	init *engine.Init

	// Server is the name of the dhcp server resource to group this into. If
	// it is omitted, and there is only a single dhcp resource, then it will
	// be grouped into it automatically. If there is more than one main dhcp
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// Mac is the mac address of the host in lower case and separated with
	// colons.
	Mac string `lang:"mac" yaml:"mac"`

	// IP is the IPv4 address with the CIDR suffix. The suffix is required
	// because it specifies the netmask to be used in the DHCPv4 protocol.
	// For example, you might specify 192.0.2.42/24 which represents a mask
	// of 255.255.255.0 that will be sent.
	IP string `lang:"ip" yaml:"ip"`

	// NBP is the network boot program URL. This is used for the tftp server
	// name and the boot file name. For example, you might use:
	// tftp://192.0.2.13/pxelinux.0 for a common bios, pxe boot setup. Note
	// that the "scheme" prefix is required, and that it's impossible to
	// specify a file that doesn't begin with a leading slash. If you wish
	// to specify a "root less" file (common for legacy tftp setups) then
	// you can use this feature in conjunction with the NBPPath parameter.
	// For DHCPv4, the scheme must be "tftp".
	NBP string `lang:"nbp" yaml:"nbp"`

	// NBPPath overrides the path that is sent for the nbp protocols. By
	// default it is taken from parsing a URL in NBP, but this can override
	// that. This is useful if you require a path that doesn't start with a
	// slash. This is sometimes desirable for legacy tftp setups.
	NBPPath string `lang:"nbp_path" yaml:"nbp_path"`

	ipv4Addr net.IP // XXX: port to netip.Addr
	ipv4Mask net.IPMask
	opt66    *dhcpv4.Option
	opt67    *dhcpv4.Option
}

// Default returns some sensible defaults for this resource.
func (obj *DHCPHostRes) Default() engine.Res {
	return &DHCPHostRes{}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *DHCPHostRes) Validate() error {
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

	// If we didn't require the CIDR, then we could do this...
	//if obj.IP != "" && net.ParseIP(obj.IP) == nil {
	//	return fmt.Errorf("the IP was not a valid address")
	//}
	//if obj.IP != "" && net.ParseIP(obj.IP).To4() == nil {
	//	return fmt.Errorf("only IPv4 is currently supported")
	//}

	// validate the network boot program URL
	if obj.NBP != "" {
		u, err := url.Parse(obj.NBP)
		if err != nil {
			return errwrap.Wrapf(err, "invalid nbp URL")
		}
		if u.Scheme == "" {
			return fmt.Errorf("missing nbp scheme")
		}
		// TODO: remove this check when we support DHCPv6
		if u.Scheme != "tftp" {
			return fmt.Errorf("the scheme must be `tftp` for DHCPv4")
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *DHCPHostRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	ipv4Addr, ipv4Net, err := net.ParseCIDR(obj.IP)
	if err != nil {
		return errwrap.Wrapf(err, "unexpected invalid IP/CIDR address")
	}
	if ipv4Addr.To4() == nil {
		return fmt.Errorf("unexpectedly missing an IPv4 address")
	}

	obj.ipv4Addr = ipv4Addr
	obj.ipv4Mask = ipv4Net.Mask

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *DHCPHostRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *DHCPHostRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *DHCPHostRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DHCPHostRes) Cmp(r engine.Res) error {
	// we can only compare DHCPHostRes to others of the same resource kind
	res, ok := r.(*DHCPHostRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}
	if obj.Mac != res.Mac {
		return fmt.Errorf("the Mac differs")
	}
	if obj.IP != res.IP {
		return fmt.Errorf("the IP differs")
	}
	if obj.NBP != res.NBP {
		return fmt.Errorf("the NBP differs")
	}
	if obj.NBPPath != res.NBPPath {
		return fmt.Errorf("the NBPPath differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *DHCPHostRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DHCPHostRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*DHCPHostRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DHCPHostRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DHCPHostRes(raw) // restore from indirection with type conversion!
	return nil
}

// handler4 returns the handler for the host resource. It gets called from the
// main handler4 function in the dhcp server resource. This combines the concept
// of multiple "plugins" inside of coredhcp. It includes "file" and also "nbp"
// and others.
func (obj *DHCPHostRes) handler4(data *HostData) (func(*dhcpv4.DHCPv4, *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool), error) {
	nbp := ""
	if data != nil {
		nbp = data.NBP // from server
	}
	if obj.NBP != "" { // host-specific override
		nbp = obj.NBP
	}
	result, err := url.Parse(nbp)
	if err != nil {
		// this should have been checked in Validate :/
		return nil, errwrap.Wrapf(err, "unexpected invalid nbp URL")
	}
	otsn := dhcpv4.OptTFTPServerName(result.Host)
	obj.opt66 = &otsn
	p := result.Path
	if obj.NBPPath != "" { // override the path if this is specified
		p = obj.NBPPath
	}
	obfn := dhcpv4.OptBootFileName(p)
	if p != "" {
		obj.opt67 = &obfn
	}

	return func(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
		// Might not be *this* reservation, but another one...
		if obj.init.Debug {
			obj.init.Logf("comparing mac %s to %s", req.ClientHWAddr.String(), obj.Mac)
		}
		if req.ClientHWAddr.String() != obj.Mac {
			//obj.init.Logf("MAC address %s is unknown", req.ClientHWAddr.String())
			return resp, false
		}
		if obj.init.Debug || true { // TODO: should we silence this?
			obj.init.Logf("found IP address %s for MAC %s", obj.IP, req.ClientHWAddr.String())
		}

		resp.YourIPAddr = obj.ipv4Addr // TODO: make a copy for this?

		// XXX: This is done in the standalone leasetime handler for now
		//resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(obj.LeaseTime.Round(time.Second)))

		// XXX: https://tools.ietf.org/html/rfc2132#section-3.3
		// If both the subnet mask and the router option are specified
		// in a DHCP reply, the subnet mask option MUST be first.
		// If mask is not set, some DHCP clients will DECLINE.
		resp.Options.Update(dhcpv4.OptSubnetMask(obj.ipv4Mask)) // net.IPMask

		// nbp section
		if obj.opt66 != nil && req.IsOptionRequested(dhcpv4.OptionTFTPServerName) {
			resp.Options.Update(*obj.opt66)
		}
		if obj.opt67 != nil && req.IsOptionRequested(dhcpv4.OptionBootfileName) {
			resp.Options.Update(*obj.opt67)
		}
		if obj.init.Debug {
			obj.init.Logf("Added NBP %s / %s to request", obj.opt66, obj.opt67)
		}

		return resp, true
	}, nil
}

// DHCPRangeRes is a representation of a range allocator in DHCP. To declare a
// range you must specify either the `network` field or the `from` and `to`
// fields as ip with cidr's, or `from` and `to` fields without cidr's but with
// the `mask` field as either a dotted netmask or a `/number` field. If you
// specify none of these, then the resource name will be interpreted the same
// way that the `network` field os. The last ip in the range (which is often
// used as a broadcast address) is never allocated.
// TODO: Add a setting to determine if we should allocate the last address.
type DHCPRangeRes struct {
	traits.Base // add the base methods without re-implementation
	//traits.Edgeable // XXX: add autoedge support?
	traits.Groupable // can be grouped into DHCPServerRes

	init *engine.Init

	// Server is the name of the dhcp server resource to group this into. If
	// it is omitted, and there is only a single dhcp resource, then it will
	// be grouped into it automatically. If there is more than one main dhcp
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// MacMatch only allocates ip addresses if the mac address matches this
	// wildcard pattern. The default pattern of the empty string, means any
	// mac address is permitted.
	// TODO: This is not implemented at the moment.
	// TODO: Consider implementing this sort of functionality.
	// TODO: Can we use https://pkg.go.dev/path/filepath#Match ?
	//MacMatch string `lang:"macmatch" yaml:"macmatch"`

	// Network is the network number and cidr to determine the range. For
	// example, the common network range of 192.168.42.1 to 192.168.42.255
	// should have a network field here of 192.168.42.0/24. You can either
	// specify this field or `from` and `to`, but not a different
	// combination. If you don't specify any of these fields, then the
	// resource name will be parsed as if it was used here.
	Network string `lang:"network" yaml:"network"`

	// From is the start address in the range inclusive. If it is specified
	// in cidr notation, then the `mask` field must not be used. Otherwise
	// it must be used. In both situations the cidr or mask must be
	// consistent with the `to` field. If this field is used, you must not
	// use the `network` field.
	From string `lang:"from" yaml:"from"`

	// To is the end address in the range inclusive. If it is specified in
	// cidr notation, then the `mask` field must not be used. Otherwise it
	// must be used. In both situations the cidr or mask must be consistent
	// with the `from` field. If this field is used, you must not use the
	// `network` field.
	To string `lang:"to" yaml:"to"`

	// Mask is the cidr or netmask of ip addresses in the specified range.
	// This field must only be used if both `from` and `to` are specified,
	// and if neither of them specify a cidr suffix. If neither do, then the
	// mask here can be in either dotted format or, preferably, in cidr
	// format by starting with a slash.
	Mask string `lang:"mask" yaml:"mask"`

	// Skip is a list ip's in either cidr or standalone representation which
	// will be skipped and not allocated.
	Skip []string `lang:"skip" yaml:"skip"`

	// Persist should be true if you want to persist the lease information
	// to disk so that a new (or changed) invocation of this resource with
	// the same name, will regain that existing initial state at startup.
	// TODO: Add a new param to persist the data to etcd in the world API so
	// that we could have redundant dhcp servers which share the same state.
	// This would require having a distributed allocator through etcd too!
	// TODO: Consider adding a new param to erase the persisted record
	// database if any field param changes, as opposed to just looking at
	// the name field alone.
	// XXX: This is currently not implemented.
	Persist bool `lang:"persist" yaml:"persist"`

	// NBP is the network boot program URL. This is used for the tftp server
	// name and the boot file name. For example, you might use:
	// tftp://192.0.2.13/pxelinux.0 for a common bios, pxe boot setup. Note
	// that the "scheme" prefix is required, and that it's impossible to
	// specify a file that doesn't begin with a leading slash. If you wish
	// to specify a "root less" file (common for legacy tftp setups) then
	// you can use this feature in conjunction with the NBPPath parameter.
	// For DHCPv4, the scheme must be "tftp".
	NBP string `lang:"nbp" yaml:"nbp"`

	// NBPPath overrides the path that is sent for the nbp protocols. By
	// default it is taken from parsing a URL in NBP, but this can override
	// that. This is useful if you require a path that doesn't start with a
	// slash. This is sometimes desirable for legacy tftp setups.
	NBPPath string `lang:"nbp_path" yaml:"nbp_path"`

	// TODO: consider persisting this to disk (with the Local API)
	records map[string]*HostRecord // key is mac address

	// TODO: port the allocator to use the net/netip types
	// TODO: add a new allocator that can work on multiple hosts over etcd
	allocator allocators.Allocator

	// mutex guards access to records and allocator when running.
	mutex *sync.Mutex

	from netip.Addr
	to   netip.Addr
	mask net.IPMask
	skip []netip.Addr

	opt66 *dhcpv4.Option
	opt67 *dhcpv4.Option
}

// Default returns some sensible defaults for this resource.
func (obj *DHCPRangeRes) Default() engine.Res {
	return &DHCPRangeRes{}
}

// parse handles the permutations of options to parse the fields into our data
// format that we use. This is a helper function because it is used in both
// Validate and also Init. It only stores the result on no error, and if set is
// true.
func (obj *DHCPRangeRes) parse(set bool) error {
	if a, b := obj.From == "", obj.To == ""; a != b {
		return fmt.Errorf("must specify both from and to or neither")
	}

	if obj.From != "" && obj.To != "" {
		var from, to netip.Addr
		var mask net.IPMask // nil unless set

		if prefix, err := netip.ParsePrefix(obj.From); err == nil {
			from = prefix.Addr()

			ones := prefix.Bits() // set portion of the mask, -1 if invalid
			bits := 128           // ipv6
			if from.Is4() {       // ipv4
				bits = 32
			}
			mask = net.CIDRMask(ones, bits) // net.IPMask

		} else if addr, err := netip.ParseAddr(obj.From); err == nil { // without cidr
			from = addr
		}
		// else we error (caught below)
		if prefix, err := netip.ParsePrefix(obj.To); err == nil {
			to = prefix.Addr()

			ones := prefix.Bits() // set portion of the mask, -1 if invalid
			bits := 128           // ipv6
			if to.Is4() {         // ipv4
				bits = 32
			}
			mask = net.CIDRMask(ones, bits) // net.IPMask

		} else if addr, err := netip.ParseAddr(obj.To); err == nil { // without cidr
			to = addr
		}
		// else we error (caught below)

		if !from.Is4() || !to.Is4() {
			// TODO: support ipv6
			return fmt.Errorf("only ipv4 is supported at this time")
		}

		if a, b := obj.Mask == "", mask == nil; a == b {
			return fmt.Errorf("mask must be specified somehow")
		}

		// weird "/cidr" form
		// TODO: Do we want to allow this form?
		if obj.Mask != "" && strings.HasPrefix(obj.Mask, "/") {

			ones, err := strconv.Atoi(obj.Mask[1:])
			if err != nil {
				return fmt.Errorf("invalid cidr suffix: %s", obj.Mask)
			}

			// The range is [0,32] for IPv4 or [0,128] for IPv6.
			if ones < 0 || ones > 128 {
				return fmt.Errorf("invalid cidr: %s", obj.Mask)
			}

			bits := 128                 // ipv6
			if from.Is4() && to.Is4() { // ipv4
				if ones > 32 {
					return fmt.Errorf("invalid ipv4 cidr: %s", obj.Mask)
				}
				bits = 32
			}
			mask = net.CIDRMask(ones, bits) // net.IPMask

		} else if obj.Mask != "" { // standard 255.255.255.0 form

			ip := net.ParseIP(obj.Mask)
			if ip == nil || ip.IsUnspecified() {
				return fmt.Errorf("invalid mask: %s", obj.Mask)
			}
			ip4 := ip.To4()
			if ip4 == nil {
				return fmt.Errorf("invalid ipv4 mask: %s", obj.Mask)
			}
			mask = net.IPv4Mask(ip4[0], ip4[1], ip4[2], ip4[3])
		}

		if !checkValidNetmask(mask) { // TODO: is this needed?
			return fmt.Errorf("invalid mask: %s", mask)
		}

		if !set {
			return nil
		}

		r := netipx.IPRangeFrom(from, to) // netipx.IPRange
		obj.from = r.From()               // netip.Addr
		obj.to = r.To()                   // netip.Addr
		obj.mask = mask

		return nil
	}

	if obj.Mask != "" {
		return fmt.Errorf("mask must not be set when using network")
	}

	if obj.Network != "" {
		return obj.parseNetwork(set, obj.Network)
	}

	// try to parse based on name if nothing else is possible
	return obj.parseNetwork(set, obj.Name())
}

// parseNetwork handles the permutations of options to parse the network field
// into the data format that we use. This is a helper function because it is
// used in both Validate and also Init for both the network field and the name
// field. It only stores the result on no error, and if set is true.
func (obj *DHCPRangeRes) parseNetwork(set bool, network string) error {
	if network == "" {
		return fmt.Errorf("empty network")
	}

	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return err
	}

	ones := prefix.Bits()    // set portion of the mask, -1 if invalid
	bits := 128              // ipv6
	if prefix.Addr().Is4() { // ipv4
		bits = 32
	}
	mask := net.CIDRMask(ones, bits) // net.IPMask

	if !checkValidNetmask(mask) { // TODO: is this needed?
		return fmt.Errorf("invalid mask: %s", mask)
	}

	// XXX: Are we doing the network math here correctly? (eg with .Next())
	r := netipx.RangeOfPrefix(prefix) // netipx.IPRange
	from := r.From()                  // netip.Addr
	next := from.Next()               // skip the network addr
	if !next.IsValid() {
		return fmt.Errorf("not enough addresses") // or a bug?
	}

	if !set {
		return nil
	}

	obj.from = next // netip.Addr
	// TODO: should we use .Prev() for to?
	obj.to = r.To() // netip.Addr
	obj.mask = mask

	return nil
}

// parseSkip handles the permutations of options to parse the skip field into
// the data format that we use. This is a helper function because it is used in
// both Validate and also Init. It only stores the result on no error, and if
// set is true.
func (obj *DHCPRangeRes) parseSkip(set bool) error {
	ips := []netip.Addr{}
	for _, s := range obj.Skip {
		if prefix, err := netip.ParsePrefix(s); err == nil {
			addr := prefix.Addr()
			ips = append(ips, addr)

			// TODO: check if mask matches mask from range?
			//ones := prefix.Bits() // set portion of the mask, -1 if invalid
			//bits := 128 // ipv6
			//if addr.Is4() { // ipv4
			//	bits = 32
			//}
			//mask = net.CIDRMask(ones, bits) // net.IPMask
			continue

		} else if addr, err := netip.ParseAddr(s); err == nil { // without cidr
			ips = append(ips, addr)
			continue
		}

		return fmt.Errorf("invalid ip: %s", s)
	}

	if !set {
		return nil
	}

	obj.skip = ips

	return nil
}

// Validate checks if the resource data structure was populated correctly.
func (obj *DHCPRangeRes) Validate() error {

	//if obj.MacMatch != "" {
	// TODO: validate pattern
	//}

	// If input is false, this doesn't modify any private struct fields.
	if err := obj.parse(false); err != nil {
		return err
	}

	if err := obj.parseSkip(false); err != nil {
		return err
	}

	// validate the network boot program URL
	if obj.NBP != "" {
		u, err := url.Parse(obj.NBP)
		if err != nil {
			return errwrap.Wrapf(err, "invalid nbp URL")
		}
		if u.Scheme == "" {
			return fmt.Errorf("missing nbp scheme")
		}
		// TODO: remove this check when we support DHCPv6
		if u.Scheme != "tftp" {
			return fmt.Errorf("the scheme must be `tftp` for DHCPv4")
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *DHCPRangeRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// This sets some private struct fields if it doesn't error.
	if err := obj.parse(true); err != nil {
		return err
	}

	if err := obj.parseSkip(true); err != nil {
		return err
	}

	obj.records = make(map[string]*HostRecord)
	// TODO: consider persisting this to disk
	if obj.Persist {
		return fmt.Errorf("persist not implemented")
		//records, err := obj.load()
		//if err != nil {
		//	return nil
		//}
		//obj.records = records
	}

	from := net.IP(obj.from.AsSlice()) // netip.Addr -> net.IP
	to := net.IP(obj.to.AsSlice())
	allocator, err := bitmap.NewIPv4Allocator(from, to)
	if err != nil {
		return nil
	}
	obj.allocator = allocator
	obj.mutex = &sync.Mutex{}

	res, ok := obj.Parent().(*DHCPServerRes)
	if !ok {
		// programming error
		return fmt.Errorf("unexpected parent resource")
	}
	// TODO: res.reservedMutex?
	for addr := range res.reservedIPs {
		ip := net.IP(addr.AsSlice()) // netip.Addr -> net.IP
		hint := net.IPNet{IP: ip}
		ipNet, err := obj.allocator.Allocate(hint) // (net.IPNet, error)
		if err != nil {
			return fmt.Errorf("could not reserve ip %s: %v", ip, err)
		}
		if ip.String() != ipNet.IP.String() {
			return fmt.Errorf("requested %s, allocator returned %s", ip, ipNet.IP)
		}

		// NOTE: We don't add these to the memory or stateful record
		// store, since we have these "stored" by virtue of them being a
		// resource as code. If those changed, we'd have an out-of-date
		// stateful database!
	}

	macs := []string{}
	for k := range obj.records { // deterministic for now
		macs = append(macs, k)
	}
	sort.Strings(macs)
	for _, mac := range macs {
		record, ok := obj.records[mac]
		if !ok {
			// programming error
			return fmt.Errorf("missing record")
		}
		if !record.IP.IsValid() || record.IP.IsUnspecified() {
			// programming error
			return fmt.Errorf("bad ip in record")
		}

		// Allocate what we already chose to statically in the database.
		// NOTE: The API lets us request an ip, but it's not guaranteed.
		hint := net.IPNet{IP: net.IP(record.IP.AsSlice())} // netip.Addr -> net.IP
		ipNet, err := obj.allocator.Allocate(hint)         // (net.IPNet, error)
		if err != nil {
			return fmt.Errorf("could not re-allocate leased ip %s: %v", record.IP, err)
		}
		if record.IP.String() != ipNet.IP.String() {
			return fmt.Errorf("pre-requested %s, allocator returned %s", record.IP, ipNet.IP)
		}
	}

	for _, ip := range obj.skip {
		// Allocate what the config already chose to skip statically.
		// NOTE: The API lets us request an ip, but it's not guaranteed.
		hint := net.IPNet{IP: net.IP(ip.AsSlice())} // netip.Addr -> net.IP
		ipNet, err := obj.allocator.Allocate(hint)  // (net.IPNet, error)
		if err != nil {
			return fmt.Errorf("could not allocate skip ip %s: %v", ip, err)
		}
		if ip.String() != ipNet.IP.String() {
			return fmt.Errorf("skip-requested %s, allocator returned %s", ip, ipNet.IP)
		}
	}

	obj.init.Logf("from: %s", obj.from)
	obj.init.Logf("  to: %s", obj.to)
	obj.init.Logf("mask: %s", netmaskAsQuadString(obj.mask))

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *DHCPRangeRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *DHCPRangeRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *DHCPRangeRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	return true, nil // always succeeds, with nothing to do!
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DHCPRangeRes) Cmp(r engine.Res) error {
	// we can only compare DHCPRangeRes to others of the same resource kind
	res, ok := r.(*DHCPRangeRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}

	//if obj.MacMatch != res.MacMatch {
	//	return fmt.Errorf("the MacMatch field differs")
	//}

	if obj.Network != res.Network {
		return fmt.Errorf("the Network field differs")
	}
	if obj.From != res.From {
		return fmt.Errorf("the From field differs")
	}
	if obj.To != res.To {
		return fmt.Errorf("the To field differs")
	}
	if obj.Mask != res.Mask {
		return fmt.Errorf("the Mask field differs")
	}

	if len(obj.Skip) != len(res.Skip) {
		return fmt.Errorf("the size of Skip differs")
	}
	for i, x := range obj.Skip {
		if x != res.Skip[i] {
			return fmt.Errorf("the Skip at index %d differs", i)
		}
	}

	if obj.Persist != res.Persist {
		return fmt.Errorf("the Persist field differs")
	}

	if obj.NBP != res.NBP {
		return fmt.Errorf("the NBP differs")
	}
	if obj.NBPPath != res.NBPPath {
		return fmt.Errorf("the NBPPath differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *DHCPRangeRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DHCPRangeRes // indirection to avoid infinite recursion

	def := obj.Default()           // get the default
	res, ok := def.(*DHCPRangeRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DHCPRangeRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DHCPRangeRes(raw) // restore from indirection with type conversion!
	return nil
}

// handler4 returns the handler for the range resource. It gets called from the
// main handler4 function in the dhcp server resource. This combines the concept
// of multiple "plugins" inside of coredhcp. It includes "file" and also "nbp"
// and others.
func (obj *DHCPRangeRes) handler4(data *HostData) (func(*dhcpv4.DHCPv4, *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool), error) {
	nbp := ""
	if data != nil {
		nbp = data.NBP // from server
	}
	if obj.NBP != "" { // host-specific override
		nbp = obj.NBP
	}
	result, err := url.Parse(nbp)
	if err != nil {
		// this should have been checked in Validate :/
		return nil, errwrap.Wrapf(err, "unexpected invalid nbp URL")
	}
	otsn := dhcpv4.OptTFTPServerName(result.Host)
	obj.opt66 = &otsn
	p := result.Path
	if obj.NBPPath != "" { // override the path if this is specified
		p = obj.NBPPath
	}
	obfn := dhcpv4.OptBootFileName(p)
	if p != "" {
		obj.opt67 = &obfn
	}

	res, ok := obj.Parent().(*DHCPServerRes)
	if !ok {
		// programming error
		return nil, fmt.Errorf("unexpected parent resource")
	}

	leaseTime := res.leaseTime

	// FIXME: Run this somewhere for now, eventually it should get scheduled
	// to run in the returned duration of time. This way, it would clean old
	// peristed entries when they're stale, not when a new request comes in.
	if _, err := obj.leaseClean(); err != nil {
		return nil, errwrap.Wrapf(err, "clean error")
	}

	return func(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
		obj.mutex.Lock()
		defer obj.mutex.Unlock()

		mac := req.ClientHWAddr.String()
		//hostname := req.HostName() // TODO: is it needed in the record?

		// Incoming allocation request in our range.
		if obj.init.Debug {
			obj.init.Logf("range lookup for mac %s", mac)
		}

		update := false
		record, ok := obj.records[mac]
		if !ok {
			ipNet, err := obj.allocator.Allocate(net.IPNet{}) // (net.IPNet, error)
			if err != nil {
				obj.init.Logf("could not allocate for mac %s: %v", mac, err)
				return nil, true
			}
			// TODO: Do we need to use ipNet.Mask ?

			// TODO: do we want this complex version instead?
			//addr, ok := netipx.FromStdIP(ipNet.IP) // unmap's
			addr, ok := netip.AddrFromSlice(ipNet.IP) // net.IP -> netip.Addr
			if !ok {
				// programming error
				obj.init.Logf("could not convert ip: %s", ipNet.IP)
				return nil, true
			}

			rec := &HostRecord{
				IP:      addr,
				Expires: int(time.Now().Add(leaseTime).Unix()),
				//Hostname: hostname,
			}

			obj.records[mac] = rec
			record = rec // set it
			update = true
		} else {
			// extend lease
			expiry := time.Unix(int64(record.Expires), 0) // 0 is nsec
			if expiry.Before(time.Now().Add(leaseTime)) {
				record.Expires = int(time.Now().Add(leaseTime).Round(time.Second).Unix())
				//record.Hostname = hostname
				update = true
			}
		}

		// TODO: consider persisting this to disk
		if obj.Persist && update {
			//if err = obj.store(req.ClientHWAddr, record); err != nil {
			//	obj.init.Logf("could not store mac %s: %v", mac, err)
			//}
		}

		if obj.init.Debug || true { // TODO: should we silence this?
			obj.init.Logf("allocated ip %s for MAC %s", record.IP, mac)
		}

		resp.YourIPAddr = net.IP(record.IP.AsSlice()) // netip.Addr -> net.IP

		// XXX: This is done in the standalone leasetime handler for now
		//resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(record.Expires.Round(time.Second)))

		// XXX: https://tools.ietf.org/html/rfc2132#section-3.3
		// If both the subnet mask and the router option are specified
		// in a DHCP reply, the subnet mask option MUST be first.
		// If mask is not set, some DHCP clients will DECLINE.
		resp.Options.Update(dhcpv4.OptSubnetMask(obj.mask)) // net.IPMask

		// nbp section
		if obj.opt66 != nil && req.IsOptionRequested(dhcpv4.OptionTFTPServerName) {
			resp.Options.Update(*obj.opt66)
		}
		if obj.opt67 != nil && req.IsOptionRequested(dhcpv4.OptionBootfileName) {
			resp.Options.Update(*obj.opt67)
		}
		if obj.init.Debug {
			obj.init.Logf("Added NBP %s / %s to request", obj.opt66, obj.opt67)
		}

		return resp, true
	}, nil
}

// leaseClean frees any expired leases. It also returns the duration till the
// next expected cleaning.
func (obj *DHCPRangeRes) leaseClean() (time.Duration, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	min := time.Duration(-1)
	now := time.Now()
	expire := []string{}
	for mac, record := range obj.records { // see who is expired...
		expiry := time.Unix(int64(record.Expires), 0) // 0 is nsec
		//if !expiry.After(now) // same
		if delta := expiry.Sub(now); delta > 0 { // positive if expired
			if min == -1 { // initialize
				min = delta
			}
			min = minDuration(min, delta) // soonest time to wakeup
			continue
		}

		// it's expired
		expire = append(expire, mac)
	}

	for _, mac := range expire {
		record, exists := obj.records[mac]
		if !exists {
			// programming error
			return 0, fmt.Errorf("missing record")
		}
		free := net.IPNet{IP: net.IP(record.IP.AsSlice())} // netip.Addr -> net.IP
		err := obj.allocator.Free(free)
		delete(obj.records, mac)
		obj.init.Logf("unallocated (free) ip %s for MAC %s", record.IP, mac)
		// TODO: run the persist somewhere...
		// if obj.Persist {
		//
		//}
		if err == nil {
			continue
		}
		if dblErr, ok := err.(*allocators.ErrDoubleFree); ok {
			// programming error
			obj.init.Logf("double free programming error on: %v", dblErr.Loc)
			continue
		}
		return 0, err // actual unknown error
	}

	//if min >= 0 // schedule in `min * time.Second` seconds
	return min, nil
}

// HostRecord is some information that we store about each reservation that we
// allocated. This struct is stored as a value which is mapped to by a mac
// address key, which is why the mac address is not stored in this record.
type HostRecord struct {
	IP netip.Addr

	// Expires represents the number of seconds since the epoch that this
	// lease expires at.
	Expires int
}

// HostData is some data that each host will get made available to its handler.
type HostData struct {
	// NBP is the network boot program URL. See the resources for more docs.
	NBP string
}

// overEngineeredLogger is a helper struct that fulfills the over-engineered
// logging interface that was introduced in:
// https://github.com/insomniacslk/dhcp/pull/371/
type overEngineeredLogger struct {
	logf func(format string, v ...interface{})
}

func (obj *overEngineeredLogger) Printf(format string, v ...interface{}) {
	obj.logf(format, v...)
}

func (obj *overEngineeredLogger) PrintMessage(prefix string, message *dhcpv4.DHCPv4) {
	obj.logf("%s: %s", prefix, message)
}

func minDuration(d1, d2 time.Duration) time.Duration {
	if d1 < d2 {
		return d1
	}
	return d2
}

// from: https://github.com/coredhcp/coredhcp/blob/master/plugins/netmask/plugin.go
func checkValidNetmask(netmask net.IPMask) bool {
	netmaskInt := binary.BigEndian.Uint32(netmask)
	x := ^netmaskInt
	y := x + 1
	return (y & x) == 0
}

// netmaskAsQuadString returns a dotted-quad string giving you something like:
// 255.255.255.0 instead of ffffff00 which is what's seen when you print it now.
func netmaskAsQuadString(netmask net.IPMask) string {
	return fmt.Sprintf("%d.%d.%d.%d", netmask[0], netmask[1], netmask[2], netmask[3])
}
