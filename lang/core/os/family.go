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

package coreos

import (
	"context"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/types"
	distroUtil "github.com/purpleidea/mgmt/util/distro"
)

func init() {
	simple.ModuleRegister(ModuleName, "family", &simple.Scaffold{
		I: &simple.Info{
			// In theory these are dependent on runtime.
			Pure: false,
			Memo: false,
			Fast: true,
			Spec: false,
		},
		T: types.NewType("func() str"),
		F: Family,
	})

	vars.ModuleRegister(ModuleName, "family_redhat", func() vars.Value {
		return &types.StrValue{
			V: distroUtil.FamilyRedHat,
		}
	})
	vars.ModuleRegister(ModuleName, "family_debian", func() vars.Value {
		return &types.StrValue{
			V: distroUtil.FamilyDebian,
		}
	})
	vars.ModuleRegister(ModuleName, "family_archlinux", func() vars.Value {
		return &types.StrValue{
			V: distroUtil.FamilyArchLinux,
		}
	})

	// TODO: Create a family method that will return a giant struct.
	simple.ModuleRegister(ModuleName, "is_family_redhat", &simple.Scaffold{
		I: &simple.Info{
			// In theory these are dependent on runtime.
			Pure: false,
			Memo: false,
			Fast: true,
			Spec: false,
		},
		T: types.NewType("func() bool"),
		F: IsFamilyRedHat,
	})
	simple.ModuleRegister(ModuleName, "is_family_debian", &simple.Scaffold{
		I: &simple.Info{
			// In theory these are dependent on runtime.
			Pure: false,
			Memo: false,
			Fast: true,
			Spec: false,
		},
		T: types.NewType("func() bool"),
		F: IsFamilyDebian,
	})
	simple.ModuleRegister(ModuleName, "is_family_archlinux", &simple.Scaffold{
		I: &simple.Info{
			// In theory these are dependent on runtime.
			Pure: false,
			Memo: false,
			Fast: true,
			Spec: false,
		},
		T: types.NewType("func() bool"),
		F: IsFamilyArchLinux,
	})
}

// Family returns the distro family.
// TODO: Detect OS changes.
func Family(ctx context.Context, input []types.Value) (types.Value, error) {
	s, err := distroUtil.Family(ctx)
	if err != nil {
		return nil, err
	}
	return &types.StrValue{
		V: s, // empty if unknown
	}, nil
}

// IsFamilyRedHat detects if the os family is redhat.
// TODO: Detect OS changes.
func IsFamilyRedHat(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := distroUtil.IsFamilyRedHat(ctx)
	return &types.BoolValue{
		V: b,
	}, err
}

// IsFamilyDebian detects if the os family is debian.
// TODO: Detect OS changes.
func IsFamilyDebian(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := distroUtil.IsFamilyDebian(ctx)
	return &types.BoolValue{
		V: b,
	}, err
}

// IsFamilyArchLinux detects if the os family is archlinux.
// TODO: Detect OS changes.
func IsFamilyArchLinux(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := distroUtil.IsFamilyArchLinux(ctx)
	return &types.BoolValue{
		V: b,
	}, err
}
