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
	"math"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ListLookupDefaultFuncName is the name this function is registered as.
	ListLookupDefaultFuncName = "list_lookup_default"
)

func init() {
	simple.Register(ListLookupDefaultFuncName, &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(list []?1, index int, default ?1) ?1"),
		F: ListLookupDefault,
	})
}

// ListLookupDefault returns the value corresponding to the input index in the
// list. If the value is not present, it returns the default value supplied.
func ListLookupDefault(ctx context.Context, input []types.Value) (types.Value, error) {
	l := input[0].(*types.ListValue)
	index := input[1].Int()

	// TODO: should we handle overflow by returning default?
	if index > math.MaxInt { // max int size varies by arch
		return nil, fmt.Errorf("list index overflow, got: %d, max is: %d", index, math.MaxInt)
	}
	if index < 0 { // lists can't have negative indexes (for now)
		return nil, fmt.Errorf("list index negative, got: %d", index)
	}

	val, exists := l.Lookup(int(index))
	if !exists {
		return input[2], nil // default value
	}
	return val, nil
}
