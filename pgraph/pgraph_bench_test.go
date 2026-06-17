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

//go:build !root

package pgraph

import (
	"fmt"
	"math/rand"
	"testing"
)

// topoBenchCase describes a single benchmark dataset. The build function is
// invoked once per benchmark (outside the timed loop) to construct the input
// graph. Add new shapes/sizes by appending entries to topoBenchCases below.
type topoBenchCase struct {
	name  string
	build func() *Graph
}

// topoBenchCases is the list of datasets used by both topological sort
// benchmarks. To add a new dataset, append a new entry with a descriptive name
// and a builder function. The name is used as the b.Run sub-benchmark name.
var topoBenchCases = []topoBenchCase{
	// Chain: a single linear path v0 -> v1 -> ... -> vN-1.
	{name: "chain/100", build: func() *Graph { return buildChain(100) }},
	{name: "chain/1000", build: func() *Graph { return buildChain(1000) }},
	{name: "chain/10000", build: func() *Graph { return buildChain(10000) }},

	// Fanout: a single root vertex with N-1 leaves (root -> leaf_i).
	{name: "fanout/100", build: func() *Graph { return buildFanout(100) }},
	{name: "fanout/1000", build: func() *Graph { return buildFanout(1000) }},
	{name: "fanout/10000", build: func() *Graph { return buildFanout(10000) }},

	// Fanin: N-1 roots all pointing at a single sink (root_i -> sink).
	{name: "fanin/100", build: func() *Graph { return buildFanin(100) }},
	{name: "fanin/1000", build: func() *Graph { return buildFanin(1000) }},
	{name: "fanin/10000", build: func() *Graph { return buildFanin(10000) }},

	// Layered: layers of `width` vertices, each vertex connected to every
	// vertex in the next layer. Edges = width^2 * (layers-1).
	{name: "layered/10x10", build: func() *Graph { return buildLayered(10, 10) }},
	{name: "layered/20x50", build: func() *Graph { return buildLayered(20, 50) }},
	{name: "layered/50x100", build: func() *Graph { return buildLayered(50, 100) }},

	// Random DAG: N vertices, each non-root vertex picks `avgDeg` random
	// predecessors from earlier vertices. Seeded for reproducibility.
	{name: "random/100/deg4", build: func() *Graph { return buildRandomDAG(100, 4, 42) }},
	{name: "random/1000/deg4", build: func() *Graph { return buildRandomDAG(1000, 4, 42) }},
	{name: "random/1000/deg16", build: func() *Graph { return buildRandomDAG(1000, 16, 42) }},
	{name: "random/10000/deg4", build: func() *Graph { return buildRandomDAG(10000, 4, 42) }},

	// Disconnected: many small chains sitting side by side. Stresses the
	// outer loop / S-set initialization rather than edge traversal.
	{name: "disconnected/1000x10", build: func() *Graph { return buildDisconnectedChains(1000, 10) }},
}

// BenchmarkTopologicalSort benchmarks the (non-deterministic) Kahn-based sort
// across each dataset in topoBenchCases.
func BenchmarkTopologicalSort(b *testing.B) {
	for _, tc := range topoBenchCases {
		g := tc.build()
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := g.TopologicalSort(); err != nil {
					b.Fatalf("TopologicalSort: %v", err)
				}
			}
		})
	}
}

// BenchmarkDeterministicTopologicalSort benchmarks the deterministic variant
// across the same datasets, so results are directly comparable.
func BenchmarkDeterministicTopologicalSort(b *testing.B) {
	for _, tc := range topoBenchCases {
		g := tc.build()
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := g.DeterministicTopologicalSort(); err != nil {
					b.Fatalf("DeterministicTopologicalSort: %v", err)
				}
			}
		})
	}
}

