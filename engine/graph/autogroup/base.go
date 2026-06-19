// Mgmt
// Copyright (C) James Shubin and the project contributors
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
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
)

// baseGrouper is the base type for implementing the AutoGrouper interface.
type baseGrouper struct {
	graph    *pgraph.Graph   // store a pointer to the graph
	vertices []pgraph.Vertex // cached list of vertices
	chunks   []string        // first kind chunk per vertex ("" matches all)
	i        int
	j        int
	done     bool
}

// Name provides a friendly name for the logs to see.
func (obj *baseGrouper) Name() string {
	return "baseGrouper"
}

// Init is called only once and before using other AutoGrouper interface methods
// the name method is the only exception: call it any time without side effects!
func (obj *baseGrouper) Init(g *pgraph.Graph) error {
	if obj.graph != nil {
		return fmt.Errorf("the init method has already been called")
	}
	obj.graph = g // pointer

	// We sort deterministically, first by kind, and then by name. In
	// particular, longer kind chunks sort first. So http:server:ui:input
	// should appear before http:server and http:server:ui. This is a
	// strategy so that if we are doing hierarchical automatic grouping, it
	// gives the http:server:ui:input a chance to get grouped into
	// http:server:ui, before http:server:ui gets grouped into http:server,
	// because once that happens, http:server:ui:input will never get
	// grouped, and this won't work properly. This works, because when we
	// start comparing iteratively the list of resources, it does this with
	// a O(n^2) loop that compares the X and Y zero indexes first, and then
	// continues along. If the "longer" resources appear first, then they'll
	// group together first. We should probably put this into a new Grouper
	// struct, but for now we might as well leave it here.
	//vertices := obj.graph.VerticesSorted() // formerly
	vertices := RHVSort(obj.graph.Vertices())

	obj.vertices = vertices // cache in deterministic order!

	// Cache the first colon-separated chunk of each resource kind. Pairs
	// with two different non-empty chunks can never group (see the
	// GroupCmp docs in the engine package) so the iterator skips them.
	obj.chunks = make([]string, len(vertices))
	for i, v := range vertices {
		res, ok := v.(engine.Res)
		if !ok || res.Kind() == "" {
			continue // empty chunk matches everything
		}
		// consistent specific index as we may skip gaps if we continue!
		obj.chunks[i] = strings.SplitN(res.Kind(), ":", 2)[0]
	}

	obj.i = 0
	obj.j = 0
	if len(obj.vertices) == 0 { // empty graph
		obj.done = true
		return nil
	}
	return nil
}

// VertexNext is a simple iterator that loops through vertex (pair) combinations
// an intelligent algorithm would selectively offer only valid pairs of vertices
// these should satisfy logical grouping requirements for the autogroup designs!
// the desired algorithms can override, but keep this method as a base iterator!
func (obj *baseGrouper) VertexNext() (v1, v2 pgraph.Vertex, err error) {
	// this does a for v... { for w... { return v, w }} but stepwise!
	// fast-forward over pairs whose kinds could never group together, so
	// large graphs full of ungroupable resources don't pay the full cost
	for !obj.done {
		c1, c2 := obj.chunks[obj.i], obj.chunks[obj.j]
		if c1 == c2 || c1 == "" || c2 == "" {
			break // a candidate pair
		}
		obj.advance()
	}

	l := len(obj.vertices)
	if obj.i < l {
		v1 = obj.vertices[obj.i]
	}
	if obj.j < l {
		v2 = obj.vertices[obj.j]
	}

	// in case the vertex was deleted
	if !obj.graph.HasVertex(v1) {
		v1 = nil
	}
	if !obj.graph.HasVertex(v2) {
		v2 = nil
	}

	obj.advance()
	return
}

// advance moves the iterator indexes to the next pair in the deterministic
// iteration order, marking the iterator as done when it runs off the end.
func (obj *baseGrouper) advance() {
	l := len(obj.vertices)
	// two nested loops...
	if obj.j < l {
		obj.j++
	}
	if obj.j == l {
		obj.j = 0
		if obj.i < l {
			obj.i++
		}
		if obj.i == l {
			obj.done = true
		}
	}
	// TODO: is this index swap better or even valid?
	//if obj.i < l {
	//	obj.i++
	//}
	//if obj.i == l {
	//	obj.i = 0
	//	if obj.j < l {
	//		obj.j++
	//	}
	//	if obj.j == l {
	//		obj.done = true
	//	}
	//}
}

// VertexCmp can be used in addition to an overriding implementation.
func (obj *baseGrouper) VertexCmp(v1, v2 pgraph.Vertex) error {
	if v1 == nil || v2 == nil {
		return fmt.Errorf("the vertex is nil")
	}
	if v1 == v2 { // skip yourself
		return fmt.Errorf("the vertices are the same")
	}

	return nil // success
}

// VertexViable returns whether the graph would still make sense if these two
// vertices were merged. This base version always says yes; algorithms should
// override it with their structural check. It is split out from VertexCmp so
// that the cheap resource comparison can run first, and this potentially more
// expensive graph traversal only runs for pairs which actually want to merge.
func (obj *baseGrouper) VertexViable(v1, v2 pgraph.Vertex) error {
	if v1 == nil || v2 == nil {
		return fmt.Errorf("the vertex is nil")
	}

	return nil // viable
}

// VertexMerge needs to be overridden to add the actual merging functionality.
func (obj *baseGrouper) VertexMerge(v1, v2 pgraph.Vertex) (v pgraph.Vertex, err error) {
	return nil, fmt.Errorf("vertexMerge needs to be overridden")
}

// EdgeMerge can be overridden, since it just simply returns the first edge.
func (obj *baseGrouper) EdgeMerge(e1, e2 pgraph.Edge) pgraph.Edge {
	return e1 // noop
}

// VertexTest processes the results of the grouping for the algorithm to know
// return an error if something went horribly wrong, and bool false to stop.
func (obj *baseGrouper) VertexTest(b bool) (bool, error) {
	// NOTE: this particular baseGrouper version doesn't track what happens
	// because since we iterate over every pair, we don't care which merge!
	if obj.done {
		return false, nil
	}
	return true, nil
}
