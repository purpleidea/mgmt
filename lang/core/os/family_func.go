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
	"os"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	simple.ModuleRegister(ModuleName, "family", &simple.Scaffold{
		T: types.NewType("func() str"),
		F: Family,
	})

	vars.ModuleRegister(ModuleName, "family_redhat", func() vars.Value {
		return &types.StrValue{
			V: familyRedHat,
		}
	})
	vars.ModuleRegister(ModuleName, "family_debian", func() vars.Value {
		return &types.StrValue{
			V: familyDebian,
		}
	})
	vars.ModuleRegister(ModuleName, "family_archlinux", func() vars.Value {
		return &types.StrValue{
			V: familyArchLinux,
		}
	})

	// TODO: Create a family method that will return a giant struct.
	simple.ModuleRegister(ModuleName, "is_redhat", &simple.Scaffold{
		T: types.NewType("func() bool"),
		F: IsRedHat,
	})
	simple.ModuleRegister(ModuleName, "is_debian", &simple.Scaffold{
		T: types.NewType("func() bool"),
		F: IsDebian,
	})
	simple.ModuleRegister(ModuleName, "is_archlinux", &simple.Scaffold{
		T: types.NewType("func() bool"),
		F: IsArchLinux,
	})
}

const (
	familyRedHat    = "redhat"
	familyDebian    = "debian"
	familyArchLinux = "archlinux"
)

// Family returns the distro family.
// TODO: Detect OS changes.
func Family(ctx context.Context, input []types.Value) (types.Value, error) {
	if b, err := isRedHat(ctx); err != nil {
		return nil, err
	} else if b {
		return &types.StrValue{
			V: familyRedHat,
		}, nil
	}
	if b, err := isDebian(ctx); err != nil {
		return nil, err
	} else if b {
		return &types.StrValue{
			V: familyDebian,
		}, nil
	}
	if b, err := isArchLinux(ctx); err != nil {
		return nil, err
	} else if b {
		return &types.StrValue{
			V: familyArchLinux,
		}, nil
	}
	return &types.StrValue{
		V: "", // unknown
	}, nil
}

// IsRedHat detects if the os family is redhat.
// TODO: Detect OS changes.
func IsRedHat(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := isRedHat(ctx)
	return &types.BoolValue{
		V: b,
	}, err
}

// IsDebian detects if the os family is debian.
// TODO: Detect OS changes.
func IsDebian(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := isDebian(ctx)
	return &types.BoolValue{
		V: b,
	}, err
}

// IsArchLinux detects if the os family is archlinux.
// TODO: Detect OS changes.
func IsArchLinux(ctx context.Context, input []types.Value) (types.Value, error) {
	b, err := isArchLinux(ctx)
	return &types.BoolValue{
		V: b,
	}, err
}

func isRedHat(ctx context.Context) (bool, error) {
	// TODO: use ctx around io operations
	_, err := os.Stat("/etc/redhat-release")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func isDebian(ctx context.Context) (bool, error) {
	// TODO: use ctx around io operations
	_, err := os.Stat("/etc/debian_version")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func isArchLinux(ctx context.Context) (bool, error) {
	// TODO: use ctx around io operations
	_, err := os.Stat("/etc/arch-release")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
