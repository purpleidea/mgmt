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

package coremath

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/multi"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	typInt := types.NewType("func() int")
	typFloat := types.NewType("func() float")
	multi.ModuleRegister(ModuleName, "fortytwo", &multi.Scaffold{
		I: &multi.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func() ?1"),
		M: multi.TypeMatch(map[string]interfaces.FuncSig{
			"func() int":   fortyTwo(typInt),
			"func() float": fortyTwo(typFloat),
		}),
		//M: func(typ *types.Type) (interfaces.FuncSig, error) {
		//	if typ == nil {
		//		return nil, fmt.Errorf("nil type")
		//	}
		//	if typ.Kind != types.KindFunc {
		//		return nil, fmt.Errorf("not a func")
		//	}
		//	if len(typ.Map) != 0 || len(typ.Ord) != 0 {
		//		return nil, fmt.Errorf("arg count wrong")
		//	}
		//	if err := typ.Out.Cmp(types.TypeInt); err == nil {
		//		return fortyTwo(typInt), nil
		//	}
		//	if err := typ.Out.Cmp(types.TypeFloat); err == nil {
		//		return fortyTwo(typFloat), nil
		//	}
		//	return nil, fmt.Errorf("can't use return type of: %s", typ.Out)
		//},
		D: FortyTwo, // get the docs from this
	})
}

// fortyTwo is a helper function to build the correct function for the desired
// signature, because the multi func API doesn't tell the implementing function
// what its signature should be! In the next version of this API, we could pass
// in a sig field, like how we demonstrate in the implementation of FortyTwo. If
// the API doesn't change, then this is an example of how to build this as a
// wrapper.
func fortyTwo(sig *types.Type) interfaces.FuncSig {
	return func(ctx context.Context, input []types.Value) (types.Value, error) {
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
