// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package util

import (
	"context"
	"os/user"
	"reflect"
	"strconv"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/types"
)

func TestUnknownGroup(t *testing.T) {
	gid, err := GetGID("unknowngroup")
	if err == nil {
		t.Errorf("expected failure, but passed with: %d", gid)
	}
}

func TestUnknownUser(t *testing.T) {
	uid, err := GetUID("unknownuser")
	if err == nil {
		t.Errorf("expected failure, but passed with: %d", uid)
	}
}

func TestCurrentUserGroupByName(t *testing.T) {
	// get current user
	userObj, err := user.Current()
	if err != nil {
		t.Errorf("error trying to lookup current user: %s", err.Error())
	}

	currentUID := userObj.Uid
	currentGID := userObj.Gid

	var uid int
	var gid int

	// now try to get the uid/gid via our API (via username and group name)
	if uid, err = GetUID(userObj.Username); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(uid) != currentUID {
		t.Errorf("uid didn't match current user's: %s vs %s", strconv.Itoa(uid), currentUID)
	}

	// macOS users do not have a group with their name on it, so not assuming this here
	group, err := user.LookupGroupId(currentGID)
	if err != nil {
		t.Errorf("failed to lookup group by id: %s", currentGID)
	}
	if gid, err = GetGID(group.Name); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(gid) != currentGID {
		t.Errorf("gid didn't match current user's: %s vs %s", strconv.Itoa(gid), currentGID)
	}
}

func TestCurrentUserGroupById(t *testing.T) {
	// get current user
	userObj, err := user.Current()
	if err != nil {
		t.Errorf("error trying to lookup current user: %s", err.Error())
	}

	currentUID := userObj.Uid
	currentGID := userObj.Gid

	var uid int
	var gid int

	// now try to get the uid/gid via our API (via uid and gid)
	if uid, err = GetUID(currentUID); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(uid) != currentUID {
		t.Errorf("uid didn't match current user's: %s vs %s", strconv.Itoa(uid), currentUID)
	}

	if gid, err = GetGID(currentGID); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(gid) != currentGID {
		t.Errorf("gid didn't match current user's: %s vs %s", strconv.Itoa(gid), currentGID)
	}
}

func TestStructTagToFieldName0(t *testing.T) {
	type foo struct {
		A string `lang:"aaa"`
		B bool   `lang:"bbb"`
		C int64  `lang:"ccc"`
	}
	f := &foo{ // a ptr!
		A: "hello",
		B: true,
		C: 13,
	}
	m, err := StructTagToFieldName(f) // (map[string]string, error)
	if err != nil {
		t.Errorf("got error: %+v", err)
		return
	}
	t.Logf("got output: %+v", m)
	expected := map[string]string{
		"aaa": "A",
		"bbb": "B",
		"ccc": "C",
	}
	if !reflect.DeepEqual(m, expected) {
		t.Errorf("unexpected result")
		return
	}
}

func TestStructTagToFieldName1(t *testing.T) {
	type foo struct {
		A string `lang:"aaa"`
		B bool   `lang:"bbb"`
		C int64  `lang:"ccc"`
	}
	f := foo{ // not a ptr!
		A: "hello",
		B: true,
		C: 13,
	}
	m, err := StructTagToFieldName(f) // (map[string]string, error)
	if err == nil {
		t.Errorf("expected error, got nil")
		//return
	}
	t.Logf("got output: %+v", m)
	t.Logf("got error: %+v", err)
}

func TestStructTagToFieldName2(t *testing.T) {
	m, err := StructTagToFieldName(nil) // (map[string]string, error)
	if err == nil {
		t.Errorf("expected error, got nil")
		//return
	}
	t.Logf("got output: %+v", m)
	t.Logf("got error: %+v", err)
}

type testEngineRes struct {
	PublicProp1  string                      `lang:"PublicProp1" yaml:"PublicProp1"`
	PublicProp2  map[string][]map[string]int `lang:"PublicProp2" yaml:"PublicProp2"`
	privateProp1 bool
	privateProp2 []int
}

func (t *testEngineRes) CheckApply(context.Context, bool) (bool, error) { return false, nil }

func (t *testEngineRes) Cleanup() error { return nil }

func (t *testEngineRes) Cmp(engine.Res) error { return nil }

func (t *testEngineRes) Default() engine.Res { return t }

func (t *testEngineRes) Init(*engine.Init) error { return nil }

func (t *testEngineRes) Kind() string { return "test-kind" }

func (t *testEngineRes) MetaParams() *engine.MetaParams { return nil }

func (t *testEngineRes) Name() string { return "test-name" }

func (t *testEngineRes) SetKind(string) {}

func (t *testEngineRes) SetMetaParams(*engine.MetaParams) {}

func (t *testEngineRes) SetName(string) {}

func (t *testEngineRes) String() string { return "test-string" }

func (t *testEngineRes) Validate() error { return nil }

func (t *testEngineRes) Watch(context.Context) error { return nil }

func TestLangFieldNameToStructType(t *testing.T) {
	k := "test-kind"

	engine.RegisterResource(k, func() engine.Res { return &testEngineRes{} })
	res, err := LangFieldNameToStructType(k)

	expected := map[string]*types.Type{
		"PublicProp1": types.TypeStr,
		"PublicProp2": {
			Kind: types.KindMap,
			Key:  types.TypeStr,
			Val: &types.Type{
				Kind: types.KindList,
				Val: &types.Type{
					Kind: types.KindMap,
					Key:  types.TypeStr,
					Val:  types.TypeInt,
				},
			},
		},
	}
	if err != nil {
		t.Errorf("error trying to get the field name type map: %s", err.Error())
		return
	}
	if !reflect.DeepEqual(res, expected) {
		t.Errorf("unexpected result: %+v", res)
		return
	}
}
