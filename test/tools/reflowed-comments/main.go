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

// XXX: consider using the https://pkg.go.dev/go/doc parser instead and also
// checking the other fields.

package main

import (
	"bytes"
	"flag"
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

	// CommentPrefixTab is the prefix we look for at the beginning of
	// comments when we have a tab instead of a space.
	CommentPrefixTab = "//\t"

	// CommentGolangPrefix is the magic golang prefix that tools use.
	CommentGolangPrefix = "//go:"

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

	// commentDirectivePattern matches other special golang directive comments.
	commentDirectivePattern = regexp.MustCompile(`^//(line |extern |export |[a-z0-9]+:[a-z0-9])`)
)

func main() {
	write := flag.Bool("w", false, "reflow comments in place")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-w] <file> [file...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
		return
	}

	exit := 0
	for _, filename := range flag.Args() {
		if filename == "" {
			fmt.Fprintf(os.Stderr, "filename is empty\n")
			os.Exit(2)
			return
		}

		var err error
		if *write {
			err = Reflow(filename)
		} else {
			err = Check(filename)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed with: %+v\n", err)
			exit = 1
		}
	}

	os.Exit(exit)
}

// Check returns the comment checker on an individual filename.
func Check(filename string) error {
	if Debug {
		log.Printf("filename: %s", filename)
	}

	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()

	// f is a: https://golang.org/pkg/go/ast/#File
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return err
	}

	for _, doc := range docCommentGroups(f) {
		if err := checkCommentGroup(filename, src, fset, doc.group, doc.allowNonDoc); err != nil {
			return err
		}
	}

	return nil
}

type docCommentGroup struct {
	group       *ast.CommentGroup
	allowNonDoc bool
}

// docCommentGroups returns the comment groups checked by this tool.
func docCommentGroups(f *ast.File) []docCommentGroup {
	groups := []docCommentGroup{}
	appendGroup := func(group *ast.CommentGroup, allowNonDoc bool) {
		if group != nil {
			groups = append(groups, docCommentGroup{
				group:       group,
				allowNonDoc: allowNonDoc,
			})
		}
	}
	appendGroup(f.Doc, false)

	for _, node := range f.Decls {
		// TODO: move to type switch ?
		if x, ok := node.(*ast.FuncDecl); ok {
			appendGroup(x.Doc, false)
		} else if x, ok := node.(*ast.GenDecl); ok {
			switch x.Tok {
			case token.IMPORT: // i don't think this is needed, aiui
			case token.VAR:
				// TODO: recurse into https://golang.org/pkg/go/ast/#ValueSpec
				appendGroup(x.Doc, false)
			case token.CONST:
				appendGroup(x.Doc, false)
				for _, spec := range x.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					appendGroup(valueSpec.Doc, true)
				}
			case token.TYPE: // struct, usually
				appendGroup(x.Doc, false)
				for _, spec := range x.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					appendGroup(typeSpec.Doc, false)
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range structType.Fields.List {
						appendGroup(field.Doc, true)
					}
				}
			default:
			}
		}
	}

	return groups
}

// Reflow fixes the comment wrapping in an individual filename.
func Reflow(filename string) error {
	if Debug {
		log.Printf("filename: %s", filename)
	}

	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return err
	}

	newline := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		newline = "\r\n"
	}

	type replacement struct {
		start int
		end   int
		text  []byte
	}
	replacements := []replacement{}
	for _, doc := range docCommentGroups(f) {
		start, end, text, ok := reflowCommentGroup(src, fset, doc.group, newline, doc.allowNonDoc)
		if !ok || bytes.Equal(src[start:end], text) {
			continue
		}
		replacements = append(replacements, replacement{
			start: start,
			end:   end,
			text:  text,
		})
	}

	result := src
	for i := len(replacements) - 1; i >= 0; i-- {
		x := replacements[i]
		updated := make([]byte, 0, len(result)-x.end+x.start+len(x.text))
		updated = append(updated, result[:x.start]...)
		updated = append(updated, x.text...)
		updated = append(updated, result[x.end:]...)
		result = updated
	}

	if bytes.Equal(src, result) {
		return Check(filename)
	}
	if err := os.WriteFile(filename, result, info.Mode()); err != nil {
		return err
	}

	return Check(filename)
}

