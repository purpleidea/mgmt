// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

//go:build !root

package ast

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"
)

func TestScopeIndexesPush0(t *testing.T) {
	type test struct { // an individual test
		name     string
		indexes  map[int][]interfaces.Expr
		pushed   []interfaces.Expr
		expected map[int][]interfaces.Expr
	}
	testCases := []test{}

	//{
	//	testCases = append(testCases, test{
	//		name: "empty",
	//		pushed: nil, // TODO: undefined, but should we do it?
	//		expected: map[int][]interfaces.Expr{
	//			0: {}, // empty list ?
	//		},
	//	})
	//}
	{
		testCases = append(testCases, test{
			name:   "empty list",
			pushed: []interfaces.Expr{}, // empty list
			expected: map[int][]interfaces.Expr{
				0: {}, // empty list
			},
		})
	}
	{
		b1 := &ExprBool{}
		b2 := &ExprBool{}
		b3 := &ExprBool{}
		b4 := &ExprBool{}
		b5 := &ExprBool{}
		b6 := &ExprBool{}
		b7 := &ExprBool{}
		b8 := &ExprBool{}
		testCases = append(testCases, test{
			name: "simple push",
			indexes: map[int][]interfaces.Expr{
				0: {
					b1, b2, b3,
				},
				1: {
					b4,
				},
				2: {
					b5, b6,
				},
			},
			pushed: []interfaces.Expr{
				b7, b8,
			},
			expected: map[int][]interfaces.Expr{
				0: {
					b7, b8,
				},
				1: {
					b1, b2, b3,
				},
				2: {
					b4,
				},
				3: {
					b5, b6,
				},
			},
		})
	}
	{
		b1 := &ExprBool{}
		b2 := &ExprBool{}
		b3 := &ExprBool{}
		b4 := &ExprBool{}
		b5 := &ExprBool{}
		b6 := &ExprBool{}
		b7 := &ExprBool{}
		b8 := &ExprBool{}
		testCases = append(testCases, test{
			name: "push with gaps",
			indexes: map[int][]interfaces.Expr{
				0: {
					b1, b2, b3,
				},
				// there is a gap here
				2: {
					b4,
				},
				3: {
					b5, b6,
				},
			},
			pushed: []interfaces.Expr{
				b7, b8,
			},
			expected: map[int][]interfaces.Expr{
				0: {
					b7, b8,
				},
				// the gap remains
				1: {
					b1, b2, b3,
				},
				3: {
					b4,
				},
				4: {
					b5, b6,
				},
			},
		})
	}
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
			name, indexes, pushed, expected := tc.name, tc.indexes, tc.pushed, tc.expected

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			scope := &interfaces.Scope{
				Indexes: indexes,
			}
			scope.PushIndexes(pushed)
			out := scope.Indexes

			if !reflect.DeepEqual(out, expected) {
				t.Errorf("test #%d: indexes did not match expected", index)
				t.Logf("test #%d:   actual: \n\n%+v\n", index, out)
				t.Logf("test #%d: expected: \n\n%+v", index, expected)
				return
			}
		})
	}
}
