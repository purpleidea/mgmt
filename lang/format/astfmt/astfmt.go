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

// Package astfmt contains the mcl AST formatter. It prints a parsed AST back
// out as canonically formatted mcl source code. It must be used on a freshly
// parsed AST, since interpolation rewrites the AST into a form that no longer
// corresponds to the original source code. The comments that the lexer
// collected are woven back into the output based on their original positions.
//
// The formatting rules are:
//
// * Indentation is done with tabs, one per nesting level.
//
// * Blank lines are preserved where the author put them, but two or more
// consecutive blank lines collapse into exactly one. Leading blank lines at the
// start of the file and trailing ones at the end are removed.
//
// * The single-line vs multi-line form of lists, maps, structs, function call
// args, definition args, and if expressions is preserved as the author wrote
// it. Multi-line elements get a trailing comma, single-line elements don't, as
// required by the grammar.
//
// * Resource bodies always print one field per line. Empty bodies and empty
// containers collapse to their compact form unless they contain comments.
//
// * A comment on its own line is printed at the indentation depth of the
// statements around it. A comment on the same line as code is printed after
// that code with a single space before the pound (#) character. The text after
// the pound character is never modified.
//
// * Parenthesis are preserved exactly where the author wrote them. (They are
// stored in the AST with the transient ExprParen node.) No new parenthesis are
// invented, which is why this must only run on an AST which was created by the
// parser, since it is position and precedence faithful by parsing.
package astfmt

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/operators"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// Format runs the mcl formatter against a freshly parsed AST and the comments
// which the lexer collected for the same input. It returns the canonically
// formatted source code.
func Format(ctx context.Context, stmt interfaces.Stmt, comments []*interfaces.Comment) ([]byte, error) {
	if stmt == nil {
		return nil, fmt.Errorf("nil formatter AST")
	}

	// Sort defensively. The lexer returns these in order already.
	cs := make([]*interfaces.Comment, len(comments))
	copy(cs, comments)
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Row != cs[j].Row {
			return cs[i].Row < cs[j].Row
		}
		return cs[i].Col < cs[j].Col
	})

	printer := &printer{
		comments: cs,
		lastRow:  -1,
	}

	if prog, ok := stmt.(*ast.StmtProg); ok {
		if err := printer.prog(ctx, prog, 0); err != nil {
			return nil, err
		}
	} else {
		start := startRow(stmt)
		printer.flushComments(start, 0)
		printer.gap(start)
		if err := printer.stmt(ctx, stmt, 0); err != nil {
			return nil, err
		}
	}

	printer.flushComments(-1, 0) // flush any remaining comments

	return printer.buf.Bytes(), nil
}

// startRow returns the zero-based source row on which this node starts, or -1
// if no position information is available. It prefers the node's own recorded
// position and falls back to the smallest position in the whole subtree.
func startRow(node interfaces.Node) int {
	if pn, ok := node.(interfaces.PositionableNode); ok && pn.IsSet() {
		row, _ := pn.Pos()
		return row
	}
	row := -1
	fn := func(n interfaces.Node) error {
		if pn, ok := n.(interfaces.PositionableNode); ok && pn.IsSet() {
			if r, _ := pn.Pos(); row == -1 || r < row {
				row = r
			}
		}
		return nil
	}
	_ = node.Apply(fn) // the fn never errors
	return row
}

// endRow returns the largest zero-based end row over the whole subtree, or -1
// if no position information is available. We take the maximum because the end
// positions which the parser stores on the nodes themselves can be too small
// when a grammar rule ends with a non-terminal, but the positions of the
// individual tokens are always correct.
func endRow(node interfaces.Node) int {
	row := -1
	fn := func(n interfaces.Node) error {
		if pn, ok := n.(interfaces.PositionableNode); ok && pn.IsSet() {
			if r, _ := pn.End(); r > row {
				row = r
			}
		}
		return nil
	}
	_ = node.Apply(fn) // the fn never errors
	return row
}

// printer holds the state used while printing an AST.
type printer struct {
	buf bytes.Buffer

	// comments are all the source comments, sorted by position.
	comments []*interfaces.Comment

	// idx is the index of the next comment which was not yet printed.
	idx int

	// lastRow is the source row of the last content that was printed, or
	// -1 if nothing was printed yet.
	lastRow int

	// started is true after the first line was printed. It is used to
	// suppress leading blank lines at the start of the file.
	started bool
}

// indent writes the indentation for the given depth.
func (obj *printer) indent(depth int) {
	for i := 0; i < depth; i++ {
		obj.buf.WriteByte('\t')
	}
}

// gap writes a single blank line if the original source had one or more blank
// lines between the last printed content and the given row.
func (obj *printer) gap(row int) {
	if obj.started && row >= 0 && obj.lastRow >= 0 && row > obj.lastRow+1 {
		obj.buf.WriteByte('\n')
	}
}

// flushComments prints every pending standalone comment which appears before
// the given source row, each on its own line at the given depth. Pass a row of
// -1 to flush every remaining comment.
func (obj *printer) flushComments(row int, depth int) {
	for obj.idx < len(obj.comments) {
		comment := obj.comments[obj.idx]
		if row >= 0 && comment.Row >= row {
			return
		}
		obj.idx++
		obj.gap(comment.Row)
		obj.indent(depth)
		obj.buf.WriteByte('#')
		obj.buf.WriteString(comment.Value)
		obj.buf.WriteByte('\n')
		obj.lastRow = comment.Row
		obj.started = true
	}
}

