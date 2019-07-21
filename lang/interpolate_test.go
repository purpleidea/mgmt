// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package lang

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/pretty"
)

func TestInterpolate0(t *testing.T) {
	type test struct { // an individual test
		name string
		code string
		fail bool
		ast  interfaces.Stmt
	}
	testCases := []test{}
	// NOTE: to run an individual test, first run: `go test -v` to list the
	// names, and then run `go test -run <pattern>` with the name(s) to run.

	{
		ast := &StmtProg{
			Prog: []interfaces.Stmt{},
		}
		testCases = append(testCases, test{ // 0
			"nil",
			``,
			false,
			ast,
		})
	}
	{
		ast := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "basic string",
			code: `
			test "t1" {
				stringptr => "foo",
			}
			`,
			fail: false,
			ast:  ast,
		})
	}
	{
		fieldName := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "foo-",
				},
				&ExprVar{
					Name: "x",
				},
			},
		}
		ast := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: fieldName,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "basic expansion",
			code: `
			#$x = "hello"	# not actually needed to test interpolation
			test "t1" {
				stringptr => "foo-${x}",
			}
			`,
			fail: false,
			ast:  ast,
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
			code, fail, exp := tc.code, tc.fail, tc.ast

			str := strings.NewReader(code)
			ast, err := LexParse(str)
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			t.Logf("test #%d: AST: %+v", index, ast)

			data := &interfaces.Data{
				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					t.Logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			iast, err := ast.Interpolate()
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate expected error, not nil", index)
				return
			}

			// init exp so that the match will look identical...
			if !fail {
				if err := exp.Init(data); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: match init failed with: %+v", index, err)
					return
				}
			}

			if reflect.DeepEqual(iast, exp) {
				return
			}
			// double check because DeepEqual is different since the logf exists
			diff := pretty.Compare(iast, exp)
			if diff == "" { // bonus
				return
			}
			t.Errorf("test #%d: AST did not match expected", index)
			// TODO: consider making our own recursive print function
			t.Logf("test #%d:   actual: \n%s", index, spew.Sdump(iast))
			t.Logf("test #%d: expected: \n%s", index, spew.Sdump(exp))
			t.Logf("test #%d: diff:\n%s", index, diff)
		})
	}
}

func TestInterpolateBasicStmt(t *testing.T) {
	type test struct { // an individual test
		name string
		ast  interfaces.Stmt
		fail bool
		exp  interfaces.Stmt
	}
	testCases := []test{}

	// this causes a panic, so it can't be used
	//{
	//	testCases = append(testCases, test{
	//		"nil",
	//		nil,
	//		false,
	//		nil,
	//	})
	//}
	{
		ast := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "basic resource",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	{
		ast := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t${blah}",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		resName := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "t",
				},
				&ExprVar{
					Name: "blah",
				},
			},
		}
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: resName,
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "expanded resource",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	{
		ast := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t${42}", // incorrect type
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		resName := &ExprCall{
			Name: operatorFuncName,
			// incorrect sig for this function, but correct interpolation
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "t",
				},
				&ExprInt{
					V: 42,
				},
			},
		}
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: resName,
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: &ExprStr{
								V: "foo",
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "expanded invalid resource name",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			ast, fail, exp := tc.ast, tc.fail, tc.exp

			data := &interfaces.Data{
				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					t.Logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			iast, err := ast.Interpolate()
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate expected error, not nil", index)
				return
			}

			// init exp so that the match will look identical...
			if !fail {
				if err := exp.Init(data); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: match init failed with: %+v", index, err)
					return
				}
			}

			if reflect.DeepEqual(iast, exp) {
				return
			}
			// double check because DeepEqual is different since the logf exists
			diff := pretty.Compare(iast, exp)
			if diff == "" { // bonus
				return
			}
			t.Errorf("test #%d: AST did not match expected", index)
			// TODO: consider making our own recursive print function
			t.Logf("test #%d:   actual: \n%s", index, spew.Sdump(iast))
			t.Logf("test #%d: expected: \n%s", index, spew.Sdump(exp))
			t.Logf("test #%d: diff:\n%s", index, diff)
		})
	}
}

