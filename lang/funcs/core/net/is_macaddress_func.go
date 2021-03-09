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
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "is_macaddress", &types.FuncValue{
		T: types.NewType("func(a str) bool"),
		V: IsMacAddress,
	})
}

// IsMacAddress validates mac address in string format that contains one of the
// accepted delimiter [":","-","."]
func IsMacAddress(input []types.Value) (types.Value, error) {
	mac := input[0].Str()
	if len(mac) != len("00:00:00:00:00:00") {
		return &types.BoolValue{V: false}, nil
	}
	delimiter := strings.Split(mac, "")[2]
	return &types.BoolValue{V: validate(mac, delimiter)}, nil
}

func validate(mac, delimiter string) bool {
	// valid delimiters
	delims := map[string]bool{
		":": true,
		"-": true,
		".": true,
	}
	_, exists := delims[delimiter]
	if !exists {
		return false
	}

	// valid chars
	chars := map[string]bool{
		"0": true,
		"1": true,
		"2": true,
		"3": true,
		"4": true,
		"5": true,
		"6": true,
		"7": true,
		"8": true,
		"9": true,
		"a": true,
		"b": true,
		"c": true,
		"d": true,
		"e": true,
		"f": true,
	}

	macSlice := strings.Split(mac, delimiter)
	// validates that each split has 2 chars
	for _, v := range macSlice {
		if len(v) != 2 {
			return false
		}
		v = strings.ToLower(v)
		vs := strings.Split(v, "")
		// validate that each char is in the valid chars slice
		for _, c := range vs {
			_, exists = chars[c]
			if !exists {
				return false
			}
		}
	}
	return true
}
