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

package util

import (
	"fmt"
	"os/user"
	"path"
	"regexp"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// ExpandHome does an expansion of ~/ or ~james/ into user's home dir value.
func ExpandHome(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		usr, err := user.Current()
		if err != nil {
			return p, fmt.Errorf("can't expand ~ into home directory")
		}
		return path.Join(usr.HomeDir, p[len("~/"):]), nil
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
		return path.Join(usr.HomeDir, p[len(match[0]):]), nil
	}

	return p, nil
}