// buildChain returns a graph of n vertices linked v0 -> v1 -> ... -> v(n-1).
func buildChain(n int) *Graph {
	g, _ := NewGraph("chain")
	if n <= 0 {
		return g
	}
	vs := make([]Vertex, n)
	for i := 0; i < n; i++ {
		vs[i] = NV(fmt.Sprintf("v%d", i))
	}
	g.AddVertex(vs[0])
	for i := 1; i < n; i++ {
		g.AddEdge(vs[i-1], vs[i], edgeGenFn(vs[i-1], vs[i]))
	}
	return g
}

// buildFanout returns a graph with one root and n-1 leaves: root -> leaf_i.
func buildFanout(n int) *Graph {
	g, _ := NewGraph("fanout")
	if n <= 0 {
		return g
	}
	root := NV("root")
	g.AddVertex(root)
	for i := 1; i < n; i++ {
		leaf := NV(fmt.Sprintf("leaf%d", i))
		g.AddEdge(root, leaf, edgeGenFn(root, leaf))
	}
	return g
}

// buildFanin returns a graph with n-1 roots and one sink: root_i -> sink.
func buildFanin(n int) *Graph {
	g, _ := NewGraph("fanin")
	if n <= 0 {
		return g
	}
	sink := NV("sink")
	g.AddVertex(sink)
	for i := 1; i < n; i++ {
		root := NV(fmt.Sprintf("root%d", i))
		g.AddEdge(root, sink, edgeGenFn(root, sink))
	}
	return g
}

// buildLayered returns a graph of `layers` layers of `width` vertices each,
// where every vertex in layer L points to every vertex in layer L+1.
func buildLayered(layers, width int) *Graph {
	g, _ := NewGraph("layered")
	if layers <= 0 || width <= 0 {
		return g
	}
	prev := make([]Vertex, width)
	for j := 0; j < width; j++ {
		prev[j] = NV(fmt.Sprintf("L0_%d", j))
		g.AddVertex(prev[j])
	}
	for l := 1; l < layers; l++ {
		curr := make([]Vertex, width)
		for j := 0; j < width; j++ {
			curr[j] = NV(fmt.Sprintf("L%d_%d", l, j))
		}
		for _, p := range prev {
			for _, c := range curr {
				g.AddEdge(p, c, edgeGenFn(p, c))
			}
		}
		prev = curr
	}
	return g
}

// buildRandomDAG returns a random DAG with n vertices. Each vertex i > 0 picks
// up to `avgDeg` distinct predecessors uniformly at random from {0..i-1}, which
// guarantees acyclicity. The seed makes the dataset reproducible.
func buildRandomDAG(n, avgDeg int, seed int64) *Graph {
	g, _ := NewGraph("random")
	if n <= 0 {
		return g
	}
	//nolint:gosec // G404: deterministic seed for reproducible benchmark data
	r := rand.New(rand.NewSource(seed))
	vs := make([]Vertex, n)
	for i := 0; i < n; i++ {
		vs[i] = NV(fmt.Sprintf("v%d", i))
	}
	g.AddVertex(vs[0])
	for i := 1; i < n; i++ {
		k := avgDeg
		if k > i {
			k = i
		}
		seen := make(map[int]struct{}, k)
		for added := 0; added < k; {
			p := r.Intn(i)
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			g.AddEdge(vs[p], vs[i], edgeGenFn(vs[p], vs[i]))
			added++
		}
	}
	return g
}

// buildDisconnectedChains returns a graph composed of `chains` independent
// linear chains, each of length `chainLen`. Total vertices = chains * chainLen.
func buildDisconnectedChains(chains, chainLen int) *Graph {
	g, _ := NewGraph("disconnected")
	for c := 0; c < chains; c++ {
		var prev Vertex
		for i := 0; i < chainLen; i++ {
			v := NV(fmt.Sprintf("c%d_v%d", c, i))
			if i == 0 {
				g.AddVertex(v)
			} else {
				g.AddEdge(prev, v, edgeGenFn(prev, v))
			}
			prev = v
		}
	}
	return g
}
