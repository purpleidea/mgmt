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

// Graph is the graph structure in this library.
// The graph abstract data type (ADT) is defined as follows:
// * the directed graph arrows point from left to right ( -> )
// * the arrows point away from their dependencies (eg: arrows mean "before")
// * IOW, you might see package -> file -> service (where package runs first)
// * This is also the direction that the notify should happen in...
type Graph struct {
	Name string

	adjacency map[*Vertex]map[*Vertex]*Edge // *Vertex -> *Vertex (edge)
	kv        map[string]interface{}        // some values associated with the graph

	// legacy
	state     graphState
	fastPause bool        // used to disable pokes for a fast pause
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

// Init initializes the graph which populates all the internal structures.
func (g *Graph) Init() error {
	if g.Name == "" {
		return fmt.Errorf("can't initialize graph with empty name")
	}

	g.adjacency = make(map[*Vertex]map[*Vertex]*Edge)
	g.kv = make(map[string]interface{})

	// legacy
	g.state = graphStateNil
	// ptr b/c: Mutex/WaitGroup must not be copied after first use
	g.mutex = &sync.Mutex{}
	g.wg = &sync.WaitGroup{}
	return nil
}

// NewGraph builds a new graph.
func NewGraph(name string) (*Graph, error) {
	g := &Graph{
		Name: name,
	}
	if err := g.Init(); err != nil {
		return nil, err
	}
	return g, nil
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

// Value returns a value stored alongside the graph in a particular key.
func (g *Graph) Value(key string) (interface{}, bool) {
	val, exists := g.kv[key]
	return val, exists
}

// SetValue sets a value to be stored alongside the graph in a particular key.
func (g *Graph) SetValue(key string, val interface{}) {
	g.kv[key] = val
}

// Copy makes a copy of the graph struct
func (g *Graph) Copy() *Graph {
	newGraph := &Graph{
		Name:      g.Name,
		adjacency: make(map[*Vertex]map[*Vertex]*Edge, len(g.adjacency)),
		kv:        g.kv,

		// legacy
		state:     g.state,
		mutex:     g.mutex,
		wg:        g.wg,
		fastPause: g.fastPause,
	}
	for k, v := range g.adjacency {
		newGraph.adjacency[k] = v // copy
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
		if _, exists := g.adjacency[v]; !exists {
			g.adjacency[v] = make(map[*Vertex]*Edge)
		}
	}
}

// DeleteVertex deletes a particular vertex from the graph.
func (g *Graph) DeleteVertex(v *Vertex) {
	delete(g.adjacency, v)
	for k := range g.adjacency {
		delete(g.adjacency[k], v)
	}
}

// AddEdge adds a directed edge to the graph from v1 to v2.
func (g *Graph) AddEdge(v1, v2 *Vertex, e *Edge) {
	// NOTE: this doesn't allow more than one edge between two vertexes...
	g.AddVertex(v1, v2) // supports adding N vertices now
	// TODO: check if an edge exists to avoid overwriting it!
	// NOTE: VertexMerge() depends on overwriting it at the moment...
	g.adjacency[v1][v2] = e
}

// DeleteEdge deletes a particular edge from the graph.
// FIXME: add test cases
func (g *Graph) DeleteEdge(e *Edge) {
	for v1 := range g.adjacency {
		for v2, edge := range g.adjacency[v1] {
			if e == edge {
				delete(g.adjacency[v1], v2)
			}
		}
	}
}

// VertexMatchFn searches for a vertex in the graph and returns the vertex if
// one matches. It uses a user defined function to match. That function must
// return true on match, and an error if anything goes wrong.
func (g *Graph) VertexMatchFn(fn func(*Vertex) (bool, error)) (*Vertex, error) {
	for v := range g.adjacency {
		if b, err := fn(v); err != nil {
			return nil, errwrap.Wrapf(err, "fn in VertexMatchFn() errored")
		} else if b {
			return v, nil
		}
	}
	return nil, nil // nothing found
}

// TODO: consider adding a mutate API.
//func (g *Graph) MutateMatch(obj resources.Res) *Vertex {
//	for v := range g.adjacency {
//		if err := v.Res.Mutate(obj); err == nil {
//			// transmogrified!
//			return v
//		}
//	}
//	return nil
//}

// HasVertex returns if the input vertex exists in the graph.
func (g *Graph) HasVertex(v *Vertex) bool {
	if _, exists := g.adjacency[v]; exists {
		return true
	}
	return false
}

// NumVertices returns the number of vertices in the graph.
func (g *Graph) NumVertices() int {
	return len(g.adjacency)
}

// NumEdges returns the number of edges in the graph.
func (g *Graph) NumEdges() int {
	count := 0
	for k := range g.adjacency {
		count += len(g.adjacency[k])
	}
	return count
}

// Adjacency returns the adjacency map representing this graph. This is useful
// for users who which to operate on the raw data structure more efficiently.
func (g *Graph) Adjacency() map[*Vertex]map[*Vertex]*Edge {
	return g.adjacency
}

// Vertices returns a randomly sorted slice of all vertices in the graph.
// The order is random, because the map implementation is intentionally so!
func (g *Graph) Vertices() []*Vertex {
	var vertices []*Vertex
	for k := range g.adjacency {
		vertices = append(vertices, k)
	}
	return vertices
}

// VerticesChan returns a channel of all vertices in the graph.
func (g *Graph) VerticesChan() chan *Vertex {
	ch := make(chan *Vertex)
	go func(ch chan *Vertex) {
		for k := range g.adjacency {
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

// VerticesSorted returns a sorted slice of all vertices in the graph
// The order is sorted by String() to avoid the non-determinism in the map type
func (g *Graph) VerticesSorted() []*Vertex {
	var vertices []*Vertex
	for k := range g.adjacency {
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
	return fmt.Sprintf("%s[%s]", v.Res.GetKind(), v.Res.GetName())
}

// IncomingGraphVertices returns an array (slice) of all directed vertices to
// vertex v (??? -> v). OKTimestamp should probably use this.
func (g *Graph) IncomingGraphVertices(v *Vertex) []*Vertex {
	// TODO: we might be able to implement this differently by reversing
	// the Adjacency graph and then looping through it again...
	var s []*Vertex
	for k := range g.adjacency { // reverse paths
		for w := range g.adjacency[k] {
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
	for k := range g.adjacency[v] { // forward paths
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
	for v1 := range g.adjacency { // reverse paths
		for v2, e := range g.adjacency[v1] {
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
	for _, e := range g.adjacency[v] { // forward paths
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
	if _, exists := g.adjacency[start]; !exists {
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
func (g *Graph) FilterGraph(name string, vertices []*Vertex) (*Graph, error) {
	newGraph := &Graph{Name: name}
	if err := newGraph.Init(); err != nil {
		return nil, errwrap.Wrapf(err, "could not run FilterGraph() properly")
	}
	for k1, x := range g.adjacency {
		for k2, e := range x {
			//log.Printf("Filter: %s -> %s # %s", k1.Name, k2.Name, e.Name)
			if VertexContains(k1, vertices) || VertexContains(k2, vertices) {
				newGraph.AddEdge(k1, k2, e)
			}
		}
	}
	return newGraph, nil
}

// DisconnectedGraphs returns a list containing the N disconnected graphs.
func (g *Graph) DisconnectedGraphs() ([]*Graph, error) {
	graphs := []*Graph{}
	var start *Vertex
	var d []*Vertex // discovered
	c := g.NumVertices()
	for len(d) < c {

		// get an undiscovered vertex to start from
		for _, s := range g.Vertices() {
			if !VertexContains(s, d) {
				start = s
			}
		}

		// dfs through the graph
		dfs := g.DFS(start)
		// filter all the collected elements into a new graph
		newgraph, err := g.FilterGraph(g.Name, dfs)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not run DisconnectedGraphs() properly")
		}
		// add number of elements found to found variable
		d = append(d, dfs...) // extend

		// append this new graph to the list
		graphs = append(graphs, newgraph)

		// if we've found all the elements, then we're done
		// otherwise loop through to continue...
	}
	return graphs, nil
}

// InDegree returns the count of vertices that point to me in one big lookup map.
func (g *Graph) InDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)
	for k := range g.adjacency {
		result[k] = 0 // initialize
	}

	for k := range g.adjacency {
		for z := range g.adjacency[k] {
			result[z]++
		}
	}
	return result
}

// OutDegree returns the count of vertices that point away in one big lookup map.
func (g *Graph) OutDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)

	for k := range g.adjacency {
		result[k] = 0 // initialize
		for range g.adjacency[k] {
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
		for n := range g.adjacency[v] {
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
			for n := range g.adjacency[c] {
				if remaining[n] > 0 {
					return nil, fmt.Errorf("not a dag")
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
// tries to mutate existing elements into new ones, if they support this.
// FIXME: add test cases
func (g *Graph) GraphSync(oldGraph *Graph) (*Graph, error) {

	if oldGraph == nil {
		var err error
		oldGraph, err = NewGraph(g.GetName()) // copy over the name
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not run GraphSync() properly")
		}
	}
	oldGraph.SetName(g.GetName()) // overwrite the name

	var lookup = make(map[*Vertex]*Vertex)
	var vertexKeep []*Vertex // list of vertices which are the same in new graph
	var edgeKeep []*Edge     // list of vertices which are the same in new graph

	for v := range g.adjacency { // loop through the vertices (resources)
		res := v.Res // resource
		var vertex *Vertex

		// step one, direct compare with res.Compare
		if vertex == nil { // redundant guard for consistency
			fn := func(v *Vertex) (bool, error) {
				return v.Res.Compare(res), nil
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
	for v := range oldGraph.adjacency {
		if !VertexContains(v, vertexKeep) {
			// wait for exit before starting new graph!
			v.SendEvent(event.EventExit, nil) // sync
			v.Res.WaitGroup().Wait()
			oldGraph.DeleteVertex(v)
		}
	}

	// compare edges
	for v1 := range g.adjacency { // loop through the vertices (resources)
		for v2, e := range g.adjacency[v1] {
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

			edge, exists := oldGraph.adjacency[vertex1][vertex2]
			if !exists || edge.Name != e.Name { // TODO: edgeCmp
				edge = e // use or overwrite edge
			}
			oldGraph.adjacency[vertex1][vertex2] = edge // store it (AddEdge)
			edgeKeep = append(edgeKeep, edge)           // mark as saved
		}
	}

	// delete unused edges
	for v1 := range oldGraph.adjacency {
		for _, e := range oldGraph.adjacency[v1] {
			// we have an edge!
			if !EdgeContains(e, edgeKeep) {
				oldGraph.DeleteEdge(e)
			}
		}
	}

	return oldGraph, nil
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
