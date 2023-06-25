// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

package unification

import (
	"fmt"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
)

func TestSimpleSolver1(t *testing.T) {
	type test struct { // an individual test
		name       string
		invariants []interfaces.Invariant
		expected   []interfaces.Expr
		fail       bool
		expect     map[interfaces.Expr]*types.Type
		experr     error  // expected error if fail == true (nil ignores it)
		experrstr  string // expected error prefix
	}
	testCases := []test{}

	{
		expr := &ast.ExprStr{V: "hello"}

		invariants := []interfaces.Invariant{
			&interfaces.EqualsInvariant{
				Expr: expr,
				Type: types.NewType("str"),
			},
		}

		invars, err := expr.Unify()
		if err != nil {
			panic("bad test")
		}
		invariants = append(invariants, invars...)

		testCases = append(testCases, test{
			name:       "simple str",
			invariants: invariants,
			expected: []interfaces.Expr{
				expr,
			},
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				expr: types.TypeStr,
			},
		})
	}
	{
		expr := &ast.ExprStr{V: "hello"}

		invariants := []interfaces.Invariant{
			&interfaces.EqualsInvariant{
				Expr: expr,
				Type: types.NewType("int"),
			},
		}

		invars, err := expr.Unify()
		if err != nil {
			panic("bad test")
		}
		invariants = append(invariants, invars...)

		testCases = append(testCases, test{
			name:       "simple fail",
			invariants: invariants,
			expected: []interfaces.Expr{
				expr,
			},
			fail: true,
			//experr: ErrAmbiguous,
		})
	}
	{
		// ?1 = func(x ?2) ?3
		// ?1 = func(arg0 str) ?4
		// ?3 = str # needed since we don't know what the func body is
		expr1 := &interfaces.ExprAny{} // ?1
		expr2 := &interfaces.ExprAny{} // ?2
		expr3 := &interfaces.ExprAny{} // ?3
		expr4 := &interfaces.ExprAny{} // ?4

		arg0 := &interfaces.ExprAny{} // arg0

		invarA := &interfaces.EqualityWrapFuncInvariant{
			Expr1: expr1, // Expr
			Expr2Map: map[string]interfaces.Expr{ // map[string]Expr
				"x": expr2,
			},
			Expr2Ord: []string{"x"}, // []string
			Expr2Out: expr3,         // Expr
		}

		invarB := &interfaces.EqualityWrapFuncInvariant{
			Expr1: expr1, // Expr
			Expr2Map: map[string]interfaces.Expr{ // map[string]Expr
				"arg0": arg0,
			},
			Expr2Ord: []string{"arg0"}, // []string
			Expr2Out: expr4,            // Expr
		}

		invarC := &interfaces.EqualsInvariant{
			Expr: expr3,
			Type: types.NewType("str"),
		}

		invarD := &interfaces.EqualsInvariant{
			Expr: arg0,
			Type: types.NewType("str"),
		}

		testCases = append(testCases, test{
			name: "dual functions",
			invariants: []interfaces.Invariant{
				invarA,
				invarB,
				invarC,
				invarD,
			},
			expected: []interfaces.Expr{
				expr1,
				expr2,
				expr3,
				expr4,
				arg0,
			},
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				expr1: types.NewType("func(str) str"),
				expr2: types.NewType("str"),
				expr3: types.NewType("str"),
				expr4: types.NewType("str"),
				arg0:  types.NewType("str"),
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
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			invariants, expected, fail, expect, experr, experrstr := tc.invariants, tc.expected, tc.fail, tc.expect, tc.experr, tc.experrstr

			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			}
			debug := testing.Verbose()

			solver := SimpleInvariantSolverLogger(logf) // generates a solver with built-in logging

			solution, err := solver(invariants, expected)
			t.Logf("test #%d: solver completed with: %+v", index, err)

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: solver failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: solver passed, expected fail", index)
				return
			}
			if fail && experr != nil && err != experr { // test for specific error!
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected fail, got wrong error", index)
				t.Errorf("test #%d: got error: %+v", index, err)
				t.Errorf("test #%d: exp error: %+v", index, experr)
				return
			}

			if fail && err != nil {
				t.Logf("test #%d: err: %+v", index, err)
			}
			// test for specific error string!
			if fail && experrstr != "" && !strings.HasPrefix(err.Error(), experrstr) {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected fail, got wrong error", index)
				t.Errorf("test #%d: got error: %s", index, err.Error())
				t.Errorf("test #%d: exp error: %s", index, experrstr)
				return
			}

			if expect == nil { // map[interfaces.Expr]*types.Type
				return
			}

			solutions := solution.Solutions
			if debug {
				t.Logf("\n\ntest #%d: solutions: %+v\n", index, solutions)
			}

			solutionsMap := make(map[interfaces.Expr]*types.Type)
			for _, invar := range solutions {
				solutionsMap[invar.Expr] = invar.Type
			}

			var failed bool
			// TODO: do this in sorted order
			for expr, exptyp := range expect {
				typ, exists := solutionsMap[expr]
				if !exists {
					t.Errorf("test #%d: solution missing for: %+v", index, expr)
					failed = true
					break
				}
				if err := exptyp.Cmp(typ); err != nil {
					t.Errorf("test #%d: solutions type cmp failed with: %+v", index, err)
					t.Logf("test #%d: got: %+v", index, exptyp)
					t.Logf("test #%d: exp: %+v", index, typ)
					failed = true
					break
				}
			}
			if failed {
				return
			}
		})
	}
}
