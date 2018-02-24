// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package simplepoly // TODO: should this be in its own individual package?

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	Register("len", []*types.FuncValue{
		{
			T: types.NewType("func([]variant) int"),
			V: Len,
		},
		{
			T: types.NewType("func({variant: variant}) int"),
			V: Len,
		},
		// TODO: should we add support for struct or func lengths?
	})
}

// Len returns the number of elements in a list or the number of key pairs in a
// map. It can operate on either of these types.
func Len(input []types.Value) (types.Value, error) {
	var length int
	switch k := input[0].Type().Kind; k {
	case types.KindList:
		length = len(input[0].List())
	case types.KindMap:
		length = len(input[0].Map())

	default:
		return nil, fmt.Errorf("unsupported kind: %+v", k)
	}

	return &types.IntValue{
		V: int64(length),
	}, nil
}
