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

// Package format contains the mcl formatter. It orchestrates the lexer/parser
// and the AST printer, which are injected as function pointers. The injection
// is unfortunately mandatory: this package is imported by the cli package, and
// the cli package is imported by the embedded provisioner, which lives below
// lang/core, which the lang/ast package imports. As a result, importing the
// parser or the AST printer here directly would be an import cycle. The lang
// gapi wires in the real implementations and hands them to the cli through the
// gapi registry.
//
// After formatting, this always verifies its own output before letting it
// escape: the result must re-parse successfully, it must contain the exact same
// comments, the re-parsed AST must be equivalent to the original one, and
// formatting must be idempotent. Any verification failure returns an error
// instead of silently writing out broken code.
package format

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/kylelemons/godebug/pretty"
)

// LexParserFunc is the signature of the function which parses raw mcl input
// into an AST and a list of comments. The parser package implements it with
// LexParseWithComments.
type LexParserFunc = func(io.Reader) (interfaces.Stmt, []*interfaces.Comment, error)

// ASTFormatterFunc is the signature of the function which formats a parsed AST
// and its comments. The astfmt package implements it with Format.
type ASTFormatterFunc = func(context.Context, interfaces.Stmt, []*interfaces.Comment) ([]byte, error)

// FormatterProvider is implemented by frontends which can supply a fully wired
// Formatter. The cli looks this up through the gapi registry.
type FormatterProvider interface {
	// Formatter returns a Formatter with the parser and printer wired in.
	Formatter() *Formatter
}

// Formatter formats mcl input.
type Formatter struct {
	// LexParser parses raw mcl input into an AST and a list of comments.
	LexParser LexParserFunc

	// ASTFormatter formats a parsed AST and its comments.
	ASTFormatter ASTFormatterFunc

	// Test specifies that we should only check and not perform any writes.
	Test bool

	// Verbose specifies that we should logf the output of our operations.
	Verbose bool

	Debug bool
	Logf  func(format string, v ...any)
}

// Init sets up the struct before first use.
func (obj *Formatter) Init() {
	if obj.Logf == nil {
		obj.Logf = func(string, ...any) {}
	}
}

// FormatPath runs the mcl formatter against a directory tree or single file.
// Directories must be specified with a trailing slash. Returns false if
// something changed or if something needs changing.
func (obj *Formatter) FormatPath(ctx context.Context, filename string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if filename == "" {
		return false, fmt.Errorf("empty filename")
	}
	if !strings.HasPrefix(filename, "/") {
		return false, fmt.Errorf("filename is not absolute")
	}

	if obj.Debug {
		obj.Logf("input: %s", filename)
	}

	if strings.HasSuffix(filename, "/") {
		return obj.FormatDir(ctx, filename)
	}

	return obj.FormatFile(ctx, filename)
}

// FormatDir runs the mcl formatter against a directory tree.
func (obj *Formatter) FormatDir(ctx context.Context, dir string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	info, err := os.Stat(dir)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("not a directory: %s", dir)
	}

	checkOK := true
	err = filepath.WalkDir(dir, func(filename string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil // skip
		}
		if !strings.HasSuffix(filename, interfaces.DotFileNameExtension) {
			return nil // skip
		}

		b, err := obj.FormatFile(ctx, filename)
		if err != nil {
			return errwrap.Wrapf(err, "error formatting: %s", filename)
		}
		if !b {
			checkOK = false
		}

		return nil
	})
	if err != nil {
		return false, err
	}
	return checkOK, nil
}

