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

package traits

// Named contains a general implementation of the properties and methods needed
// to support named resources. It should be used as a starting point to avoid
// re-implementing the straightforward name methods.
type Named struct {
	// Xname is the stored name. It should be called `name` but it must be
	// public so that the `encoding/gob` package can encode it properly.
	Xname string

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// Name returns the unique name this resource has. It is only unique within its
// own kind.
func (obj *Named) Name() string {
	return obj.Xname
}

// SetName sets the unique name for this resource. It must only be unique within
// its own kind.
func (obj *Named) SetName(name string) {
	obj.Xname = name
}
