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
	"strconv"
	"sync"
	"syscall"
)

//go:generate stringer -type=graphState -output=graphstate_stringer.go
type graphState int

const (
	graphNil graphState = iota
	graphStarting
	graphStarted
	graphPausing
	graphPaused
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
	//Directed  bool
}

type Vertex struct {
	graph *Graph            // store a pointer to the graph it's on
	Type                    // anonymous field
	data  map[string]string // XXX: currently unused i think, remove?
}

type Edge struct {
	Name string
}

func NewGraph(name string) *Graph {
	return &Graph{
		Name:      name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge),
		state:     graphNil,
	}
}

func NewVertex(t Type) *Vertex {
	return &Vertex{
		Type: t,
		data: make(map[string]string),
	}
}

func NewEdge(name string) *Edge {
	return &Edge{
		Name: name,
	}
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
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.state
}

func (g *Graph) SetState(state graphState) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.state = state
}

// store a pointer in the type to it's parent vertex
func (g *Graph) SetVertex() {
	for v := range g.GetVerticesChan() {
		v.Type.SetVertex(v)
	}
}

// add a new vertex to the graph
func (g *Graph) AddVertex(v *Vertex) {
	if _, exists := g.Adjacency[v]; !exists {
		g.Adjacency[v] = make(map[*Vertex]*Edge)

		// store a pointer to the graph it's on for convenience and readability
		v.graph = g
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
	// TODO: is this a problem?
	g.AddVertex(v1)
	g.AddVertex(v2)
	g.Adjacency[v1][v2] = e
}

// XXX: does it make sense to return a channel here?
// GetVertex finds the vertex in the graph with a particular search name
func (g *Graph) GetVertex(name string) chan *Vertex {
	ch := make(chan *Vertex, 1)
	go func(name string) {
		for k := range g.Adjacency {
			if k.GetName() == name {
				ch <- k
				break
			}
		}
		close(ch)
	}(name)
	return ch
}

func (g *Graph) GetVertexMatch(obj Type) *Vertex {
	for k := range g.Adjacency {
		if k.Compare(obj) { // XXX test
			return k
		}
	}
	return nil
}

func (g *Graph) HasVertex(v *Vertex) bool {
	if _, exists := g.Adjacency[v]; exists {
		return true
	}
	//for k := range g.Adjacency {
	//	if k == v {
	//		return true
	//	}
	//}
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

// get an array (slice) of all vertices in the graph
func (g *Graph) GetVertices() []*Vertex {
	vertices := make([]*Vertex, 0)
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

// make the graph pretty print
func (g *Graph) String() string {
	return fmt.Sprintf("Vertices(%d), Edges(%d)", g.NumVertices(), g.NumEdges())
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
	for i, _ := range g.Adjacency { // reverse paths
		out += fmt.Sprintf("\t%v [label=\"%v[%v]\"];\n", i.GetName(), i.GetType(), i.GetName())
		for j, _ := range g.Adjacency[i] {
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

// google/golang hackers apparently do not think contains should be a built-in!
func Contains(s []*Vertex, element *Vertex) bool {
	for _, v := range s {
		if element == v {
			return true
		}
	}
	return false
}

// return an array (slice) of all directed vertices to vertex v (??? -> v)
// ostimestamp should use this
func (g *Graph) IncomingGraphEdges(v *Vertex) []*Vertex {
	// TODO: we might be able to implement this differently by reversing
	// the Adjacency graph and then looping through it again...
	s := make([]*Vertex, 0)
	for k, _ := range g.Adjacency { // reverse paths
		for w, _ := range g.Adjacency[k] {
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
	s := make([]*Vertex, 0)
	for k, _ := range g.Adjacency[v] { // forward paths
		s = append(s, k)
	}
	return s
}

// return an array (slice) of all vertices that connect to vertex v
func (g *Graph) GraphEdges(v *Vertex) []*Vertex {
	s := make([]*Vertex, 0)
	s = append(s, g.IncomingGraphEdges(v)...)
	s = append(s, g.OutgoingGraphEdges(v)...)
	return s
}

func (g *Graph) DFS(start *Vertex) []*Vertex {
	d := make([]*Vertex, 0) // discovered
	s := make([]*Vertex, 0) // stack
	if _, exists := g.Adjacency[start]; !exists {
		return nil // TODO: error
	}
	v := start
	s = append(s, v)
	for len(s) > 0 {

		v, s = s[len(s)-1], s[:len(s)-1] // s.pop()

		if !Contains(d, v) { // if not discovered
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

			//fmt.Printf("Filter: %v -> %v # %v\n", k1.Name, k2.Name, e.Name)
			if Contains(vertices, k1) || Contains(vertices, k2) {
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
		d := make([]*Vertex, 0) // discovered
		c := g.NumVertices()
		for len(d) < c {

			// get an undiscovered vertex to start from
			for _, s := range g.GetVertices() {
				if !Contains(d, s) {
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

// return the indegree for the graph
func (g *Graph) InDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)
	for k := range g.Adjacency {
		result[k] = 0 // initialize
	}

	for k := range g.Adjacency {
		for z := range g.Adjacency[k] {
			result[z] += 1
		}
	}
	return result
}

// return the outdegree for the graph
func (g *Graph) OutDegree() map[*Vertex]int {
	result := make(map[*Vertex]int)

	for k := range g.Adjacency {
		result[k] = 0 // initialize
		for _ = range g.Adjacency[k] {
			result[k] += 1
		}
	}
	return result
}

// returns a topological sort for the graph
// based on descriptions and code from wikipedia and rosetta code
// TODO: add memoization, and cache invalidation to speed this up :)
func (g *Graph) TopologicalSort() (result []*Vertex, ok bool) { // kahn's algorithm

	L := make([]*Vertex, 0)            // empty list that will contain the sorted elements
	S := make([]*Vertex, 0)            // set of all nodes with no incoming edges
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
		for n, _ := range g.Adjacency[v] {
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
			for n, _ := range g.Adjacency[c] {
				if remaining[n] > 0 {
					return nil, false // not a dag!
				}
			}
		}
	}

	return L, true
}

func (v *Vertex) Value(key string) (string, bool) {
	if value, exists := v.data[key]; exists {
		return value, true
	}
	return "", false
}

func (v *Vertex) SetValue(key, value string) bool {
	v.data[key] = value
	return true
}

func (g *Graph) GetVerticesKeyValue(key, value string) chan *Vertex {
	ch := make(chan *Vertex)
	go func() {
		for vertex := range g.GetVerticesChan() {
			if v, exists := vertex.Value(key); exists && v == value {
				ch <- vertex
			}
		}
		close(ch)
	}()
	return ch
}

// return a pointer to the graph a vertex is on
func (v *Vertex) GetGraph() *Graph {
	return v.graph
}

func HeisenbergCount(ch chan *Vertex) int {
	c := 0
	for x := range ch {
		_ = x
		c++
	}
	return c
}

// main kick to start the graph
func (g *Graph) Start(wg *sync.WaitGroup) { // start or continue
	t, _ := g.TopologicalSort()
	for _, v := range Reverse(t) {

		if !v.Type.IsWatching() { // if Watch() is not running...
			wg.Add(1)
			// must pass in value to avoid races...
			// see: https://ttboj.wordpress.com/2015/07/27/golang-parallelism-issues-causing-too-many-open-files-error/
			go func(vv *Vertex) {
				defer wg.Done()
				vv.Type.Watch()
				log.Printf("Finish: %v", vv.GetName())
			}(v)
		}

		// ensure state is started before continuing on to next vertex
		v.Type.SendEvent(eventStart, true)

	}
}

func (g *Graph) Pause() {
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		v.Type.SendEvent(eventPause, true)
	}
}

func (g *Graph) Exit() {
	t, _ := g.TopologicalSort()
	for _, v := range t { // squeeze out the events...
		// turn off the taps...
		// XXX: do this by sending an exit signal, and then returning
		// when we hit the 'default' in the select statement!
		// XXX: we can do this to quiesce, but it's not necessary now

		v.Type.SendEvent(eventExit, true)
	}
}

func (g *Graph) SetConvergedCallback(ctimeout int, converged chan bool) {
	for v := range g.GetVerticesChan() {
		v.Type.SetConvegedCallback(ctimeout, converged)
	}
}

// in array function to test *vertices in a slice of *vertices
func HasVertex(v *Vertex, haystack []*Vertex) bool {
	for _, r := range haystack {
		if v == r {
			return true
		}
	}
	return false
}

// reverse a list of vertices
func Reverse(vs []*Vertex) []*Vertex {
	out := make([]*Vertex, 0) // empty list
	l := len(vs)
	for i := range vs {
		out = append(out, vs[l-i-1])
	}
	return out
}
