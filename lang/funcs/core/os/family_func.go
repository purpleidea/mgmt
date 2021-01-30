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

package coreos

import (
	"os"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	// TODO: Create a family method that will return a giant struct.
	simple.ModuleRegister(ModuleName, "is_debian", &types.FuncValue{
		T: types.NewType("func() bool"),
		V: IsDebian,
	})
	simple.ModuleRegister(ModuleName, "is_redhat", &types.FuncValue{
		T: types.NewType("func() bool"),
		V: IsRedHat,
	})
	simple.ModuleRegister(ModuleName, "is_archlinux", &types.FuncValue{
		T: types.NewType("func() bool"),
		V: IsArchLinux,
	})
}

// IsDebian detects if the os family is debian.
// TODO: Detect OS changes.
func IsDebian(input []types.Value) (types.Value, error) {
	exists := true
	_, err := os.Stat("/etc/debian_version")
	if os.IsNotExist(err) {
		exists = false
	}
	return &types.BoolValue{
		V: exists,
	}, nil
}

// IsRedHat detects if the os family is redhat.
// TODO: Detect OS changes.
func IsRedHat(input []types.Value) (types.Value, error) {
	exists := true
	_, err := os.Stat("/etc/redhat-release")
	if os.IsNotExist(err) {
		exists = false
	}
	return &types.BoolValue{
		V: exists,
	}, nil
}

// IsArchLinux detects if the os family is archlinux.
// TODO: Detect OS changes.
func IsArchLinux(input []types.Value) (types.Value, error) {
	exists := true
	_, err := os.Stat("/etc/arch-release")
	if os.IsNotExist(err) {
		exists = false
	}
	return &types.BoolValue{
		V: exists,
	}, nil
}
