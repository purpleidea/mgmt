// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package pgraph

import (
	"fmt"

	errwrap "github.com/pkg/errors"
)

// GraphSync updates the Graph so that it matches the newGraph. It leaves
// identical elements alone so that they don't need to be refreshed.
// It tries to mutate existing elements into new ones, if they support this.
// This updates the Graph on success only.
// FIXME: should we do this with copies of the vertex resources?
// FIXME: add test cases
func (obj *Graph) GraphSync(newGraph *Graph, vertexCmpFn func(Vertex, Vertex) (bool, error), vertexAddFn func(Vertex) error, vertexRemoveFn func(Vertex) error) error {

	oldGraph := obj.Copy() // work on a copy of the old graph
	if oldGraph == nil {
		var err error
		oldGraph, err = NewGraph(newGraph.GetName()) // copy over the name
		if err != nil {
			return errwrap.Wrapf(err, "GraphSync failed")
		}
	}
	oldGraph.SetName(newGraph.GetName()) // overwrite the name

	var lookup = make(map[Vertex]Vertex)
	var vertexKeep []Vertex // list of vertices which are the same in new graph
	var edgeKeep []*Edge    // list of vertices which are the same in new graph

	for v := range newGraph.Adjacency() { // loop through the vertices (resources)
		var vertex Vertex
		// step one, direct compare with res.Compare
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

		if vertex == nil { // no match found yet
			if err := vertexAddFn(v); err != nil {
				return errwrap.Wrapf(err, "vertexAddFn failed")
			}
			vertex = v
			oldGraph.AddVertex(vertex) // call standalone in case not part of an edge
		}
		lookup[v] = vertex                      // used for constructing edges
		vertexKeep = append(vertexKeep, vertex) // append
	}

	// get rid of any vertices we shouldn't keep (that aren't in new graph)
	for v := range oldGraph.Adjacency() {
		if !VertexContains(v, vertexKeep) {
			if err := vertexRemoveFn(v); err != nil {
				return errwrap.Wrapf(err, "vertexRemoveFn failed")
			}
			oldGraph.DeleteVertex(v)
		}
	}

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
			if !exists || edge.Name != e.Name { // TODO: edgeCmp
				edge = e // use or overwrite edge
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
