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

package graph

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autoedge"
	"github.com/purpleidea/mgmt/pgraph"
)

// autoEdgeCacheEntry stores the fingerprint and the edges that were added by
// the previous AutoEdge run, so that we can replay them cheaply when the
// pre-autoedge graph hasn't changed.
type autoEdgeCacheEntry struct {
	fingerprint string
	edges       []autoEdgeCachedEdge
}

// autoEdgeCachedEdge records a single edge that AutoEdge added, identified by
// the String() representations of its endpoints and the edge name.
type autoEdgeCachedEdge struct {
	from string // vertex String(), e.g. "file[/tmp/foo]"
	to   string // vertex String()
	name string // edge name (the "from -> to" text)
}

// computeAutoEdgeFingerprint builds a deterministic string from the
// pre-autoedge graph state. It captures vertex identities, their UIDs,
// AutoEdgeMeta, and explicit edges. If any of these change, the fingerprint
// changes and the cache is invalidated.
//
// NOTE: This does not capture the output of AutoEdges() (the seeking UIDs) or
// the IsReversed() field on UIDs, because both are determined by the resource
// Kind which is already present in the vertex and UID strings. If a future
// resource derives seeking UIDs or edge direction from its configuration rather
// than its Kind, the fingerprint must be extended.
func computeAutoEdgeFingerprint(graph *pgraph.Graph) string {
	var b strings.Builder

	vertices := graph.VerticesSorted()

	// Vertices section: identity, UIDs, and disabled flag. Each value
	// is length-prefixed to prevent delimiter injection from vertex or
	// UID strings that happen to contain our separators.
	for _, v := range vertices {
		s := v.String()
		fmt.Fprintf(&b, "v:%d:%s", len(s), s)

		res, ok := v.(engine.EdgeableRes)
		if ok {
			// Collect UID strings sorted for determinism.
			uids := res.UIDs()
			uidStrs := make([]string, len(uids))
			for i, u := range uids {
				uidStrs[i] = u.String()
			}
			sort.Strings(uidStrs)
			for _, us := range uidStrs {
				fmt.Fprintf(&b, ",u:%d:%s", len(us), us)
			}

			if res.AutoEdgeMeta().Disabled {
				b.WriteString(",d:true")
			}
		}
		b.WriteByte('\n')
	}

	// Edges section: sorted by from+to for determinism.
	type edgeKey struct {
		from, to string
	}
	var edgeKeys []edgeKey
	adj := graph.Adjacency()
	for v1, m := range adj {
		for v2 := range m {
			edgeKeys = append(edgeKeys, edgeKey{
				from: v1.String(),
				to:   v2.String(),
			})
		}
	}
	sort.Slice(edgeKeys, func(i, j int) bool {
		if edgeKeys[i].from != edgeKeys[j].from {
			return edgeKeys[i].from < edgeKeys[j].from
		}
		return edgeKeys[i].to < edgeKeys[j].to
	})
	for _, ek := range edgeKeys {
		// Length-prefix edge endpoint strings too.
		fmt.Fprintf(&b, "e:%d:%s->%d:%s\n",
			len(ek.from), ek.from,
			len(ek.to), ek.to,
		)
	}

	return b.String()
}

// replayAutoEdges adds the cached edges to the graph. It validates all vertices
// exist before adding any edges, so that a lookup failure cannot leave the
// graph in a partially-modified state.
func replayAutoEdges(graph *pgraph.Graph, edges []autoEdgeCachedEdge) error {
	// Build a lookup map from String() to vertex.
	lookup := make(map[string]pgraph.Vertex)
	for _, v := range graph.Vertices() {
		lookup[v.String()] = v
	}

	// Pre-validate: check that every referenced vertex exists before
	// we modify the graph, to avoid partial replay on failure.
	for _, e := range edges {
		if _, ok := lookup[e.from]; !ok {
			return fmt.Errorf(
				"stale cache: vertex %s not found",
				strconv.Quote(e.from),
			)
		}
		if _, ok := lookup[e.to]; !ok {
			return fmt.Errorf(
				"stale cache: vertex %s not found",
				strconv.Quote(e.to),
			)
		}
	}

	for _, e := range edges {
		from := lookup[e.from]
		to := lookup[e.to]
		graph.AddEdge(from, to, &engine.Edge{Name: e.name})
	}
	return nil
}

// snapshotEdges captures the current edge set as a set of (from, to) string
// pairs. This is used to diff against the post-AutoEdge state to find which
// edges were added.
func snapshotEdges(graph *pgraph.Graph) map[[2]string]struct{} {
	result := make(map[[2]string]struct{})
	for v1, m := range graph.Adjacency() {
		for v2 := range m {
			result[[2]string{v1.String(), v2.String()}] = struct{}{}
		}
	}
	return result
}

// AutoEdge adds the automatic edges to the graph. It caches the results from
// the previous run keyed by a fingerprint of the pre-autoedge graph state. When
// the graph hasn't changed, cached edges are replayed instead of running the
// full autoedge algorithm.
func (obj *Engine) AutoEdge() error {
	logf := func(format string, v ...interface{}) {
		obj.Logf("autoedge: "+format, v...)
	}

	fp := computeAutoEdgeFingerprint(obj.nextGraph)
	if c := obj.autoEdgeCache; c != nil && c.fingerprint == fp {
		logf("replaying %d cached edge(s)...", len(c.edges))
		if err := replayAutoEdges(obj.nextGraph, c.edges); err == nil {
			return nil
		}
		// Replay failed (stale vertex), fall through to full run.
		// Because replayAutoEdges pre-validates, the graph is still
		// unmodified here so the full run starts clean.
		logf("cache replay failed, running full computation...")
	}

	before := snapshotEdges(obj.nextGraph)

	if err := autoedge.AutoEdge(obj.nextGraph, obj.Debug, logf); err != nil {
		return err
	}

	// Diff edges to find what AutoEdge added. Use a type assertion
	// to read the Name field directly rather than relying on the
	// String() interface matching the Name.
	var added []autoEdgeCachedEdge
	for v1, m := range obj.nextGraph.Adjacency() {
		for v2, e := range m {
			key := [2]string{v1.String(), v2.String()}
			if _, existed := before[key]; !existed {
				ee, ok := e.(*engine.Edge)
				if !ok {
					return fmt.Errorf(
						"edge %s->%s is not *engine.Edge",
						key[0], key[1],
					)
				}
				added = append(added, autoEdgeCachedEdge{
					from: key[0],
					to:   key[1],
					name: ee.Name,
				})
			}
		}
	}

	// Sort cached edges for deterministic ordering across runs,
	// since map iteration order is non-deterministic.
	sort.Slice(added, func(i, j int) bool {
		if added[i].from != added[j].from {
			return added[i].from < added[j].from
		}
		return added[i].to < added[j].to
	})

	obj.autoEdgeCache = &autoEdgeCacheEntry{
		fingerprint: fp,
		edges:       added,
	}
	return nil
}
