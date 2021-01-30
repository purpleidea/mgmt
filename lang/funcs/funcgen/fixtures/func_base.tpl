// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"testpkg"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister("golang/testpkg", "all_kind", &types.FuncValue{
		T: types.NewType("func(x int, y str) float"),
		V: TestpkgAllKind,
	})
	simple.ModuleRegister("golang/testpkg", "to_upper", &types.FuncValue{
		T: types.NewType("func(s str) str"),
		V: TestpkgToUpper,
	})
	simple.ModuleRegister("golang/testpkg", "max", &types.FuncValue{
		T: types.NewType("func(x float, y float) float"),
		V: TestpkgMax,
	})
	simple.ModuleRegister("golang/testpkg", "with_error", &types.FuncValue{
		T: types.NewType("func(s str) str"),
		V: TestpkgWithError,
	})
	simple.ModuleRegister("golang/testpkg", "with_int", &types.FuncValue{
		T: types.NewType("func(s float, i int, x int, j int, k int, b bool, t str) str"),
		V: TestpkgWithInt,
	})
	simple.ModuleRegister("golang/testpkg", "super_byte", &types.FuncValue{
		T: types.NewType("func(s str, t str) str"),
		V: TestpkgSuperByte,
	})

}

func TestpkgAllKind(input []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: testpkg.AllKind(input[0].Int(), input[1].Str()),
	}, nil
}

func TestpkgToUpper(input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: testpkg.ToUpper(input[0].Str()),
	}, nil
}

func TestpkgMax(input []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: testpkg.Max(input[0].Float(), input[1].Float()),
	}, nil
}

func TestpkgWithError(input []types.Value) (types.Value, error) {
	v, err := testpkg.WithError(input[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

func TestpkgWithInt(input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: testpkg.WithInt(input[0].Float(), int(input[1].Int()), input[2].Int(), int(input[3].Int()), int(input[4].Int()), input[5].Bool(), input[6].Str()),
	}, nil
}

func TestpkgSuperByte(input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(testpkg.SuperByte([]byte(input[0].Str()), input[1].Str())),
	}, nil
}
