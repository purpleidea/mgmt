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
{{ range $i, $func := .Packages }}	{{ if not (eq .Alias "") }}{{.Alias}} {{end}}"{{.Name}}"
{{ end }}
	"github.com/purpleidea/mgmt/lang/funcs/funcgen/util"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
{{ range $i, $func := .Functions }}	simple.ModuleRegister("{{$func.MgmtPackage}}", "{{$func.MclName}}", &simple.Scaffold{
		// XXX: pull these from a database, remove the impure functions
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		T: types.NewType("{{$func.Signature}}"),
		F: {{$func.InternalName}},
	})
{{ end }}
}
{{ range $i, $func := .Functions }}
{{$func.Help}}func {{$func.InternalName}}(ctx context.Context, args []types.Value) (types.Value, error) {
{{- if $func.Errorful }}
	v, err := {{ if not (eq $func.GolangPackage.Alias "") }}{{$func.GolangPackage.Alias}}{{else}}{{$func.GolangPackage.Name}}{{end}}.{{$func.GolangFunc}}({{$func.MakeGolangArgs}})
	if err != nil {
		return nil, err
	}
	return &types.{{$func.MakeGoReturn}}Value{
		V: {{$func.ConvertStart}}v{{$func.ConvertStop}},
	}, nil
{{ else }}
	return &types.{{$func.MakeGoReturn}}Value{
		V: {{$func.ConvertStart}}{{ if not (eq $func.GolangPackage.Alias "") }}{{$func.GolangPackage.Alias}}{{else}}{{$func.GolangPackage.Name}}{{end}}.{{$func.GolangFunc}}({{$func.MakeGolangArgs}}{{$func.ConvertStop}}),
	}, nil
{{ end -}}
}
{{ end -}}
