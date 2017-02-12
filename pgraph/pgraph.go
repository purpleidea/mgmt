// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
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

// Package pgraph represents the internal "pointer graph" that we use.
package pgraph

import (
	"fmt"
	"sort"
	"sync"

	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/resources"

	errwrap "github.com/pkg/errors"
)

//go:generate stringer -type=graphState -output=graphstate_stringer.go
type graphState int

const (
	graphStateNil graphState = iota
	graphStateStarting
	graphStateStarted
	graphStatePausing
	graphStatePaused
)

// Flags contains specific constants used by the graph.
type Flags struct {
	Debug bool
}

// Graph is the graph structure in this library.
// The graph abstract data type (ADT) is defined as follows:
// * the directed graph arrows point from left to right ( -> )
// * the arrows point away from their dependencies (eg: arrows mean "before")
// * IOW, you might see package -> file -> service (where package runs first)
// * This is also the direction that the notify should happen in...
type Graph struct {
	Name      string
	Adjacency map[*Vertex]map[*Vertex]*Edge // *Vertex -> *Vertex (edge)
	Flags     Flags
	state     graphState
	mutex     *sync.Mutex // used when modifying graph State variable
	wg        *sync.WaitGroup
}

// Vertex is the primary vertex struct in this library.
type Vertex struct {
	resources.Res       // anonymous field
	timestamp     int64 // last updated timestamp ?
}

// Edge is the primary edge struct in this library.
type Edge struct {
	Name   string
	Notify bool // should we send a refresh notification along this edge?

	refresh bool // is there a notify pending for the dest vertex ?
}

// NewGraph builds a new graph.
func NewGraph(name string) *Graph {
	return &Graph{
		Name:      name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge),
		state:     graphStateNil,
		// ptr b/c: Mutex/WaitGroup must not be copied after first use
		mutex: &sync.Mutex{},
		wg:    &sync.WaitGroup{},
	}
}

// NewVertex returns a new graph vertex struct with a contained resource.
func NewVertex(r resources.Res) *Vertex {
	return &Vertex{
		Res: r,
	}
}

// NewEdge returns a new graph edge struct.
func NewEdge(name string) *Edge {
	return &Edge{
		Name: name,
	}
}

// Refresh returns the pending refresh status of this edge.
func (obj *Edge) Refresh() bool {
	return obj.refresh
}

// SetRefresh sets the pending refresh status of this edge.
func (obj *Edge) SetRefresh(b bool) {
	obj.refresh = b
}

// Copy makes a copy of the graph struct
func (g *Graph) Copy() *Graph {
	newGraph := &Graph{
		Name:      g.Name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge, len(g.Adjacency)),
		Flags:     g.Flags,
		state:     g.state,
		mutex:     g.mutex,
		wg:        g.wg,
	}
	for k, v := range g.Adjacency {
		newGraph.Adjacency[k] = v // copy
	}
	return newGraph
}

// GetName returns the name of the graph.
func (g *Graph) GetName() string {
	return g.Name
}

// SetName sets the name of the graph.
func (g *Graph) SetName(name string) {
	g.Name = name
}

// getState returns the state of the graph. This state is used for optimizing
// certain algorithms by knowing what part of processing the graph is currently
// undergoing.
func (g *Graph) getState() graphState {
	//g.mutex.Lock()
	//defer g.mutex.Unlock()
	return g.state
}

// setState sets the graph state and returns the previous state.
func (g *Graph) setState(state graphState) graphState {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	prev := g.getState()
	g.state = state
	return prev
}

// AddVertex uses variadic input to add all listed vertices to the graph
func (g *Graph) AddVertex(xv ...*Vertex) {
	for _, v := range xv {
		if _, exists := g.Adjacency[v]; !exists {
			g.Adjacency[v] = make(map[*Vertex]*Edge)
		}
	}
}

