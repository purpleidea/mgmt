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
	"strconv"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "ip_port", &types.FuncValue{
		T: types.NewType("func(ip, port str) str"),
		V: IPPort,
	})
}

// IPPort returns the combind IP:(input[0]) and Port:(input[1]) arguments as
// long as port input is within range 1-65536
func IPPort(input []types.Value) (types.Value, error) {
	ip := input[0].Str()
	port := input[1].Str()
	portInt, err := strconv.Atoi(port)

	if err != nil {
		return &types.StrValue{V: ""}, fmt.Errorf("err converting str to int %v", err)
	}
	if portInt < 1 || portInt > 65536 {
		return &types.StrValue{V: ""}, errors.New("port number must be between 1-65536")
	}
	return &types.StrValue{
		V: ip + ":" + port,
	}, nil
}
