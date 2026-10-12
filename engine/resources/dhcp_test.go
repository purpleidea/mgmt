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

//go:build !root

package resources

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
)

func TestDHCPServerValidateNBP(t *testing.T) {
	serverID := "192.0.2.1"
	res := &DHCPServerRes{
		Address:   "127.0.0.1:6767",
		Interface: "lo",
		ServerID:  &serverID,
		NBP:       "http://192.0.2.1/pxelinux.0",
	}
	res.SetName("server")

	if err := res.Validate(); err == nil {
		t.Fatalf("expected invalid NBP error")
	}
}

func TestDHCPHostNBPHandlerNoRace(t *testing.T) {
	init := &engine.Init{
		Event: func(ctx context.Context) error { return nil },
		Logf:  func(format string, v ...interface{}) {},
	}

	res := &DHCPHostRes{
		Mac: "de:ad:be:ef:00:01",
		IP:  "192.0.2.42/24",
	}
	res.SetName("host")
	if err := res.Init(init); err != nil {
		t.Fatalf("init failed: %+v", err)
	}

	hw, err := net.ParseMAC(res.Mac)
	if err != nil {
		t.Fatalf("parse mac failed: %+v", err)
	}
	req, err := dhcpv4.NewDiscovery(hw, dhcpv4.WithRequestedOptions(
		dhcpv4.OptionTFTPServerName,
		dhcpv4.OptionBootfileName,
	))
	if err != nil {
		t.Fatalf("new discovery failed: %+v", err)
	}

	handler, err := res.handler4(&HostData{NBP: "tftp://192.0.2.1/boot-a"})
	if err != nil {
		t.Fatalf("handler failed: %+v", err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1000; i++ {
			if _, err := res.handler4(&HostData{NBP: "tftp://192.0.2.2/boot-b"}); err != nil {
				t.Errorf("handler rebuild failed: %+v", err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1000; i++ {
			resp, err := dhcpv4.NewReplyFromRequest(req)
			if err != nil {
				t.Errorf("new reply failed: %+v", err)
				return
			}
			if _, stop := handler(req, resp); !stop {
				t.Errorf("handler did not stop")
				return
			}
		}
	}()

	close(start)
	wg.Wait()
}

func TestDHCPRangeDecline(t *testing.T) {
	init := &engine.Init{
		Event: func(ctx context.Context) error { return nil },
		Logf: func(format string, v ...interface{}) {
			t.Logf("dhcp: "+format, v...)
		},
	}

	serverID := net.ParseIP("192.0.2.1").To4()
	server := &DHCPServerRes{}
	server.SetName("server")
	server.init = init
	server.leaseTime = 10 * time.Minute
	server.sidMutex = &sync.Mutex{}
	server.serverID = serverID

	rng := &DHCPRangeRes{
		Network: "192.0.2.0/24",
		Skip:    []string{"192.0.2.1/24"},
	}
	rng.SetName("range")
	rng.SetParent(server)
	server.SetGroup([]engine.GroupableRes{rng})
	if err := rng.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}

	mac := func(s string) net.HardwareAddr {
		hw, err := net.ParseMAC(s)
		if err != nil {
			t.Fatalf("func ParseMAC: %v", err)
		}
		return hw
	}
	macA := mac("de:ad:be:ef:00:01")
	macB := mac("de:ad:be:ef:00:02")

	discover := func(hw net.HardwareAddr) netip.Addr {
		req, err := dhcpv4.NewDiscovery(hw)
		if err != nil {
			t.Fatalf("func NewDiscovery: %v", err)
		}
		resp, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			t.Fatalf("func NewReplyFromRequest: %v", err)
		}
		handler, err := rng.handler4(nil)
		if err != nil {
			t.Fatalf("func handler4: %v", err)
		}
		resp, _ = handler(req, resp)
		if resp == nil {
			t.Fatalf("no response for %s", hw)
		}
		addr, ok := netip.AddrFromSlice(resp.YourIPAddr.To4())
		if !ok {
			t.Fatalf("bad ip for %s: %s", hw, resp.YourIPAddr)
		}
		return addr
	}

	decline := func(hw net.HardwareAddr, addr netip.Addr, sid net.IP) {
		req, err := dhcpv4.New(
			dhcpv4.WithMessageType(dhcpv4.MessageTypeDecline),
			dhcpv4.WithHwAddr(hw),
			dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.IP(addr.AsSlice()))),
			dhcpv4.WithOption(dhcpv4.OptServerIdentifier(sid)),
		)
		if err != nil {
			t.Fatalf("func New: %v", err)
		}
		server.decline4(req)
	}

	ip1 := discover(macA)

	// A decline for another server's lease changes nothing.
	decline(macA, ip1, net.ParseIP("192.0.2.254"))
	if ip := discover(macA); ip != ip1 {
		t.Fatalf("lease changed after a decline for another server: %s != %s", ip, ip1)
	}

	// After our decline, this mac gets a different ip, and so does a new one.
	decline(macA, ip1, serverID)
	ip2 := discover(macA)
	if ip2 == ip1 {
		t.Fatalf("got declined ip %s again", ip1)
	}
	ipB := discover(macB)
	if ipB == ip1 {
		t.Fatalf("new mac got declined ip %s", ip1)
	}

	// A decline of an ip that isn't the lease of this mac changes nothing.
	decline(macB, ip2, serverID)
	if ip := discover(macB); ip != ipB {
		t.Fatalf("lease changed after a decline of someone else's ip: %s != %s", ip, ipB)
	}
	if ip := discover(macA); ip != ip2 {
		t.Fatalf("lease changed after someone else declined it: %s != %s", ip, ip2)
	}

	// Once the hold is over, the declined ip can be allocated again.
	if _, err := rng.leaseClean(); err != nil {
		t.Fatalf("func leaseClean: %v", err)
	}
	if _, exists := rng.declined[ip1]; !exists {
		t.Fatalf("declined ip %s was freed before its hold was over", ip1)
	}
	rng.declined[ip1] = int(time.Now().Add(-time.Second).Unix())
	if _, err := rng.leaseClean(); err != nil {
		t.Fatalf("func leaseClean: %v", err)
	}
	if _, exists := rng.declined[ip1]; exists {
		t.Fatalf("declined ip %s was not freed after its hold was over", ip1)
	}
	ipNet, err := rng.allocator.Allocate(net.IPNet{IP: net.IP(ip1.AsSlice())})
	if err != nil {
		t.Fatalf("func Allocate: %v", err)
	}
	if !ipNet.IP.Equal(net.IP(ip1.AsSlice())) {
		t.Fatalf("declined ip %s is still allocated, got: %s", ip1, ipNet.IP)
	}
}

