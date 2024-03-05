// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package core

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simplepoly"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simplepoly.Register("panic", []*types.FuncValue{
		{
			T: types.NewType("func(x bool) bool"),
			V: Panic,
		},
		{
			T: types.NewType("func(x str) bool"),
			V: Panic,
		},
	})
}

// Panic returns an error when it receives a non-empty string or a true boolean.
// The error should cause the function engine to shutdown. If there's no error,
// it returns false.
func Panic(input []types.Value) (types.Value, error) {
	switch k := input[0].Type().Kind; k {
	case types.KindBool:
		if input[0].Bool() {
			return nil, fmt.Errorf("bool panic occurred")
		}
	case types.KindStr:
		if s := input[0].Str(); s != "" {
			return nil, fmt.Errorf("str panic occurred: %s", s)
		}
	default:
		return nil, fmt.Errorf("unsupported kind: %+v", k)
	}

	return &types.BoolValue{
		V: false,
	}, nil
}
