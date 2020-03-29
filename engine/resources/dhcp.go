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

package resources

import (
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

func init() {
	engine.RegisterResource("dhcp:server", func() engine.Res { return &DHCPServerRes{} })
	engine.RegisterResource("dhcp:host", func() engine.Res { return &DHCPHostRes{} })
	//engine.RegisterResource("dhcp:range", func() engine.Res { return &DhcpRangeRes{} })

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

	// These private fields are ordered in the handler order, the above
	// public fields are ordered in the human logical order.
	leaseTime   time.Duration
	sidMutex    *sync.Mutex // guards the serverID field
	serverID    net.IP      // can be nil
	dnsServers4 []net.IP
	routers4    []net.IP

	//mutex *sync.RWMutex

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

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *DHCPServerRes) Close() error {
	// NOTE: if this ever panics, it might mean the engine is running Close
	// before Watch finishes exiting, which is an engine bug in that code...
	//obj.mutex.RUnlock()
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DHCPServerRes) Watch() error {
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
			obj.init.Logf("dhcpv4: "+format, v...)
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
		if err == nil {
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

		case <-obj.init.Done: // closed by the engine to signal shutdown
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
func (obj *DHCPServerRes) sidCheckApply(apply bool) (bool, error) {
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
func (obj *DHCPServerRes) CheckApply(apply bool) (bool, error) {
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
	//case <-obj.init.Done: // closed by the engine to signal shutdown
	//}

	// Cheap runtime validation!
	checkOK := true
	if c, err := obj.sidCheckApply(apply); err != nil {
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

	//res2, ok2 := r.(*DhcpRangeRes) // different from what we usually do!
	//if ok2 {
	//	// If the dhcp range resource has the Server field specified,
	//	// then it must match against our name field if we want it to
	//	// group with us.
	//	if res2.Server != "" && res2.Server != obj.Name() {
	//		return fmt.Errorf("resource groups with a different server name")
	//	}

	//	return nil
	//}

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
				h := res.handler4()
				hostHandlers = append(hostHandlers, h)

			//case *DhcpRangeRes:
			//	h := res.handler4()
			//	rangeHandlers = append(rangeHandlers, h)

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

	ipv4Addr net.IP
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

	result, err := url.Parse(obj.NBP)
	if err != nil {
		// this should have been checked in Validate :/
		return errwrap.Wrapf(err, "unexpected invalid nbp URL")
	}
	otsn := dhcpv4.OptTFTPServerName(result.Host)
	obj.opt66 = &otsn
	p := result.Path
	if obj.NBPPath != "" { // override the path if this is specified
		p = obj.NBPPath
	}
	obfn := dhcpv4.OptBootFileName(p)
	obj.opt67 = &obfn

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *DHCPHostRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *DHCPHostRes) Watch() error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-obj.init.Done: // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *DHCPHostRes) CheckApply(apply bool) (bool, error) {
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
func (obj *DHCPHostRes) handler4() func(*dhcpv4.DHCPv4, *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
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

		// XXX: https://tools.ietf.org/html/rfc2132#section-3.3
		// If both the subnet mask and the router option are specified
		// in a DHCP reply, the subnet mask option MUST be first.
		// XXX: Should we do this? Does it matter? Does the lib do it?
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
	}
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
