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
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// reachCache stores a precomputed transitive closure for O(1) reachability
// queries. It is built from a single TopologicalSort and rebuilt after each
// VertexMerge.
type reachCache struct {
	reachable map[pgraph.Vertex]map[pgraph.Vertex]bool
}

// buildReachCache constructs the transitive closure of the graph. It performs
// one TopologicalSort, then processes vertices in reverse topological order so
// that each vertex's reachable set is the union of its direct successors'
// reachable sets plus the successors themselves.
func buildReachCache(g *pgraph.Graph) (*reachCache, error) {
	topo, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	rc := &reachCache{
		reachable: make(map[pgraph.Vertex]map[pgraph.Vertex]bool, len(topo)),
	}

	// Process in reverse topological order so successors are done first.
	for i := len(topo) - 1; i >= 0; i-- {
		v := topo[i]
		s := make(map[pgraph.Vertex]bool)
		for _, w := range g.OutgoingGraphVertices(v) {
			s[w] = true
			for r := range rc.reachable[w] {
				s[r] = true
			}
		}
		rc.reachable[v] = s
	}

	return rc, nil
}

// isReachable returns true if there is a path from a to b.
func (rc *reachCache) isReachable(a, b pgraph.Vertex) bool {
	if s, ok := rc.reachable[a]; ok {
		return s[b]
	}
	return false
}

// kindFamily returns the "grouping family" key for a vertex. Vertices in the
// same family are candidates for grouping. For hierarchical kinds separated by
// colons (e.g. "http:server", "http:server:file", "dhcp:server", "dhcp:host"),
// the family is the first colon-separated segment. For simple kinds (no colon)
// the family is the kind itself.
func kindFamily(v pgraph.Vertex) string {
	r, ok := v.(engine.Res)
	if !ok {
		return v.String()
	}
	kind := r.Kind()
	if i := strings.IndexByte(kind, ':'); i >= 0 {
		return kind[:i]
	}
	return kind
}

// NonReachabilityFastGrouper is an optimized version of NonReachabilityGrouper.
// It uses kind-family partitioning so only same-family pairs are compared, and
// a precomputed reachability cache for O(1) reachability checks instead of
// recursive DFS on every pair.
type NonReachabilityFastGrouper struct {
	baseGrouper // "inherit" what we want, and reimplement the rest

	// families holds vertices partitioned by kind family, each sub-slice
	// in RHVSort order.
	families [][]pgraph.Vertex

	// fi and fj are the current family index and inner pair indices.
	fi int
	pi int // outer index within current family
	pj int // inner index within current family

	cache *reachCache
	done  bool
}

// Name returns the name for the grouper algorithm.
func (ag *NonReachabilityFastGrouper) Name() string {
	return "NonReachabilityFastGrouper"
}

// Init builds the kind-family partitions and the reachability cache.
func (ag *NonReachabilityFastGrouper) Init(g *pgraph.Graph) error {
	if err := ag.baseGrouper.Init(g); err != nil {
		return err
	}

	// Build the reachability cache.
	rc, err := buildReachCache(g)
	if err != nil {
		return err
	}
	ag.cache = rc

	// Partition vertices by kind family.
	familyMap := make(map[string][]pgraph.Vertex)
	for _, v := range ag.baseGrouper.vertices {
		f := kindFamily(v)
		familyMap[f] = append(familyMap[f], v)
	}

	ag.families = [][]pgraph.Vertex{}
	for _, verts := range familyMap {
		// Keep RHVSort order within each family so hierarchical
		// grouping still works correctly (longer kinds first).
		sorted := RHVSort(verts)
		ag.families = append(ag.families, sorted)
	}

	ag.fi = 0
	ag.pi = 0
	ag.pj = 0
	ag.done = len(ag.families) == 0

	return nil
}

// familyNext advances through pairs within families. Returns nil, nil when all
// pairs are exhausted.
func (ag *NonReachabilityFastGrouper) familyNext() (pgraph.Vertex, pgraph.Vertex) {
	for ag.fi < len(ag.families) {
		fam := ag.families[ag.fi]
		l := len(fam)
		if ag.pi < l && ag.pj < l {
			v1 := fam[ag.pi]
			v2 := fam[ag.pj]

			// Advance indices (nested loop within family).
			ag.pj++
			if ag.pj == l {
				ag.pj = 0
				ag.pi++
				if ag.pi == l {
					// Move to next family.
					ag.fi++
					ag.pi = 0
					ag.pj = 0
				}
			}

			// Skip vertices that were deleted from the graph.
			if !ag.graph.HasVertex(v1) || !ag.graph.HasVertex(v2) {
				continue
			}
			return v1, v2
		}
		// Move to next family.
		ag.fi++
		ag.pi = 0
		ag.pj = 0
	}
	return nil, nil
}

// VertexNext iteratively finds vertex pairs with simple graph reachability...
// This algorithm relies on the observation that if there's a path from a to b,
// then they *can't* be merged (b/c of the existing dependency) so therefore we
// merge anything that *doesn't* satisfy this condition or that of the reverse!
func (ag *NonReachabilityFastGrouper) VertexNext() (v1, v2 pgraph.Vertex, err error) {
	for !ag.done {
		v1, v2 = ag.familyNext()
		if v1 == nil && v2 == nil {
			ag.done = true
			return nil, nil, nil // done!
		}

		// ignore self cmp early (perf optimization)
		if v1 != v2 {
			// O(1) reachability check via precomputed cache.
			if !ag.cache.isReachable(v1, v2) && !ag.cache.isReachable(v2, v1) {
				return // return v1 and v2, they're viable
			}
		}

		// if we got here, it means we're skipping over this candidate!
		if ok, err := ag.baseGrouper.VertexTest(false); err != nil {
			return nil, nil, errwrap.Wrapf(err, "error running autoGroup(vertexTest)")
		} else if !ok {
			return nil, nil, nil // done!
		}

		// the vertexTest passed, so loop and try with a new pair...
	}
	return nil, nil, nil
}

// VertexTest processes the results of the grouping. When a merge happens, we
// rebuild the reachability cache (graph has shrunk, so it's cheaper) and
// restart the family iteration from the beginning.
func (ag *NonReachabilityFastGrouper) VertexTest(b bool) (bool, error) {
	if b {
		// A merge happened. Rebuild the cache for the updated graph.
		rc, err := buildReachCache(ag.graph)
		if err != nil {
			return false, err
		}
		ag.cache = rc

		// Rebuild family partitions with current graph vertices.
		familyMap := make(map[string][]pgraph.Vertex)
		for _, v := range ag.graph.Vertices() {
			f := kindFamily(v)
			familyMap[f] = append(familyMap[f], v)
		}
		ag.families = [][]pgraph.Vertex{}
		for _, verts := range familyMap {
			sorted := RHVSort(verts)
			ag.families = append(ag.families, sorted)
		}

		// Restart iteration from the beginning after a merge.
		ag.fi = 0
		ag.pi = 0
		ag.pj = 0
		ag.done = len(ag.families) == 0
		return !ag.done, nil
	}

	// No merge happened, just continue if not done.
	if ag.done {
		return false, nil
	}
	return true, nil
}
