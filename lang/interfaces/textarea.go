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

import (
	"fmt"
	"os"
	"strings"

	"github.com/purpleidea/mgmt/util"
)

// Textarea stores the coordinates of a statement or expression in the form of a
// starting line/column and ending line/column.
type Textarea struct {
	// debug represents if we're running in debug mode or not.
	debug bool

	// logf is a logger which should be used.
	logf func(format string, v ...interface{})

	// sf is the SourceFinder function implementation that maps a filename
	// to the source.
	sf SourceFinderFunc

	// path is the full path/filename where this text area exists.
	path string

	// This data is zero-based. (Eg: first line of file is 0)
	startLine   int // first
	startColumn int // left
	endLine     int // last
	endColumn   int // right

	isSet bool

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// Setup is used during AST initialization in order to store in each AST node
// the name of the source file from which it was generated.
func (obj *Textarea) Setup(data *Data) {
	obj.debug = data.Debug
	obj.logf = data.Logf
	obj.sf = data.SourceFinder
	obj.path = data.AbsFilename()
}

// IsSet returns if the position was already set with Locate already.
func (obj *Textarea) IsSet() bool {
	return obj.isSet
}

// Locate is used by the parser to store the token positions in AST nodes. The
// path will be filled during AST node initialization usually, because the
// parser does not know the name of the file it is processing.
func (obj *Textarea) Locate(line int, col int, endline int, endcol int) {
	obj.startLine = line
	obj.startColumn = col
	obj.endLine = endline
	obj.endColumn = endcol
	obj.isSet = true
}

// Pos returns the starting line/column of an AST node.
func (obj *Textarea) Pos() (int, int) {
	return obj.startLine, obj.startColumn
}

// End returns the end line/column of an AST node.
func (obj *Textarea) End() (int, int) {
	return obj.endLine, obj.endColumn
}

// Path returns the name of the source file that holds the code for an AST node.
func (obj *Textarea) Path() string {
	return obj.path
}

// Filename returns the printable filename that we'd like to display. It tries
// to return a relative version if possible.
func (obj *Textarea) Filename() string {
	if obj.path == "" {
		return "<unknown>" // TODO: should this be <stdin> ?
	}

	wd, _ := os.Getwd() // ignore error since "" would just pass through
	wd += "/"           // it's a dir
	if s, err := util.RemoveBasePath(obj.path, wd); err == nil {
		return s
	}

	return obj.path
}

// Byline gives a succinct representation of the Textarea, but is useful only in
// debugging. In order to generate pretty error messages, see HighlightText.
func (obj *Textarea) Byline() string {
	// We convert to 1-based for user display.
	return fmt.Sprintf("%s @ %d:%d-%d:%d", obj.Filename(), obj.startLine+1, obj.startColumn+1, obj.endLine+1, obj.endColumn+1)
}

// HighlightText generates a generic description that just visually indicates
// part of the line described by a Textarea. If the coordinates that are passed
// span multiple lines, don't show those lines, but just a description of the
// area. If it can't generate a valid snippet, then it returns the empty string.
func (obj *Textarea) HighlightText() string {
	if obj.sf == nil {
		// XXX: when all functions are ported over to use ast.Textarea,
		// then uncomment this return and add in the panic below.
		return "" // XXX: temporary
		// programming error
		//panic("nil SourceFinderFunc")
	}
	b, err := obj.sf(obj.path) // source finder!
	if err != nil {
		return ""
	}
	contents := string(b)

	result := &strings.Builder{}

	result.WriteString(obj.Byline())

	lines := strings.Split(contents, "\n")
	if len(lines) < obj.endLine-1 {
		// XXX: out of bounds?
		return ""
	}

	result.WriteString("\n\n")

	if obj.startLine == obj.endLine {
		line := lines[obj.startLine] + "\n"
		text := strings.TrimLeft(line, " \t")
		indent := strings.TrimSuffix(line, text)
		offset := len(indent)

		result.WriteString(line)
		result.WriteString(indent)
		result.WriteString(strings.Repeat(" ", obj.startColumn-offset))
		// TODO: add on the width of the second element as well
		result.WriteString(strings.Repeat("^", obj.endColumn-obj.startColumn))
		result.WriteString("\n")

		return result.String()
	}

	// XXX: the below code might be buggy, write some tests and check it...
	line := lines[obj.startLine] + "\n"
	text := strings.TrimLeft(line, " \t")
	indent := strings.TrimSuffix(line, text)
	offset := len(indent)

	result.WriteString(line)
	result.WriteString(indent)
	result.WriteString(strings.Repeat(" ", obj.startColumn-offset))
	result.WriteString("^ from here ...\n")

	line = lines[obj.endLine] + "\n"
	text = strings.TrimLeft(line, " \t")
	indent = strings.TrimSuffix(line, text)
	offset = len(indent)

	result.WriteString(line)
	result.WriteString(indent)
	result.WriteString(strings.Repeat(" ", obj.endColumn-offset-1)) // -1 because it's the space before the caret
	result.WriteString("^ ... to here\n")

	return result.String()
}

// HighlightHelper gives the user better file/line number feedback.
func HighlightHelper(node Node, logf func(format string, v ...interface{}), err error) error {
	displayer, ok := node.(TextDisplayer)
	if !ok {
		return err
	}

	if highlight := displayer.HighlightText(); highlight != "" {
		logf("%s: %s", err.Error(), highlight)
	}
	return fmt.Errorf("%s: %s", err.Error(), displayer.Byline())
}
