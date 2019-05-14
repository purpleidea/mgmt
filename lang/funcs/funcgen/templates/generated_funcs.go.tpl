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
{{ range $i, $func := .Packages }}	{{ if not (eq .Alias "") }}{{.Alias}} {{end}}"{{.Name}}"
{{ end }}
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
{{ range $i, $func := .Functions }}	simple.ModuleRegister("{{$func.MgmtPackage}}", "{{$func.MclName}}", &types.FuncValue{
		T: types.NewType("{{$func.Signature}}"),
		V: {{$func.InternalName}},
	})
{{ end }}
}
{{ range $i, $func := .Functions }}
{{$func.Help}}func {{$func.InternalName}}(input []types.Value) (types.Value, error) {
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
