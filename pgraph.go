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

// Pgraph (Pointer Graph)
package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
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

// The graph abstract data type (ADT) is defined as follows:
// * the directed graph arrows point from left to right ( -> )
// * the arrows point away from their dependencies (eg: arrows mean "before")
// * IOW, you might see package -> file -> service (where package runs first)
// * This is also the direction that the notify should happen in...
type Graph struct {
	Name      string
	Adjacency map[*Vertex]map[*Vertex]*Edge // *Vertex -> *Vertex (edge)
	state     graphState
	mutex     sync.Mutex // used when modifying graph State variable
}

type Vertex struct {
	Res             // anonymous field
	timestamp int64 // last updated timestamp ?
}

type Edge struct {
	Name string
}

func NewGraph(name string) *Graph {
	return &Graph{
		Name:      name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge),
		state:     graphStateNil,
	}
}

func NewVertex(r Res) *Vertex {
	return &Vertex{
		Res: r,
	}
}

func NewEdge(name string) *Edge {
	return &Edge{
		Name: name,
	}
}

// Copy makes a copy of the graph struct
func (g *Graph) Copy() *Graph {
	newGraph := &Graph{
		Name:      g.Name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge, len(g.Adjacency)),
		state:     g.state,
	}
	for k, v := range g.Adjacency {
		newGraph.Adjacency[k] = v // copy
	}
	return newGraph
}

// returns the name of the graph
func (g *Graph) GetName() string {
	return g.Name
}

// set name of the graph
func (g *Graph) SetName(name string) {
	g.Name = name
}

func (g *Graph) GetState() graphState {
	//g.mutex.Lock()
	//defer g.mutex.Unlock()
	return g.state
}

// set graph state and return previous state
func (g *Graph) SetState(state graphState) graphState {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	prev := g.GetState()
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

func (g *Graph) DeleteVertex(v *Vertex) {
	delete(g.Adjacency, v)
	for k := range g.Adjacency {
		delete(g.Adjacency[k], v)
	}
}

// adds a directed edge to the graph from v1 to v2
func (g *Graph) AddEdge(v1, v2 *Vertex, e *Edge) {
	// NOTE: this doesn't allow more than one edge between two vertexes...
	g.AddVertex(v1, v2) // supports adding N vertices now
	// TODO: check if an edge exists to avoid overwriting it!
	// NOTE: VertexMerge() depends on overwriting it at the moment...
	g.Adjacency[v1][v2] = e
}

func (g *Graph) GetVertexMatch(obj Res) *Vertex {
	for k := range g.Adjacency {
		if k.Res.Compare(obj) {
			return k
		}
	}
	return nil
}

func (g *Graph) HasVertex(v *Vertex) bool {
	if _, exists := g.Adjacency[v]; exists {
		return true
	}
	return false
}

// number of vertices in the graph
func (g *Graph) NumVertices() int {
	return len(g.Adjacency)
}

// number of edges in the graph
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

// returns a channel of all vertices in the graph
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

// make the graph pretty print
func (g *Graph) String() string {
	return fmt.Sprintf("Vertices(%d), Edges(%d)", g.NumVertices(), g.NumEdges())
}

// String returns the canonical form for a vertex
func (v *Vertex) String() string {
	return fmt.Sprintf("%s[%s]", v.Res.Kind(), v.Res.GetName())
}

// output the graph in graphviz format
// https://en.wikipedia.org/wiki/DOT_%28graph_description_language%29
func (g *Graph) Graphviz() (out string) {
	//digraph g {
	//	label="hello world";
	//	node [shape=box];
	//	A [label="A"];
	//	B [label="B"];
	//	C [label="C"];
	//	D [label="D"];
	//	E [label="E"];
	//	A -> B [label=f];
	//	B -> C [label=g];
	//	D -> E [label=h];
	//}
	out += fmt.Sprintf("digraph %v {\n", g.GetName())
	out += fmt.Sprintf("\tlabel=\"%v\";\n", g.GetName())
	//out += "\tnode [shape=box];\n"
	str := ""
	for i := range g.Adjacency { // reverse paths
		out += fmt.Sprintf("\t%v [label=\"%v[%v]\"];\n", i.GetName(), i.Kind(), i.GetName())
		for j := range g.Adjacency[i] {
			k := g.Adjacency[i][j]
			// use str for clearer output ordering
			str += fmt.Sprintf("\t%v -> %v [label=%v];\n", i.GetName(), j.GetName(), k.Name)
		}
	}
	out += str
	out += "}\n"
	return
}

// write out the graphviz data and run the correct graphviz filter command
func (g *Graph) ExecGraphviz(program, filename string) error {

	switch program {
	case "dot", "neato", "twopi", "circo", "fdp":
	default:
		return errors.New("Invalid graphviz program selected!")
	}

	if filename == "" {
		return errors.New("No filename given!")
	}

	// run as a normal user if possible when run with sudo
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))

	err := ioutil.WriteFile(filename, []byte(g.Graphviz()), 0644)
	if err != nil {
		return errors.New("Error writing to filename!")
	}

	if err1 == nil && err2 == nil {
		if err := os.Chown(filename, uid, gid); err != nil {
			return errors.New("Error changing file owner!")
		}
	}

	path, err := exec.LookPath(program)
	if err != nil {
		return errors.New("Graphviz is missing!")
	}

	out := fmt.Sprintf("%v.png", filename)
	cmd := exec.Command(path, "-Tpng", fmt.Sprintf("-o%v", out), filename)

	if err1 == nil && err2 == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
	}
	_, err = cmd.Output()
	if err != nil {
		return errors.New("Error writing to image!")
	}
	return nil
}

