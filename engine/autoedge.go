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

package engine

import (
	"fmt"
)

// EdgeableRes is the interface a resource must implement to support automatic
// edges. Both the vertices involved in an edge need to implement this for it to
// be able to work.
type EdgeableRes interface {
	Res // implement everything in Res but add the additional requirements

	// AutoEdgeMeta lets you get or set meta params for the automatic edges
	// trait.
	AutoEdgeMeta() *AutoEdgeMeta

	// SetAutoEdgeMeta lets you set all of the meta params for the automatic
	// edges trait in a single call.
	SetAutoEdgeMeta(*AutoEdgeMeta)

	// UIDs includes all params to make a unique identification of this
	// object.
	UIDs() []ResUID // most resources only return one

	// AutoEdges returns a struct that implements the AutoEdge interface.
	// This interface can be used to generate automatic edges to other
	// resources.
	AutoEdges() (AutoEdge, error)
}

// AutoEdgeMeta provides some parameters specific to automatic edges.
// TODO: currently this only supports disabling the feature per-resource, but in
// the future you could conceivably have some small pattern to control it better
type AutoEdgeMeta struct {
	// Disabled specifies that automatic edges should be disabled for this
	// resource.
	Disabled bool
}

// Cmp compares two AutoEdgeMeta structs and determines if they're equivalent.
func (obj *AutoEdgeMeta) Cmp(aem *AutoEdgeMeta) error {
	if obj.Disabled != aem.Disabled {
		return fmt.Errorf("values for Disabled are different")
	}
	return nil
}

// The AutoEdge interface is used to implement the autoedges feature.
type AutoEdge interface {
	Next() []ResUID   // call to get list of edges to add
	Test([]bool) bool // call until false
}

// ResUID is a unique identifier for a resource, namely it's name, and the kind ("type").
type ResUID interface {
	fmt.Stringer // String() string

	GetName() string
	GetKind() string

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
		panic("programming error!")
	}
	return *obj.Reversed
}
