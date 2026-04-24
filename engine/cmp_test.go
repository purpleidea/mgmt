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

package engine

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/pgraph"
	
)

type TestRes struct {
	kind       string
	name       string
	metaParams *MetaParams
	recv       map[string]*Send
}

func (r *TestRes) Kind() string                                   { return r.kind }
func (r *TestRes) SetKind(k string)                               { r.kind = k }
func (r *TestRes) Name() string                                   { return r.name }
func (r *TestRes) SetName(n string)                               { r.name = n }
func (r *TestRes) String() string                                 { return r.kind + "[" + r.name + "]" }
func (r *TestRes) Validate() error                                { return nil }
func (r *TestRes) Default() Res                                   { return &TestRes{} }
func (r *TestRes) Init(*Init) error                               { return nil }
func (r *TestRes) Cleanup() error                                 { return nil }
func (r *TestRes) Watch(context.Context) error                    { return nil }
func (r *TestRes) CheckApply(context.Context, bool) (bool, error) { return false, nil }
func (r *TestRes) Cmp(Res) error                                  { return nil }
func (r *TestRes) Metadata() interface{}                          { return nil }
func (r *TestRes) SetMetadata(interface{})                        {}
func (r *TestRes) SetWorld(World)                                 {}
func (r *TestRes) GetAutoEdges() ([]AutoEdge, error)              { return nil, nil }
func (r *TestRes) GetAutoGroup() (bool, error)                    { return false, nil }
func (r *TestRes) UID() string                                    { return r.kind + "/" + r.name }
func (r *TestRes) Reversible() (bool, error)                      { return false, nil }
func (r *TestRes) MetaParams() *MetaParams                        { return r.metaParams }
func (r *TestRes) SetMetaParams(p *MetaParams)                    { r.metaParams = p }
func (r *TestRes) SetRecv(recv map[string]*Send)                  { r.recv = recv }
func (r *TestRes) Recv() map[string]*Send                         { return r.recv }

var _ Res = (*TestRes)(nil)
var _ RecvableRes = (*TestRes)(nil)

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
	old.AddVertex(&TestRes{kind: "file", name: "/etc/hosts"})
	new.AddVertex(&TestRes{kind: "file", name: "/etc/hosts"})

	got, err := ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("ResGraphMapper() error = %v", err)
		return
	}
	if len(got) != 1 {
		t.Errorf("len(mapper) = %d, expected 1", len(got))
	}
	for newRes, oldRes := range got {
		if newRes.Kind() != oldRes.Kind() {
			t.Errorf("Kind mismatch: %s != %s", newRes.Kind(), oldRes.Kind())
		}
		if newRes.Name() != oldRes.Name() {
			t.Errorf("Name mismatch: %s != %s", newRes.Name(), oldRes.Name())
		}
	}
}

// TestResGraphMapper2 tests that a resource only in oldGraph produces no mapping.
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
	old.AddVertex(&TestRes{kind: "file", name: "/etc/old"})
	new.AddVertex(&TestRes{kind: "file", name: "/etc/new"})

	got, err := ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("ResGraphMapper() error = %v", err)
		return
	}
	if len(got) != 0 {
		t.Errorf("len(mapper) = %d, expected 0", len(got))
	}
}

// TestResGraphMapper3 tests that resources with the same name but different kind are not mapped.
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
	old.AddVertex(&TestRes{kind: "file", name: "foo"})
	new.AddVertex(&TestRes{kind: "svc", name: "foo"})

	got, err := ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("ResGraphMapper() error = %v", err)
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
	old.AddVertex(&TestRes{kind: "file", name: "/etc/hosts"})
	new.AddVertex(&TestRes{kind: "file", name: "/etc/hosts"})
	old.AddVertex(&TestRes{kind: "file", name: "/etc/resolv.conf"})
	new.AddVertex(&TestRes{kind: "pkg", name: "vim"})

	got, err := ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("ResGraphMapper() error = %v", err)
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

	got, err := ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("ResGraphMapper() error = %v", err)
		return
	}
	if len(got) != 0 {
		t.Errorf("len(mapper) = %d, expected 0", len(got))
	}
}

// kind and name are keyed separately, so "file|x"/"foo" and "file"/"x|foo" are distinct
func TestResGraphMapper6(t *testing.T) {
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
	// "file|x" + "|" + "foo" == "file" + "|" + "x|foo" — should not match
	old.AddVertex(&TestRes{kind: "file|x", name: "foo"})
	new.AddVertex(&TestRes{kind: "file", name: "x|foo"})

	got, err := ResGraphMapper(old, new)
	if err != nil {
		t.Errorf("ResGraphMapper() error = %v", err)
		return
	}
	if len(got) != 0 {
		t.Errorf("len(mapper) = %d, expected 0 (key collision)", len(got))
	}
}
