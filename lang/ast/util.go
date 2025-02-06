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

package ast

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// FuncPrefixToFunctionsScope is a helper function to return the functions
// portion of the scope from a function prefix lookup. Basically this wraps the
// implementation in the Func interface in the *ExprFunc struct.
func FuncPrefixToFunctionsScope(prefix string) map[string]interfaces.Expr {
	fns := funcs.LookupPrefix(prefix) // map[string]func() interfaces.Func
	exprs := make(map[string]interfaces.Expr)
	for name, f := range fns {

		x := f() // inspect
		// We can pass in Fns []*types.FuncValue for the simple and
		// simplepoly API's and avoid the double wrapping from the
		// simple/simplepoly API's to the main function api and back.
		if st, ok := x.(*simple.WrappedFunc); simple.DirectInterface && ok {
			fn := &ExprFunc{
				Title: name,

				Values: []*types.FuncValue{st.Fn}, // just one!
			}
			// XXX: should we run fn.SetType(st.Fn.Type()) ?
			exprs[name] = fn
			continue
		}

		//if st, ok := x.(*simplepoly.WrappedFunc); simplepoly.DirectInterface && ok {
		//	fn := &ExprFunc{
		//		Title: name,

		//		Values: st.Fns,
		//	}
		//	exprs[name] = fn
		//	continue
		//}

		fn := &ExprFunc{
			Title: name,
			// We need to pass in the constructor function, because
			// we'll need more than one copy of this function if it
			// is used in more than one place so we can build more.
			Function: f, // func() interfaces.Func
		}
		exprs[name] = fn
	}

	// Wrap every Expr in ExprPoly, so that the function can be used with
	// different types. Those functions are all builtins, so they don't need
	// to access the surrounding scope.
	exprPolys := make(map[string]interfaces.Expr)
	for name, expr := range exprs {
		exprPolys[name] = &ExprPoly{
			Definition: &ExprTopLevel{
				Definition:    expr,
				CapturedScope: interfaces.EmptyScope(),
			},
		}
	}

	return exprPolys
}

// VarPrefixToVariablesScope is a helper function to return the variables
// portion of the scope from a variable prefix lookup. Basically this is useful
// to pull out a portion of the variables we've defined by API.
// TODO: pass `data` into here so we can plumb it into Init for Expr's ?
func VarPrefixToVariablesScope(prefix string) map[string]interfaces.Expr {
	fns := vars.LookupPrefix(prefix) // map[string]func() interfaces.Var
	exprs := make(map[string]interfaces.Expr)
	for name, f := range fns {
		x := f() // inspect
		expr, err := ValueToExpr(x)
		if err != nil {
			panic(fmt.Sprintf("could not build expr: %+v", err))
		}
		exprs[name] = &ExprTopLevel{
			Definition: &ExprSingleton{
				Definition: expr,

				mutex: &sync.Mutex{}, // TODO: call Init instead
			},
			CapturedScope: interfaces.EmptyScope(),
		}
	}
	return exprs
}

// MergeExprMaps merges the two maps of Expr's, and errors if any overwriting
// would occur. If any prefix string is specified, that is added to the keys of
// the second "extra" map before doing the merge. This doesn't change the input
// maps.
func MergeExprMaps(m, extra map[string]interfaces.Expr, prefix ...string) (map[string]interfaces.Expr, error) {
	p := strings.Join(prefix, "") // hack to have prefix be optional

	result := map[string]interfaces.Expr{}
	for k, v := range m {
		result[k] = v // copy
	}

	for k, v := range extra {
		name := p + k
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate variable: %s", name)
		}
		result[name] = v
	}

	return result, nil
}

