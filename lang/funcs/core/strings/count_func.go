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

package corestrings

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "count", &types.FuncValue{
		T: types.NewType("func(s str, substr str) str"),
		V: Count,
	})
}

// Count counts the number of non-overlapping instances of substr in s. If
// substr is an empty string, Count returns 0 if empty string and len(string) if
// substr is empty
func Count(input []types.Value) (types.Value, error) {
	s, substr := input[0].Str(), input[1].Str()
	if s == "" {
		return &types.StrValue{V: "0"}, nil
	}

	count := strings.Count(s, substr)
	if substr == "" {
		count--
	}
	return &types.StrValue{V: fmt.Sprint(count)}, nil
}
