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

package interfaces

const (
	// ModuleSep is the character used for the module scope separation. For
	// example when using `fmt.printf` or `math.sin` this is the char used.
	// It is also used for variable scope separation such as `$foo.bar.baz`.
	ModuleSep = "."

	// ClassSep is the character used for the class embedding separation.
	// For example when defining `class base:inner` this is the char used.
	ClassSep = ":"

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