func TestInterpolateBasicExpr(t *testing.T) {
	type test struct { // an individual test
		name string
		ast  interfaces.Expr
		fail bool
		exp  interfaces.Expr
	}
	testCases := []test{}

	// this causes a panic, so it can't be used
	//{
	//	testCases = append(testCases, test{
	//		"nil",
	//		nil,
	//		false,
	//		nil,
	//	})
	//}
	{
		ast := &ExprStr{
			V: "hello",
		}
		exp := &ExprStr{
			V: "hello",
		}
		testCases = append(testCases, test{
			name: "basic string",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	{
		ast := &ExprStr{
			V: "hello ${person_name}",
		}
		exp := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "hello ",
				},
				&ExprVar{
					Name: "person_name",
				},
			},
		}
		testCases = append(testCases, test{
			name: "basic expansion",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	{
		ast := &ExprStr{
			V: "hello ${x ${y} z}",
		}
		testCases = append(testCases, test{
			name: "invalid expansion",
			ast:  ast,
			fail: true,
		})
	}
	// TODO: patterns like what are shown below are supported by the `hil`
	// library, but are not yet supported by our translation layer, nor do
	// they necessarily work or make much sense at this point in time...
	//{
	//	ast := &ExprStr{
	//		V: `hello ${func("hello ${var.foo}")}`,
	//	}
	//	exp := nil // TODO: add this
	//	testCases = append(testCases, test{
	//		name: "double expansion",
	//		ast:  ast,
	//		fail: false,
	//		exp:  exp,
	//	})
	//}
	{
		ast := &ExprStr{
			V: "sweetie${3.14159}", // invalid but only at type check
		}
		exp := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "sweetie",
				},
				&ExprFloat{
					V: 3.14159,
				},
			},
		}
		testCases = append(testCases, test{
			name: "float expansion",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	{
		ast := &ExprStr{
			V: "i am: ${sys.hostname()}",
		}
		exp := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "i am: ",
				},
				&ExprCall{
					Name: "sys.hostname",
					Args: []interfaces.Expr{},
				},
			},
		}
		testCases = append(testCases, test{
			name: "function expansion",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	{
		ast := &ExprStr{
			V: "i am: ${blah(21, 12.3)}",
		}
		exp := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprStr{
					V: "i am: ",
				},
				&ExprCall{
					Name: "blah",
					Args: []interfaces.Expr{
						&ExprInt{
							V: 21,
						},
						&ExprFloat{
							V: 12.3,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "function expansion arg",
			ast:  ast,
			fail: false,
			exp:  exp,
		})
	}
	// FIXME: i am broken, i don't deal well with negatives for some reason
	//{
	//	ast := &ExprStr{
	//		V: "i am: ${blah(21, -12.3)}",
	//	}
	//	exp := &ExprCall{
	//		Name: operatorFuncName,
	//		Args: []interfaces.Expr{
	//			&ExprStr{
	//				V: "+",
	//			},
	//			&ExprStr{
	//				V: "i am: ",
	//			},
	//			&ExprCall{
	//				Name: "blah",
	//				Args: []interfaces.Expr{
	//					&ExprInt{
	//						V: 21,
	//					},
	//					&ExprFloat{
	//						V: -12.3,
	//					},
	//				},
	//			},
	//		},
	//	}
	//	testCases = append(testCases, test{
	//		name: "function expansion arg negative",
	//		ast:  ast,
	//		fail: false,
	//		exp:  exp,
	//	})
	//}
	// FIXME: i am broken :(
	//{
	//	ast := &ExprStr{
	//		V: "sweetie${-3.14159}", // FIXME: only the negative breaks this
	//	}
	//	exp := &ExprCall{
	//		Name: operatorFuncName,
	//		Args: []interfaces.Expr{
	//			&ExprStr{
	//				V: "+",
	//			},
	//			&ExprStr{
	//				V: "sweetie",
	//			},
	//			&ExprFloat{
	//				V: -3.14159,
	//			},
	//		},
	//	}
	//	testCases = append(testCases, test{
	//		name: "negative float expansion",
	//		ast:  ast,
	//		fail: false,
	//		exp:  exp,
	//	})
	//}
	// FIXME: i am also broken, but less important
	//{
	//	ast := &ExprStr{
	//		V: `i am: ${blah(42, "${foo}")}`,
	//	}
	//	exp := &ExprCall{
	//		Name: operatorFuncName,
	//		Args: []interfaces.Expr{
	//			&ExprStr{
	//				V: "+",
	//			},
	//			&ExprStr{
	//				V: "i am: ",
	//			},
	//			&ExprCall{
	//				Name: "blah",
	//				Args: []interfaces.Expr{
	//					&ExprInt{
	//						V: 42,
	//					},
	//					&ExprVar{
	//						Name: "foo",
	//					},
	//				},
	//			},
	//		},
	//	}
	//	testCases = append(testCases, test{
	//		name: "function expansion arg with var",
	//		ast:  ast,
	//		fail: false,
	//		exp:  exp,
	//	})
	//}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			ast, fail, exp := tc.ast, tc.fail, tc.exp

			data := &interfaces.Data{
				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					t.Logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			iast, err := ast.Interpolate()
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate expected error, not nil", index)
				return
			}

			// init exp so that the match will look identical...
			if !fail {
				if err := exp.Init(data); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: match init failed with: %+v", index, err)
					return
				}
			}

			if reflect.DeepEqual(iast, exp) {
				return
			}
			// double check because DeepEqual is different since the logf exists
			diff := pretty.Compare(iast, exp)
			if diff == "" { // bonus
				return
			}
			t.Errorf("test #%d: AST did not match expected", index)
			// TODO: consider making our own recursive print function
			t.Logf("test #%d:   actual: \n%s", index, spew.Sdump(iast))
			t.Logf("test #%d: expected: \n%s", index, spew.Sdump(exp))
			t.Logf("test #%d: diff:\n%s", index, diff)
		})
	}
}
