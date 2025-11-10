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
	"fmt"
	"regexp"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// ValidUser determines if the string is a valid user name. This follows the
// strict systemd description: https://systemd.io/USER_NAMES/ which is sane.
// This matcher generally works the same for group names as well.
func ValidUser(s string) error {
	// check if provided path is in format ~username and keep track of provided username
	r, err := regexp.Compile("^[a-zA-Z_][a-zA-Z0-9_-]{0,30}$")
	if err != nil {
		// TODO: should be a panic?
		return errwrap.Wrapf(err, "can't compile regexp")
	}
	if match := r.FindStringSubmatch(s); match != nil && len(match) == 1 && match[0] == s {
		return nil // success
	}

	return fmt.Errorf("invalid value")
}
