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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
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

package graph

import (
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
)

// RefreshPending determines if any previous nodes have a refresh pending here.
// If this is true, it means I am expected to apply a refresh when I next run.
func (obj *Engine) RefreshPending(vertex pgraph.Vertex) bool {
	var refresh bool
	for _, e := range obj.graph.IncomingGraphEdges(vertex) {
		// if we asked for a notify *and* if one is pending!
		edge := e.(*engine.Edge) // panic if wrong
		if edge.Notify && edge.Refresh() {
			refresh = true
			break
		}
	}
	return refresh
}

// SetUpstreamRefresh sets the refresh value to any upstream vertices.
func (obj *Engine) SetUpstreamRefresh(vertex pgraph.Vertex, b bool) {
	for _, e := range obj.graph.IncomingGraphEdges(vertex) {
		edge := e.(*engine.Edge) // panic if wrong
		if edge.Notify {
			edge.SetRefresh(b)
		}
	}
}

// SetDownstreamRefresh sets the refresh value to any downstream vertices.
func (obj *Engine) SetDownstreamRefresh(vertex pgraph.Vertex, b bool) {
	for _, e := range obj.graph.OutgoingGraphEdges(vertex) {
		edge := e.(*engine.Edge) // panic if wrong
		// if we asked for a notify *and* if one is pending!
		if edge.Notify {
			edge.SetRefresh(b)
		}
	}
}
