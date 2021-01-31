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

package coremath

import (
	"fmt"
	"math"

	"github.com/purpleidea/mgmt/lang/funcs/simplepoly"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simplepoly.ModuleRegister(ModuleName, "mod", []*types.FuncValue{
		{
			T: types.NewType("func(int, int) int"),
			V: Mod,
		},
		{
			T: types.NewType("func(float, float) float"),
			V: Mod,
		},
	})
}

// Mod returns mod(x, y), the remainder of x/y. The two values must be either
// both of KindInt or both of KindFloat, and it will return the same kind. If
// you pass in a divisor of zero, this will error, eg: mod(x, 0) = NaN.
// TODO: consider returning zero instead of erroring?
func Mod(input []types.Value) (types.Value, error) {
	var x, y float64
	var float bool
	k := input[0].Type().Kind
	switch k {
	case types.KindFloat:
		float = true
		x = input[0].Float()
		y = input[1].Float()
	case types.KindInt:
		x = float64(input[0].Int())
		y = float64(input[1].Int())
	default:
		return nil, fmt.Errorf("unexpected kind: %s", k)
	}
	z := math.Mod(x, y)
	if math.IsNaN(z) {
		return nil, fmt.Errorf("result is not a number")
	}
	if math.IsInf(z, 1) {
		return nil, fmt.Errorf("unexpected positive infinity")
	}
	if math.IsInf(z, -1) {
		return nil, fmt.Errorf("unexpected negative infinity")
	}
	if float {
		return &types.FloatValue{
			V: z,
		}, nil
	}
	return &types.IntValue{
		V: int64(z), // XXX: does this truncate?
	}, nil
}
