// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"bytes"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// test cases for NetRes.unitFileContents()
var unitFileContentsTests = []struct {
	dev string
	in  *NetRes
	out []byte
}{
	{
		"eth0",
		&NetRes{
			State:   "up",
			Addrs:   []string{"192.168.42.13/24"},
			Gateway: "192.168.42.1",
		},
		[]byte(
			strings.Join(
				[]string{
					"[Match]",
					"Name=eth0",
					"[Network]",
					"Address=192.168.42.13/24",
					"Gateway=192.168.42.1",
				},
				"\n"),
		),
	},
	{
		"wlp5s0",
		&NetRes{
			State:   "up",
			Addrs:   []string{"10.0.2.13/24", "10.0.2.42/24"},
			Gateway: "10.0.2.1",
		},
		[]byte(
			strings.Join(
				[]string{
					"[Match]",
					"Name=wlp5s0",
					"[Network]",
					"Address=10.0.2.13/24",
					"Address=10.0.2.42/24",
					"Gateway=10.0.2.1",
				},
				"\n"),
		),
	},
}

// test NetRes.unitFileContents()
func TestUnitFileContents(t *testing.T) {
	for _, test := range unitFileContentsTests {
		test.in.SetName(test.dev)
		result := test.in.unitFileContents()
		if !bytes.Equal(test.out, result) {
			t.Errorf("nfd test wanted:\n %s, got:\n %s", test.out, result)
		}
	}
}

// test cases for socketSet.fdSet()
var fdSetTests = []struct {
	in  *socketSet
	out *unix.FdSet
}{
	{
		&socketSet{
			fdEvents: 3,
			fdPipe:   4,
		},
		&unix.FdSet{
			Bits: [16]int64{0x18}, // 11000
		},
	},
	{
		&socketSet{
			fdEvents: 12,
			fdPipe:   8,
		},
		&unix.FdSet{
			Bits: [16]int64{0x1100}, // 1000100000000
		},
	},
	{
		&socketSet{
			fdEvents: 9,
			fdPipe:   21,
		},
		&unix.FdSet{
			Bits: [16]int64{0x200200}, // 1000000000001000000000
		},
	},
}

// test socketSet.fdSet()
func TestFdSet(t *testing.T) {
	for _, test := range fdSetTests {
		result := test.in.fdSet()
		if *result != *test.out {
			t.Errorf("fdSet test wanted: %b, got: %b", *test.out, *result)
		}
	}
}

// test cases for socketSet.nfd()
var nfdTests = []struct {
	in  *socketSet
	out int
}{
	{
		&socketSet{
			fdEvents: 3,
			fdPipe:   4,
		},
		5,
	},
	{
		&socketSet{
			fdEvents: 8,
			fdPipe:   4,
		},
		9,
	},
	{
		&socketSet{
			fdEvents: 90,
			fdPipe:   900,
		},
		901,
	},
}

// test socketSet.nfd()
func TestNfd(t *testing.T) {
	for _, test := range nfdTests {
		result := test.in.nfd()
		if result != test.out {
			t.Errorf("nfd test wanted: %d, got: %d", test.out, result)
		}
	}
}
