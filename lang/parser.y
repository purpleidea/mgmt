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

%{
package lang

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	errstrParseAdditionalEquals = "additional equals in bind statement"
	errstrParseExpectingComma = "expecting trailing comma"
)

func init() {
	yyErrorVerbose = true // set the global that enables showing full errors
}
%}

%union {
	row int
	col int

	//err error // TODO: if we ever match ERROR in the parser

	bool    bool
	str     string
	int     int64 // this is the .int as seen in lexer.nex
	float   float64

	strSlice []string

	typ   *types.Type

	stmts []interfaces.Stmt
	stmt interfaces.Stmt

	exprs []interfaces.Expr
	expr  interfaces.Expr

	mapKVs []*ExprMapKV
	mapKV  *ExprMapKV

	structFields []*ExprStructField
	structField  *ExprStructField

	args []*Arg
	arg  *Arg

	resContents []StmtResContents // interface
	resField    *StmtResField
	resEdge     *StmtResEdge

	edgeHalfList []*StmtEdgeHalf
	edgeHalf     *StmtEdgeHalf
}

%token OPEN_CURLY CLOSE_CURLY
%token OPEN_PAREN CLOSE_PAREN
%token OPEN_BRACK CLOSE_BRACK
%token IF ELSE
%token STRING BOOL INTEGER FLOAT
%token EQUALS
%token COMMA COLON SEMICOLON
%token ELVIS ROCKET ARROW DOT
%token STR_IDENTIFIER BOOL_IDENTIFIER INT_IDENTIFIER FLOAT_IDENTIFIER
%token STRUCT_IDENTIFIER VARIANT_IDENTIFIER VAR_IDENTIFIER IDENTIFIER
%token VAR_IDENTIFIER_HX CAPITALIZED_IDENTIFIER
%token CLASS_IDENTIFIER INCLUDE_IDENTIFIER
%token COMMENT ERROR

// precedence table
// "Operator precedence is determined by the line ordering of the declarations;
// the higher the line number of the declaration (lower on the page or screen),
// the higher the precedence."
// From: https://www.gnu.org/software/bison/manual/html_node/Infix-Calc.html
// FIXME: a yacc specialist should check the precedence and add more tests!
%left AND OR
%nonassoc LT GT LTE GTE EQ NEQ	// TODO: is %nonassoc correct for all of these?
%left PLUS MINUS
%left MULTIPLY DIVIDE
%right NOT
//%right EXP	// exponentiation
%nonassoc IN	// TODO: is %nonassoc correct for this?

%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET BOOL CLOSE_CURLY: errstrParseExpectingComma
%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET STRING CLOSE_CURLY: errstrParseExpectingComma
%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET INTEGER CLOSE_CURLY: errstrParseExpectingComma
%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET FLOAT CLOSE_CURLY: errstrParseExpectingComma

%error VAR_IDENTIFIER EQ BOOL: errstrParseAdditionalEquals
%error VAR_IDENTIFIER EQ STRING: errstrParseAdditionalEquals
%error VAR_IDENTIFIER EQ INTEGER: errstrParseAdditionalEquals
%error VAR_IDENTIFIER EQ FLOAT: errstrParseAdditionalEquals

%%
top:
	prog
	{
		posLast(yylex, yyDollar) // our pos
		// store the AST in the struct that we previously passed in
		lp := cast(yylex)
		lp.ast = $1.stmt
		// this is equivalent to:
		//lp := yylex.(*Lexer).parseResult
		//lp.(*lexParseAST).ast = $1.stmt
	}
;
prog:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtProg{
			Prog: []interfaces.Stmt{},
		}
	}
|	prog stmt
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: should we just skip comments for now?
		//if _, ok := $2.stmt.(*StmtComment); !ok {
		//}
		if stmt, ok := $1.stmt.(*StmtProg); ok {
			stmts := stmt.Prog
			stmts = append(stmts, $2.stmt)
			$$.stmt = &StmtProg{
				Prog: stmts,
			}
		}
	}
;
stmt:
	COMMENT
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtComment{
			Value: $1.str,
		}
	}
|	bind
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = $1.stmt
	}
|	resource
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = $1.stmt
	}
|	edge
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = $1.stmt
	}
|	IF expr OPEN_CURLY prog CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtIf{
			Condition:  $2.expr,
			ThenBranch: $4.stmt,
			//ElseBranch: nil,
		}
	}
|	IF expr OPEN_CURLY prog CLOSE_CURLY ELSE OPEN_CURLY prog CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtIf{
			Condition:  $2.expr,
			ThenBranch: $4.stmt,
			ElseBranch: $8.stmt,
		}
	}
	// `class name { <prog> }`
|	CLASS_IDENTIFIER IDENTIFIER OPEN_CURLY prog CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtClass{
			Name: $2.str,
			Args: nil,
			Body: $4.stmt,
		}
	}
	// `class name(<arg>) { <prog> }`
	// `class name(<arg>, <arg>) { <prog> }`
