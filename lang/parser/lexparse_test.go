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

package parser

import (
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/funcs/operators"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	langUtil "github.com/purpleidea/mgmt/lang/util"
	"github.com/purpleidea/mgmt/util"

	godiff "github.com/kylelemons/godebug/diff"
	"github.com/kylelemons/godebug/pretty"
	"github.com/sanity-io/litter"
)

func TestLexParse0(t *testing.T) {
	type test struct { // an individual test
		name string
		code string
		fail bool
		exp  interfaces.Stmt
	}
	testCases := []test{}

	{
		testCases = append(testCases, test{
			"nil",
			``,
			false,
			nil,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple assignment",
			code: `$rewsna = -42`,
			fail: false,
			exp: &ast.StmtProg{
				Body: []interfaces.Stmt{
					&ast.StmtBind{
						Ident: "rewsna",
						Value: &ast.ExprInt{
							V: -42,
						},
					},
				},
			},
		})
	}
	{
		testCases = append(testCases, test{
			name: "one res",
			code: `noop "n1" {}`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		testCases = append(testCases, test{
			name: "res with keyword",
			code: `false "n1" {}`, // false is a special keyword
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
			name: "bad escaping 2",
			code: `
			test "t1" {
				str => "he\\ llo", # incorrect escaping
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
			name: "one res with param",
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
	//	testCases = append(testCases, test{ // ?
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
			name: "maps 1",
			code: `
			# make sure the "str:" part doesn't match a single ident
			$strmap map{str: int} = {
				"key1" => 42,
				"key2" => -13,
			}
			`,
			fail: false,
			//exp: ???, // FIXME: add the expected AST
		})
	}
	{
		testCases = append(testCases, test{
			name: "maps 2",
			code: `
			$mapstrintlist map{str: []int} = {
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
		testCases = append(testCases, test{
			name: "maps and lists",
			code: `
			$strmap map{str: int} = {
				"key1" => 42,
				"key2" => -13,
			}
			$mapstrintlist map{str: []int} = {
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
		testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
	//	testCases = append(testCases, test{
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
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x1",
					Value: &ast.ExprCall{
						Name: "foo1",
						Args: []interfaces.Expr{},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call 1",
			code: `
			$x1 = foo1()
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x1",
					Value: &ast.ExprCall{
						Name: "foo1",
						Args: []interfaces.Expr{
							&ast.ExprInt{
								V: 13,
							},
							&ast.ExprStr{
								V: "hello",
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call 2",
			code: `
			$x1 = foo1(13, "hello")
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x1",
					Value: &ast.ExprCall{
						Name: "pkg.foo1",
						Args: []interfaces.Expr{},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call dotted 1",
			code: `
			$x1 = pkg.foo1()
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x1",
					Value: &ast.ExprCall{
						Name: "pkg.foo1",
						Args: []interfaces.Expr{
							&ast.ExprBool{
								V: true,
							},
							&ast.ExprStr{
								V: "hello",
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "func call dotted 2",
			code: `
			$x1 = pkg.foo1(true, "hello")
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		testCases = append(testCases, test{
			name: "func call dotted invalid 1",
			code: `
			$x1 = .pkg.foo1(true, "hello")
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "func call dotted invalid 2",
			code: `
			$x1 = pkg.foo1.(true, "hello")
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "func call dotted invalid 3",
			code: `
			$x1 = .pkg.foo1.(true, "hello")
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "func call dotted invalid 4",
			code: `
			$x1 = pkg..foo1(true, "hello")
			`,
			fail: true,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x1",
					Value: &ast.ExprVar{
						Name: "pkg.foo1",
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "dotted var 1",
			code: `
			$x1 = $pkg.foo1
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "x1",
					Value: &ast.ExprVar{
						Name: "pkg.foo1.bar",
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "dotted var 2",
			code: `
			$x1 = $pkg.foo1.bar
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		testCases = append(testCases, test{
			name: "invalid dotted var 1",
			code: `
			$x1 = $.pkg.foo1.bar
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "invalid dotted var 2",
			code: `
			$x1 = $pkg.foo1.bar.
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "invalid dotted var 3",
			code: `
			$x1 = $.pkg.foo1.bar.
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "invalid dotted var 4",
			code: `
			$x1 = $pkg..foo1.bar
			`,
			fail: true,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64ptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
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
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "float32",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: "+",
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "+",
											},
											&ast.ExprFloat{
												V: -25.38789,
											},
											&ast.ExprFloat{
												V: 32.6,
											},
										},
									},
									&ast.ExprFloat{
										V: 13.7,
									},
								},
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64ptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: "+",
									},
									&ast.ExprInt{
										V: 4,
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "*",
											},
											&ast.ExprInt{
												V: 3,
											},
											&ast.ExprInt{
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
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64ptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: "+",
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "*",
											},
											&ast.ExprInt{
												V: 3,
											},
											&ast.ExprInt{
												V: 12,
											},
										},
									},
									&ast.ExprInt{
										V: 4,
									},
								},
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64ptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: "*",
									},
									&ast.ExprInt{
										V: 3,
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "+",
											},
											&ast.ExprInt{
												V: 12,
											},
											&ast.ExprInt{
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
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "boolptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: ">",
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "+",
											},
											&ast.ExprInt{
												V: 3,
											},
											&ast.ExprInt{
												V: 4,
											},
										},
									},
									&ast.ExprInt{
										V: 5,
									},
								},
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "boolptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: ">",
									},
									&ast.ExprInt{
										V: 3,
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "+",
											},
											&ast.ExprInt{
												V: 4,
											},
											&ast.ExprInt{
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
		testCases = append(testCases, test{
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
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "boolptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: ">",
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "not",
											},
											&ast.ExprInt{
												V: 3,
											},
										},
									},
									&ast.ExprInt{
										V: 4,
									},
								},
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "order of operations with not",
			code: `
			test "t1" {
				boolptr => not 3 > 4, # should parse, but not compile
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "boolptr",
							Value: &ast.ExprCall{
								Name: operators.OperatorFuncName,
								Args: []interfaces.Expr{
									&ast.ExprStr{
										V: "and",
									},
									&ast.ExprCall{
										Name: operators.OperatorFuncName,
										Args: []interfaces.Expr{
											&ast.ExprStr{
												V: "<",
											},
											&ast.ExprInt{
												V: 7,
											},
											&ast.ExprInt{
												V: 4,
											},
										},
									},
									&ast.ExprBool{
										V: true,
									},
								},
							},
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "order of operations logical",
			code: `
			test "t1" {
				boolptr => 7 < 4 and true, # should be false
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64ptr",
							Value: &ast.ExprInt{
								V: 42,
							},
						},
					},
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t2",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResField{
							Field: "int64ptr",
							Value: &ast.ExprInt{
								V: 13,
							},
						},
					},
				},
				&ast.StmtEdge{
					EdgeHalfList: []*ast.StmtEdgeHalf{
						{
							Kind: "test",
							Name: &ast.ExprStr{
								V: "t1",
							},
							SendRecv: "foosend",
						},
						{
							Kind: "test",
							Name: &ast.ExprStr{
								V: "t2",
							},
							SendRecv: "barrecv",
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
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
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t1",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResMeta{
							Property: "noop",
							MetaExpr: &ast.ExprBool{
								V: true,
							},
						},
						&ast.StmtResMeta{
							Property: "delay",
							MetaExpr: &ast.ExprInt{
								V: 42,
							},
							Condition: &ast.ExprBool{
								V: true,
							},
						},
					},
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t2",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResMeta{
							Property: "limit",
							MetaExpr: &ast.ExprFloat{
								V: 0.45,
							},
						},
						&ast.StmtResMeta{
							Property: "burst",
							MetaExpr: &ast.ExprInt{
								V: 4,
							},
						},
					},
				},
				&ast.StmtRes{
					Kind: "test",
					Name: &ast.ExprStr{
						V: "t3",
					},
					Contents: []ast.StmtResContents{
						&ast.StmtResMeta{
							Property: "noop",
							MetaExpr: &ast.ExprBool{
								V: true,
							},
						},
						&ast.StmtResMeta{
							Property: "meta",
							MetaExpr: &ast.ExprStruct{
								Fields: []*ast.ExprStructField{
									{Name: "poll", Value: &ast.ExprInt{V: 5}},
									{Name: "retry", Value: &ast.ExprInt{V: 3}},
									{
										Name: "sema",
										Value: &ast.ExprList{
											Elements: []interfaces.Expr{
												&ast.ExprStr{V: "foo:1"},
												&ast.ExprStr{V: "bar:3"},
											},
										},
									},
								},
							},
						},
					},
				}},
		}
		testCases = append(testCases, test{
			name: "res meta stmt",
			code: `
			test "t1" {
				Meta:noop => true,
				Meta:delay => true ?: 42,
			}
			test "t2" {
				Meta:limit => 0.45,
				Meta:burst => 4,
			}
			test "t3" {
				Meta:noop => true, # meta params can be combined
				Meta => struct{
					poll => 5,
					retry => 3,
					sema => ["foo:1", "bar:3",],
				},
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		testCases = append(testCases, test{
			name: "parser set type incompatibility str",
			code: `
			$x int = "hello"	# type should be str to work
			test "t1" {
				str => $x,
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "parser set type incompatibility int",
			code: `
			$x int = "hello"	# value should be int to work
			test "t1" {
				int => $x,
			}
			`,
			fail: true,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "c1",
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{
							&ast.StmtRes{
								Kind: "test",
								Name: &ast.ExprStr{
									V: "t1",
								},
								Contents: []ast.StmtResContents{
									&ast.StmtResField{
										Field: "stringptr",
										Value: &ast.ExprStr{
											V: "hello",
										},
									},
								},
							},
						},
					},
				},
				&ast.StmtInclude{
					Name: "c1",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple class 1",
			code: `
			class c1 {
				test "t1" {
					stringptr => "hello",
				}
			}
			include c1
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "c1",
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{
							&ast.StmtRes{
								Kind: "test",
								Name: &ast.ExprStr{
									V: "t1",
								},
								Contents: []ast.StmtResContents{
									&ast.StmtResField{
										Field: "stringptr",
										Value: &ast.ExprStr{
											V: "hello",
										},
									},
								},
							},
						},
					},
				},
				&ast.StmtInclude{
					Name: "pkg.c1",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple dotted class 1",
			code: `
			# a dotted identifier only occurs via an imported class
			class c1 {
				test "t1" {
					stringptr => "hello",
				}
			}
			# a dotted identifier is allowed here if it's imported
			include pkg.c1
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "c1",
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{
							&ast.StmtRes{
								Kind: "test",
								Name: &ast.ExprStr{
									V: "t1",
								},
								Contents: []ast.StmtResContents{
									&ast.StmtResField{
										Field: "stringptr",
										Value: &ast.ExprStr{
											V: "hello",
										},
									},
								},
							},
						},
					},
				},
				&ast.StmtInclude{
					Name: "pkg.ns.c1",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple dotted class 2",
			code: `
			# a dotted identifier only occurs via an imported class
			class c1 {
				test "t1" {
					stringptr => "hello",
				}
			}
			# a dotted identifier is allowed here if it's imported
			include pkg.ns.c1
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple dotted invalid class 1",
			code: `
			# a dotted identifier only occurs via an imported class
			class foo.c1 {
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple dotted invalid class 2",
			code: `
			# a dotted identifier only occurs via an imported class
			class foo.bar.c1 {
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple dotted invalid include 1",
			code: `
			class .foo.c1 {
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple dotted invalid include 2",
			code: `
			class foo.c1. {
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple dotted invalid include 3",
			code: `
			class .foo.c1. {
			}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple dotted invalid include 4",
			code: `
			class foo..c1 {
			}
			`,
			fail: true,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "x",
					Args: []*interfaces.Arg{},
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{},
					},
				},
				&ast.StmtClass{
					Name: "y1",
					Args: []*interfaces.Arg{},
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{},
					},
				},
				&ast.StmtInclude{
					Name: "z",
					Args: nil,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple class with args 0",
			code: `
			class x() {
			}
			class y1() {
			}
			include z
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		testCases = append(testCases, test{
			name: "simple class underscore failure",
			code: `
			class x_() {
			}
			`,
			fail: true,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "c1",
					Args: []*interfaces.Arg{
						{
							Name: "a",
							//Type: &types.Type{},
						},
						{
							Name: "b",
							//Type: &types.Type{},
						},
					},
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{
							&ast.StmtRes{
								Kind: "test",
								Name: &ast.ExprVar{
									Name: "a",
								},
								Contents: []ast.StmtResContents{
									&ast.StmtResField{
										Field: "stringptr",
										Value: &ast.ExprVar{
											Name: "b",
										},
									},
								},
							},
						},
					},
				},
				&ast.StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ast.ExprStr{
							V: "t1",
						},
						&ast.ExprStr{
							V: "hello",
						},
					},
				},
				&ast.StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ast.ExprStr{
							V: "t2",
						},
						&ast.ExprStr{
							V: "world",
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple class with args 1",
			code: `
			class c1($a, $b) {
				test $a {
					stringptr => $b,
				}
			}
			include c1("t1", "hello")
			include c1("t2", "world")
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "c1",
					Args: []*interfaces.Arg{
						{
							Name: "a",
							Type: types.TypeStr,
						},
						{
							Name: "b",
							//Type: &types.Type{},
						},
					},
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{
							&ast.StmtRes{
								Kind: "test",
								Name: &ast.ExprVar{
									Name: "a",
								},
								Contents: []ast.StmtResContents{
									&ast.StmtResField{
										Field: "stringptr",
										Value: &ast.ExprVar{
											Name: "b",
										},
									},
								},
							},
						},
					},
				},
				&ast.StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ast.ExprStr{
							V: "t1",
						},
						&ast.ExprStr{
							V: "hello",
						},
					},
				},
				&ast.StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ast.ExprStr{
							V: "t2",
						},
						&ast.ExprStr{
							V: "world",
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple class with typed args 1",
			code: `
			class c1($a str, $b) {
				test $a {
					stringptr => $b,
				}
			}
			include c1("t1", "hello")
			include c1("t2", "world")
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name:  "foo1",
					Alias: "",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple import 1",
			code: `
			import "foo1"
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name:  "foo1",
					Alias: "bar",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple import 2",
			code: `
			import "foo1" as bar
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name:  "foo1",
					Alias: "",
				},
				&ast.StmtImport{
					Name:  "foo2",
					Alias: "bar",
				},
				&ast.StmtImport{
					Name:  "foo3",
					Alias: "",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple import 3",
			code: `
			import "foo1"
			import "foo2" as bar
			import "foo3"
			`,
			fail: false,
			exp:  exp,
		})
	}
	if ast.AllowBareImports {
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name:  "foo1",
					Alias: interfaces.BareSymbol,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple import 4",
			code: `
			import "foo1" as *
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtClass{
					Name: "c1",
					Body: &ast.StmtProg{
						Body: []interfaces.Stmt{
							&ast.StmtImport{
								Name:  "foo",
								Alias: "bar",
							},
							&ast.StmtImport{
								Name:  "baz",
								Alias: "",
							},
						},
					},
				},
				&ast.StmtInclude{
					Name: "c1",
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple import inside class 1",
			code: `
			class c1 {
				import "foo" as bar
				import "baz"
			}
			include c1
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtFunc{
					Name: "f1",
					Func: &ast.ExprFunc{
						Title: "f1",
						Args:  []*interfaces.Arg{},
						Body: &ast.ExprInt{
							V: 42,
						},
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function stmt 1",
			code: `
			func f1() {
				42
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		fn := &ast.ExprFunc{
			Title:  "f2",
			Args:   []*interfaces.Arg{},
			Return: types.TypeInt,
			Body: &ast.ExprCall{
				Name: operators.OperatorFuncName,
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
			},
		}
		// sometimes, the type can get set by the parser when it's known
		typ := types.NewType("func() int")
		if err := fn.SetType(typ); err != nil {
			t.Fatal("could not build type")
		}
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtFunc{
					Name: "f2",
					Func: fn,
					Type: typ,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function stmt 2",
			code: `
			func f2() int {
				13 + 42
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		fn := &ast.ExprFunc{
			Title: "f3",
			Args: []*interfaces.Arg{
				{
					Name: "a",
					Type: types.TypeInt,
				},
				{
					Name: "b",
					//Type: &types.Type{},
				},
			},
			Return: types.TypeInt,
			Body: &ast.ExprCall{
				Name: operators.OperatorFuncName,
				Args: []interfaces.Expr{
					&ast.ExprStr{
						V: "+",
					},
					&ast.ExprVar{
						Name: "a",
					},
					&ast.ExprVar{
						Name: "b",
					},
				},
			},
		}
		// we can't set the type here, because it's only partially known
		//if err := fn.SetType(types.NewType("func() int")); err != nil {
		//	t.Fatal("could not build type")
		//}
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtFunc{
					Name: "f3",
					Func: fn,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function stmt 3",
			code: `
			func f3($a int, $b) int {
				$a + $b
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		fn := &ast.ExprFunc{
			Title: "f4",
			Args: []*interfaces.Arg{
				{
					Name: "x",
					Type: types.TypeStr,
				},
			},
			Return: types.TypeStr,
			Body: &ast.ExprCall{
				Name: operators.OperatorFuncName,
				Args: []interfaces.Expr{
					&ast.ExprStr{
						V: "+",
					},
					&ast.ExprStr{
						V: "hello",
					},
					&ast.ExprVar{
						Name: "x",
					},
				},
			},
		}
		typ := types.NewType("func(x str) str")
		if err := fn.SetType(typ); err != nil {
			t.Fatal("could not build type")
		}
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtFunc{
					Name: "f4",
					Func: fn,
					Type: typ,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function stmt 4",
			code: `
			func f4($x str) str {
				"hello" + $x
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{

		fn := &ast.ExprFunc{
			Args: []*interfaces.Arg{},
			Body: &ast.ExprInt{
				V: 42,
			},
		}
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "fn",
					Value: fn,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function expr 1",
			code: `
			# lambda
			$fn = func() {
				42
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		fn := &ast.ExprFunc{
			Args: []*interfaces.Arg{
				{
					Name: "x",
					Type: types.TypeStr,
				},
			},
			Return: types.TypeStr,
			Body: &ast.ExprCall{
				Name: operators.OperatorFuncName,
				Args: []interfaces.Expr{
					&ast.ExprStr{
						V: "+",
					},
					&ast.ExprStr{
						V: "hello",
					},
					&ast.ExprVar{
						Name: "x",
					},
				},
			},
		}
		if err := fn.SetType(types.NewType("func(x str) str")); err != nil {
			t.Fatal("could not build type")
		}
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "fn",
					Value: fn,
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function expr 2",
			code: `
			# lambda
			$fn = func($x str) str {
				"hello" + $x
			}
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		fn := &ast.ExprFunc{
			Args: []*interfaces.Arg{
				{
					Name: "x",
					Type: types.TypeStr,
				},
			},
			Return: types.TypeStr,
			Body: &ast.ExprCall{
				Name: operators.OperatorFuncName,
				Args: []interfaces.Expr{
					&ast.ExprStr{
						V: "+",
					},
					&ast.ExprStr{
						V: "hello",
					},
					&ast.ExprVar{
						Name: "x",
					},
				},
			},
		}
		if err := fn.SetType(types.NewType("func(x str) str")); err != nil {
			t.Fatal("could not build type")
		}
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtBind{
					Ident: "fn",
					Value: fn,
				},
				&ast.StmtBind{
					Ident: "foo",
					Value: &ast.ExprCall{
						Name: "fn",
						Args: []interfaces.Expr{
							&ast.ExprStr{
								V: "world",
							},
						},
						Var: true,
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple function expr 3",
			code: `
			# lambda
			$fn = func($x str) str {
				"hello" + $x
			}
			$foo = $fn("world")	# helloworld
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtFunc{
					Name: "funcgen",
					// This is the outer function...
					Func: &ast.ExprFunc{
						Title: "funcgen",
						Args:  []*interfaces.Arg{},
						// This is the inner function...
						Body: &ast.ExprFunc{
							Args: []*interfaces.Arg{},
							Body: &ast.ExprStr{
								V: "hello",
							},
						},
					},
				},
				&ast.StmtBind{
					Ident: "fn",
					Value: &ast.ExprCall{
						Name: "funcgen",
						Args: []interfaces.Expr{},
						Var:  false,
					},
				},
				&ast.StmtBind{
					Ident: "foo",
					Value: &ast.ExprCall{
						Name: "fn",
						Args: []interfaces.Expr{},
						Var:  true, // comes from a var
					},
				},
			},
		}
		testCases = append(testCases, test{
			name: "simple nested function 1",
			code: `
			func funcgen() {	# returns a function expression
				func() {
					"hello"
				}
			}
			$fn = funcgen()
			$foo = $fn()	# hello
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		fn := &ast.ExprFunc{
			Args: []*interfaces.Arg{
				{
					Name: "x",
					//Type: types.TypeInt,
				},
			},
			//Return: types.TypeInt,
			Body: &ast.ExprCall{
				Name: operators.OperatorFuncName,
				Args: []interfaces.Expr{
					&ast.ExprStr{
						V: "*",
					},
					&ast.ExprVar{
						Name: "x",
					},
					&ast.ExprVar{
						Name: "x",
					},
				},
			},
		}
		//if err := fn.SetType(types.NewType("func(x int) int")); err != nil {
		//	t.Fatal("could not build type")
		//}

		exp := &ast.StmtProg{
			Body: []interfaces.Stmt{
				&ast.StmtImport{
					Name: "iter",
				},
				&ast.StmtBind{
					Ident: "fn",
					Value: fn,
				},
				&ast.StmtBind{
					Ident: "out",
					Value: &ast.ExprCall{
						Name: "iter.map", // does this name lex/parse correctly?
						Args: []interfaces.Expr{
							&ast.ExprList{
								Elements: []interfaces.Expr{
									&ast.ExprInt{
										V: 1,
									},
									&ast.ExprInt{
										V: 2,
									},
									&ast.ExprInt{
										V: 3,
									},
								},
							},
							&ast.ExprVar{
								Name: "fn",
							},
						},
					},
				},
			},
		}

		testCases = append(testCases, test{
			name: "iter.map",
			code: `
			import "iter"
			$fn = func($x) { $x * $x }
			$out = iter.map([1,2,3,], $fn)
			`,
			fail: false,
			exp:  exp,
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
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

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			name, code, fail, exp := tc.name, tc.code, tc.fail, tc.exp

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			str := strings.NewReader(code)
			xast, err := LexParse(str)

			if !fail && err != nil {
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: lex/parse passed, expected fail", index)
				return
			}

			if !fail && xast == nil {
				t.Errorf("test #%d: lex/parse was nil", index)
				return
			}

			if exp == nil {
				return
			}
			if reflect.DeepEqual(xast, exp) {
				return
			}
			// double check because DeepEqual is different since the func exists

			lo := &litter.Options{
				//Compact: false,
				StripPackageNames: true,
				HidePrivateFields: true,
				HideZeroValues:    true,
				FieldExclusions:   regexp.MustCompile(`^(Textarea)$`),
				//FieldFilter       func(reflect.StructField, reflect.Value) bool
				//HomePackage       string
				//Separator         string
				DisablePointerReplacement: true,
			}

			// The litter package adds pointer comments everywhere,
			// which make it not diff correctly. Clean them here!
			pattern := regexp.MustCompile(`\ \/\/\ p[0-9]+$`) // the p0, p1 comments...
			clean := func(s string) string {
				lines := []string{}
				for _, line := range strings.Split(s, "\n") {
					s := pattern.ReplaceAllLiteralString(line, "")
					lines = append(lines, s)
				}
				return strings.Join(lines, "\n")
			}

			lo1 := clean(lo.Sdump(exp))
			lo2 := clean(lo.Sdump(xast))
			if lo1 == lo2 { // simple diff
				return
			}

			// more details, for tricky cases:
			diffable := &pretty.Config{
				Diffable:          true,
				IncludeUnexported: false,
				//PrintStringers: false, // always false!
				//PrintTextMarshalers: false,
				SkipZeroFields: true,
				//Formatter: map[reflect.Type]interface{}{
				//	reflect.TypeOf(ast.Textarea{}): func(x ast.Textarea) string {
				//		return ""
				//	},
				//},
			}
			diff := diffable.Compare(exp, xast)
			if diff == "" { // bonus
				return
			}
			diff = godiff.Diff(lo1, lo2) // for printing

			t.Errorf("test #%d: AST did not match expected", index)
			// TODO: consider making our own recursive print function
			t.Logf("test #%d:   actual: \n\n%s\n", index, lo1)
			t.Logf("test #%d: expected: \n\n%s", index, lo2)
			t.Logf("test #%d: diff:\n%s", index, diff)
		})
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

func TestLexParseWithOffsets1(t *testing.T) {
	code1 := `
	# "file1"
	$a = 42
	$b = true
	$c = 13
	$d = "hello"
	$e = true
	$f = 3.13
	`
	code2 := `
	# "file2"
	# some noop resource
	noop "n0" {
		foo => true,
		bar => false	# this should be a parser error (no comma)
	}
	# hello
	# world
	test "t2" {}
	`
	code3 := `
	# "file3"
	# this is some more code
	test "t3" {}
	`
	str1 := strings.NewReader(code1)
	str2 := strings.NewReader(code2)
	str3 := strings.NewReader(code3)
	// TODO: this is currently in number of lines instead of bytes
	o1 := uint64(len(strings.Split(code1, "\n")) - 1)
	o2 := uint64(len(strings.Split(code2, "\n")) - 1)
	//o1 := uint64(len(code1))
	//o2 := uint64(len(code2))
	t.Logf("o1: %+v", o1)
	t.Logf("o2: %+v", o2)
	t.Logf("o1+o2: %+v", o1+o2)
	readers := io.MultiReader(str1, str2, str3)
	offsets := map[uint64]string{
		0:       "file1",
		o1:      "file2",
		o1 + o2: "file3", // offset is cumulative
	}
	_, err := LexParseWithOffsets(readers, offsets)
	if e, ok := err.(*LexParseErr); ok && e.Err != ErrParseExpectingComma {
		t.Errorf("lex/parse failure, got: %+v", e)
	} else if err == nil {
		t.Errorf("lex/parse success, expected error")
	} else {
		if e.Row != 5 || e.Col != 9 || e.Filename != "file2" {
			t.Errorf("expected error in 'file2' @ 5 x 9, got: '%s' @ %d x %d", e.Filename, e.Row, e.Col)
		}
		t.Logf("file @ row x col: '%s' @ %d x %d", e.Filename, e.Row, e.Col)
		t.Logf("message: %s", e.Str)
		t.Logf("output: %+v", err) // this will be 1-indexed, instead of zero-indexed
	}
}

func TestImportParsing0(t *testing.T) {
	type test struct { // an individual test
		name     string
		fail     bool
		alias    string
		isSystem bool
		isLocal  bool
		isFile   bool
		path     string
		url      string
	}
	testCases := []test{}
	testCases = append(testCases, test{ // index: 0
		name: "",
		fail: true, // can't be empty
	})
	testCases = append(testCases, test{
		name: "/",
		fail: true, // can't be root
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/mgmt",
		alias:   "mgmt",
		isLocal: false,
		path:    "example.com/purpleidea/mgmt/",
		url:     "git://example.com/purpleidea/mgmt",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/mgmt/",
		alias:   "mgmt",
		isLocal: false,
		path:    "example.com/purpleidea/mgmt/",
		url:     "git://example.com/purpleidea/mgmt/",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/mgmt/foo/bar/",
		alias:   "bar",
		isLocal: false,
		path:    "example.com/purpleidea/mgmt/foo/bar/",
		// TODO: change this to be more clever about the clone URL
		//url: "git://example.com/purpleidea/mgmt/",
		// TODO: also consider changing `git` to `https` ?
		url: "git://example.com/purpleidea/mgmt/foo/bar/",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/mgmt-foo",
		alias:   "foo", // prefix is magic
		isLocal: false,
		path:    "example.com/purpleidea/mgmt-foo/",
		url:     "git://example.com/purpleidea/mgmt-foo",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/foo-bar",
		alias:   "foo_bar",
		isLocal: false,
		path:    "example.com/purpleidea/foo-bar/",
		url:     "git://example.com/purpleidea/foo-bar",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/FOO-bar",
		alias:   "foo_bar",
		isLocal: false,
		path:    "example.com/purpleidea/FOO-bar/",
		url:     "git://example.com/purpleidea/FOO-bar",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/foo-BAR",
		alias:   "foo_bar",
		isLocal: false,
		path:    "example.com/purpleidea/foo-BAR/",
		url:     "git://example.com/purpleidea/foo-BAR",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/foo-BAR-baz",
		alias:   "foo_bar_baz",
		isLocal: false,
		path:    "example.com/purpleidea/foo-BAR-baz/",
		url:     "git://example.com/purpleidea/foo-BAR-baz",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/Module-Name",
		alias:   "module_name",
		isLocal: false,
		path:    "example.com/purpleidea/Module-Name/",
		url:     "git://example.com/purpleidea/Module-Name",
	})
	testCases = append(testCases, test{
		name: "git://example.com/purpleidea/foo-",
		fail: true, // trailing dash or underscore
	})
	testCases = append(testCases, test{
		name: "git://example.com/purpleidea/foo_",
		fail: true, // trailing dash or underscore
	})
	testCases = append(testCases, test{
		name:  "/var/lib/mgmt",
		alias: "mgmt",
		fail:  true, // don't allow absolute paths
		//isLocal: true,
		//path: "/var/lib/mgmt",
	})
	testCases = append(testCases, test{
		name:  "/var/lib/mgmt/",
		alias: "mgmt",
		fail:  true, // don't allow absolute paths
		//isLocal: true,
		//path: "/var/lib/mgmt/",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/Module-Name?foo=bar&baz=42",
		alias:   "module_name",
		isLocal: false,
		path:    "example.com/purpleidea/Module-Name/",
		url:     "git://example.com/purpleidea/Module-Name",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/Module-Name/?foo=bar&baz=42",
		alias:   "module_name",
		isLocal: false,
		path:    "example.com/purpleidea/Module-Name/",
		url:     "git://example.com/purpleidea/Module-Name/",
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/Module-Name/?sha1=25ad05cce36d55ce1c55fd7e70a3ab74e321b66e",
		alias:   "module_name",
		isLocal: false,
		path:    "example.com/purpleidea/Module-Name/",
		url:     "git://example.com/purpleidea/Module-Name/",
		// TODO: report the query string info as an additional param
	})
	testCases = append(testCases, test{
		name:    "git://example.com/purpleidea/Module-Name/subpath/foo",
		alias:   "foo",
		isLocal: false,
		path:    "example.com/purpleidea/Module-Name/subpath/foo/",
		url:     "git://example.com/purpleidea/Module-Name/subpath/foo",
	})
	testCases = append(testCases, test{
		name:    "foo/",
		alias:   "foo",
		isLocal: true,
		path:    "foo/",
	})
	testCases = append(testCases, test{
		name:     "foo/bar/",
		alias:    "bar",
		isSystem: false,
		isLocal:  true,
		isFile:   false,
		path:     "foo/bar/",
	})
	testCases = append(testCases, test{
		name:     "foo/bar/baz/",
		alias:    "baz",
		isSystem: false,
		isLocal:  true,
		isFile:   false,
		path:     "foo/bar/baz/",
	})
	testCases = append(testCases, test{
		// import foo.mcl # import a file next to me
		name:     "foo.mcl",
		alias:    "foo",
		isSystem: false,
		isLocal:  true,
		isFile:   true,
		path:     "foo.mcl",
	})
	testCases = append(testCases, test{
		// import server/foo.mcl # import a file in a dir next to me
		name:     "server/foo.mcl",
		alias:    "foo",
		isSystem: false,
		isLocal:  true,
		isFile:   true,
		path:     "server/foo.mcl",
	})
	testCases = append(testCases, test{
		// import a deeper file (not necessarily a good idea)
		name:     "server/vars/blah.mcl",
		alias:    "blah",
		isSystem: false,
		isLocal:  true,
		isFile:   true,
		path:     "server/vars/blah.mcl",
	})
	testCases = append(testCases, test{
		name:     "foo/bar",
		alias:    "bar",
		isSystem: true, // system because not a dir (no trailing slash)
		isLocal:  true, // not really used, but this is what we return
	})
	testCases = append(testCases, test{
		name:     "foo/bar/baz",
		alias:    "baz",
		isSystem: true, // system because not a dir (no trailing slash)
		isLocal:  true, // not really used, but this is what we return
	})
	testCases = append(testCases, test{
		name:     "fmt",
		alias:    "fmt",
		isSystem: true,
		isLocal:  true, // not really used, but this is what we return
	})
	testCases = append(testCases, test{
		name:     "blah",
		alias:    "blah",
		isSystem: true, // even modules that don't exist return true here
		isLocal:  true,
	})
	testCases = append(testCases, test{
		name:     "git:///home/james/code/mgmt-example1/",
		alias:    "example1",
		isSystem: false,
		isLocal:  false,
		// FIXME: do we want to have a special "local" imports dir?
		path: "home/james/code/mgmt-example1/",
		url:  "git:///home/james/code/mgmt-example1/",
	})
	testCases = append(testCases, test{
		name: "git:////home/james/code/mgmt-example1/",
		fail: true, // don't allow double root slash
	})

	t.Logf("ModuleMagicPrefix: %s", langUtil.ModuleMagicPrefix)
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, fail, alias, isSystem, isLocal, isFile, path, url := tc.name, tc.fail, tc.alias, tc.isSystem, tc.isLocal, tc.isFile, tc.path, tc.url

			output, err := langUtil.ParseImportName(name)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseImportName failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseImportName expected error, not nil", index)
				t.Logf("test #%d: output: %+v", index, output)
				return
			}
			if fail { // we failed as expected, don't continue...
				return
			}

			if alias != output.Alias {
				t.Errorf("test #%d: unexpected value for: `Alias`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d:  alias: %s", index, alias)
				return
			}
			if isSystem != output.IsSystem {
				t.Errorf("test #%d: unexpected value for: `IsSystem`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d:   output: %+v", index, output)
				t.Logf("test #%d: isSystem: %t", index, isSystem)
				return
			}
			if isLocal != output.IsLocal {
				t.Errorf("test #%d: unexpected value for: `IsLocal`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d:  output: %+v", index, output)
				t.Logf("test #%d: isLocal: %t", index, isLocal)
				return
			}
			if isFile != output.IsFile {
				t.Errorf("test #%d: unexpected value for: `isFile`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d: isFile: %t", index, isFile)
				return
			}
			if path != output.Path {
				t.Errorf("test #%d: unexpected value for: `Path`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d:   path: %s", index, path)
				return
			}
			if url != output.URL {
				t.Errorf("test #%d: unexpected value for: `URL`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d:    url: %s", index, url)
				return
			}

			// add some additional sanity checking:
			if strings.HasPrefix(path, "/") {
				t.Errorf("test #%d: the path value starts with a / (it should be relative)", index)
			}
			if !isSystem {
				if !strings.HasSuffix(path, "/") && !strings.HasSuffix(path, interfaces.DotFileNameExtension) {
					t.Errorf("test #%d: the path value should be a directory or a code file", index)
				}
			}
		})
	}
}
