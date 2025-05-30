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
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	_ "github.com/purpleidea/mgmt/engine/resources" // import so the resources register
	"github.com/purpleidea/mgmt/pgraph"
)

// ListPgraphVertexCmp compares two lists of pgraph.Vertex pointers.
func ListPgraphVertexCmp(a, b []pgraph.Vertex) bool {
	//fmt.Printf("CMP: %v with %v\n", a, b) // debugging
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// empty graph
func TestRHVSort1(t *testing.T) {

	r1, err := engine.NewNamedResource("http:server", "foo")
	if err != nil {
		panic(fmt.Sprintf("unexpected error: %+v", err))
	}
	r2, err := engine.NewNamedResource("http:server:ui", "bar")
	if err != nil {
		panic(fmt.Sprintf("unexpected error: %+v", err))
	}

	vertices := []pgraph.Vertex{r1, r2}
	expected := []pgraph.Vertex{r2, r1}

	if out := RHVSort(vertices); !ListPgraphVertexCmp(expected, out) {
		t.Errorf("vertices: %+v", vertices)
		t.Errorf("expected: %+v", expected)
		t.Errorf("test out: %+v", out)
	}

}
