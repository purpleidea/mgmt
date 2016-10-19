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
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/event"
	"github.com/purpleidea/mgmt/global"
	"github.com/purpleidea/mgmt/resources"
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
	Name      string
	Adjacency map[*Vertex]map[*Vertex]*Edge // *Vertex -> *Vertex (edge)
	state     graphState
	mutex     sync.Mutex // used when modifying graph State variable
}

// Vertex is the primary vertex struct in this library.
type Vertex struct {
	resources.Res       // anonymous field
	timestamp     int64 // last updated timestamp ?
}

// Edge is the primary edge struct in this library.
type Edge struct {
	Name string
}

// NewGraph builds a new graph.
func NewGraph(name string) *Graph {
	return &Graph{
		Name:      name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge),
		state:     graphStateNil,
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

// GetVertexMatch searches for an equivalent resource in the graph and returns
// the vertex it is found in, or nil if not found.
func (g *Graph) GetVertexMatch(obj resources.Res) *Vertex {
	for k := range g.Adjacency {
		if k.Res.Compare(obj) {
			return k
		}
	}
	return nil
}

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

// Graphviz outputs the graph in graphviz format.
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

// ExecGraphviz writes out the graphviz data and runs the correct graphviz
// filter command.
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

// IncomingGraphEdges returns an array (slice) of all directed vertices to
// vertex v (??? -> v). OKTimestamp should probably use this.
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

// OutgoingGraphEdges returns an array (slice) of all vertices that vertex v
// points to (v -> ???). Poke should probably use this.
func (g *Graph) OutgoingGraphEdges(v *Vertex) []*Vertex {
	var s []*Vertex
	for k := range g.Adjacency[v] { // forward paths
		s = append(s, k)
	}
	return s
}

// GraphEdges returns an array (slice) of all vertices that connect to vertex v.
// This is the union of IncomingGraphEdges and OutgoingGraphEdges.
func (g *Graph) GraphEdges(v *Vertex) []*Vertex {
	var s []*Vertex
	s = append(s, g.IncomingGraphEdges(v)...)
	s = append(s, g.OutgoingGraphEdges(v)...)
	return s
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

			for _, w := range g.GraphEdges(v) {
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
			//log.Printf("Filter: %v -> %v # %v", k1.Name, k2.Name, e.Name)
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

// GetTimestamp returns the timestamp of a vertex
func (v *Vertex) GetTimestamp() int64 {
	return v.timestamp
}

// UpdateTimestamp updates the timestamp on a vertex and returns the new value
func (v *Vertex) UpdateTimestamp() int64 {
	v.timestamp = time.Now().UnixNano() // update
	return v.timestamp
}

// OKTimestamp returns true if this element can run right now?
func (g *Graph) OKTimestamp(v *Vertex) bool {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		// if the vertex has a greater timestamp than any pre-req (n)
		// then we can't run right now...
		// if they're equal (eg: on init of 0) then we also can't run
		// b/c we should let our pre-req's go first...
		x, y := v.GetTimestamp(), n.GetTimestamp()
		if global.DEBUG {
			log.Printf("%v[%v]: OKTimestamp: (%v) >= %v[%v](%v): !%v", v.Kind(), v.GetName(), x, n.Kind(), n.GetName(), y, x >= y)
		}
		if x >= y {
			return false
		}
	}
	return true
}

// Poke notifies nodes after me in the dependency graph that they need refreshing...
// NOTE: this assumes that this can never fail or need to be rescheduled
func (g *Graph) Poke(v *Vertex, activity bool) {
	// these are all the vertices pointing AWAY FROM v, eg: v -> ???
	for _, n := range g.OutgoingGraphEdges(v) {
		// XXX: if we're in state event and haven't been cancelled by
		// apply, then we can cancel a poke to a child, right? XXX
		// XXX: if n.Res.getState() != resources.ResStateEvent { // is this correct?
		if true { // XXX
			if global.DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			n.SendEvent(event.EventPoke, false, activity) // XXX: can this be switched to sync?
		} else {
			if global.DEBUG {
				log.Printf("%v[%v]: Poke: %v[%v]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
}

// BackPoke pokes the pre-requisites that are stale and need to run before I can run.
func (g *Graph) BackPoke(v *Vertex) {
	// these are all the vertices pointing TO v, eg: ??? -> v
	for _, n := range g.IncomingGraphEdges(v) {
		x, y, s := v.GetTimestamp(), n.GetTimestamp(), n.Res.GetState()
		// if the parent timestamp needs poking AND it's not in state
		// ResStateEvent, then poke it. If the parent is in ResStateEvent it
		// means that an event is pending, so we'll be expecting a poke
		// back soon, so we can safely discard the extra parent poke...
		// TODO: implement a stateLT (less than) to tell if something
		// happens earlier in the state cycle and that doesn't wrap nil
		if x >= y && (s != resources.ResStateEvent && s != resources.ResStateCheckApply) {
			if global.DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
			n.SendEvent(event.EventBackPoke, false, false) // XXX: can this be switched to sync?
		} else {
			if global.DEBUG {
				log.Printf("%v[%v]: BackPoke: %v[%v]: Skipped!", v.Kind(), v.GetName(), n.Kind(), n.GetName())
			}
		}
	}
}

// Process is the primary function to execute for a particular vertex in the graph.
func (g *Graph) Process(v *Vertex) error {
	obj := v.Res
	if global.DEBUG {
		log.Printf("%v[%v]: Process()", obj.Kind(), obj.GetName())
	}
	obj.SetState(resources.ResStateEvent)
	var ok = true
	var apply = false // did we run an apply?
	// is it okay to run dependency wise right now?
	// if not, that's okay because when the dependency runs, it will poke
	// us back and we will run if needed then!
	if g.OKTimestamp(v) {
		if global.DEBUG {
			log.Printf("%v[%v]: OKTimestamp(%v)", obj.Kind(), obj.GetName(), v.GetTimestamp())
		}

		obj.SetState(resources.ResStateCheckApply)
		// if this fails, don't UpdateTimestamp()
		checkok, err := obj.CheckApply(!obj.Meta().Noop)
		if checkok && err != nil { // should never return this way
			log.Fatalf("%v[%v]: CheckApply(): %t, %+v", obj.Kind(), obj.GetName(), checkok, err)
		}
		if global.DEBUG {
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
		if obj.Meta().Noop && err == nil {
			ok = true
		}

		if ok {
			// update this timestamp *before* we poke or the poked
			// nodes might fail due to having a too old timestamp!
			v.UpdateTimestamp()                    // this was touched...
			obj.SetState(resources.ResStatePoking) // can't cancel parent poke
			g.Poke(v, apply)
		}
		// poke at our pre-req's instead since they need to refresh/run...
		return err
	}
	// else... only poke at the pre-req's that need to run
	go g.BackPoke(v)
	return nil
}

// SentinelErr is a sentinal as an error type that wraps an arbitrary error.
type SentinelErr struct {
	err error
}

// Error is the required method to fulfill the error type.
func (obj *SentinelErr) Error() string {
	return obj.err.Error()
}

// Worker is the common run frontend of the vertex. It handles all of the retry
// and retry delay common code, and ultimately returns the final status of this
// vertex execution.
func (g *Graph) Worker(v *Vertex) error {
	// listen for chan events from Watch() and run
	// the Process() function when they're received
	// this avoids us having to pass the data into
	// the Watch() function about which graph it is
	// running on, which isolates things nicely...
	obj := v.Res
	chanProcess := make(chan event.Event)
	go func() {
		running := false
		var timer = time.NewTimer(time.Duration(math.MaxInt64)) // longest duration
		if !timer.Stop() {
			<-timer.C // unnecessary, shouldn't happen
		}
		var delay = time.Duration(v.Meta().Delay) * time.Millisecond
		var retry = v.Meta().Retry // number of tries left, -1 for infinite
		var saved event.Event
	Loop:
		for {
			// this has to be synchronous, because otherwise the Res
			// event loop will keep running and change state,
			// causing the converged timeout to fire!
			select {
			case event, ok := <-chanProcess: // must use like this
				if running && ok {
					// we got an event that wasn't a close,
					// while we were waiting for the timer!
					// if this happens, it might be a bug:(
					log.Fatalf("%v[%v]: Worker: Unexpected event: %+v", v.Kind(), v.GetName(), event)
				}
				if !ok { // chanProcess closed, let's exit
					break Loop // no event, so no ack!
				}

				// the above mentioned synchronous part, is the
				// running of this function, paired with an ack.
				if e := g.Process(v); e != nil {
					saved = event
					log.Printf("%v[%v]: CheckApply errored: %v", v.Kind(), v.GetName(), e)
					if retry == 0 {
						// wrap the error in the sentinel
						event.ACKNACK(&SentinelErr{e}) // fail the Watch()
						break Loop
					}
					if retry > 0 { // don't decrement the -1
						retry--
					}
					log.Printf("%v[%v]: CheckApply: Retrying after %.4f seconds (%d left)", v.Kind(), v.GetName(), delay.Seconds(), retry)
					// start the timer...
					timer.Reset(delay)
					running = true
					continue
				}
				retry = v.Meta().Retry // reset on success
				event.ACK()            // sync

			case <-timer.C:
				if !timer.Stop() {
					//<-timer.C // blocks, docs are wrong!
				}
				running = false
				log.Printf("%s[%s]: CheckApply delay expired!", v.Kind(), v.GetName())
				// re-send this failed event, to trigger a CheckApply()
				go func() { chanProcess <- saved }()
				// TODO: should we send a fake event instead?
				//saved = nil
			}
		}
	}()
	var err error // propagate the error up (this is a permanent BAD error!)
	// the watch delay runs inside of the Watch resource loop, so that it
	// can still process signals and exit if needed. It shouldn't run any
	// resource specific code since this is supposed to be a retry delay.
	// NOTE: we're using the same retry and delay metaparams that CheckApply
	// uses. This is for practicality. We can separate them later if needed!
	var watchDelay time.Duration
	var watchRetry = v.Meta().Retry // number of tries left, -1 for infinite
	// watch blocks until it ends, & errors to retry
	for {
		// TODO: do we have to stop the converged-timeout when in this block (perhaps we're in the delay block!)
		// TODO: should we setup/manage some of the converged timeout stuff in here anyways?

		// if a retry-delay was requested, wait, but don't block our events!
		if watchDelay > 0 {
			//var pendingSendEvent bool
			timer := time.NewTimer(watchDelay)
		Loop:
			for {
				select {
				case <-timer.C: // the wait is over
					break Loop // critical

				// TODO: resources could have a separate exit channel to avoid this complexity!?
				case event := <-obj.Events():
					// NOTE: this code should match the similar Res code!
					//cuid.SetConverged(false) // TODO: ?
					if exit, send := obj.ReadEvent(&event); exit {
						return nil // exit
					} else if send {
						// if we dive down this rabbit hole, our
						// timer.C won't get seen until we get out!
						// in this situation, the Watch() is blocked
						// from performing until CheckApply returns
						// successfully, or errors out. This isn't
						// so bad, but we should document it. Is it
						// possible that some resource *needs* Watch
						// to run to be able to execute a CheckApply?
						// That situation shouldn't be common, and
						// should probably not be allowed. Can we
						// avoid it though?
						//if exit, err := doSend(); exit || err != nil {
						//	return err // we exit or bubble up a NACK...
						//}
						// Instead of doing the above, we can
						// add events to a pending list, and
						// when we finish the delay, we can run
						// them.
						//pendingSendEvent = true // all events are identical for now...
					}
				}
			}
			timer.Stop() // it's nice to cleanup
			log.Printf("%s[%s]: Watch delay expired!", v.Kind(), v.GetName())
			// NOTE: we can avoid the send if running Watch guarantees
			// one CheckApply event on startup!
			//if pendingSendEvent { // TODO: should this become a list in the future?
			//	if exit, err := obj.DoSend(chanProcess, ""); exit || err != nil {
			//		return err // we exit or bubble up a NACK...
			//	}
			//}
		}

		// TODO: reset the watch retry count after some amount of success
		e := v.Res.Watch(chanProcess)
		if e == nil { // exit signal
			err = nil // clean exit
			break
		}
		if sentinelErr, ok := e.(*SentinelErr); ok { // unwrap the sentinel
			err = sentinelErr.err
			break // sentinel means, perma-exit
		}
		log.Printf("%v[%v]: Watch errored: %v", v.Kind(), v.GetName(), e)
		if watchRetry == 0 {
			err = fmt.Errorf("Permanent watch error: %v", e)
			break
		}
		if watchRetry > 0 { // don't decrement the -1
			watchRetry--
		}
		watchDelay = time.Duration(v.Meta().Delay) * time.Millisecond
		log.Printf("%v[%v]: Watch: Retrying after %.4f seconds (%d left)", v.Kind(), v.GetName(), watchDelay.Seconds(), watchRetry)
		// We need to trigger a CheckApply after Watch restarts, so that
		// we catch any lost events that happened while down. We do this
		// by getting the Watch resource to send one event once it's up!
		//v.SendEvent(eventPoke, false, false)
	}
	close(chanProcess)
	return err
}

// Start is a main kick to start the graph. It goes through in reverse topological
// sort order so that events can't hit un-started vertices.
func (g *Graph) Start(wg *sync.WaitGroup, first bool) { // start or continue
	log.Printf("State: %v -> %v", g.setState(graphStateStarting), g.getState())
	defer log.Printf("State: %v -> %v", g.setState(graphStateStarted), g.getState())
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
				// TODO: if a sufficient number of workers error,
				// should something be done? Will these restart
				// after perma-failure if we have a graph change?
				if err := g.Worker(vv); err != nil { // contains the Watch and CheckApply loops
					log.Printf("%s[%s]: Exited with failure: %v", vv.Kind(), vv.GetName(), err)
					return
				}
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
			for !v.SendEvent(event.EventStart, true, false) {
				if global.DEBUG {
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

// Pause sends pause events to the graph in a topological sort order.
func (g *Graph) Pause() {
	log.Printf("State: %v -> %v", g.setState(graphStatePausing), g.getState())
	defer log.Printf("State: %v -> %v", g.setState(graphStatePaused), g.getState())
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		v.SendEvent(event.EventPause, true, false)
	}
}

// Exit sends exit events to the graph in a topological sort order.
func (g *Graph) Exit() {
	if g == nil {
		return
	} // empty graph that wasn't populated yet
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		// turn off the taps...
		// XXX: consider instead doing this by closing the Res.events channel instead?
		// XXX: do this by sending an exit signal, and then returning
		// when we hit the 'default' in the select statement!
		// XXX: we can do this to quiesce, but it's not necessary now

		v.SendEvent(event.EventExit, true, false)
	}
}

// AssociateData associates some data with the object in the graph in question
func (g *Graph) AssociateData(converger converger.Converger) {
	for v := range g.GetVerticesChan() {
		v.Res.AssociateData(converger)
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
