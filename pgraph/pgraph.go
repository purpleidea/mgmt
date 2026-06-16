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
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// ErrNotAcyclic specifies that a particular graph was not found to be a dag.
type ErrNotAcyclic struct {
	Cycle []Vertex
}

// Error lets this satisfy the error interface.
func (obj *ErrNotAcyclic) Error() string {
	//return fmt.Sprintf("not a dag: %v", obj.Cycle)
	return "not a dag"
}

// Graph is the graph structure in this library. The graph abstract data type
// (ADT) is defined as follows:
// * The directed graph arrows point from left to right. ( -> )
// * The arrows point away from their dependencies. (eg: arrows mean "before")
// * IOW, you might see package -> file -> service. (where package runs first)
// * This is also the direction that the notify should happen in...
type Graph struct {
	Name string

	adjacency map[Vertex]map[Vertex]Edge // Vertex -> Vertex (edge)
	revadjmap map[Vertex]map[Vertex]Edge // Vertex <- Vertex (edge) mirror index
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
func (obj *Graph) Init() error {
	if obj.Name == "" { // FIXME: is this really a good requirement?
		return fmt.Errorf("can't initialize graph with empty name")
	}

	obj.adjacency = make(map[Vertex]map[Vertex]Edge)
	obj.revadjmap = make(map[Vertex]map[Vertex]Edge)
	//obj.kv = make(map[string]interface{}) // not required
	return nil
}

// NewGraph builds a new graph.
func NewGraph(name string) (*Graph, error) {
	g := &Graph{
		Name: name,
	}
	return g, g.Init()
}

// Value returns a value stored alongside the graph in a particular key.
func (obj *Graph) Value(key string) (interface{}, bool) {
	val, exists := obj.kv[key]
	return val, exists
}

// SetValue sets a value to be stored alongside the graph in a particular key.
func (obj *Graph) SetValue(key string, val interface{}) {
	if obj.kv == nil { // initialize on first use
		obj.kv = make(map[string]interface{})
	}
	obj.kv[key] = val
}

// Copy makes a copy of the graph struct. This doesn't copy the individual
// vertices or edges, those pointers remain untouched. This lets you modify the
// structure of the graph without changing the original. If you also want to
// copy the nodes, please use CopyWithFn instead.
func (obj *Graph) Copy() *Graph {
	if obj == nil { // allow nil graphs through
		return obj
	}
	newGraph := &Graph{
		Name:      obj.Name,
		adjacency: make(map[Vertex]map[Vertex]Edge, len(obj.adjacency)),
		revadjmap: make(map[Vertex]map[Vertex]Edge, len(obj.revadjmap)),
		kv:        obj.kv,
	}
	for v1, m := range obj.adjacency {
		newGraph.adjacency[v1] = make(map[Vertex]Edge, len(m))
		for v2, e := range m {
			newGraph.adjacency[v1][v2] = e // copy
		}
	}
	for v1, m := range obj.revadjmap {
		newGraph.revadjmap[v1] = make(map[Vertex]Edge, len(m))
		for v2, e := range m {
			newGraph.revadjmap[v1][v2] = e // copy
		}
	}
	return newGraph
}

// CopyWithFn makes a copy of the graph struct but lets you provide a function
// to copy the vertices.
// TODO: add tests
func (obj *Graph) CopyWithFn(vertexCpFn func(Vertex) (Vertex, error)) (*Graph, error) {
	if obj == nil { // allow nil graphs through
		return obj, nil
	}
	if l := len(obj.adjacency); vertexCpFn == nil && l > 0 {
		return nil, fmt.Errorf("graph has %d vertices, but vertexCpFn is nil", l)
	}
	newGraph := &Graph{
		Name:      obj.Name,
		adjacency: make(map[Vertex]map[Vertex]Edge, len(obj.adjacency)),
		revadjmap: make(map[Vertex]map[Vertex]Edge, len(obj.revadjmap)),
		kv:        obj.kv,
	}
	vm := make(map[Vertex]Vertex) // copy mapping from old ptr to new ptr...
	for v1, m := range obj.adjacency {
		// We copy each vertex, but then we need to do a lookup so that
		// when (if) we see that old pointer again, we use the new one.
		v, err := vertexCpFn(v1) // copy
		if err != nil {
			return nil, err
		}
		vm[v1] = v // mapping
		newGraph.AddVertex(v)
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
			//newGraph.AddEdge(v, vx, edge)
			newGraph.AddEdge(v, vx, e) // store the edge
		}
	}
	return newGraph, nil
}

