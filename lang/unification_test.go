// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package lang // XXX: move this to the unification package

import (
	"fmt"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/util"
)

func TestUnification1(t *testing.T) {
	type test struct { // an individual test
		name      string
		ast       interfaces.Stmt // raw AST
		fail      bool
		expect    map[interfaces.Expr]*types.Type
		experr    error  // expected error if fail == true (nil ignores it)
		experrstr string // expected error prefix
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
		expr := &ast.ExprStr{V: "hello"}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "t1"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		v1 := &ast.ExprStr{}
		v2 := &ast.ExprStr{}
		v3 := &ast.ExprStr{}
		expr := &ast.ExprList{
			Elements: []interfaces.Expr{
				v1,
				v2,
				v3,
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "test"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		k1 := &ast.ExprInt{}
		k2 := &ast.ExprInt{}
		k3 := &ast.ExprInt{}
		v1 := &ast.ExprFloat{}
		v2 := &ast.ExprFloat{}
		v3 := &ast.ExprFloat{}
		expr := &ast.ExprMap{
			KVs: []*ast.ExprMapKV{
				{Key: k1, Val: v1},
				{Key: k2, Val: v2},
				{Key: k3, Val: v3},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "test"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		b := &ast.ExprBool{}
		s := &ast.ExprStr{}
		i := &ast.ExprInt{}
		f := &ast.ExprFloat{}
		expr := &ast.ExprStruct{
			Fields: []*ast.ExprStructField{
				{Name: "somebool", Value: b},
				{Name: "somestr", Value: s},
				{Name: "someint", Value: i},
				{Name: "somefloat", Value: f},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "test"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		expr := &ast.ExprCall{
			Name: funcs.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "+",
				},
				&ast.ExprInt{
					V: 13,
				},

				&ast.ExprInt{
					V: 42,
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "n1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		innerFunc := &ast.ExprCall{
			Name: funcs.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "-",
				},
				&ast.ExprInt{
					V: 42,
				},
				&ast.ExprInt{
					V: 4,
				},
			},
		}
		expr := &ast.ExprCall{
			Name: funcs.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "+",
				},
				&ast.ExprInt{
					V: 13,
				},
				innerFunc, // nested func, can we unify?
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "n1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		innerFunc := &ast.ExprCall{
			Name: funcs.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "+",
				},
				&ast.ExprFloat{
					V: 32.6,
				},
				&ast.ExprFloat{
					V: 13.7,
				},
			},
		}
		expr := &ast.ExprCall{
			Name: funcs.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "+",
				},
				&ast.ExprFloat{
					V: -25.38789,
				},
				innerFunc, // nested func, can we unify?
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "n1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		//test "t1" {
		//	int64 => $x,
		//}
		innerFunc := &ast.ExprCall{
			Name: funcs.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "-",
				},
				&ast.ExprInt{
					V: 42,
				},
				&ast.ExprInt{
					V: 13,
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x",
					Value: innerFunc,
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64",
							Value: &ast.ExprVar{
								Name: "x",
							},
						},
					},
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
		//test "t1" {
		//	anotherstr => $x,
		//}
		innerFunc := &ast.ExprCall{
			Name: "template",
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "hello",
				},
				&ast.ExprInt{
					V: 42,
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x",
					Value: innerFunc,
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "anotherstr",
							Value: &ast.ExprVar{
								Name: "x",
							},
						},
					},
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
		//test "t1" {
		//	anotherstr => $x,
		//}
		innerFunc := &ast.ExprCall{
			Name: "template",
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "hello", // whatever...
				},
				&ast.ExprVar{
					Name: "v",
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "v",
					Value: &ast.ExprInt{
						V: 42,
					},
				},
				&ast.StmtBind{
					Ident: "x",
					Value: innerFunc,
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "anotherstr",
							Value: &ast.ExprVar{
								Name: "x",
							},
						},
					},
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
		// import "datetime"
		//test "t1" {
		//	stringptr => datetime.now(),	# bad (str vs. int)
		//}
		expr := &ast.ExprCall{
			Name: "datetime.now",
			Args: []interfaces.Expr{},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name: "datetime",
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "t1"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
		//import "sys"
		//test "t1" {
		//	stringptr => sys.getenv("GOPATH", "bug"),	# bad (two args vs. one)
		//}
		expr := &ast.ExprCall{
			Name: "sys.getenv",
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "GOPATH",
				},
				&ast.ExprStr{
					V: "bug",
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name: "sys",
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "t1"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
	//	//import "fmt"
	//	//test "t1" {
	//	//	stringptr => fmt.printf("hello %s and %s", "one"),	# bad
	//	//}
	//	expr := &ast.ExprCall{
	//		Name: "fmt.printf",
	//		Args: []interfaces.Expr{
	//			&ast.ExprStr{
	//				V: "hello %s and %s",
	//			},
	//			&ast.ExprStr{
	//				V: "one",
	//			},
	//		},
	//	}
	//	stmt := &ast.StmtProg{
	//		Body: []interfaces.Stmt{
	//			&ast.StmtImport{
	//				Name: "fmt",
	//			},
	//			&ast.StmtRes{
	//				Kind: "test",
	//				Name: &ast.ExprStr{V: "t1"},
	//				Contents: []ast.StmtResContents{
	//					&ast.StmtResField{
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
	//	//import "fmt"
	//	//test "t1" {
	//	//	stringptr => fmt.printf("hello %s and %s", "one", "two", "three"),	# bad
	//	//}
	//	expr := &ast.ExprCall{
	//		Name: "fmt.printf",
	//		Args: []interfaces.Expr{
	//			&ast.ExprStr{
	//				V: "hello %s and %s",
	//			},
	//			&ast.ExprStr{
	//				V: "one",
	//			},
	//			&ast.ExprStr{
	//				V: "two",
	//			},
	//			&ast.ExprStr{
	//				V: "three",
	//			},
	//		},
	//	}
	//	stmt := &ast.StmtProg{
	//		Body: []interfaces.Stmt{
	//			&ast.StmtImport{
	//				Name: "fmt",
	//			},
	//			&ast.StmtRes{
	//				Kind: "test",
	//				Name: &ast.ExprStr{V: "t1"},
	//				Contents: []ast.StmtResContents{
	//					&ast.StmtResField{
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
		//import "fmt"
		//test "t1" {
		//	stringptr => fmt.printf("hello %s and %s", "one", "two"),
		//}
		expr := &ast.ExprCall{
			Name: "fmt.printf",
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "hello %s and %s",
				},
				&ast.ExprStr{
					V: "one",
				},
				&ast.ExprStr{
					V: "two",
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name: "fmt",
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "t1"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
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
	{
		//import "fmt"
		//$x str = if true {	# should fail unification
		//	42
		//} else {
		//	13
		//}
		//test "t1" {
		//	stringptr => fmt.printf("hello %s", $x),
		//}
		cond := &ast.ExprIf{
			Condition:  &ast.ExprBool{V: true},
			ThenBranch: &ast.ExprInt{V: 42},
			ElseBranch: &ast.ExprInt{V: 13},
		}
		cond.SetType(types.TypeStr) // should fail unification
		expr := &ast.ExprCall{
			Name: "fmt.printf",
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "hello %s",
				},
				&ast.ExprVar{
					Name: "x", // the var
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name: "fmt",
				},
				&ast.StmtBind{
					Ident: "x", // the var
					Value: cond,
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "t1"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "anotherstr",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name:      "typed if expr",
			ast:       stmt,
			fail:      true,
			experrstr: "can't unify, invariant illogicality with equality: base kind does not match (Str != Int)",
		})
	}
	{
		//import "fmt"
		//$w = true
		//$x str = $w	# should fail unification
		//test "t1" {
		//	stringptr => fmt.printf("hello %s", $x),
		//}
		wvar := &ast.ExprBool{V: true}
		xvar := &ast.ExprVar{Name: "w"}
		xvar.SetType(types.TypeStr) // should fail unification
		expr := &ast.ExprCall{
			Name: "fmt.printf",
			Args: []interfaces.Expr{
				&ast.ExprStr{
					V: "hello %s",
				},
				&ast.ExprVar{
					Name: "x", // the var
				},
			},
		}
		stmt := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name: "fmt",
				},
				&ast.StmtBind{
					Ident: "w",
					Value: wvar,
				},
				&ast.StmtBind{
					Ident: "x", // the var
					Value: xvar,
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{V: "t1"},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "anotherstr",
							Value: expr,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name:      "typed var expr",
			ast:       stmt,
			fail:      true,
			experrstr: "can't unify, invariant illogicality with equality: base kind does not match (Str != Bool)",
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
			xast, fail, expect, experr, experrstr := tc.ast, tc.fail, tc.expect, tc.experr, tc.experrstr

			//str := strings.NewReader(code)
			//xast, err := parser.LexParse(str)
			//if err != nil {
			//	t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
			//	return
			//}
			// TODO: print out the AST's so that we can see the types
			t.Logf("\n\ntest #%d: AST (before): %+v\n", index, xast)

			data := &interfaces.Data{
				// TODO: add missing fields here if/when needed
				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					t.Logf(fmt.Sprintf("test #%d", index)+": ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := xast.Init(data); err != nil {
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

			variables := map[string]interfaces.Expr{
				"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
				//"hostname": &ast.ExprStr{V: obj.Hostname},
			}
			consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
			addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
			var err error
			variables, err = ast.MergeExprMaps(variables, consts, addback)
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: couldn't merge in consts: %+v", index, err)
				return
			}

			// top-level, built-in, initial global scope
			scope := &interfaces.Scope{
				Variables: variables,
				// all the built-in top-level, core functions enter here...
				Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
			}
			// propagate the scope down through the AST...
			if err := xast.SetScope(scope); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: set scope failed with: %+v", index, err)
				return
			}

			// apply type unification
			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": unification: "+format, v...)
			}
			unifier := &unification.Unifier{
				AST:    xast,
				Solver: unification.SimpleInvariantSolverLogger(logf),
				Debug:  testing.Verbose(),
				Logf:   logf,
			}
			err = unifier.Unify()

			// TODO: print out the AST's so that we can see the types
			t.Logf("\n\ntest #%d: AST (after): %+v\n", index, xast)

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
