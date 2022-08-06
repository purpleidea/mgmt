// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package graph

import (
	"fmt"
	"os"
	"path"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// varDir returns the path to a working directory for the resource. It will try
// and create the directory first, and return an error if this failed. The dir
// should be cleaned up by the resource on Close if it wishes to discard the
// contents. If it does not, then a future resource with the same kind and name
// may see those contents in that directory. The resource should clean up the
// contents before use if it is important that nothing exist. It is always
// possible that contents could remain after an abrupt crash, so do not store
// overly sensitive data unless you're aware of the risks.
func (obj *State) varDir(extra string) (string, error) {
	// Using extra adds additional dirs onto our namespace. An empty extra
	// adds no additional directories.
	if obj.Prefix == "" { // safety
		return "", fmt.Errorf("the VarDir prefix is empty")
	}

	// an empty string at the end has no effect
	p := fmt.Sprintf("%s/", path.Join(obj.Prefix, extra))
	if err := os.MkdirAll(p, 0770); err != nil {
		return "", errwrap.Wrapf(err, "can't create prefix in: %s", p)
	}

	// returns with a trailing slash as per the mgmt file res convention
	return p, nil
}
