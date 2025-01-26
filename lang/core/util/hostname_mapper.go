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

package coreutil

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "hostname_mapper", &simple.Scaffold{
		T: types.NewType("func(map{str:str}) str"),
		F: HostnameMapper,
	})
}

// HostnameMapper takes a map from mac address to hostname, and finds a hostname
// that corresponds to one of the mac addresses on this machine. If it cannot
// find a match, it returns the empty string. If it's ambiguous, it errors. This
// is useful for bootstrapping the hostname setup on hosts.
//
// XXX: This should produce new values if the list of interfaces change.
func HostnameMapper(ctx context.Context, input []types.Value) (types.Value, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	v := input[0].Value() // interface{}
	m, ok := v.(map[string]string)
	if !ok {
		// programming error
		return nil, err
	}

	hostnamesMap := make(map[string]struct{})

	for _, iface := range ifs {
		// Check if the interface has the loopback flag set
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		mac := iface.HardwareAddr.String()

		// Check if the MAC address is valid.
		if len(mac) != len("00:00:00:00:00:00") {
			continue // maybe something weird we want to ignore...
		}

		hostname, exists := m[mac]
		if !exists {
			continue
		}
		if hostname == "" {
			// TODO: should we error here?
			continue
		}
		hostnamesMap[hostname] = struct{}{} // found
	}
	hostnames := []string{}
	for k := range hostnamesMap {
		hostnames = append(hostnames, k)
	}

	if len(hostnames) > 1 {
		return nil, fmt.Errorf("multiple hostnames found: %s", strings.Join(hostnames, ", "))
	}

	result := ""
	if len(hostnames) == 1 {
		result = hostnames[0]
	}

	return &types.StrValue{
		V: result,
	}, nil
}
