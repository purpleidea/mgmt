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

// Package pgraph represents the internal "pointer graph" that we use.
package pgraph

import (
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/global"
	"github.com/purpleidea/mgmt/resources"
)

// add edges to the vertex in a graph based on if it matches a uuid list
func (g *Graph) addEdgesByMatchingUUIDS(v *Vertex, uuids []resources.ResUUID) []bool {
	// search for edges and see what matches!
	var result []bool

	// loop through each uuid, and see if it matches any vertex
	for _, uuid := range uuids {
		var found = false
		// uuid is a ResUUID object
		for _, vv := range g.GetVertices() { // search
			if v == vv { // skip self
				continue
			}
			if global.DEBUG {
				log.Printf("Compile: AutoEdge: Match: %v[%v] with UUID: %v[%v]", vv.Kind(), vv.GetName(), uuid.Kind(), uuid.GetName())
			}
			// we must match to an effective UUID for the resource,
			// that is to say, the name value of a res is a helpful
			// handle, but it is not necessarily a unique identity!
			// remember, resources can return multiple UUID's each!
			if resources.UUIDExistsInUUIDs(uuid, vv.GetUUIDs()) {
				// add edge from: vv -> v
				if uuid.Reversed() {
					txt := fmt.Sprintf("AutoEdge: %v[%v] -> %v[%v]", vv.Kind(), vv.GetName(), v.Kind(), v.GetName())
					log.Printf("Compile: Adding %v", txt)
					g.AddEdge(vv, v, NewEdge(txt))
				} else { // edges go the "normal" way, eg: pkg resource
					txt := fmt.Sprintf("AutoEdge: %v[%v] -> %v[%v]", v.Kind(), v.GetName(), vv.Kind(), vv.GetName())
					log.Printf("Compile: Adding %v", txt)
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
			log.Printf("%v[%v]: Config: No auto edges were found!", v.Kind(), v.GetName())
			continue // next vertex
		}

		for { // while the autoEdgeObj has more uuids to add...
			uuids := autoEdgeObj.Next() // get some!
			if uuids == nil {
				log.Printf("%v[%v]: Config: The auto edge list is empty!", v.Kind(), v.GetName())
				break // inner loop
			}
			if global.DEBUG {
				log.Println("Compile: AutoEdge: UUIDS:")
				for i, u := range uuids {
					log.Printf("Compile: AutoEdge: UUID%d: %v", i, u)
				}
			}

			// match and add edges
			result := g.addEdgesByMatchingUUIDS(v, uuids)

			// report back, and find out if we should continue
			if !autoEdgeObj.Test(result) {
				break
			}
		}
	}
}
