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