|	CLASS_IDENTIFIER IDENTIFIER OPEN_PAREN args CLOSE_PAREN OPEN_CURLY prog CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtClass{
			Name: $2.str,
			Args: $4.args,
			Body: $7.stmt,
		}
	}
	// `include name`
|	INCLUDE_IDENTIFIER IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtInclude{
			Name: $2.str,
		}
	}
	// `include name(...)`
|	INCLUDE_IDENTIFIER IDENTIFIER OPEN_PAREN call_args CLOSE_PAREN
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtInclude{
			Name: $2.str,
			Args: $4.exprs,
		}
	}
/*
	// resource bind
|	rbind
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = $1.stmt
	}
*/
;
expr:
	BOOL
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprBool{
			V: $1.bool,
		}
	}
|	STRING
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprStr{
			V: $1.str,
		}
	}
|	INTEGER
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprInt{
			V: $1.int,
		}
	}
|	FLOAT
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprFloat{
			V: $1.float,
		}
	}
|	list
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: list could be squashed in here directly...
		$$.expr = $1.expr
	}
|	map
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: map could be squashed in here directly...
		$$.expr = $1.expr
	}
|	struct
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: struct could be squashed in here directly...
		$$.expr = $1.expr
	}
|	call
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: call could be squashed in here directly...
		$$.expr = $1.expr
	}
|	var
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: var could be squashed in here directly...
		$$.expr = $1.expr
	}
|	IF expr OPEN_CURLY expr CLOSE_CURLY ELSE OPEN_CURLY expr CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprIf{
			Condition:  $2.expr,
			ThenBranch: $4.expr,
			ElseBranch: $8.expr,
		}
	}
	// parenthesis wrap an expression for precedence
|	OPEN_PAREN expr CLOSE_PAREN
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = $2.expr
	}
;
list:
	// `[42, 0, -13]`
	OPEN_BRACK list_elements CLOSE_BRACK
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprList{
			Elements: $2.exprs,
		}
	}
;
list_elements:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.exprs = []interfaces.Expr{}
	}
|	list_elements list_element
	{
		posLast(yylex, yyDollar) // our pos
		$$.exprs = append($1.exprs, $2.expr)
	}
;
list_element:
	expr COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = $1.expr
	}
;
map:
	// `{"hello" => "there", "world" => "big",}`
	OPEN_CURLY map_kvs CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprMap{
			KVs: $2.mapKVs,
		}
	}
;
map_kvs:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.mapKVs = []*ExprMapKV{}
	}
|	map_kvs map_kv
	{
		posLast(yylex, yyDollar) // our pos
		$$.mapKVs = append($1.mapKVs, $2.mapKV)
	}
;
map_kv:
	expr ROCKET expr COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.mapKV = &ExprMapKV{
			Key: $1.expr,
			Val: $3.expr,
		}
	}
;
struct:
	// `struct{answer => 0, truth => false, hello => "world",}`
	STRUCT_IDENTIFIER OPEN_CURLY struct_fields CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprStruct{
			Fields: $3.structFields,
		}
	}
;
struct_fields:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.structFields = []*ExprStructField{}
	}
|	struct_fields struct_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.structFields = append($1.structFields, $2.structField)
	}
;
struct_field:
	IDENTIFIER ROCKET expr COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.structField = &ExprStructField{
			Name:  $1.str,
			Value: $3.expr,
		}
	}
;
call:
	IDENTIFIER OPEN_PAREN call_args CLOSE_PAREN
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: $1.str,
			Args: $3.exprs,
		}
	}
|	expr PLUS expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{          // operator first
					V: $2.str, // for PLUS this is a `+` character
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr MINUS expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr MULTIPLY expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr DIVIDE expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr EQ expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr NEQ expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr LT expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr GT expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr LTE expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr GTE expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr AND expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	expr OR expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
	}
|	NOT expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: operatorFuncName,
			Args: []interfaces.Expr{
				&ExprStr{ // operator first
					V: $1.str,
				},
				$2.expr,
			},
		}
	}
|	VAR_IDENTIFIER_HX
	// get the N-th historical value, eg: $foo{3} is equivalent to: history($foo, 3)
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: historyFuncName,
			Args: []interfaces.Expr{
				&ExprVar{
					Name: $1.str,
				},
				&ExprInt{
					V: $1.int,
				},
			},
		}
	}
//|	VAR_IDENTIFIER OPEN_CURLY INTEGER CLOSE_CURLY
//	{
//		posLast(yylex, yyDollar) // our pos
//		$$.expr = &ExprCall{
//			Name: historyFuncName,
//			Args: []interfaces.Expr{
//				&ExprVar{
//					Name: $1.str,
//				},
//				&ExprInt{
//					V: $3.int,
//				},
//			},
//		}
//	}
|	expr IN expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprCall{
			Name: containsFuncName,
			Args: []interfaces.Expr{
				$1.expr,
				$3.expr,
			},
		}
	}