// DeleteVertex deletes a particular vertex from the graph.
func (g *Graph) DeleteVertex(v *Vertex) {
	delete(g.Adjacency, v)
	for k := range g.Adjacency {
		delete(g.Adjacency[k], v)
	}
}

// AddEdge adds a directed edge to the graph from v1 to v2.
func (g *Graph) AddEdge(v1, v2 *Vertex, e *Edge) {
	// NOTE: this doesn't allow more than one edge between two vertexes...
	g.AddVertex(v1, v2) // supports adding N vertices now
	// TODO: check if an edge exists to avoid overwriting it!
	// NOTE: VertexMerge() depends on overwriting it at the moment...
	g.Adjacency[v1][v2] = e
}

// DeleteEdge deletes a particular edge from the graph.
// FIXME: add test cases
func (g *Graph) DeleteEdge(e *Edge) {
	for v1 := range g.Adjacency {
		for v2, edge := range g.Adjacency[v1] {
			if e == edge {
				delete(g.Adjacency[v1], v2)
			}
		}
	}
}

// CompareMatch searches for an equivalent resource in the graph and returns the
// vertex it is found in, or nil if not found.
func (g *Graph) CompareMatch(obj resources.Res) *Vertex {
	for v := range g.Adjacency {
		if v.Res.Compare(obj) {
			return v
		}
	}
	return nil
}

// TODO: consider adding a transmogrify API.
//func (g *Graph) MogrifyMatch(obj resources.Res) *Vertex {
//	for v := range g.Adjacency {
//		if err := v.Res.Mogrify(obj); err == nil {
//			// transmogrified!
//			return v
//		}
//	}
//	return nil
//}

// HasVertex returns if the input vertex exists in the graph.
func (g *Graph) HasVertex(v *Vertex) bool {
	if _, exists := g.Adjacency[v]; exists {
		return true
	}
	return false
}

// NumVertices returns the number of vertices in the graph.
func (g *Graph) NumVertices() int {
	return len(g.Adjacency)
}

// NumEdges returns the number of edges in the graph.
func (g *Graph) NumEdges() int {
	count := 0
	for k := range g.Adjacency {
		count += len(g.Adjacency[k])
	}
	return count
}

// GetVertices returns a randomly sorted slice of all vertices in the graph
// The order is random, because the map implementation is intentionally so!
func (g *Graph) GetVertices() []*Vertex {
	var vertices []*Vertex
	for k := range g.Adjacency {
		vertices = append(vertices, k)
	}
	return vertices
}

// GetVerticesChan returns a channel of all vertices in the graph.
func (g *Graph) GetVerticesChan() chan *Vertex {
	ch := make(chan *Vertex)
	go func(ch chan *Vertex) {
		for k := range g.Adjacency {
			ch <- k
		}
		close(ch)
	}(ch)
	return ch
}

// VertexSlice is a linear list of vertices. It can be sorted.
type VertexSlice []*Vertex

func (vs VertexSlice) Len() int           { return len(vs) }
func (vs VertexSlice) Swap(i, j int)      { vs[i], vs[j] = vs[j], vs[i] }
func (vs VertexSlice) Less(i, j int) bool { return vs[i].String() < vs[j].String() }

// GetVerticesSorted returns a sorted slice of all vertices in the graph
// The order is sorted by String() to avoid the non-determinism in the map type
func (g *Graph) GetVerticesSorted() []*Vertex {
	var vertices []*Vertex
	for k := range g.Adjacency {
		vertices = append(vertices, k)
	}
	sort.Sort(VertexSlice(vertices)) // add determinism
	return vertices
}

// String makes the graph pretty print.
func (g *Graph) String() string {
	return fmt.Sprintf("Vertices(%d), Edges(%d)", g.NumVertices(), g.NumEdges())
}

// String returns the canonical form for a vertex
func (v *Vertex) String() string {
	return fmt.Sprintf("%s[%s]", v.Res.Kind(), v.Res.GetName())
}