// return an array (slice) of all directed vertices to vertex v (??? -> v)
// OKTimestamp should use this
func (g *Graph) IncomingGraphEdges(v *Vertex) []*Vertex {
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

// return an array (slice) of all vertices that vertex v points to (v -> ???)
// poke should use this
func (g *Graph) OutgoingGraphEdges(v *Vertex) []*Vertex {
	var s []*Vertex
	for k := range g.Adjacency[v] { // forward paths
		s = append(s, k)
	}
	return s
}

// return an array (slice) of all vertices that connect to vertex v
func (g *Graph) GraphEdges(v *Vertex) []*Vertex {
	var s []*Vertex
	s = append(s, g.IncomingGraphEdges(v)...)
	s = append(s, g.OutgoingGraphEdges(v)...)
	return s
}

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

			for _, w := range g.GraphEdges(v) {
				s = append(s, w)
			}
		}
	}
	return d
}

// build a new graph containing only vertices from the list...
func (g *Graph) FilterGraph(name string, vertices []*Vertex) *Graph {
	newgraph := NewGraph(name)
	for k1, x := range g.Adjacency {
		for k2, e := range x {
			//log.Printf("Filter: %v -> %v # %v", k1.Name, k2.Name, e.Name)
			if VertexContains(k1, vertices) || VertexContains(k2, vertices) {
				newgraph.AddEdge(k1, k2, e)
			}
		}
	}
	return newgraph
}

// return a channel containing the N disconnected graphs in our main graph
// we can then process each of these in parallel
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

// return the indegree for the graph, IOW the count of vertices that point to me
// NOTE: this returns the values for all vertices in one big lookup table
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

// return the outdegree for the graph, IOW the count of vertices that point away
// NOTE: this returns the values for all vertices in one big lookup table
func (g *Graph) OutDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)

	for k := range g.Adjacency {
		result[k] = 0 // initialize
		for _ = range g.Adjacency[k] {
			result[k]++
		}
	}
	return result
}

// returns a topological sort for the graph
// based on descriptions and code from wikipedia and rosetta code
// TODO: add memoization, and cache invalidation to speed this up :)
func (g *Graph) TopologicalSort() (result []*Vertex, ok bool) { // kahn's algorithm
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
					return nil, false // not a dag!
				}
			}
		}
	}

	return L, true
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
	vertices := g.OutgoingGraphEdges(a) // what points away from a ?
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