func TestDHCPRangeSkipsHostIPs(t *testing.T) {
	init := &engine.Init{
		Event: func(ctx context.Context) error { return nil },
		Logf: func(format string, v ...interface{}) {
			t.Logf("dhcp: "+format, v...)
		},
	}

	server := &DHCPServerRes{}
	server.SetName("server")

	rng := &DHCPRangeRes{
		Network: "192.0.2.0/29", // 192.0.2.1 to 192.0.2.7
		Skip:    []string{"192.0.2.1/29"},
	}
	rng.SetName("range")
	rng.SetParent(server)

	group := []engine.GroupableRes{rng}
	for i, ip := range []string{
		"192.0.2.5/29",    // in the range
		"198.51.100.5/24", // outside of the range
		"192.0.2.1/29",    // also skipped by the range
	} {
		host := &DHCPHostRes{
			Mac: fmt.Sprintf("de:ad:be:ef:01:%02x", i),
			IP:  ip,
		}
		host.SetName(fmt.Sprintf("host%d", i))
		host.SetParent(server)
		group = append(group, host)
	}
	server.SetGroup(group)

	if err := server.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}

	handler, err := rng.handler4(nil)
	if err != nil {
		t.Fatalf("func handler4: %v", err)
	}
	lease := func(i int) net.IP {
		hw, err := net.ParseMAC(fmt.Sprintf("de:ad:be:ef:02:%02x", i))
		if err != nil {
			t.Fatalf("func ParseMAC: %v", err)
		}
		req, err := dhcpv4.NewDiscovery(hw)
		if err != nil {
			t.Fatalf("func NewDiscovery: %v", err)
		}
		resp, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			t.Fatalf("func NewReplyFromRequest: %v", err)
		}
		resp, _ = handler(req, resp)
		if resp == nil {
			return nil
		}
		return resp.YourIPAddr
	}

	// Everything but the skipped and reserved ip's gets handed out.
	got := map[string]struct{}{}
	for i := 0; i < 5; i++ {
		ip := lease(i)
		if ip == nil {
			t.Fatalf("no lease for client %d", i)
		}
		got[ip.String()] = struct{}{}
	}
	for _, ip := range []string{"192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.6", "192.0.2.7"} {
		if _, exists := got[ip]; !exists {
			t.Fatalf("ip %s was not handed out, got: %v", ip, got)
		}
	}
	if ip := lease(5); ip != nil {
		t.Fatalf("range should be full, but got: %s", ip)
	}
}

