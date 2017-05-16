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

package resources

import (
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

//go:generate stringer -type=graphState -output=graphstate_stringer.go
type graphState uint

const (
	graphStateNil graphState = iota
	graphStateStarting
	graphStateStarted
	graphStatePausing
	graphStatePaused
)

// getState returns the state of the graph. This state is used for optimizing
// certain algorithms by knowing what part of processing the graph is currently
// undergoing.
func getState(g *pgraph.Graph) graphState {
	//mutex := StateLockFromGraph(g)
	//mutex.Lock()
	//defer mutex.Unlock()
	if u, ok := g.Value("state"); ok {
		return graphState(util.Uint(u))
	}
	return graphStateNil
}

// setState sets the graph state and returns the previous state.
func setState(g *pgraph.Graph, state graphState) graphState {
	mutex := StateLockFromGraph(g)
	mutex.Lock()
	defer mutex.Unlock()
	prev := getState(g)
	g.SetValue("state", uint(state))
	return prev
}

// StateLockFromGraph returns a pointer to the state lock stored with the graph,
// otherwise it panics. If one does not exist, it will create it.
func StateLockFromGraph(g *pgraph.Graph) *sync.Mutex {
	x, exists := g.Value("mutex")
	if !exists {
		g.SetValue("mutex", &sync.Mutex{})
		x, _ = g.Value("mutex")
	}

	m, ok := x.(*sync.Mutex)
	if !ok {
		panic("not a *sync.Mutex")
	}
	return m
}

// VtoR casts the Vertex into a Res for use. It panics if it can't convert.
func VtoR(v pgraph.Vertex) Res {
	res, ok := v.(Res)
	if !ok {
		panic("not a Res")
	}
	return res
}

// GraphSync updates the oldGraph so that it matches the newGraph receiver. It
// leaves identical elements alone so that they don't need to be refreshed. It
// tries to mutate existing elements into new ones, if they support this.
// FIXME: add test cases
func GraphSync(g *pgraph.Graph, oldGraph *pgraph.Graph) (*pgraph.Graph, error) {

	if oldGraph == nil {
		var err error
		oldGraph, err = pgraph.NewGraph(g.GetName()) // copy over the name
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not run GraphSync() properly")
		}
	}
	oldGraph.SetName(g.GetName()) // overwrite the name

	var lookup = make(map[pgraph.Vertex]pgraph.Vertex)
	var vertexKeep []pgraph.Vertex // list of vertices which are the same in new graph
	var edgeKeep []*pgraph.Edge    // list of vertices which are the same in new graph

	for v := range g.Adjacency() { // loop through the vertices (resources)
		res := VtoR(v) // resource
		var vertex pgraph.Vertex

		// step one, direct compare with res.Compare
		if vertex == nil { // redundant guard for consistency
			fn := func(v pgraph.Vertex) (bool, error) {
				return VtoR(v).Compare(res), nil
			}
			var err error
			vertex, err = oldGraph.VertexMatchFn(fn)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not VertexMatchFn() resource")
			}
		}

		// TODO: consider adding a mutate API.
		// step two, try and mutate with res.Mutate
		//if vertex == nil { // not found yet...
		//	vertex = oldGraph.MutateMatch(res)
		//}

		if vertex == nil { // no match found yet
			if err := res.Validate(); err != nil {
				return nil, errwrap.Wrapf(err, "could not Validate() resource")
			}
			vertex = v
			oldGraph.AddVertex(vertex) // call standalone in case not part of an edge
		}
		lookup[v] = vertex                      // used for constructing edges
		vertexKeep = append(vertexKeep, vertex) // append
	}

	// get rid of any vertices we shouldn't keep (that aren't in new graph)
	for v := range oldGraph.Adjacency() {
		if !pgraph.VertexContains(v, vertexKeep) {
			// wait for exit before starting new graph!
			VtoR(v).SendEvent(event.EventExit, nil) // sync
			VtoR(v).WaitGroup().Wait()
			oldGraph.DeleteVertex(v)
		}
	}

	// compare edges
	for v1 := range g.Adjacency() { // loop through the vertices (resources)
		for v2, e := range g.Adjacency()[v1] {
			// we have an edge!

			// lookup vertices (these should exist now)
			//res1 := v1.Res // resource
			//res2 := v2.Res
			//vertex1 := oldGraph.CompareMatch(res1) // now: VertexMatchFn
			//vertex2 := oldGraph.CompareMatch(res2) // now: VertexMatchFn
			vertex1, exists1 := lookup[v1]
			vertex2, exists2 := lookup[v2]
			if !exists1 || !exists2 { // no match found, bug?
				//if vertex1 == nil || vertex2 == nil { // no match found
				return nil, fmt.Errorf("new vertices weren't found") // programming error
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
			if !pgraph.EdgeContains(e, edgeKeep) {
				oldGraph.DeleteEdge(e)
			}
		}
	}

	return oldGraph, nil
}
