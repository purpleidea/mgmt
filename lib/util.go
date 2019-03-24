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

package lib

import (
	"strings"
)

// safeProgram returns the correct program string when given a buggy variant.
func safeProgram(program string) string {
	// FIXME: in sub commands, the cli package appends a space and the sub
	// command name at the end. hack around this by only using the first bit
	// see: https://github.com/urfave/cli/issues/783 for more details...
	split := strings.Split(program, " ")
	program = split[0]
	//if program == "" {
	//	program = "<unknown>"
	//}
	return program
}
