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

package interpolate

// Stream is the list of tokens that are produced after interpolating a string.
// It is created by using the generated Parse function.
// TODO: In theory a more advanced parser could produce an AST here instead.
type Stream []Token

// Token is the interface that every token must implement.
type Token interface {
	token()
}

// Literal is a string literal that we have found after interpolation parsing.
type Literal struct {
	Value string
}

// token ties the Literal to the Token interface.
func (Literal) token() {}

// Variable is a variable name that we have found after interpolation parsing.
type Variable struct {
	Name string
}

// token ties the Variable to the Token interface.
func (Variable) token() {}

// TODO: do we want to allow inline-function calls in a string?
