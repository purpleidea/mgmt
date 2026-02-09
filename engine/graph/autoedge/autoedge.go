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

package autoedge

import (
	"fmt"
	"reflect"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// vertexInfo holds pre-computed information about a vertex for use during edge
// matching. This avoids recomputing UIDs() on every comparison.
type vertexInfo struct {
	res  engine.EdgeableRes
	uids []engine.ResUID
}

// edgeMatcher holds the pre-computed state needed for matching UIDs to
// vertices. It is built once per AutoEdge run and used for all matching calls,
// avoiding the need to pass many parameters to each call.
type edgeMatcher struct {
	// verticesInfo is the pre-computed list of non-disabled EdgeableRes
	// vertices and their UIDs.
	verticesInfo []vertexInfo

	// uidTypeIndex maps concrete UID types to the vertices that produce
	// UIDs of that type, for fast candidate narrowing.
	uidTypeIndex map[reflect.Type][]vertexInfo

	// graph is the graph being modified.
	graph *pgraph.Graph

	// adj is a reference to the graph's adjacency map, obtained once so
	// that edges added during the run are visible to later iterations.
	adj map[pgraph.Vertex]map[pgraph.Vertex]pgraph.Edge

	debug bool
	logf  func(format string, v ...interface{})
}

// AutoEdge adds the automatic edges to the graph.
func AutoEdge(graph *pgraph.Graph, debug bool, logf func(format string, v ...interface{})) error {
	logf("building...")

	// initially get all of the autoedges to seek out all possible errors
	var err error
	autoEdgeObjMap := make(map[engine.EdgeableRes]engine.AutoEdge)
	sorted := []engine.EdgeableRes{}
	for _, v := range graph.VerticesSorted() {
		res, ok := v.(engine.EdgeableRes)
		if !ok {
			continue
		}
		if res.AutoEdgeMeta().Disabled { // skip if this res is disabled
			continue
		}
		sorted = append(sorted, res)
	}

	for _, res := range sorted { // for each vertices autoedges
		autoEdgeObj, e := res.AutoEdges()
		if e != nil {
			err = errwrap.Append(err, e) // collect all errors
			continue
		}
		if autoEdgeObj == nil {
			if debug {
				logf("no auto edges were found for: %s", res)
			}
			continue // next vertex
		}
		autoEdgeObjMap[res] = autoEdgeObj // save for next loop
	}
	if err != nil {
		return errwrap.Wrapf(err, "the auto edges had errors")
	}

	// Pre-compute vertex info once so we don't call UIDs() on each
	// comparison. This turns the per-UID cost from O(n) interface calls
	// into a single slice lookup.
	verticesInfo := make([]vertexInfo, 0, len(sorted))
	for _, res := range sorted {
		verticesInfo = append(verticesInfo, vertexInfo{
			res:  res,
			uids: res.UIDs(),
		})
	}

	// Build an index from concrete UID type to the vertices that produce
	// UIDs of that type. Every real IFF() implementation starts with a
	// type assertion that returns false across kinds, so we can skip
	// candidates whose UID types don't overlap with the seeking UID.
	uidTypeIndex := make(map[reflect.Type][]vertexInfo)
	for _, vi := range verticesInfo {
		seen := make(map[reflect.Type]struct{})
		for _, u := range vi.uids {
			t := reflect.TypeOf(u)
			if _, ok := seen[t]; ok {
				continue // don't add same vertex twice per type
			}
			seen[t] = struct{}{}
			uidTypeIndex[t] = append(uidTypeIndex[t], vi)
		}
	}

	m := &edgeMatcher{
		verticesInfo: verticesInfo,
		uidTypeIndex: uidTypeIndex,
		graph:        graph,
		// Get a reference to the adjacency map once. Since
		// Adjacency() returns a direct reference, it reflects edges
		// added during this run, allowing later iterations to see
		// earlier autoedges.
		adj:   graph.Adjacency(),
		debug: debug,
		logf:  logf,
	}

	// now that we're guaranteed error free, we can modify the graph safely
	for _, res := range sorted { // stable sort order for determinism in logs
		autoEdgeObj, exists := autoEdgeObjMap[res]
		if !exists {
			continue
		}

		for { // while the autoEdgeObj has more uids to add...
			uids := autoEdgeObj.Next() // get some!
			if uids == nil {
				logf("the auto edge list is empty for: %s", res)
				break // inner loop
			}
			if debug {
				logf("UIDS:")
				for i, u := range uids {
					logf("UID%d: %v", i, u)
				}
			}

			// match and add edges
			result := m.addEdgesByMatchingUIDS(res, uids)

			// report back, and find out if we should continue
			if !autoEdgeObj.Test(result) {
				break
			}
		}
	}

	// It would be great to ensure we didn't add any graph cycles here, but
	// instead of checking now, we'll move the check into the main loop.
	return nil
}

// addEdgesByMatchingUIDS adds edges to the vertex in a graph based on if it
// matches a uid list. It uses the pre-computed verticesInfo and uidTypeIndex to
// avoid recomputing UIDs and to narrow the search to vertices that share a
// concrete UID type with the seeking UID. Before adding an edge, it checks
// whether the edge already exists or whether the target is already reachable
// through existing edges, to avoid redundant edges that would reduce
// parallelism.
func (obj *edgeMatcher) addEdgesByMatchingUIDS(res engine.EdgeableRes, uids []engine.ResUID) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uid, and see if it matches any vertex
	for _, uid := range uids {
		var found = false
		// uid is a ResUID object

		// Look up candidates by UID type. Since IFF() across
		// different concrete types always returns false, we only
		// need to check vertices that produce UIDs of the same
		// type. Fall back to the full list if the type is unknown
		// (e.g. when using BaseUID directly without overriding).
		candidates, ok := obj.uidTypeIndex[reflect.TypeOf(uid)]
		if !ok {
			candidates = obj.verticesInfo
		}

		for _, vi := range candidates { // search
			if vi.res.AutoEdgeMeta().Disabled {
				continue // skip disabled targets
			}
			if res == vi.res { // skip self
				continue
			}
			if obj.debug {
				obj.logf("match: %s with UID: %s", vi.res, uid)
			}
			// we must match to an effective UID for the resource,
			// that is to say, the name value of a res is a helpful
			// handle, but it is not necessarily a unique identity!
			// remember, resources can return multiple UID's each!
			if UIDExistsInUIDs(uid, vi.uids) {
				// determine edge direction
				var from, to pgraph.Vertex
				if uid.IsReversed() {
					from, to = vi.res, res
				} else {
					from, to = res, vi.res
				}

				// Skip if the edge already exists (O(1)
				// check) or the target is already reachable
				// through existing edges (directed DFS).
				// This avoids redundant transitive edges
				// that waste memory and reduce parallelism.
				if obj.graph.FindEdge(from, to) != nil {
					if obj.debug {
						obj.logf("skip existing: %s -> %s", from, to)
					}
				} else if isReachable(obj.adj, from, to) {
					if obj.debug {
						obj.logf("skip reachable: %s -> %s", from, to)
					}
				} else {
					txt := fmt.Sprintf("%s -> %s", from, to)
					obj.logf("adding: %s", txt)
					edge := &engine.Edge{Name: txt}
					obj.graph.AddEdge(from, to, edge)
				}
				found = true
				break
			}
		}
		result = append(result, found)
	}
	return result
}

// isReachable returns true if there is a directed path from vertex a to vertex
// b in the graph. It performs an iterative DFS following outgoing edges only.
// The visited set ensures termination even if the graph contains cycles.
func isReachable(adj map[pgraph.Vertex]map[pgraph.Vertex]pgraph.Edge, a, b pgraph.Vertex) bool {
	visited := make(map[pgraph.Vertex]struct{})
	stack := []pgraph.Vertex{a}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := visited[v]; ok {
			continue
		}
		visited[v] = struct{}{}
		for next := range adj[v] {
			if next == b {
				return true
			}
			stack = append(stack, next)
		}
	}
	return false
}

// UIDExistsInUIDs wraps the IFF method when used with a list of UID's.
func UIDExistsInUIDs(uid engine.ResUID, uids []engine.ResUID) bool {
	for _, u := range uids {
		if uid.IFF(u) {
			return true
		}
	}
	return false
}
