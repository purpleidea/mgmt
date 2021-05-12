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

	"github.com/purpleidea/mgmt/lang/funcs/simplepoly"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	typInt := types.NewType("func() int")
	typFloat := types.NewType("func() float")
	simplepoly.ModuleRegister(ModuleName, "fortytwo", []*types.FuncValue{
		{
			T: typInt,
			V: fortyTwo(typInt), // generate the correct function here
		},
		{
			T: typFloat,
			V: fortyTwo(typFloat),
		},
	})
}

// fortyTwo is a helper function to build the correct function for the desired
// signature, because the simplepoly API doesn't tell the implementing function
// what its signature should be! In the next version of this API, we could pass
// in a sig field, like how we demonstrate in the implementation of FortyTwo. If
// the API doesn't change, then this is an example of how to build this as a
// wrapper.
func fortyTwo(sig *types.Type) func([]types.Value) (types.Value, error) {
	return func(input []types.Value) (types.Value, error) {
		return FortyTwo(sig, input)
	}
}

// FortyTwo returns 42 as either an int or a float. This is not especially
// useful, but was built for a fun test case of a simple poly function with two
// different return types, but no input args.
func FortyTwo(sig *types.Type, input []types.Value) (types.Value, error) {
	if sig.Out.Kind == types.KindInt {
		return &types.IntValue{
			V: 42,
		}, nil
	}
	if sig.Out.Kind == types.KindFloat {
		return &types.FloatValue{
			V: 42.0,
		}, nil
	}
	return nil, fmt.Errorf("unknown output type: %+v", sig.Out)
}
