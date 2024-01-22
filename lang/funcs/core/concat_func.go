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

package core

import (
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ConcatFuncName is the name this function is registered as.
	ConcatFuncName = funcs.ConcatFuncName
)

func init() {
	simple.Register(ConcatFuncName, &types.FuncValue{
		T: types.NewType("func(a str, b str) str"),
		V: Concat,
	})
}

// Concat concatenates two strings together.
func Concat(input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: input[0].Str() + input[1].Str(),
	}, nil
}
