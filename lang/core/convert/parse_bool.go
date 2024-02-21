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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package convert

import (
	"fmt"
	"strconv"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "parse_bool", &types.FuncValue{
		T: types.NewType("func(a str) bool"),
		V: ParseBool,
	})
}

// ParseBool parses a bool string and returns a boolean. It errors if you pass
// it an invalid value. Valid values match what is accepted by the golang
// strconv.ParseBool function. It's recommended to use the strings `true` or
// `false` if you are undecided about what string representation to choose.
func ParseBool(input []types.Value) (types.Value, error) {
	s := input[0].Str()
	b, err := strconv.ParseBool(s)
	if err != nil {
		return nil, fmt.Errorf("invalid bool: `%s`", s)
	}
	return &types.BoolValue{
		V: b,
	}, nil
}
