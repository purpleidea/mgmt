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

package engine

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/pgraph"
)

func BenchmarkResGraphMapper(b *testing.B) {
	sizes := []int{10, 50, 100, 500}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			oldGraph, err := pgraph.NewGraph("old")
			if err != nil {
				b.Fatal(err)
			}
			newGraph, err := pgraph.NewGraph("new")
			if err != nil {
				b.Fatal(err)
			}

			for i := 0; i < size; i++ {
				r := &testRes{kind: "file", name: fmt.Sprintf("/tmp/file%d", i)}
				oldGraph.AddVertex(r)
				newGraph.AddVertex(r)
			}

			for i := size; i < size+size/2; i++ {
				oldGraph.AddVertex(&testRes{kind: "file", name: fmt.Sprintf("/tmp/oldonly%d", i)})
				newGraph.AddVertex(&testRes{kind: "file", name: fmt.Sprintf("/tmp/newonly%d", i)})
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ResGraphMapper(oldGraph, newGraph)
			}
		})
	}
}