// VertexMerge merges v2 into v1 by reattaching the edges where appropriate,
// and then by deleting v2 from the graph. Since more than one edge between two
// vertices is not allowed, duplicate edges are merged as well. an edge merge
// function can be provided if you'd like to control how you merge the edges!
func (g *Graph) VertexMerge(v1, v2 *Vertex, vertexMergeFn func(*Vertex, *Vertex) (*Vertex, error), edgeMergeFn func(*Edge, *Edge) *Edge) error {
	// methodology
	// 1) edges between v1 and v2 are removed
	//Loop:
	for k1 := range g.Adjacency {
		for k2 := range g.Adjacency[k1] {
			// v1 -> v2 || v2 -> v1
			if (k1 == v1 && k2 == v2) || (k1 == v2 && k2 == v1) {
				delete(g.Adjacency[k1], k2) // delete map & edge
				// NOTE: if we assume this is a DAG, then we can
				// assume only v1 -> v2 OR v2 -> v1 exists, and
				// we can break out of these loops immediately!
				//break Loop
				break
			}
		}
	}

	// 2) edges that point towards v2 from X now point to v1 from X (no dupes)
	for _, x := range g.IncomingGraphEdges(v2) { // all to vertex v (??? -> v)
		e := g.Adjacency[x][v2] // previous edge
		r := g.Reachability(x, v1)
		// merge e with ex := g.Adjacency[x][v1] if it exists!
		if ex, exists := g.Adjacency[x][v1]; exists && edgeMergeFn != nil && len(r) == 0 {
			e = edgeMergeFn(e, ex)
		}
		if len(r) == 0 { // if not reachable, add it
			g.AddEdge(x, v1, e) // overwrite edge
		} else if edgeMergeFn != nil { // reachable, merge e through...
			prev := x // initial condition
			for i, next := range r {
				if i == 0 {
					// next == prev, therefore skip
					continue
				}
				// this edge is from: prev, to: next
				ex, _ := g.Adjacency[prev][next] // get
				ex = edgeMergeFn(ex, e)
				g.Adjacency[prev][next] = ex // set
				prev = next
			}
		}
		delete(g.Adjacency[x], v2) // delete old edge
	}

	// 3) edges that point from v2 to X now point from v1 to X (no dupes)
	for _, x := range g.OutgoingGraphEdges(v2) { // all from vertex v (v -> ???)
		e := g.Adjacency[v2][x] // previous edge
		r := g.Reachability(v1, x)
		// merge e with ex := g.Adjacency[v1][x] if it exists!
		if ex, exists := g.Adjacency[v1][x]; exists && edgeMergeFn != nil && len(r) == 0 {
			e = edgeMergeFn(e, ex)
		}
		if len(r) == 0 {
			g.AddEdge(v1, x, e) // overwrite edge
		} else if edgeMergeFn != nil { // reachable, merge e through...
			prev := v1 // initial condition
			for i, next := range r {
				if i == 0 {
					// next == prev, therefore skip
					continue
				}
				// this edge is from: prev, to: next
				ex, _ := g.Adjacency[prev][next]
				ex = edgeMergeFn(ex, e)
				g.Adjacency[prev][next] = ex
				prev = next
			}
		}
		delete(g.Adjacency[v2], x)
	}

	// 4) merge and then remove the (now merged/grouped) vertex
	if vertexMergeFn != nil { // run vertex merge function
		if v, err := vertexMergeFn(v1, v2); err != nil {
			return err
		} else if v != nil { // replace v1 with the "merged" version...
			v1 = v // XXX: will this replace v1 the way we want?
		}
	}
	g.DeleteVertex(v2) // remove grouped vertex

	// 5) creation of a cyclic graph should throw an error
	if _, dag := g.TopologicalSort(); !dag { // am i a dag or not?
		return fmt.Errorf("Graph is not a dag!")
	}
	return nil // success
}