// ValueToExpr converts a Value into the equivalent Expr.
// FIXME: Add some tests for this function.
func ValueToExpr(val types.Value) (interfaces.Expr, error) {
	var expr interfaces.Expr

	switch x := val.(type) {
	case *types.BoolValue:
		expr = &ExprBool{
			V: x.Bool(),
		}

	case *types.StrValue:
		expr = &ExprStr{
			V: x.Str(),
		}

	case *types.IntValue:
		expr = &ExprInt{
			V: x.Int(),
		}

	case *types.FloatValue:
		expr = &ExprFloat{
			V: x.Float(),
		}

	case *types.ListValue:
		exprs := []interfaces.Expr{}

		for _, v := range x.List() {
			e, err := ValueToExpr(v)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, e)
		}

		expr = &ExprList{
			Elements: exprs,
		}

	case *types.MapValue:
		kvs := []*ExprMapKV{}

		for k, v := range x.Map() {
			kx, err := ValueToExpr(k)
			if err != nil {
				return nil, err
			}
			vx, err := ValueToExpr(v)
			if err != nil {
				return nil, err
			}
			kv := &ExprMapKV{
				Key: kx,
				Val: vx,
			}
			kvs = append(kvs, kv)
		}

		expr = &ExprMap{
			KVs: kvs,
		}

	case *types.StructValue:
		fields := []*ExprStructField{}

		for k, v := range x.Struct() {
			fx, err := ValueToExpr(v)
			if err != nil {
				return nil, err
			}
			field := &ExprStructField{
				Name:  k,
				Value: fx,
			}
			fields = append(fields, field)
		}

		expr = &ExprStruct{
			Fields: fields,
		}

	//case *types.FuncValue:
	//	// TODO: this particular case is particularly untested!
	//	expr = &ExprFunc{
	//		Title: "<func from ValueToExpr>", // TODO: change this?
	//		// TODO: symmetrically, it would have used x.Func() here
	//		Values: []*types.FuncValue{
	//			x, // just one!
	//		},
	//	}

	case *types.VariantValue:
		// TODO: should this be allowed, or should we unwrap them?
		return nil, fmt.Errorf("variant values are not supported")

	default:
		return nil, fmt.Errorf("unknown type (%T) for value: %+v", val, val)
	}

	return expr, expr.SetType(val.Type())
}

// CollectFiles collects all the files used in the AST. You will see more files
// based on how many compiling steps have run. In general, this is useful for
// collecting all the files needed to store in our file system for a deploy.
func CollectFiles(ast interfaces.Stmt) ([]string, error) {
	// collect the list of files
	fileList := []string{}
	fn := func(node interfaces.Node) error {
		// redundant check for example purposes
		stmt, ok := node.(interfaces.Stmt)
		if !ok {
			return nil
		}
		body, ok := stmt.(*StmtProg)
		if !ok {
			return nil
		}
		// collect into global
		fileList = append(fileList, body.importFiles...)
		return nil
	}
	if err := ast.Apply(fn); err != nil {
		return nil, errwrap.Wrapf(err, "can't retrieve paths")
	}
	return fileList, nil
}

// CopyNodeMapping copies the map of string to node and is used in Ordering.
func CopyNodeMapping(in map[string]interfaces.Node) map[string]interfaces.Node {
	out := make(map[string]interfaces.Node)
	for k, v := range in {
		out[k] = v // copy the map, not the Node's
	}
	return out
}

// getScope pulls the local stored scope out of an Expr, without needing to add
// a similarly named method to the Expr interface. This is private and not part
// of the interface, because it is only used internally.
// TODO: we could extend this to include Stmt's if it was ever useful
func getScope(node interfaces.Expr) (*interfaces.Scope, error) {
	//if _, ok := node.(interfaces.Expr); !ok {
	//	return nil, fmt.Errorf("unexpected: %+v", node)
	//}

	switch expr := node.(type) {
	case *ExprBool:
		return expr.scope, nil
	case *ExprStr:
		return expr.scope, nil
	case *ExprInt:
		return expr.scope, nil
	case *ExprFloat:
		return expr.scope, nil
	case *ExprList:
		return expr.scope, nil
	case *ExprMap:
		return expr.scope, nil
	case *ExprStruct:
		return expr.scope, nil
	case *ExprFunc:
		return expr.scope, nil
	case *ExprCall:
		return expr.scope, nil
	case *ExprVar:
		return expr.scope, nil
	case *ExprIf:
		return expr.scope, nil

	default:
		return nil, fmt.Errorf("unexpected: %+v", node)
	}
}

// trueCallee is a helper function because ExprTopLevel and ExprSingleton are
// sometimes added around builtins. This makes it difficult for the type checker
// to check if a particular builtin is the callee or not. This function removes
// the ExprTopLevel and ExprSingleton wrappers, if they exist.
func trueCallee(apparentCallee interfaces.Expr) interfaces.Expr {
	switch x := apparentCallee.(type) {
	case *ExprTopLevel:
		return trueCallee(x.Definition)
	case *ExprSingleton:
		return trueCallee(x.Definition)
	default:
		return apparentCallee
	}
}

// variableScopeFeedback logs some messages about what is actually in scope so
// that the user gets a hint about what's going on. This is useful for catching
// bugs in our programming or in user code!
func variableScopeFeedback(scope *interfaces.Scope, logf func(format string, v ...interface{})) {
	logf("variables in scope:")
	names := []string{}
	for name := range scope.Variables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		logf("$%s", name)
	}
}

