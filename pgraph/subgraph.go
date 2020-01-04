// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package pgraph

// AddGraph adds the set of edges and vertices of a graph to the existing graph.
func (g *Graph) AddGraph(graph *Graph) {
	g.addEdgeVertexGraphHelper(nil, graph, nil, false, false)
}

// AddEdgeVertexGraph adds a directed edge to the graph from a vertex.
// This is useful for flattening the relationship between a subgraph and an
// existing graph, without having to run the subgraph recursively. It adds the
// maximum number of edges, creating a relationship to every vertex.
func (g *Graph) AddEdgeVertexGraph(vertex Vertex, graph *Graph, edgeGenFn func(v1, v2 Vertex) Edge) {
	g.addEdgeVertexGraphHelper(vertex, graph, edgeGenFn, false, false)
}

// AddEdgeVertexGraphLight adds a directed edge to the graph from a vertex.
// This is useful for flattening the relationship between a subgraph and an
// existing graph, without having to run the subgraph recursively. It adds the
// minimum number of edges, creating a relationship to the vertices with
// indegree equal to zero.
func (g *Graph) AddEdgeVertexGraphLight(vertex Vertex, graph *Graph, edgeGenFn func(v1, v2 Vertex) Edge) {
	g.addEdgeVertexGraphHelper(vertex, graph, edgeGenFn, false, true)
}

// AddEdgeGraphVertex adds a directed edge to the vertex from a graph.
// This is useful for flattening the relationship between a subgraph and an
// existing graph, without having to run the subgraph recursively. It adds the
// maximum number of edges, creating a relationship from every vertex.
func (g *Graph) AddEdgeGraphVertex(graph *Graph, vertex Vertex, edgeGenFn func(v1, v2 Vertex) Edge) {
	g.addEdgeVertexGraphHelper(vertex, graph, edgeGenFn, true, false)
}

// AddEdgeGraphVertexLight adds a directed edge to the vertex from a graph.
// This is useful for flattening the relationship between a subgraph and an
// existing graph, without having to run the subgraph recursively. It adds the
// minimum number of edges, creating a relationship from the vertices with
// outdegree equal to zero.
func (g *Graph) AddEdgeGraphVertexLight(graph *Graph, vertex Vertex, edgeGenFn func(v1, v2 Vertex) Edge) {
	g.addEdgeVertexGraphHelper(vertex, graph, edgeGenFn, true, true)
}

// addEdgeVertexGraphHelper is a helper function to add a directed edges to the
// graph from a vertex, or vice-versa. It operates in this reverse direction by
// specifying the reverse argument as true. It is useful for flattening the
// relationship between a subgraph and an existing graph, without having to run
// the subgraph recursively. It adds the maximum number of edges, creating a
// relationship to or from every vertex if the light argument is false, and if
// it is true, it adds the minimum number of edges, creating a relationship to
// or from the vertices with an indegree or outdegree equal to zero depending on
// if we specified reverse or not.
func (g *Graph) addEdgeVertexGraphHelper(vertex Vertex, graph *Graph, edgeGenFn func(v1, v2 Vertex) Edge, reverse, light bool) {
	if graph == nil {
		return // if the graph is empty, there's nothing to do!
	}

	var degree map[Vertex]int // compute all of the in/outdegree's if needed
	if light && reverse {
		degree = graph.OutDegree()
	} else if light { // && !reverse
		degree = graph.InDegree()
	}
	for _, v := range graph.VerticesSorted() { // sort to help out edgeGenFn

		// forward:
		// we only want to add edges to indegree == 0, because every
		// other vertex is a dependency of at least one of those

		// reverse:
		// we only want to add edges to outdegree == 0, because every
		// other vertex is a pre-requisite to at least one of these
		if light && degree[v] != 0 {
			continue
		}

		g.AddVertex(v) // ensure vertex is part of the graph

		if vertex != nil && reverse {
			edge := edgeGenFn(v, vertex) // generate a new unique edge
			g.AddEdge(v, vertex, edge)
		} else if vertex != nil { // && !reverse
			edge := edgeGenFn(vertex, v)
			g.AddEdge(vertex, v, edge)
		}
	}

	// also remember to suck in all of the graph's edges too!
	for v1 := range graph.Adjacency() {
		for v2, e := range graph.Adjacency()[v1] {
			g.AddEdge(v1, v2, e)
		}
	}
}
