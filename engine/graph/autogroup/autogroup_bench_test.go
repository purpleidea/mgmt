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

package autogroup

import (
	"context"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/pgraph"
)

// buildGroupable returns a disconnected graph of n test resources which all
// want to group together (same starting letter), so this measures the cost of
// the merges themselves.
func buildGroupable(n int) *pgraph.Graph {
	g, _ := pgraph.NewGraph("bench")
	for i := 0; i < n; i++ {
		g.AddVertex(NewNoopResTest(fmt.Sprintf("a%d", i)))
	}
	return g
}

// buildLetters returns a disconnected graph of n test resources whose names
// cycle through 26 starting letters, so they collapse into 26 groups and most
// pair comparisons are rejected by the cheap cmp.
func buildLetters(n int) *pgraph.Graph {
	g, _ := pgraph.NewGraph("bench")
	for i := 0; i < n; i++ {
		g.AddVertex(NewNoopResTest(fmt.Sprintf("%c%d", 'a'+(i%26), i)))
	}
	return g
}

// buildChain returns a linear chain of n test resources which all want to group
// (same starting letter) but never can, because each pair is connected by a
// path. This is the worst case for the reachability viability check.
func buildChain(n int) *pgraph.Graph {
	g, _ := pgraph.NewGraph("bench")
	var prev pgraph.Vertex
	for i := 0; i < n; i++ {
		v := NewNoopResTest(fmt.Sprintf("a%d", i))
		if i == 0 {
			g.AddVertex(v)
		} else {
			g.AddEdge(prev, v, NE(fmt.Sprintf("e%d", i)))
		}
		prev = v
	}
	return g
}

// buildDisabled returns a disconnected graph of n test resources with the
// autogroup meta param disabled, so every pair is rejected as early as
// possible. This approximates a large graph of resources that don't group.
func buildDisabled(n int) *pgraph.Graph {
	g, _ := pgraph.NewGraph("bench")
	for i := 0; i < n; i++ {
		r := NewNoopResTest(fmt.Sprintf("a%d", i))
		r.AutoGroupMeta().Disabled = true
		g.AddVertex(r)
	}
	return g
}

// BenchmarkAutoGroup benchmarks the autogroup stage, which runs on every graph
// deploy. The graph and its resources are rebuilt every iteration, because
// grouping mutates the resources themselves; the build cost is included in
// every variant equally.
func BenchmarkAutoGroup(b *testing.B) {
	benchCases := []struct {
		name  string
		build func() *pgraph.Graph
	}{
		{name: "group/100", build: func() *pgraph.Graph { return buildGroupable(100) }},
		{name: "group/1000", build: func() *pgraph.Graph { return buildGroupable(1000) }},
		{name: "letters/100", build: func() *pgraph.Graph { return buildLetters(100) }},
		{name: "letters/1000", build: func() *pgraph.Graph { return buildLetters(1000) }},
		{name: "chain/100", build: func() *pgraph.Graph { return buildChain(100) }},
		//{name: "chain/1000", build: func() *pgraph.Graph { return buildChain(1000) }}, // too slow
		{name: "disabled/1000", build: func() *pgraph.Graph { return buildDisabled(1000) }},
	}
	logf := func(format string, v ...interface{}) {} // discard
	for _, tc := range benchCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g := tc.build()
				if err := AutoGroup(context.TODO(), &testGrouper{}, g, false, logf); err != nil {
					b.Fatalf("error running autogroup: %v", err)
				}
			}
		})
	}
}
