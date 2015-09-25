// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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
	"code.google.com/p/go-uuid/uuid"
	//"container/list" // doubly linked list
	"fmt"
	"log"
	"sync"
	"time"
)

// The graph abstract data type (ADT) is defined as follows:
// NOTE: the directed graph arrows point from left to right ( --> )
// NOTE: the arrows point towards their dependencies (eg: arrows mean requires)
type Graph struct {
	uuid      string
	Name      string
	Adjacency map[*Vertex]map[*Vertex]*Edge
	//Directed  bool
	startcount int
}

type Vertex struct {
	uuid      string
	graph     *Graph // store a pointer to the graph it's on
	Name      string
	Type      string
	Timestamp int64       // last updated timestamp ?
	Events    chan string // FIXME: eventually a struct for the event?
	Typedata  Type
	data      map[string]string
}

type Edge struct {
	uuid string
	Name string
}

func NewGraph(name string) *Graph {
	return &Graph{
		uuid:      uuid.New(),
		Name:      name,
		Adjacency: make(map[*Vertex]map[*Vertex]*Edge),
	}
}

func NewVertex(name, t string) *Vertex {
	return &Vertex{
		uuid:      uuid.New(),
		Name:      name,
		Type:      t,
		Timestamp: -1,
		Events:    make(chan string, 1), // XXX: chan size?
		data:      make(map[string]string),
	}
}

func NewEdge(name string) *Edge {
	return &Edge{
		uuid: uuid.New(),
		Name: name,
	}
}

// Graph() creates a new, empty graph.
// addVertex(vert) adds an instance of Vertex to the graph.
func (g *Graph) AddVertex(v *Vertex) {
	if _, exists := g.Adjacency[v]; !exists {
		g.Adjacency[v] = make(map[*Vertex]*Edge)

		// store a pointer to the graph it's on for convenience and readability
		v.graph = g
	}
}

// addEdge(fromVert, toVert) Adds a new, directed edge to the graph that connects two vertices.
func (g *Graph) AddEdge(v1, v2 *Vertex, e *Edge) {
	// NOTE: this doesn't allow more than one edge between two vertexes...
	// TODO: is this a problem?
	g.AddVertex(v1)
	g.AddVertex(v2)
	g.Adjacency[v1][v2] = e
}

// addEdge(fromVert, toVert, weight) Adds a new, weighted, directed edge to the graph that connects two vertices.
// getVertex(vertKey) finds the vertex in the graph named vertKey.
func (g *Graph) GetVertex(uuid string) chan *Vertex {
	ch := make(chan *Vertex, 1)
	go func(uuid string) {
		for k := range g.Adjacency {
			v := *k
			if v.uuid == uuid {
				ch <- k
				break
			}
		}
		close(ch)
	}(uuid)
	return ch
}

func (g *Graph) NumVertices() int {
	return len(g.Adjacency)
}

func (g *Graph) NumEdges() int {
	// XXX: not implemented
	return -1
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
	// TODO: do you need to pass this through into the go routine?
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

//func (s []*Vertex) contains(element *Vertex) bool {
// google/golang hackers apparently do not think contains should be a built-in!
func Contains(s []*Vertex, element *Vertex) bool {
	for _, v := range s {
		if element == v {
			return true
		}
	}
	return false
}

// return an array (slice) of all vertices that connect to vertex v
func (g *Graph) GraphEdges(vertex *Vertex) []*Vertex {
	// TODO: we might be able to implement this differently by reversing
	// the Adjacency graph and then looping through it again...
	s := make([]*Vertex, 0)                 // stack
	for w, _ := range g.Adjacency[vertex] { // forward paths
		//fmt.Printf("forward: %v -> %v\n", v.Name, w.Name)
		s = append(s, w)
	}

	for k, x := range g.Adjacency { // reverse paths
		for w, _ := range x {
			if w == vertex {
				//fmt.Printf("reverse: %v -> %v\n", v.Name, k.Name)
				s = append(s, k)
			}
		}
	}
	return s
}

// return an array (slice) of all directed vertices to vertex v
func (g *Graph) DirectedGraphEdges(vertex *Vertex) []*Vertex {
	// TODO: we might be able to implement this differently by reversing
	// the Adjacency graph and then looping through it again...
	s := make([]*Vertex, 0)                 // stack
	for w, _ := range g.Adjacency[vertex] { // forward paths
		//fmt.Printf("forward: %v -> %v\n", v.Name, w.Name)
		s = append(s, w)
	}
	return s
}

// get timestamp of a vertex
func (v *Vertex) GetTimestamp() int64 {
	return v.Timestamp
}

// update timestamp of a vertex
func (v *Vertex) UpdateTimestamp() int64 {
	v.Timestamp = time.Now().UnixNano() // update
	return v.Timestamp
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

func (v *Vertex) Associate(t Type) {
	v.Typedata = t
}

func (v *Vertex) OKTimestamp() bool {
	g := v.GetGraph()
	for _, n := range g.DirectedGraphEdges(v) {
		if v.GetTimestamp() > n.GetTimestamp() {
			return false
		}
	}

	return true
}

// poke the XXX children?
func (v *Vertex) Poke() {
	g := v.GetGraph()

	for _, n := range g.DirectedGraphEdges(v) { // XXX: do we want the reverse order?
		// poke!
		n.Events <- fmt.Sprintf("poke(%v)", v.Name)
	}
}

func (g *Graph) Exit() {
	// tell all the vertices to exit...
	for v := range g.GetVerticesChan() {
		v.Exit()
	}
}

func (v *Vertex) Exit() {
	v.Events <- "exit"
}

// main loop for each vertex
// warning: this logic might be subtle and tricky.
// be careful as it might not even be correct now!
func (v *Vertex) Start() {
	log.Printf("Main->Vertex[%v]->Start()\n", v.Name)

	//g := v.GetGraph()
	var t = v.Typedata

	// this whole wg2 wait group is only necessary if we need to wait for
	// the go routine to exit...
	var wg2 sync.WaitGroup

	wg2.Add(1)
	go func(v *Vertex, t Type) {
		defer wg2.Done()
		//fmt.Printf("About to watch [%v].\n", v.Name)
		t.Watch(v)
	}(v, t)

	var ok bool
	//XXX make sure dependencies run and become more current first...
	for {
		select {
		case event := <-v.Events:

			log.Printf("Event[%v]: %v\n", v.Name, event)

			if event == "exit" {
				t.Exit()   // type exit
				wg2.Wait() // wait for worker to exit
				return
			}

			ok = true
			if v.OKTimestamp() {
				if !t.StateOK() { // TODO: can we rename this to something better?
					// throw an error if apply fails...
					// if this fails, don't UpdateTimestamp()
					if !t.Apply() { // check for error
						ok = false
					}
				}

				if ok {
					v.UpdateTimestamp() // this was touched...
					v.Poke()            // XXX
				}
			}
		}
	}
}