// hasCommentBefore reports whether a pending comment appears before this row.
func (obj *printer) hasCommentBefore(row int) bool {
	if obj.idx >= len(obj.comments) || row < 0 {
		return false
	}
	return obj.comments[obj.idx].Row < row
}

// trailing writes the trailing comment for this source row if there is one. It
// must be called after the last content character of an output line, and before
// the newline is written.
func (obj *printer) trailing(row int) {
	if obj.idx >= len(obj.comments) || row < 0 {
		return
	}
	comment := obj.comments[obj.idx]
	if comment.Row != row {
		return
	}
	obj.idx++
	obj.buf.WriteString(" #")
	obj.buf.WriteString(comment.Value)
}

// endLine finishes an output line whose content ends on the given source row.
// It appends the trailing comment for that row if there is one.
func (obj *printer) endLine(row int) {
	obj.trailing(row)
	obj.buf.WriteByte('\n')
	if row >= 0 {
		obj.lastRow = row
	}
	obj.started = true
}

// progBlock prints a ` {...}` block containing a program body. The caller must
// have printed the header contents already, and it must print the final newline
// (or the else branch) afterwards. The parser locates these programs to span
// their braces exactly, so the openRow and closeRow args are only fallbacks for
// programs which weren't built by the parser.
func (obj *printer) progBlock(ctx context.Context, body interfaces.Stmt, openRow, closeRow, depth int) error {
	prog, ok := body.(*ast.StmtProg)
	if !ok {
		return fmt.Errorf("unsupported block body type: %T", body)
	}
	if prog.IsSet() {
		openRow, _ = prog.Pos()
		closeRow, _ = prog.End()
	}

	if len(prog.Body) == 0 && !obj.hasCommentBefore(closeRow) {
		obj.buf.WriteString(" {}")
		obj.lastRow = max(obj.lastRow, closeRow)
		return nil
	}

	obj.buf.WriteString(" {")
	obj.endLine(openRow)
	if err := obj.prog(ctx, prog, depth+1); err != nil {
		return err
	}
	obj.closeBlock(closeRow, depth)
	return nil
}

// closeBlock prints the closing curly brace of a block, flushing any interior
// comments first. It does not print a newline after the brace.
func (obj *printer) closeBlock(closeRow int, depth int) {
	obj.flushComments(closeRow, depth+1)
	obj.gap(closeRow)
	obj.indent(depth)
	obj.buf.WriteByte('}')
	if closeRow >= 0 {
		obj.lastRow = closeRow
	}
}

// prog prints the statements of a program body at the given depth.
func (obj *printer) prog(ctx context.Context, prog *ast.StmtProg, depth int) error {
	for _, stmt := range prog.Body {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := startRow(stmt)
		obj.flushComments(start, depth)
		obj.gap(start)
		if err := obj.stmt(ctx, stmt, depth); err != nil {
			return err
		}
	}
	return nil
}

