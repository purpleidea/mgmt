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

package engine

import (
	"fmt"
)

// ReversibleRes is an interface that a resource can implement if it wants to
// have some resource run when it disappears. A disappearance happens when a
// resource is defined in one instance of the graph, and is gone in the
// subsequent one. This is helpful for building robust programs with the engine.
// Default implementations for most of the methods declared in this interface
// can be obtained for your resource by anonymously adding the traits.Reversible
// struct to your resource implementation.
type ReversibleRes interface {
	Res

	// ReversibleMeta lets you get or set meta params for the reversible
	// trait.
	ReversibleMeta() *ReversibleMeta

	// SetReversibleMeta lets you set all of the meta params for the
	// reversible trait in a single call.
	SetReversibleMeta(*ReversibleMeta)

	// Reversed returns the "reverse" or "reciprocal" resource. This is used
	// to "clean" up after a previously defined resource has been removed.
	// Interestingly, this could return the core Res interface instead of a
	// ReversibleRes, because there is no requirement that the reverse of a
	// Res be the same kind of Res, and the reverse might not be reversible!
	// However, in practice, it's nice to use some of the Reversible meta
	// params in the built value, so keep things simple and have this be a
	// reversible res. The Res itself doesn't have to implement Reversed()
	// in a meaningful way, it can just return nil and it will get ignored.
	Reversed() (ReversibleRes, error)
}

// ReversibleMeta provides some parameters specific to reversible resources.
type ReversibleMeta struct {
	// Disabled specifies that reversing should be disabled for this
	// resource.
	Disabled bool

	// Reversal specifies that the resource was built from a reversal. This
	// must be set if the resource was built by a reversal.
	Reversal bool

	// Overwrite specifies that we should overwrite any existing stored
	// reversible resource if one that is pending already exists. If this is
	// false, and a resource with the same name and kind exists, then this
	// will cause an error.
	Overwrite bool

	// TODO: add options here, including whether to reverse edges, etc...
}

// Cmp compares two ReversibleMeta structs and determines if they're equivalent.
func (obj *ReversibleMeta) Cmp(rm *ReversibleMeta) error {
	if obj.Disabled != rm.Disabled {
		return fmt.Errorf("values for Disabled are different")
	}
	if obj.Reversal != rm.Reversal { // TODO: do we want to compare these?
		return fmt.Errorf("values for Reversal are different")
	}
	if obj.Overwrite != rm.Overwrite {
		return fmt.Errorf("values for Overwrite are different")
	}
	return nil
}
