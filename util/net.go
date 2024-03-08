// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package util

import (
	"strings"

	"github.com/vishvananda/netlink"
)

// GetPhysicalEthernetDevices returns a link of physical ethernet devices. This
// is a heuristic and I wish I knew the better way.
//
// XXX: Patches welcome!
func GetPhysicalEthernetDevices() ([]string, error) {
	devices := []string{}
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		// wifi, eth, and loopback all do this, bridges do not.
		if link.Type() != "device" {
			continue
		}
		name := link.Attrs().Name
		if name == "lo" { // loopback
			continue
		}
		if strings.HasPrefix(name, "w") { // heuristic, eg: wlp0s20f3
			continue
		}
		devices = append(devices, name)
	}

	return devices, nil
}
