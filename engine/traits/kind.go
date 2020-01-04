// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

import (
	"encoding/gob"
)

func init() {
	gob.Register(&Kinded{})
}

// Kinded contains a general implementation of the properties and methods needed
// to support the resource kind. It should be used as a starting point to avoid
// re-implementing the straightforward kind methods.
type Kinded struct {
	// Xkind is the stored kind. It should be called `kind` but it must be
	// public so that the `encoding/gob` package can encode it properly.
	Xkind string

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// Kind returns the string representation for the kind this resource is.
func (obj *Kinded) Kind() string {
	return obj.Xkind
}

// SetKind sets the kind string for this resource. It must only be set by the
// engine.
func (obj *Kinded) SetKind(kind string) {
	obj.Xkind = kind
}
