// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

import (
	"fmt"
)

%%{
	machine interpolate;
	write data;
}%%

// Parse performs string interpolation on the input. It returns the list of
// tokens found. It looks for variables of the format ${foo}. The curly braces
// are required.
func Parse(data string) (out Stream, _ error) {
	var (
		// variables used by Ragel
		cs  = 0 // current state
		p   = 0 // current position in data
		pe  = len(data)
		eof = pe // eof == pe if this is the last data block

		// Index in data where the currently captured string started.
		idx int

		x string   // The string we use for holding a temporary value.
		l Literal  // The string literal being read, if any.
		v Variable // The variable being read, if any.

		// Current token. This is either the variable that we just read
		// or the string literal. We will append it to `out` and move
		// on.
		t Token
	)

	%%{
		# Record the current position as the start of a string. This is
		# usually used with the entry transition (>) to start capturing
		# the string when a state machine is entered.
		#
		# fpc is the current position in the string (basically the same
		# as the variable `p` but a special Ragel keyword) so after
		# executing `start`, data[idx:fpc+1] is the string from when
		# start was called to the current position (inclusive).
		action start { idx = fpc }

		# A variable always starts with an lowercase alphabetical char
		# and contains lowercase alphanumeric characters or numbers,
		# underscores, and non-consecutive dots. The last char must not
		# be an underscore or a dot.
		# XXX: check that we don't get consecutive underscores or dots!
		var_name = ( [a-z] ([a-z0-9_] | ('.' | '_') [a-z0-9_])* )
		>start
		@{
			v.Name = data[idx:fpc+1]
		};

		# var is a reference to a variable.
		var = '${' var_name '}' ;

		# Any special escape characters are matched here.
		escaped_lit = '\\' ( 'a' | 'b' | 'f' | 'n' | 'r' | 't' | 'v' | '\\' | '"' | '$' )
		@{
			switch s := data[fpc:fpc+1]; s {
			case "a":
				x = "\a"
			case "b":
				x = "\b"
			//case "e":
			//	x = "\e" // non-standard
			case "f":
				x = "\f"
			case "n":
				x = "\n"
			case "r":
				x = "\r"
			case "t":
				x = "\t"
			case "v":
				x = "\v"
			case "\\":
				x = "\\"
			case "\"":
				x = "\""
			case "$":
				x = "$"
			//case "0":
			//	x = "\x00"
			default:
				//x = s // in case we want to avoid erroring
				// this is a programming (parser) error I think
				return nil, fmt.Errorf("unhandled escape sequence token: %s", s)
			}
			l = Literal{Value: x}
		};

		# XXX: explicitly try and add this one?
		#escape_lit = '\\\\'
		#@{
		#	l = Literal{Value: "\\\\"}
		#};

		# Anything followed by a '$' that is not a '{' is used as-is
		# with the dollar.
		dollar_lit = '$' (any - '{')
		@{
			l = Literal{Value: data[fpc-1:fpc+1]}
		};

		# Literal strings that don't contain '$' or '\'.
		simple_lit = (any - '$' - '\\')+
		>start
		@{
			l = Literal{Value: data[idx:fpc + 1]}
		};

		lit = escaped_lit | dollar_lit | simple_lit;

		# Tokens are the two possible components in a string. Either a
		# literal or a variable reference.
		token = (var @{ t = v }) | (lit @{ t = l });

		main := (token %{ out = append(out, t) })**;

		write init;
		write exec;
	}%%

	if cs < %%{ write first_final; }%% {
		return nil, fmt.Errorf("cannot parse string: %s", data)
	}

	return out, nil
}
