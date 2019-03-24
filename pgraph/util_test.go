// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

// +build !root

package pgraph

import (
	"fmt"
	"testing"
)

// vertex is a test struct to test the library.
type vertex struct {
	name string
}

// String is a required method of the Vertex interface that we must fulfill.
func (v *vertex) String() string {
	return v.name
}

// NV is a helper function to make testing easier. It creates a new noop vertex.
func NV(s string) Vertex {
	return &vertex{s}
}

// edge is a test struct to test the library.
type edge struct {
	name string
}

// String is a required method of the Edge interface that we must fulfill.
func (e *edge) String() string {
	return e.name
}

// NE is a helper function to make testing easier. It creates a new noop edge.
func NE(s string) Edge {
	return &edge{s}
}

// edgeGenFn generates unique edges for each vertex pair, assuming unique
// vertices.
func edgeGenFn(v1, v2 Vertex) Edge {
	return NE(fmt.Sprintf("%s,%s", v1, v2))
}

func vertexAddFn(v Vertex) error {
	return nil
}

func vertexRemoveFn(v Vertex) error {
	return nil
}

func runGraphCmp(t *testing.T, g1, g2 *Graph) string {
	err := g1.GraphCmp(g2, strVertexCmpFn, strEdgeCmpFn)
	if err != nil {
		str := ""
		str += fmt.Sprintf("  actual (g1): %v%s", g1, fullPrint(g1))
		str += fmt.Sprintf("expected (g2): %v%s", g2, fullPrint(g2))
		str += fmt.Sprintf("cmp error:")
		str += fmt.Sprintf("%v", err)
		return str
	}
	return ""
}

func fullPrint(g *Graph) (str string) {
	str += "\n"
	for v := range g.Adjacency() {
		str += fmt.Sprintf("* v: %s\n", v)
	}
	for v1 := range g.Adjacency() {
		for v2, e := range g.Adjacency()[v1] {
			str += fmt.Sprintf("* e: %s -> %s # %s\n", v1, v2, e)
		}
	}
	return
}
