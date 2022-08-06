// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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
	simple.ModuleRegister(ModuleName, "pow", &types.FuncValue{
		T: types.NewType("func(x float, y float) float"),
		V: Pow,
	})
}

// Pow returns x ^ y, the base-x exponential of y.
func Pow(input []types.Value) (types.Value, error) {
	x, y := input[0].Float(), input[1].Float()
	// FIXME: check for overflow
	z := math.Pow(x, y)
	if math.IsNaN(z) {
		return nil, fmt.Errorf("result is not a number")
	}
	if math.IsInf(z, 1) {
		return nil, fmt.Errorf("result is positive infinity")
	}
	if math.IsInf(z, -1) {
		return nil, fmt.Errorf("result is negative infinity")
	}
	// TODO: consider only returning floats, and adding isinf and
	// isnan functions so that users can decide for themselves...
	return &types.FloatValue{
		V: z,
	}, nil
}
