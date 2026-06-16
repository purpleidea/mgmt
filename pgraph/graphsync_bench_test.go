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
	"testing"
)

// graphSyncBenchCase describes a single GraphSync benchmark dataset. The two
// build functions are invoked once per benchmark (outside the timed loop) and
// must build graphs with distinct vertex pointers, like a real graph swap where
// the next graph is regenerated from scratch.
type graphSyncBenchCase struct {
	name     string
	buildOld func() *Graph
	buildNew func() *Graph
}

// graphSyncBenchCases is the list of datasets used by the GraphSync benchmark.
// The "identical" cases simulate the steady state where a value changed but the
// graph shape didn't: every new vertex has an old String() twin. The "delta"
// cases add roughly ten percent new vertices on top of that.
var graphSyncBenchCases = []graphSyncBenchCase{
	{
		name:     "identical/100",
		buildOld: func() *Graph { return buildRandomDAG(100, 4, 42) },
		buildNew: func() *Graph { return buildRandomDAG(100, 4, 42) },
	},
	{
		name:     "identical/1000",
		buildOld: func() *Graph { return buildRandomDAG(1000, 4, 42) },
		buildNew: func() *Graph { return buildRandomDAG(1000, 4, 42) },
	},
	{
		name:     "identical/10000",
		buildOld: func() *Graph { return buildRandomDAG(10000, 4, 42) },
		buildNew: func() *Graph { return buildRandomDAG(10000, 4, 42) },
	},
	{
		name:     "delta/1000",
		buildOld: func() *Graph { return buildRandomDAG(1000, 4, 42) },
		buildNew: func() *Graph { return buildRandomDAG(1100, 4, 42) },
	},
	{
		name:     "delta/10000",
		buildOld: func() *Graph { return buildRandomDAG(10000, 4, 42) },
		buildNew: func() *Graph { return buildRandomDAG(11000, 4, 42) },
	},
}

// BenchmarkGraphSync benchmarks syncing a regenerated graph onto an existing
// one, using the default String() based comparison functions. GraphSync mutates
// the receiver, so each iteration starts from a fresh copy of the old graph.
// The copy is cheap relative to the sync and identical across runs.
func BenchmarkGraphSync(b *testing.B) {
	for _, tc := range graphSyncBenchCases {
		oldGraph := tc.buildOld()
		newGraph := tc.buildNew()
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g := oldGraph.Copy()
				if err := g.GraphSync(newGraph, nil, nil, nil, nil); err != nil {
					b.Fatalf("GraphSync: %v", err)
				}
			}
		})
	}
}

// BenchmarkGraphCmp benchmarks comparing two topologically equal graphs that
// were built separately, so all the vertex pointers differ and every vertex
// must be matched by the comparison function.
func BenchmarkGraphCmp(b *testing.B) {
	cmpCases := []graphSyncBenchCase{
		graphSyncBenchCases[0], // identical/100
		graphSyncBenchCases[1], // identical/1000
		graphSyncBenchCases[2], // identical/10000
	}
	vertexCmpFn := func(v1, v2 Vertex) (bool, error) {
		return v1.String() == v2.String(), nil
	}
	edgeCmpFn := func(e1, e2 Edge) (bool, error) {
		return e1.String() == e2.String(), nil
	}
	for _, tc := range cmpCases {
		g1 := tc.buildOld()
		g2 := tc.buildNew()
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := g1.GraphCmp(g2, vertexCmpFn, edgeCmpFn); err != nil {
					b.Fatalf("GraphCmp: %v", err)
				}
			}
		})
	}
}
