// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

// Package pgraph represents the internal "pointer graph" that we use.
package pgraph

import (
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/resources"
	"github.com/purpleidea/mgmt/util"
)

// add edges to the vertex in a graph based on if it matches a uid list
func (g *Graph) addEdgesByMatchingUIDS(v *Vertex, uids []resources.ResUID) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uid, and see if it matches any vertex
	for _, uid := range uids {
		var found = false
		// uid is a ResUID object
		for _, vv := range g.GetVertices() { // search
			if v == vv { // skip self
				continue
			}
			if b, ok := g.Value("debug"); ok && util.Bool(b) {
				log.Printf("Compile: AutoEdge: Match: %s[%s] with UID: %s[%s]", vv.GetKind(), vv.GetName(), uid.GetKind(), uid.GetName())
			}
			// we must match to an effective UID for the resource,
			// that is to say, the name value of a res is a helpful
			// handle, but it is not necessarily a unique identity!
			// remember, resources can return multiple UID's each!
			if resources.UIDExistsInUIDs(uid, vv.UIDs()) {
				// add edge from: vv -> v
				if uid.IsReversed() {
					txt := fmt.Sprintf("AutoEdge: %s[%s] -> %s[%s]", vv.GetKind(), vv.GetName(), v.GetKind(), v.GetName())
					log.Printf("Compile: Adding %s", txt)
					g.AddEdge(vv, v, NewEdge(txt))
				} else { // edges go the "normal" way, eg: pkg resource
					txt := fmt.Sprintf("AutoEdge: %s[%s] -> %s[%s]", v.GetKind(), v.GetName(), vv.GetKind(), vv.GetName())
					log.Printf("Compile: Adding %s", txt)
					g.AddEdge(v, vv, NewEdge(txt))
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
func (g *Graph) AutoEdges() {
	log.Println("Compile: Adding AutoEdges...")
	for _, v := range g.GetVertices() { // for each vertexes autoedges
		if !v.Meta().AutoEdge { // is the metaparam true?
			continue
		}
		autoEdgeObj := v.AutoEdges()
		if autoEdgeObj == nil {
			log.Printf("%s[%s]: Config: No auto edges were found!", v.GetKind(), v.GetName())
			continue // next vertex
		}

		for { // while the autoEdgeObj has more uids to add...
			uids := autoEdgeObj.Next() // get some!
			if uids == nil {
				log.Printf("%s[%s]: Config: The auto edge list is empty!", v.GetKind(), v.GetName())
				break // inner loop
			}
			if b, ok := g.Value("debug"); ok && util.Bool(b) {
				log.Println("Compile: AutoEdge: UIDS:")
				for i, u := range uids {
					log.Printf("Compile: AutoEdge: UID%d: %v", i, u)
				}
			}

			// match and add edges
			result := g.addEdgesByMatchingUIDS(v, uids)

			// report back, and find out if we should continue
			if !autoEdgeObj.Test(result) {
				break
			}
		}
	}
}
