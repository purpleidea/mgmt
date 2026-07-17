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

package coreencoding

import (
	"context"
	"encoding/base64"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	simple.ModuleRegister(ModuleName, "base64_encode", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(data str) str"),
		F: Base64Encode,
	})
	simple.ModuleRegister(ModuleName, "base64_decode", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(data str) str"),
		F: Base64Decode,
	})
}

// Base64Encode encodes a string with the standard base64 encoding.
func Base64Encode(ctx context.Context, input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: base64.StdEncoding.EncodeToString([]byte(input[0].Str())),
	}, nil
}

// Base64Decode decodes a string with the standard base64 encoding. It errors if
// the input is not valid base64.
func Base64Decode(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := base64.StdEncoding.DecodeString(input[0].Str())
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not decode base64")
	}
	return &types.StrValue{
		V: string(b),
	}, nil
}
