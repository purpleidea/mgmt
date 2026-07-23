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
	"context"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine"

	libvirt "libvirt.org/go/libvirt"
	libvirtxml "libvirt.org/go/libvirtxml"
)

// TestVirtNetworkTransientDownValidate exercises the "tear down whatever's
// there" config: Transient=true, State=down. Base fields are intentionally not
// required, since they only matter for create/update and there is nothing to
// create here.
func TestVirtNetworkTransientDownValidate(t *testing.T) {
	obj := &VirtNetworkRes{
		State:     VirtNetworkStateDown,
		Transient: true,
	}
	obj.SetName("old-network")

	if err := obj.Validate(); err != nil {
		t.Fatalf("func Validate: %v", err)
	}
	if err := obj.Init(&engine.Init{}); err != nil {
		t.Fatalf("func Init: %v", err)
	}
}

// TestVirtNetworkTransientStateUnset exercises the use case of a pre-existing
// libvirt network that we want to undefine (Transient=true) while leaving its
// running state alone (State=""), and reconcile the grouped DHCP hosts on it.
// Validate must require the parent IP/MAC for the children to be checked
// against, Init must populate the subnet metadata, and host reconciliation must
// run through to hostsCheckApply rather than being short-circuited.
func TestVirtNetworkTransientStateUnset(t *testing.T) {
	t.Run("func Validate rejects missing base fields", func(t *testing.T) {
		obj := &VirtNetworkRes{Transient: true} // no IP/MAC
		obj.SetName("net")
		if err := obj.Validate(); err == nil {
			t.Fatal("func Validate should require base fields when state is unset")
		}
	})

	t.Run("init populates subnet and hosts reconcile", func(t *testing.T) {
		obj := newTestVirtNetwork(t)
		obj.Transient = true
		obj.State = ""
		host := newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.101/24")
		obj.SetGroup([]engine.GroupableRes{host})
		initTestVirtNetwork(t, obj)

		if obj.ipv4Addr == nil || obj.ipv4Net == nil {
			t.Fatal("func Init should have populated subnet metadata")
		}

		// Drive hostsCheckApply against a current XML that lacks our
		// host. We expect checkOK=false (drift detected) - the guard
		// previously short-circuited this whole branch.
		checkOK, err := obj.hostsCheckApply(context.Background(), false, networkWithDHCP(nil, nil), libvirt.NETWORK_UPDATE_AFFECT_LIVE)
		if err != nil {
			t.Fatalf("hostsCheckApply: %v", err)
		}
		if checkOK {
			t.Fatal("expected hostsCheckApply to detect missing host")
		}
	})
}

// TestVirtNetworkRemoveNoNetwork covers the convergence path when there is no
// existing network. removeCheckApply must report checkOK=true so the rest of
// the pipeline (addCheckApply) can drive a create/skip decision.
func TestVirtNetworkRemoveNoNetwork(t *testing.T) {
	cases := []struct {
		name      string
		state     string
		transient bool
	}{
		{"persistent up", VirtNetworkStateUp, false},
		{"persistent down", VirtNetworkStateDown, false},
		{"persistent undefined", "", false},
		{"transient up", VirtNetworkStateUp, true},
		{"transient down", VirtNetworkStateDown, true},
		{"transient undefined", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &VirtNetworkRes{State: tc.state, Transient: tc.transient}
			obj.SetName("absent")
			// obj.network is the nil zero value; we never reach init.

			checkOK, err := obj.removeCheckApply(context.Background(), true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !checkOK {
				t.Fatalf("expected checkOK=true when no network exists")
			}
		})
	}
}

