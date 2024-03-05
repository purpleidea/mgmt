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

package autogroup

import (
	"fmt"

	"github.com/purpleidea/mgmt/pgraph"
)

// baseGrouper is the base type for implementing the AutoGrouper interface.
type baseGrouper struct {
	graph    *pgraph.Graph   // store a pointer to the graph
	vertices []pgraph.Vertex // cached list of vertices
	i        int
	j        int
	done     bool
}

// Name provides a friendly name for the logs to see.
func (ag *baseGrouper) Name() string {
	return "baseGrouper"
}

// Init is called only once and before using other AutoGrouper interface methods
// the name method is the only exception: call it any time without side effects!
func (ag *baseGrouper) Init(g *pgraph.Graph) error {
	if ag.graph != nil {
		return fmt.Errorf("the init method has already been called")
	}
	ag.graph = g // pointer

	// We sort deterministically, first by kind, and then by name. In
	// particular, longer kind chunks sort first. So http:ui:text should
	// appear before http:server and http:ui. This is a hack so that if we
	// are doing hierarchical automatic grouping, it gives the http:ui:text
	// a chance to get grouped into http:ui, before http:ui gets grouped
	// into http:server, because once that happens, http:ui:text will never
	// get grouped, and this won't work properly. This works, because when
	// we start comparing iteratively the list of resources, it does this
	// with a O(n^2) loop that compares the X and Y zero indexes first, and
	// and then continues along. If the "longer" resources appear first,
	// then they'll group together first. We should probably put this into
	// a new Grouper struct, but for now we might as well leave it here.
	//vertices := ag.graph.VerticesSorted() // formerly
	vertices := RHVSort(ag.graph.Vertices())

	ag.vertices = vertices // cache in deterministic order!
	ag.i = 0
	ag.j = 0
	if len(ag.vertices) == 0 { // empty graph
		ag.done = true
		return nil
	}
	return nil
}

// VertexNext is a simple iterator that loops through vertex (pair) combinations
// an intelligent algorithm would selectively offer only valid pairs of vertices
// these should satisfy logical grouping requirements for the autogroup designs!
// the desired algorithms can override, but keep this method as a base iterator!
func (ag *baseGrouper) VertexNext() (v1, v2 pgraph.Vertex, err error) {
	// this does a for v... { for w... { return v, w }} but stepwise!
	l := len(ag.vertices)
	if ag.i < l {
		v1 = ag.vertices[ag.i]
	}
	if ag.j < l {
		v2 = ag.vertices[ag.j]
	}

	// in case the vertex was deleted
	if !ag.graph.HasVertex(v1) {
		v1 = nil
	}
	if !ag.graph.HasVertex(v2) {
		v2 = nil
	}

	// two nested loops...
	if ag.j < l {
		ag.j++
	}
	if ag.j == l {
		ag.j = 0
		if ag.i < l {
			ag.i++
		}
		if ag.i == l {
			ag.done = true
		}
	}
	// TODO: is this index swap better or even valid?
	//if ag.i < l {
	//	ag.i++
	//}
	//if ag.i == l {
	//	ag.i = 0
	//	if ag.j < l {
	//		ag.j++
	//	}
	//	if ag.j == l {
	//		ag.done = true
	//	}
	//}

	return
}

// VertexCmp can be used in addition to an overridding implementation.
func (ag *baseGrouper) VertexCmp(v1, v2 pgraph.Vertex) error {
	if v1 == nil || v2 == nil {
		return fmt.Errorf("the vertex is nil")
	}
	if v1 == v2 { // skip yourself
		return fmt.Errorf("the vertices are the same")
	}

	return nil // success
}

// VertexMerge needs to be overridden to add the actual merging functionality.
func (ag *baseGrouper) VertexMerge(v1, v2 pgraph.Vertex) (v pgraph.Vertex, err error) {
	return nil, fmt.Errorf("vertexMerge needs to be overridden")
}

// EdgeMerge can be overridden, since it just simply returns the first edge.
func (ag *baseGrouper) EdgeMerge(e1, e2 pgraph.Edge) pgraph.Edge {
	return e1 // noop
}

// VertexTest processes the results of the grouping for the algorithm to know
// return an error if something went horribly wrong, and bool false to stop.
func (ag *baseGrouper) VertexTest(b bool) (bool, error) {
	// NOTE: this particular baseGrouper version doesn't track what happens
	// because since we iterate over every pair, we don't care which merge!
	if ag.done {
		return false, nil
	}
	return true, nil
}
