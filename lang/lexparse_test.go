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

package lang

import (
	"reflect"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"

	"github.com/davecgh/go-spew/spew"
)

func TestLexParse0(t *testing.T) {
	type test struct { // an individual test
		name string
		code string
		fail bool
		exp  interfaces.Stmt
	}
	values := []test{}

	{
		values = append(values, test{
			"nil",
			``,
			false,
			nil,
		})
	}
	{
		values = append(values, test{
			name: "simple assignment",
			code: `$rewsna = -42`,
			fail: false,
			exp: &StmtProg{
				Prog: []interfaces.Stmt{
					&StmtBind{
						Ident: "rewsna",
						Value: &ExprInt{
							V: -42,
						},
					},
				},
			},
		})
	}
	{
		values = append(values, test{
			name: "one res",
			code: `noop "n1" {}`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "res with keyword",
			code: `false "n1" {}`, // false is a special keyword
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "bad escaping",
			code: `
			test "t1" {
				str => "he\ llo", # incorrect escaping
			}
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "int overflow",
			code: `
			test "t1" {
				int => 888888888888888888888888, # overflows
			}
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "overflow after lexer",
			code: `
			test "t1" {
				uint8 => 128, # does not overflow at lexer stage
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "one res",
			code: `
			test "t1" {
				int16 => 01134, # some comment
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "one res with elvis",
			code: `
			test "t1" {
				int16 => true ?: 42, # elvis operator
				int32 => 42,
				stringptr => false ?: "", # missing is not ""
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		// TODO: skip trailing comma requirement on one-liners
		values = append(values, test{
			name: "two lists",
			code: `
			$somelist = [42, 0, -13,]
			$somelonglist = [
				"hello",
				"and",
				"how",
				"are",
				"you?",
			]
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "one map",
			code: `
			$somemap = {
				"foo" => "foo1",
				"bar" => "bar1",
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "another map",
			code: `
			$somemap = {
				"foo" => -13,
				"bar" => 42,
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	// TODO: alternate possible syntax ?
	//{
	//	values = append(values, test{ // ?
	//		code: `
	//		$somestruct = struct{
	//			foo: "foo1";
	//			bar: 42	# no trailing semicolon at the moment
	//		}
	//		`,
	//		fail:  false,
	//		//exp: ???, // FIXME: add the expected AST
	//	})
	//}
	{
		values = append(values, test{
			name: "one struct",
			code: `
			$somestruct = struct{
				foo => "foo1",
				bar => 42,
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "struct with nested struct",
			code: `
			$somestruct = struct{
				foo => "foo1",
				bar => struct{
					a => true,
					b => "hello",
				},
				baz => 42,
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	// types
	{
		values = append(values, test{
			name: "some lists",
			code: `
			$intlist []int = [42, -0, 13,]
			$intlistnested [][]int = [[42,], [], [100, -0,], [-13,],]
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "maps and lists",
			code: `
			$strmap {str: int} = {
				"key1" => 42,
				"key2" => -13,
			}
			$mapstrintlist {str: []int} = {
				"key1" => [42, 44,],
				"key2" => [],
				"key3" => [-13,],
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "some structs",
			code: `
			$structx struct{a int; b bool; c str} = struct{
				a => 42,
				b => true,
				c => "hello",
			}
			$structx2 struct{a int; b []bool; c str} = struct{
				a => 42,
				b => [true, false, false, true,],
				c => "hello",
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		values = append(values, test{
			name: "res with floats",
			code: `
			test "t1" {
				float32 => -25.38789, # some float
				float64 => 53.393908945, # some float
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	// FIXME: why doesn't this overflow, and thus fail?
	// from the docs: If s is syntactically well-formed but is more than 1/2
	// ULP away from the largest floating point number of the given size,
	// ParseFloat returns f = ±Inf, err.Err = ErrRange.
	//{
	//	values = append(values, test{
	//		name: "overflowing float",
	//		code: `
	//		test "t1" {
	//			float32 => -457643875645764387564578645457864525457643875645764387564578645457864525.457643875645764387564578645457864525387899898753459879587574928798759863965, # overflow
	//		}
	//		`,
	//		fail: true,
	//	})
	//}
	{
		values = append(values, test{
			name: "res and addition",
			code: `
			test "t1" {
				float32 => -25.38789 + 32.6,
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "int64ptr",
							Value: &ExprCall{
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
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "addition",
			code: `
			test "t1" {
				int64ptr => 13 + 42,
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "float32",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: "+",
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "+",
											},
											&ExprFloat{
												V: -25.38789,
											},
											&ExprFloat{
												V: 32.6,
											},
										},
									},
									&ExprFloat{
										V: 13.7,
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "multiple float addition",
			code: `
			test "t1" {
				float32 => -25.38789 + 32.6 + 13.7,
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "int64ptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: "+",
									},
									&ExprInt{
										V: 4,
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "*",
											},
											&ExprInt{
												V: 3,
											},
											&ExprInt{
												V: 12,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations lucky",
			code: `
			test "t1" {
				int64ptr => 4 + 3 * 12, # 40, not 84
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "int64ptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: "+",
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "*",
											},
											&ExprInt{
												V: 3,
											},
											&ExprInt{
												V: 12,
											},
										},
									},
									&ExprInt{
										V: 4,
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations needs left precedence",
			code: `
			test "t1" {
				int64ptr => 3 * 12 + 4, # 40, not 48
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "int64ptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: "*",
									},
									&ExprInt{
										V: 3,
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "+",
											},
											&ExprInt{
												V: 12,
											},
											&ExprInt{
												V: 4,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations parens",
			code: `
			test "t1" {
				int64ptr => 3 * (12 + 4), # 48, not 40
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "boolptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: ">",
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "+",
											},
											&ExprInt{
												V: 3,
											},
											&ExprInt{
												V: 4,
											},
										},
									},
									&ExprInt{
										V: 5,
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations bools",
			code: `
			test "t1" {
				boolptr => 3 + 4 > 5, # should be true
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "boolptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: ">",
									},
									&ExprInt{
										V: 3,
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "+",
											},
											&ExprInt{
												V: 4,
											},
											&ExprInt{
												V: 5,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations bools reversed",
			code: `
			test "t1" {
				boolptr => 3 > 4 + 5, # should be false
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "boolptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: ">",
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "!",
											},
											&ExprInt{
												V: 3,
											},
										},
									},
									&ExprInt{
										V: 4,
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations with not",
			code: `
			test "t1" {
				boolptr => ! 3 > 4, # should parse, but not compile
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "boolptr",
							Value: &ExprCall{
								Name: operatorFuncName,
								Args: []interfaces.Expr{
									&ExprStr{
										V: "&&",
									},
									&ExprCall{
										Name: operatorFuncName,
										Args: []interfaces.Expr{
											&ExprStr{
												V: "<",
											},
											&ExprInt{
												V: 7,
											},
											&ExprInt{
												V: 4,
											},
										},
									},
									&ExprBool{
										V: true,
									},
								},
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "order of operations logical",
			code: `
			test "t1" {
				boolptr => 7 < 4 && true, # should be false
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t1",
					},
					Fields: []*StmtResField{
						{
							Field: "int64ptr",
							Value: &ExprInt{
								V: 42,
							},
						},
					},
				},
				&StmtRes{
					Kind: "test",
					Name: &ExprStr{
						V: "t2",
					},
					Fields: []*StmtResField{
						{
							Field: "int64ptr",
							Value: &ExprInt{
								V: 13,
							},
						},
					},
				},
				&StmtEdge{
					EdgeHalfList: []*StmtEdgeHalf{
						{
							Kind: "test",
							Name: &ExprStr{
								V: "t1",
							},
							SendRecv: "foosend",
						},
						{
							Kind: "test",
							Name: &ExprStr{
								V: "t2",
							},
							SendRecv: "barrecv",
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "edge stmt",
			code: `
			test "t1" {
				int64ptr => 42,
			}
			test "t2" {
				int64ptr => 13,
			}

			Test["t1"].foosend -> Test["t2"].barrecv # send/recv
			`,
			fail: false,
			exp:  exp,
		})
	}

	for index, test := range values { // run all the tests
		name, code, fail, exp := test.name, test.code, test.fail, test.exp

		if name == "" {
			name = "<sub test not named>"
		}

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if (index != 20 && index != 21) {
		//if test.name != "nil" {
		//	continue
		//}

		t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

		str := strings.NewReader(code)
		ast, err := LexParse(str)

		if !fail && err != nil {
			t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
			continue
		}
		if fail && err == nil {
			t.Errorf("test #%d: lex/parse passed, expected fail", index)
			continue
		}

		if !fail && ast == nil {
			t.Errorf("test #%d: lex/parse was nil", index)
			continue
		}

		if exp != nil {
			if !reflect.DeepEqual(ast, exp) {
				t.Errorf("test #%d: AST did not match expected", index)
				// TODO: consider making our own recursive print function
				t.Logf("test #%d:   actual: \n\n%s\n", index, spew.Sdump(ast))
				t.Logf("test #%d: expected: \n\n%s", index, spew.Sdump(exp))
				continue
			}
		}
	}
}

func TestLexParse1(t *testing.T) {
	code := `
	$a = 42
	$b = true
	$c = 13
	$d = "hello"
	$e = true
	$f = 3.13
	# some noop resource
	noop "n0" {
		foo => true,
		bar => false	# this should be a parser error (no comma)
	}
	# hello
	# world
	test "t1" {}
	` // error
	str := strings.NewReader(code)
	_, err := LexParse(str)
	if e, ok := err.(*LexParseErr); ok && e.Err != ErrParseExpectingComma {
		t.Errorf("lex/parse failure, got: %+v", e)
	} else if err == nil {
		t.Errorf("lex/parse success, expected error")
	} else {
		if e.Row != 10 || e.Col != 9 {
			t.Errorf("expected error at 10 x 9, got: %d x %d", e.Row, e.Col)
		}
		t.Logf("row x col: %d x %d", e.Row, e.Col)
		t.Logf("message: %s", e.Str)
		t.Logf("output: %+v", err)
	}
}

func TestLexParse2(t *testing.T) {
	code := `
	$a == 13
	test "t1" {
		int8 => $a,
	}
	` // error, assignment is a single equals, not two
	str := strings.NewReader(code)
	_, err := LexParse(str)
	if e, ok := err.(*LexParseErr); ok && e.Err != ErrParseAdditionalEquals {
		t.Errorf("lex/parse failure, got: %+v", e)
	} else if err == nil {
		t.Errorf("lex/parse success, expected error")
	} else {
		// TODO: when this is accurate, pick values and enable this!
		//if e.Row != 8 || e.Col != 2 {
		//	t.Errorf("expected error at 8 x 2, got: %d x %d", e.Row, e.Col)
		//}
		t.Logf("row x col: %d x %d", e.Row, e.Col)
		t.Logf("message: %s", e.Str)
		t.Logf("output: %+v", err)
	}
}