func TestVirtNetworkBuildNetworkXML(t *testing.T) {
	obj := newTestVirtNetwork(t)
	host := newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.101/24")
	rng := newTestVirtNetworkRange(t, "main", "192.168.124.150", "192.168.124.200")
	obj.SetGroup([]engine.GroupableRes{host, rng})
	initTestVirtNetwork(t, obj)

	doc, err := obj.buildNetworkXML()
	if err != nil {
		t.Fatalf("could not build network XML: %v", err)
	}

	got := &libvirtxml.Network{}
	if err := got.Unmarshal(doc); err != nil {
		t.Fatalf("could not parse rendered XML: %v", err)
	}
	if got.Name != "mgmt-test" {
		t.Fatalf("unexpected network name: %s", got.Name)
	}
	if got.Forward == nil || got.Forward.Mode != "nat" {
		t.Fatalf("unexpected forward mode: %#v", got.Forward)
	}
	if got.Bridge == nil || got.Bridge.Name != "mgmtbr0" {
		t.Fatalf("unexpected bridge: %#v", got.Bridge)
	}
	if got.MAC == nil || got.MAC.Address != "52:54:00:bd:00:01" {
		t.Fatalf("unexpected bridge mac: %#v", got.MAC)
	}
	if len(got.IPs) != 1 {
		t.Fatalf("expected one IP block, got %d", len(got.IPs))
	}
	ip := got.IPs[0]
	if ip.Address != "192.168.124.1" || ip.Netmask != "255.255.255.0" {
		t.Fatalf("unexpected IP block: %#v", ip)
	}
	if ip.DHCP == nil {
		t.Fatal("expected DHCP block")
	}
	if len(ip.DHCP.Hosts) != 1 || ip.DHCP.Hosts[0].MAC != host.Mac || ip.DHCP.Hosts[0].IP != "192.168.124.101" {
		t.Fatalf("unexpected DHCP hosts: %#v", ip.DHCP.Hosts)
	}
	if len(ip.DHCP.Ranges) != 1 || ip.DHCP.Ranges[0].Start != rng.Start || ip.DHCP.Ranges[0].End != rng.End {
		t.Fatalf("unexpected DHCP ranges: %#v", ip.DHCP.Ranges)
	}
}

func TestVirtNetworkInitRejectsInvalidAllocations(t *testing.T) {
	cases := []struct {
		name  string
		group []engine.GroupableRes
		err   string
	}{
		{
			name: "duplicate host mac",
			group: []engine.GroupableRes{
				newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.101/24"),
				newTestVirtNetworkHost(t, "vm-2", "52:54:00:00:00:11", "192.168.124.102/24"),
			},
			err: "already reserved mac",
		},
		{
			name: "duplicate host ip",
			group: []engine.GroupableRes{
				newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.101/24"),
				newTestVirtNetworkHost(t, "vm-2", "52:54:00:00:00:12", "192.168.124.101/24"),
			},
			err: "already reserved ip",
		},
		{
			name: "host uses router ip",
			group: []engine.GroupableRes{
				newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.1/24"),
			},
			err: "already reserved ip",
		},
		{
			name: "range contains static host",
			group: []engine.GroupableRes{
				newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.101/24"),
				newTestVirtNetworkRange(t, "main", "192.168.124.100", "192.168.124.120"),
			},
			err: "contains reserved ip",
		},
		{
			name: "overlapping ranges",
			group: []engine.GroupableRes{
				newTestVirtNetworkRange(t, "main", "192.168.124.100", "192.168.124.120"),
				newTestVirtNetworkRange(t, "extra", "192.168.124.110", "192.168.124.130"),
			},
			err: "overlaps",
		},
		{
			name: "broadcast range edge",
			group: []engine.GroupableRes{
				newTestVirtNetworkRange(t, "main", "192.168.124.250", "192.168.124.255"),
			},
			err: "broadcast address",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := newTestVirtNetwork(t)
			obj.SetGroup(tc.group)
			err := obj.Init(&engine.Init{})
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("expected init error containing %q, got: %v", tc.err, err)
			}
		})
	}
}

