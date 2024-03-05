// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package engine

import (
	"fmt"
)

// Edge is a struct that represents a graph's edge.
type Edge struct {
	Name   string
	Notify bool // should we send a refresh notification along this edge?

	refresh bool // is there a notify pending for the dest vertex ?
}

// String is a required method of the Edge interface that we must fulfill.
func (obj *Edge) String() string {
	return obj.Name
}

// Cmp compares this edge to another. It returns nil if they are equivalent.
func (obj *Edge) Cmp(edge *Edge) error {
	if obj.Name != edge.Name {
		return fmt.Errorf("edge names differ")
	}
	if obj.Notify != edge.Notify {
		return fmt.Errorf("notify values differ")
	}
	// FIXME: should we compare this as well?
	//if obj.refresh != edge.refresh {
	//	return fmt.Errorf("refresh values differ")
	//}
	return nil
}

// Refresh returns the pending refresh status of this edge.
func (obj *Edge) Refresh() bool {
	return obj.refresh
}

// SetRefresh sets the pending refresh status of this edge.
func (obj *Edge) SetRefresh(b bool) {
	obj.refresh = b
}
