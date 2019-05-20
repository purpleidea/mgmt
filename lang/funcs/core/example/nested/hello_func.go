// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package corenested

import (
	coreexample "github.com/purpleidea/mgmt/lang/funcs/core/example"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(coreexample.ModuleName+"/"+ModuleName, "hello", &types.FuncValue{
		T: types.NewType("func() str"),
		V: Hello,
	})
}

// Hello returns some string. This is just to test nesting.
func Hello(input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: "Hello!",
	}, nil
}