func TestVirtNetworkManagedIPv4Index(t *testing.T) {
	obj := newTestVirtNetwork(t)
	initTestVirtNetwork(t, obj)

	cases := []struct {
		name    string
		ips     []libvirtxml.NetworkIP
		index   int
		wantErr bool
	}{
		{
			name: "desired address wins",
			ips: []libvirtxml.NetworkIP{
				{Address: "2001:db8::1", Family: "ipv6"},
				{Address: "192.168.124.1", Netmask: "255.255.255.0"},
			},
			index: 1,
		},
		{
			name: "single alternate IPv4 is manageable",
			ips: []libvirtxml.NetworkIP{
				{Address: "192.168.125.1", Netmask: "255.255.255.0"},
			},
			index: 0,
		},
		{
			name: "missing IPv4 can be added",
			ips: []libvirtxml.NetworkIP{
				{Address: "2001:db8::1", Family: "ipv6"},
			},
			index: -1,
		},
		{
			name: "ambiguous IPv4 is rejected",
			ips: []libvirtxml.NetworkIP{
				{Address: "192.168.125.1", Netmask: "255.255.255.0"},
				{Address: "192.168.126.1", Netmask: "255.255.255.0"},
			},
			index:   -1,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := obj.findIPv4Index(&libvirtxml.Network{IPs: tc.ips})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if idx != tc.index {
				t.Fatalf("expected index %d, got %d", tc.index, idx)
			}
		})
	}
}

func TestVirtNetworkDHCPHostCheckMode(t *testing.T) {
	host := newTestVirtNetworkHost(t, "vm-1", "52:54:00:00:00:11", "192.168.124.101/24")

	cases := []struct {
		name       string
		purgeHosts bool
		current    []libvirtxml.NetworkDHCPHost
		checkOK    bool
	}{
		{
			name: "matching managed host",
			current: []libvirtxml.NetworkDHCPHost{
				{MAC: host.Mac, Name: host.getHostname(), IP: "192.168.124.101"},
			},
			checkOK: true,
		},
		{
			name: "managed host missing",
			current: []libvirtxml.NetworkDHCPHost{
				{MAC: "52:54:00:00:00:22", Name: "external", IP: "192.168.124.202"},
			},
			checkOK: false,
		},
		{
			name: "unmanaged host preserved",
			current: []libvirtxml.NetworkDHCPHost{
				{MAC: host.Mac, Name: host.getHostname(), IP: "192.168.124.101"},
				{MAC: "52:54:00:00:00:22", Name: "external", IP: "192.168.124.202"},
			},
			checkOK: true,
		},
		{
			name:       "unmanaged host purged",
			purgeHosts: true,
			current: []libvirtxml.NetworkDHCPHost{
				{MAC: host.Mac, Name: host.getHostname(), IP: "192.168.124.101"},
				{MAC: "52:54:00:00:00:22", Name: "external", IP: "192.168.124.202"},
			},
			checkOK: false,
		},
		{
			name: "same mac modified",
			current: []libvirtxml.NetworkDHCPHost{
				{MAC: host.Mac, Name: host.getHostname(), IP: "192.168.124.111"},
			},
			checkOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := newTestVirtNetwork(t)
			obj.PurgeHosts = tc.purgeHosts
			obj.SetGroup([]engine.GroupableRes{host})
			initTestVirtNetwork(t, obj)

			checkOK, err := obj.hostsCheckApply(context.Background(), false, networkWithDHCP(tc.current, nil), libvirt.NETWORK_UPDATE_AFFECT_CONFIG)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if checkOK != tc.checkOK {
				t.Fatalf("expected checkOK=%t, got %t", tc.checkOK, checkOK)
			}
		})
	}
}