// stmt prints a single statement, including its indentation and its final
// newline.
func (obj *printer) stmt(ctx context.Context, stmt interfaces.Stmt, depth int) error {
	switch x := stmt.(type) {
	case *ast.StmtBind:
		obj.indent(depth)
		obj.buf.WriteByte('$')
		obj.buf.WriteString(x.Ident)
		if x.Type != nil {
			s, err := typeString(x.Type)
			if err != nil {
				return err
			}
			obj.buf.WriteByte(' ')
			obj.buf.WriteString(s)
		}
		obj.buf.WriteString(" = ")
		if err := obj.expr(ctx, x.Value, depth); err != nil {
			return err
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtRes:
		return obj.stmtRes(ctx, x, depth)

	case *ast.StmtEdge:
		obj.indent(depth)
		for i, half := range x.EdgeHalfList {
			if i > 0 {
				obj.buf.WriteString(" -> ")
			}
			if err := obj.edgeHalf(ctx, half, depth); err != nil {
				return err
			}
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtIf:
		return obj.stmtIf(ctx, x, depth)

	case *ast.StmtFor:
		obj.indent(depth)
		obj.buf.WriteString("for $")
		obj.buf.WriteString(x.Index)
		obj.buf.WriteString(", $")
		obj.buf.WriteString(x.Value)
		obj.buf.WriteString(" in ")
		if err := obj.expr(ctx, x.Expr, depth); err != nil {
			return err
		}
		if err := obj.progBlock(ctx, x.Body, endRow(x.Expr), endRow(x), depth); err != nil {
			return err
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtForKV:
		obj.indent(depth)
		obj.buf.WriteString("forkv $")
		obj.buf.WriteString(x.Key)
		obj.buf.WriteString(", $")
		obj.buf.WriteString(x.Val)
		obj.buf.WriteString(" in ")
		if err := obj.expr(ctx, x.Expr, depth); err != nil {
			return err
		}
		if err := obj.progBlock(ctx, x.Body, endRow(x.Expr), endRow(x), depth); err != nil {
			return err
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtFunc:
		fn, ok := x.Func.(*ast.ExprFunc)
		if !ok {
			return fmt.Errorf("unsupported func statement contents type: %T", x.Func)
		}
		obj.indent(depth)
		obj.buf.WriteString("func ")
		obj.buf.WriteString(x.Name)
		if err := obj.funcSignatureBody(ctx, fn, startRow(x), depth); err != nil {
			return err
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtClass:
		obj.indent(depth)
		obj.buf.WriteString("class ")
		obj.buf.WriteString(x.Name)
		if x.Args != nil {
			// The grammar forbids newlines between the class keyword
			// and the open parenthesis, so that's the node's own row,
			// and the close parenthesis shares the row of the open
			// curly brace, which the body block knows.
			openParenRow := startRow(x)
			closeParenRow := openParenRow
			if prog, ok := x.Body.(*ast.StmtProg); ok && prog.IsSet() {
				closeParenRow, _ = prog.Pos()
			}
			if err := obj.defArgs(x.Args, openParenRow, closeParenRow, depth); err != nil {
				return err
			}
		}
		if err := obj.progBlock(ctx, x.Body, startRow(x), endRow(x), depth); err != nil {
			return err
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtInclude:
		obj.indent(depth)
		obj.buf.WriteString("include ")
		obj.buf.WriteString(x.Name)
		if x.Args != nil {
			if err := obj.callArgs(ctx, x.Args, startRow(x), endRow(x), depth); err != nil {
				return err
			}
		}
		if x.Alias != "" {
			obj.buf.WriteString(" as ")
			obj.buf.WriteString(x.Alias)
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtImport:
		obj.indent(depth)
		obj.buf.WriteString("import \"")
		obj.buf.WriteString(x.Name)
		obj.buf.WriteByte('"')
		if x.Alias != "" {
			obj.buf.WriteString(" as ")
			obj.buf.WriteString(x.Alias)
		}
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtComment:
		// These are not produced by the parser, but handle them anyway.
		obj.indent(depth)
		obj.buf.WriteByte('#')
		obj.buf.WriteString(x.Value)
		obj.endLine(endRow(x))
		return nil

	}

	return fmt.Errorf("unsupported statement type: %T", stmt)
}

// stmtRes prints a resource statement, including the collect variant.
func (obj *printer) stmtRes(ctx context.Context, x *ast.StmtRes, depth int) error {
	obj.indent(depth)
	if x.Collect {
		obj.buf.WriteString("collect ")
	}
	obj.buf.WriteString(x.Kind)
	obj.buf.WriteByte(' ')
	if err := obj.expr(ctx, x.Name, depth); err != nil {
		return err
	}

	// The special collect field is invisible, the parser generated it.
	contents := []ast.StmtResContents{}
	for _, line := range x.Contents {
		if _, ok := line.(*ast.StmtResCollect); ok {
			continue
		}
		contents = append(contents, line)
	}

	openRow := endRow(x.Name) // the row the open curly brace is on
	closeRow := endRow(x)

	if len(contents) == 0 && !obj.hasCommentBefore(closeRow) {
		obj.buf.WriteString(" {}")
		obj.endLine(closeRow)
		return nil
	}

	obj.buf.WriteString(" {")
	obj.endLine(openRow)

	for _, line := range contents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := startRow(line)
		obj.flushComments(start, depth+1)
		obj.gap(start)
		if err := obj.resContent(ctx, line, depth+1); err != nil {
			return err
		}
	}

	obj.closeBlock(closeRow, depth)
	obj.endLine(closeRow)
	return nil
}

// resContent prints one line of a resource body, with the trailing comma.
func (obj *printer) resContent(ctx context.Context, content ast.StmtResContents, depth int) error {
	switch x := content.(type) {
	case *ast.StmtResField:
		obj.indent(depth)
		obj.buf.WriteString(x.Field)
		obj.buf.WriteString(" => ")
		if x.Condition != nil {
			if err := obj.expr(ctx, x.Condition, depth); err != nil {
				return err
			}
			obj.buf.WriteString(" ?: ")
		}
		if err := obj.expr(ctx, x.Value, depth); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtResEdge:
		obj.indent(depth)
		obj.buf.WriteString(capitalizeKind(x.Property))
		obj.buf.WriteString(" => ")
		if x.Condition != nil {
			if err := obj.expr(ctx, x.Condition, depth); err != nil {
				return err
			}
			obj.buf.WriteString(" ?: ")
		}
		if err := obj.edgeHalf(ctx, x.EdgeHalf, depth); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(x))
		return nil

	case *ast.StmtResMeta:
		obj.indent(depth)
		if strings.ToLower(x.Property) == strings.ToLower(ast.MetaField) {
			obj.buf.WriteString("Meta")
		} else {
			obj.buf.WriteString("Meta:")
			obj.buf.WriteString(x.Property)
		}
		obj.buf.WriteString(" => ")
		if x.Condition != nil {
			if err := obj.expr(ctx, x.Condition, depth); err != nil {
				return err
			}
			obj.buf.WriteString(" ?: ")
		}
		if err := obj.expr(ctx, x.MetaExpr, depth); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(x))
		return nil
	}

	return fmt.Errorf("unsupported resource content type: %T", content)
}

// edgeHalf prints a single edge half, eg: Test["t1"] or Test["t1"].foo_send.
func (obj *printer) edgeHalf(ctx context.Context, half *ast.StmtEdgeHalf, depth int) error {
	obj.buf.WriteString(capitalizeKind(half.Kind))
	obj.buf.WriteByte('[')
	if err := obj.expr(ctx, half.Name, depth); err != nil {
		return err
	}
	obj.buf.WriteByte(']')
	if half.SendRecv != "" {
		obj.buf.WriteByte('.')
		obj.buf.WriteString(half.SendRecv)
	}
	return nil
}

// stmtIf prints an if statement. It also handles the panic special case, since
// the parser desugars a panic statement into a particular if statement shape
// which can not be built any other way.
func (obj *printer) stmtIf(ctx context.Context, x *ast.StmtIf, depth int) error {
	if call, ok := panicCall(x); ok {
		obj.indent(depth)
		obj.buf.WriteString(funcs.PanicFuncName)
		if err := obj.callArgs(ctx, call.Args, startRow(call), endRow(call), depth); err != nil {
			return err
		}
		obj.endLine(endRow(call))
		return nil
	}

	obj.indent(depth)
	obj.buf.WriteString("if ")
	if err := obj.expr(ctx, x.Condition, depth); err != nil {
		return err
	}

	// The blocks span their braces, and the grammar guarantees that the
	// `} else {` tokens all share one line, so every row we need is on
	// one of the two blocks. The fallback rows only matter for AST's that
	// weren't built by the parser.
	closeRow := endRow(x)
	if err := obj.progBlock(ctx, x.ThenBranch, endRow(x.Condition), closeRow, depth); err != nil {
		return err
	}
	if x.ElseBranch != nil {
		obj.buf.WriteString(" else")
		elseRow := obj.lastRow // the row of the closing then brace
		if err := obj.progBlock(ctx, x.ElseBranch, elseRow, closeRow, depth); err != nil {
			return err
		}
	}
	obj.endLine(closeRow)
	return nil
}

// expr prints an expression inline, starting at the current buffer position.
// The depth is the indentation depth to use if the expression spans multiple
// lines.
func (obj *printer) expr(ctx context.Context, expr interfaces.Expr, depth int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch x := expr.(type) {
	case *ast.ExprBool:
		obj.buf.WriteString(strconv.FormatBool(x.V))
		return nil

	case *ast.ExprStr:
		// The V field contains the raw source between the two quotes,
		// with all of the escape sequences intact, since those are
		// only processed during interpolation, which happens later.
		obj.buf.WriteByte('"')
		obj.buf.WriteString(x.V)
		obj.buf.WriteByte('"')
		return nil

	case *ast.ExprInt:
		obj.buf.WriteString(strconv.FormatInt(x.V, 10))
		return nil

	case *ast.ExprFloat:
		s, err := floatString(x.V)
		if err != nil {
			return err
		}
		obj.buf.WriteString(s)
		return nil

	case *ast.ExprList:
		return obj.exprList(ctx, x, depth)

	case *ast.ExprMap:
		return obj.exprMap(ctx, x, depth)

	case *ast.ExprStruct:
		return obj.exprStruct(ctx, x, depth)

	case *ast.ExprFunc:
		obj.buf.WriteString("func")
		return obj.funcSignatureBody(ctx, x, startRow(x), depth)

	case *ast.ExprCall:
		return obj.exprCall(ctx, x, depth)

	case *ast.ExprVar:
		obj.buf.WriteByte('$')
		obj.buf.WriteString(x.Name)
		return nil

	case *ast.ExprIf:
		return obj.exprIf(ctx, x, depth)

	case *ast.ExprParen:
		obj.buf.WriteByte('(')
		if err := obj.expr(ctx, x.Inner, depth); err != nil {
			return err
		}
		obj.buf.WriteByte(')')
		return nil

	case *ast.ExprBlock:
		// These only appear as if expression branches and function
		// bodies, and those printers unwrap them directly, so this is
		// only reached on an AST which wasn't built by the parser.
		return obj.expr(ctx, x.Inner, depth)
	}

	return fmt.Errorf("unsupported expression type: %T", expr)
}

// exprList prints a list expression in single or multi line form.
func (obj *printer) exprList(ctx context.Context, x *ast.ExprList, depth int) error {
	openRow := startRow(x)
	closeRow := endRow(x)

	if len(x.Elements) == 0 && !obj.hasCommentBefore(closeRow) {
		obj.buf.WriteString("[]")
		return nil
	}

	multi := obj.hasCommentBefore(closeRow)
	if len(x.Elements) > 0 && openRow >= 0 && startRow(x.Elements[0]) > openRow {
		multi = true
	}

	if !multi {
		obj.buf.WriteByte('[')
		for i, element := range x.Elements {
			if i > 0 {
				obj.buf.WriteString(", ")
			}
			if err := obj.expr(ctx, element, depth); err != nil {
				return err
			}
		}
		obj.buf.WriteByte(']')
		return nil
	}

	obj.buf.WriteByte('[')
	obj.endLine(openRow)
	for _, element := range x.Elements {
		start := startRow(element)
		obj.flushComments(start, depth+1)
		obj.gap(start)
		obj.indent(depth + 1)
		if err := obj.expr(ctx, element, depth+1); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(element))
	}
	obj.flushComments(closeRow, depth+1)
	obj.gap(closeRow)
	obj.indent(depth)
	obj.buf.WriteByte(']')
	if closeRow >= 0 {
		obj.lastRow = closeRow
	}
	return nil
}

// exprMap prints a map expression in single or multi line form.
func (obj *printer) exprMap(ctx context.Context, x *ast.ExprMap, depth int) error {
	openRow := startRow(x)
	closeRow := endRow(x)

	if len(x.KVs) == 0 && !obj.hasCommentBefore(closeRow) {
		obj.buf.WriteString("{}")
		return nil
	}

	multi := obj.hasCommentBefore(closeRow)
	if len(x.KVs) > 0 && openRow >= 0 && startRow(x.KVs[0]) > openRow {
		multi = true
	}

	if !multi {
		obj.buf.WriteByte('{')
		for i, kv := range x.KVs {
			if i > 0 {
				obj.buf.WriteString(", ")
			}
			if err := obj.expr(ctx, kv.Key, depth); err != nil {
				return err
			}
			obj.buf.WriteString(" => ")
			if err := obj.expr(ctx, kv.Val, depth); err != nil {
				return err
			}
		}
		obj.buf.WriteByte('}')
		return nil
	}

	obj.buf.WriteByte('{')
	obj.endLine(openRow)
	for _, kv := range x.KVs {
		start := startRow(kv)
		obj.flushComments(start, depth+1)
		obj.gap(start)
		obj.indent(depth + 1)
		if err := obj.expr(ctx, kv.Key, depth+1); err != nil {
			return err
		}
		obj.buf.WriteString(" => ")
		if err := obj.expr(ctx, kv.Val, depth+1); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(kv))
	}
	obj.flushComments(closeRow, depth+1)
	obj.gap(closeRow)
	obj.indent(depth)
	obj.buf.WriteByte('}')
	if closeRow >= 0 {
		obj.lastRow = closeRow
	}
	return nil
}

// exprStruct prints a struct expression in single or multi line form.
func (obj *printer) exprStruct(ctx context.Context, x *ast.ExprStruct, depth int) error {
	openRow := startRow(x)
	closeRow := endRow(x)

	if len(x.Fields) == 0 && !obj.hasCommentBefore(closeRow) {
		obj.buf.WriteString("struct{}")
		return nil
	}

	multi := obj.hasCommentBefore(closeRow)
	if len(x.Fields) > 0 && openRow >= 0 && startRow(x.Fields[0]) > openRow {
		multi = true
	}

	if !multi {
		obj.buf.WriteString("struct{")
		for i, field := range x.Fields {
			if i > 0 {
				obj.buf.WriteString(", ")
			}
			obj.buf.WriteString(field.Name)
			obj.buf.WriteString(" => ")
			if err := obj.expr(ctx, field.Value, depth); err != nil {
				return err
			}
		}
		obj.buf.WriteByte('}')
		return nil
	}

	obj.buf.WriteString("struct{")
	obj.endLine(openRow)
	for _, field := range x.Fields {
		start := startRow(field)
		obj.flushComments(start, depth+1)
		obj.gap(start)
		obj.indent(depth + 1)
		obj.buf.WriteString(field.Name)
		obj.buf.WriteString(" => ")
		if err := obj.expr(ctx, field.Value, depth+1); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(field))
	}
	obj.flushComments(closeRow, depth+1)
	obj.gap(closeRow)
	obj.indent(depth)
	obj.buf.WriteByte('}')
	if closeRow >= 0 {
		obj.lastRow = closeRow
	}
	return nil
}

// funcSignatureBody prints the parenthesized arg list, the optional return
// type, and the curly brace body of a function definition or lambda. The caller
// prints anything that comes before the open parenthesis. The headRow is the
// row that the signature starts on, which is also the row of the open
// parenthesis, because the grammar forbids newlines between them. Everything
// else is derived from the body block, which spans its braces. The close
// parenthesis usually shares the open brace row, unless a multi-line function
// type annotation sits between them, which only shifts some blank lines.
func (obj *printer) funcSignatureBody(ctx context.Context, x *ast.ExprFunc, headRow, depth int) error {
	inner := x.Body
	openBraceRow, closeBraceRow := headRow, endRow(x)
	if block, ok := x.Body.(*ast.ExprBlock); ok {
		inner = block.Inner
		if block.IsSet() {
			openBraceRow, _ = block.Pos()
			closeBraceRow, _ = block.End()
		}
	}

	if err := obj.defArgs(x.Args, headRow, openBraceRow, depth); err != nil {
		return err
	}
	if x.Return != nil {
		s, err := typeString(x.Return)
		if err != nil {
			return err
		}
		obj.buf.WriteByte(' ')
		obj.buf.WriteString(s)
	}

	bodyRow := startRow(inner)
	multi := obj.hasCommentBefore(closeBraceRow)
	if openBraceRow >= 0 && bodyRow > openBraceRow {
		multi = true
	}

	if !multi {
		obj.buf.WriteString(" { ")
		if err := obj.expr(ctx, inner, depth); err != nil {
			return err
		}
		obj.buf.WriteString(" }")
		obj.lastRow = max(obj.lastRow, closeBraceRow)
		return nil
	}

	obj.buf.WriteString(" {")
	obj.endLine(openBraceRow)
	obj.flushComments(bodyRow, depth+1)
	obj.gap(bodyRow)
	obj.indent(depth + 1)
	if err := obj.expr(ctx, inner, depth+1); err != nil {
		return err
	}
	obj.endLine(endRow(inner))
	obj.closeBlock(closeBraceRow, depth)
	return nil
}

// exprCall prints a call expression. The parser desugars many of the language
// constructs into special function calls, and this must reverse all of them, so
// that the printed output matches what the author originally wrote. The special
// function names can not be lexed as identifiers, so matching on them is
// unambiguous.
func (obj *printer) exprCall(ctx context.Context, x *ast.ExprCall, depth int) error {
	switch x.Name {
	case operators.OperatorFuncName:
		if len(x.Args) == 0 {
			return fmt.Errorf("operator call with no args")
		}
		operator, ok := x.Args[0].(*ast.ExprStr)
		if !ok {
			return fmt.Errorf("unsupported operator arg type: %T", x.Args[0])
		}
		if len(x.Args) == 1 { // zero arg, eg: `π`
			obj.buf.WriteString(operator.V)
			return nil
		}
		if len(x.Args) == 2 { // unary, eg: `not $b`
			obj.buf.WriteString(operator.V)
			obj.buf.WriteByte(' ')
			return obj.expr(ctx, x.Args[1], depth)
		}
		if len(x.Args) == 3 { // binary, eg: `$a + $b`
			if err := obj.expr(ctx, x.Args[1], depth); err != nil {
				return err
			}
			obj.buf.WriteByte(' ')
			obj.buf.WriteString(operator.V)
			obj.buf.WriteByte(' ')
			return obj.expr(ctx, x.Args[2], depth)
		}
		return fmt.Errorf("operator call with %d args", len(x.Args))

	case funcs.LookupFuncName: // `$foo[$key]`
		if len(x.Args) != 2 {
			return fmt.Errorf("lookup call with %d args", len(x.Args))
		}
		if err := obj.expr(ctx, x.Args[0], depth); err != nil {
			return err
		}
		obj.buf.WriteByte('[')
		if err := obj.expr(ctx, x.Args[1], depth); err != nil {
			return err
		}
		obj.buf.WriteByte(']')
		return nil

	case funcs.LookupDefaultFuncName: // `$foo[$key] || "default"`
		if len(x.Args) != 3 {
			return fmt.Errorf("lookup default call with %d args", len(x.Args))
		}
		if err := obj.expr(ctx, x.Args[0], depth); err != nil {
			return err
		}
		obj.buf.WriteByte('[')
		if err := obj.expr(ctx, x.Args[1], depth); err != nil {
			return err
		}
		obj.buf.WriteString("] || ")
		return obj.expr(ctx, x.Args[2], depth)

	case funcs.StructLookupFuncName: // `$foo->field`
		if len(x.Args) != 2 {
			return fmt.Errorf("struct lookup call with %d args", len(x.Args))
		}
		field, ok := x.Args[1].(*ast.ExprStr)
		if !ok {
			return fmt.Errorf("unsupported struct lookup field type: %T", x.Args[1])
		}
		if err := obj.expr(ctx, x.Args[0], depth); err != nil {
			return err
		}
		obj.buf.WriteString("->")
		obj.buf.WriteString(field.V)
		return nil

	case funcs.StructLookupOptionalFuncName: // `$foo->field || "default"`
		if len(x.Args) != 3 {
			return fmt.Errorf("struct lookup optional call with %d args", len(x.Args))
		}
		field, ok := x.Args[1].(*ast.ExprStr)
		if !ok {
			return fmt.Errorf("unsupported struct lookup field type: %T", x.Args[1])
		}
		if err := obj.expr(ctx, x.Args[0], depth); err != nil {
			return err
		}
		obj.buf.WriteString("->")
		obj.buf.WriteString(field.V)
		obj.buf.WriteString(" || ")
		return obj.expr(ctx, x.Args[2], depth)

	case funcs.ContainsFuncName:
		// This might have come from the `$needle in $haystack` sugar,
		// or from a literal `contains()` function call. The two parse
		// identically except that with the sugar, the position of the
		// call is the position of the first arg, since there is no
		// function name in the source.
		if len(x.Args) == 2 && x.IsSet() {
			row1, col1 := x.Pos()
			if pn, ok := x.Args[0].(interfaces.PositionableNode); ok && pn.IsSet() {
				if row2, col2 := pn.Pos(); row1 == row2 && col1 == col2 {
					if err := obj.expr(ctx, x.Args[0], depth); err != nil {
						return err
					}
					obj.buf.WriteString(" in ")
					return obj.expr(ctx, x.Args[1], depth)
				}
			}
		}
		// fall through to a normal function call below

	case funcs.CollectFuncName:
		// This is generated by the parser for the collect statement,
		// and the resource printer skips over it, so we should never
		// get here.
		return fmt.Errorf("unexpected collect function call")
	}

	// normal function call
	if x.Anon != nil {
		if err := obj.expr(ctx, x.Anon, depth); err != nil {
			return err
		}
	} else {
		if x.Var {
			obj.buf.WriteByte('$')
		}
		obj.buf.WriteString(x.Name)
	}

	openRow := startRow(x) // the function name can't contain a newline
	if x.Anon != nil {
		openRow = endRow(x.Anon)
	}
	return obj.callArgs(ctx, x.Args, openRow, endRow(x), depth)
}

// callArgs prints a parenthesized function call arg list in single or multi
// line form. The openRow is the row that the open parenthesis is on.
func (obj *printer) callArgs(ctx context.Context, args []interfaces.Expr, openRow, closeRow, depth int) error {
	multi := false
	if len(args) > 0 && openRow >= 0 && startRow(args[0]) > openRow {
		multi = true
	}

	if !multi {
		obj.buf.WriteByte('(')
		for i, arg := range args {
			if i > 0 {
				obj.buf.WriteString(", ")
			}
			if err := obj.expr(ctx, arg, depth); err != nil {
				return err
			}
		}
		obj.buf.WriteByte(')')
		return nil
	}

	obj.buf.WriteByte('(')
	obj.endLine(openRow)
	for _, arg := range args {
		start := startRow(arg)
		obj.flushComments(start, depth+1)
		obj.gap(start)
		obj.indent(depth + 1)
		if err := obj.expr(ctx, arg, depth+1); err != nil {
			return err
		}
		obj.buf.WriteByte(',')
		obj.endLine(endRow(arg))
	}
	obj.flushComments(closeRow, depth+1)
	obj.gap(closeRow)
	obj.indent(depth)
	obj.buf.WriteByte(')')
	if closeRow >= 0 {
		obj.lastRow = closeRow
	}
	return nil
}

// exprIf prints an if expression in single or multi line form. The two branch
// blocks span their braces, and the grammar guarantees that the `} else {`
// tokens all share one line, so every row we need is on one of the two blocks.
// The fallback rows only matter for AST's that weren't built by the parser.
func (obj *printer) exprIf(ctx context.Context, x *ast.ExprIf, depth int) error {
	obj.buf.WriteString("if ")
	if err := obj.expr(ctx, x.Condition, depth); err != nil {
		return err
	}

	thenInner, elseInner := x.ThenBranch, x.ElseBranch
	condRow := endRow(x.Condition)
	openRow, elseRow := condRow, condRow
	closeRow := endRow(x)
	if block, ok := x.ThenBranch.(*ast.ExprBlock); ok {
		thenInner = block.Inner
		if block.IsSet() {
			openRow, _ = block.Pos()
			elseRow, _ = block.End()
		}
	}
	if block, ok := x.ElseBranch.(*ast.ExprBlock); ok {
		elseInner = block.Inner
		if block.IsSet() {
			closeRow, _ = block.End()
		}
	}

	thenRow := startRow(thenInner)
	multi := obj.hasCommentBefore(closeRow)
	if openRow >= 0 && thenRow > openRow {
		multi = true
	}

	if !multi {
		obj.buf.WriteString(" { ")
		if err := obj.expr(ctx, thenInner, depth); err != nil {
			return err
		}
		obj.buf.WriteString(" } else { ")
		if err := obj.expr(ctx, elseInner, depth); err != nil {
			return err
		}
		obj.buf.WriteString(" }")
		obj.lastRow = max(obj.lastRow, closeRow)
		return nil
	}

	obj.buf.WriteString(" {")
	obj.endLine(openRow)
	obj.flushComments(thenRow, depth+1)
	obj.gap(thenRow)
	obj.indent(depth + 1)
	if err := obj.expr(ctx, thenInner, depth+1); err != nil {
		return err
	}
	obj.endLine(endRow(thenInner))

	obj.flushComments(elseRow, depth+1)
	obj.gap(elseRow)
	obj.indent(depth)
	obj.buf.WriteString("} else {")
	obj.endLine(elseRow)

	innerRow := startRow(elseInner)
	obj.flushComments(innerRow, depth+1)
	obj.gap(innerRow)
	obj.indent(depth + 1)
	if err := obj.expr(ctx, elseInner, depth+1); err != nil {
		return err
	}
	obj.endLine(endRow(elseInner))

	obj.closeBlock(closeRow, depth)
	return nil
}

// defArgs prints the parenthesized definition arg list of a function or class
// in single or multi line form, eg: `($a, $b str)`. The openRow and closeRow
// are the rows that the two parenthesis are on.
func (obj *printer) defArgs(args []*interfaces.Arg, openRow, closeRow, depth int) error {
	multi := obj.hasCommentBefore(closeRow)
	if len(args) > 0 && openRow >= 0 && args[0].IsSet() {
		if row, _ := args[0].Pos(); row > openRow {
			multi = true
		}
	}

	if !multi {
		obj.buf.WriteByte('(')
		for i, arg := range args {
			if i > 0 {
				obj.buf.WriteString(", ")
			}
			s, err := argString(arg)
			if err != nil {
				return err
			}
			obj.buf.WriteString(s)
		}
		obj.buf.WriteByte(')')
		return nil
	}

	obj.buf.WriteByte('(')
	obj.endLine(openRow)
	for _, arg := range args {
		start, end := -1, -1
		if arg.IsSet() {
			start, _ = arg.Pos()
			end, _ = arg.End()
		}
		obj.flushComments(start, depth+1)
		obj.gap(start)
		obj.indent(depth + 1)
		s, err := argString(arg)
		if err != nil {
			return err
		}
		obj.buf.WriteString(s)
		obj.buf.WriteByte(',')
		obj.endLine(end)
	}
	obj.flushComments(closeRow, depth+1)
	obj.gap(closeRow)
	obj.indent(depth)
	obj.buf.WriteByte(')')
	if closeRow >= 0 {
		obj.lastRow = closeRow
	}
	return nil
}

// argString returns a single definition arg, eg: `$a` or `$b str`.
func argString(arg *interfaces.Arg) (string, error) {
	s := "$" + arg.Name
	if arg.Type != nil {
		t, err := typeString(arg.Type)
		if err != nil {
			return "", err
		}
		s += " " + t
	}
	return s, nil
}

// typeString returns the mcl source representation of a type. This differs from
// the String method on the type, because function arg names must be printed
// with a dollar sign prefix to be parseable as mcl.
func typeString(typ *types.Type) (string, error) {
	if typ == nil {
		return "", fmt.Errorf("nil type")
	}

	switch typ.Kind {
	case types.KindBool:
		return "bool", nil

	case types.KindStr:
		return "str", nil

	case types.KindInt:
		return "int", nil

	case types.KindFloat:
		return "float", nil

	case types.KindList:
		s, err := typeString(typ.Val)
		if err != nil {
			return "", err
		}
		return "[]" + s, nil

	case types.KindMap:
		key, err := typeString(typ.Key)
		if err != nil {
			return "", err
		}
		val, err := typeString(typ.Val)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("map{%s: %s}", key, val), nil

	case types.KindStruct:
		fields := []string{}
		for _, name := range typ.Ord {
			t, ok := typ.Map[name]
			if !ok {
				return "", fmt.Errorf("malformed struct type field: %s", name)
			}
			s, err := typeString(t)
			if err != nil {
				return "", err
			}
			fields = append(fields, fmt.Sprintf("%s %s", name, s))
		}
		return fmt.Sprintf("struct{%s}", strings.Join(fields, "; ")), nil

	case types.KindFunc:
		args := []string{}
		for i, name := range typ.Ord {
			t, ok := typ.Map[name]
			if !ok {
				return "", fmt.Errorf("malformed func type arg: %s", name)
			}
			s, err := typeString(t)
			if err != nil {
				return "", err
			}
			// The parser and types.NewType invent positional digit
			// names for unnamed args. Those can't be written in
			// source (identifiers start with a letter) so we know
			// to omit them, which preserves what the user wrote. We
			// are storing the digit as a "hack" to represent the
			// "name was omitted here" signal.
			if name == strconv.Itoa(i) {
				args = append(args, s)
				continue
			}
			args = append(args, fmt.Sprintf("$%s %s", name, s))
		}
		out := ""
		if typ.Out != nil {
			s, err := typeString(typ.Out)
			if err != nil {
				return "", err
			}
			out = " " + s
		}
		return fmt.Sprintf("func(%s)%s", strings.Join(args, ", "), out), nil

	case types.KindVariant:
		return "variant", nil
	}

	return "", fmt.Errorf("unsupported type: %+v", typ)
}

// floatString returns the mcl source representation of a float. The lexer only
// accepts floats which contain a decimal point and no exponent, so this can't
// use the shortest scientific representation.
func floatString(v float64) (string, error) {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if strings.ContainsAny(s, "nI") { // NaN, Inf, -Inf
		return "", fmt.Errorf("unrepresentable float: %s", s)
	}
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s, nil
}

// capitalizeKind capitalizes the first letter of each colon separated chunk of
// a resource kind, eg: `dhcp:server` becomes `Dhcp:Server`. The lexer
// lowercases these when tokenizing, and this is the faithful inverse, since the
// lexer only accepts a single leading capital letter per chunk.
func capitalizeKind(kind string) string {
	chunks := strings.Split(kind, ":")
	for i, chunk := range chunks {
		chunks[i] = strings.Title(chunk)
	}
	return strings.Join(chunks, ":")
}

// panicCall detects the desugared form of the panic statement and returns the
// panic function call if it matches. The shape can not be built any other way
// because the panic resource kind is not a valid identifier in the lexer.
func panicCall(x *ast.StmtIf) (*ast.ExprCall, bool) {
	if x.ElseBranch != nil {
		return nil, false
	}
	res, ok := x.ThenBranch.(*ast.StmtRes)
	if !ok || res.Kind != interfaces.PanicResKind {
		return nil, false
	}
	call, ok := x.Condition.(*ast.ExprCall)
	if !ok {
		return nil, false
	}
	if call.Name != funcs.PanicFuncName && call.Name != funcs.PanicDebugFuncName {
		return nil, false
	}
	return call, true
}
