// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package resources

import (
	"testing"

	"github.com/purpleidea/mgmt/pgraph"
)

func TestFileAutoEdge1(t *testing.T) {

	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	r1 := &FileRes{
		BaseRes: BaseRes{
			Name: "file1",
			Kind: "file",
			MetaParams: MetaParams{
				AutoEdge: true,
			},
		},
		Path: "/tmp/a/b/", // some dir
	}
	r2 := &FileRes{
		BaseRes: BaseRes{
			Name: "file2",
			Kind: "file",
			MetaParams: MetaParams{
				AutoEdge: true,
			},
		},
		Path: "/tmp/a/", // some parent dir
	}
	r3 := &FileRes{
		BaseRes: BaseRes{
			Name: "file3",
			Kind: "file",
			MetaParams: MetaParams{
				AutoEdge: true,
			},
		},
		Path: "/tmp/a/b/c", // some child file
	}
	g.AddVertex(r1, r2, r3)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
	}

	// run artificially without the entire engine
	if err := AutoEdges(g); err != nil {
		t.Errorf("error running autoedges: %v", err)
	}

	// two edges should have been added
	if i := g.NumEdges(); i != 2 {
		t.Errorf("should have 2 edges instead of: %d", i)
	}
}
