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
	"os/user"
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

// fakeGroupCmd records one RunCmd invocation.
type fakeGroupCmd struct {
	Name string
	Args []string
}

// fakeGroupFuncs serves group lookups from in-memory tables and records every
// RunCmd call. Lookup tables are not mutated by RunCmd; the goal is to assert
// what command would be issued, given a particular system state.
type fakeGroupFuncs struct {
	byName map[string]*user.Group
	byID   map[string]*user.Group

	cmds []fakeGroupCmd
}

func (obj *fakeGroupFuncs) lookupGroup(name string) (*user.Group, error) {
	if g, ok := obj.byName[name]; ok {
		return g, nil
	}
	return nil, user.UnknownGroupError(name)
}

func (obj *fakeGroupFuncs) lookupGroupId(gid string) (*user.Group, error) {
	if g, ok := obj.byID[gid]; ok {
		return g, nil
	}
	return nil, user.UnknownGroupIdError(gid)
}

func (obj *fakeGroupFuncs) runCmd(_ context.Context, name string, args []string) error {
	obj.cmds = append(obj.cmds, fakeGroupCmd{Name: name, Args: append([]string(nil), args...)})
	return nil
}

// addGroup registers a group, indexed by both name and GID.
func (obj *fakeGroupFuncs) addGroup(g *user.Group) {
	if obj.byName == nil {
		obj.byName = map[string]*user.Group{}
		obj.byID = map[string]*user.Group{}
	}
	obj.byName[g.Name] = g
	obj.byID[g.Gid] = g
}

// install replaces defaultGroupFuncs with the fake for the duration of the
// test; the original is restored via t.Cleanup.
func (obj *fakeGroupFuncs) install(t *testing.T) {
	t.Helper()
	orig := defaultGroupFuncs
	defaultGroupFuncs = groupFuncs{
		LookupGroup:   obj.lookupGroup,
		LookupGroupId: obj.lookupGroupId,
		RunCmd:        obj.runCmd,
	}
	t.Cleanup(func() { defaultGroupFuncs = orig })
}

// fakeGroupInit returns the minimal *engine.Init that CheckApply needs.
func fakeGroupInit(t *testing.T) *engine.Init {
	return &engine.Init{
		Debug: testing.Verbose(),
		Logf:  func(format string, v ...interface{}) { t.Logf("group: "+format, v...) },
	}
}

// mkGroup builds a *GroupRes with name and state pre-set, plus any per-test
// tweaks applied by opts.
func mkGroup(name, state string, opts ...func(*GroupRes)) *GroupRes {
	r := &GroupRes{State: state}
	r.SetName(name)
	r.SetKind("group")
	for _, o := range opts {
		o(r)
	}
	return r
}

// TestGroupCheckApply_AbsentSkipsGIDConflict guards against the GID-uniqueness
// check firing for groups we want absent. If the resource asks for `doomed` to
// be absent, the result should delete it -- even if obj.GID is set and some
// other group happens to hold that GID. Previously the GID-conflict check ran
// regardless of state and would error with "the requested GID belongs to
// another group", masking the intended absent semantics.
func TestGroupCheckApply_AbsentSkipsGIDConflict(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.addGroup(&user.Group{Gid: "1000", Name: "wheel"})  // holds GID 1000
	f.addGroup(&user.Group{Gid: "1234", Name: "doomed"}) // the group to delete
	f.install(t)

	// doomed should be absent; the GID 1000 we (pointlessly) request is held
	// by wheel, but that must not block deletion.
	res := mkGroup("doomed", "absent", func(r *GroupRes) { r.GID = u32Ptr(1000) })
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false on delete; got true")
	}
	want := fakeGroupCmd{Name: "groupdel", Args: []string{"doomed"}}
	if len(f.cmds) != 1 || !reflect.DeepEqual(f.cmds[0], want) {
		t.Errorf("expected groupdel doomed; got %v", f.cmds)
	}
}

