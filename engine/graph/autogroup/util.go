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

package autogroup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// VertexMerge merges v2 into v1 by reattaching the edges where appropriate, and
// then by deleting v2 from the graph. Since more than one edge between two
// vertices is not allowed, duplicate edges are merged as well. An edge merge
// function can be provided if you'd like to control how you merge the edges!
func VertexMerge(g *pgraph.Graph, v1, v2 pgraph.Vertex, vertexMergeFn func(pgraph.Vertex, pgraph.Vertex) (pgraph.Vertex, error), edgeMergeFn func(pgraph.Edge, pgraph.Edge) pgraph.Edge) error {
	// methodology
	// 1) edges between v1 and v2 are removed
	//Loop:
	for k1 := range g.Adjacency() {
		for k2 := range g.Adjacency()[k1] {
			// v1 -> v2 || v2 -> v1
			if (k1 == v1 && k2 == v2) || (k1 == v2 && k2 == v1) {
				delete(g.Adjacency()[k1], k2) // delete map & edge
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
		e := g.Adjacency()[x][v2] // previous edge
		r, err := g.Reachability(x, v1)
		if err != nil {
			return err
		}
		// merge e with ex := g.Adjacency()[x][v1] if it exists!
		if ex, exists := g.Adjacency()[x][v1]; exists && edgeMergeFn != nil && len(r) == 0 {
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
				ex, _ := g.Adjacency()[prev][next] // get
				ex = edgeMergeFn(ex, e)
				g.Adjacency()[prev][next] = ex // set
				prev = next
			}
		}
		delete(g.Adjacency()[x], v2) // delete old edge
	}

	// 3) edges that point from v2 to X now point from v1 to X (no dupes)
	for _, x := range g.OutgoingGraphVertices(v2) { // all from vertex v (v -> ???)
		e := g.Adjacency()[v2][x] // previous edge
		r, err := g.Reachability(v1, x)
		if err != nil {
			return err
		}
		// merge e with ex := g.Adjacency()[v1][x] if it exists!
		if ex, exists := g.Adjacency()[v1][x]; exists && edgeMergeFn != nil && len(r) == 0 {
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
				ex, _ := g.Adjacency()[prev][next]
				ex = edgeMergeFn(ex, e)
				g.Adjacency()[prev][next] = ex
				prev = next
			}
		}
		delete(g.Adjacency()[v2], x)
	}

	// 4) merge and then remove the (now merged/grouped) vertex
	if vertexMergeFn != nil { // run vertex merge function
		if v, err := vertexMergeFn(v1, v2); err != nil {
			return err
		} else if v != nil { // replace v1 with the "merged" version...
			// note: This branch isn't used if the vertexMergeFn
			// decides to just merge logically on its own instead
			// of actually returning something that we then merge.
			v1 = v // XXX: ineffassign?
			//*v1 = *v

			// Ensure that everything still validates. (For safety!)
			r, ok := v1.(engine.Res) // TODO: v ?
			if !ok {
				return fmt.Errorf("not a Res")
			}
			if err := engine.Validate(r); err != nil {
				return errwrap.Wrapf(err, "the Res did not Validate")
			}
		}
	}
	g.DeleteVertex(v2) // remove grouped vertex

	// 5) creation of a cyclic graph should throw an error
	if _, err := g.TopologicalSort(); err != nil { // am i a dag or not?
		return errwrap.Wrapf(err, "the TopologicalSort failed") // not a dag
	}
	return nil // success
}

// RHVSlice is a linear list of vertices. It can be sorted by the Kind, taking
// into account the hierarchy of names separated by colons. Afterwards, it uses
// String() to avoid the non-determinism in the map type. RHV stands for Reverse
// Hierarchical Vertex, meaning the hierarchical topology of the vertex
// (resource) names are used.
type RHVSlice []pgraph.Vertex

// Len returns the length of the slice of vertices.
func (obj RHVSlice) Len() int { return len(obj) }

// Swap swaps two elements in the slice.
func (obj RHVSlice) Swap(i, j int) { obj[i], obj[j] = obj[j], obj[i] }

// Less returns the smaller element in the sort order according to the
// aforementioned rules.
// XXX: Add some tests to make sure I didn't get any "reverse" part backwards.
func (obj RHVSlice) Less(i, j int) bool {
	resi, oki := obj[i].(engine.Res)
	resj, okj := obj[j].(engine.Res)
	if !oki || !okj || resi.Kind() == "" || resj.Kind() == "" {
		// One of these isn't a normal Res, so just compare normally.
		return obj[i].String() > obj[j].String() // reverse
	}

	si := strings.Split(resi.Kind(), ":")
	sj := strings.Split(resj.Kind(), ":")
	// both lengths should each be at least one now
	li := len(si)
	lj := len(sj)

	if li != lj { // eg: http:ui vs. http:ui:text
		return li > lj // reverse
	}

	// same number of chunks
	for k := 0; k < li; k++ {
		if si[k] != sj[k] { // lhs chunk differs
			return si[k] > sj[k] // reverse
		}

		// if the chunks are the same, we continue...
	}

	// They must all have the same chunks, so finally we compare the names.
	return resi.Name() > resj.Name() // reverse
}

// Sort is a convenience method.
func (obj RHVSlice) Sort() { sort.Sort(obj) }

// RHVSort returns a deterministically sorted slice of all vertices in the list.
// The order is sorted by the Kind, taking into account the hierarchy of names
// separated by colons. Afterwards, it uses String() to avoid the
// non-determinism in the map type. RHV stands for Reverse Hierarchical Vertex,
// meaning the hierarchical topology of the vertex (resource) names are used.
func RHVSort(vertices []pgraph.Vertex) []pgraph.Vertex {
	var vs []pgraph.Vertex
	for _, v := range vertices { // copy first
		vs = append(vs, v)
	}
	sort.Sort(RHVSlice(vs)) // add determinism
	return vs
}
