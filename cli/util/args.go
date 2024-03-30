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

package util

import (
	"reflect"
	"strings"
)

// LookupSubcommand returns the name of the subcommand in the obj, of a struct.
// This is useful for determining the name of the subcommand that was activated.
// It returns an empty string if a specific name was not found.
func LookupSubcommand(obj interface{}, st interface{}) string {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr { // max one de-referencing
		val = val.Elem()
	}

	v := reflect.ValueOf(st) // value of the struct
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := val.Field(i) // value of the field
		if f.Interface() != v.Interface() {
			continue
		}

		field := typ.Field(i)
		alias, ok := field.Tag.Lookup("arg")
		if !ok {
			continue
		}

		// XXX: `arg` needs a split by comma first or fancier parsing
		prefix := "subcommand"
		split := strings.Split(alias, ":")
		if len(split) != 2 || split[0] != prefix {
			continue
		}

		return split[1] // found
	}
	return "" // not found
}

// EmptyArgs is the empty CLI parsing structure and type of the parsed result.
type EmptyArgs struct{}

// LangArgs is the lang CLI parsing structure and type of the parsed result.
type LangArgs struct {
	// Input is the input mcl code or file path or any input specification.
	Input string `arg:"positional,required"`

	// TODO: removed (temporarily?)
	//Stdin bool `arg:"--stdin" help:"use passthrough stdin"`

	Download     bool `arg:"--download" help:"download any missing imports"`
	OnlyDownload bool `arg:"--only-download" help:"stop after downloading any missing imports"`
	Update       bool `arg:"--update" help:"update all dependencies to the latest versions"`

	OnlyUnify          bool     `arg:"--only-unify" help:"stop after type unification"`
	SkipUnify          bool     `arg:"--skip-unify" help:"skip type unification"`
	UnifySolver        *string  `arg:"--unify-name" help:"pick a specific unification solver"`
	UnifyOptimizations []string `arg:"--unify-optimizations" help:"list of unification optimizations to request (experts only)"`

	Depth int `arg:"--depth" default:"-1" help:"max recursion depth limit (-1 is unlimited)"`

	// The default of 0 means any error is a failure by default.
	Retry int `arg:"--depth" help:"max number of retries (-1 is unlimited)"`

	ModulePath string `arg:"--module-path,env:MGMT_MODULE_PATH" help:"choose the modules path (absolute)"`
}

// YamlArgs is the yaml CLI parsing structure and type of the parsed result.
type YamlArgs struct {
	// Input is the input yaml code or file path or any input specification.
	Input string `arg:"positional,required"`
}
