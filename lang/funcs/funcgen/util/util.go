// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

// Package util provides some functions to be imported by the generated file.
package util

import (
	"github.com/purpleidea/mgmt/lang/types"
)

// MclListToGolang is a helper function that converts an mcl []str to the golang
// equivalent of []string. This is imported by the generated functions.
func MclListToGolang(val types.Value) []string {
	ret := []string{}
	for _, x := range val.List() {
		ret = append(ret, x.Str())
	}
	return ret
}
