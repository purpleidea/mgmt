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

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"regexp"
	"strings"
	"unicode"
)

const (
	// Debug specifies if we want to be more verbose.
	Debug = false

	// CommentPrefix is the prefix we look for at the beginning of comments.
	CommentPrefix = "// "

	// CommentMultilinePrefix is what a multiline comment starts with.
	CommentMultilinePrefix = "/*"

	// StandardWidth is 80 chars. If something sneaks past, this is okay,
	// but there is no excuse if docstrings don't wrap and reflow to this.
	StandardWidth = 80

	// maxLength is the effective maximum length of each comment line.
	maxLength = StandardWidth - len(CommentPrefix)
)

var (
	// commentPrefixTrimmed is a trimmed copy of the CommentPrefix constant.
	commentPrefixTrimmed = strings.TrimRightFunc(CommentPrefix, unicode.IsSpace)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: ./%s <file>\n", os.Args[0])
		os.Exit(2)
		return
	}

	filename := os.Args[1]
	if filename == "" {
		fmt.Fprintf(os.Stderr, "filename is empty\n")
		os.Exit(2)
		return
	}

	if err := Check(filename); err != nil {
		fmt.Fprintf(os.Stderr, "failed with: %+v\n", err)
		os.Exit(1)
		return
	}
}

// Check returns the comment checker on an individual filename.
func Check(filename string) error {
	if Debug {
		log.Printf("filename: %s", filename)
	}

	fset := token.NewFileSet()

	// f is a: https://golang.org/pkg/go/ast/#File
	f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	// XXX: f.Doc // CommentGroup
	// XXX: f.Comments []*CommentGroup // list of all comments in the source file
	// XXX: f.Decls []Decl // top-level declarations; or nil

	for _, node := range f.Decls {
		var doc *ast.CommentGroup

		// TODO: move to type switch ?
		if x, ok := node.(*ast.FuncDecl); ok {
			doc = x.Doc
		} else if x, ok := node.(*ast.GenDecl); ok {
			switch x.Tok {
			case token.IMPORT: // i don't think this is needed, aiui
			case token.VAR:
				// TODO: recurse into https://golang.org/pkg/go/ast/#ValueSpec
				doc = x.Doc
			case token.CONST:
				// TODO: recurse into https://golang.org/pkg/go/ast/#ValueSpec
				doc = x.Doc
			case token.TYPE: // struct, usually
				// TODO: recurse into x.Tok (eg: TypeSpec.Doc and so on)
				doc = x.Doc
			default:
			}
		}

		if doc == nil { // we got nothing
			continue
		}

		pos := doc.Pos()
		ff := fset.File(pos)
		items := strings.Split(ff.Name(), "/")
		if len(items) == 0 {
			return fmt.Errorf("file name is empty")
		}
		name := items[len(items)-1]
		ident := fmt.Sprintf("%s:%d", name, ff.Line(pos))

		block := []string{}
		for _, comment := range doc.List {
			if comment == nil {
				continue
			}
			s := comment.Text

			// TODO: how do we deal with multiline comments?
			if strings.HasPrefix(s, CommentMultilinePrefix) {
				break // skip
			}

			if s != commentPrefixTrimmed && !strings.HasPrefix(s, CommentPrefix) {
				return fmt.Errorf("location (%s) missing comment prefix", ident)
			}
			if s == commentPrefixTrimmed { // blank lines
				s = ""
			}

			s = strings.TrimPrefix(s, CommentPrefix)

			block = append(block, s)
		}

		if err := IsWrappedProperly(block, maxLength); err != nil {
			m := strings.Join(block, "\n")
			msg := filename + " " + strings.Repeat(".", maxLength-len(filename+" "+"V")) + fmt.Sprintf("V\n%+v\n", m)
			fmt.Fprintf(os.Stderr, msg)
			return fmt.Errorf("block (%s) failed: %+v", ident, err) // TODO: errwrap ?
		}
	}

	return nil
}

// IsWrappedProperly returns whether a block of lines are correctly wrapped.
// This also checks for a few related formatting situations. The list of lines
// should not have trailing newline characters present. While you most surely
// will want to ensure a maximum length of 80 characters, you'll want to
// subtract the comment prefix length from this value so that the final result
// is correctly wrapped.
func IsWrappedProperly(lines []string, length int) error {
	blank := false
	previous := length // default to full
	for i, line := range lines {
		lineno := i + 1 // human indexing

		// Allow a maximum of one blank line in a row.
		if line == "" {
			if blank {
				return fmt.Errorf("line %d was a sequential blank line", lineno)
			}
			blank = true
			previous = length // reset
			continue
		}
		blank = false

		if line != strings.TrimSpace(line) {
			return fmt.Errorf("line %d wasn't trimmed properly", lineno)
		}

		if strings.Contains(line, "  ") { // double spaces
			return fmt.Errorf("line %d contained multiple spaces", lineno)
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			//continue // should not happen with above check
			return fmt.Errorf("line %d had an unexpected empty list of fields", lineno)
		}

		lastIndex := len(fields) - 1
		lastChunk := fields[lastIndex]
		beginning := strings.Join(fields[0:lastIndex], " ")
		// !strings.Contains(lastChunk, " ") // redundant

		// Either of these conditions is a reason we can skip this test.
		skip1 := IsSpecialLine(line)
		skip2 := (len(beginning) <= length && IsSpecialLine(lastChunk))

		if len(line) > length && (!skip1) && (!skip2) {
			return fmt.Errorf("line %d is too long", lineno)
		}

		// If we have a new start word, then we don't need to reflow it
		// back to the previous line, and if not, then we check the fit.
		if !IsNewStart(fields[0]) && previous+len(" ")+len(fields[0]) <= length {
			return fmt.Errorf("line %d is not reflowed properly", lineno)
		}

		previous = len(line) // prepare for next iteration
	}

	return nil
}

// IsNewStart returns true if the input word is one which is a valid start to a
// new line. This means that it doesn't need to get reflowed into the previous
// line. You should pass in the word without any surrounding whitespace.
func IsNewStart(word string) bool {
	if word == "TODO:" {
		return true
	}
	if word == "FIXME:" {
		return true
	}
	if word == "XXX:" {
		return true
	}

	if word == "NOTE:" {
		return true
	}
	if word == "Eg:" || word == "Example:" { // might as well
		return true
	}

	if word == "https://" || word == "http://" { // for the occasional docs
		return true
	}

	if word == "*" { // bullets
		return true
	}
	if IsNumberBullet(word) {
		return true
	}

	return false
}

// IsSpecialLine allows lines that contain an entire special sentence to be
// allowed without breaking the reflow rules.
func IsSpecialLine(line string) bool {
	fields := strings.Fields(line)

	// If it's a URL and it doesn't contain any words after the end of it...
	if strings.HasPrefix(line, "https://") && len(fields) == 1 {
		return true
	}
	if strings.HasPrefix(line, "http://") && len(fields) == 1 {
		return true
	}

	return false
}

// IsNumberBullet returns true if the word starts with a number bullet like 42).
func IsNumberBullet(word string) bool {
	matched, err := regexp.MatchString(`[0-9]+\)*`, word)
	if err != nil {
		return false
	}
	return matched
}
