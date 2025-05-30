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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

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
			result := addEdgesByMatchingUIDS(res, uids, graph, debug, logf)

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
func addEdgesByMatchingUIDS(res engine.EdgeableRes, uids []engine.ResUID, graph *pgraph.Graph, debug bool, logf func(format string, v ...interface{})) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uid, and see if it matches any vertex
	for _, uid := range uids {
		var found = false
		// uid is a ResUID object
		for _, v := range graph.Vertices() { // search
			r, ok := v.(engine.EdgeableRes)
			if !ok {
				continue
			}
			if r.AutoEdgeMeta().Disabled { // skip if this res is disabled
				continue
			}
			if res == r { // skip self
				continue
			}
			if debug {
				logf("match: %s with UID: %s", r, uid)
			}
			// we must match to an effective UID for the resource,
			// that is to say, the name value of a res is a helpful
			// handle, but it is not necessarily a unique identity!
			// remember, resources can return multiple UID's each!
			if UIDExistsInUIDs(uid, r.UIDs()) {
				// add edge from: r -> res
				if uid.IsReversed() {
					txt := fmt.Sprintf("%s -> %s", r, res)
					logf("adding: %s", txt)
					edge := &engine.Edge{Name: txt}
					graph.AddEdge(r, res, edge)
				} else { // edges go the "normal" way, eg: pkg resource
					txt := fmt.Sprintf("%s -> %s", res, r)
					logf("adding: %s", txt)
					edge := &engine.Edge{Name: txt}
					graph.AddEdge(res, r, edge)
				}
				found = true
				break
			}
		}
		result = append(result, found)
	}
	return result
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
