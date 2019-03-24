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

package pgraph

import (
	"fmt"

	"github.com/purpleidea/mgmt/util/errwrap"
)

func strVertexCmpFn(v1, v2 Vertex) (bool, error) {
	if v1.String() == "" || v2.String() == "" {
		return false, fmt.Errorf("empty vertex")
	}
	return v1.String() == v2.String(), nil
}

func strEdgeCmpFn(e1, e2 Edge) (bool, error) {
	if e1.String() == "" || e2.String() == "" {
		return false, fmt.Errorf("empty edge")
	}
	return e1.String() == e2.String(), nil
}

// GraphSync updates the Graph so that it matches the newGraph. It leaves
// identical elements alone so that they don't need to be refreshed.
// It tries to mutate existing elements into new ones, if they support this.
// This updates the Graph on success only. If it fails, then the graph won't
// have been modified.
// FIXME: should we do this with copies of the vertex resources?
func (obj *Graph) GraphSync(newGraph *Graph, vertexCmpFn func(Vertex, Vertex) (bool, error), vertexAddFn func(Vertex) error, vertexRemoveFn func(Vertex) error, edgeCmpFn func(Edge, Edge) (bool, error)) error {
	oldGraph := obj.Copy() // work on a copy of the old graph
	if oldGraph == nil {
		var err error
		oldGraph, err = NewGraph(newGraph.GetName()) // copy over the name
		if err != nil {
			return errwrap.Wrapf(err, "GraphSync failed")
		}
	}
	oldGraph.SetName(newGraph.GetName()) // overwrite the name

	if vertexCmpFn == nil {
		vertexCmpFn = strVertexCmpFn // use simple string cmp version
	}
	if vertexAddFn == nil {
		vertexAddFn = func(Vertex) error { return nil } // noop
	}
	if vertexRemoveFn == nil {
		vertexRemoveFn = func(Vertex) error { return nil } // noop
	}
	if edgeCmpFn == nil {
		edgeCmpFn = strEdgeCmpFn // use simple string cmp version
	}

	var lookup = make(map[Vertex]Vertex)
	var vertexKeep []Vertex // list of vertices which are the same in new graph
	var vertexDels []Vertex // list of vertices which are to be removed
	var vertexAdds []Vertex // list of vertices which are to be added
	var edgeKeep []Edge     // list of edges which are the same in new graph

	// XXX: run this as a topological sort or reverse topological sort?
	for v := range newGraph.Adjacency() { // loop through the vertices (resources)
		var vertex Vertex
		// step one, direct compare with res.Cmp
		if vertex == nil { // redundant guard for consistency
			fn := func(vv Vertex) (bool, error) {
				b, err := vertexCmpFn(vv, v)
				return b, errwrap.Wrapf(err, "vertexCmpFn failed")
			}
			var err error
			vertex, err = oldGraph.VertexMatchFn(fn)
			if err != nil {
				return errwrap.Wrapf(err, "VertexMatchFn failed")
			}
		}

		// TODO: consider adding a mutate API.
		// step two, try and mutate with res.Mutate
		//if vertex == nil { // not found yet...
		//	vertex = oldGraph.MutateMatch(res)
		//}

		// run the removes BEFORE the adds, so don't do the add here...
		if vertex == nil { // no match found yet
			vertexAdds = append(vertexAdds, v) // append
			vertex = v
		}
		lookup[v] = vertex                      // used for constructing edges
		vertexKeep = append(vertexKeep, vertex) // append
	}
	// get rid of any vertices we shouldn't keep (that aren't in new graph)
	for v := range oldGraph.Adjacency() {
		if !VertexContains(v, vertexKeep) {
			vertexDels = append(vertexDels, v) // append
		}
	}

	// see if any of the add/remove functions actually fail first
	// XXX: run this as a reverse topological sort or topological sort?
	for _, vertex := range vertexDels {
		if err := vertexRemoveFn(vertex); err != nil {
			return errwrap.Wrapf(err, "vertexRemoveFn failed")
		}
	}
	for _, vertex := range vertexAdds {
		if err := vertexAddFn(vertex); err != nil {
			return errwrap.Wrapf(err, "vertexAddFn failed")
		}
	}

	// no add/remove functions failed, so we can actually modify the graph!
	for _, vertex := range vertexDels {
		oldGraph.DeleteVertex(vertex)
	}
	for _, vertex := range vertexAdds {
		oldGraph.AddVertex(vertex) // call standalone in case not part of an edge
	}

	// XXX: fixup this part so the CmpFn stuff fails early, and THEN we edit
	// the graph at the end, if no errors happened...
	// compare edges
	for v1 := range newGraph.Adjacency() { // loop through the vertices (resources)
		for v2, e := range newGraph.Adjacency()[v1] {
			// we have an edge!
			// lookup vertices (these should exist now)
			vertex1, exists1 := lookup[v1]
			vertex2, exists2 := lookup[v2]
			if !exists1 || !exists2 { // no match found, bug?
				//if vertex1 == nil || vertex2 == nil { // no match found
				return fmt.Errorf("new vertices weren't found") // programming error
			}

			edge, exists := oldGraph.Adjacency()[vertex1][vertex2]
			if !exists {
				edge = e // use edge
			} else if b, err := edgeCmpFn(edge, e); err != nil {
				return errwrap.Wrapf(err, "edgeCmpFn failed")
			} else if !b {
				edge = e // overwrite edge
			}

			oldGraph.Adjacency()[vertex1][vertex2] = edge // store it (AddEdge)
			edgeKeep = append(edgeKeep, edge)             // mark as saved
		}
	}

	// delete unused edges
	for v1 := range oldGraph.Adjacency() {
		for _, e := range oldGraph.Adjacency()[v1] {
			// we have an edge!
			if !EdgeContains(e, edgeKeep) {
				oldGraph.DeleteEdge(e)
			}
		}
	}

	// success
	*obj = *oldGraph // save old graph
	return nil
}