func TestVirtNetworkDHCPRangeCheckMode(t *testing.T) {
	rng := newTestVirtNetworkRange(t, "main", "192.168.124.150", "192.168.124.200")

	cases := []struct {
		name        string
		purgeRanges bool
		current     []libvirtxml.NetworkDHCPRange
		checkOK     bool
	}{
		{
			name: "matching managed range",
			current: []libvirtxml.NetworkDHCPRange{
				{Start: rng.Start, End: rng.End},
			},
			checkOK: true,
		},
		{
			name: "managed range missing",
			current: []libvirtxml.NetworkDHCPRange{
				{Start: "192.168.124.10", End: "192.168.124.20"},
			},
			checkOK: false,
		},
		{
			name: "unmanaged range preserved",
			current: []libvirtxml.NetworkDHCPRange{
				{Start: rng.Start, End: rng.End},
				{Start: "192.168.124.10", End: "192.168.124.20"},
			},
			checkOK: true,
		},
		{
			name:        "unmanaged range purged",
			purgeRanges: true,
			current: []libvirtxml.NetworkDHCPRange{
				{Start: rng.Start, End: rng.End},
				{Start: "192.168.124.10", End: "192.168.124.20"},
			},
			checkOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := newTestVirtNetwork(t)
			obj.PurgeRanges = tc.purgeRanges
			obj.SetGroup([]engine.GroupableRes{rng})
			initTestVirtNetwork(t, obj)

			checkOK, err := obj.rangeCheckApply(context.Background(), false, networkWithDHCP(nil, tc.current), libvirt.NETWORK_UPDATE_AFFECT_CONFIG)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if checkOK != tc.checkOK {
				t.Fatalf("expected checkOK=%t, got %t", tc.checkOK, checkOK)
			}
		})
	}
}

func TestVirtNetworkXMLDirUsesCanonicalURI(t *testing.T) {
	obj := &VirtNetworkRes{}

	if got, err := obj.networkXMLDir(""); err == nil || got != "" {
		t.Fatalf("empty URI should error without a watch dir, got: %s, err: %v", got, err)
	}
	if got, err := obj.networkXMLDir("qemu:///system"); err != nil || got != "/etc/libvirt/qemu/networks/" {
		t.Fatalf("unexpected system network dir: %s, err: %v", got, err)
	}
	if got, err := obj.networkXMLDir("lxc:///system"); err == nil || got != "" {
		t.Fatalf("non-qemu URI should error without a watch dir, got: %s, err: %v", got, err)
	}
	if got, err := obj.networkXMLDir("qemu:///wat"); err == nil || got != "" {
		t.Fatalf("unknown qemu path should error, got: %s, err: %v", got, err)
	}

	t.Setenv("HOME", "/home/test")
	want := "/home/test/.config/libvirt/qemu/networks/"
	if got, err := obj.networkXMLDir("qemu:///session"); err != nil || got != want {
		t.Fatalf("unexpected session network dir: %s, err: %v (want %s)", got, err, want)
	}
}

func TestVirtNetworkBaseMatch(t *testing.T) {
	obj := newTestVirtNetwork(t)
	initTestVirtNetwork(t, obj)

	full := func() *libvirtxml.Network {
		return &libvirtxml.Network{
			Forward: &libvirtxml.NetworkForward{Mode: "nat"},
			Bridge:  &libvirtxml.NetworkBridge{Name: "mgmtbr0"},
			MAC:     &libvirtxml.NetworkMAC{Address: "52:54:00:bd:00:01"},
			IPs: []libvirtxml.NetworkIP{{
				Address: "192.168.124.1",
				Netmask: "255.255.255.0",
			}},
		}
	}

	cases := []struct {
		name  string
		mut   func(*libvirtxml.Network)
		match bool
	}{
		{"all fields match", func(*libvirtxml.Network) {}, true},
		{"forward mode differs", func(n *libvirtxml.Network) { n.Forward.Mode = "route" }, false},
		{"bridge name differs", func(n *libvirtxml.Network) { n.Bridge.Name = "other" }, false},
		{"mac differs", func(n *libvirtxml.Network) { n.MAC.Address = "52:54:00:bd:00:02" }, false},
		{"address differs", func(n *libvirtxml.Network) { n.IPs[0].Address = "192.168.99.1" }, false},
		{"netmask differs", func(n *libvirtxml.Network) { n.IPs[0].Netmask = "255.255.0.0" }, false},
		{"forward missing", func(n *libvirtxml.Network) { n.Forward = nil }, false},
		{"bridge missing", func(n *libvirtxml.Network) { n.Bridge = nil }, false},
		{"mac missing", func(n *libvirtxml.Network) { n.MAC = nil }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			net := full()
			tc.mut(net)
			idx, err := obj.findIPv4Index(net)
			if err != nil {
				t.Fatalf("findIPv4Index: %v", err)
			}
			got := obj.baseMatch(net, idx)
			if got != tc.match {
				t.Fatalf("baseMatch = %t, want %t", got, tc.match)
			}
		})
	}
}

