// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

	"github.com/purpleidea/mgmt/pgraph"
)

// GroupableRes is the interface a resource must implement to support automatic
// grouping. Default implementations for most of the methods declared in this
// interface can be obtained for your resource by anonymously adding the
// traits.Groupable struct to your resource implementation.
type GroupableRes interface {
	Res // implement everything in Res but add the additional requirements

	// AutoGroupMeta lets you get or set meta params for the automatic
	// grouping trait.
	AutoGroupMeta() *AutoGroupMeta

	// SetAutoGroupMeta lets you set all of the meta params for the
	// automatic grouping trait in a single call.
	SetAutoGroupMeta(*AutoGroupMeta)

	// GroupCmp compares two resources and decides if they're suitable for
	//grouping. This usually needs to be unique to your resource.
	GroupCmp(res GroupableRes) error

	// GroupRes groups resource argument (res) into self.
	GroupRes(res GroupableRes) error

	// IsGrouped determines if we are grouped.
	IsGrouped() bool // am I grouped?

	// SetGrouped sets a flag to tell if we are grouped.
	SetGrouped(bool)

	// GetGroup returns everyone grouped inside me.
	GetGroup() []GroupableRes // return everyone grouped inside me

	// SetGroup sets the grouped resources into me.
	SetGroup([]GroupableRes)
}

// AutoGroupMeta provides some parameters specific to automatic grouping.
// TODO: currently this only supports disabling the feature per-resource, but in
// the future you could conceivably have some small pattern to control it better
type AutoGroupMeta struct {
	// Disabled specifies that automatic grouping should be disabled for
	// this resource.
	Disabled bool
}

// Cmp compares two AutoGroupMeta structs and determines if they're equivalent.
func (obj *AutoGroupMeta) Cmp(agm *AutoGroupMeta) error {
	if obj.Disabled != agm.Disabled {
		return fmt.Errorf("values for Disabled are different")
	}
	return nil
}

// AutoGrouper is the required interface to implement an autogrouping algorithm.
type AutoGrouper interface {
	// listed in the order these are typically called in...
	Name() string                                                    // friendly identifier
	Init(*pgraph.Graph) error                                        // only call once
	VertexNext() (pgraph.Vertex, pgraph.Vertex, error)               // mostly algorithmic
	VertexCmp(pgraph.Vertex, pgraph.Vertex) error                    // can we merge these ?
	VertexMerge(pgraph.Vertex, pgraph.Vertex) (pgraph.Vertex, error) // vertex merge fn to use
	EdgeMerge(pgraph.Edge, pgraph.Edge) pgraph.Edge                  // edge merge fn to use
	VertexTest(bool) (bool, error)                                   // call until false
}