func TestDHCPProxy(t *testing.T) {
	init := &engine.Init{
		Event: func(ctx context.Context) error { return nil },
		Logf: func(format string, v ...interface{}) {
			t.Logf("dhcp: "+format, v...)
		},
	}

	newServer := func(group ...engine.GroupableRes) (*DHCPServerRes, error) {
		serverID := "192.0.2.1"
		server := &DHCPServerRes{
			ServerID: &serverID,
			NBP:      "tftp://192.0.2.1/uefi/shim.efi",
		}
		server.SetName("server")
		for i, res := range group {
			res.SetName(fmt.Sprintf("res%d", i))
			res.SetParent(server)
		}
		server.SetGroup(group)
		return server, server.Init(init)
	}

	host := &DHCPHostRes{Mac: "de:ad:be:ef:00:01", IP: "198.51.100.42/24", Proxy: true}
	bios := &DHCPHostRes{Mac: "de:ad:be:ef:00:02", Proxy: true} // no ip to check
	full := &DHCPHostRes{Mac: "de:ad:be:ef:00:03", IP: "192.0.2.3/24"}
	rng := &DHCPRangeRes{Network: "198.51.100.0/24", Proxy: true}
	server, err := newServer(host, bios, full, rng)
	if err != nil {
		t.Fatalf("func Init: %v", err)
	}

	unknown := "de:ad:be:ef:00:04"
	discover := func(mac string, class string, arch iana.Arch) *dhcpv4.DHCPv4 {
		hw, err := net.ParseMAC(mac)
		if err != nil {
			t.Fatalf("func ParseMAC: %v", err)
		}
		req, err := dhcpv4.NewDiscovery(hw,
			dhcpv4.WithOption(dhcpv4.OptClassIdentifier(class)),
			dhcpv4.WithOption(dhcpv4.OptClientArch(arch)),
		)
		if err != nil {
			t.Fatalf("func NewDiscovery: %v", err)
		}
		return req
	}
	offer := func(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
		resp, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			t.Fatalf("func NewReplyFromRequest: %v", err)
		}
		return server.proxyOfferHandler4(req, resp)
	}
	uefi := "PXEClient:Arch:00007:UNDI:003016"
	legacy := "PXEClient:Arch:00000:UNDI:002001"

	// A regular host is left for the lease handlers.
	if _, stop := offer(discover(full.Mac, uefi, iana.EFI_X86_64)); stop {
		t.Fatalf("regular host was handled as a proxy")
	}

	// A uefi client is told to ask us for its nbp later, without option 43.
	resp, stop := offer(discover(host.Mac, uefi, iana.EFI_X86_64))
	if !stop || resp == nil {
		t.Fatalf("no proxy offer for a uefi host")
	}
	if !resp.ServerIPAddr.Equal(net.ParseIP("192.0.2.1")) || resp.BootFileName != "" {
		t.Fatalf("bad proxy offer: %s", resp.Summary())
	}
	if resp.ClassIdentifier() != dhcpPXEClient || resp.Options.Has(dhcpv4.OptionVendorSpecificInformation) {
		t.Fatalf("bad proxy offer options: %s", resp.Summary())
	}
	if resp.Options.Has(dhcpv4.OptionIPAddressLeaseTime) || !resp.YourIPAddr.IsUnspecified() {
		t.Fatalf("proxy offer has a lease: %s", resp.Summary())
	}

	// The request for the lease is for the other server.
	req := discover(host.Mac, uefi, iana.EFI_X86_64)
	req.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeRequest))
	if resp, stop := offer(req); !stop || resp != nil {
		t.Fatalf("proxy host got a reply to a request")
	}

	// Anyone else is matched by the proxy range, but only if it's pxe.
	if resp, stop := offer(discover(unknown, "something else", iana.EFI_X86_64)); !stop || resp != nil {
		t.Fatalf("non-pxe client got a proxy offer")
	}
	if resp, _ := offer(discover(unknown, uefi, iana.EFI_X86_64)); resp == nil {
		t.Fatalf("no proxy offer for a uefi client in the range")
	}

	// A bios client gets the nbp right away, so it must have no ip to check.
	if resp, stop := offer(discover(unknown, legacy, iana.INTEL_X86PC)); !stop || resp != nil {
		t.Fatalf("bios client in a proxy range got an offer")
	}
	if resp, stop := offer(discover(host.Mac, legacy, iana.INTEL_X86PC)); !stop || resp != nil {
		t.Fatalf("bios host with an ip got an offer")
	}
	resp, _ = offer(discover(bios.Mac, legacy, iana.INTEL_X86PC))
	if resp == nil || resp.BootFileName != "/uefi/shim.efi" || !resp.Options.Has(dhcpv4.OptionVendorSpecificInformation) {
		t.Fatalf("bad bios proxy offer: %v", resp)
	}

	// On the pxe boot server port, we know the ip of the client.
	for _, x := range []struct {
		mac string
		ip  string
		nbp bool
	}{
		{host.Mac, "198.51.100.42", true},
		{host.Mac, "198.51.100.43", false}, // in the range, but not ours
		{bios.Mac, "203.0.113.7", true},
		{full.Mac, "192.0.2.3", false},
		{unknown, "198.51.100.7", true},
		{unknown, "203.0.113.7", false},
	} {
		nbp, err := server.proxyNBP4(x.mac, netip.MustParseAddr(x.ip))
		if err != nil {
			t.Fatalf("func proxyNBP4: %v", err)
		}
		if (nbp != nil) != x.nbp {
			t.Fatalf("mac %s with ip %s got nbp: %v", x.mac, x.ip, nbp)
		}
	}

	// Some proxy setups don't make sense.
	if _, err := newServer(
		&DHCPRangeRes{Network: "198.51.100.0/24", Proxy: true},
		&DHCPRangeRes{Network: "198.51.100.128/25", Proxy: true},
	); err == nil {
		t.Fatalf("overlapping proxy ranges were allowed")
	}
	if _, err := newServer(
		&DHCPRangeRes{Network: "198.51.100.0/24", Proxy: true},
		&DHCPRangeRes{Network: "203.0.113.0/24"},
	); err == nil {
		t.Fatalf("mixed proxy and regular ranges were allowed")
	}
}

