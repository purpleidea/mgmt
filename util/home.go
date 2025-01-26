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
	"os/user"
	"path"
	"regexp"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// ExpandHome does an expansion of ~/ or ~james/ into user's home dir value. If
// the input path ends in a slash, the result does too.
func ExpandHome(p string) (string, error) {
	suffix := "" // If it the input ends with a slash, so should the output.
	if strings.HasSuffix(p, "/") {
		suffix = "/"
	}
	if strings.HasPrefix(p, "~/") {
		usr, err := user.Current()
		if err != nil {
			return p, fmt.Errorf("can't expand ~ into home directory")
		}
		return path.Join(usr.HomeDir, p[len("~/"):]) + suffix, nil
	}

	// check if provided path is in format ~username and keep track of provided username
	r, err := regexp.Compile("~([^/]+)/")
	if err != nil {
		return p, errwrap.Wrapf(err, "can't compile regexp")
	}
	if match := r.FindStringSubmatch(p); match != nil {
		username := match[len(match)-1]
		usr, err := user.Lookup(username)
		if err != nil {
			return p, fmt.Errorf("can't expand %s into home directory", match[0])
		}
		return path.Join(usr.HomeDir, p[len(match[0]):]) + suffix, nil
	}

	return p, nil
}