;
// list order gets us the position of the arg, but named params would work too!
// this is also used by the include statement when the called class uses args!
call_args:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.exprs = []interfaces.Expr{}
	}
	// seems that "left recursion" works here... thanks parser generator!
|	call_args COMMA expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.exprs = append($1.exprs, $3.expr)
	}
|	expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.exprs = append([]interfaces.Expr{}, $1.expr)
	}
;
var:
	VAR_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.expr = &ExprVar{
			Name: $1.str,
		}
	}
;
args:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = []*Arg{}
	}
|	args COMMA arg
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append($1.args, $3.arg)
	}
|	arg
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append([]*Arg{}, $1.arg)
	}
;
arg:
	// `$x`
	VAR_IDENTIFIER
	{
		$$.arg = &Arg{
			Name: $1.str,
		}
	}
	// `$x <type>`
|	VAR_IDENTIFIER type
	{
		$$.arg = &Arg{
			Name: $1.str,
			Type: $2.typ,
		}
	}
;
bind:
	VAR_IDENTIFIER EQUALS expr
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtBind{
			Ident: $1.str,
			Value: $3.expr,
		}
	}
|	VAR_IDENTIFIER type EQUALS expr
	{
		posLast(yylex, yyDollar) // our pos
		var expr interfaces.Expr = $4.expr
		if err := expr.SetType($2.typ); err != nil {
			// this will ultimately cause a parser error to occur...
			yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
		}
		$$.stmt = &StmtBind{
			Ident: $1.str,
			Value: expr,
		}
	}
;
/* TODO: do we want to include this?
// resource bind
rbind:
	VAR_IDENTIFIER EQUALS resource
	{
		posLast(yylex, yyDollar) // our pos
		// XXX: this kind of bind is different than the others, because
		// it can only really be used for send->recv stuff, eg:
		// foo.SomeString -> bar.SomeOtherString
		$$.expr = &StmtBind{
			Ident: $1.str,
			Value: $3.stmt,
		}
	}
;
*/
resource:
	IDENTIFIER expr OPEN_CURLY resource_body CLOSE_CURLY
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtRes{
			Kind:     $1.str,
			Name:     $2.expr,
			Contents: $4.resContents,
		}
	}
;
resource_body:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = []StmtResContents{}
	}
|	resource_body resource_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resField)
	}
|	resource_body conditional_resource_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resField)
	}
|	resource_body resource_edge
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resEdge)
	}
|	resource_body conditional_resource_edge
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resEdge)
	}
;
resource_field:
	IDENTIFIER ROCKET expr COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.resField = &StmtResField{
			Field: $1.str,
			Value: $3.expr,
		}
	}
;
conditional_resource_field:
	// content => $present ?: "hello",
	IDENTIFIER ROCKET expr ELVIS expr COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.resField = &StmtResField{
			Field:     $1.str,
			Value:     $5.expr,
			Condition: $3.expr,
		}
	}
;
resource_edge:
	// Before => Test["t1"],
	CAPITALIZED_IDENTIFIER ROCKET edge_half COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.resEdge = &StmtResEdge{
			Property: $1.str,
			EdgeHalf: $3.edgeHalf,
		}
	}
;
conditional_resource_edge:
	// Before => $present ?: Test["t1"],
	CAPITALIZED_IDENTIFIER ROCKET expr ELVIS edge_half COMMA
	{
		posLast(yylex, yyDollar) // our pos
		$$.resEdge = &StmtResEdge{
			Property:  $1.str,
			EdgeHalf:  $5.edgeHalf,
			Condition: $3.expr,
		}
	}
;
edge:
	// TODO: we could technically prevent single edge_half pieces from being
	// parsed, but it's probably more work than is necessary...
	// Test["t1"] -> Test["t2"] -> Test["t3"] # chain or pair
	edge_half_list
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtEdge{
			EdgeHalfList: $1.edgeHalfList,
			//Notify: false, // unused here
		}
	}
	// Test["t1"].foo_send -> Test["t2"].blah_recv # send/recv
|	edge_half_sendrecv ARROW edge_half_sendrecv
	{
		posLast(yylex, yyDollar) // our pos
		$$.stmt = &StmtEdge{
			EdgeHalfList: []*StmtEdgeHalf{
				$1.edgeHalf,
				$3.edgeHalf,
			},
			//Notify: false, // unused here, it is implied (i think)
		}
	}
;
edge_half_list:
	edge_half
	{
		posLast(yylex, yyDollar) // our pos
		$$.edgeHalfList = []*StmtEdgeHalf{$1.edgeHalf}
	}
