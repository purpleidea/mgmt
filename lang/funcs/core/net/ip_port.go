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
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "ip_port", &types.FuncValue{
		T: types.NewType("func(ipport str) str"),
		V: IPPort,
	})
}

// IPPort returns the combind IPv4/IPv6:(input[0]) and Port:(input[1]). Returns
// error if IPv4/IPv6 string is incorrect format or Port not in range 0-65536.
func IPPort(input []types.Value) (types.Value, error) {
	ip := net.ParseIP(input[0].Str()) // Is nil if incorrect format.
	ipStr := input[0].Str()
	port := input[1].Str()
	portInt, err := strconv.Atoi(port)

	if ip == nil {
		return &types.StrValue{V: ""}, errors.New("incorrect ip format")
	}
	if err != nil {
		return &types.StrValue{V: ""}, fmt.Errorf("err converting str to int %v", err)
	}
	if portInt < 0 || portInt > 65536 {
		return &types.StrValue{V: ""}, errors.New("port not in range 0-65536")
	}

	return &types.StrValue{
		V: ipStr + ":" + port,
	}, nil
}
