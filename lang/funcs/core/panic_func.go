// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

package core

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.Register("panic", &types.FuncValue{
		T: types.NewType("func(x str) str"),
		V: Panic,
	})
}

// Panic returns an error when it receives a non-empty string. The error should
// cause the function engine to shutdown.
func Panic(input []types.Value) (types.Value, error) {
	if s := input[0].Str(); s != "" {
		// This StrValue not really used here, since we error...
		return &types.StrValue{
			V: s,
		}, fmt.Errorf("panic occurred: %s", s)
	}
	return &types.StrValue{
		V: "panic", // name can't be empty
	}, nil
}