|	edge_half_list ARROW edge_half
	{
		posLast(yylex, yyDollar) // our pos
		$$.edgeHalfList = append($1.edgeHalfList, $3.edgeHalf)
	}
;
edge_half:
	// eg: Test["t1"]
	CAPITALIZED_IDENTIFIER OPEN_BRACK expr CLOSE_BRACK
	{
		posLast(yylex, yyDollar) // our pos
		$$.edgeHalf = &StmtEdgeHalf{
			Kind: $1.str,
			Name: $3.expr,
			//SendRecv: "", // unused
		}
	}
;
edge_half_sendrecv:
	// eg: Test["t1"].foo_send
	CAPITALIZED_IDENTIFIER OPEN_BRACK expr CLOSE_BRACK DOT IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.edgeHalf = &StmtEdgeHalf{
			Kind: $1.str,
			Name: $3.expr,
			SendRecv: $6.str,
		}
	}
;
type:
	BOOL_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType($1.str) // "bool"
	}
|	STR_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType($1.str) // "str"
	}
|	INT_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType($1.str) // "int"
	}
|	FLOAT_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType($1.str) // "float"
	}
|	OPEN_BRACK CLOSE_BRACK type
	// list: []int or [][]str (with recursion)
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType("[]" + $3.typ.String())
	}
|	OPEN_CURLY type COLON type CLOSE_CURLY
	// map: {str: int} or {str: []int}
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType(fmt.Sprintf("{%s: %s}", $2.typ.String(), $4.typ.String()))
	}
|	STRUCT_IDENTIFIER OPEN_CURLY type_struct_fields CLOSE_CURLY
	// struct: struct{} or struct{a bool} or struct{a bool; bb int}
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType(fmt.Sprintf("%s{%s}", $1.str, strings.Join($3.strSlice, "; ")))
	}
|	VARIANT_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType($1.str) // "variant"
	}
;
type_struct_fields:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.strSlice = []string{}
	}
|	type_struct_fields SEMICOLON type_struct_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.strSlice = append($1.strSlice, $3.str)
	}
|	type_struct_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.strSlice = []string{$1.str}
	}
;
type_struct_field:
	IDENTIFIER type
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = fmt.Sprintf("%s %s", $1.str, $2.typ.String())
	}
;
%%
// pos is a helper function used to track the position in the parser.
func pos(y yyLexer, dollar yySymType) {
	lp := cast(y)
	lp.row = dollar.row
	lp.col = dollar.col
	// FIXME: in some cases the before last value is most meaningful...
	//lp.row = append(lp.row, dollar.row)
	//lp.col = append(lp.col, dollar.col)
	//log.Printf("parse: %d x %d", lp.row, lp.col)
	return
}

// cast is used to pull out the parser run-specific struct we store our AST in.
// this is usually called in the parser.
func cast(y yyLexer) *lexParseAST {
	x := y.(*Lexer).parseResult
	return x.(*lexParseAST)
}

// postLast pulls out the "last token" and does a pos with that. This is a hack!
func posLast(y yyLexer, dollars []yySymType) {
	// pick the last token in the set matched by the parser
	pos(y, dollars[len(dollars)-1]) // our pos
}

// cast is used to pull out the parser run-specific struct we store our AST in.
// this is usually called in the lexer.
func (yylex *Lexer) cast() *lexParseAST {
	return yylex.parseResult.(*lexParseAST)
}

// pos is a helper function used to track the position in the lexer.
func (yylex *Lexer) pos(lval *yySymType) {
	lval.row = yylex.Line()
	lval.col = yylex.Column()
	// TODO: we could use: `s := yylex.Text()` to calculate a delta length!
	//log.Printf("lexer: %d x %d", lval.row, lval.col)
}

// Error is the error handler which gets called on a parsing error.
func (yylex *Lexer) Error(str string) {
	lp := yylex.cast()
	if str != "" {
		// This error came from the parser. It is usually also set when
		// the lexer fails, because it ends up generating ERROR tokens,
		// which most parsers usually don't match and store in the AST.
		err := ErrParseError // TODO: add more specific types...
		if strings.HasSuffix(str, ErrParseAdditionalEquals.Error()) {
			err = ErrParseAdditionalEquals
		} else if strings.HasSuffix(str, ErrParseExpectingComma.Error()) {
			err = ErrParseExpectingComma
		} else if strings.HasPrefix(str, ErrParseSetType.Error()) {
			err = ErrParseSetType
		}
		lp.parseErr = &LexParseErr{
			Err: err,
			Str: str,
			// FIXME: get these values, by tracking pos in parser...
			// FIXME: currently, the values we get are mostly wrong!
			Row: lp.row, //lp.row[len(lp.row)-1],
			Col: lp.col, //lp.col[len(lp.col)-1],
		}
	}
}
