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

package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autoedge"
	"github.com/purpleidea/mgmt/pgraph"
)

// buildFileTree returns the resources for a three level directory tree with n
// files: /tmp/bench/dX/dY/fZ, plus the file resources for every intermediate
// directory. Each file matches its parent directory resource, so this exercises
// the realistic case where automatic edges are actually found.
func buildFileTree(n int) []engine.Res {
	resources := []engine.Res{}
	dirs := make(map[string]struct{})
	for i := 0; i < n; i++ {
		d1 := fmt.Sprintf("/tmp/bench/d%d/", i/100)
		d2 := fmt.Sprintf("%sd%d/", d1, (i/10)%10)
		dirs[d1] = struct{}{}
		dirs[d2] = struct{}{}
		resources = append(resources, &FileRes{
			Path: fmt.Sprintf("%sf%d", d2, i),
		})
	}
	for d := range dirs {
		resources = append(resources, &FileRes{
			Path: d,
		})
	}
	return resources
}

// buildFileFlat returns n file resources in one directory, without a resource
// for the directory itself. Nothing matches, so this exercises the worst case
// where every parent directory question must be answered with a miss.
func buildFileFlat(n int) []engine.Res {
	resources := []engine.Res{}
	for i := 0; i < n; i++ {
		resources = append(resources, &FileRes{
			Path: fmt.Sprintf("/tmp/bench/flat/f%d", i),
		})
	}
	return resources
}

// BenchmarkAutoEdge benchmarks the autoedge stage on file-heavy graphs, since
// files are the resource kind that appears in large numbers, and each one asks
// about every one of its parent directories. The graph is rebuilt every
// iteration because AutoEdge mutates it, but AddVertex is cheap relative to the
// matching work being measured.
func BenchmarkAutoEdge(b *testing.B) {
	benchCases := []struct {
		name  string
		build func() []engine.Res
	}{
		{name: "tree/100", build: func() []engine.Res { return buildFileTree(100) }},
		{name: "tree/1000", build: func() []engine.Res { return buildFileTree(1000) }},
		{name: "tree/10000", build: func() []engine.Res { return buildFileTree(10000) }},
		{name: "flat/1000", build: func() []engine.Res { return buildFileFlat(1000) }},
	}
	logf := func(format string, v ...interface{}) {} // discard
	for _, tc := range benchCases {
		resources := tc.build()
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g, err := pgraph.NewGraph("bench")
				if err != nil {
					b.Fatalf("error creating graph: %v", err)
				}
				for _, res := range resources {
					g.AddVertex(res)
				}
				if err := autoedge.AutoEdge(context.TODO(), g, false, logf); err != nil {
					b.Fatalf("error running autoedges: %v", err)
				}
			}
		})
	}
}
