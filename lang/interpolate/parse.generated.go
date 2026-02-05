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

package interpolate

import (
	"fmt"
)

//line interpolate/parse.generated.go:38
const interpolate_start int = 5
const interpolate_first_final int = 5
const interpolate_error int = 0

const interpolate_en_main int = 5

// Parse performs string interpolation on the input. It returns the list of
// tokens found. It looks for variables of the format ${foo}. The curly braces
// are required.
// XXX: Pull dollar sign and curly chars from VarPrefix and other constants.
//
//line interpolate/parse.rl:39
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

//line interpolate/parse.generated.go:73
	{
		cs = interpolate_start
	}

//line interpolate/parse.generated.go:77
	{
		if p == pe {
			goto _test_eof
		}
		switch cs {
		case 5:
			goto st_case_5
		case 6:
			goto st_case_6
		case 7:
			goto st_case_7
		case 1:
			goto st_case_1
		case 8:
			goto st_case_8
		case 2:
			goto st_case_2
		case 0:
			goto st_case_0
		case 3:
			goto st_case_3
		case 4:
			goto st_case_4
		}
		goto st_out
	st_case_5:
		switch data[p] {
		case 36:
			goto tr7
		case 92:
			goto st1
		}
		goto tr6
	tr6:
//line interpolate/parse.rl:75
		idx = p
//line interpolate/parse.rl:136

		l = Literal{Value: data[idx : p+1]}

//line interpolate/parse.rl:144
		t = l
		goto st6
	tr9:
//line interpolate/parse.rl:136

		l = Literal{Value: data[idx : p+1]}

//line interpolate/parse.rl:144
		t = l
		goto st6
	tr12:
//line interpolate/parse.rl:146
		out = append(out, t)
//line interpolate/parse.rl:75
		idx = p
//line interpolate/parse.rl:136

		l = Literal{Value: data[idx : p+1]}

//line interpolate/parse.rl:144
		t = l
		goto st6
	st6:
		if p++; p == pe {
			goto _test_eof6
		}
	st_case_6:
//line interpolate/parse.generated.go:146
		switch data[p] {
		case 36:
			goto tr10
		case 92:
			goto tr11
		}
		goto tr9
	tr7:
//line interpolate/parse.rl:129

		l = Literal{Value: data[p : p+1]}

//line interpolate/parse.rl:144
		t = l
		goto st7
	tr10:
//line interpolate/parse.rl:146
		out = append(out, t)
//line interpolate/parse.rl:129

		l = Literal{Value: data[p : p+1]}

//line interpolate/parse.rl:144
		t = l
		goto st7
	st7:
		if p++; p == pe {
			goto _test_eof7
		}
	st_case_7:
//line interpolate/parse.generated.go:177
		switch data[p] {
		case 36:
			goto tr10
		case 92:
			goto tr11
		case 123:
			goto st2
		}
		goto tr12
	tr11:
//line interpolate/parse.rl:146
		out = append(out, t)
		goto st1
	st1:
		if p++; p == pe {
			goto _test_eof1
		}
	st_case_1:
//line interpolate/parse.generated.go:196
		goto tr0
	tr0:
//line interpolate/parse.rl:93

		switch s := data[p : p+1]; s {
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
			return nil, fmt.Errorf("unknown escape sequence: \\%s", s)
		}
		l = Literal{Value: x}

//line interpolate/parse.rl:144
		t = l
		goto st8
	tr5:
//line interpolate/parse.rl:144
		t = v
		goto st8
	st8:
		if p++; p == pe {
			goto _test_eof8
		}
	st_case_8:
//line interpolate/parse.generated.go:244
		switch data[p] {
		case 36:
			goto tr10
		case 92:
			goto tr11
		}
		goto tr12
	st2:
		if p++; p == pe {
			goto _test_eof2
		}
	st_case_2:
		if 97 <= data[p] && data[p] <= 122 {
			goto tr1
		}
		goto st0
	st_case_0:
	st0:
		cs = 0
		goto _out
	tr1:
//line interpolate/parse.rl:75
		idx = p
//line interpolate/parse.rl:84

		v.Name = data[idx : p+1]

		goto st3
	tr4:
//line interpolate/parse.rl:84

		v.Name = data[idx : p+1]

		goto st3
	st3:
		if p++; p == pe {
			goto _test_eof3
		}
	st_case_3:
//line interpolate/parse.generated.go:284
		switch data[p] {
		case 46:
			goto st4
		case 95:
			goto tr4
		case 125:
			goto tr5
		}
		switch {
		case data[p] > 57:
			if 97 <= data[p] && data[p] <= 122 {
				goto tr4
			}
		case data[p] >= 48:
			goto tr4
		}
		goto st0
	st4:
		if p++; p == pe {
			goto _test_eof4
		}
	st_case_4:
		if data[p] == 95 {
			goto tr4
		}
		switch {
		case data[p] > 57:
			if 97 <= data[p] && data[p] <= 122 {
				goto tr4
			}
		case data[p] >= 48:
			goto tr4
		}
		goto st0
	st_out:
	_test_eof6:
		cs = 6
		goto _test_eof
	_test_eof7:
		cs = 7
		goto _test_eof
	_test_eof1:
		cs = 1
		goto _test_eof
	_test_eof8:
		cs = 8
		goto _test_eof
	_test_eof2:
		cs = 2
		goto _test_eof
	_test_eof3:
		cs = 3
		goto _test_eof
	_test_eof4:
		cs = 4
		goto _test_eof

	_test_eof:
		{
		}
		if p == eof {
			switch cs {
			case 6, 7, 8:
//line interpolate/parse.rl:146
				out = append(out, t)
//line interpolate/parse.generated.go:334
			}
		}

	_out:
		{
		}
	}

//line interpolate/parse.rl:150

	if cs < 5 {
		return nil, fmt.Errorf("cannot parse string: %s", data)
	}

	return out, nil
}
