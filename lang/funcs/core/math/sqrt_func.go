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

package coremath

import (
	"fmt"
	"math"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "sqrt", &types.FuncValue{
		T: types.NewType("func(x float) float"),
		V: Sqrt,
	})
}

// Sqrt returns sqrt(x), the square root of x.
func Sqrt(input []types.Value) (types.Value, error) {
	x := input[0].Float()
	y := math.Sqrt(x)
	if math.IsNaN(y) {
		return nil, fmt.Errorf("result is not a number")
	}
	if math.IsInf(y, 1) {
		return nil, fmt.Errorf("result is positive infinity")
	}
	if math.IsInf(y, -1) {
		return nil, fmt.Errorf("result is negative infinity")
	}
	return &types.FloatValue{
		V: y,
	}, nil
}
