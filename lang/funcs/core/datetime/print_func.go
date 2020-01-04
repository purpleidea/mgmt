// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package coredatetime

import (
	"fmt"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	// FIXME: consider renaming this to printf, and add in a format string?
	simple.ModuleRegister(ModuleName, "print", &types.FuncValue{
		T: types.NewType("func(a int) str"),
		V: func(input []types.Value) (types.Value, error) {
			epochDelta := input[0].Int()
			if epochDelta < 0 {
				return nil, fmt.Errorf("epoch delta must be positive")
			}
			return &types.StrValue{
				V: time.Unix(epochDelta, 0).String(),
			}, nil
		},
	})
}
