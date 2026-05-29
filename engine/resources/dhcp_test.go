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
	"net"
	"sync"
	"testing"

	"github.com/purpleidea/mgmt/engine"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

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