// reflowCommentGroup returns a replacement for the checkable prefix of a
// comment group. Multiline and directive comments, and everything after them,
// are left untouched in the source file.
func reflowCommentGroup(src []byte, fset *token.FileSet, doc *ast.CommentGroup, newline string, allowNonDoc bool) (int, int, []byte, bool) {
	comments := []*ast.Comment{}
	for _, comment := range doc.List {
		if comment == nil {
			continue
		}
		if strings.HasPrefix(comment.Text, CommentMultilinePrefix) ||
			strings.HasPrefix(comment.Text, CommentGolangPrefix) ||
			commentDirectivePattern.MatchString(comment.Text) {
			break
		}
		if comment.Text != commentPrefixTrimmed &&
			!strings.HasPrefix(comment.Text, CommentPrefix) &&
			!strings.HasPrefix(comment.Text, CommentPrefixTab) {
			if allowNonDoc {
				break
			}
			return 0, 0, nil, false
		}
		comments = append(comments, comment)
	}
	if len(comments) == 0 {
		return 0, 0, nil, false
	}

	file := fset.File(comments[0].Pos())
	start := file.Offset(comments[0].Pos())
	end := file.Offset(comments[len(comments)-1].End())
	lineStart := bytes.LastIndexByte(src[:start], '\n') + 1
	indent := src[lineStart:start]
	separator := newline + string(indent)
	length := commentLineLength(indent)

	lines := []string{}
	for _, comment := range comments {
		line := strings.TrimPrefix(comment.Text, commentPrefixTrimmed)
		if strings.HasPrefix(line, "\t") {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}

	formatted := []string{}
	for _, line := range reflowLines(lines, length) {
		switch {
		case line == "":
			formatted = append(formatted, commentPrefixTrimmed)
		case strings.HasPrefix(line, "\t"):
			formatted = append(formatted, commentPrefixTrimmed+line)
		default:
			formatted = append(formatted, CommentPrefix+line)
		}
	}

	return start, end, []byte(strings.Join(formatted, separator)), true
}

// commentLineLength returns the available comment text width after accounting
// for its indentation and comment prefix.
func commentLineLength(indent []byte) int {
	width := 0
	for _, c := range indent {
		if c == '\t' {
			width += 8 - (width % 8)
			continue
		}
		width++
	}

	length := maxLength - width
	if length < 1 {
		return 1
	}
	return length
}

// reflowLines greedily wraps prose while preserving paragraph starts,
// tab-indented code blocks, and URLs that do not fit on the preceding line.
func reflowLines(lines []string, length int) []string {
	result := []string{}
	words := []string{}

	flush := func() {
		if len(words) == 0 {
			return
		}

		merged := mergeMarkdownLinkWords(words)
		line := merged[0]
		for _, word := range merged[1:] {
			if len(line)+len(" ")+len(word) <= length || (len(line) <= length && IsSpecialLine(word)) {
				line += " " + word
				continue
			}
			result = append(result, line)
			line = word
		}
		result = append(result, line)
		words = []string{}
	}

	for i, line := range lines {
		fields := commentWords(line)
		if len(fields) == 0 {
			flush()
			if len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}
			continue
		}
		if strings.HasPrefix(line, "\t") {
			flush()
			result = append(result, line)
			continue
		}
		if IsSpecialLine(fields[0]) && i > 0 &&
			len(lines[i-1])+len(" ")+len(fields[0]) > length {
			flush()
			result = append(result, line)
			continue
		}
		if IsNewStart(fields[0]) {
			flush()
		}
		words = append(words, fields...)
	}
	flush()

	return result
}