// functionScopeFeedback logs some messages about what is actually in scope so
// that the user gets a hint about what's going on. This is useful for catching
// bugs in our programming or in user code!
func functionScopeFeedback(scope *interfaces.Scope, logf func(format string, v ...interface{})) {
	logf("functions in scope:")
	names := []string{}
	for name := range scope.Functions {
		if strings.HasPrefix(name, "_") { // hidden function
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		logf("%s(...)", name)
	}
}

// lambdaScopeFeedback logs some messages about what is actually in scope so
// that the user gets a hint about what's going on. This is useful for catching
// bugs in our programming or in user code!
func lambdaScopeFeedback(scope *interfaces.Scope, logf func(format string, v ...interface{})) {
	logf("lambdas in scope:")
	names := []string{}
	for name, val := range scope.Variables {
		// XXX: Is this a valid way to filter?
		if _, ok := trueCallee(val).(*ExprFunc); !ok {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		logf("$%s(...)", name)
	}
}

// AreaParentOf is a (flawed) heuristic that tries to find the parent of the
// given node (needle) in a tree (haystack), in the case that the given node
// does not store its own line/column location (i.e., the node struct does not
// embed the TextArea struct, and hence does not implement the LocalNode
// interface). This makes two assumptions:
// a) the immediate parent of needle is a LocalNode and
// b) any siblings to the left of needle (nor their children) are LocalNode
// Basically it only works in a subtree like
//
//	L
//	|\
//	x n
//
// where L is the LocalNode parent of x and n, and n is the needle.
func AreaParentOf(needle, haystack interfaces.Node) LocalNode {
	var LastArea LocalNode

	err := haystack.Apply(func(n interfaces.Node) error {
		ln, ok := n.(LocalNode)
		if ok {
			LastArea = ln
		}
		if n == needle {
			return fmt.Errorf("found")
		}
		return nil
	})

	if err != nil && err.Error() == "found" {
		return LastArea
	}
	return haystack.(LocalNode)
}

// TextArea stores the coordinates of a statement or expression in the form of a
// starting line/column and ending line/column.
type TextArea struct {
	startLine   int
	startColumn int
	endLine     int
	endColumn   int

	path string

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// Locate is used by the parser to store the token positions in AST nodes.
// The path will be filled during AST node initialization usually, because
// the parser does not know the name of the file it is processing.
func (obj *TextArea) Locate(line int, col int, endline int, endcol int) {
	obj.startLine = line
	obj.startColumn = col
	obj.endLine = endline
	obj.endColumn = endcol
}

// SetPath is used during AST initialization in order to store in each AST node
// the name of the source file from which it was generated.
func (obj *TextArea) SetPath(path string) {
	obj.path = path
}

// String gives a succinct representation of the TextArea, but is useful only
// in debugging. In order to generate pretty error messages, see HighlightText.
func (obj *TextArea) String() string {
	return fmt.Sprintf("%s:%d:%d", obj.path, obj.startLine, obj.startColumn) // XXX: +1 ?
}

// LocalNode is the interface implemented by AST nodes that store their code
// position. It is implemented by node types that embed TextArea.
type LocalNode interface {
	Locate(int, int, int, int)
	Pos() (int, int)
	End() (int, int)
	String() string
}

// Pos returns the starting line/column of an AST node.
func (obj *TextArea) Pos() (int, int) {
	return obj.startLine, obj.startColumn
}

// End returns the end line/column of an AST node.
func (obj *TextArea) End() (int, int) {
	return obj.endLine, obj.endColumn
}

// Path returns the name of the source file that holds the code for an AST node.
func (obj *TextArea) Path() string {
	return obj.path
}

// HighlightText generates a generic description that just visually indicates
// part of the line described by a TextArea. If the coordinates that are passed
// span multiple lines, don't show those lines, but just a description of the
// area.
func (obj *TextArea) HighlightText() (string, error) {
	first := obj.startLine
	left := obj.startColumn
	last := obj.endLine
	right := obj.endColumn
	filename := obj.path

	if filename == "" {
		return "unknown source file", nil
	}

	if first == 0 {
		return "in " + filename, nil
	}

	if first != last {
		return fmt.Sprintf("in %s L%d-%d\n", filename, first, last), nil
	}

	readFile, err := os.Open(filename)
	if err != nil {
		return "", errwrap.Wrapf(err, "could not open file %s", filename)
	}
	defer readFile.Close()
	scanner := bufio.NewScanner(readFile)
	for i := 0 ; i < first ; i++ {
		scanner.Scan()
	}
	if err := scanner.Err() ; err != nil {
		return "", errwrap.Wrapf(err, "could not read file %s", filename)
	}

	// handle indentation correctly, even if it mixes spaces and tabs
	line := scanner.Text() + "\n"
	text := strings.TrimLeft(line, " \t")
	indent := strings.TrimSuffix(line, text)
	offset := len(indent)

	result := fmt.Sprintf("in %s L%d:\n--\n", filename, first)
	result += scanner.Text() + "\n"
	result += indent + strings.Repeat(" ", left-1-offset) + strings.Repeat("^", right-left+1)
	return result, nil
}

