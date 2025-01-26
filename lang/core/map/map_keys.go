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

package coremap

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "keys", &simple.Scaffold{
		// TODO: Maybe saying ?0 could mean we don't care about that type?
		T: types.NewType("func(x map{?1: ?2}) []?1"),
		F: MapKeys,
	})
}

// MapKeys returns a list of keys from the map.
func MapKeys(ctx context.Context, input []types.Value) (types.Value, error) {
	m, ok := input[0].(*types.MapValue)
	if !ok {
		// programming error
		return nil, fmt.Errorf("invalid map input")
	}

	values := []types.Value{}

	for k := range m.V {
		values = append(values, k)
	}

	return &types.ListValue{
		T: types.NewType(fmt.Sprintf("[]%s", m.Type().Key)),
		V: values,
	}, nil
}
