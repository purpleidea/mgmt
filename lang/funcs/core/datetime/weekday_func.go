// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
	"strings"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "weekday", &types.FuncValue{
		T: types.NewType("func(a int) str"),
		V: Weekday,
	})
}

// Weekday returns the lowercased day of the week corresponding to the input
// time. The time is the number of seconds since the epoch, and matches what
// comes from our Now function.
func Weekday(input []types.Value) (types.Value, error) {
	epochDelta := input[0].Int()
	if epochDelta < 0 {
		return nil, fmt.Errorf("epoch delta must be positive")
	}

	weekday := time.Unix(epochDelta, 0).Weekday()
	return &types.StrValue{
		V: strings.ToLower(weekday.String()),
	}, nil
}
