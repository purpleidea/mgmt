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

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// LogWriter is a simple interface that wraps our logf interface.
// TODO: Logf should end in (n int, err error) like fmt.Printf does!
type LogWriter struct {
	Prefix string
	Logf   func(format string, v ...interface{})
}

// Write satisfies the io.Writer interface.
func (obj *LogWriter) Write(p []byte) (n int, err error) {
	// TODO: logf should pass through (n int, err error)
	obj.Logf(obj.Prefix + string(p))
	return len(p), nil // TODO: hack for now
}

// EscapeLog returns a string with non-graphic characters escaped so that it can
// be included in a log message without interpreting attacker-controlled data.
func EscapeLog(s string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			b.WriteString(`\x`)
			b.WriteByte(hex[s[0]>>4])
			b.WriteByte(hex[s[0]&0x0f])
			s = s[1:]
			continue
		}
		if strconv.IsGraphic(r) {
			b.WriteString(s[:size])
		} else {
			quoted := strconv.QuoteRune(r)
			b.WriteString(quoted[1 : len(quoted)-1])
		}
		s = s[size:]
	}
	return b.String()
}

// EscapeLogError preserves an error chain while escaping non-graphic characters
// in its displayed message.
func EscapeLogError(err error) error {
	if err == nil {
		return nil
	}
	return &escapedLogError{err: err}
}

// escapedLogError holds an error to run EscapeLog on.
type escapedLogError struct {
	err error
}

// Error returns the escaped error message.
func (obj *escapedLogError) Error() string {
	return EscapeLog(obj.err.Error())
}

// Unwrap returns the original error.
func (obj *escapedLogError) Unwrap() error {
	return obj.err
}
