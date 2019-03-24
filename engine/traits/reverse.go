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

package traits

import (
	"github.com/purpleidea/mgmt/engine"
)

// Reversible contains a general implementation with most of the properties and
// methods needed to support reversing resources. It may be used as a starting
// point to avoid re-implementing the straightforward methods.
type Reversible struct {
	// Xmeta is the stored meta. It should be called `meta` but it must be
	// public so that the `encoding/gob` package can encode it properly.
	Xmeta *engine.ReversibleMeta

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// ReversibleMeta lets you get or set meta params for the reversing trait.
func (obj *Reversible) ReversibleMeta() *engine.ReversibleMeta {
	if obj.Xmeta == nil { // set the defaults if previously empty
		obj.Xmeta = &engine.ReversibleMeta{
			Disabled: true, // by default we're disabled
		}
	}
	return obj.Xmeta
}

// SetReversibleMeta lets you set all of the meta params for the reversing trait
// in a single call.
func (obj *Reversible) SetReversibleMeta(meta *engine.ReversibleMeta) {
	obj.Xmeta = meta
}
