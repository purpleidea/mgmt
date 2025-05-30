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

// Package pgraph represents the internal "pointer graph" that we use.
package pgraph

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// ErrNotAcyclic specifies that a particular graph was not found to be a dag.
var ErrNotAcyclic = errors.New("not a dag")

// Graph is the graph structure in this library. The graph abstract data type
// (ADT) is defined as follows:
// * The directed graph arrows point from left to right. ( -> )
// * The arrows point away from their dependencies. (eg: arrows mean "before")
// * IOW, you might see package -> file -> service. (where package runs first)
// * This is also the direction that the notify should happen in...
type Graph struct {
	Name string

	adjacency map[Vertex]map[Vertex]Edge // Vertex -> Vertex (edge)
	kv        map[string]interface{}     // some values associated with the graph
}

// Vertex is the primary vertex struct in this library. It can be anything that
// implements Stringer. The string output must be stable and unique in a graph.
type Vertex interface {
	fmt.Stringer // String() string
}

// Edge is the primary edge struct in this library. It can be anything that
// implements Stringer. The string output must be stable and unique in a graph.
type Edge interface {
	fmt.Stringer // String() string
}

// Init initializes the graph which populates all the internal structures.
func (g *Graph) Init() error {
	if g.Name == "" { // FIXME: is this really a good requirement?
		return fmt.Errorf("can't initialize graph with empty name")
	}

	if g.adjacency == nil {
		g.adjacency = make(map[Vertex]map[Vertex]Edge)
	}
	//g.kv = make(map[string]interface{}) // not required
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

// Value returns a value stored alongside the graph in a particular key.
func (g *Graph) Value(key string) (interface{}, bool) {
	val, exists := g.kv[key]
	return val, exists
}

// SetValue sets a value to be stored alongside the graph in a particular key.
func (g *Graph) SetValue(key string, val interface{}) {
	if g.kv == nil { // initialize on first use
		g.kv = make(map[string]interface{})
	}
	g.kv[key] = val
}

// Copy makes a copy of the graph struct. This doesn't copy the individual
// vertices or edges, those pointers remain untouched. This lets you modify the
// structure of the graph without changing the original. If you also want to
// copy the nodes, please use CopyWithFn instead.
func (g *Graph) Copy() *Graph {
	if g == nil { // allow nil graphs through
		return g
	}
	newGraph := &Graph{
		Name:      g.Name,
		adjacency: make(map[Vertex]map[Vertex]Edge, len(g.adjacency)),
		kv:        g.kv,
	}
	for v1, m := range g.adjacency {
		newGraph.adjacency[v1] = make(map[Vertex]Edge)
		for v2, e := range m {
			newGraph.adjacency[v1][v2] = e // copy
		}
	}
	return newGraph
}

// CopyWithFn makes a copy of the graph struct but lets you provide a function
// to copy the vertices.
// TODO: add tests
func (g *Graph) CopyWithFn(vertexCpFn func(Vertex) (Vertex, error)) (*Graph, error) {
	if g == nil { // allow nil graphs through
		return g, nil
	}
	if l := len(g.adjacency); vertexCpFn == nil && l > 0 {
		return nil, fmt.Errorf("graph has %d vertices, but vertexCpFn is nil", l)
	}
	newGraph := &Graph{
		Name:      g.Name,
		adjacency: make(map[Vertex]map[Vertex]Edge, len(g.adjacency)),
		kv:        g.kv,
	}
	vm := make(map[Vertex]Vertex) // copy mapping from old ptr to new ptr...
	for v1, m := range g.adjacency {
		// We copy each vertex, but then we need to do a lookup so that
		// when (if) we see that old pointer again, we use the new one.
		v, err := vertexCpFn(v1) // copy
		if err != nil {
			return nil, err
		}
		vm[v1] = v // mapping
		newGraph.adjacency[v] = make(map[Vertex]Edge)
		for v2, e := range m {
			vx, exists := vm[v2] // copied equivalent of v2
			if !exists {
				// programming error or corrupt adjacency maps!
				// anything in the second map, should be in the
				// first one, or else it was added/modified oob
				return nil, fmt.Errorf("corrupt datastructure")
			}
			// TODO: add edgeCpFn if it's deemed useful somehow...
			//edge, err := edgeCpFn(e) // copy edge
			//if err != nil {
			//	return nil, err
			//}
			//newGraph.adjacency[v][vx] = edge
			newGraph.adjacency[v][vx] = e // store the edge
		}
	}
	return newGraph, nil
}

// VertexSwap swaps vertices in a graph. It returns a new graph with the same
// structure but with replacements done according to the translation map passed
// in. If a vertex is not found in the graph, then it is not substituted.
// TODO: add tests
func (g *Graph) VertexSwap(vs map[Vertex]Vertex) (*Graph, error) {
	vertexCpFn := func(v Vertex) (Vertex, error) {
		if vs == nil { // pass through
			return v, nil
		}
		vx, exists := vs[v]
		if !exists {
			return v, nil // pass through
		}
		return vx, nil // swap!
	}

	// We can implement the logic we want on top of CopyWithFn easily!
	return g.CopyWithFn(vertexCpFn)
}

// GetName returns the name of the graph.
func (g *Graph) GetName() string {
	return g.Name
}

// SetName sets the name of the graph.
func (g *Graph) SetName(name string) {
	g.Name = name
}

// AddVertex uses variadic input to add all listed vertices to the graph.
func (g *Graph) AddVertex(xv ...Vertex) {
	if g.adjacency == nil { // initialize on first use
		g.adjacency = make(map[Vertex]map[Vertex]Edge)
	}
	for _, v := range xv {
		if v == nil {
			panic("nil vertex")
		}
		if _, exists := g.adjacency[v]; !exists {
			g.adjacency[v] = make(map[Vertex]Edge)
		}
	}
}

// DeleteVertex uses variadic input to delete all listed vertices from the
// graph.
func (g *Graph) DeleteVertex(xv ...Vertex) {
	if len(xv) == 1 {
		v := xv[0]
		if v == nil {
			panic("nil vertex")
		}
		delete(g.adjacency, v)
		for k := range g.adjacency {
			delete(g.adjacency[k], v)
		}
		return
	}

	// handles case len(xv) == 0 and len(xv) > 1
	for _, v := range xv {
		g.DeleteVertex(v)
	}
}

// AddEdge adds a directed edge to the graph from v1 to v2.
func (g *Graph) AddEdge(v1, v2 Vertex, e Edge) {
	// NOTE: this doesn't allow more than one edge between two vertices...
	g.AddVertex(v1, v2) // supports adding N vertices now
	// TODO: check if an edge exists to avoid overwriting it!
	// NOTE: VertexMerge() depends on overwriting it at the moment...
	// NOTE: Interpret() depends on overwriting it at the moment...
	g.adjacency[v1][v2] = e
}

// DeleteEdge uses variadic input to delete all the listed edges from the graph.
func (g *Graph) DeleteEdge(xe ...Edge) {
	if len(xe) == 0 {
		return
	}
	// handles case len(xv) > 0
	for v1 := range g.adjacency {
		for v2, edge := range g.adjacency[v1] {
			for _, e := range xe {
				if e == edge {
					delete(g.adjacency[v1], v2)
				}
			}
		}
	}
}

// HasVertex returns if the input vertex exists in the graph.
func (g *Graph) HasVertex(v Vertex) bool {
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
// This works because maps are reference types so we can edit this at will.
func (g *Graph) Adjacency() map[Vertex]map[Vertex]Edge {
	return g.adjacency
}

// FindEdge returns the edge from v1 -> v2 if it exists. Otherwise nil.
func (g *Graph) FindEdge(v1, v2 Vertex) Edge {
	x, exists := g.adjacency[v1]
	if !exists {
		return nil // not found
	}
	edge, exists := x[v2]
	if !exists {
		return nil
	}
	return edge
}

// LookupEdge takes an edge and tries to find the vertex pair that connects it.
// If it finds a match, then it returns the pair and true. Otherwise it returns
// false.
func (g *Graph) LookupEdge(e Edge) (Vertex, Vertex, bool) {
	for v1, x := range g.adjacency {
		for v2, edge := range x {
			if edge == e {
				return v1, v2, true
			}
		}
	}

	return nil, nil, false // not found
}

// Vertices returns a randomly sorted slice of all vertices in the graph. The
// order is random, because the map implementation is intentionally so!
func (g *Graph) Vertices() []Vertex {
	var vertices []Vertex
	for k := range g.adjacency {
		vertices = append(vertices, k)
	}
	return vertices
}

// Edges returns a randomly sorted slice of all edges in the graph. The order is
// random, because the map implementation is intentionally so!
func (g *Graph) Edges() []Edge {
	var edges []Edge
	for vertex := range g.adjacency {
		for _, edge := range g.adjacency[vertex] {
			edges = append(edges, edge)
		}
	}
	return edges
}

// VerticesChan returns a channel of all vertices in the graph.
func (g *Graph) VerticesChan() chan Vertex {
	ch := make(chan Vertex)
	go func(ch chan Vertex) {
		for k := range g.adjacency {
			ch <- k
		}
		close(ch)
	}(ch)
	return ch
}

// VertexSlice is a linear list of vertices. It can be sorted.
type VertexSlice []Vertex

// Len returns the length of the slice of vertices.
func (vs VertexSlice) Len() int { return len(vs) }

// Swap swaps two elements in the slice.
func (vs VertexSlice) Swap(i, j int) { vs[i], vs[j] = vs[j], vs[i] }

// Less returns the smaller element in the sort order.
func (vs VertexSlice) Less(i, j int) bool {
	a := vs[i].String()
	b := vs[j].String()
	if a == b { // fallback to ptr compare
		return fmt.Sprintf("%p", vs[i]) < fmt.Sprintf("%p", vs[j])
	}
	return a < b
}

// Sort is a convenience method.
func (vs VertexSlice) Sort() { sort.Sort(vs) }

// VerticesSorted returns a sorted slice of all vertices in the graph. The order
// is sorted by String() to avoid the non-determinism in the map type.
func (g *Graph) VerticesSorted() []Vertex {
	var vertices []Vertex
	for k := range g.adjacency {
		vertices = append(vertices, k)
	}
	sort.Sort(VertexSlice(vertices)) // add determinism
	return vertices
}

// String makes the graph pretty print.
func (g *Graph) String() string {
	if g == nil { // don't panic if we're printing a nil graph
		return fmt.Sprintf("%v", nil) // prints a <nil>
	}
	return fmt.Sprintf("Vertices(%d), Edges(%d)", g.NumVertices(), g.NumEdges())
}

// Sprint prints a full graph in textual form out to a string. To log this you
// might want to use Logf, which will keep everything aligned with whatever your
// logging prefix is. This function returns the result in a deterministic order.
func (g *Graph) Sprint() string {
	if g == nil {
		return ""
	}
	var str string
	for _, v := range g.VerticesSorted() {
		str += fmt.Sprintf("Vertex: %s\n", v)
	}
	for _, v1 := range g.VerticesSorted() {
		vs := []Vertex{}
		for v2 := range g.Adjacency()[v1] {
			vs = append(vs, v2)
		}
		sort.Sort(VertexSlice(vs)) // deterministic order
		for _, v2 := range vs {
			e := g.Adjacency()[v1][v2]
			str += fmt.Sprintf("Edge: %s -> %s # %s\n", v1, v2, e)
		}
	}
	return strings.TrimSuffix(str, "\n") // trim off trailing \n if it exists
}

// Logf logs a printed representation of the graph with the logf of your choice.
// This is helpful to ensure each line of logged output has the prefix you want.
func (g *Graph) Logf(logf func(format string, v ...interface{})) {
	for _, x := range strings.Split(g.Sprint(), "\n") {
		logf("%s", x)
	}
}

// IncomingGraphVertices returns an array (slice) of all directed vertices to
// vertex v (??? -> v). OKTimestamp should probably use this.
func (g *Graph) IncomingGraphVertices(v Vertex) []Vertex {
	// TODO: we might be able to implement this differently by reversing
	// the Adjacency graph and then looping through it again...
	var s []Vertex
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
func (g *Graph) OutgoingGraphVertices(v Vertex) []Vertex {
	var s []Vertex
	for k := range g.adjacency[v] { // forward paths
		s = append(s, k)
	}
	return s
}

// GraphVertices returns an array (slice) of all vertices that connect to vertex
// v. This is the union of IncomingGraphVertices and OutgoingGraphVertices.
func (g *Graph) GraphVertices(v Vertex) []Vertex {
	var s []Vertex
	s = append(s, g.IncomingGraphVertices(v)...)
	s = append(s, g.OutgoingGraphVertices(v)...)
	return s
}

// IncomingGraphEdges returns all of the edges that point to vertex v.
// Eg: (??? -> v).
func (g *Graph) IncomingGraphEdges(v Vertex) []Edge {
	var edges []Edge
	for v1 := range g.adjacency { // reverse paths
		for v2, e := range g.adjacency[v1] {
			if v2 == v {
				edges = append(edges, e)
			}
		}
	}
	return edges
}

// OutgoingGraphEdges returns all of the edges that point from vertex v.
// Eg: (v -> ???).
func (g *Graph) OutgoingGraphEdges(v Vertex) []Edge {
	var edges []Edge
	for _, e := range g.adjacency[v] { // forward paths
		edges = append(edges, e)
	}
	return edges
}

// GraphEdges returns an array (slice) of all edges that connect to vertex v.
// This is the union of IncomingGraphEdges and OutgoingGraphEdges.
func (g *Graph) GraphEdges(v Vertex) []Edge {
	var edges []Edge
	edges = append(edges, g.IncomingGraphEdges(v)...)
	edges = append(edges, g.OutgoingGraphEdges(v)...)
	return edges
}

// DFS returns a depth first search for the graph, starting at the input vertex.
func (g *Graph) DFS(start Vertex) []Vertex {
	var d []Vertex // discovered
	var s []Vertex // stack
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
func (g *Graph) FilterGraph(vertices []Vertex) (*Graph, error) {
	fn := func(v Vertex) (bool, error) {
		return VertexContains(v, vertices), nil
	}
	return g.FilterGraphWithFn(fn)
}

// FilterGraphWithFn builds a new graph containing only vertices which match. It
// uses a user defined function to match. That function must return true on
// match, and an error if anything goes wrong.
func (g *Graph) FilterGraphWithFn(fn func(Vertex) (bool, error)) (*Graph, error) {
	newGraph, err := NewGraph(g.Name)
	if err != nil {
		return nil, err
	}
	for k1, x := range g.adjacency {
		contains, err := fn(k1)
		if err != nil {
			return nil, errwrap.Wrapf(err, "fn in FilterGraphWithFn() errored")
		} else if contains {
			newGraph.AddVertex(k1)
		}
		for k2, e := range x {
			innerContains, err := fn(k2)
			if err != nil {
				return nil, errwrap.Wrapf(err, "fn in FilterGraphWithFn() errored")
			}
			if contains && innerContains {
				newGraph.AddEdge(k1, k2, e)
			}
		}
	}
	return newGraph, nil
}

// DisconnectedGraphs returns a list containing the N disconnected graphs.
func (g *Graph) DisconnectedGraphs() ([]*Graph, error) {
	graphs := []*Graph{}
	var start Vertex
	var d []Vertex // discovered
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
		// TODO: is this method of filtering correct here? && or || ?
		newGraph, err := g.FilterGraph(dfs)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not run DisconnectedGraphs() properly")
		}
		// add number of elements found to found variable
		d = append(d, dfs...) // extend

		// append this new graph to the list
		graphs = append(graphs, newGraph)

		// if we've found all the elements, then we're done
		// otherwise loop through to continue...
	}
	return graphs, nil
}

// InDegree returns the count of vertices that point to me in one big lookup
// map.
func (g *Graph) InDegree() map[Vertex]int {
	result := make(map[Vertex]int)
	if g == nil || g.adjacency == nil {
		return result
	}
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

// OutDegree returns the count of vertices that point away in one big lookup
// map.
func (g *Graph) OutDegree() map[Vertex]int {
	result := make(map[Vertex]int)
	if g == nil || g.adjacency == nil {
		return result
	}
	for k := range g.adjacency {
		result[k] = 0 // initialize
		for range g.adjacency[k] {
			result[k]++
		}
	}
	return result
}

// TopologicalSort returns the sort of graph vertices in that order. It is based
// on descriptions and code from wikipedia and rosetta code.
// TODO: add memoization, and cache invalidation to speed this up :)
func (g *Graph) TopologicalSort() ([]Vertex, error) { // kahn's algorithm
	var L []Vertex                    // empty list that will contain the sorted elements
	var S []Vertex                    // set of all nodes with no incoming edges
	remaining := make(map[Vertex]int) // amount of edges remaining

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
					return nil, ErrNotAcyclic
				}
			}
		}
	}

	return L, nil
}