// VertexSwap swaps vertices in a graph. It returns a new graph with the same
// structure but with replacements done according to the translation map passed
// in. If a vertex is not found in the graph, then it is not substituted.
// TODO: add tests
func (obj *Graph) VertexSwap(vs map[Vertex]Vertex) (*Graph, error) {
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
	return obj.CopyWithFn(vertexCpFn)
}

// GetName returns the name of the graph.
func (obj *Graph) GetName() string {
	return obj.Name
}

// SetName sets the name of the graph.
func (obj *Graph) SetName(name string) {
	obj.Name = name
}

// AddVertex uses variadic input to add all listed vertices to the graph.
func (obj *Graph) AddVertex(xv ...Vertex) {
	if obj.adjacency == nil { // initialize on first use
		obj.adjacency = make(map[Vertex]map[Vertex]Edge)
	}
	if obj.revadjmap == nil { // initialize on first use
		obj.revadjmap = make(map[Vertex]map[Vertex]Edge)
	}
	for _, v := range xv {
		if v == nil {
			panic("nil vertex")
		}
		if _, exists := obj.adjacency[v]; !exists {
			obj.adjacency[v] = make(map[Vertex]Edge)
		}
		if _, exists := obj.revadjmap[v]; !exists {
			obj.revadjmap[v] = make(map[Vertex]Edge)
		}
	}
}

// DeleteVertex uses variadic input to delete all listed vertices from the
// graph.
func (obj *Graph) DeleteVertex(xv ...Vertex) {
	if len(xv) == 1 {
		v := xv[0]
		if v == nil {
			panic("nil vertex")
		}
		// remove the mirror entries of the incoming/outgoing edges
		for k := range obj.revadjmap[v] { // edges that point to v
			delete(obj.adjacency[k], v)
		}
		for k := range obj.adjacency[v] { // edges that point from v
			delete(obj.revadjmap[k], v)
		}
		delete(obj.adjacency, v)
		delete(obj.revadjmap, v)
		return
	}

	// handles case len(xv) == 0 and len(xv) > 1
	for _, v := range xv {
		obj.DeleteVertex(v)
	}
}

// AddEdge adds a directed edge to the graph from v1 to v2.
func (obj *Graph) AddEdge(v1, v2 Vertex, e Edge) {
	// NOTE: this doesn't allow more than one edge between two vertices...
	obj.AddVertex(v1, v2) // supports adding N vertices now
	// TODO: check if an edge exists to avoid overwriting it!
	// NOTE: VertexMerge() depends on overwriting it at the moment...
	// NOTE: Interpret() depends on overwriting it at the moment...
	obj.adjacency[v1][v2] = e
	obj.revadjmap[v2][v1] = e
}

// DeleteEdge uses variadic input to delete all the listed edges from the graph.
func (obj *Graph) DeleteEdge(xe ...Edge) {
	if len(xe) == 0 {
		return
	}
	// handles case len(xv) > 0
	for v1 := range obj.adjacency {
		for v2, edge := range obj.adjacency[v1] {
			for _, e := range xe {
				if e == edge {
					obj.DeleteEdgeBetween(v1, v2)
				}
			}
		}
	}
}

// DeleteEdgeBetween deletes the edge from v1 to v2 if it exists. Unlike
// DeleteEdge, which removes an edge object wherever it appears, this removes
// the single directed edge between the two vertices in O(1) time.
func (obj *Graph) DeleteEdgeBetween(v1, v2 Vertex) {
	if m, exists := obj.adjacency[v1]; exists {
		delete(m, v2)
	}
	if m, exists := obj.revadjmap[v2]; exists {
		delete(m, v1)
	}
}

