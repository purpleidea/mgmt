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
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

const structDistroUID = "struct{distro str; version str; arch str}"

var typeParseDistroUID = types.NewType(fmt.Sprintf("func(str) %s", structDistroUID))

func init() {
	simple.ModuleRegister(ModuleName, "parse_distro_uid", &simple.Scaffold{
		T: typeParseDistroUID,
		F: ParseDistroUID,
	})
}

// ParseDistroUID parses a distro UID into its component values. If it cannot
// parse correctly, all the struct fields have the zero values.
// NOTE: The UID pattern is subject to change.
func ParseDistroUID(ctx context.Context, input []types.Value) (types.Value, error) {
	fn := func(distro, version, arch string) (types.Value, error) {
		st := types.NewStruct(types.NewType(structDistroUID))
		if err := st.Set("distro", &types.StrValue{V: distro}); err != nil {
			return nil, err
		}
		if err := st.Set("version", &types.StrValue{V: version}); err != nil {
			return nil, err
		}
		if err := st.Set("arch", &types.StrValue{V: arch}); err != nil {
			return nil, err
		}

		return st, nil
	}

	distro, version, arch, err := parseDistroUID(input[0].Str())
	//if err != nil {
	//	return fn("", "", "") // empty
	//}
	_ = err
	return fn(distro, version, arch)
}

// parseDistroUID returns distro, version, arch in that order after parsing a
// string like: fedora39-x86_64 and returning error if it doesn't match.
// NOTE: The UID pattern is subject to change.
func parseDistroUID(uid string) (string, string, string, error) {
	before, arch, found := strings.Cut(uid, "-")
	if !found {
		return "", "", "", fmt.Errorf("dash not found")
	}

	distro := strings.TrimRight(before, "0123456789")
	version := before[len(distro):]
	if distro == "" || version == "" || arch == "" {
		return "", "", "", fmt.Errorf("got empty value")
	}

	// TODO: check for valid distro or arch?

	return distro, version, arch, nil
}