// FormatFile runs the mcl formatter against a single file. It returns true if
// the file was already formatted correctly. Unless Test is set, it also
// rewrites the file if it was not.
func (obj *Formatter) FormatFile(ctx context.Context, filename string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if filename == "" {
		return false, fmt.Errorf("empty filename")
	}
	if !strings.HasPrefix(filename, "/") {
		return false, fmt.Errorf("filename is not absolute")
	}

	if obj.Debug {
		obj.Logf("format: %s", filename)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}

	formatted, err := obj.FormatData(ctx, bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	if bytes.Equal(formatted, data) {
		return true, nil
	}

	if obj.Test {
		if obj.Verbose {
			obj.Logf("not formatted: %s", filename)
		}
		return false, nil
	}

	info, err := os.Stat(filename)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(filename, formatted, info.Mode()); err != nil { //nolint:gosec // G703: formatter intentionally rewrites the caller-selected absolute path
		return false, err
	}
	if obj.Verbose {
		obj.Logf("wrote: %s", filename)
	}
	return false, nil
}

// FormatData runs the mcl formatter against some input file contents. Before
// returning, it verifies that the output re-parses to an equivalent AST with
// identical comments, and that formatting is idempotent, so that a formatter
// bug can not destroy anyone's code.
func (obj *Formatter) FormatData(ctx context.Context, input io.Reader) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if input == nil {
		return nil, fmt.Errorf("nil formatter input")
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}

	// The parser needs a trailing newline to be able to finish the last
	// statement, so this is a formatting fix that we always apply first.
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	if obj.LexParser == nil {
		return nil, fmt.Errorf("missing lexer/parser")
	}
	if obj.ASTFormatter == nil {
		return nil, fmt.Errorf("missing AST formatter")
	}

	xast, comments, err := obj.LexParser(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	formatted, err := obj.ASTFormatter(ctx, xast, comments)
	if err != nil {
		return nil, err
	}

	// Verify the output before we let it escape. Errors below this point
	// are formatter bugs, and failing here protects the original code.
	newAST, newComments, err := obj.LexParser(bytes.NewReader(formatted))
	if err != nil {
		// programming error
		return nil, errwrap.Wrapf(err, "mcl fmt bug: output does not parse")
	}

	if a, b := len(newComments), len(comments); a != b {
		// programming error
		return nil, fmt.Errorf("mcl fmt bug: expected %d comments, output has %d", b, a)
	}
	for i, comment := range comments {
		if newComments[i].Value != comment.Value {
			// programming error
			return nil, fmt.Errorf("mcl fmt bug: comment #%d changed", i)
		}
	}

	// This config compares only the exported fields, which excludes all of
	// the position information, and that is exactly what we want here.
	prettyConfig := &pretty.Config{
		Diffable: true,
	}
	if diff := prettyConfig.Compare(xast, newAST); diff != "" {
		// programming error
		if obj.Debug {
			obj.Logf("formatter diff:\n%s", diff)
		}
		return nil, fmt.Errorf("mcl fmt bug: output AST differs")
	}

	again, err := obj.ASTFormatter(ctx, newAST, newComments)
	if err != nil {
		// programming error
		return nil, errwrap.Wrapf(err, "mcl fmt bug: could not reformat output")
	}
	if !bytes.Equal(again, formatted) {
		// programming error
		return nil, fmt.Errorf("mcl fmt bug: formatting is not idempotent")
	}

	return formatted, nil
}

// CheckFiles runs the mcl formatter against the contents of each named source
// file and checks that the formatted output matches the original source. The
// contents are looked up with the given source finder function. It returns
// false if at least one file is not formatted.
func (obj *Formatter) CheckFiles(ctx context.Context, paths []string, sourceFinder interfaces.SourceFinderFunc) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if sourceFinder == nil {
		return false, fmt.Errorf("nil source finder")
	}

	checkOK := true
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		if path == "" {
			return false, fmt.Errorf("empty check path")
		}

		source, err := sourceFinder(path)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not read: %s", path)
		}

		formatted, err := obj.FormatData(ctx, bytes.NewReader(source))
		if err != nil {
			return false, errwrap.Wrapf(err, "could not format: %s", path)
		}
		if bytes.Equal(formatted, source) {
			continue
		}

		checkOK = false
		if obj.Verbose {
			obj.Logf("not formatted: %s", path)
		}
	}

	return checkOK, nil
}