// IncomingGraphVertices returns an array (slice) of all directed vertices to
// vertex v (??? -> v). OKTimestamp should probably use this.
func (g *Graph) IncomingGraphVertices(v *Vertex) []*Vertex {
	// TODO: we might be able to implement this differently by reversing
	// the Adjacency graph and then looping through it again...
	var s []*Vertex
	for k := range g.Adjacency { // reverse paths
		for w := range g.Adjacency[k] {
			if w == v {
				s = append(s, k)
			}
		}
	}
	return s
}

// OutgoingGraphVertices returns an array (slice) of all vertices that vertex v
// points to (v -> ???). Poke should probably use this.
func (g *Graph) OutgoingGraphVertices(v *Vertex) []*Vertex {
	var s []*Vertex
	for k := range g.Adjacency[v] { // forward paths
		s = append(s, k)
	}
	return s
}

// GraphVertices returns an array (slice) of all vertices that connect to vertex v.
// This is the union of IncomingGraphVertices and OutgoingGraphVertices.
func (g *Graph) GraphVertices(v *Vertex) []*Vertex {
	var s []*Vertex
	s = append(s, g.IncomingGraphVertices(v)...)
	s = append(s, g.OutgoingGraphVertices(v)...)
	return s
}

// IncomingGraphEdges returns all of the edges that point to vertex v (??? -> v).
func (g *Graph) IncomingGraphEdges(v *Vertex) []*Edge {
	var edges []*Edge
	for v1 := range g.Adjacency { // reverse paths
		for v2, e := range g.Adjacency[v1] {
			if v2 == v {
				edges = append(edges, e)
			}
		}
	}
	return edges
}

// OutgoingGraphEdges returns all of the edges that point from vertex v (v -> ???).
func (g *Graph) OutgoingGraphEdges(v *Vertex) []*Edge {
	var edges []*Edge
	for _, e := range g.Adjacency[v] { // forward paths
		edges = append(edges, e)
	}
	return edges
}

// GraphEdges returns an array (slice) of all edges that connect to vertex v.
// This is the union of IncomingGraphEdges and OutgoingGraphEdges.
func (g *Graph) GraphEdges(v *Vertex) []*Edge {
	var edges []*Edge
	edges = append(edges, g.IncomingGraphEdges(v)...)
	edges = append(edges, g.OutgoingGraphEdges(v)...)
	return edges
}

// DFS returns a depth first search for the graph, starting at the input vertex.
func (g *Graph) DFS(start *Vertex) []*Vertex {
	var d []*Vertex // discovered
	var s []*Vertex // stack
	if _, exists := g.Adjacency[start]; !exists {
		return nil // TODO: error
	}
	v := start
	s = append(s, v)
	for len(s) > 0 {
		v, s = s[len(s)-1], s[:len(s)-1] // s.pop()

		if !VertexContains(v, d) { // if not discovered
			d = append(d, v) // label as discovered

			for _, w := range g.GraphVertices(v) {
				s = append(s, w)
			}
		}
	}
	return d
}

// FilterGraph builds a new graph containing only vertices from the list.
func (g *Graph) FilterGraph(name string, vertices []*Vertex) *Graph {
	newgraph := NewGraph(name)
	for k1, x := range g.Adjacency {
		for k2, e := range x {
			//log.Printf("Filter: %s -> %s # %s", k1.Name, k2.Name, e.Name)
			if VertexContains(k1, vertices) || VertexContains(k2, vertices) {
				newgraph.AddEdge(k1, k2, e)
			}
		}
	}
	return newgraph
}

// GetDisconnectedGraphs returns a channel containing the N disconnected graphs
// in our main graph. We can then process each of these in parallel.
func (g *Graph) GetDisconnectedGraphs() chan *Graph {
	ch := make(chan *Graph)
	go func() {
		var start *Vertex
		var d []*Vertex // discovered
		c := g.NumVertices()
		for len(d) < c {

			// get an undiscovered vertex to start from
			for _, s := range g.GetVertices() {
				if !VertexContains(s, d) {
					start = s
				}
			}

			// dfs through the graph
			dfs := g.DFS(start)
			// filter all the collected elements into a new graph
			newgraph := g.FilterGraph(g.Name, dfs)

			// add number of elements found to found variable
			d = append(d, dfs...) // extend

			// return this new graph to the channel
			ch <- newgraph

			// if we've found all the elements, then we're done
			// otherwise loop through to continue...
		}
		close(ch)
	}()
	return ch
}

