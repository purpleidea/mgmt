// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

package resources

import (
	"fmt"
	"log"
)

// ResUID is a unique identifier for a resource, namely it's name, and the kind ("type").
type ResUID interface {
	GetName() string
	GetKind() string
	fmt.Stringer // String() string

	IFF(ResUID) bool

	IsReversed() bool // true means this resource happens before the generator
}

// The BaseUID struct is used to provide a unique resource identifier.
type BaseUID struct {
	Name string // name and kind are the values of where this is coming from
	Kind string

	Reversed *bool // piggyback edge information here
}

// GetName returns the name of the resource UID.
func (obj *BaseUID) GetName() string {
	return obj.Name
}

// GetKind returns the kind of the resource UID.
func (obj *BaseUID) GetKind() string {
	return obj.Kind
}

// String returns the canonical string representation for a resource UID.
func (obj *BaseUID) String() string {
	return fmt.Sprintf("%s[%s]", obj.GetKind(), obj.GetName())
}

// IFF looks at two UID's and if and only if they are equivalent, returns true.
// If they are not equivalent, it returns false.
// Most resources will want to override this method, since it does the important
// work of actually discerning if two resources are identical in function.
func (obj *BaseUID) IFF(uid ResUID) bool {
	res, ok := uid.(*BaseUID)
	if !ok {
		return false
	}
	return obj.Name == res.Name
}

// IsReversed is part of the ResUID interface, and true means this resource
// happens before the generator.
func (obj *BaseUID) IsReversed() bool {
	if obj.Reversed == nil {
		log.Fatal("Programming error!")
	}
	return *obj.Reversed
}