func TestDHCPRangeRequest(t *testing.T) {
	init := &engine.Init{
		Event: func(ctx context.Context) error { return nil },
		Logf: func(format string, v ...interface{}) {
			t.Logf("dhcp: "+format, v...)
		},
	}

	server := &DHCPServerRes{}
	server.SetName("server")
	server.leaseTime = 10 * time.Minute

	rng := &DHCPRangeRes{
		Network: "192.0.2.0/24",
		Skip:    []string{"192.0.2.1/24"},
	}
	rng.SetName("range")
	rng.SetParent(server)
	server.SetGroup([]engine.GroupableRes{rng})
	if err := rng.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	handler, err := rng.handler4(nil)
	if err != nil {
		t.Fatalf("func handler4: %v", err)
	}

	send := func(mt dhcpv4.MessageType, mac, ip string) (*dhcpv4.DHCPv4, bool) {
		hw, err := net.ParseMAC(mac)
		if err != nil {
			t.Fatalf("func ParseMAC: %v", err)
		}
		mods := []dhcpv4.Modifier{
			dhcpv4.WithMessageType(mt),
			dhcpv4.WithHwAddr(hw),
		}
		if ip != "" {
			mods = append(mods, dhcpv4.WithOption(dhcpv4.OptRequestedIPAddress(net.ParseIP(ip))))
		}
		req, err := dhcpv4.New(mods...)
		if err != nil {
			t.Fatalf("func New: %v", err)
		}
		resp, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			t.Fatalf("func NewReplyFromRequest: %v", err)
		}
		return handler(req, resp)
	}
	ack := func(mac, ip string) {
		resp, stop := send(dhcpv4.MessageTypeRequest, mac, ip)
		if !stop || resp == nil || resp.MessageType() == dhcpv4.MessageTypeNak || resp.YourIPAddr.String() != ip {
			t.Fatalf("request of ip %s from %s was not acked: %v", ip, mac, resp)
		}
	}
	nak := func(mac, ip string) {
		resp, stop := send(dhcpv4.MessageTypeRequest, mac, ip)
		if !stop || resp == nil || resp.MessageType() != dhcpv4.MessageTypeNak {
			t.Fatalf("request of ip %s from %s did not get a nak: %v", ip, mac, resp)
		}
	}

	macA := "de:ad:be:ef:00:01"
	resp, _ := send(dhcpv4.MessageTypeDiscover, macA, "")
	if resp == nil {
		t.Fatalf("no offer for %s", macA)
	}
	ip1 := resp.YourIPAddr.String()

	ack(macA, ip1)
	nak(macA, "192.0.2.200") // we leased it a different ip

	// An ip outside of the range is not for us, but maybe for another range.
	if _, stop := send(dhcpv4.MessageTypeRequest, macA, "198.51.100.5"); stop {
		t.Fatalf("request for an ip outside of the range was handled")
	}

	// Without a lease, which happens if we restarted, it can have a free ip.
	ack("de:ad:be:ef:00:02", "192.0.2.100")
	nak("de:ad:be:ef:00:03", ip1)         // leased to someone else
	nak("de:ad:be:ef:00:03", "192.0.2.1") // skipped
	ack("de:ad:be:ef:00:03", "192.0.2.101")
}

