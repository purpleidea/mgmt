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

func TestAddEdgeGraph1(t *testing.T) {
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

	sub := &Graph{}
	sub.AddEdge(v4, v5, e3)

	g.AddGraph(sub)

	// expected (can reuse the same vertices)
	expected := &Graph{}
	expected.AddEdge(v1, v3, e1)
	expected.AddEdge(v2, v3, e2)
	expected.AddEdge(v4, v5, e3)

	//expected.AddEdge(v3, v4, NE("v3,v4"))
	//expected.AddEdge(v3, v5, NE("v3,v5"))

	if s := runGraphCmp(t, g, expected); s != "" {
		t.Errorf("%s", s)
	}
}

func TestAddEdgeVertexGraph1(t *testing.T) {
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

	sub := &Graph{}
	sub.AddEdge(v4, v5, e3)

	g.AddEdgeVertexGraph(v3, sub, edgeGenFn)

	// expected (can reuse the same vertices)
	expected := &Graph{}
	expected.AddEdge(v1, v3, e1)
	expected.AddEdge(v2, v3, e2)
	expected.AddEdge(v4, v5, e3)

	expected.AddEdge(v3, v4, NE("v3,v4"))
	expected.AddEdge(v3, v5, NE("v3,v5"))

	if s := runGraphCmp(t, g, expected); s != "" {
		t.Errorf("%s", s)
	}
}

func TestAddEdgeGraphVertex1(t *testing.T) {
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

	sub := &Graph{}
	sub.AddEdge(v4, v5, e3)

	g.AddEdgeGraphVertex(sub, v3, edgeGenFn)

	// expected (can reuse the same vertices)
	expected := &Graph{}
	expected.AddEdge(v1, v3, e1)
	expected.AddEdge(v2, v3, e2)
	expected.AddEdge(v4, v5, e3)

	expected.AddEdge(v4, v3, NE("v4,v3"))
	expected.AddEdge(v5, v3, NE("v5,v3"))

	if s := runGraphCmp(t, g, expected); s != "" {
		t.Errorf("%s", s)
	}
}

func TestAddEdgeVertexGraphLight1(t *testing.T) {
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

	sub := &Graph{}
	sub.AddEdge(v4, v5, e3)

	g.AddEdgeVertexGraphLight(v3, sub, edgeGenFn)

	// expected (can reuse the same vertices)
	expected := &Graph{}
	expected.AddEdge(v1, v3, e1)
	expected.AddEdge(v2, v3, e2)
	expected.AddEdge(v4, v5, e3)

	expected.AddEdge(v3, v4, NE("v3,v4"))
	//expected.AddEdge(v3, v5, NE("v3,v5")) // not needed with light

	if s := runGraphCmp(t, g, expected); s != "" {
		t.Errorf("%s", s)
	}
}

func TestAddEdgeGraphVertexLight1(t *testing.T) {
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

	sub := &Graph{}
	sub.AddEdge(v4, v5, e3)

	g.AddEdgeGraphVertexLight(sub, v3, edgeGenFn)

	// expected (can reuse the same vertices)
	expected := &Graph{}
	expected.AddEdge(v1, v3, e1)
	expected.AddEdge(v2, v3, e2)
	expected.AddEdge(v4, v5, e3)

	//expected.AddEdge(v4, v3, NE("v4,v3")) // not needed with light
	expected.AddEdge(v5, v3, NE("v5,v3"))

	if s := runGraphCmp(t, g, expected); s != "" {
		t.Errorf("%s", s)
	}
}