func HeisenbergCount(ch chan *Vertex) int {
	c := 0
	for x := range ch {
		_ = x
		c++
	}
	return c
}

// GetTimestamp returns the timestamp of a vertex
func (v *Vertex) GetTimestamp() int64 {
	return v.timestamp
}

// UpdateTimestamp updates the timestamp on a vertex and returns the new value
func (v *Vertex) UpdateTimestamp() int64 {
	v.timestamp = time.Now().UnixNano() // update
	return v.timestamp
}

// can this element run right now?
func (g *Graph) OKTimestamp(v *Vertex) bool {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		// if the vertex has a greater timestamp than any pre-req (n)
		// then we can't run right now...
		// if they're equal (eg: on init of 0) then we also can't run
		// b/c we should let our pre-req's go first...
		x, y := v.GetTimestamp(), n.GetTimestamp()
		if DEBUG {
			log.Printf("%v[%v]: OKTimestamp: (%v) >= %v[%v](%v): !%v", v.Kind(), v.GetName(), x, n.Kind(), n.GetName(), y, x >= y)
		}
		if x >= y {
			return false
		}
	}
	return true
}

// notify nodes after me in the dependency graph that they need refreshing...
// NOTE: this assumes that this can never fail or need to be rescheduled
func (g *Graph) Poke(v *Vertex, activity bool) {
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphEdges(v) {
		// XXX: if we're in state event and haven't been cancelled by
		// apply, then we can cancel a poke to a child, right? XXX
		// XXX: if n.Res.GetState() != resStateEvent { // is this correct?
		if true { // XXX
			if DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			n.SendEvent(eventPoke, false, activity) // XXX: can this be switched to sync?
		} else {
			if DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
}

// poke the pre-requisites that are stale and need to run before I can run...
func (g *Graph) BackPoke(v *Vertex) {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		x, y, s := v.GetTimestamp(), n.GetTimestamp(), n.Res.GetState()
		// if the parent timestamp needs poking AND it's not in state
		// resStateEvent, then poke it. If the parent is in resStateEvent it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != resStateEvent && s != resStateCheckApply) {
			if DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			n.SendEvent(eventBackPoke, false, false) // XXX: can this be switched to sync?
		} else {
			if DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
}

// XXX: rename this function
func (g *Graph) Process(v *Vertex) {
	obj := v.Res
	if DEBUG {
		log.Printf("%v[%v]: Process()", obj.Kind(), obj.GetName())
	}
	obj.SetState(resStateEvent)
	var ok = true
	var apply = false // did we run an apply?
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if g.OKTimestamp(v) {
		if DEBUG {
			log.Printf("%v[%v]: OKTimestamp(%v)", obj.Kind(), obj.GetName(), v.GetTimestamp())
		}

		obj.SetState(resStateCheckApply)
		// if this fails, don't UpdateTimestamp()
		checkok, err := obj.CheckApply(!obj.GetMeta().Noop)
		if checkok && err != nil { // should never return this way
			log.Fatalf("%v[%v]: CheckApply(): %t, %+v", obj.Kind(), obj.GetName(), checkok, err)
		}
		if DEBUG {
			log.Printf("%v[%v]: CheckApply(): %t, %v", obj.Kind(), obj.GetName(), checkok, err)
		}

		if !checkok { // if state *was* not ok, we had to have apply'ed
			if err != nil { // error during check or apply
				ok = false
			} else {
				apply = true
			}
		}

		// when noop is true we always want to update timestamp
		if obj.GetMeta().Noop && err == nil {
			ok = true
		}

		if ok {
			// update this timestamp *before* we poke or the poked
			// nodes might fail due to having a too old timestamp!
			v.UpdateTimestamp()          // this was touched...
			obj.SetState(resStatePoking) // can't cancel parent poke
			g.Poke(v, apply)
		}
		// poke at our pre-req's instead since they need to refresh/run...
	} else {
		// only poke at the pre-req's that need to run
		go g.BackPoke(v)
	}
}

// main kick to start the graph
func (g *Graph) Start(wg *sync.WaitGroup, first bool) { // start or continue
	log.Printf("State: %v -> %v", g.SetState(graphStateStarting), g.GetState())
	defer log.Printf("State: %v -> %v", g.SetState(graphStateStarted), g.GetState())
	t, _ := g.TopologicalSort()
	// TODO: only calculate indegree if `first` is true to save resources
	indegree := g.InDegree() // compute all of the indegree's
	for _, v := range Reverse(t) {

		if !v.Res.IsWatching() { // if Watch() is not running...
			wg.Add(1)
			// must pass in value to avoid races...
			// see: https://ttboj.wordpress.com/2015/07/27/golang-parallelism-issues-causing-too-many-open-files-error/
			go func(vv *Vertex) {
				defer wg.Done()
				// listen for chan events from Watch() and run
				// the Process() function when they're received
				// this avoids us having to pass the data into
				// the Watch() function about which graph it is
				// running on, which isolates things nicely...
				chanProcess := make(chan Event)
				go func() {
					for event := range chanProcess {
						// this has to be synchronous,
						// because otherwise the Res
						// event loop will keep running
						// and change state, causing the
						// converged timeout to fire!
						g.Process(vv)
						event.ACK() // sync
					}
				}()
				vv.Res.Watch(chanProcess) // i block until i end
				close(chanProcess)
				log.Printf("%v[%v]: Exited", vv.Kind(), vv.GetName())
			}(v)
		}

		// selective poke: here we reduce the number of initial pokes
		// to the minimum required to activate every vertex in the
		// graph, either by direct action, or by getting poked by a
		// vertex that was previously activated. if we poke each vertex
		// that has no incoming edges, then we can be sure to reach the
		// whole graph. Please note: this may mask certain optimization
		// failures, such as any poke limiting code in Poke() or
		// BackPoke(). You might want to disable this selective start
		// when experimenting with and testing those elements.
		// if we are unpausing (since it's not the first run of this
		// function) we need to poke to *unpause* every graph vertex,
		// and not just selectively the subset with no indegree.
		if (!first) || indegree[v] == 0 {
			// ensure state is started before continuing on to next vertex
			for !v.SendEvent(eventStart, true, false) {
				if DEBUG {
					// if SendEvent fails, we aren't up yet
					log.Printf("%v[%v]: Retrying SendEvent(Start)", v.Kind(), v.GetName())
					// sleep here briefly or otherwise cause
					// a different goroutine to be scheduled
					time.Sleep(1 * time.Millisecond)
				}
			}
		}
	}
}

func (g *Graph) Pause() {
	log.Printf("State: %v -> %v", g.SetState(graphStatePausing), g.GetState())
	defer log.Printf("State: %v -> %v", g.SetState(graphStatePaused), g.GetState())
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		v.SendEvent(eventPause, true, false)
	}
}

func (g *Graph) Exit() {
	if g == nil {
		return
	} // empty graph that wasn't populated yet
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		// turn off the taps...
		// XXX: do this by sending an exit signal, and then returning
		// when we hit the 'default' in the select statement!
		// XXX: we can do this to quiesce, but it's not necessary now

		v.SendEvent(eventExit, true, false)
	}
}

// AssociateData associates some data with the object in the graph in question
func (g *Graph) AssociateData(converger Converger) {
	for v := range g.GetVerticesChan() {
		v.Res.AssociateData(converger)
	}
}

// in array function to test *Vertex in a slice of *Vertices
func VertexContains(needle *Vertex, haystack []*Vertex) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// reverse a list of vertices
func Reverse(vs []*Vertex) []*Vertex {
	//var out []*Vertex       // XXX: golint suggests, but it fails testing
	out := make([]*Vertex, 0) // empty list
	l := len(vs)
	for i := range vs {
		out = append(out, vs[l-i-1])
	}
	return out
}
