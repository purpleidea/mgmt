// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

package interfaces

const (
	// ModuleSep is the character used for the module scope separation. For
	// example when using `fmt.printf` or `math.sin` this is the char used.
	// It is also used for variable scope separation such as `$foo.bar.baz`.
	ModuleSep = "."

	// VarPrefix is the prefix character that precedes the variables
	// identifier. For example, `$foo` or for a lambda, `$fn(42)`. It is
	// also used with `ModuleSep` for scoped variables like `$foo.bar.baz`.
	VarPrefix = "$"

	// BareSymbol is the character used primarily for imports to specify
	// that we want to import the entire contents and flatten them into our
	// current scope. It should probably be removed entirely to force
	// explicit imports.
	BareSymbol = "*"

	// PanicResKind is the kind string used for the panic resource.
	PanicResKind = "_panic"
)
