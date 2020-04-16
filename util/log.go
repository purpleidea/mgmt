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

package util

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
