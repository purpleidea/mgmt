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

// +build !root

package core

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/pretty"
)

func TestPureFuncExec0(t *testing.T) {
	type test struct { // an individual test
		name     string
		funcname string
		args     []types.Value
		fail     bool
		expect   types.Value
	}
	testCases := []test{}

	//{
	//	testCases = append(testCases, test{
	//		name: "",
	//		funcname: "",
	//		args: []types.Value{
	//		},
	//		fail: false,
	//		expect: nil,
	//	})
	//}
	{
		testCases = append(testCases, test{
			name:     "strings.to_lower 0",
			funcname: "strings.to_lower",
			args: []types.Value{
				&types.StrValue{
					V: "HELLO",
				},
			},
			fail: false,
			expect: &types.StrValue{
				V: "hello",
			},
		})
	}
	{
		testCases = append(testCases, test{
			name:     "datetime.now fail",
			funcname: "datetime.now",
			args:     nil,
			fail:     true,
			expect:   nil,
		})
	}
	// TODO: run unification in PureFuncExec if it makes sense to do so...
	//{
	//	testCases = append(testCases, test{
	//		name:     "len 0",
	//		funcname: "len",
	//		args: []types.Value{
	//			&types.StrValue{
	//				V: "Hello, world!",
	//			},
	//		},
	//		fail: false,
	//		expect: &types.IntValue{
	//			V: 13,
	//		},
	//	})
	//}

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
		//if (index != 20 && index != 21) {
		//if tc.name != "nil" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, funcname, args, fail, expect := tc.name, tc.funcname, tc.args, tc.fail, tc.expect

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			f, err := funcs.Lookup(funcname)
			if err != nil {
				t.Errorf("test #%d: func lookup failed with: %+v", index, err)
				return
			}

			result, err := funcs.PureFuncExec(f, args)

			if !fail && err != nil {
				t.Errorf("test #%d: func failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: func passed, expected fail", index)
				return
			}
			if !fail && result == nil {
				t.Errorf("test #%d: func output was nil", index)
				return
			}

			if !reflect.DeepEqual(result, expect) {
				// double check because DeepEqual is different since the func exists
				diff := pretty.Compare(result, expect)
				if diff != "" { // bonus
					t.Errorf("test #%d: result did not match expected", index)
					// TODO: consider making our own recursive print function
					t.Logf("test #%d:   actual: \n\n%s\n", index, spew.Sdump(result))
					t.Logf("test #%d: expected: \n\n%s", index, spew.Sdump(expect))

					// more details, for tricky cases:
					diffable := &pretty.Config{
						Diffable:          true,
						IncludeUnexported: true,
						//PrintStringers: false,
						//PrintTextMarshalers: false,
						//SkipZeroFields: false,
					}
					t.Logf("test #%d:   actual: \n\n%s\n", index, diffable.Sprint(result))
					t.Logf("test #%d: expected: \n\n%s", index, diffable.Sprint(expect))
					t.Logf("test #%d: diff:\n%s", index, diff)
					return
				}
			}
		})
	}
}
