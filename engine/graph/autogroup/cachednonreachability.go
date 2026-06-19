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

	"github.com/purpleidea/mgmt/pgraph"
)

// CachedNonReachabilityGrouper is a cached reachability algorithm for grouping.
// TODO: this algorithm may not be correct in all cases. replace if needed!
type CachedNonReachabilityGrouper struct {
	baseGrouper // "inherit" what we want, and reimplement the rest

	// descendants caches the set of vertices reachable from a vertex. The
	// graph topology only changes when a merge happens, so the cache lives
	// between merges and gets flushed in VertexTest after each one.
	descendants map[pgraph.Vertex]map[pgraph.Vertex]struct{}
}

// Name returns the name for the grouper algorithm.
func (obj *CachedNonReachabilityGrouper) Name() string {
	return "CachedNonReachabilityGrouper"
}

// VertexViable checks pairs with simple graph reachability... This algorithm
// relies on the observation that if there's a path from a to b, then they
// *can't* be merged (b/c of the existing dependency) so therefore we merge
// anything that *doesn't* satisfy this condition or that of the reverse! The
// candidate pairs come unfiltered from the base VertexNext iterator, and the
// main loop runs this check only for pairs which already passed the much
// cheaper VertexCmp, instead of doing this traversal for every single pair.
func (obj *CachedNonReachabilityGrouper) VertexViable(v1, v2 pgraph.Vertex) error {
	if err := obj.baseGrouper.VertexViable(v1, v2); err != nil {
		return err
	}

	// If NOT reachable, they're viable. Unlike the old Reachability calls,
	// this doesn't re-validate the whole graph as a DAG with a topological
	// sort each time. The graph is a DAG on entry, and merging mutually
	// unreachable vertices keeps it one, so that check was redundant here.
	if obj.reachable(v1, v2) || obj.reachable(v2, v1) {
		return fmt.Errorf("the two vertices are reachable from each other")
	}

	return nil // viable!
}

// reachable returns whether there's a path from a to b, computing and caching
// the full descendant set of a on first use, so checking the N candidate pairs
// between merges costs one traversal per source vertex instead of one per pair.
// XXX: implement caching and flushing in pgraph instead and use that elsewhere?
func (obj *CachedNonReachabilityGrouper) reachable(a, b pgraph.Vertex) bool {
	if obj.descendants == nil {
		obj.descendants = make(map[pgraph.Vertex]map[pgraph.Vertex]struct{})
	}
	d, exists := obj.descendants[a]
	if !exists {
		d = make(map[pgraph.Vertex]struct{})
		adjacency := obj.graph.Adjacency() // read-only
		stack := []pgraph.Vertex{a}
		for len(stack) > 0 {
			last := len(stack) - 1
			v := stack[last]
			stack = stack[:last]
			for n := range adjacency[v] {
				if _, ok := d[n]; ok {
					continue
				}
				d[n] = struct{}{}
				stack = append(stack, n)
			}
		}
		obj.descendants[a] = d
	}
	_, ok := d[b]
	return ok
}

// VertexTest processes the results of the grouping. A successful merge changed
// the graph topology, which invalidates the cached reachability, so flush it.
func (obj *CachedNonReachabilityGrouper) VertexTest(b bool) (bool, error) {
	if b {
		obj.descendants = nil // flush the cache
	}
	return obj.baseGrouper.VertexTest(b)
}
