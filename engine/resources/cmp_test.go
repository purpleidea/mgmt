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
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
)

func newRes(tb testing.TB, kind, name string) engine.RecvableRes {
	tb.Helper()

	res, err := engine.NewNamedResource(kind, name)
	if err != nil {
		tb.Fatalf("error creating resource: %v", err)
	}

	r, ok := res.(engine.RecvableRes)
	if !ok {
		tb.Fatalf("resource is not recvable")
	}
	return r
}

// TestResGraphMapper1 tests that a resource present in both graphs is mapped.
func TestResGraphMapper1(t *testing.T) {
	old, err := pgraph.NewGraph("old")
	if err != nil {
		t.Errorf("error creating old graph: %v", err)
		return
	}
	new, err := pgraph.NewGraph("new")
	if err != nil {
		t.Errorf("error creating new graph: %v", err)
		return
	}
	old.AddVertex(newRes(t, "file", "/etc/hosts"))
	new.AddVertex(newRes(t, "file", "/etc/hosts"))

	got, err := engine.ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if len(got) != 1 {
		t.Errorf("len(mapper) = %d, expected 1", len(got))
	}
	for newRes, oldRes := range got {
		if newRes.Kind() != oldRes.Kind() {
			t.Errorf("kind mismatch: %s != %s", newRes.Kind(), oldRes.Kind())
		}
		if newRes.Name() != oldRes.Name() {
			t.Errorf("name mismatch: %s != %s", newRes.Name(), oldRes.Name())
		}
	}
}

// TestResGraphMapper2 tests that a resource only in oldGraph produces no
// mapping.
func TestResGraphMapper2(t *testing.T) {
	old, err := pgraph.NewGraph("old")
	if err != nil {
		t.Errorf("error creating old graph: %v", err)
		return
	}
	new, err := pgraph.NewGraph("new")
	if err != nil {
		t.Errorf("error creating new graph: %v", err)
		return
	}
	old.AddVertex(newRes(t, "file", "/etc/old"))
	new.AddVertex(newRes(t, "file", "/etc/new"))

	got, err := engine.ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if len(got) != 0 {
		t.Errorf("len(mapper) = %d, expected 0", len(got))
	}
}

// TestResGraphMapper3 tests that resources with the same name but different
// kind are not mapped.
func TestResGraphMapper3(t *testing.T) {
	old, err := pgraph.NewGraph("old")
	if err != nil {
		t.Errorf("error creating old graph: %v", err)
		return
	}
	new, err := pgraph.NewGraph("new")
	if err != nil {
		t.Errorf("error creating new graph: %v", err)
		return
	}
	old.AddVertex(newRes(t, "file", "foo"))
	new.AddVertex(newRes(t, "test", "foo"))

	got, err := engine.ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if len(got) != 0 {
		t.Errorf("len(mapper) = %d, expected 0", len(got))
	}
}

// TestResGraphMapper4 tests mapping with multiple resources across both graphs.
func TestResGraphMapper4(t *testing.T) {
	old, err := pgraph.NewGraph("old")
	if err != nil {
		t.Errorf("error creating old graph: %v", err)
		return
	}
	new, err := pgraph.NewGraph("new")
	if err != nil {
		t.Errorf("error creating new graph: %v", err)
		return
	}
	old.AddVertex(newRes(t, "file", "/etc/hosts"))
	new.AddVertex(newRes(t, "file", "/etc/hosts"))
	old.AddVertex(newRes(t, "file", "/etc/resolv.conf"))
	new.AddVertex(newRes(t, "test", "vim"))

	got, err := engine.ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if len(got) != 1 {
		t.Errorf("len(mapper) = %d, expected 1", len(got))
	}
}

// TestResGraphMapper5 tests that empty graphs produce an empty mapping.
func TestResGraphMapper5(t *testing.T) {
	old, err := pgraph.NewGraph("old")
	if err != nil {
		t.Errorf("error creating old graph: %v", err)
		return
	}
	new, err := pgraph.NewGraph("new")
	if err != nil {
		t.Errorf("error creating new graph: %v", err)
		return
	}

	got, err := engine.ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("error = %v", err)
		return
	}
	if len(got) != 0 {
		t.Errorf("len(mapper) = %d, expected 0", len(got))
	}
}