// TestGroupCheckApply_CreateGIDConflict asserts that creating a brand-new group
// with a GID already held by a different group is rejected up front, rather
// than relying on groupadd to fail. Previously the GID-conflict check was
// nested inside the `exists` branch, so creating a group never checked whether
// the requested GID was free and would just run `groupadd -g`.
func TestGroupCheckApply_CreateGIDConflict(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.addGroup(&user.Group{Gid: "1000", Name: "wheel"}) // GID 1000 taken
	f.install(t)

	// newgrp does not exist yet and requests the taken GID 1000.
	res := mkGroup("newgrp", "exists", func(r *GroupRes) { r.GID = u32Ptr(1000) })
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err == nil {
		t.Errorf("expected error for taken GID, got nil")
	}
	if checkOK {
		t.Errorf("expected checkOK=false on GID conflict, got true")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands on GID conflict, got %v", f.cmds)
	}
}

// TestGroupCheckApply_ExistsNoOp pins the no-op case: an existing group with no
// GID requested needs no change.
func TestGroupCheckApply_ExistsNoOp(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.addGroup(&user.Group{Gid: "1000", Name: "wheel"})
	f.install(t)

	res := mkGroup("wheel", "exists")
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected no-op (checkOK=true); got false")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}

// TestGroupCheckApply_ExistsMatchingGID is a no-op when the existing group
// already has the requested GID.
func TestGroupCheckApply_ExistsMatchingGID(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.addGroup(&user.Group{Gid: "1000", Name: "wheel"})
	f.install(t)

	res := mkGroup("wheel", "exists", func(r *GroupRes) { r.GID = u32Ptr(1000) })
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected no-op (checkOK=true); got false")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}

// TestGroupCheckApply_ModifyGID changes the GID of an existing group via
// groupmod when the current GID differs and the requested one is free.
func TestGroupCheckApply_ModifyGID(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.addGroup(&user.Group{Gid: "1000", Name: "wheel"})
	f.install(t)

	res := mkGroup("wheel", "exists", func(r *GroupRes) { r.GID = u32Ptr(2000) })
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false (GID differs); got true")
	}
	want := fakeGroupCmd{Name: "groupmod", Args: []string{"wheel", "-g", "2000"}}
	if len(f.cmds) != 1 || !reflect.DeepEqual(f.cmds[0], want) {
		t.Errorf("expected %v; got %v", want, f.cmds)
	}
}

// TestGroupCheckApply_Create adds a brand-new group with a free GID.
func TestGroupCheckApply_Create(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.install(t)

	res := mkGroup("newgrp", "exists", func(r *GroupRes) { r.GID = u32Ptr(1234) })
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false on create; got true")
	}
	want := fakeGroupCmd{Name: "groupadd", Args: []string{"newgrp", "-g", "1234"}}
	if len(f.cmds) != 1 || !reflect.DeepEqual(f.cmds[0], want) {
		t.Errorf("expected %v; got %v", want, f.cmds)
	}
}

// TestGroupCheckApply_Delete removes an existing group.
func TestGroupCheckApply_Delete(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.addGroup(&user.Group{Gid: "1000", Name: "wheel"})
	f.install(t)

	res := mkGroup("wheel", "absent")
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if checkOK {
		t.Errorf("expected checkOK=false on delete; got true")
	}
	want := fakeGroupCmd{Name: "groupdel", Args: []string{"wheel"}}
	if len(f.cmds) != 1 || !reflect.DeepEqual(f.cmds[0], want) {
		t.Errorf("expected %v; got %v", want, f.cmds)
	}
}

// TestGroupCheckApply_AbsentAlready is a no-op when the group is already gone.
func TestGroupCheckApply_AbsentAlready(t *testing.T) {
	f := &fakeGroupFuncs{}
	f.install(t)

	res := mkGroup("ghost", "absent")
	if err := res.Init(fakeGroupInit(t)); err != nil {
		t.Fatal(err)
	}

	checkOK, err := res.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckApply: unexpected error: %v", err)
	}
	if !checkOK {
		t.Errorf("expected checkOK=true for already-absent group; got false")
	}
	if len(f.cmds) != 0 {
		t.Errorf("expected no commands; got %v", f.cmds)
	}
}