// InDegree returns the count of vertices that point to me in one big lookup map.
func (g *Graph) InDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)
	for k := range g.Adjacency {
		result[k] = 0 // initialize
	}

	for k := range g.Adjacency {
		for z := range g.Adjacency[k] {
			result[z]++
		}
	}
	return result
}

// OutDegree returns the count of vertices that point away in one big lookup map.
func (g *Graph) OutDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)

	for k := range g.Adjacency {
		result[k] = 0 // initialize
		for range g.Adjacency[k] {
			result[k]++
		}
	}
	return result
}

// TopologicalSort returns the sort of graph vertices in that order.
// based on descriptions and code from wikipedia and rosetta code
// TODO: add memoization, and cache invalidation to speed this up :)
func (g *Graph) TopologicalSort() ([]*Vertex, error) { // kahn's algorithm
	var L []*Vertex                    // empty list that will contain the sorted elements
	var S []*Vertex                    // set of all nodes with no incoming edges
	remaining := make(map[*Vertex]int) // amount of edges remaining

	for v, d := range g.InDegree() {
		if d == 0 {
			// accumulate set of all nodes with no incoming edges
			S = append(S, v)
		} else {
			// initialize remaining edge count from indegree
			remaining[v] = d
		}
	}

	for len(S) > 0 {
		last := len(S) - 1 // remove a node v from S
		v := S[last]
		S = S[:last]
		L = append(L, v) // add v to tail of L
		for n := range g.Adjacency[v] {
			// for each node n remaining in the graph, consume from
			// remaining, so for remaining[n] > 0
			if remaining[n] > 0 {
				remaining[n]--         // remove edge from the graph
				if remaining[n] == 0 { // if n has no other incoming edges
					S = append(S, n) // insert n into S
				}
			}
		}
	}

	// if graph has edges, eg if any value in rem is > 0
	for c, in := range remaining {
		if in > 0 {
			for n := range g.Adjacency[c] {
				if remaining[n] > 0 {
					return nil, fmt.Errorf("Not a dag!")
				}
			}
		}
	}

	return L, nil
}

// Reachability finds the shortest path in a DAG from a to b, and returns the
// slice of vertices that matched this particular path including both a and b.
// It returns nil if a or b is nil, and returns empty list if no path is found.
// Since there could be more than one possible result for this operation, we
// arbitrarily choose one of the shortest possible. As a result, this should
// actually return a tree if we cared about correctness.
// This operates by a recursive algorithm; a more efficient version is likely.
// If you don't give this function a DAG, you might cause infinite recursion!
func (g *Graph) Reachability(a, b *Vertex) []*Vertex {
	if a == nil || b == nil {
		return nil
	}
	vertices := g.OutgoingGraphVertices(a) // what points away from a ?
	if len(vertices) == 0 {
		return []*Vertex{} // nope
	}
	if VertexContains(b, vertices) {
		return []*Vertex{a, b} // found
	}
	// TODO: parallelize this with go routines?
	var collected = make([][]*Vertex, len(vertices))
	pick := -1
	for i, v := range vertices {
		collected[i] = g.Reachability(v, b) // find b by recursion
		if l := len(collected[i]); l > 0 {
			// pick shortest path
			// TODO: technically i should return a tree
			if pick < 0 || l < len(collected[pick]) {
				pick = i
			}
		}
	}
	if pick < 0 {
		return []*Vertex{} // nope
	}
	result := []*Vertex{a} // tack on a
	result = append(result, collected[pick]...)
	return result
}

