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
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	jsonUtil "github.com/purpleidea/mgmt/lang/types/json"
)

const (
	// EncodeJSONFuncName is the name this function is registered as.
	EncodeJSONFuncName = "encode_json"
)

func init() {
	simple.ModuleRegister(ModuleName, EncodeJSONFuncName, &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x ?1) str"),
		C: func(typ *types.Type) error {
			if typ == nil || typ.Kind != types.KindFunc || len(typ.Ord) != 1 {
				return fmt.Errorf("invalid type: %s", typ)
			}
			return jsonUtil.CheckType(typ.Map[typ.Ord[0]])
		},
		F: EncodeJSON,
	})
}

// EncodeJSON returns the JSON of a value, which decode_json can read back with
// the same type, eg: to put it into a document with json:edit. A struct is an
// object, with a key for each field, and so is a map, which must have str keys.
// The keys are sorted, so a value always has the same JSON. A func can't be
// encoded.
func EncodeJSON(ctx context.Context, input []types.Value) (types.Value, error) {
	s, err := jsonUtil.ValueToJSON(input[0])
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: s,
	}, nil
}
