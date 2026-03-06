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

// vertexInfo caches the EdgeableRes reference and its UIDs to avoid repeated
// interface assertions and slice allocations in the inner loop.
type vertexInfo struct {
	res  engine.EdgeableRes
	uids []engine.ResUID
}

// edgeMatcher holds the pre-computed state for UID matching. It is built once
// per AutoEdge run and used for all matching operations.
type edgeMatcher struct {
	graph     *pgraph.Graph
	adjacency map[pgraph.Vertex]map[pgraph.Vertex]pgraph.Edge

	// uidIndex maps each concrete UID type to the vertices that have UIDs
	// of that type. This narrows the candidate set for IFF matching since
	// every IFF implementation starts with a type assertion that returns
	// false for different concrete types.
	uidIndex map[reflect.Type][]vertexInfo

	// allInfo is the full list of vertex info, used as a fallback when a
	// UID type is not in the index (e.g. a custom resource using BaseUID).
	allInfo []vertexInfo

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

	// Pre-compute vertex info and build the UID type index so that the
	// inner matching loop can avoid repeated Vertices(), type assertion,
	// and UIDs() calls.
	allInfo := make([]vertexInfo, 0, len(sorted))
	uidIndex := make(map[reflect.Type][]vertexInfo)
	for _, res := range sorted {
		uids := res.UIDs()
		vi := vertexInfo{res: res, uids: uids}
		allInfo = append(allInfo, vi)
		for _, uid := range uids {
			t := reflect.TypeOf(uid)
			uidIndex[t] = append(uidIndex[t], vi)
		}
	}

	matcher := &edgeMatcher{
		graph:     graph,
		adjacency: graph.Adjacency(),
		uidIndex:  uidIndex,
		allInfo:   allInfo,
		debug:     debug,
		logf:      logf,
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
			result := matcher.addEdgesByMatchingUIDS(res, uids)

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
// matches a uid list.
func (obj *edgeMatcher) addEdgesByMatchingUIDS(res engine.EdgeableRes, uids []engine.ResUID) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uid, and see if it matches any vertex
	for _, uid := range uids {
		var found = false

		// Use the type index to narrow candidates. Every IFF in the
		// codebase starts with a type assertion that returns false for
		// different concrete types, so only candidates sharing the
		// same concrete UID type can match.
		t := reflect.TypeOf(uid)
		candidates, ok := obj.uidIndex[t]
		if !ok {
			// Unindexed type (e.g. custom resource using BaseUID
			// directly); fall back to checking all vertices.
			candidates = obj.allInfo
		}

		for _, vi := range candidates {
			if vi.res == res { // skip self
				continue
			}
			if vi.res.AutoEdgeMeta().Disabled { // skip if disabled
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
				var from, to pgraph.Vertex
				// add edge from: r -> res
				if uid.IsReversed() {
					from = vi.res
					to = res
				} else { // edges go the "normal" way, eg: pkg resource
					from = res
					to = vi.res
				}

				// Skip if this exact edge already exists.
				if obj.graph.FindEdge(from, to) != nil {
					found = true
					break
				}

				// Skip if the destination is already reachable
				// from the source through existing edges, since
				// that would be a redundant transitive edge.
				if obj.isReachable(from, to, 10, 100) {
					found = true
					break
				}

				txt := fmt.Sprintf("%s -> %s", from, to)
				obj.logf("adding: %s", txt)
				edge := &engine.Edge{Name: txt}
				obj.graph.AddEdge(from, to, edge)
				found = true
				break
			}
		}
		result = append(result, found)
	}
	return result
}

// isReachable does a bounded BFS on the graph adjacency to check if dst is
// reachable from src through existing outgoing edges. It stops after maxDepth
// levels or maxVisit vertices to avoid expensive searches in very large or
// cyclic graphs.
func (obj *edgeMatcher) isReachable(src, dst pgraph.Vertex, maxDepth, maxVisit int) bool {
	visited := make(map[pgraph.Vertex]struct{})
	current := []pgraph.Vertex{src}
	visited[src] = struct{}{}
	totalVisited := 1

	for depth := 0; depth < maxDepth && len(current) > 0; depth++ {
		next := []pgraph.Vertex{}
		for _, v := range current {
			neighbors, exists := obj.adjacency[v]
			if !exists {
				continue
			}
			for n := range neighbors {
				if n == dst {
					return true
				}
				if _, seen := visited[n]; seen {
					continue
				}
				visited[n] = struct{}{}
				totalVisited++
				if totalVisited >= maxVisit {
					return false
				}
				next = append(next, n)
			}
		}
		current = next
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
