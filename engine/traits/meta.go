// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"github.com/purpleidea/mgmt/engine"
)

// Meta contains a general implementation of the properties and methods needed
// to support meta parameters. It should be used as a starting point to avoid
// re-implementing the straightforward meta methods.
type Meta struct {
	meta *engine.MetaParams

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// MetaParams lets you get or set meta params for this trait.
func (obj *Meta) MetaParams() *engine.MetaParams {
	if obj.meta == nil { // set the defaults if previously empty
		obj.meta = engine.DefaultMetaParams.Copy()
	}
	return obj.meta
}
