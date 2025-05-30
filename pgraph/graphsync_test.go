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

func TestGraphSync1(t *testing.T) {
	g := &Graph{}
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")

	e1 := NE("e1")
	e2 := NE("e2")
	g.AddEdge(v1, v3, e1)
	g.AddEdge(v2, v3, e2)

	// new graph
	newGraph := &Graph{}
	v4 := NV("v4")
	v5 := NV("v5")
	e3 := NE("e3")
	newGraph.AddEdge(v4, v5, e3)

	err := g.GraphSync(newGraph, nil, nil, nil, nil)
	if err != nil {
		t.Errorf("failed with: %+v", err)
		return
	}

	// g should change and become the same
	if s := runGraphCmp(t, g, newGraph); s != "" {
		t.Errorf("%s", s)
	}
}

func TestGraphSync2(t *testing.T) {
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")
	v4 := NV("v4")
	v5 := NV("v5")
	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")

	g := &Graph{}
	g.AddEdge(v1, v3, e1)
	g.AddEdge(v2, v3, e2)

	// new graph
	newGraph := &Graph{}
	newGraph.AddEdge(v1, v3, e1)
	newGraph.AddEdge(v2, v3, e2)
	newGraph.AddEdge(v4, v5, e3)
	//newGraph.AddEdge(v3, v4, NE("v3,v4"))
	//newGraph.AddEdge(v3, v5, NE("v3,v5"))

	// graphs should differ!
	if runGraphCmp(t, g, newGraph) == "" {
		t.Errorf("graphs should differ initially")
		return
	}

	err := g.GraphSync(newGraph, strVertexCmpFn, vertexAddFn, vertexRemoveFn, strEdgeCmpFn)
	if err != nil {
		t.Errorf("failed with: %+v", err)
		return
	}

	// g should change and become the same
	if s := runGraphCmp(t, g, newGraph); s != "" {
		t.Errorf("%s", s)
	}
}

func TestGraphSync3(t *testing.T) {
	v1 := NV("v1")
	v2 := NV("v2")
	v3 := NV("v3")

	e1 := NE("e1")
	e2 := NE("e2")
	e3 := NE("e3")
	e4 := NE("e4")

	// g base graph with 3 edges
	g := &Graph{}
	g.AddEdge(v1, v2, e1)
	g.AddEdge(v2, v3, e2)
	g.AddEdge(v1, v3, e3)

	// newGraph input with 4 edges
	newGraph := &Graph{}
	newGraph.AddEdge(v1, v3, e1)
	newGraph.AddEdge(v2, v3, e2)
	newGraph.AddEdge(v1, v2, e3)
	newGraph.AddEdge(v3, v1, e4)

	if runGraphCmp(t, g, newGraph) == "" {
		t.Errorf("identical graphs: fail")
		return
	}

	if err := g.GraphSync(newGraph, strVertexCmpFn, vertexAddFn, vertexRemoveFn, strEdgeCmpFn); err != nil {
		t.Errorf("fail: %v", err)
		return
	}

	if s := runGraphCmp(t, g, newGraph); s != "" {
		t.Errorf("%s", s)
	}
}
