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

%{
package parser

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/operators"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
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

	bool  bool
	str   string
	int   int64 // this is the .int as seen in lexer.nex
	float float64

	typ *types.Type

	stmts []interfaces.Stmt
	stmt interfaces.Stmt

	exprs []interfaces.Expr
	expr  interfaces.Expr

	mapKVs []*ast.ExprMapKV
	mapKV  *ast.ExprMapKV

	structFields []*ast.ExprStructField
	structField  *ast.ExprStructField

	args []*interfaces.Arg
	arg  *interfaces.Arg

	resContents []ast.StmtResContents // interface
	resField    *ast.StmtResField
	resEdge     *ast.StmtResEdge
	resMeta     *ast.StmtResMeta

	edgeHalfList []*ast.StmtEdgeHalf
	edgeHalf     *ast.StmtEdgeHalf
}

%token OPEN_CURLY CLOSE_CURLY
%token OPEN_PAREN CLOSE_PAREN
%token OPEN_BRACK CLOSE_BRACK
%token IF ELSE FOR FORKV
%token BOOL STRING INTEGER FLOAT
%token EQUALS DOLLAR
%token COMMA COLON SEMICOLON
%token ELVIS DEFAULT ROCKET ARROW DOT
%token BOOL_IDENTIFIER STR_IDENTIFIER INT_IDENTIFIER FLOAT_IDENTIFIER
%token MAP_IDENTIFIER STRUCT_IDENTIFIER VARIANT_IDENTIFIER
%token IDENTIFIER CAPITALIZED_IDENTIFIER
%token FUNC_IDENTIFIER
%token CLASS_IDENTIFIER INCLUDE_IDENTIFIER
%token IMPORT_IDENTIFIER AS_IDENTIFIER
%token COMMENT ERROR
%token COLLECT_IDENTIFIER
%token PANIC_IDENTIFIER

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
%nonassoc ARROW		// XXX: is %nonassoc correct for this?
%nonassoc DEFAULT	// XXX: is %nonassoc correct for this?
%nonassoc OPEN_BRACK	// XXX: is %nonassoc correct for this?
%nonassoc IN		// XXX: is %nonassoc correct for this?

%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET BOOL CLOSE_CURLY: errstrParseExpectingComma
%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET STRING CLOSE_CURLY: errstrParseExpectingComma
%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET INTEGER CLOSE_CURLY: errstrParseExpectingComma
%error IDENTIFIER STRING OPEN_CURLY IDENTIFIER ROCKET FLOAT CLOSE_CURLY: errstrParseExpectingComma

%error var_identifier EQ BOOL: errstrParseAdditionalEquals
%error var_identifier EQ STRING: errstrParseAdditionalEquals
%error var_identifier EQ INTEGER: errstrParseAdditionalEquals
%error var_identifier EQ FLOAT: errstrParseAdditionalEquals

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
		$$.stmt = &ast.StmtProg{
			Body: []interfaces.Stmt{},
		}
	}
|	prog stmt
	{
		posLast(yylex, yyDollar) // our pos
		// TODO: should we just skip comments for now?
		//if _, ok := $2.stmt.(*ast.StmtComment); !ok {
		//}
		if stmt, ok := $1.stmt.(*ast.StmtProg); ok {
			stmts := stmt.Body
			stmts = append(stmts, $2.stmt)
			$$.stmt = &ast.StmtProg{
				Body: stmts,
			}
			locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
		}
	}
