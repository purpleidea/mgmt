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

package pgraph

import (
	"fmt"
	"log"

	errwrap "github.com/pkg/errors"
)

// AutoGrouper is the required interface to implement for an autogroup algorithm
type AutoGrouper interface {
	// listed in the order these are typically called in...
	name() string                                  // friendly identifier
	init(*Graph) error                             // only call once
	vertexNext() (*Vertex, *Vertex, error)         // mostly algorithmic
	vertexCmp(*Vertex, *Vertex) error              // can we merge these ?
	vertexMerge(*Vertex, *Vertex) (*Vertex, error) // vertex merge fn to use
	edgeMerge(*Edge, *Edge) *Edge                  // edge merge fn to use
	vertexTest(bool) (bool, error)                 // call until false
}

// baseGrouper is the base type for implementing the AutoGrouper interface
type baseGrouper struct {
	graph    *Graph    // store a pointer to the graph
	vertices []*Vertex // cached list of vertices
	i        int
	j        int
	done     bool
}

// name provides a friendly name for the logs to see
func (ag *baseGrouper) name() string {
	return "baseGrouper"
}

// init is called only once and before using other AutoGrouper interface methods
// the name method is the only exception: call it any time without side effects!
func (ag *baseGrouper) init(g *Graph) error {
	if ag.graph != nil {
		return fmt.Errorf("The init method has already been called!")
	}
	ag.graph = g                               // pointer
	ag.vertices = ag.graph.GetVerticesSorted() // cache in deterministic order!
	ag.i = 0
	ag.j = 0
	if len(ag.vertices) == 0 { // empty graph
		ag.done = true
		return nil
	}
	return nil
}

// vertexNext is a simple iterator that loops through vertex (pair) combinations
// an intelligent algorithm would selectively offer only valid pairs of vertices
// these should satisfy logical grouping requirements for the autogroup designs!
// the desired algorithms can override, but keep this method as a base iterator!
func (ag *baseGrouper) vertexNext() (v1, v2 *Vertex, err error) {
	// this does a for v... { for w... { return v, w }} but stepwise!
	l := len(ag.vertices)
	if ag.i < l {
		v1 = ag.vertices[ag.i]
	}
	if ag.j < l {
		v2 = ag.vertices[ag.j]
	}

	// in case the vertex was deleted
	if !ag.graph.HasVertex(v1) {
		v1 = nil
	}
	if !ag.graph.HasVertex(v2) {
		v2 = nil
	}

	// two nested loops...
	if ag.j < l {
		ag.j++
	}
	if ag.j == l {
		ag.j = 0
		if ag.i < l {
			ag.i++
		}
		if ag.i == l {
			ag.done = true
		}
	}

	return
}

func (ag *baseGrouper) vertexCmp(v1, v2 *Vertex) error {
	if v1 == nil || v2 == nil {
		return fmt.Errorf("Vertex is nil!")
	}
	if v1 == v2 { // skip yourself
		return fmt.Errorf("Vertices are the same!")
	}
	if v1.Kind() != v2.Kind() { // we must group similar kinds
		// TODO: maybe future resources won't need this limitation?
		return fmt.Errorf("The two resources aren't the same kind!")
	}
	// someone doesn't want to group!
	if !v1.Meta().AutoGroup || !v2.Meta().AutoGroup {
		return fmt.Errorf("One of the autogroup flags is false!")
	}
	if v1.Res.IsGrouped() { // already grouped!
		return fmt.Errorf("Already grouped!")
	}
	if len(v2.Res.GetGroup()) > 0 { // already has children grouped!
		return fmt.Errorf("Already has groups!")
	}
	if !v1.Res.GroupCmp(v2.Res) { // resource groupcmp failed!
		return fmt.Errorf("The GroupCmp failed!")
	}
	return nil // success
}

func (ag *baseGrouper) vertexMerge(v1, v2 *Vertex) (v *Vertex, err error) {
	// NOTE: it's important to use w.Res instead of w, b/c
	// the w by itself is the *Vertex obj, not the *Res obj
	// which is contained within it! They both satisfy the
	// Res interface, which is why both will compile! :(
	err = v1.Res.GroupRes(v2.Res) // GroupRes skips stupid groupings
	return                        // success or fail, and no need to merge the actual vertices!
}

func (ag *baseGrouper) edgeMerge(e1, e2 *Edge) *Edge {
	return e1 // noop
}

