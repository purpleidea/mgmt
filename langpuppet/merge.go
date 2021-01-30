// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

// Package langpuppet implements an integration entrypoint that combines lang and Puppet.
package langpuppet

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
)

const (
	// MergePrefixLang is how a mergeable vertex name starts in mcl code.
	MergePrefixLang = "puppet_"
	// MergePrefixPuppet is how a mergeable Puppet class name starts.
	MergePrefixPuppet = "mgmt_"
)

// mergeGraph returns the merged graph containing all vertices and edges found
// in the graphs produced by the lang and Puppet GAPIs associated with the
// wrapping GAPI. Vertices are merged if they adhere to the following rules (for
// any given value of POSTFIX): (1) The graph from lang contains a noop vertex
// named puppet_POSTFIX. (2) The graph from Puppet contains an empty class
// mgmt_POSTFIX. (3) The resulting graph will contain one noop vertex named
// POSTFIX that replaces all nodes mentioned in (1) and (2). All edges
// connecting to any of the vertices merged this way will be present in the
// merged graph.
func mergeGraphs(graphFromLang, graphFromPuppet *pgraph.Graph) (*pgraph.Graph, error) {
	if graphFromLang == nil || graphFromPuppet == nil {
		return nil, fmt.Errorf("cannot merge graphs until both child graphs are loaded")
	}

	result, err := pgraph.NewGraph(graphFromLang.Name + "+" + graphFromPuppet.Name)
	if err != nil {
		return nil, err
	}

	mergeTargets := make(map[string]pgraph.Vertex)

	// first add all vertices from the lang graph
	for _, vertex := range graphFromLang.Vertices() {
		if strings.Index(vertex.String(), "noop["+MergePrefixLang) == 0 {
			resource, ok := vertex.(engine.Res)
			if !ok {
				return nil, fmt.Errorf("vertex %s is not a named resource", vertex.String())
			}
			basename := strings.TrimPrefix(resource.Name(), MergePrefixLang)
			resource.SetName(basename)
			mergeTargets[basename] = vertex
		}
		result.AddVertex(vertex)
		for _, neighbor := range graphFromLang.OutgoingGraphVertices(vertex) {
			result.AddVertex(neighbor)
			result.AddEdge(vertex, neighbor, graphFromLang.FindEdge(vertex, neighbor))
		}
	}

	var anchor pgraph.Vertex
	mergePairs := make(map[pgraph.Vertex]pgraph.Vertex)

	// do a scan through the Puppet graph, and mark all vertices that will be
	// subject to a merge, so it will be easier do generate the new edges
	// in the final pass
	for _, vertex := range graphFromPuppet.Vertices() {
		if vertex.String() == "noop[admissible_Stage[main]]" {
			// we can start a depth first search here
			anchor = vertex
			continue
		}
		// at this stage we don't distinguis between class start and end
		if strings.Index(vertex.String(), "noop[admissible_Class["+strings.Title(MergePrefixPuppet)) != 0 &&
			strings.Index(vertex.String(), "noop[completed_Class["+strings.Title(MergePrefixPuppet)) != 0 {
			continue
		}

		resource, ok := vertex.(engine.Res)
		if !ok {
			return nil, fmt.Errorf("vertex %s is not a named resource", vertex.String())
		}
		// strip either prefix (plus the closing bracket)
		basename := strings.TrimSuffix(
			strings.TrimPrefix(
				strings.TrimPrefix(resource.Name(),
					"admissible_Class["+strings.Title(MergePrefixPuppet)),
				"completed_Class["+strings.Title(MergePrefixPuppet)),
			"]")

		if _, found := mergeTargets[basename]; !found {
			// FIXME: should be a warning not an error?
			return nil, fmt.Errorf("puppet graph has unmatched class %s%s", MergePrefixPuppet, basename)
		}

		mergePairs[vertex] = mergeTargets[basename]

		if strings.Index(resource.Name(), "admissible_Class["+strings.Title(MergePrefixPuppet)) != 0 {
			continue
		}

		// is there more than one edge outgoing from the class start?
		if graphFromPuppet.OutDegree()[vertex] > 1 {
			return nil, fmt.Errorf("class %s is not empty", basename)
		}

		// does this edge not lead to the class end?
		next := graphFromPuppet.OutgoingGraphVertices(vertex)[0]
		if next.String() != "noop[completed_Class["+strings.Title(MergePrefixPuppet)+basename+"]]" {
			return nil, fmt.Errorf("class %s%s is not empty, start is followed by %s", MergePrefixPuppet, basename, next.String())
		}
	}

	merged := make(map[pgraph.Vertex]bool)
	result.AddVertex(anchor)
	// traverse the puppet graph, add all vertices and perform merges
	// using DFS so we can be sure the "admissible" is visited before the "completed" vertex
	for _, vertex := range graphFromPuppet.DFS(anchor) {
		source := vertex

		// when adding edges, the source might be a different vertex
		// than the current one, if this is a merged vertex
		if _, found := mergePairs[vertex]; found {
			source = mergePairs[vertex]
		}

		// the current vertex has been added by previous iterations,
		// we only add neighbors here
		for _, neighbor := range graphFromPuppet.OutgoingGraphVertices(vertex) {
			if strings.Index(neighbor.String(), "noop[admissible_Class["+strings.Title(MergePrefixPuppet)) == 0 {
				result.AddEdge(source, mergePairs[neighbor], graphFromPuppet.FindEdge(vertex, neighbor))
				continue
			}
			if strings.Index(neighbor.String(), "noop[completed_Class["+strings.Title(MergePrefixPuppet)) == 0 {
				// mark target vertex as merged
				merged[mergePairs[neighbor]] = true
				continue
			}
			// if we reach here, this neighbor is a regular vertex
			result.AddVertex(neighbor)
			result.AddEdge(source, neighbor, graphFromPuppet.FindEdge(vertex, neighbor))
		}
	}

	for _, vertex := range mergeTargets {
		if !merged[vertex] {
			// FIXME: should be a warning not an error?
			return nil, fmt.Errorf("lang graph has unmatched %s", vertex.String())
		}
	}

	return result, nil
}
