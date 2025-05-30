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

package util

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
)

func TestUnify1(t *testing.T) {
	typ1 := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}
	typ2 := types.TypeStr

	t.Logf("before: %v -- %v", typ1, typ2)

	if err := Unify(typ1, typ2); err != nil {
		t.Errorf("unify failed: %+v", err)
		return
	}

	t1, t2 := Extract(typ1), Extract(typ2)
	t.Logf("after: %v -- %v", t1, t2)

	if err := t1.Cmp(t2); err != nil {
		t.Errorf("cmp failed: %+v", err)
		return
	}
}

func TestUnify2(t *testing.T) {
	typ1 := types.TypeStr
	typ2 := types.TypeStr

	t.Logf("before: %v -- %v", typ1, typ2)

	if err := Unify(typ1, typ2); err != nil {
		t.Errorf("unify failed: %+v", err)
		return
	}

	t1, t2 := Extract(typ1), Extract(typ2)
	t.Logf("after: %v -- %v", t1, t2)

	if err := t1.Cmp(t2); err != nil {
		t.Errorf("cmp failed: %+v", err)
		return
	}
}

func TestUnify3(t *testing.T) {
	typ1 := types.TypeBool
	typ2 := types.TypeStr

	t.Logf("before: %v -- %v", typ1, typ2)

	if err := Unify(typ1, typ2); err == nil {
		t.Errorf("unify didn't fail")
		return
	}
}

func TestUnify4(t *testing.T) {
	uni1 := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}

	typ1 := &types.Type{
		Kind: types.KindList,
		Val:  uni1,
	}
	typ2 := types.TypeListStr

	t.Logf("before: %v -- %v", typ1, typ2)
	t.Logf("before (uni.Data): %v", uni1.Uni.Data)

	if err := Unify(typ1, typ2); err != nil {
		t.Errorf("unify failed: %+v", err)
		return
	}

	t1, t2 := Extract(typ1), Extract(typ2)
	//t1, t2 := typ1, typ2
	t.Logf("after: %v -- %v", t1, t2)
	//t.Logf("after (uni1): %v", uni1)
	//t.Logf("after (uni1.Uni): %v", uni1.Uni)
	//t.Logf("after (uni1.Uni.Data): %v", uni1.Uni.Data)
	//t.Logf("after (typ): %v -- %v", typ1, typ2)

	if err := t1.Cmp(t2); err != nil {
		t.Errorf("cmp failed: %+v", err)
		return
	}
}

func TestUnifyTable(t *testing.T) {
	type test struct { // an individual test
		name string
		typ1 *types.Type
		typ2 *types.Type
		fail bool
		exp  *types.Type // concrete type without unification variables
	}
	testCases := []test{}

	testCases = append(testCases, test{ // 0
		"nil",
		nil,
		nil,
		true,
		nil,
	})
	testCases = append(testCases, test{
		name: "two strings",
		typ1: types.TypeStr,
		typ2: types.TypeStr,
		fail: false,
	})
	testCases = append(testCases, test{
		name: "different simple types",
		typ1: types.TypeStr,
		typ2: types.TypeBool,
		fail: true,
	})
	testCases = append(testCases, test{
		name: "two lists",
		typ1: types.TypeListStr,
		typ2: types.TypeListStr,
		fail: false,
	})
	testCases = append(testCases, test{
		name: "two lists, one elem",
		typ1: types.TypeListStr,     // []str
		typ2: types.NewType("[]?1"), // []?1
		fail: false,
	})
	testCases = append(testCases, test{
		name: "two functions",
		typ1: types.NewType("func([]str, ?42, float, int) ?42"),
		typ2: types.NewType("func(?13, bool, ?4, int) ?42"),
		fail: false,
		exp:  types.NewType("func([]str, bool, float, int) bool"),
	})

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "nil" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, typ1, typ2, fail, exp := tc.name, tc.typ1, tc.typ2, tc.fail, tc.exp

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			t.Logf("test #%d: Unify: %v -- %v", index, typ1, typ2)
			err := Unify(typ1, typ2)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Unify failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Unify passed, expected fail", index)
				return
			}

			if fail { // can't compare types if it failed
				//t.Logf("test #%d: PASS", index)
				t.Logf("test #%d: Unify failed, expected fail", index)
				return
			}

			t1, t2 := Extract(typ1), Extract(typ2)
			t.Logf("test #%d: Extract: %v -- %v", index, t1, t2)

			if err := t1.Cmp(t2); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: cmp error:\n%v", index, err)
				return
			}
			if exp != nil {
				if t1.HasUni() {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: has uni: %+v", index, t1)
					return
				}
				if exp.HasUni() {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: exp has uni: %+v", index, exp)
					return
				}
				if err := exp.Cmp(t1); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: cmp error:\n%v", index, err)
					return
				}
			}
			//t.Logf("test #%d: PASS", index)
			//t.Logf("test #%d: Unify passed, cmp passed", index)
		})
	}
}

func TestExtract1(t *testing.T) {
	uni1 := &types.Type{
		Kind: types.KindUnification,
		Uni:  types.NewElem(), // unification variable, eg: ?1
	}

	typ1 := &types.Type{
		Kind: types.KindList,
		Val:  uni1,
	}
	typ2 := types.TypeListStr

	t.Logf("before: %v -- %v", typ1, typ2)
	t.Logf("before (uni.Data): %v", uni1.Uni.Data)

	if err := Unify(typ1, typ2); err != nil {
		t.Errorf("unify failed: %+v", err)
		return
	}

	typ := Extract(typ1) // should produce a []str

	if err := typ.Cmp(typ2); err != nil {
		t.Errorf("cmp failed: %+v", err)
		return
	}
}
