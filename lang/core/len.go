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

package core

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.Register("len", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(?1) int"),
		// TODO: should we add support for struct or func lengths?
		C: simple.TypeMatch([]string{
			"func(str) int",
			"func([]?1) int",
			"func(map{?1: ?2}) int",
		}),
		//C: func(typ *types.Type) error {
		//	if typ == nil {
		//		return fmt.Errorf("nil type")
		//	}
		//	if typ.Kind != types.KindFunc {
		//		return fmt.Errorf("not a func")
		//	}
		//	if len(typ.Map) != 1 || len(typ.Ord) != 1 {
		//		return fmt.Errorf("arg count wrong")
		//	}
		//	if err := typ.Out.Cmp(types.TypeInt); err != nil {
		//		return err
		//	}
		//	t := typ.Map[typ.Ord[0]]
		//	if t.Cmp(types.TypeStr) == nil {
		//		return nil // func(str) int
		//	}
		//	if t.Kind == types.KindList {
		//		return nil // func([]?1) int
		//	}
		//	if t.Kind == types.KindMap {
		//		return nil // func(map{?1: ?2}) int
		//	}
		//	return fmt.Errorf("can't determine length of %s", t)
		//},
		F: Len,
	})
}

// Len returns the number of elements in a list or the number of key pairs in a
// map. It can operate on either of these types.
func Len(ctx context.Context, input []types.Value) (types.Value, error) {
	var length int
	switch k := input[0].Type().Kind; k {
	case types.KindStr:
		length = len(input[0].Str())
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