;
stmt:
	COMMENT
	{
		$$.stmt = &ast.StmtComment{
			Value: $1.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	bind
	{
		$$.stmt = $1.stmt
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	panic
	{
		$$.stmt = $1.stmt
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	collect
	{
		$$.stmt = $1.stmt
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	resource
	{
		$$.stmt = $1.stmt
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	edge
	{
		$$.stmt = $1.stmt
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	IF expr OPEN_CURLY prog CLOSE_CURLY
	{
		$$.stmt = &ast.StmtIf{
			Condition:  $2.expr,
			ThenBranch: $4.stmt,
			//ElseBranch: nil,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
|	IF expr OPEN_CURLY prog CLOSE_CURLY ELSE OPEN_CURLY prog CLOSE_CURLY
	{
		$$.stmt = &ast.StmtIf{
			Condition:  $2.expr,
			ThenBranch: $4.stmt,
			ElseBranch: $8.stmt,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// iterate over lists
	// `for $index, $value in $list { <body> }`
|	FOR var_identifier COMMA var_identifier IN expr OPEN_CURLY prog CLOSE_CURLY
	{
		$$.stmt = &ast.StmtFor{
			Index: $2.str, // no $ prefix
			Value: $4.str, // no $ prefix
			Expr:  $6.expr, // XXX: name this List ?
			Body:  $8.stmt,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// iterate over maps
	// `forkv $key, $val in $map { <body> }`
|	FORKV var_identifier COMMA var_identifier IN expr OPEN_CURLY prog CLOSE_CURLY
	{
		$$.stmt = &ast.StmtForKV{
			Key:  $2.str, // no $ prefix
			Val:  $4.str, // no $ prefix
			Expr: $6.expr, // XXX: name this Map ?
			Body: $8.stmt,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// this is the named version, iow, a user-defined function (statement)
	// `func name() { <expr> }`
	// `func name(<arg>) { <expr> }`
	// `func name(<arg>, <arg>) { <expr> }`
|	FUNC_IDENTIFIER IDENTIFIER OPEN_PAREN args CLOSE_PAREN OPEN_CURLY expr CLOSE_CURLY
	{
		$$.stmt = &ast.StmtFunc{
			Name: $2.str,
			Func: &ast.ExprFunc{
				Title:  $2.str,
				Args:   $4.args,
				Return: nil,
				Body:   $7.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `func name(...) <type> { <expr> }`
|	FUNC_IDENTIFIER IDENTIFIER OPEN_PAREN args CLOSE_PAREN type OPEN_CURLY expr CLOSE_CURLY
	{
		fn := &ast.ExprFunc{
			Title:  $2.str,
			Args:   $4.args,
			Return: $6.typ, // return type is known
			Body:   $8.expr,
		}
		isFullyTyped := $6.typ != nil // true if set
		m := make(map[string]*types.Type)
		ord := []string{}
		for _, a := range $4.args {
			if a.Type == nil {
				// at least one is unknown, can't run SetType...
				isFullyTyped = false
				break
			}
			m[a.Name] = a.Type
			ord = append(ord, a.Name)
		}
		var typ *types.Type
		if isFullyTyped {
			typ = &types.Type{
				Kind: types.KindFunc,
				Map:  m,
				Ord:  ord,
				Out:  $6.typ,
			}
			// XXX: We might still need to do this for now...
			if err := fn.SetType(typ); err != nil {
				// this will ultimately cause a parser error to occur...
				yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
			}
		}
		$$.stmt = &ast.StmtFunc{
			Name: $2.str,
			Func: fn,
			Type: typ, // sam says add the type here instead...
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `class name { <prog> }`
|	CLASS_IDENTIFIER colon_identifier OPEN_CURLY prog CLOSE_CURLY
	{
		$$.stmt = &ast.StmtClass{
			Name: $2.str,
			Args: nil,
			Body: $4.stmt,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `class name(<arg>) { <prog> }`
	// `class name(<arg>, <arg>) { <prog> }`
|	CLASS_IDENTIFIER colon_identifier OPEN_PAREN args CLOSE_PAREN OPEN_CURLY prog CLOSE_CURLY
	{
		$$.stmt = &ast.StmtClass{
			Name: $2.str,
			Args: $4.args,
			Body: $7.stmt,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `include name`
|	INCLUDE_IDENTIFIER dotted_identifier
	{
		$$.stmt = &ast.StmtInclude{
			Name: $2.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `include name(...)`
|	INCLUDE_IDENTIFIER dotted_identifier OPEN_PAREN call_args CLOSE_PAREN
	{
		$$.stmt = &ast.StmtInclude{
			Name: $2.str,
			Args: $4.exprs,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `include name as foo`
	// TODO: should we support: `include name as *`
|	INCLUDE_IDENTIFIER dotted_identifier AS_IDENTIFIER IDENTIFIER
	{
		$$.stmt = &ast.StmtInclude{
			Name:  $2.str,
			Alias: $4.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `include name(...) as foo`
	// TODO: should we support: `include name(...) as *`
|	INCLUDE_IDENTIFIER dotted_identifier OPEN_PAREN call_args CLOSE_PAREN AS_IDENTIFIER IDENTIFIER
	{
		$$.stmt = &ast.StmtInclude{
			Name:  $2.str,
			Args:  $4.exprs,
			Alias: $7.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `import "name"`
|	IMPORT_IDENTIFIER STRING
	{
		$$.stmt = &ast.StmtImport{
			Name: $2.str,
			//Alias: "",
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `import "name" as alias`
|	IMPORT_IDENTIFIER STRING AS_IDENTIFIER IDENTIFIER
	{
		$$.stmt = &ast.StmtImport{
			Name:  $2.str,
			Alias: $4.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `import "name" as *`
|	IMPORT_IDENTIFIER STRING AS_IDENTIFIER MULTIPLY
	{
		$$.stmt = &ast.StmtImport{
			Name:  $2.str,
			Alias: $4.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
/*
	// resource bind
|	rbind
	{
		$$.stmt = $1.stmt
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
*/
;
expr:
	BOOL
	{
		$$.expr = &ast.ExprBool{
			V: $1.bool,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	STRING
	{
		$$.expr = &ast.ExprStr{
			V: $1.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	INTEGER
	{
		$$.expr = &ast.ExprInt{
			V: $1.int,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	FLOAT
	{
		$$.expr = &ast.ExprFloat{
			V: $1.float,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	list
	{
		// TODO: list could be squashed in here directly...
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	map
	{
		// TODO: map could be squashed in here directly...
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	struct
	{
		// TODO: struct could be squashed in here directly...
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	call
	{
		// TODO: call could be squashed in here directly...
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	var
	{
		// TODO: var could be squashed in here directly...
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	func
	{
		// TODO: var could be squashed in here directly...
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	IF expr OPEN_CURLY expr CLOSE_CURLY ELSE OPEN_CURLY expr CLOSE_CURLY
	{
		$$.expr = &ast.ExprIf{
			Condition:  $2.expr,
			ThenBranch: $4.expr,
			ElseBranch: $8.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// parenthesis wrap an expression for precedence
|	OPEN_PAREN expr CLOSE_PAREN
	{
		$$.expr = $2.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
;
list:
	// `[42, 0, -13]`
	OPEN_BRACK list_elements CLOSE_BRACK
	{
		$$.expr = &ast.ExprList{
			Elements: $2.exprs,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
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
		$$.expr = $1.expr
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
;
map:
	// `{"hello" => "there", "world" => "big",}`
	OPEN_CURLY map_kvs CLOSE_CURLY
	{
		$$.expr = &ast.ExprMap{
			KVs: $2.mapKVs,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
;
map_kvs:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.mapKVs = []*ast.ExprMapKV{}
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
		$$.mapKV = &ast.ExprMapKV{
			Key: $1.expr,
			Val: $3.expr,
		}
	}
;
struct:
	// `struct{answer => 0, truth => false, hello => "world",}`
	STRUCT_IDENTIFIER OPEN_CURLY struct_fields CLOSE_CURLY
	{
		$$.expr = &ast.ExprStruct{
			Fields: $3.structFields,
		}
	}
;
struct_fields:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.structFields = []*ast.ExprStructField{}
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
		$$.structField = &ast.ExprStructField{
			Name:  $1.str,
			Value: $3.expr,
		}
	}
;
call:
	// fmt.printf(...)
	// iter.map(...)
	dotted_identifier OPEN_PAREN call_args CLOSE_PAREN
	{
		$$.expr = &ast.ExprCall{
			Name: $1.str,
			Args: $3.exprs,
			//Var: false, // default
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// calling a function that's stored in a variable (a lambda)
	// `$foo(4, "hey")` # call function value
|	dotted_var_identifier OPEN_PAREN call_args CLOSE_PAREN
	{
		$$.expr = &ast.ExprCall{
			Name: $1.str,
			Args: $3.exprs,
			// Instead of `Var: true`, we could have added a `$`
			// prefix to the Name, but I felt this was more elegant.
			Var: true, // lambda
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// calling an inline function
|	func OPEN_PAREN call_args CLOSE_PAREN
	{
		$$.expr = &ast.ExprCall{
			Name: "", // anonymous!
			Args: $3.exprs,
			Anon: $1.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr PLUS expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{      // operator first
					V: $2.str, // for PLUS this is a `+` character
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr MINUS expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr MULTIPLY expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr DIVIDE expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr EQ expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr NEQ expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr LT expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr GT expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr LTE expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr GTE expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr AND expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr OR expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $2.str,
				},
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	NOT expr
	{
		$$.expr = &ast.ExprCall{
			Name: operators.OperatorFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{ // operator first
					V: $1.str,
				},
				$2.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// lookup an index in a list or a key in a map
	// lookup($foo, $key)
	// `$foo[$key]` // no default specifier
|	expr OPEN_BRACK expr CLOSE_BRACK
	{
		$$.expr = &ast.ExprCall{
			Name: funcs.LookupFuncName,
			Args: []interfaces.Expr{
				$1.expr, // the list or map
				$3.expr, // the index or key is an expr
				//$6.expr, // the default
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// lookup an index in a list or a key in a map with a default
	// lookup_default($foo, $key, $default)
	// `$foo[$key] || "default"`
|	expr OPEN_BRACK expr CLOSE_BRACK DEFAULT expr
	{
		$$.expr = &ast.ExprCall{
			Name: funcs.LookupDefaultFuncName,
			Args: []interfaces.Expr{
				$1.expr, // the list or map
				$3.expr, // the index or key is an expr
				$6.expr, // the default
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// lookup a field in a struct
	// _struct_lookup($foo, "field")
	// $foo->field
|	expr ARROW IDENTIFIER
	{
		$$.expr = &ast.ExprCall{
			Name: funcs.StructLookupFuncName,
			Args: []interfaces.Expr{
				$1.expr, // the struct
				&ast.ExprStr{
					V: $3.str, // the field is always an str
				},
				//$5.expr, // the default
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// lookup a field in a struct with a default
	// _struct_lookup_optional($foo, "field", "default")
	// $foo->field || "default"
|	expr ARROW IDENTIFIER DEFAULT expr
	{
		$$.expr = &ast.ExprCall{
			Name: funcs.StructLookupOptionalFuncName,
			Args: []interfaces.Expr{
				$1.expr, // the struct
				&ast.ExprStr{
					V: $3.str, // the field is always an str
				},
				$5.expr, // the default
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
|	expr IN expr
	{
		$$.expr = &ast.ExprCall{
			Name: funcs.ContainsFuncName,
			Args: []interfaces.Expr{
				$1.expr,
				$3.expr,
			},
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
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
	dotted_var_identifier
	{
		$$.expr = &ast.ExprVar{
			Name: $1.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
;
func:
	// this is the lambda version, iow, a function as a value (expression)
	// `func() { <expr> }`
	// `func(<arg>) { <expr> }`
	// `func(<arg>, <arg>) { <expr> }`
	FUNC_IDENTIFIER OPEN_PAREN args CLOSE_PAREN OPEN_CURLY expr CLOSE_CURLY
	{
		$$.expr = &ast.ExprFunc{
			Args: $3.args,
			//Return: nil,
			Body: $6.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
	// `func(...) <type> { <expr> }`
|	FUNC_IDENTIFIER OPEN_PAREN args CLOSE_PAREN type OPEN_CURLY expr CLOSE_CURLY
	{
		$$.expr = &ast.ExprFunc{
			Args:   $3.args,
			Return: $5.typ, // return type is known
			Body:   $7.expr,
		}
		isFullyTyped := $5.typ != nil // true if set
		m := make(map[string]*types.Type)
		ord := []string{}
		for _, a := range $3.args {
			if a.Type == nil {
				// at least one is unknown, can't run SetType...
				isFullyTyped = false
				break
			}
			m[a.Name] = a.Type
			ord = append(ord, a.Name)
		}
		if isFullyTyped {
			typ := &types.Type{
				Kind: types.KindFunc,
				Map:  m,
				Ord:  ord,
				Out:  $5.typ,
			}
			if err := $$.expr.SetType(typ); err != nil {
				// this will ultimately cause a parser error to occur...
				yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
			}
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
;
args:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = []*interfaces.Arg{}
	}
|	args COMMA arg
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append($1.args, $3.arg)
	}
|	arg
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append([]*interfaces.Arg{}, $1.arg)
	}
;
arg:
	// `$x`
	var_identifier
	{
		$$.arg = &interfaces.Arg{
			Name: $1.str,
		}
	}
	// `$x <type>`
|	var_identifier type
	{
		$$.arg = &interfaces.Arg{
			Name: $1.str,
			Type: $2.typ,
		}
	}
;
bind:
	// `$s = "hey"`
	var_identifier EQUALS expr
	{
		$$.stmt = &ast.StmtBind{
			Ident: $1.str,
			Value: $3.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// `$x bool = true`
	// `$x int = if true { 42 } else { 13 }`
|	var_identifier type EQUALS expr
	{
		var expr interfaces.Expr = $4.expr
		// XXX: We still need to do this for now it seems...
		if err := expr.SetType($2.typ); err != nil {
			// this will ultimately cause a parser error to occur...
			yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
		}
		$$.stmt = &ast.StmtBind{
			Ident: $1.str,
			Value: expr,
			Type:  $2.typ, // sam says add the type here instead...
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
;
panic:
	// panic("some error")
	// generates:
	// if panic("some error") {
	//	_panic "_panic" {} # resource
	//}
	PANIC_IDENTIFIER OPEN_PAREN call_args CLOSE_PAREN
	{
		call := &ast.ExprCall{
			Name: $1.str, // the function name
			Args: $3.exprs,
			//Var: false, // default
		}
		name := &ast.ExprStr{
			V: $1.str, // any constant, non-empty name
		}
		res := &ast.StmtRes{
			Kind:     interfaces.PanicResKind,
			Name:     name,
			Contents: []ast.StmtResContents{},
		}
		$$.stmt = &ast.StmtIf{
			Condition:  call,
			ThenBranch: res,
			//ElseBranch: nil,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
;
collect:
	// `collect file "/tmp/hello" { ... }`
	// `collect file ["/tmp/hello", ...,] { ... }`
	// `collect file [struct{name => "/tmp/hello", host => "foo",}, ...,] { ... }`
	COLLECT_IDENTIFIER resource
	{
		// A "collect" stmt is exactly a regular "res" statement, except
		// it has the boolean "Collect" field set to true, and it also
		// has a special "resource body" entry which accepts the special
		// collected data from the function graph.
		$$.stmt = $2.stmt // it's us now
		kind := $2.stmt.(*ast.StmtRes).Kind
		res := $$.stmt.(*ast.StmtRes)
		res.Collect = true
		// We are secretly adding a special field to the res contents,
		// which receives all of the exported data so that we have it
		// arrive in our function graph in the standard way. We'd need
		// to have this data to be able to build the resources we want!
		call := &ast.ExprCall{
			// function name to lookup special values from that kind
			Name: funcs.CollectFuncName,
			Args: []interfaces.Expr{
				&ast.ExprStr{      // magic operator first
					V: kind,   // tell it what we're reading
				},
				// names to collect
				// XXX: Can we copy the same AST nodes to here?
				// XXX: Do I need to run .Copy() on them ?
				// str, []str, or []struct{name str; host str}
				res.Name, // expr (hopefully one of those types)
			},
		}
		collect := &ast.StmtResCollect{ // special field
			Kind:  kind, // might as well tell it directly
			Value: call,
		}
		res.Contents = append(res.Contents, collect)
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
;
/* TODO: do we want to include this?
// resource bind
rbind:
	var_identifier EQUALS resource
	{
		// XXX: this kind of bind is different than the others, because
		// it can only really be used for send->recv stuff, eg:
		// foo.SomeString -> bar.SomeOtherString
		$$.expr = &ast.StmtBind{
			Ident: $1.str,
			Value: $3.stmt,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.expr)
	}
;
*/
resource:
	// `file "/tmp/hello" { ... }` or `aws:ec2 "/tmp/hello" { ... }`
	colon_identifier expr OPEN_CURLY resource_body CLOSE_CURLY
	{
		$$.stmt = &ast.StmtRes{
			Kind:     $1.str,
			Name:     $2.expr,
			Contents: $4.resContents,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
;
resource_body:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = []ast.StmtResContents{}
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
|	resource_body resource_meta
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resMeta)
	}
|	resource_body conditional_resource_meta
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resMeta)
	}
|	resource_body resource_meta_struct
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resMeta)
	}
|	resource_body conditional_resource_meta_struct
	{
		posLast(yylex, yyDollar) // our pos
		$$.resContents = append($1.resContents, $2.resMeta)
	}
;
resource_field:
	IDENTIFIER ROCKET expr COMMA
	{
		$$.resField = &ast.StmtResField{
			Field: $1.str,
			Value: $3.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resField)
	}
;
conditional_resource_field:
	// content => $present ?: "hello",
	IDENTIFIER ROCKET expr ELVIS expr COMMA
	{
		$$.resField = &ast.StmtResField{
			Field:     $1.str,
			Value:     $5.expr,
			Condition: $3.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resField)
	}
;
resource_edge:
	// Before => Test["t1"],
	CAPITALIZED_IDENTIFIER ROCKET edge_half COMMA
	{
		$$.resEdge = &ast.StmtResEdge{
			Property: $1.str,
			EdgeHalf: $3.edgeHalf,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resEdge)
	}
;
conditional_resource_edge:
	// Before => $present ?: Test["t1"],
	CAPITALIZED_IDENTIFIER ROCKET expr ELVIS edge_half COMMA
	{
		$$.resEdge = &ast.StmtResEdge{
			Property:  $1.str,
			EdgeHalf:  $5.edgeHalf,
			Condition: $3.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resEdge)
	}
;
resource_meta:
	// Meta:noop => true,
	CAPITALIZED_IDENTIFIER COLON IDENTIFIER ROCKET expr COMMA
	{
		if strings.ToLower($1.str) != strings.ToLower(ast.MetaField) {
			// this will ultimately cause a parser error to occur...
			yylex.Error(fmt.Sprintf("%s: %s", ErrParseResFieldInvalid, $1.str))
		}
		$$.resMeta = &ast.StmtResMeta{
			Property: $3.str,
			MetaExpr: $5.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resMeta)
	}
;
conditional_resource_meta:
	// Meta:limit => $present ?: 4,
	CAPITALIZED_IDENTIFIER COLON IDENTIFIER ROCKET expr ELVIS expr COMMA
	{
		posLast(yylex, yyDollar) // our pos
		if strings.ToLower($1.str) != strings.ToLower(ast.MetaField) {
			// this will ultimately cause a parser error to occur...
			yylex.Error(fmt.Sprintf("%s: %s", ErrParseResFieldInvalid, $1.str))
		}
		$$.resMeta = &ast.StmtResMeta{
			Property:  $3.str,
			MetaExpr:  $7.expr,
			Condition: $5.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resMeta)
	}
;
resource_meta_struct:
	// Meta => struct{meta => true, retry => 3,},
	CAPITALIZED_IDENTIFIER ROCKET expr COMMA
	{
		if strings.ToLower($1.str) != strings.ToLower(ast.MetaField) {
			// this will ultimately cause a parser error to occur...
			yylex.Error(fmt.Sprintf("%s: %s", ErrParseResFieldInvalid, $1.str))
		}
		$$.resMeta = &ast.StmtResMeta{
			Property: $1.str,
			MetaExpr: $3.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resMeta)
	}
;
conditional_resource_meta_struct:
	// Meta => $present ?: struct{poll => 60, sema => ["foo:1", "bar:3",],},
	CAPITALIZED_IDENTIFIER ROCKET expr ELVIS expr COMMA
	{
		if strings.ToLower($1.str) != strings.ToLower(ast.MetaField) {
			// this will ultimately cause a parser error to occur...
			yylex.Error(fmt.Sprintf("%s: %s", ErrParseResFieldInvalid, $1.str))
		}
		$$.resMeta = &ast.StmtResMeta{
			Property:  $1.str,
			MetaExpr:  $5.expr,
			Condition: $3.expr,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.resMeta)
	}
;
edge:
	// TODO: we could technically prevent single edge_half pieces from being
	// parsed, but it's probably more work than is necessary...
	// Test["t1"] -> Test["t2"] -> Test["t3"] # chain or pair
	edge_half_list
	{
		$$.stmt = &ast.StmtEdge{
			EdgeHalfList: $1.edgeHalfList,
			//Notify: false, // unused here
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
	// Test["t1"].foo_send -> Test["t2"].blah_recv # send/recv
|	edge_half_sendrecv ARROW edge_half_sendrecv
	{
		$$.stmt = &ast.StmtEdge{
			EdgeHalfList: []*ast.StmtEdgeHalf{
				$1.edgeHalf,
				$3.edgeHalf,
			},
			//Notify: false, // unused here, it is implied (i think)
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.stmt)
	}
;
edge_half_list:
	edge_half
	{
		posLast(yylex, yyDollar) // our pos
		$$.edgeHalfList = []*ast.StmtEdgeHalf{$1.edgeHalf}
	}
|	edge_half_list ARROW edge_half
	{
		posLast(yylex, yyDollar) // our pos
		$$.edgeHalfList = append($1.edgeHalfList, $3.edgeHalf)
	}
;
edge_half:
	// eg: Test["t1"]
	capitalized_res_identifier OPEN_BRACK expr CLOSE_BRACK
	{
		$$.edgeHalf = &ast.StmtEdgeHalf{
			Kind: $1.str,
			Name: $3.expr,
			//SendRecv: "", // unused
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.edgeHalf)
	}
;
edge_half_sendrecv:
	// eg: Test["t1"].foo_send
	capitalized_res_identifier OPEN_BRACK expr CLOSE_BRACK DOT IDENTIFIER
	{
		$$.edgeHalf = &ast.StmtEdgeHalf{
			Kind: $1.str,
			Name: $3.expr,
			SendRecv: $6.str,
		}
		locate(yylex, $1, yyDollar[len(yyDollar)-1], $$.edgeHalf)
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
|	MAP_IDENTIFIER OPEN_CURLY type COLON type CLOSE_CURLY
	// map: map{str: int} or map{str: []int}
	{
		posLast(yylex, yyDollar) // our pos
		$$.typ = types.NewType(fmt.Sprintf("map{%s: %s}", $3.typ.String(), $5.typ.String()))
	}
|	STRUCT_IDENTIFIER OPEN_CURLY type_struct_fields CLOSE_CURLY
	// struct: struct{} or struct{a bool} or struct{a bool; bb int}
	{
		posLast(yylex, yyDollar) // our pos

		names := make(map[string]struct{})
		strs := []string{}
		for _, arg := range $3.args {
			s := fmt.Sprintf("%s %s", arg.Name, arg.Type.String())
			if _, exists := names[arg.Name]; exists {
				// duplicate field name used
				err := fmt.Errorf("duplicate struct field of `%s`", s)
				// this will ultimately cause a parser error to occur...
				yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
				break // we must skip, because code continues!
			}
			names[arg.Name] = struct{}{}
			strs = append(strs, s)
		}

		$$.typ = types.NewType(fmt.Sprintf("%s{%s}", $1.str, strings.Join(strs, "; ")))
	}
|	FUNC_IDENTIFIER OPEN_PAREN type_func_args CLOSE_PAREN type
	// XXX: should we allow named args in the type signature?
	// func: func() float or func(bool) str or func(a bool, bb int) float
	{
		posLast(yylex, yyDollar) // our pos

		m := make(map[string]*types.Type)
		ord := []string{}
		for i, a := range $3.args {
			if a.Type == nil {
				// at least one is unknown, can't run SetType...
				// this means there is a programming error here!
				err := fmt.Errorf("type is unspecified for arg #%d", i)
				// this will ultimately cause a parser error to occur...
				yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
				break // safety
			}
			name := a.Name
			if name == "" {
				name = util.NumToAlpha(i) // if unspecified...
			}
			if util.StrInList(name, ord) {
				// duplicate arg name used
				err := fmt.Errorf("duplicate arg name of `%s`", name)
				// this will ultimately cause a parser error to occur...
				yylex.Error(fmt.Sprintf("%s: %+v", ErrParseSetType, err))
				break // safety
			}
			m[name] = a.Type
			ord = append(ord, name)
		}

		$$.typ = &types.Type{
			Kind: types.KindFunc,
			Map:  m,
			Ord:  ord,
			Out:  $5.typ,
		}
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
		$$.args = []*interfaces.Arg{}
	}
|	type_struct_fields SEMICOLON type_struct_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append($1.args, $3.arg)
	}
|	type_struct_field
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append([]*interfaces.Arg{}, $1.arg)
	}
;
type_struct_field:
	IDENTIFIER type
	{
		posLast(yylex, yyDollar) // our pos
		$$.arg = &interfaces.Arg{ // reuse the Arg struct
			Name: $1.str,
			Type: $2.typ,
		}
	}
;
type_func_args:
	/* end of list */
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = []*interfaces.Arg{}
	}
|	type_func_args COMMA type_func_arg
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append($1.args, $3.arg)
	}
|	type_func_arg
	{
		posLast(yylex, yyDollar) // our pos
		$$.args = append([]*interfaces.Arg{}, $1.arg)
		//$$.args = []*interfaces.Arg{$1.arg} // TODO: is this equivalent?
	}
;
type_func_arg:
	// `<type>`
	type
	{
		$$.arg = &interfaces.Arg{
			Type: $1.typ,
		}
	}
	// `$x <type>`
	// XXX: should we allow specifying the arg name here?
|	var_identifier type
	{
		$$.arg = &interfaces.Arg{
			Name: $1.str,
			Type: $2.typ,
		}
	}
;
undotted_identifier:
	IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str
	}
	// a function could be named map()!
|	MAP_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str
	}
	// a function could be named collect.res()!
|	COLLECT_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str
	}
;
var_identifier:
	// eg: $ foo (dollar prefix + identifier)
	DOLLAR undotted_identifier
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $2.str // don't include the leading $
	}
;
colon_identifier:
	// eg: `foo`
	IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str
	}
	// eg: `foo:bar` (used in `docker:image` or `class base:inner:deeper`)
|	colon_identifier COLON IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str + $2.str + $3.str
	}
;
dotted_identifier:
	undotted_identifier
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str
	}
|	dotted_identifier DOT undotted_identifier
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str + interfaces.ModuleSep + $3.str
	}
;
// there are different ways the lexer/parser might choose to represent this...
dotted_var_identifier:
	// eg: $ foo.bar.baz (dollar prefix + dotted identifier)
	DOLLAR dotted_identifier
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $2.str // don't include the leading $
	}
;
capitalized_res_identifier:
	CAPITALIZED_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str
	}
|	capitalized_res_identifier COLON CAPITALIZED_IDENTIFIER
	{
		posLast(yylex, yyDollar) // our pos
		$$.str = $1.str + $2.str + $3.str
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

// locate should be called after creating AST nodes from lexer tokens to store
// the positions of the involved tokens in the AST node.
func locate(y yyLexer, first yySymType, last yySymType, node interface{}) {
	pos(y, last)
	// Only run Locate on nodes that look like they have not received
	// locations yet otherwise the parser will come back and overwrite them
	// with invalid ending positions.
	if pn, ok := node.(interfaces.PositionableNode); !ok {
		return
	} else if !pn.IsSet() {
		pn.Locate(first.row, first.col, last.row, last.col)
	}
}

// posLast runs pos on the last token of the current stmt/expr.
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