// vertexTest processes the results of the grouping for the algorithm to know
// return an error if something went horribly wrong, and bool false to stop
func (ag *baseGrouper) vertexTest(b bool) (bool, error) {
	// NOTE: this particular baseGrouper version doesn't track what happens
	// because since we iterate over every pair, we don't care which merge!
	if ag.done {
		return false, nil
	}
	return true, nil
}

// TODO: this algorithm may not be correct in all cases. replace if needed!
type nonReachabilityGrouper struct {
	baseGrouper // "inherit" what we want, and reimplement the rest
}

func (ag *nonReachabilityGrouper) name() string {
	return "nonReachabilityGrouper"
}

// this algorithm relies on the observation that if there's a path from a to b,
// then they *can't* be merged (b/c of the existing dependency) so therefore we
// merge anything that *doesn't* satisfy this condition or that of the reverse!
func (ag *nonReachabilityGrouper) vertexNext() (v1, v2 *Vertex, err error) {
	for {
		v1, v2, err = ag.baseGrouper.vertexNext() // get all iterable pairs
		if err != nil {
			log.Fatalf("Error running autoGroup(vertexNext): %v", err)
		}

		if v1 != v2 { // ignore self cmp early (perf optimization)
			// if NOT reachable, they're viable...
			out1 := ag.graph.Reachability(v1, v2)
			out2 := ag.graph.Reachability(v2, v1)
			if len(out1) == 0 && len(out2) == 0 {
				return // return v1 and v2, they're viable
			}
		}

		// if we got here, it means we're skipping over this candidate!
		if ok, err := ag.baseGrouper.vertexTest(false); err != nil {
			log.Fatalf("Error running autoGroup(vertexTest): %v", err)
		} else if !ok {
			return nil, nil, nil // done!
		}

		// the vertexTest passed, so loop and try with a new pair...
	}
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
	for _, x := range g.IncomingGraphVertices(v2) { // all to vertex v (??? -> v)
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
	for _, x := range g.OutgoingGraphVertices(v2) { // all from vertex v (v -> ???)
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
			*v1 = *v // TODO: is this safe? (replacing mutexes is undefined!)
		}
	}
	g.DeleteVertex(v2) // remove grouped vertex

	// 5) creation of a cyclic graph should throw an error
	if _, err := g.TopologicalSort(); err != nil { // am i a dag or not?
		return errwrap.Wrapf(err, "TopologicalSort failed") // not a dag
	}
	return nil // success
}

// autoGroup is the mechanical auto group "runner" that runs the interface spec
func (g *Graph) autoGroup(ag AutoGrouper) chan string {
	strch := make(chan string) // output log messages here
	go func(strch chan string) {
		strch <- fmt.Sprintf("Compile: Grouping: Algorithm: %v...", ag.name())
		if err := ag.init(g); err != nil {
			log.Fatalf("Error running autoGroup(init): %v", err)
		}

		for {
			var v, w *Vertex
			v, w, err := ag.vertexNext() // get pair to compare
			if err != nil {
				log.Fatalf("Error running autoGroup(vertexNext): %v", err)
			}
			merged := false
			// save names since they change during the runs
			vStr := fmt.Sprintf("%s", v) // valid even if it is nil
			wStr := fmt.Sprintf("%s", w)

			if err := ag.vertexCmp(v, w); err != nil { // cmp ?
				if g.Flags.Debug {
					strch <- fmt.Sprintf("Compile: Grouping: !GroupCmp for: %s into %s", wStr, vStr)
				}

				// remove grouped vertex and merge edges (res is safe)
			} else if err := g.VertexMerge(v, w, ag.vertexMerge, ag.edgeMerge); err != nil { // merge...
				strch <- fmt.Sprintf("Compile: Grouping: !VertexMerge for: %s into %s", wStr, vStr)

			} else { // success!
				strch <- fmt.Sprintf("Compile: Grouping: Success for: %s into %s", wStr, vStr)
				merged = true // woo
			}

			// did these get used?
			if ok, err := ag.vertexTest(merged); err != nil {
				log.Fatalf("Error running autoGroup(vertexTest): %v", err)
			} else if !ok {
				break // done!
			}
		}

		close(strch)
		return
	}(strch) // call function
	return strch
}

// AutoGroup runs the auto grouping on the graph and prints out log messages
func (g *Graph) AutoGroup() {
	// receive log messages from channel...
	// this allows test cases to avoid printing them when they're unwanted!
	// TODO: this algorithm may not be correct in all cases. replace if needed!
	for str := range g.autoGroup(&nonReachabilityGrouper{}) {
		log.Println(str)
	}
}
