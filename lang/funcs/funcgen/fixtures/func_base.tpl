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
	"testpkg"

	"github.com/purpleidea/mgmt/lang/funcs/funcgen/util"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister("golang/testpkg", "all_kind", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x int, y str) float"),
		F: TestpkgAllKind,
	})
	simple.ModuleRegister("golang/testpkg", "to_upper", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: TestpkgToUpper,
	})
	simple.ModuleRegister("golang/testpkg", "max", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(x float, y float) float"),
		F: TestpkgMax,
	})
	simple.ModuleRegister("golang/testpkg", "with_error", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str) str"),
		F: TestpkgWithError,
	})
	simple.ModuleRegister("golang/testpkg", "with_int", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s float, i int, x int, j int, k int, b bool, t str) str"),
		F: TestpkgWithInt,
	})
	simple.ModuleRegister("golang/testpkg", "super_byte", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("func(s str, t str) str"),
		F: TestpkgSuperByte,
	})

}

func TestpkgAllKind(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: testpkg.AllKind(args[0].Int(), args[1].Str()),
	}, nil
}

func TestpkgToUpper(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: testpkg.ToUpper(args[0].Str()),
	}, nil
}

func TestpkgMax(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.FloatValue{
		V: testpkg.Max(args[0].Float(), args[1].Float()),
	}, nil
}

func TestpkgWithError(ctx context.Context, args []types.Value) (types.Value, error) {
	v, err := testpkg.WithError(args[0].Str())
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: v,
	}, nil
}

func TestpkgWithInt(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: testpkg.WithInt(args[0].Float(), int(args[1].Int()), args[2].Int(), int(args[3].Int()), int(args[4].Int()), args[5].Bool(), args[6].Str()),
	}, nil
}

func TestpkgSuperByte(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: string(testpkg.SuperByte([]byte(args[0].Str()), args[1].Str())),
	}, nil
}
