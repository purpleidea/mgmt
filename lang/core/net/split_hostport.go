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

package corenet

import (
	"context"
	"fmt"
	"net"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	splitHostPortFieldHost = "host"
	splitHostPortFieldPort = "port"
)

var splitHostPortReturnType = fmt.Sprintf(
	"struct{%s str; %s str}",
	splitHostPortFieldHost,
	splitHostPortFieldPort,
)

func init() {
	simple.ModuleRegister(ModuleName, "split_hostport", &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType(fmt.Sprintf("func(str) %s", splitHostPortReturnType)),
		F: SplitHostPort,
	})
}

// SplitHostPort takes an input as a string, and splits it into host and port.
// The function will error out if the given input doesn't contain a valid host
// port pair, as this is a requirement of the underlying library.
func SplitHostPort(ctx context.Context, input []types.Value) (types.Value, error) {
	h, p, err := net.SplitHostPort(input[0].Str())
	if err != nil {
		return nil, errwrap.Wrapf(err, "error parsing the input")
	}

	v := types.NewStruct(types.NewType(splitHostPortReturnType))
	if err := v.Set(splitHostPortFieldHost, &types.StrValue{V: h}); err != nil {
		return nil, errwrap.Wrapf(err, "invalid host value")
	}
	if err := v.Set(splitHostPortFieldPort, &types.StrValue{V: p}); err != nil {
		return nil, errwrap.Wrapf(err, "invalid port value")
	}

	return v, nil
}