// GraphSync updates the oldGraph so that it matches the newGraph receiver. It
// leaves identical elements alone so that they don't need to be refreshed. It
// tries to transmogrify existing elements into new ones, if they support this.
// FIXME: add test cases
func (g *Graph) GraphSync(oldGraph *Graph) (*Graph, error) {

	if oldGraph == nil {
		oldGraph = NewGraph(g.GetName()) // copy over the name
	}
	oldGraph.SetName(g.GetName()) // overwrite the name

	var lookup = make(map[*Vertex]*Vertex)
	var vertexKeep []*Vertex // list of vertices which are the same in new graph
	var edgeKeep []*Edge     // list of vertices which are the same in new graph

	for v := range g.Adjacency { // loop through the vertices (resources)
		res := v.Res // resource
		var vertex *Vertex

		// step one, direct compare with res.Compare
		if vertex == nil { // redundant guard for consistency
			vertex = oldGraph.CompareMatch(res)
		}

		// TODO: consider adding a transmogrify API.
		// step two, try and mogrify with res.Mogrify
		//if vertex == nil { // not found yet...
		//	vertex = oldGraph.MogrifyMatch(res)
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
	for v := range oldGraph.Adjacency {
		if !VertexContains(v, vertexKeep) {
			// wait for exit before starting new graph!
			v.SendEvent(event.EventExit, nil) // sync
			oldGraph.DeleteVertex(v)
		}
	}

	// compare edges
	for v1 := range g.Adjacency { // loop through the vertices (resources)
		for v2, e := range g.Adjacency[v1] {
			// we have an edge!

			// lookup vertices (these should exist now)
			//res1 := v1.Res // resource
			//res2 := v2.Res
			//vertex1 := oldGraph.CompareMatch(res1)
			//vertex2 := oldGraph.CompareMatch(res2)
			vertex1, exists1 := lookup[v1]
			vertex2, exists2 := lookup[v2]
			if !exists1 || !exists2 { // no match found, bug?
				//if vertex1 == nil || vertex2 == nil { // no match found
				return nil, fmt.Errorf("New vertices weren't found!") // programming error
			}

			edge, exists := oldGraph.Adjacency[vertex1][vertex2]
			if !exists || edge.Name != e.Name { // TODO: edgeCmp
				edge = e // use or overwrite edge
			}
			oldGraph.Adjacency[vertex1][vertex2] = edge // store it (AddEdge)
			edgeKeep = append(edgeKeep, edge)           // mark as saved
		}
	}

	// delete unused edges
	for v1 := range oldGraph.Adjacency {
		for _, e := range oldGraph.Adjacency[v1] {
			// we have an edge!
			if !EdgeContains(e, edgeKeep) {
				oldGraph.DeleteEdge(e)
			}
		}
	}

	return oldGraph, nil
}

// GraphMetas returns a list of pointers to each of the resource MetaParams.
func (g *Graph) GraphMetas() []*resources.MetaParams {
	metas := []*resources.MetaParams{}
	for v := range g.Adjacency { // loop through the vertices (resources))
		res := v.Res // resource
		meta := res.Meta()
		metas = append(metas, meta)
	}
	return metas
}

// AssociateData associates some data with the object in the graph in question.
func (g *Graph) AssociateData(data *resources.Data) {
	for k := range g.Adjacency {
		k.Res.AssociateData(data)
	}
}

// VertexContains is an "in array" function to test for a vertex in a slice of vertices.
func VertexContains(needle *Vertex, haystack []*Vertex) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// EdgeContains is an "in array" function to test for an edge in a slice of edges.
func EdgeContains(needle *Edge, haystack []*Edge) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// Reverse reverses a list of vertices.
func Reverse(vs []*Vertex) []*Vertex {
	//var out []*Vertex       // XXX: golint suggests, but it fails testing
	out := make([]*Vertex, 0) // empty list
	l := len(vs)
	for i := range vs {
		out = append(out, vs[l-i-1])
	}
	return out
}
