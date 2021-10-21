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

package parser

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// These constants represent the different possible lexer/parser errors.
const (
	ErrLexerUnrecognized      = interfaces.Error("unrecognized")
	ErrLexerUnrecognizedCR    = interfaces.Error("unrecognized carriage return")
	ErrLexerStringBadEscaping = interfaces.Error("string: bad escaping")
	ErrLexerIntegerOverflow   = interfaces.Error("integer: overflow")
	ErrLexerFloatOverflow     = interfaces.Error("float: overflow")
	ErrParseError             = interfaces.Error("parser")
	ErrParseSetType           = interfaces.Error("can't set return type in parser")
	ErrParseResFieldInvalid   = interfaces.Error("can't use unknown resource field")
	ErrParseAdditionalEquals  = interfaces.Error(errstrParseAdditionalEquals)
	ErrParseExpectingComma    = interfaces.Error(errstrParseExpectingComma)
)

// LexParseErr is a permanent failure error to notify about borkage.
type LexParseErr struct {
	Err interfaces.Error
	Str string
	Row int // this is zero-indexed (the first line is 0)
	Col int // this is zero-indexed (the first char is 0)

	// Filename is the file that this error occurred in. If this is unknown,
	// then it will be empty. This is not set when run by the basic LexParse
	// function.
	Filename string
}

// Error displays this error with all the relevant state information.
func (e *LexParseErr) Error() string {
	return fmt.Sprintf("%s: `%s` @%d:%d", e.Err, e.Str, e.Row+1, e.Col+1)
}

// lexParseAST is a struct which we pass into the lexer/parser so that we have a
// location to store the AST to avoid having to use a global variable.
type lexParseAST struct {
	ast interfaces.Stmt

	row int
	col int

	lexerErr error // from lexer
	parseErr error // from Error(e string)
}

// LexParse runs the lexer/parser machinery and returns the AST.
func LexParse(input io.Reader) (interfaces.Stmt, error) {
	lp := &lexParseAST{}
	// parseResult is a seemingly unused field in the Lexer struct for us...
	lexer := NewLexerWithInit(input, func(y *Lexer) { y.parseResult = lp })
	yyParse(lexer) // writes the result to lp.ast
	var err error
	if e := lp.parseErr; e != nil {
		err = e
	}
	if e := lp.lexerErr; e != nil {
		err = e
	}
	if err != nil {
		return nil, err
	}
	return lp.ast, nil
}

// LexParseWithOffsets takes an io.Reader input and a list of corresponding
// offsets and runs LexParse on them. The input to this function is most
// commonly the output from DirectoryReader which returns a single io.Reader and
// the offsets map. It usually produces the combined io.Reader from an
// io.MultiReader grouper. If the offsets map is nil or empty, then it simply
// redirects directly to LexParse. This differs because when it errors it will
// also report the corresponding file the error occurred in based on some offset
// math. The offsets are in units of file size (bytes) and not length (lines).
// TODO: Due to an implementation difficulty, offsets are currently in length!
// NOTE: This was used for an older deprecated form of lex/parse file combining.
func LexParseWithOffsets(input io.Reader, offsets map[uint64]string) (interfaces.Stmt, error) {
	if offsets == nil || len(offsets) == 0 {
		return LexParse(input) // special case, no named offsets...
	}

	stmt, err := LexParse(input)
	if err == nil { // handle the success case first because it ends faster
		return stmt, nil
	}
	e, ok := err.(*LexParseErr)
	if !ok {
		return nil, err // unexpected error format
	}

	// rebuild the error so it contains the right filename index, etc...

	uints := []uint64{}
	for i := range offsets {
		uints = append(uints, i)
	}
	sort.Sort(util.UInt64Slice(uints))
	if i := uints[0]; i != 0 { // first offset is supposed to be zero
		return nil, fmt.Errorf("unexpected first offset of %d", i)
	}

	// TODO: switch this to an offset in bytes instead of lines
	// TODO: we'll also need a way to convert that into the new row number!
	row := uint64(e.Row)
	var i uint64           // initial condition
	filename := offsets[0] // (assumption)
	for _, i = range uints {
		if row <= i {
			break
		}

		// if we fall off the end of the loop, the last file is correct
		filename = offsets[i]
	}

	return nil, &LexParseErr{
		Err:      e.Err,        // same
		Str:      e.Str,        // same
		Row:      int(i - row), // computed offset
		Col:      e.Col,        // same
		Filename: filename,     // actual filename
	}
}

// DirectoryReader takes a filesystem and an absolute directory path, and it
// returns a combined reader into that directory, and an offset map of the file
// contents. This is used to build a reader from a directory containing language
// source files, and as a result, this will skip over files that don't have the
// correct extension. The offsets are in units of file size (bytes) and not
// length (lines).
// TODO: Due to an implementation difficulty, offsets are currently in length!
// NOTE: This was used for an older deprecated form of lex/parse file combining.
func DirectoryReader(fs engine.Fs, dir string) (io.Reader, map[uint64]string, error) {
	fis, err := fs.ReadDir(dir) // ([]os.FileInfo, error)
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "can't stat directory contents `%s`", dir)
	}

	var offset uint64
	offsets := make(map[uint64]string) // cumulative offset to abs. filename
	readers := []io.Reader{}

	for _, fi := range fis {
		if fi.IsDir() {
			continue // skip directories
		}
		name := path.Join(dir, fi.Name()) // relative path made absolute
		if !strings.HasSuffix(name, interfaces.DotFileNameExtension) {
			continue
		}

		f, err := fs.Open(name) // opens read-only
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "can't open file `%s`", name)
		}
		defer f.Close()
		//stat, err := f.Stat() // (os.FileInfo, error)
		//if err != nil {
		//	return nil, nil, errwrap.Wrapf(err, "can't stat file `%s`", name)
		//}

		offsets[offset] = name // save cumulative offset (starts at 0)
		//offset += uint64(stat.Size()) // the earlier stat causes file download

		// TODO: store the offset in size instead of length! we're using
		// length at the moment since it is not clear how easy it is for
		// the lexer/parser to return the byte offset as well as line no
		// NOTE: in addition, this scanning is not the fastest for perf!
		scanner := bufio.NewScanner(f)
		lines := 0
		for scanner.Scan() { // each line
			lines++
		}
		if err := scanner.Err(); err != nil {
			return nil, nil, errwrap.Wrapf(err, "can't scan file `%s`", name)
		}
		offset += uint64(lines)
		if start, err := f.Seek(0, io.SeekStart); err != nil { // reset
			return nil, nil, errwrap.Wrapf(err, "can't reset file `%s`", name)
		} else if start != 0 { // we should be at the start (0)
			return nil, nil, fmt.Errorf("reset of file `%s` was %d", name, start)
		}

		readers = append(readers, f)
	}
	if len(offsets) == 0 {
		// TODO: this condition should be validated during the deploy...
		return nil, nil, fmt.Errorf("no files in main directory")
	}

	if len(offsets) == 1 { // no need for a multi reader
		return readers[0], offsets, nil
	}

	return io.MultiReader(readers...), offsets, nil
}
