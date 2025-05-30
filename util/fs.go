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

	"github.com/spf13/afero"
)

// AferoFs is a simple wrapper to a file system to be used for standalone
// deploys. This is basically a pass-through so that we fulfill the same
// interface that the deploy mechanism uses. If you give it Scheme and Path
// fields it will use those to build the URI. NOTE: This struct is here, since I
// don't know where else to put it for now.
type AferoFs struct {
	*afero.Afero

	Scheme string
	Path   string
}

// URI returns the unique URI of this filesystem. It returns the root path.
func (obj *AferoFs) URI() string {
	if obj.Scheme != "" {
		// if obj.Path is not empty and doesn't start with a slash, then
		// the first chunk will disappear when being parsed with stdlib
		return obj.Scheme + "://" + obj.Path
	}
	return fmt.Sprintf("%s://"+"/", obj.Name()) // old
}