func TestVirtNetworkBuildNetworkXMLNoDHCP(t *testing.T) {
	obj := newTestVirtNetwork(t)
	initTestVirtNetwork(t, obj)

	doc, err := obj.buildNetworkXML()
	if err != nil {
		t.Fatalf("buildNetworkXML: %v", err)
	}

	got := &libvirtxml.Network{}
	if err := got.Unmarshal(doc); err != nil {
		t.Fatalf("could not parse rendered XML: %v", err)
	}
	if len(got.IPs) != 1 {
		t.Fatalf("expected one IP block, got %d", len(got.IPs))
	}
	if got.IPs[0].DHCP != nil {
		t.Fatalf("expected no DHCP block, got: %#v", got.IPs[0].DHCP)
	}
	// And belt-and-braces, no <dhcp in the raw text.
	if strings.Contains(doc, "<dhcp") {
		t.Fatalf("rendered XML should not contain a <dhcp> element:\n%s", doc)
	}
}

func newTestVirtNetwork(t *testing.T) *VirtNetworkRes {
	t.Helper()

	obj := &VirtNetworkRes{
		Device: "mgmtbr0",
		Mode:   "nat",
		Mac:    "52:54:00:bd:00:01",
		IP:     "192.168.124.1/24",
	}
	obj.SetName("mgmt-test")
	if err := obj.Validate(); err != nil {
		t.Fatalf("invalid network fixture: %v", err)
	}
	return obj
}

func newTestVirtNetworkHost(t *testing.T, name, mac, ip string) *VirtNetworkHostRes {
	t.Helper()

	obj := &VirtNetworkHostRes{
		Network: "mgmt-test",
		Mac:     mac,
		IP:      ip,
	}
	obj.SetName(name)
	if err := obj.Validate(); err != nil {
		t.Fatalf("invalid host fixture %s: %v", name, err)
	}
	return obj
}

func newTestVirtNetworkRange(t *testing.T, name, start, end string) *VirtNetworkRangeRes {
	t.Helper()

	obj := &VirtNetworkRangeRes{
		Network: "mgmt-test",
		Start:   start,
		End:     end,
	}
	obj.SetName(name)
	if err := obj.Validate(); err != nil {
		t.Fatalf("invalid range fixture %s: %v", name, err)
	}
	return obj
}

func initTestVirtNetwork(t *testing.T, obj *VirtNetworkRes) {
	t.Helper()

	if err := obj.Init(&engine.Init{}); err != nil {
		t.Fatalf("could not init network fixture: %v", err)
	}
}

func networkWithDHCP(hosts []libvirtxml.NetworkDHCPHost, ranges []libvirtxml.NetworkDHCPRange) *libvirtxml.Network {
	return &libvirtxml.Network{
		IPs: []libvirtxml.NetworkIP{{
			Address: "192.168.124.1",
			Netmask: "255.255.255.0",
			DHCP: &libvirtxml.NetworkDHCP{
				Hosts:  hosts,
				Ranges: ranges,
			},
		}},
	}
}
