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
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"

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
				&StmtBind{
					Ident: "x1",
					Value: &ExprCall{
						Name: "foo1",
						Args: []interfaces.Expr{},
					},
				},
			},
		}
		values = append(values, test{
			name: "func call 1",
			code: `
			$x1 = foo1()
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x1",
					Value: &ExprCall{
						Name: "foo1",
						Args: []interfaces.Expr{
							&ExprInt{
								V: 13,
							},
							&ExprStr{
								V: "hello",
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "func call 2",
			code: `
			$x1 = foo1(13, "hello")
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x1",
					Value: &ExprCall{
						Name: "pkg.foo1",
						Args: []interfaces.Expr{},
					},
				},
			},
		}
		values = append(values, test{
			name: "func call dotted 1",
			code: `
			$x1 = pkg.foo1()
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x1",
					Value: &ExprCall{
						Name: "pkg.foo1",
						Args: []interfaces.Expr{
							&ExprBool{
								V: true,
							},
							&ExprStr{
								V: "hello",
							},
						},
					},
				},
			},
		}
		values = append(values, test{
			name: "func call dotted 2",
			code: `
			$x1 = pkg.foo1(true, "hello")
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		values = append(values, test{
			name: "func call dotted invalid 1",
			code: `
			$x1 = .pkg.foo1(true, "hello")
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "func call dotted invalid 2",
			code: `
			$x1 = pkg.foo1.(true, "hello")
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "func call dotted invalid 3",
			code: `
			$x1 = .pkg.foo1.(true, "hello")
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "func call dotted invalid 4",
			code: `
			$x1 = pkg..foo1(true, "hello")
			`,
			fail: true,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x1",
					Value: &ExprVar{
						Name: "pkg.foo1",
					},
				},
			},
		}
		values = append(values, test{
			name: "dotted var 1",
			code: `
			$x1 = $pkg.foo1
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtBind{
					Ident: "x1",
					Value: &ExprVar{
						Name: "pkg.foo1.bar",
					},
				},
			},
		}
		values = append(values, test{
			name: "dotted var 2",
			code: `
			$x1 = $pkg.foo1.bar
			`,
			fail: false,
			exp:  exp,
		})
	}
	{
		values = append(values, test{
			name: "invalid dotted var 1",
			code: `
			$x1 = $.pkg.foo1.bar
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "invalid dotted var 2",
			code: `
			$x1 = $pkg.foo1.bar.
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "invalid dotted var 3",
			code: `
			$x1 = $.pkg.foo1.bar.
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "invalid dotted var 4",
			code: `
			$x1 = $pkg..foo1.bar
			`,
			fail: true,
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
					Contents: []StmtResContents{
						&StmtResField{
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
	{
		values = append(values, test{
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
		values = append(values, test{
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
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtClass{
					Name: "c1",
					Body: &StmtProg{
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
											V: "hello",
										},
									},
								},
							},
						},
					},
				},
				&StmtInclude{
					Name: "c1",
				},
			},
		}
		values = append(values, test{
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
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtClass{
					Name: "c1",
					Body: &StmtProg{
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
											V: "hello",
										},
									},
								},
							},
						},
					},
				},
				&StmtInclude{
					Name: "pkg.c1",
				},
			},
		}
		values = append(values, test{
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
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtClass{
					Name: "c1",
					Body: &StmtProg{
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
											V: "hello",
										},
									},
								},
							},
						},
					},
				},
				&StmtInclude{
					Name: "pkg.ns.c1",
				},
			},
		}
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
			name: "simple dotted invalid include 1",
			code: `
			class .foo.c1 {
			}
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "simple dotted invalid include 2",
			code: `
			class foo.c1. {
			}
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "simple dotted invalid include 3",
			code: `
			class .foo.c1. {
			}
			`,
			fail: true,
		})
	}
	{
		values = append(values, test{
			name: "simple dotted invalid include 4",
			code: `
			class foo..c1 {
			}
			`,
			fail: true,
		})
	}
	{
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtClass{
					Name: "c1",
					Args: []*Arg{
						{
							Name: "a",
							//Type: &types.Type{},
						},
						{
							Name: "b",
							//Type: &types.Type{},
						},
					},
					Body: &StmtProg{
						Prog: []interfaces.Stmt{
							&StmtRes{
								Kind: "test",
								Name: &ExprVar{
									Name: "a",
								},
								Contents: []StmtResContents{
									&StmtResField{
										Field: "stringptr",
										Value: &ExprVar{
											Name: "b",
										},
									},
								},
							},
						},
					},
				},
				&StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ExprStr{
							V: "t1",
						},
						&ExprStr{
							V: "hello",
						},
					},
				},
				&StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ExprStr{
							V: "t2",
						},
						&ExprStr{
							V: "world",
						},
					},
				},
			},
		}
		values = append(values, test{
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
		exp := &StmtProg{
			Prog: []interfaces.Stmt{
				&StmtClass{
					Name: "c1",
					Args: []*Arg{
						{
							Name: "a",
							Type: types.TypeStr,
						},
						{
							Name: "b",
							//Type: &types.Type{},
						},
					},
					Body: &StmtProg{
						Prog: []interfaces.Stmt{
							&StmtRes{
								Kind: "test",
								Name: &ExprVar{
									Name: "a",
								},
								Contents: []StmtResContents{
									&StmtResField{
										Field: "stringptr",
										Value: &ExprVar{
											Name: "b",
										},
									},
								},
							},
						},
					},
				},
				&StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ExprStr{
							V: "t1",
						},
						&ExprStr{
							V: "hello",
						},
					},
				},
				&StmtInclude{
					Name: "c1",
					Args: []interfaces.Expr{
						&ExprStr{
							V: "t2",
						},
						&ExprStr{
							V: "world",
						},
					},
				},
			},
		}
		values = append(values, test{
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

	names := []string{}
	for index, test := range values { // run all the tests
		name, code, fail, exp := test.name, test.code, test.fail, test.exp

		if name == "" {
			name = "<sub test not named>"
		}
		if util.StrInList(name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, name)
			continue
		}
		names = append(names, name)

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
		name   string
		fail   bool
		alias  string
		system bool
		local  bool
		path   string
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
		name:  "git://example.com/purpleidea/mgmt",
		alias: "mgmt",
		local: false,
		path:  "example.com/purpleidea/mgmt/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/mgmt/",
		alias: "mgmt",
		local: false,
		path:  "example.com/purpleidea/mgmt/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/mgmt/foo/bar/",
		alias: "bar",
		local: false,
		path:  "example.com/purpleidea/mgmt/foo/bar/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/mgmt-foo",
		alias: "foo", // prefix is magic
		local: false,
		path:  "example.com/purpleidea/mgmt-foo/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/foo-bar",
		alias: "foo_bar",
		local: false,
		path:  "example.com/purpleidea/foo-bar/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/FOO-bar",
		alias: "foo_bar",
		local: false,
		path:  "example.com/purpleidea/FOO-bar/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/foo-BAR",
		alias: "foo_bar",
		local: false,
		path:  "example.com/purpleidea/foo-BAR/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/foo-BAR-baz",
		alias: "foo_bar_baz",
		local: false,
		path:  "example.com/purpleidea/foo-BAR-baz/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/Module-Name",
		alias: "module_name",
		local: false,
		path:  "example.com/purpleidea/Module-Name/",
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
		//local: true,
		//path: "/var/lib/mgmt",
	})
	testCases = append(testCases, test{
		name:  "/var/lib/mgmt/",
		alias: "mgmt",
		fail:  true, // don't allow absolute paths
		//local: true,
		//path: "/var/lib/mgmt/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/Module-Name?foo=bar&baz=42",
		alias: "module_name",
		local: false,
		path:  "example.com/purpleidea/Module-Name/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/Module-Name/?foo=bar&baz=42",
		alias: "module_name",
		local: false,
		path:  "example.com/purpleidea/Module-Name/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/Module-Name/?sha1=25ad05cce36d55ce1c55fd7e70a3ab74e321b66e",
		alias: "module_name",
		local: false,
		path:  "example.com/purpleidea/Module-Name/",
	})
	testCases = append(testCases, test{
		name:  "git://example.com/purpleidea/Module-Name/subpath/foo",
		alias: "foo",
		local: false,
		path:  "example.com/purpleidea/Module-Name/subpath/foo/",
	})
	testCases = append(testCases, test{
		name:  "foo/",
		alias: "foo",
		local: true,
		path:  "foo/",
	})
	testCases = append(testCases, test{
		name:   "foo/bar",
		alias:  "bar",
		system: true, // system because not a dir (no trailing slash)
		local:  true, // not really used, but this is what we return
	})
	testCases = append(testCases, test{
		name:   "foo/bar/baz",
		alias:  "baz",
		system: true, // system because not a dir (no trailing slash)
		local:  true, // not really used, but this is what we return
	})
	testCases = append(testCases, test{
		name:   "fmt",
		alias:  "fmt",
		system: true,
		local:  true, // not really used, but this is what we return
	})
	testCases = append(testCases, test{
		name:   "blah",
		alias:  "blah",
		system: true, // even modules that don't exist return true here
		local:  true,
	})

	t.Logf("ModuleMagicPrefix: %s", ModuleMagicPrefix)
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)
		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, fail, alias, system, local, path := tc.name, tc.fail, tc.alias, tc.system, tc.local, tc.path

			output, err := ParseImportName(name)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseImportName failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseImportName expected error, not nil", index)
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
			if system != output.System {
				t.Errorf("test #%d: unexpected value for: `System`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d: system: %t", index, system)
				return

			}
			if local != output.Local {
				t.Errorf("test #%d: unexpected value for: `Local`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d:  local: %t", index, local)
				return

			}
			if path != output.Path {
				t.Errorf("test #%d: unexpected value for: `Path`", index)
				//t.Logf("test #%d:  input: %s", index, name)
				t.Logf("test #%d: output: %+v", index, output)
				t.Logf("test #%d:   path: %s", index, path)
				return

			}
		})
	}
}
