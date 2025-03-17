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

package coredatetime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "weekday", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(a int) str"),
		F: Weekday,
	})
}

// Weekday returns the lowercased day of the week corresponding to the input
// time. The time is the number of seconds since the epoch, and matches what
// comes from our Now function.
func Weekday(ctx context.Context, input []types.Value) (types.Value, error) {
	epochDelta := input[0].Int()
	if epochDelta < 0 {
		return nil, fmt.Errorf("epoch delta must be positive")
	}

	weekday := time.Unix(epochDelta, 0).Weekday()
	return &types.StrValue{
		V: strings.ToLower(weekday.String()),
	}, nil
}
