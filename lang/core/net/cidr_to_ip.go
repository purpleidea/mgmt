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

package corenet

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "cidr_to_ip", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a str) str"),
		F: CidrToIP,
	})
	simple.ModuleRegister(ModuleName, "cidr_to_prefix", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a str) str"),
		F: CidrToPrefix,
	})
	simple.ModuleRegister(ModuleName, "cidr_to_mask", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a str) str"),
		F: CidrToMask,
	})
	simple.ModuleRegister(ModuleName, "cidr_to_first", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a str) str"),
		F: CidrToFirst,
	})
	simple.ModuleRegister(ModuleName, "cidr_to_last", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a str) str"),
		F: CidrToLast,
	})
}

// CidrToIP returns the IP from a CIDR address.
func CidrToIP(ctx context.Context, input []types.Value) (types.Value, error) {
	cidr := input[0].Str()
	ip, _, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: ip.String(),
	}, nil
}

// CidrToPrefix returns the prefix from a CIDR address. For example, if you give
// us 192.0.2.0/24 then we will return "24" as a string.
func CidrToPrefix(ctx context.Context, input []types.Value) (types.Value, error) {
	cidr := input[0].Str()
	_, ipnet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil, err
	}

	ones, _ := ipnet.Mask.Size()

	return &types.StrValue{
		V: strconv.Itoa(ones),
	}, nil
}

// CidrToMask returns the subnet mask from a CIDR address.
func CidrToMask(ctx context.Context, input []types.Value) (types.Value, error) {
	cidr := input[0].Str()
	_, ipnet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: net.IP(ipnet.Mask).String(),
	}, nil
}

// CidrToFirst returns the first usable IP from a CIDR address.
func CidrToFirst(ctx context.Context, input []types.Value) (types.Value, error) {
	cidr := input[0].Str()
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}

	// prefix.Addr() gives the network address, the "first usable" is
	// typically the next address after the network address.
	networkAddr := prefix.Addr()
	firstUsable := networkAddr.Next()

	// Check if it's still within the prefix range.
	if !prefix.Contains(firstUsable) {
		// e.g. for a /32, there's no "next" usable address
		return nil, fmt.Errorf("no usable next address")
	}

	return &types.StrValue{
		V: firstUsable.String(),
	}, nil
}

// CidrToLast returns the last IP from a CIDR address. It's often used as the
// "broadcast" ip.
func CidrToLast(ctx context.Context, input []types.Value) (types.Value, error) {
	cidr := input[0].Str()
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}

	// get the network address (masked)
	networkAddr := prefix.Masked()
	s := ""

	// check if the address is IPv4 or IPv6
	if networkAddr.Addr().Is4() {
		s = lastAddrIPv4(networkAddr.Addr(), prefix.Bits()).String()
	} else if networkAddr.Addr().Is6() {
		s = lastAddrIPv6(networkAddr.Addr(), prefix.Bits()).String()
	}

	if s == "" {
		return nil, fmt.Errorf("no usable last address")
	}
	return &types.StrValue{
		V: s,
	}, nil
}

// lastAddrIPv4 calculates the last IPv4 address given a masked network address
// and a prefix size.
func lastAddrIPv4(networkAddr netip.Addr, prefixBits int) netip.Addr {
	ipv4 := networkAddr.As4()
	ipAsUint32 := binary.BigEndian.Uint32(ipv4[:])

	hostBits := 32 - prefixBits
	// set all these host bits to 1
	ipAsUint32 |= (1 << hostBits) - 1

	// convert back to netip.Addr
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], ipAsUint32)
	return netip.AddrFrom4(out)
}

// lastAddrIPv6 calculates the last IPv6 address given a masked network address
// and a prefix size.
func lastAddrIPv6(networkAddr netip.Addr, prefixBits int) netip.Addr {
	ipv6 := networkAddr.As16()
	hostBits := 128 - prefixBits

	// flip the lowest hostBits to 1
	// bit 0 is the highest bit, bit 127 is the lowest in the 128-bit addr
	for i := 0; i < hostBits; i++ {
		bitPos := 127 - i       // which bit from the left (0-based)
		bytePos := bitPos / 8   // which byte in the array
		bitInByte := bitPos % 8 // which bit within that byte

		// set that bit to 1
		ipv6[bytePos] |= 1 << (7 - bitInByte)
	}

	return netip.AddrFrom16(ipv6)
}
