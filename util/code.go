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

package util

import (
	"strings"
)

// Code takes a code block as a backtick enclosed `heredoc` and removes any
// common indentation from each line. This helps inline code as strings to be
// formatted nicely without unnecessary indentation. It also drops the very
// first line of code if it has zero length.
func Code(code string) string {
	output := []string{}
	lines := strings.Split(code, "\n")
	var found bool
	var strip string // prefix to remove
	for i, x := range lines {
		if !found && len(x) > 0 {
			for j := 0; j < len(x); j++ {
				if x[j] != '\t' {
					break
				}
				strip += "\t"
			}
			// otherwise, there's no indentation
			found = true
		}
		if i == 0 && len(x) == 0 { // drop first line if it's empty
			continue
		}

		s := strings.TrimPrefix(x, strip)
		output = append(output, s)
	}

	return strings.Join(output, "\n")
}
