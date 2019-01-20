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

package lang

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/util"
)

func TestUnification1(t *testing.T) {
	type test struct { // an individual test
		name   string
		ast    interfaces.Stmt // raw AST
		fail   bool
		expect map[interfaces.Expr]*types.Type
		experr error // expected error if fail == true (nil ignores it)
	}
	testCases := []test{}

	// this causes a panic, so it can't be used
	//{
	//	testCases = append(testCases, test{
	//		"nil",
	//		nil,
	//		true, // expect error
	//		nil,  // no AST
	//	})
	//}
	{
		expr := &ExprStr{V: "hello"}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "t1"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "str",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "one res",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				expr: types.TypeStr,
			},
		})
	}
	{
		v1 := &ExprStr{}
		v2 := &ExprStr{}
		v3 := &ExprStr{}
		expr := &ExprList{
			Elements: []interfaces.Expr{
				v1,
				v2,
				v3,
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "test"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "slicestring",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "list of strings",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				v1:   types.TypeStr,
				v2:   types.TypeStr,
				v3:   types.TypeStr,
				expr: types.NewType("[]str"),
			},
		})
	}
	{
		k1 := &ExprInt{}
		k2 := &ExprInt{}
		k3 := &ExprInt{}
		v1 := &ExprFloat{}
		v2 := &ExprFloat{}
		v3 := &ExprFloat{}
		expr := &ExprMap{
			KVs: []*ExprMapKV{
				{Key: k1, Val: v1},
				{Key: k2, Val: v2},
				{Key: k3, Val: v3},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "test"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "mapintfloat",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "map of int->float",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				k1:   types.TypeInt,
				k2:   types.TypeInt,
				k3:   types.TypeInt,
				v1:   types.TypeFloat,
				v2:   types.TypeFloat,
				v3:   types.TypeFloat,
				expr: types.NewType("map{int: float}"),
			},
		})
	}
	{
		b := &ExprBool{}
		s := &ExprStr{}
		i := &ExprInt{}
		f := &ExprFloat{}
		expr := &ExprStruct{
			Fields: []*ExprStructField{
				{Name: "somebool", Value: b},
				{Name: "somestr", Value: s},
				{Name: "someint", Value: i},
				{Name: "somefloat", Value: f},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "test"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "mixedstruct",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple struct",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				b:    types.TypeBool,
				s:    types.TypeStr,
				i:    types.TypeInt,
				f:    types.TypeFloat,
				expr: types.NewType("struct{somebool bool; somestr str; someint int; somefloat float}"),
			},
		})
	}
	{
		// test "n1" {
		//	int64ptr => 13 + 42,
		//}
		expr := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprInt{
					V: 13,
				},

				&ExprInt{
					V: 42,
				},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "n1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "int64ptr",
							Value: expr, // func
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				expr: types.NewType("int"),
			},
		})
	}
	{
		//test "n1" {
		//	int64ptr => 13 + 42 - 4,
		//}
		innerFunc := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "-",
				},
				&ExprInt{
					V: 42,
				},
				&ExprInt{
					V: 4,
				},
			},
		}
		expr := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprInt{
					V: 13,
				},
				innerFunc, // nested func, can we unify?
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "n1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "int64ptr",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call, multiple ints",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				innerFunc: types.NewType("int"),
				expr:      types.NewType("int"),
			},
		})
	}
	{
		//test "n1" {
		//	float32 => -25.38789 + 32.6 + 13.7,
		//}
		innerFunc := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprFloat{
					V: 32.6,
				},
				&ExprFloat{
					V: 13.7,
				},
			},
		}
		expr := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "+",
				},
				&ExprFloat{
					V: -25.38789,
				},
				innerFunc, // nested func, can we unify?
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "n1",
					},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "float32",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call, multiple floats",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				innerFunc: types.NewType("float"),
				expr:      types.NewType("float"),
			},
		})
	}
	{
		//$x = 42 - 13
		innerFunc := &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{
					V: "-",
				},
				&ExprInt{
					V: 42,
				},
				&ExprInt{
					V: 13,
				},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x",
					Value: innerFunc,
				},
			},
		}
		testCases = append(testCases, test{
			name: "assign from func call or two ints",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				innerFunc: types.NewType("int"),
			},
		})
	}
	{
		//$x = template("hello", 42)
		innerFunc := &ExprCall{
			Name: "template",
			Args: []interfaces.Expr{
				&ExprStr{
					V: "hello",
				},
				&ExprInt{
					V: 42,
				},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x",
					Value: innerFunc,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple template",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				innerFunc: types.NewType("str"),
			},
		})
	}
	{
		//$v = 42
		//$x = template("hello", $v) # redirect var for harder unification
		innerFunc := &ExprCall{
			Name: "template",
			Args: []interfaces.Expr{
				&ExprStr{
					V: "hello", // whatever...
				},
				&ExprVar{
					Name: "x",
				},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "v",
					Value: &ExprInt{
						V: 42,
					},
				},
				&StmtBind{
					Ident: "x",
					Value: innerFunc,
				},
			},
		}
		testCases = append(testCases, test{
			name: "complex template",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				innerFunc: types.NewType("str"),
			},
		})
	}
	{
		//test "t1" {
		//	stringptr => datetime(),	# bad (str vs. int)
		//}
		expr := &ExprCall{
			Name: "datetime",
			Args: []interfaces.Expr{},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "t1"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "single fact unification",
			ast:  stmt,
			fail: true,
		})
	}
	{
		//test "t1" {
		//	stringptr => getenv("GOPATH", "bug"),	# bad (two args vs. one)
		//}
		expr := &ExprCall{
			Name: "getenv",
			Args: []interfaces.Expr{
				&ExprStr{
					V: "GOPATH",
				},
				&ExprStr{
					V: "bug",
				},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "t1"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "function, wrong arg count",
			ast:  stmt,
			fail: true,
		})
	}
	// XXX: add these tests when we fix the bug!
	//{
	//	//test "t1" {
	//	//	import "fmt"
	//	//	stringptr => fmt.printf("hello %s and %s", "one"),	# bad
	//	//}
	//	expr := &ExprCall{
	//		Name: "fmt.printf",
	//		Args: []interfaces.Expr{
	//			&ExprStr{
	//				V: "hello %s and %s",
	//			},
	//			&ExprStr{
	//				V: "one",
	//			},
	//		},
	//	}
	//	stmt := &StmtProg{
	//		Prog: []interfaces.Stmt{
	//			&StmtImport{
	//				Name: "fmt",
	//			},
	//			&StmtRes{
	//				Kind: "test",
	//				Name: &ExprStr{V: "t1"},
	//				Contents: []StmtResContents{
	//					&StmtResField{
	//						Field: "stringptr",
	//						Value: expr,
	//					},
	//				},
	//			},
	//		},
	//	}
	//	testCases = append(testCases, test{
	//		name: "function, missing arg for printf",
	//		ast:  stmt,
	//		fail: true,
	//	})
	//}
	//{
	//	//test "t1" {
	//	//	import "fmt"
	//	//	stringptr => fmt.printf("hello %s and %s", "one", "two", "three"),	# bad
	//	//}
	//	expr := &ExprCall{
	//		Name: "fmt.printf",
	//		Args: []interfaces.Expr{
	//			&ExprStr{
	//				V: "hello %s and %s",
	//			},
	//			&ExprStr{
	//				V: "one",
	//			},
	//			&ExprStr{
	//				V: "two",
	//			},
	//			&ExprStr{
	//				V: "three",
	//			},
	//		},
	//	}
	//	stmt := &StmtProg{
	//		Prog: []interfaces.Stmt{
	//			&StmtImport{
	//				Name: "fmt",
	//			},
	//			&StmtRes{
	//				Kind: "test",
	//				Name: &ExprStr{V: "t1"},
	//				Contents: []StmtResContents{
	//					&StmtResField{
	//						Field: "stringptr",
	//						Value: expr,
	//					},
	//				},
	//			},
	//		},
	//	}
	//	testCases = append(testCases, test{
	//		name: "function, extra arg for printf",
	//		ast:  stmt,
	//		fail: true,
	//	})
	//}
	{
		//test "t1" {
		//	import "fmt"
		//	stringptr => fmt.printf("hello %s and %s", "one", "two"),
		//}
		expr := &ExprCall{
			Name: "fmt.printf",
			Args: []interfaces.Expr{
				&ExprStr{
					V: "hello %s and %s",
				},
				&ExprStr{
					V: "one",
				},
				&ExprStr{
					V: "two",
				},
			},
		}
		stmt := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtImport{
					Name: "fmt",
				},
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{V: "t1"},
					Contents: []StmtResContents{
						&StmtResField{
							Field: "stringptr",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "function, regular printf unification",
			ast:  stmt,
			fail: false,
			expect: map[interfaces.Expr]*types.Type{
				expr: types.NewType("str"),
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
			ast, fail, expect, experr := tc.ast, tc.fail, tc.expect, tc.experr

			//str := strings.NewReader(code)
			//ast, err := LexParse(str)
			//if err != nil {
			//	t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
			//	return
			//}
			// TODO: print out the AST's so that we can see the types
			t.Logf("\n\ntest #%d: AST (before): %+v\n", index, ast)

			data := &interfaces.Data{
				Debug: true,
				Logf: func(format string, v ...interface{}) {
					t.Logf(fmt.Sprintf("test #%d", index)+": ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			// skip interpolation in this test so that the node pointers
			// aren't changed and so we can compare directly to expected
			//astInterpolated, err := ast.Interpolate() // interpolate strings in ast
			//if err != nil {
			//	t.Errorf("test #%d: interpolate failed with: %+v", index, err)
			//	return
			//}
			//t.Logf("test #%d: astInterpolated: %+v", index, astInterpolated)

			// top-level, built-in, initial global scope
			scope := &interfaces.Scope{
				Variables: map[string]interfaces.Expr{
					"purpleidea": &ExprStr{V: "hello world!"}, // james says hi
					//"hostname": &ExprStr{V: obj.Hostname},
				},
				// all the built-in top-level, core functions enter here...
				Functions: funcs.LookupPrefix(""),
			}
			// propagate the scope down through the AST...
			if err := ast.SetScope(scope); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: set scope failed with: %+v", index, err)
				return
			}

			// apply type unification
			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": unification: "+format, v...)
			}
			err := unification.Unify(ast, unification.SimpleInvariantSolverLogger(logf))

			// TODO: print out the AST's so that we can see the types
			t.Logf("\n\ntest #%d: AST (after): %+v\n", index, ast)

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification passed, expected fail", index)
				return
			}
			if fail && experr != nil && err != experr { // test for specific error!
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected fail, got wrong error", index)
				t.Errorf("test #%d: got error: %+v", index, err)
				t.Errorf("test #%d: exp error: %+v", index, experr)
				return
			}

			if expect == nil { // test done early
				return
			}
			// TODO: do this in sorted order
			var failed bool
			for expr, exptyp := range expect {
				typ, err := expr.Type() // lookup type
				if err != nil {
					t.Errorf("test #%d: type lookup of %+v failed with: %+v", index, expr, err)
					failed = true
					break
				}

				if err := typ.Cmp(exptyp); err != nil {
					t.Errorf("test #%d: type cmp failed with: %+v", index, err)
					t.Logf("test #%d: got: %+v", index, typ)
					t.Logf("test #%d: exp: %+v", index, exptyp)
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