// HasVertex returns if the input vertex exists in the graph.
func (obj *Graph) HasVertex(v Vertex) bool {
	if _, exists := obj.adjacency[v]; exists {
		return true
	}
	return false
}

// NumVertices returns the number of vertices in the graph.
func (obj *Graph) NumVertices() int {
	return len(obj.adjacency)
}

// NumEdges returns the number of edges in the graph.
func (obj *Graph) NumEdges() int {
	count := 0
	for k := range obj.adjacency {
		count += len(obj.adjacency[k])
	}
	return count
}

// Adjacency returns the adjacency map representing this graph. This is useful
// for users who wish to read the raw data structure more efficiently. The
// returned map must be treated as read-only: any mutations must go through the
// graph API (AddVertex, AddEdge, DeleteVertex, DeleteEdge, DeleteEdgeBetween,
// and so on) so that any internal indexes stay consistent.
func (obj *Graph) Adjacency() map[Vertex]map[Vertex]Edge {
	return obj.adjacency
}

// FindEdge returns the edge from v1 -> v2 if it exists. Otherwise nil.
func (obj *Graph) FindEdge(v1, v2 Vertex) Edge {
	x, exists := obj.adjacency[v1]
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
func (obj *Graph) LookupEdge(e Edge) (Vertex, Vertex, bool) {
	for v1, x := range obj.adjacency {
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
func (obj *Graph) Vertices() []Vertex {
	vertices := make([]Vertex, 0, len(obj.adjacency))
	for k := range obj.adjacency {
		vertices = append(vertices, k)
	}
	return vertices
}

// Edges returns a randomly sorted slice of all edges in the graph. The order is
// random, because the map implementation is intentionally so!
func (obj *Graph) Edges() []Edge {
	var edges []Edge
	for vertex := range obj.adjacency {
		for _, edge := range obj.adjacency[vertex] {
			edges = append(edges, edge)
		}
	}
	return edges
}

// VerticesChan returns a channel of all vertices in the graph.
func (obj *Graph) VerticesChan() chan Vertex {
	ch := make(chan Vertex)
	go func(ch chan Vertex) {
		for k := range obj.adjacency {
			ch <- k
		}
		close(ch)
	}(ch)
	return ch
}

// VertexSlice is a linear list of vertices. It can be sorted.
type VertexSlice []Vertex

// Len returns the length of the slice of vertices.
func (obj VertexSlice) Len() int { return len(obj) }

// Swap swaps two elements in the slice.
func (obj VertexSlice) Swap(i, j int) { obj[i], obj[j] = obj[j], obj[i] }

// Less returns the smaller element in the sort order.
func (obj VertexSlice) Less(i, j int) bool {
	a := obj[i].String()
	b := obj[j].String()
	if a == b { // fallback to ptr compare
		return reflect.ValueOf(obj[i]).Pointer() < reflect.ValueOf(obj[j]).Pointer()
	}
	return a < b
}

// Sort sorts the slice in place. It precomputes each vertex's String() value
// once so that an expensive String() implementation is not re-invoked O(log N)
// times per element by the underlying sort. Calling sort.Sort(VertexSlice)
// directly still works but does not get this optimization.
func (obj VertexSlice) Sort() {
	keys := make([]string, len(obj))
	for i, v := range obj {
		keys[i] = v.String()
	}
	sort.Sort(&keyedVertexSlice{vs: obj, keys: keys})
}

// keyedVertexSlice pairs a VertexSlice with a parallel slice of precomputed
// String() keys so sort.Sort can compare without re-invoking String().
type keyedVertexSlice struct {
	vs   VertexSlice
	keys []string
}

func (obj *keyedVertexSlice) Len() int { return len(obj.vs) }

func (obj *keyedVertexSlice) Swap(i, j int) {
	obj.vs[i], obj.vs[j] = obj.vs[j], obj.vs[i]
	obj.keys[i], obj.keys[j] = obj.keys[j], obj.keys[i]
}

func (obj *keyedVertexSlice) Less(i, j int) bool {
	if obj.keys[i] == obj.keys[j] { // fallback to ptr compare
		return reflect.ValueOf(obj.vs[i]).Pointer() < reflect.ValueOf(obj.vs[j]).Pointer()
	}
	return obj.keys[i] < obj.keys[j]
}

// sortVerticesReuse sorts vs in place by String() using a caller-supplied keys
// buffer and keyedVertexSlice, both of which are reused across calls to avoid
// per-call allocations from VertexSlice.Sort. The (possibly grown) keys buffer
// is returned for reuse on the next call.
func sortVerticesReuse(vs []Vertex, keys []string, ks *keyedVertexSlice) []string {
	if cap(keys) >= len(vs) {
		keys = keys[:len(vs)]
	} else {
		keys = make([]string, len(vs))
	}
	for i, v := range vs {
		keys[i] = v.String()
	}
	ks.vs = vs
	ks.keys = keys
	sort.Sort(ks)
	return keys
}

// VerticesSorted returns a sorted slice of all vertices in the graph. The order
// is sorted by String() to avoid the non-determinism in the map type.
func (obj *Graph) VerticesSorted() []Vertex {
	vertices := make([]Vertex, 0, len(obj.adjacency))
	for k := range obj.adjacency {
		vertices = append(vertices, k)
	}
	VertexSlice(vertices).Sort() // add determinism
	return vertices
}

// String makes the graph pretty print.
func (obj *Graph) String() string {
	if obj == nil { // don't panic if we're printing a nil graph
		return fmt.Sprintf("%v", nil) // prints a <nil>
	}
	return fmt.Sprintf("Vertices(%d), Edges(%d)", obj.NumVertices(), obj.NumEdges())
}

// Sprint prints a full graph in textual form out to a string. To log this you
// might want to use Logf, which will keep everything aligned with whatever your
// logging prefix is. This function returns the result in a deterministic order.
func (obj *Graph) Sprint() string {
	if obj == nil {
		return ""
	}
	var str string
	for _, v := range obj.VerticesSorted() {
		str += fmt.Sprintf("Vertex: %s\n", v)
	}
	for _, v1 := range obj.VerticesSorted() {
		vs := []Vertex{}
		for v2 := range obj.adjacency[v1] {
			vs = append(vs, v2)
		}
		VertexSlice(vs).Sort() // deterministic order
		for _, v2 := range vs {
			e := obj.adjacency[v1][v2]
			str += fmt.Sprintf("Edge: %s -> %s # %s\n", v1, v2, e)
		}
	}
	return strings.TrimSuffix(str, "\n") // trim off trailing \n if it exists
}

// Logf logs a printed representation of the graph with the logf of your choice.
// This is helpful to ensure each line of logged output has the prefix you want.
func (obj *Graph) Logf(logf func(format string, v ...interface{})) {
	for _, x := range strings.Split(obj.Sprint(), "\n") {
		logf("%s", x)
	}
}

// IncomingGraphVertices returns an array (slice) of all directed vertices to
// vertex v (??? -> v). OKTimestamp should probably use this.
func (obj *Graph) IncomingGraphVertices(v Vertex) []Vertex {
	s := make([]Vertex, 0, len(obj.revadjmap[v]))
	for k := range obj.revadjmap[v] { // reverse paths
		s = append(s, k)
	}
	return s
}

// OutgoingGraphVertices returns an array (slice) of all vertices that vertex v
// points to (v -> ???). Poke should probably use this.
func (obj *Graph) OutgoingGraphVertices(v Vertex) []Vertex {
	s := make([]Vertex, 0, len(obj.adjacency[v]))
	for k := range obj.adjacency[v] { // forward paths
		s = append(s, k)
	}
	return s
}

// GraphVertices returns an array (slice) of all vertices that connect to vertex
// v. This is the union of IncomingGraphVertices and OutgoingGraphVertices.
func (obj *Graph) GraphVertices(v Vertex) []Vertex {
	var s []Vertex
	s = append(s, obj.IncomingGraphVertices(v)...)
	s = append(s, obj.OutgoingGraphVertices(v)...)
	return s
}

// IncomingGraphEdges returns all of the edges that point to vertex v.
// Eg: (??? -> v).
func (obj *Graph) IncomingGraphEdges(v Vertex) []Edge {
	var edges []Edge
	for _, e := range obj.revadjmap[v] { // reverse paths
		edges = append(edges, e)
	}
	return edges
}

// OutgoingGraphEdges returns all of the edges that point from vertex v.
// Eg: (v -> ???).
func (obj *Graph) OutgoingGraphEdges(v Vertex) []Edge {
	var edges []Edge
	for _, e := range obj.adjacency[v] { // forward paths
		edges = append(edges, e)
	}
	return edges
}

// GraphEdges returns an array (slice) of all edges that connect to vertex v.
// This is the union of IncomingGraphEdges and OutgoingGraphEdges.
func (obj *Graph) GraphEdges(v Vertex) []Edge {
	var edges []Edge
	edges = append(edges, obj.IncomingGraphEdges(v)...)
	edges = append(edges, obj.OutgoingGraphEdges(v)...)
	return edges
}

// DFS returns a depth first search for the graph, starting at the input vertex.
func (obj *Graph) DFS(start Vertex) []Vertex {
	var result []Vertex
	var s []Vertex                 // stack
	d := make(map[Vertex]struct{}) // discovered (map for O(1) lookups)
	if _, exists := obj.adjacency[start]; !exists {
		return nil // TODO: error
	}
	v := start
	s = append(s, v)
	for len(s) > 0 {
		v, s = s[len(s)-1], s[:len(s)-1] // s.pop()

		if _, exists := d[v]; !exists { // if not discovered
			d[v] = struct{}{} // label as discovered
			result = append(result, v)

			for _, w := range obj.GraphVertices(v) {
				s = append(s, w)
			}
		}
	}
	return result
}

// FilterGraph builds a new graph containing only vertices from the list.
func (obj *Graph) FilterGraph(vertices []Vertex) (*Graph, error) {
	fn := func(v Vertex) (bool, error) {
		return VertexContains(v, vertices), nil
	}
	return obj.FilterGraphWithFn(fn)
}

// FilterGraphWithFn builds a new graph containing only vertices which match. It
// uses a user defined function to match. That function must return true on
// match, and an error if anything goes wrong.
func (obj *Graph) FilterGraphWithFn(fn func(Vertex) (bool, error)) (*Graph, error) {
	newGraph, err := NewGraph(obj.Name)
	if err != nil {
		return nil, err
	}
	for k1, x := range obj.adjacency {
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
func (obj *Graph) DisconnectedGraphs() ([]*Graph, error) {
	graphs := []*Graph{}
	var start Vertex
	d := make(map[Vertex]struct{}) // discovered map for O(1) lookups
	c := obj.NumVertices()
	for len(d) < c {

		// get an undiscovered vertex to start from
		for s := range obj.adjacency {
			if _, exists := d[s]; !exists {
				start = s
				break
			}
		}

		// dfs through the graph
		dfs := obj.DFS(start)
		// filter all the collected elements into a new graph
		// TODO: is this method of filtering correct here? && or || ?
		newGraph, err := obj.FilterGraph(dfs)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not run DisconnectedGraphs() properly")
		}
		// add number of elements found to the discovered set
		for _, v := range dfs {
			d[v] = struct{}{}
		}

		// append this new graph to the list
		graphs = append(graphs, newGraph)

		// if we've found all the elements, then we're done
		// otherwise loop through to continue...
	}
	return graphs, nil
}

// InDegree returns the count of vertices that point to me in one big lookup
// map.
func (obj *Graph) InDegree() map[Vertex]int {
	if obj == nil || obj.adjacency == nil {
		return nil
	}
	result := make(map[Vertex]int, len(obj.adjacency))
	for k := range obj.adjacency {
		result[k] = len(obj.revadjmap[k])
	}
	return result
}

// OutDegree returns the count of vertices that point away in one big lookup
// map.
func (obj *Graph) OutDegree() map[Vertex]int {
	if obj == nil || obj.adjacency == nil {
		return nil
	}
	result := make(map[Vertex]int, len(obj.adjacency))
	for k := range obj.adjacency {
		result[k] = len(obj.adjacency[k])
	}
	return result
}

// TopologicalSort returns the sort of graph vertices in that order. It is based
// on descriptions and code from wikipedia and rosetta code.
// TODO: add memoization, and cache invalidation to speed this up :)
func (obj *Graph) TopologicalSort() ([]Vertex, error) { // kahn's algorithm
	// XXX: is "make" with this length on these three structures correct?
	l := make([]Vertex, 0, len(obj.adjacency))            // empty list that will contain the sorted elements
	s := make([]Vertex, 0, len(obj.adjacency))            // set of all nodes with no incoming edges
	remaining := make(map[Vertex]int, len(obj.adjacency)) // amount of edges remaining

	// count incoming edges directly instead of allocating a separate
	// InDegree map and then re-walking it
	for _, m := range obj.adjacency {
		for n := range m {
			remaining[n]++
		}
	}
	for v := range obj.adjacency {
		if remaining[v] == 0 {
			// accumulate set of all nodes with no incoming edges
			s = append(s, v)
		}
	}

	for len(s) > 0 {
		last := len(s) - 1 // remove a node v from s
		v := s[last]
		s = s[:last]
		l = append(l, v) // add v to tail of l
		for n := range obj.adjacency[v] {
			// remaining[n] always exists here: n is a child of v,
			// so n had at least one incoming edge and got an entry
			// during the initial count. Roots aren't reachable
			// through this walk, so no zero-key surprises.
			remaining[n]--         // remove edge from the graph
			if remaining[n] == 0 { // if n has no other incoming edges
				s = append(s, n) // insert n into s
			}
		}
	}

	// if we visited every vertex, there are no cycles; otherwise scan
	// remaining for any vertex with edges left and report the cycle
	if len(l) != len(obj.adjacency) {
		return nil, obj.notAcyclicErr(remaining)
	}

	return l, nil
}

// notAcyclicErr is a helper shared by TopologicalSort and
// DeterministicTopologicalSort. It picks any vertex with leftover incoming
// edges and runs findCycleDFS to produce an ErrNotAcyclic.
func (obj *Graph) notAcyclicErr(remaining map[Vertex]int) error {
	for c, in := range remaining {
		if in > 0 {
			cycle := obj.findCycleDFS(c)
			if len(cycle) == 0 {
				// Hopefully this doesn't happen!
				return fmt.Errorf("programming error")
			}
			return &ErrNotAcyclic{Cycle: cycle}
		}
	}
	// Hopefully this doesn't happen!
	return fmt.Errorf("programming error")
}

// findCycleDFS is a helper for the TopologicalSort functions.
// XXX: A professional should look over this function and try and find issues.
func (obj *Graph) findCycleDFS(start Vertex) []Vertex {
	visited := make(map[Vertex]bool)
	stack := make(map[Vertex]bool)
	var path []Vertex
	var result []Vertex
	found := false

	var dfs func(Vertex) bool
	dfs = func(v Vertex) bool {
		if found {
			return true
		}
		visited[v] = true
		stack[v] = true
		path = append(path, v)

		for n := range obj.adjacency[v] {
			if !visited[n] {
				if dfs(n) {
					return true
				}
			} else if stack[n] {
				// cycle detected
				idx := len(path) - 1
				for idx >= 0 && path[idx] != n {
					idx--
				}
				if idx >= 0 {
					result = append([]Vertex{}, path[idx:]...)
					result = append(result, n) // close the cycle
					found = true
					return true
				}
			}
		}

		stack[v] = false
		path = path[:len(path)-1]
		return false
	}

	// run DFS from all potentially cyclic nodes
	for v := range obj.adjacency {
		if !visited[v] {
			if dfs(v) {
				break
			}
		}
	}

	return result
}

// DeterministicTopologicalSort returns the sort of graph vertices in a stable
// topological sort order. It's slower than the TopologicalSort implementation,
// but guarantees that two identical graphs produce the same sort each time.
// TODO: add memoization, and cache invalidation to speed this up :)
func (obj *Graph) DeterministicTopologicalSort() ([]Vertex, error) { // kahn's algorithm
	// XXX: is "make" with this length on these three structures correct?
	l := make([]Vertex, 0, len(obj.adjacency))            // empty list that will contain the sorted elements
	s := make([]Vertex, 0, len(obj.adjacency))            // set of all nodes with no incoming edges
	remaining := make(map[Vertex]int, len(obj.adjacency)) // amount of edges remaining

	// count incoming edges directly instead of allocating a separate
	// InDegree map and then re-walking it
	vertices := make([]Vertex, 0, len(obj.adjacency))
	for v, m := range obj.adjacency {
		vertices = append(vertices, v)
		for n := range m {
			remaining[n]++
		}
	}
	// Reuse a single keys buffer and keyedVertexSlice across every sort
	// call in this function so we don't allocate per pop.
	ks := &keyedVertexSlice{}
	keys := sortVerticesReuse(vertices, nil, ks) // add determinism
	for _, v := range vertices {
		if remaining[v] == 0 {
			// accumulate set of all nodes with no incoming edges
			s = append(s, v)
		}
	}

	// Reusable buffer for v's children; reset to [:0] each iteration.
	var children []Vertex
	for len(s) > 0 {
		last := len(s) - 1 // remove a node v from s
		v := s[last]
		s = s[:last]
		l = append(l, v) // add v to tail of l

		children = children[:0]
		for n := range obj.adjacency[v] { // map[Vertex]Edge
			children = append(children, n)
		}
		keys = sortVerticesReuse(children, keys, ks) // add determinism
		for _, n := range children {
			// remaining[n] always exists here; see TopologicalSort.
			remaining[n]--         // remove edge from the graph
			if remaining[n] == 0 { // if n has no other incoming edges
				s = append(s, n) // insert n into s
			}
		}
	}

	// if we visited every vertex, there are no cycles; otherwise scan
	// remaining for any vertex with edges left and report the cycle
	if len(l) != len(obj.adjacency) {
		return nil, obj.notAcyclicErr(remaining)
	}

	return l, nil
}

// Reachability finds the shortest path in a DAG from a to b, and returns the
// slice of vertices that matched this particular path including both a and b.
// It returns nil if a or b is nil, and returns empty list if no path is found.
// Since there could be more than one possible result for this operation, we
// arbitrarily choose one of the shortest possible. As a result, this should
// actually return a tree if we cared about correctness.
func (obj *Graph) Reachability(a, b Vertex) ([]Vertex, error) {
	if a == nil || b == nil {
		return nil, fmt.Errorf("empty vertex")
	}
	if _, err := obj.TopologicalSort(); err != nil {
		return nil, err // not a dag
	}
	return obj.bfsShortestPath(a, b), nil
}

// ReachabilityUnsafe is identical to Reachability but without the
// TopologicalSort() DAG validation. The caller must ensure the graph is a DAG
// before calling this method if they need that guarantee; the BFS itself is
// safe to run on any graph.
func (obj *Graph) ReachabilityUnsafe(a, b Vertex) ([]Vertex, error) {
	if a == nil || b == nil {
		return nil, fmt.Errorf("empty vertex")
	}
	return obj.bfsShortestPath(a, b), nil
}

// bfsShortestPath runs a breadth-first search from a, looking for b, and
// returns the shortest path (by edge count) including both endpoints. If no
// path exists it returns an empty slice. The previous recursive implementation
// re-explored shared subpaths and (in Reachability) re-validated the DAG on
// every recursive call, giving exponential worst-case behaviour; BFS is O(V+E).
func (obj *Graph) bfsShortestPath(a, b Vertex) []Vertex {
	if _, exists := obj.adjacency[a]; !exists {
		return []Vertex{}
	}
	parent := make(map[Vertex]Vertex)
	visited := map[Vertex]struct{}{a: {}}
	queue := []Vertex{a}
	found := false
	for len(queue) > 0 && !found {
		v := queue[0]
		queue = queue[1:]
		for n := range obj.adjacency[v] {
			if _, ok := visited[n]; ok {
				continue
			}
			visited[n] = struct{}{}
			parent[n] = v
			if n == b {
				found = true
				break
			}
			queue = append(queue, n)
		}
	}
	if !found {
		return []Vertex{}
	}
	// reconstruct path from b back to a, then reverse
	path := []Vertex{b}
	for v := b; v != a; {
		v = parent[v]
		path = append(path, v)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// HasPath returns true if the directed graph has a path from a to b. It does
// not validate that the graph is acyclic, so callers that need that guarantee
// must check it separately.
func (obj *Graph) HasPath(a, b Vertex) bool {
	if obj == nil || obj.adjacency == nil || a == nil || b == nil {
		return false
	}
	if _, exists := obj.adjacency[a]; !exists {
		return false
	}
	if _, exists := obj.adjacency[b]; !exists {
		return false
	}
	if a == b {
		return true
	}

	stack := make([]Vertex, 0, len(obj.adjacency)) // XXX: what size?
	stack = append(stack, a)
	visited := make(map[Vertex]struct{}, len(obj.adjacency))
	visited[a] = struct{}{}
	for len(stack) > 0 {
		last := len(stack) - 1
		v := stack[last]
		stack = stack[:last]

		//if _, ok := obj.adjacency[v]; !ok { // badly formed adjacency?
		//	continue
		//}

		for n := range obj.adjacency[v] {
			if n == b {
				return true
			}
			if _, exists := visited[n]; exists {
				continue
			}
			visited[n] = struct{}{}
			stack = append(stack, n)
		}
	}
	return false
}

// VertexMatchFn searches for a vertex in the graph and returns the vertex if
// one matches. It uses a user defined function to match. That function must
// return true on match, and an error if anything goes wrong.
func (obj *Graph) VertexMatchFn(fn func(Vertex) (bool, error)) (Vertex, error) {
	for v := range obj.adjacency {
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
func (obj *Graph) GraphCmp(graph *Graph, vertexCmpFn func(Vertex, Vertex) (bool, error), edgeCmpFn func(Edge, Edge) (bool, error)) error {
	if graph == nil || obj == nil {
		if graph != obj {
			return fmt.Errorf("one graph is nil")
		}
		return nil
	}
	n1, n2 := obj.NumVertices(), graph.NumVertices()
	if n1 != n2 {
		return fmt.Errorf("base graph has %d vertices, while input graph has %d", n1, n2)
	}
	if e1, e2 := obj.NumEdges(), graph.NumEdges(); e1 != e2 {
		return fmt.Errorf("base graph has %d edges, while input graph has %d", e1, e2)
	}

	var m = make(map[Vertex]Vertex) // obj to graph vertex correspondence
Loop:
	// check vertices
	for v1 := range obj.adjacency { // for each vertex in g
		for v2 := range graph.adjacency { // does it match in graph ?
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
	// (check values only, the keys are unique by virtue of m being a map)
	seen := make(map[Vertex]Vertex, len(m))
	for k, v := range m {
		if prev, exists := seen[v]; exists {
			return fmt.Errorf("mapping to %s is used more than once from: %s and %s", v, prev, k)
		}
		seen[v] = k
	}

	// check edges
	for v1 := range obj.adjacency { // for each vertex in g
		v2 := m[v1] // lookup in map to get correspondence
		// obj.adjacency[v1] corresponds to graph.adjacency[v2]
		if e1, e2 := len(obj.adjacency[v1]), len(graph.adjacency[v2]); e1 != e2 {
			return fmt.Errorf("base graph, vertex(%s) has %d edges, while input graph, vertex(%s) has %d", v1, e1, v2, e2)
		}

		for vv1, ee1 := range obj.adjacency[v1] {
			vv2 := m[vv1]
			ee2 := graph.adjacency[v2][vv2]

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
	vertices := make([]Vertex, len(vs))
	copy(vertices, vs)
	VertexSlice(vertices).Sort()
	return vertices
	// sort.Sort(VertexSlice(vs)) // this is wrong, it would modify input!
	//return vs
}