func TestDHCPRangeReleaseInform(t *testing.T) {
	init := &engine.Init{
		Event: func(ctx context.Context) error { return nil },
		Logf: func(format string, v ...interface{}) {
			t.Logf("dhcp: "+format, v...)
		},
	}

	serverID := net.ParseIP("192.0.2.1").To4()
	server := &DHCPServerRes{}
	server.SetName("server")
	server.init = init
	server.leaseTime = 10 * time.Minute
	server.sidMutex = &sync.Mutex{}
	server.serverID = serverID

	rng := &DHCPRangeRes{
		Network: "192.0.2.0/24",
		Skip:    []string{"192.0.2.1/24"},
	}
	rng.SetName("range")
	rng.SetParent(server)
	server.SetGroup([]engine.GroupableRes{rng})
	if err := rng.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	handler, err := rng.handler4(nil)
	if err != nil {
		t.Fatalf("func handler4: %v", err)
	}

	newPacket := func(mt dhcpv4.MessageType, mac, ip string, sid net.IP) *dhcpv4.DHCPv4 {
		hw, err := net.ParseMAC(mac)
		if err != nil {
			t.Fatalf("func ParseMAC: %v", err)
		}
		mods := []dhcpv4.Modifier{
			dhcpv4.WithMessageType(mt),
			dhcpv4.WithHwAddr(hw),
			dhcpv4.WithClientIP(net.ParseIP(ip)),
		}
		if sid != nil {
			mods = append(mods, dhcpv4.WithOption(dhcpv4.OptServerIdentifier(sid)))
		}
		req, err := dhcpv4.New(mods...)
		if err != nil {
			t.Fatalf("func New: %v", err)
		}
		return req
	}
	reply := func(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
		resp, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			t.Fatalf("func NewReplyFromRequest: %v", err)
		}
		return handler(req, resp)
	}
	discover := func(mac string) string {
		resp, _ := reply(newPacket(dhcpv4.MessageTypeDiscover, mac, "0.0.0.0", nil))
		if resp == nil {
			t.Fatalf("no offer for %s", mac)
		}
		return resp.YourIPAddr.String()
	}

	macA := "de:ad:be:ef:00:01"
	macB := "de:ad:be:ef:00:02"
	ip1 := discover(macA)

	// A release for another server or of someone else's ip changes nothing.
	server.release4(newPacket(dhcpv4.MessageTypeRelease, macA, ip1, net.ParseIP("192.0.2.254")))
	server.release4(newPacket(dhcpv4.MessageTypeRelease, macB, ip1, serverID))
	if ip := discover(macB); ip == ip1 {
		t.Fatalf("got ip %s which wasn't released", ip)
	}

	// Once it's released, it's free right away.
	server.release4(newPacket(dhcpv4.MessageTypeRelease, macA, ip1, serverID))
	if _, exists := rng.records[macA]; exists {
		t.Fatalf("lease of %s was not released", macA)
	}
	if ip := discover("de:ad:be:ef:00:03"); ip != ip1 {
		t.Fatalf("released ip %s was not free, got: %s", ip1, ip)
	}

	// An inform gets the options, but no lease.
	inform := newPacket(dhcpv4.MessageTypeInform, "de:ad:be:ef:00:04", "192.0.2.200", nil)
	resp, stop := reply(inform)
	if !stop || resp == nil || !resp.YourIPAddr.IsUnspecified() || resp.SubnetMask() == nil {
		t.Fatalf("bad reply to an inform: %v", resp)
	}
	if _, exists := rng.records["de:ad:be:ef:00:04"]; exists {
		t.Fatalf("inform got a lease")
	}
	if resp, _ := server.leasetimeHandler4(inform, resp); resp.Options.Has(dhcpv4.OptionIPAddressLeaseTime) {
		t.Fatalf("reply to an inform has a lease time")
	}
	if _, stop := reply(newPacket(dhcpv4.MessageTypeInform, "de:ad:be:ef:00:04", "198.51.100.5", nil)); stop {
		t.Fatalf("inform from outside of the range was handled")
	}
}
