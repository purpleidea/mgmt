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

package core

import (
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
{{ range $i, $func := .Functions }}	simple.ModuleRegister("{{$func.MgmtPackage}}", "{{$func.MgmtName}}", &types.FuncValue{
		T: types.NewType("{{$func.Signature}}"),
		V: {{$func.GoFunc}},
	})
{{ end }}
}
{{ range $i, $func := .Functions }}
// {{$func.GoFunc}} {{$func.Help}}
func {{$func.GoFunc}}(input []types.Value) (types.Value, error) {
	return &types.{{$func.MakeGoReturn}}Value{
		V: {{$func.GoPackage}}.{{$func.GoFunc}}({{$func.MakeGoArgs}}),
	}, nil
}
{{ end -}}
