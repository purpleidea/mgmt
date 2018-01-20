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

package lang // TODO: move this into a sub package of lang/$name?

import (
	"fmt"
	"io"

	"github.com/purpleidea/mgmt/lang/interfaces"
)

// These constants represent the different possible lexer/parser errors.
const (
	ErrLexerUnrecognized      = interfaces.Error("unrecognized")
	ErrLexerStringBadEscaping = interfaces.Error("string: bad escaping")
	ErrLexerIntegerOverflow   = interfaces.Error("integer: overflow")
	ErrLexerFloatOverflow     = interfaces.Error("float: overflow")
	ErrParseError             = interfaces.Error("parser")
	ErrParseAdditionalEquals  = interfaces.Error(errstrParseAdditionalEquals)
	ErrParseExpectingComma    = interfaces.Error(errstrParseExpectingComma)
)

// LexParseErr is a permanent failure error to notify about borkage.
type LexParseErr struct {
	Err interfaces.Error
	Str string
	Row int // this is zero-indexed (the first line is 0)
	Col int // this is zero-indexed (the first char is 0)
}

// Error displays this error with all the relevant state information.
func (e *LexParseErr) Error() string {
	return fmt.Sprintf("%s: `%s` @%d:%d", e.Err, e.Str, e.Row+1, e.Col+1)
}

// lexParseAST is a struct which we pass into the lexer/parser so that we have a
// location to store the AST to avoid having to use a global variable.
type lexParseAST struct {
	ast interfaces.Stmt

	row int
	col int

	lexerErr error // from lexer
	parseErr error // from Error(e string)
}

// LexParse runs the lexer/parser machinery and returns the AST.
func LexParse(input io.Reader) (interfaces.Stmt, error) {
	lp := &lexParseAST{}
	// parseResult is a seemingly unused field in the Lexer struct for us...
	lexer := NewLexerWithInit(input, func(y *Lexer) { y.parseResult = lp })
	yyParse(lexer) // writes the result to lp.ast
	var err error
	if e := lp.parseErr; e != nil {
		err = e
	}
	if e := lp.lexerErr; e != nil {
		err = e
	}
	if err != nil {
		return nil, err
	}
	return lp.ast, nil
}
