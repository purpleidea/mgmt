// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
)

const (
	iface   = "lo" // loopback for local testing
	address = "127.0.0.1"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		log.Printf("Usage: %s [port] <mac address>", os.Args[0])
		return
	}

	port := string(nclient4.ServerPort) // the default is 67
	if len(os.Args) >= 3 {
		port = os.Args[1]
	}
	hwAddr := os.Args[len(os.Args)-1] // argv[1]

	hw, err := net.ParseMAC(hwAddr)
	if err != nil {
		log.Printf("Invalid mac address: %v", err)
		return
	}

	addr := fmt.Sprintf("%s:%s", address, port)
	log.Printf("Connecting to: %s", addr)

	opts := []nclient4.ClientOpt{}
	{
		opt := nclient4.WithHWAddr(hw)
		opts = append(opts, opt)
	}
	{
		opt := nclient4.WithSummaryLogger()
		opts = append(opts, opt)
	}
	//{
	//	opt := nclient4.WithDebugLogger()
	//	opts = append(opts, opt)
	//}

	//c, err := nclient4.NewWithConn(conn net.PacketConn, ifaceHWAddr net.HardwareAddr, opts...)
	c, err := nclient4.New(iface, opts...)
	if err != nil {
		log.Printf("Error connecting to server: %v", err)
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("Error closing client: %v", err)
		}
	}()

	modifiers := []dhcpv4.Modifier{}
	//{
	//	mod := dhcpv4.WithYourIP(net.ParseIP(?))
	//	modifiers = append(modifiers, mod)
	//}
	//{
	//	mod := dhcpv4.WithClientIP(net.ParseIP(?))
	//	modifiers = append(modifiers, mod)
	//}
	// TODO: add modifiers

	log.Printf("Requesting...")
	ctx := context.Background()                     // TODO: add to ^C handler
	offer, ack, err := c.Request(ctx, modifiers...) // (offer, ack *dhcpv4.DHCPv4, err error)
	if err != nil {
		log.Printf("Error requesting from server: %v", err)
		return
	}

	// Show the results of the D-O-R-A exchange.
	log.Printf("Offer: %+v", offer)
	log.Printf("Ack: %+v", ack)

	log.Printf("Done!")
}