// commentWords splits prose on whitespace while preserving inline code spans.
func commentWords(line string) []string {
	words := []string{}
	start := -1
	inCode := false
	for i, r := range line {
		if unicode.IsSpace(r) && !inCode {
			if start >= 0 {
				words = append(words, line[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		if r == '`' {
			inCode = !inCode
		}
	}
	if start >= 0 {
		words = append(words, line[start:])
	}

	return mergeMarkdownLinkWords(words)
}

// mergeMarkdownLinkWords preserves spaces in inline Markdown link labels.
func mergeMarkdownLinkWords(words []string) []string {
	result := []string{}
	for i := 0; i < len(words); i++ {
		start := strings.LastIndex(words[i], "[")
		if start < 0 {
			result = append(result, words[i])
			continue
		}

		link := words[i]
		end := markdownLinkEnd(link, start)
		j := i
		for end < 0 && j+1 < len(words) {
			j++
			link += " " + words[j]
			end = markdownLinkEnd(link, start)
		}
		if end < 0 {
			result = append(result, words[i])
			continue
		}

		result = append(result, link)
		i = j
	}

	return result
}

// markdownLinkEnd returns the end of a complete inline Markdown link.
func markdownLinkEnd(s string, start int) int {
	if start < 0 || start >= len(s) || s[start] != '[' {
		return -1
	}

	closeLabel := strings.Index(s[start+1:], "](")
	if closeLabel < 0 {
		return -1
	}
	closeLabel += start + 1
	if strings.Contains(s[start+1:closeLabel], "[") {
		return -1
	}

	depth := 1
	escaped := false
	for i := closeLabel + len("]("); i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}

	return -1
}

// checkCommentGroup checks that an individual comment group is wrapped.
func checkCommentGroup(filename string, src []byte, fset *token.FileSet, doc *ast.CommentGroup, allowNonDoc bool) error {
	if doc == nil { // we got nothing
		return nil
	}

	pos := doc.Pos()
	ff := fset.File(pos)
	if ff.Name() == "" {
		return fmt.Errorf("file name is empty")
	}
	ident := fmt.Sprintf("%s:%d", ff.Name(), ff.Line(pos))
	start := ff.Offset(pos)
	lineStart := bytes.LastIndexByte(src[:start], '\n') + 1
	length := commentLineLength(src[lineStart:start])

	block := []string{}
	for _, comment := range doc.List {
		if comment == nil {
			continue
		}
		s := comment.Text

		// TODO: how do we deal with multiline comments?
		if strings.HasPrefix(s, CommentMultilinePrefix) {
			break // skip to the end of this block
		}

		// skip the magic compiler comments
		if strings.HasPrefix(s, CommentGolangPrefix) {
			break // skip to the end of this block
		}

		// skip other special golang directive comments
		if commentDirectivePattern.MatchString(s) {
			break // skip to the end of this block
		}

		// Allow a comment prefix that starts with a space or
		// one that starts with a tab. (Common for code blocks!)
		if s != commentPrefixTrimmed && !strings.HasPrefix(s, CommentPrefix) && !strings.HasPrefix(s, CommentPrefixTab) {
			if allowNonDoc {
				break
			}
			return fmt.Errorf("location (%s) missing comment prefix, has: %s", ident, s)
		}
		if s == commentPrefixTrimmed { // blank lines
			s = ""
		}

		if strings.HasPrefix(s, CommentPrefix) {
			s = strings.TrimPrefix(s, CommentPrefix)
		} else if strings.HasPrefix(s, CommentPrefixTab) {
			s = strings.TrimPrefix(s, commentPrefixTrimmed)
		}

		block = append(block, s)
	}

	if err := IsWrappedProperly(block, length); err != nil {
		m := strings.Join(block, "\n")
		msg := filename + " " + strings.Repeat(".", maxLength-len(filename+" "+"V")) + fmt.Sprintf("V\n%+v\n", m)
		fmt.Fprintf(os.Stderr, "%s", msg)
		return fmt.Errorf("block (%s) failed: %+v", ident, err) // TODO: errwrap ?
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
		if strings.HasPrefix(line, "\t") { // eg: indented code blocks
			previous = length // reset
			continue
		}

		if line != strings.TrimSpace(line) {
			return fmt.Errorf("line %d wasn't trimmed properly: %s", lineno, line)
		}

		if strings.Contains(line, "  ") { // double spaces
			return fmt.Errorf("line %d contained multiple spaces", lineno)
		}

		fields := commentWords(line)
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
		skip3 := len(fields) == 1 // a single unbreakable token

		if len(line) > length && (!skip1) && (!skip2) && (!skip3) {
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

	if word == "BUG:" {
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
	if IsCodeBlock(word) {
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

// IsCodeBlock returns true if the word starts with a code block backtick.
func IsCodeBlock(word string) bool {
	if strings.HasPrefix(word, "`") {
		return true
	}
	return false
}
