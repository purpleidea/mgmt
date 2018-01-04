// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// The AutoEdge interface is used to implement the autoedges feature.
type AutoEdge interface {
	Next() []ResUID   // call to get list of edges to add
	Test([]bool) bool // call until false
}

// UIDExistsInUIDs wraps the IFF method when used with a list of UID's.
func UIDExistsInUIDs(uid ResUID, uids []ResUID) bool {
	for _, u := range uids {
		if uid.IFF(u) {
			return true
		}
	}
	return false
}

// addEdgesByMatchingUIDS adds edges to the vertex in a graph based on if it
// matches a uid list.
func addEdgesByMatchingUIDS(g *pgraph.Graph, v pgraph.Vertex, uids []ResUID) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uid, and see if it matches any vertex
	for _, uid := range uids {
		var found = false
		// uid is a ResUID object
		for _, vv := range g.Vertices() { // search
			if v == vv { // skip self
				continue
			}
			if b, ok := g.Value("debug"); ok && util.Bool(b) {
				log.Printf("Compile: AutoEdge: Match: %s with UID: %s", vv, uid)
			}
			// we must match to an effective UID for the resource,
			// that is to say, the name value of a res is a helpful
			// handle, but it is not necessarily a unique identity!
			// remember, resources can return multiple UID's each!
			if UIDExistsInUIDs(uid, VtoR(vv).UIDs()) {
				// add edge from: vv -> v
				if uid.IsReversed() {
					txt := fmt.Sprintf("AutoEdge: %s -> %s", vv, v)
					log.Printf("Compile: Adding %s", txt)
					edge := &Edge{Name: txt}
					g.AddEdge(vv, v, edge)
				} else { // edges go the "normal" way, eg: pkg resource
					txt := fmt.Sprintf("AutoEdge: %s -> %s", v, vv)
					log.Printf("Compile: Adding %s", txt)
					edge := &Edge{Name: txt}
					g.AddEdge(v, vv, edge)
				}
				found = true
				break
			}
		}
		result = append(result, found)
	}
	return result
}

// AutoEdges adds the automatic edges to the graph.
func AutoEdges(g *pgraph.Graph) error {
	log.Println("Compile: Adding AutoEdges...")

	// initially get all of the autoedges to seek out all possible errors
	var err error
	autoEdgeObjVertexMap := make(map[pgraph.Vertex]AutoEdge)
	sorted := g.VerticesSorted()

	for _, v := range sorted { // for each vertexes autoedges
		if !VtoR(v).Meta().AutoEdge { // is the metaparam true?
			continue
		}
		autoEdgeObj, e := VtoR(v).AutoEdges()
		if e != nil {
			err = multierr.Append(err, e) // collect all errors
			continue
		}
		if autoEdgeObj == nil {
			log.Printf("%s: No auto edges were found!", v)
			continue // next vertex
		}
		autoEdgeObjVertexMap[v] = autoEdgeObj // save for next loop
	}
	if err != nil {
		return errwrap.Wrapf(err, "the auto edges had errors")
	}

	// now that we're guaranteed error free, we can modify the graph safely
	for _, v := range sorted { // stable sort order for determinism in logs
		autoEdgeObj, exists := autoEdgeObjVertexMap[v]
		if !exists {
			continue
		}

		for { // while the autoEdgeObj has more uids to add...
			uids := autoEdgeObj.Next() // get some!
			if uids == nil {
				log.Printf("%s: The auto edge list is empty!", v)
				break // inner loop
			}
			if b, ok := g.Value("debug"); ok && util.Bool(b) {
				log.Println("Compile: AutoEdge: UIDS:")
				for i, u := range uids {
					log.Printf("Compile: AutoEdge: UID%d: %v", i, u)
				}
			}

			// match and add edges
			result := addEdgesByMatchingUIDS(g, v, uids)

			// report back, and find out if we should continue
			if !autoEdgeObj.Test(result) {
				break
			}
		}
	}
	return nil
}
