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

package ref

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

type testFunc struct {
	name string
}

func (obj *testFunc) String() string { return obj.name }

func (obj *testFunc) Validate() error { return nil }

func (obj *testFunc) Info() *interfaces.Info { return nil }

func (obj *testFunc) Init(*interfaces.Init) error { return nil }

func (obj *testFunc) Call(context.Context, []types.Value) (types.Value, error) {
	return types.NewNil(), nil
}

func TestCountEdgeUsesValueKey(t *testing.T) {
	count := (&Count{}).Init()
	f1 := &testFunc{name: "f1"}
	f2 := &testFunc{name: "f2"}
	edge := &interfaces.FuncEdge{Args: []string{"arg", "arg"}}

	v1New, v2New := count.EdgeInc(f1, f2, edge)
	if !v1New || !v2New {
		t.Fatalf("expected first edge inc to add both vertices")
	}
	key := CountEdge{f1: f1, f2: f2, arg: "arg"}
	if c := count.edges[key]; c != 2 {
		t.Fatalf("expected duplicate arg refcount to be 2, got %d", c)
	}
	if l := len(count.edges); l != 1 {
		t.Fatalf("expected one value-keyed edge entry, got %d", l)
	}

	v1New, v2New = count.EdgeInc(f1, f2, &interfaces.FuncEdge{Args: []string{"arg"}})
	if v1New || v2New {
		t.Fatalf("expected second edge inc to reuse both vertices")
	}
	if c := count.edges[key]; c != 3 {
		t.Fatalf("expected refcount to be 3, got %d", c)
	}

	count.EdgeDec(f1, f2, &interfaces.FuncEdge{Args: []string{"arg"}})
	if c := count.edges[key]; c != 2 {
		t.Fatalf("expected refcount to be 2, got %d", c)
	}
	count.EdgeDec(f1, f2, edge)
	if c := count.edges[key]; c != 0 {
		t.Fatalf("expected refcount to be 0, got %d", c)
	}
	if err := count.FreeEdge(f1, f2, "arg"); err != nil {
		t.Fatalf("expected zero-count edge to free: %v", err)
	}
	if _, exists := count.edges[key]; exists {
		t.Fatalf("expected edge key to be deleted")
	}
}
