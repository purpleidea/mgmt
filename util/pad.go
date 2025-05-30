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

package util

// LeftPad adds multiples of the pad string to the left of the input string
// until it reaches a minimum length. If the padding string is not an integer
// multiple of the missing length to pad, then this will overshoot. It is better
// to overshoot than to undershoot because if you need a string of a precise
// length, then it's easier to truncate the result, rather than having to pad
// even more. Most scenarios pad with a single char meaning this is not even an
// issue.
func LeftPad(s string, pad string, length int) string {
	out := s
	for len(out) < length {
		out = pad + out
	}
	return out
}

// RightPad adds multiples of the pad string to the right of the input string
// until it reaches a minimum length. If the padding string is not an integer
// multiple of the missing length to pad, then this will overshoot. It is better
// to overshoot than to undershoot because if you need a string of a precise
// length, then it's easier to truncate the result, rather than having to pad
// even more. Most scenarios pad with a single char meaning this is not even an
// issue.
func RightPad(s string, pad string, length int) string {
	out := s
	for len(out) < length {
		out += pad
	}
	return out
}
