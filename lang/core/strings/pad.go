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

package corestrings

import (
	"context"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
)

func init() {
	simple.ModuleRegister(ModuleName, "left_pad", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, pad str, len int) str"),
		F: LeftPad,
	})
	simple.ModuleRegister(ModuleName, "right_pad", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, pad str, len int) str"),
		F: RightPad,
	})
}

// LeftPad adds multiples of the pad string to the left of the input string
// until it reaches a minimum length. If the padding string is not an integer
// multiple of the missing length to pad, then this will overshoot. It is better
// to overshoot than to undershoot because if you need a string of a precise
// length, then it's easier to truncate the result, rather than having to pad
// even more. Most scenarios pad with a single char meaning this is not even an
// issue.
func LeftPad(ctx context.Context, input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: util.LeftPad(input[0].Str(), input[1].Str(), int(input[2].Int())),
	}, nil
}

// RightPad adds multiples of the pad string to the right of the input string
// until it reaches a minimum length. If the padding string is not an integer
// multiple of the missing length to pad, then this will overshoot. It is better
// to overshoot than to undershoot because if you need a string of a precise
// length, then it's easier to truncate the result, rather than having to pad
// even more. Most scenarios pad with a single char meaning this is not even an
// issue.
func RightPad(ctx context.Context, input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: util.RightPad(input[0].Str(), input[1].Str(), int(input[2].Int())),
	}, nil
}
