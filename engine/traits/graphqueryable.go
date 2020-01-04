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
	"github.com/purpleidea/mgmt/engine"
)

// GraphQueryable contains a general implementation with some of the properties
// and methods needed to implement the graph query permission for resources.
type GraphQueryable struct {
	// TODO: we could add more fine-grained permission logic here
	//allow bool
	//allowedResourceKinds []string

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// GraphQueryAllowed returns nil if you're allowed to query the graph. This
// function accepts information about the requesting resource so we can
// determine the access with some form of fine-grained control.
func (obj *GraphQueryable) GraphQueryAllowed(opts ...engine.GraphQueryableOption) error {
	options := &engine.GraphQueryableOptions{ // default options
		//kind: "",
		//name: "",
		// TODO: add more options if needed
	}
	options.Apply(opts...) // apply the options

	// By default if you just add this trait, it does the "all allow" so
	// that you don't need to implement this function, but if you want to,
	// you can add it and implement your own auth.
	return nil
}