// DeterministicTopologicalSort returns the sort of graph vertices in a stable
// topological sort order. It's slower than the TopologicalSort implementation,
// but guarantees that two identical graphs produce the same sort each time.
// TODO: add memoization, and cache invalidation to speed this up :)
func (g *Graph) DeterministicTopologicalSort() ([]Vertex, error) { // kahn's algorithm
	var L []Vertex                    // empty list that will contain the sorted elements
	var S []Vertex                    // set of all nodes with no incoming edges
	remaining := make(map[Vertex]int) // amount of edges remaining

	var vertices []Vertex
	indegree := g.InDegree()
	for k := range indegree {
		vertices = append(vertices, k)
	}
	sort.Sort(VertexSlice(vertices)) // add determinism
	//for v, d := range g.InDegree()
	for _, v := range vertices { // map[Vertex]int
		d := indegree[v]
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

		var vertices []Vertex
		for n := range g.adjacency[v] { // map[Vertex]Edge
			vertices = append(vertices, n)
		}
		sort.Sort(VertexSlice(vertices)) // add determinism
		for _, n := range vertices {     // map[Vertex]Edge
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
					return nil, ErrNotAcyclic
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
//
// This operates by a recursive algorithm; a more efficient version is likely.
// If you don't give this function a DAG, you might cause infinite recursion!
func (g *Graph) Reachability(a, b Vertex) ([]Vertex, error) {
	if a == nil || b == nil {
		return nil, fmt.Errorf("empty vertex")
	}
	if _, err := g.TopologicalSort(); err != nil {
		return nil, err // not a dag
	}

	vertices := g.OutgoingGraphVertices(a) // what points away from a ?
	if len(vertices) == 0 {
		return []Vertex{}, nil // nope
	}
	if VertexContains(b, vertices) {
		return []Vertex{a, b}, nil // found
	}
	// TODO: parallelize this with go routines?
	var collected = make([][]Vertex, len(vertices))
	var err error
	pick := -1
	for i, v := range vertices {
		collected[i], err = g.Reachability(v, b) // find b by recursion
		if err != nil {
			return nil, err
		}
		if l := len(collected[i]); l > 0 {
			// pick shortest path
			// TODO: technically i should return a tree
			if pick < 0 || l < len(collected[pick]) {
				pick = i
			}
		}
	}
	if pick < 0 {
		return []Vertex{}, nil // nope
	}
	result := []Vertex{a} // tack on a
	result = append(result, collected[pick]...)
	return result, nil
}

// VertexMatchFn searches for a vertex in the graph and returns the vertex if
// one matches. It uses a user defined function to match. That function must
// return true on match, and an error if anything goes wrong.
func (g *Graph) VertexMatchFn(fn func(Vertex) (bool, error)) (Vertex, error) {
	for v := range g.adjacency {
		if b, err := fn(v); err != nil {
			return nil, errwrap.Wrapf(err, "fn in VertexMatchFn() errored")
		} else if b {
			return v, nil
		}
	}
	return nil, nil // nothing found
}

// GraphCmp compares the topology of this graph to another and returns nil if
// they're equal. It uses a user defined function to compare topologically
// equivalent vertices, and edges.
// FIXME: add more test cases
func (g *Graph) GraphCmp(graph *Graph, vertexCmpFn func(Vertex, Vertex) (bool, error), edgeCmpFn func(Edge, Edge) (bool, error)) error {
	if graph == nil || g == nil {
		if graph != g {
			return fmt.Errorf("one graph is nil")
		}
		return nil
	}
	n1, n2 := g.NumVertices(), graph.NumVertices()
	if n1 != n2 {
		return fmt.Errorf("base graph has %d vertices, while input graph has %d", n1, n2)
	}
	if e1, e2 := g.NumEdges(), graph.NumEdges(); e1 != e2 {
		return fmt.Errorf("base graph has %d edges, while input graph has %d", e1, e2)
	}

	var m = make(map[Vertex]Vertex) // g to graph vertex correspondence
Loop:
	// check vertices
	for v1 := range g.Adjacency() { // for each vertex in g
		for v2 := range graph.Adjacency() { // does it match in graph ?
			b, err := vertexCmpFn(v1, v2)
			if err != nil {
				return errwrap.Wrapf(err, "could not run vertexCmpFn() properly")
			}
			// does it match ?
			if b {
				m[v1] = v2 // store the mapping
				continue Loop
			}
		}
		return fmt.Errorf("base graph, has no match in input graph for: %s", v1)
	}
	// vertices match :)

	// is the mapping the right length?
	if n1 := len(m); n1 != n2 {
		return fmt.Errorf("mapping only has correspondence of %d, when it should have %d", n1, n2)
	}

	// check if mapping is unique (are there duplicates?)
	m1 := []Vertex{}
	m2 := []Vertex{}
	for k, v := range m {
		if VertexContains(k, m1) {
			return fmt.Errorf("mapping from %s is used more than once to: %s", k, m1)
		}
		if VertexContains(v, m2) {
			return fmt.Errorf("mapping to %s is used more than once from: %s", v, m2)
		}
		m1 = append(m1, k)
		m2 = append(m2, v)
	}

	// check edges
	for v1 := range g.Adjacency() { // for each vertex in g
		v2 := m[v1] // lookup in map to get correspondence
		// g.Adjacency()[v1] corresponds to graph.Adjacency()[v2]
		if e1, e2 := len(g.Adjacency()[v1]), len(graph.Adjacency()[v2]); e1 != e2 {
			return fmt.Errorf("base graph, vertex(%s) has %d edges, while input graph, vertex(%s) has %d", v1, e1, v2, e2)
		}

		for vv1, ee1 := range g.Adjacency()[v1] {
			vv2 := m[vv1]
			ee2 := graph.Adjacency()[v2][vv2]

			// these are edges from v1 -> vv1 via ee1 (graph 1)
			// to cmp to edges from v2 -> vv2 via ee2 (graph 2)

			// check: (1) vv1 == vv2 ? (we've already checked this!)

			// check: (2) ee1 == ee2
			b, err := edgeCmpFn(ee1, ee2)
			if err != nil {
				return errwrap.Wrapf(err, "could not run edgeCmpFn() properly")
			}
			if !b {
				return fmt.Errorf("base graph edge(%s) doesn't match input graph edge(%s)", ee1, ee2)
			}
		}
	}

	return nil // success!
}

// VertexContains is an "in array" function to test for a vertex in a slice of
// vertices.
func VertexContains(needle Vertex, haystack []Vertex) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// EdgeContains is an "in array" function to test for an edge in a slice of
// edges.
func EdgeContains(needle Edge, haystack []Edge) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// Reverse reverses a list of vertices.
func Reverse(vs []Vertex) []Vertex {
	out := []Vertex{}
	l := len(vs)
	for i := range vs {
		out = append(out, vs[l-i-1])
	}
	return out
}

// Sort the list of vertices and return a copy without modifying the input.
func Sort(vs []Vertex) []Vertex {
	vertices := []Vertex{}
	for _, v := range vs { // copy
		vertices = append(vertices, v)
	}
	sort.Sort(VertexSlice(vertices))
	return vertices
	// sort.Sort(VertexSlice(vs)) // this is wrong, it would modify input!
	//return vs
}
