// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// +build !go1.7

package resources

import (
	"strconv"

	group "github.com/hnakamur/group"
	errwrap "github.com/pkg/errors"
)

// gid returns the group id for the group specified in the yaml file graph.
// Caller should first check obj.Group is not empty
func (obj *FileRes) gid() (int, error) {
	g2, err2 := group.LookupId(obj.Group)
	if err2 == nil {
		return strconv.Atoi(g2.Gid)
	}

	g, err := group.Lookup(obj.Group)
	if err == nil {
		return strconv.Atoi(g.Gid)
	}

	return -1, errwrap.Wrapf(err, "Group lookup error (%s)", obj.Group)
}
