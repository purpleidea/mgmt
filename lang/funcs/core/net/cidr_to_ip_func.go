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

package corenet

import (
	"net"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "cidr_to_ip", &types.FuncValue{
		T: types.NewType("func(a str) str"),
		V: CidrToIP,
	})
}

// CidrToIP returns the IP from a CIDR address
func CidrToIP(input []types.Value) (types.Value, error) {
	cidr := input[0].Str()
	ip, _, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return &types.StrValue{}, err
	}
	return &types.StrValue{
		V: ip.String(),
	}, nil
}
