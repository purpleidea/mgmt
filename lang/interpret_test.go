// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// +build !root

package lang

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	"github.com/kylelemons/godebug/pretty"
	"github.com/spf13/afero"
)

func vertexAstCmpFn(v1, v2 pgraph.Vertex) (bool, error) {
	//fmt.Printf("V1: %T %+v\n", v1, v1)
	//node := v1.(*funcs.Node)
	//fmt.Printf("node: %T %+v\n", node, node)
	//fmt.Printf("V2: %T %+v\n", v2, v2)
	if v1.String() == "" || v2.String() == "" {
		return false, fmt.Errorf("oops, empty vertex")
	}
	return v1.String() == v2.String(), nil
}

func edgeAstCmpFn(e1, e2 pgraph.Edge) (bool, error) {
	if e1.String() == "" || e2.String() == "" {
		return false, fmt.Errorf("oops, empty edge")
	}
	return e1.String() == e2.String(), nil
}

type vtex string

func (obj *vtex) String() string {
	return string(*obj)
}

type edge string

func (obj *edge) String() string {
	return string(*obj)
}

func TestAstFunc0(t *testing.T) {
	scope := &interfaces.Scope{ // global scope
		Variables: map[string]interfaces.Expr{
			"hello":  &ExprStr{V: "world"},
			"answer": &ExprInt{V: 42},
		},
		// all the built-in top-level, core functions enter here...
		Functions: funcs.LookupPrefix(""),
	}

	type test struct { // an individual test
		name  string
		code  string
		fail  bool
		scope *interfaces.Scope
		graph *pgraph.Graph
	}
	testCases := []test{}

	{
		graph, _ := pgraph.NewGraph("g")
		testCases = append(testCases, test{ // 0
			"nil",
			``,
			false,
			nil,
			graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		testCases = append(testCases, test{
			name:  "scope only",
			code:  ``,
			fail:  false,
			scope: scope, // use the scope defined above
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2 := vtex("int(42)"), vtex("var(x)")
		e1 := edge("x")
		graph.AddVertex(&v1, &v2)
		graph.AddEdge(&v1, &v2, &e1)
		testCases = append(testCases, test{
			name: "two vars",
			code: `
				$x = 42
				$y = $x
			`,
			// TODO: this should fail with an unused variable error!
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		testCases = append(testCases, test{
			name: "self-referential vars",
			code: `
				$x = $y
				$y = $x
			`,
			fail:  true,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2, v3, v4, v5 := vtex("int(42)"), vtex("var(a)"), vtex("var(b)"), vtex("var(c)"), vtex("str(t)")
		e1, e2, e3 := edge("a"), edge("b"), edge("c")
		graph.AddVertex(&v1, &v2, &v3, &v4, &v5)
		graph.AddEdge(&v1, &v2, &e1)
		graph.AddEdge(&v2, &v3, &e2)
		graph.AddEdge(&v3, &v4, &e3)
		testCases = append(testCases, test{
			name: "chained vars",
			code: `
				test "t" {
					int64ptr => $c,
				}
				$c = $b
				$b = $a
				$a = 42
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2 := vtex("bool(true)"), vtex("var(b)")
		graph.AddVertex(&v1, &v2)
		e1 := edge("b")
		graph.AddEdge(&v1, &v2, &e1)
		testCases = append(testCases, test{
			name: "simple bool",
			code: `
				if $b {
				}
				$b = true
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2, v3, v4, v5 := vtex("str(t)"), vtex("str(+)"), vtex("int(42)"), vtex("int(13)"), vtex(fmt.Sprintf("call:%s(str(+), int(42), int(13))", operatorFuncName))
		graph.AddVertex(&v1, &v2, &v3, &v4, &v5)
		e1, e2, e3 := edge("x"), edge("a"), edge("b")
		graph.AddEdge(&v2, &v5, &e1)
		graph.AddEdge(&v3, &v5, &e2)
		graph.AddEdge(&v4, &v5, &e3)
		testCases = append(testCases, test{
			name: "simple operator",
			code: `
				test "t" {
					int64ptr => 42 + 13,
				}
			`,
			fail:  false,
			scope: scope,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2, v3 := vtex("str(t)"), vtex("str(-)"), vtex("str(+)")
		v4, v5, v6 := vtex("int(42)"), vtex("int(13)"), vtex("int(99)")
		v7 := vtex(fmt.Sprintf("call:%s(str(+), int(42), int(13))", operatorFuncName))
		v8 := vtex(fmt.Sprintf("call:%s(str(-), call:%s(str(+), int(42), int(13)), int(99))", operatorFuncName, operatorFuncName))

		graph.AddVertex(&v1, &v2, &v3, &v4, &v5, &v6, &v7, &v8)
		e1, e2, e3 := edge("x"), edge("a"), edge("b")
		graph.AddEdge(&v3, &v7, &e1)
		graph.AddEdge(&v4, &v7, &e2)
		graph.AddEdge(&v5, &v7, &e3)

		e4, e5, e6 := edge("x"), edge("a"), edge("b")
		graph.AddEdge(&v2, &v8, &e4)
		graph.AddEdge(&v7, &v8, &e5)
		graph.AddEdge(&v6, &v8, &e6)
		testCases = append(testCases, test{
			name: "simple operators",
			code: `
				test "t" {
					int64ptr => 42 + 13 - 99,
				}
			`,
			fail:  false,
			scope: scope,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2 := vtex("bool(true)"), vtex("str(t)")
		v3, v4 := vtex("int(13)"), vtex("int(42)")
		v5, v6 := vtex("var(i)"), vtex("var(x)")
		v7, v8 := vtex("str(+)"), vtex(fmt.Sprintf("call:%s(str(+), int(42), var(i))", operatorFuncName))

		e1, e2, e3, e4, e5 := edge("x"), edge("a"), edge("b"), edge("i"), edge("x")
		graph.AddVertex(&v1, &v2, &v3, &v4, &v5, &v6, &v7, &v8)
		graph.AddEdge(&v3, &v5, &e4)

		graph.AddEdge(&v7, &v8, &e1)
		graph.AddEdge(&v4, &v8, &e2)
		graph.AddEdge(&v5, &v8, &e3)

		graph.AddEdge(&v8, &v6, &e5)
		testCases = append(testCases, test{
			name: "nested resource and scoped var",
			code: `
				if true {
					test "t" {
						int64ptr => $x,
					}
					$x = 42 + $i
				}
				$i = 13
			`,
			fail:  false,
			scope: scope,
			graph: graph,
		})
	}
	{
		testCases = append(testCases, test{
			name: "out of scope error",
			code: `
				# should be out of scope, and a compile error!
				if $b {
				}
				if true {
					$b = true
				}
			`,
			fail: true,
		})
	}
	{
		testCases = append(testCases, test{
			name: "variable re-declaration error",
			code: `
				# this should fail b/c of variable re-declaration
				$x = "hello"
				$x = "world"	# woops
			`,
			fail: true,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2, v3 := vtex("str(hello)"), vtex("str(world)"), vtex("bool(true)")
		v4, v5 := vtex("var(x)"), vtex("str(t)")

		graph.AddVertex(&v1, &v2, &v3, &v4, &v5)
		e1 := edge("x")
		// only one edge! (cool)
		graph.AddEdge(&v1, &v4, &e1)

		testCases = append(testCases, test{
			name: "variable shadowing",
			code: `
				# this should be okay, because var is shadowed
				$x = "hello"
				if true {
					$x = "world"	# shadowed
				}
				test "t" {
					stringptr => $x,
				}
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		v1, v2, v3 := vtex("str(hello)"), vtex("str(world)"), vtex("bool(true)")
		v4, v5 := vtex("var(x)"), vtex("str(t)")

		graph.AddVertex(&v1, &v2, &v3, &v4, &v5)
		e1 := edge("x")
		// only one edge! (cool)
		graph.AddEdge(&v2, &v4, &e1)

		testCases = append(testCases, test{
			name: "variable shadowing inner",
			code: `
				# this should be okay, because var is shadowed
				$x = "hello"
				if true {
					$x = "world"	# shadowed
					test "t" {
						stringptr => $x,
					}
				}
			`,
			fail:  false,
			graph: graph,
		})
	}
	//	// FIXME: blocked by: https://github.com/purpleidea/mgmt/issues/199
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	v0 := vtex("bool(true)")
	//	v1, v2 := vtex("str(hello)"), vtex("str(world)")
	//	v3, v4 := vtex("var(x)"), vtex("var(x)") // different vertices!
	//	v5, v6 := vtex("str(t1)"), vtex("str(t2)")
	//
	//	graph.AddVertex(&v0, &v1, &v2, &v3, &v4, &v5, &v6)
	//	e1, e2 := edge("x"), edge("x")
	//	graph.AddEdge(&v1, &v3, &e1)
	//	graph.AddEdge(&v2, &v4, &e2)
	//
	//	testCases = append(testCases, test{
	//		name: "variable shadowing both",
	//		code: `
	//			# this should be okay, because var is shadowed
	//			$x = "hello"
	//			if true {
	//				$x = "world"	# shadowed
	//				test "t2" {
	//					stringptr => $x,
	//				}
	//			}
	//			test "t1" {
	//				stringptr => $x,
	//			}
	//		`,
	//		fail: false,
	//		graph: graph,
	//	})
	//}
	//	// FIXME: blocked by: https://github.com/purpleidea/mgmt/issues/199
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	v1, v2 := vtex("str(cowsay)"), vtex("str(cowsay)")
	//	v3, v4 := vtex("str(installed)"), vtex("str(newest)")
	//
	//	graph.AddVertex(&v1, &v2, &v3, &v4)
	//
	//	testCases = append(testCases, test{
	//		name: "duplicate resource",
	//		code: `
	//			# these two are allowed because they are compatible
	//			pkg "cowsay" {
	//				state => "installed",
	//			}
	//			pkg "cowsay" {
	//				state => "newest",
	//			}
	//		`,
	//		fail: false,
	//		graph: graph,
	//	})
	//}
	{
		testCases = append(testCases, test{
			name: "variable re-declaration and type change error",
			code: `
				# this should fail b/c of variable re-declaration
				$x = "wow"
				$x = 99	# woops, but also a change of type :P
			`,
			fail: true,
		})
	}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple operators" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, code, fail, scope, exp := tc.name, tc.code, tc.fail, tc.scope, tc.graph

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)
			str := strings.NewReader(code)
			ast, err := LexParse(str)
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			t.Logf("test #%d: AST: %+v", index, ast)

			data := &interfaces.Data{
				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					t.Logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			iast, err := ast.Interpolate()
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}

			// propagate the scope down through the AST...
			err = iast.SetScope(scope)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not set scope: %+v", index, err)
				return
			}
			if fail && err != nil {
				return // fail happened during set scope, don't run unification!
			}

			// apply type unification
			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": unification: "+format, v...)
			}
			err = unification.Unify(iast, unification.SimpleInvariantSolverLogger(logf))
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not unify types: %+v", index, err)
				return
			}
			// maybe it will fail during graph below instead?
			//if fail && err == nil {
			//	t.Errorf("test #%d: FAIL", index)
			//	t.Errorf("test #%d: unification passed, expected fail", index)
			//	continue
			//}
			if fail && err != nil {
				return // fail happened during unification, don't run Graph!
			}

			// build the function graph
			graph, err := iast.Graph()

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions failed with: %+v", index, err)
				return
			}
			if fail && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			if fail { // can't process graph if it's nil
				// TODO: match against expected error
				t.Logf("test #%d: error: %+v", index, err)
				return
			}

			t.Logf("test #%d: graph: %+v", index, graph)
			// TODO: improve: https://github.com/purpleidea/mgmt/issues/199
			if err := graph.GraphCmp(exp, vertexAstCmpFn, edgeAstCmpFn); err != nil {
				t.Errorf("test #%d: FAIL\n\n", index)
				t.Logf("test #%d:   actual (g1): %v%s\n\n", index, graph, fullPrint(graph))
				t.Logf("test #%d: expected (g2): %v%s\n\n", index, exp, fullPrint(exp))
				t.Errorf("test #%d: cmp error:\n%v", index, err)
				return
			}

			for i, v := range graph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range graph.Adjacency() {
				for v2, e := range graph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}
		})
	}
}

// TestAstFunc1 is a more advanced version which pulls code from physical dirs.
func TestAstFunc1(t *testing.T) {
	const magicError = "# err: "
	const magicError1 = "err1: "
	const magicError2 = "err2: "
	const magicError3 = "err3: "
	const magicError4 = "err4: "
	const magicEmpty = "# empty!"
	dir, err := util.TestDirFull()
	if err != nil {
		t.Errorf("FAIL: could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)
	scope := &interfaces.Scope{ // global scope
		Variables: map[string]interfaces.Expr{
			"purpleidea": &ExprStr{V: "hello world!"}, // james says hi
			// TODO: change to a func when we can change hostname dynamically!
			"hostname": &ExprStr{V: ""}, // NOTE: empty b/c not used
		},
		// all the built-in top-level, core functions enter here...
		Functions: funcs.LookupPrefix(""),
	}

	type errs struct {
		fail1 bool
		fail2 bool
		fail3 bool
		fail4 bool
	}
	type test struct { // an individual test
		name string
		path string // relative sub directory path inside tests dir
		fail bool
		//graph *pgraph.Graph
		expstr string // expected graph in string format
		errs   errs
	}
	testCases := []test{}
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	testCases = append(testCases, test{
	//		name:   "simple hello world",
	//		path:   "hello0/",
	//		fail:   false,
	//		expstr: graph.Sprint(),
	//	})
	//}

	// build test array automatically from reading the dir
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Errorf("FAIL: could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		graphFile := f + ".graph" // expected graph file
		graphFileFull := dir + graphFile
		info, err := os.Stat(graphFileFull)
		if err != nil || info.IsDir() {
			t.Errorf("FAIL: missing: %s", graphFile)
			t.Errorf("(err: %+v)", err)
			continue
		}
		content, err := ioutil.ReadFile(graphFileFull)
		if err != nil {
			t.Errorf("FAIL: could not read graph file: %+v", err)
			return
		}
		str := string(content) // expected graph

		// if the graph file has a magic error string, it's a failure
		errStr := ""
		fail1 := false
		fail2 := false
		fail3 := false
		fail4 := false
		if strings.HasPrefix(str, magicError) {
			errStr = strings.TrimPrefix(str, magicError)
			str = errStr

			if strings.HasPrefix(str, magicError1) {
				errStr = strings.TrimPrefix(str, magicError1)
				str = errStr
				fail1 = true
			}
			if strings.HasPrefix(str, magicError2) {
				errStr = strings.TrimPrefix(str, magicError2)
				str = errStr
				fail2 = true
			}
			if strings.HasPrefix(str, magicError3) {
				errStr = strings.TrimPrefix(str, magicError3)
				str = errStr
				fail3 = true
			}
			if strings.HasPrefix(str, magicError4) {
				errStr = strings.TrimPrefix(str, magicError4)
				str = errStr
				fail4 = true
			}
		}

		// add automatic test case
		testCases = append(testCases, test{
			name:   fmt.Sprintf("dir: %s", f),
			path:   f + "/",
			fail:   errStr != "",
			expstr: str,
			errs: errs{
				fail1: fail1,
				fail2: fail2,
				fail3: fail3,
				fail4: fail4,
			},
		})
		//t.Logf("adding: %s", f + "/")
	}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple operators" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, path, fail, expstr, errs := tc.name, tc.path, tc.fail, strings.Trim(tc.expstr, "\n"), tc.errs
			src := dir + path // location of the test
			fail1 := errs.fail1
			fail2 := errs.fail2
			fail3 := errs.fail3
			fail4 := errs.fail4

			t.Logf("\n\ntest #%d (%s) ----------------\npath: %s\n\n", index, name, src)

			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			}
			mmFs := afero.NewMemMapFs()
			afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
			fs := &util.Fs{Afero: afs}

			// use this variant, so that we don't copy the dir name
			// this is the equivalent to running `rsync -a src/ /`
			if err := util.CopyDiskContentsToFs(fs, src, "/", false); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: CopyDiskContentsToFs failed: %+v", index, err)
				return
			}

			// this shows us what we pulled in from the test dir:
			tree0, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree0)

			input := "/"
			logf("input: %s", input)

			output, err := parseInput(input, fs) // raw code can be passed in
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: parseInput failed: %+v", index, err)
				return
			}
			for _, fn := range output.Workers {
				if err := fn(fs); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: worker execution failed: %+v", index, err)
					return
				}
			}
			tree, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree)

			logf("main:\n%s", output.Main) // debug

			reader := bytes.NewReader(output.Main)
			ast, err := LexParse(reader)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			if fail1 && err != nil {
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if fail1 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse passed, expected fail", index)
				return
			}

			t.Logf("test #%d: AST: %+v", index, ast)

			importGraph, err := pgraph.NewGraph("importGraph")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not create graph: %+v", index, err)
				return
			}
			importVertex := &pgraph.SelfVertex{
				Name:  "",          // first node is the empty string
				Graph: importGraph, // store a reference to ourself
			}
			importGraph.AddVertex(importVertex)

			data := &interfaces.Data{
				Fs:       fs,
				Base:     output.Base,  // base dir (absolute path) the metadata file is in
				Files:    output.Files, // no really needed here afaict
				Imports:  importVertex,
				Metadata: output.Metadata,
				Modules:  "/" + interfaces.ModuleDirectory, // not really needed here afaict

				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			iast, err := ast.Interpolate()
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}

			// propagate the scope down through the AST...
			err = iast.SetScope(scope)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not set scope: %+v", index, err)
				return
			}
			if fail2 && err != nil {
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during set scope, don't run unification!
			}
			if fail2 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: set scope passed, expected fail", index)
				return
			}

			// apply type unification
			xlogf := func(format string, v ...interface{}) {
				logf("unification: "+format, v...)
			}
			err = unification.Unify(iast, unification.SimpleInvariantSolverLogger(xlogf))
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not unify types: %+v", index, err)
				return
			}
			if fail3 && err != nil {
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during unification, don't run Graph!
			}
			if fail3 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification passed, expected fail", index)
				return
			}

			// build the function graph
			graph, err := iast.Graph()

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions failed with: %+v", index, err)
				return
			}
			if fail4 && err != nil { // can't process graph if it's nil
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if fail4 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			t.Logf("test #%d: graph: %+v", index, graph)
			str := strings.Trim(graph.Sprint(), "\n") // text format of graph
			if expstr == magicEmpty {
				expstr = ""
			}
			// XXX: something isn't consistent, and I can't figure
			// out what, so workaround this by sorting these :(
			sortHack := func(x string) string {
				l := strings.Split(x, "\n")
				sort.Strings(l)
				return strings.Join(l, "\n")
			}
			str = sortHack(str)
			expstr = sortHack(expstr)
			if expstr != str {
				t.Errorf("test #%d: FAIL\n\n", index)
				t.Logf("test #%d:   actual (g1):\n%s\n\n", index, str)
				t.Logf("test #%d: expected (g2):\n%s\n\n", index, expstr)
				diff := pretty.Compare(str, expstr)
				if diff != "" { // bonus
					t.Logf("test #%d: diff:\n%s", index, diff)
				}
				return
			}

			for i, v := range graph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range graph.Adjacency() {
				for v2, e := range graph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}
		})
	}
}

// TestAstFunc2 is a more advanced version which pulls code from physical dirs.
// It also briefly runs the function engine and captures output. Only use with
// stable, static output.
func TestAstFunc2(t *testing.T) {
	const magicError = "# err: "
	const magicError1 = "err1: "
	const magicError2 = "err2: "
	const magicError3 = "err3: "
	const magicError4 = "err4: "
	const magicError5 = "err5: "
	const magicEmpty = "# empty!"
	dir, err := util.TestDirFull()
	if err != nil {
		t.Errorf("FAIL: could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)
	scope := &interfaces.Scope{ // global scope
		Variables: map[string]interfaces.Expr{
			"purpleidea": &ExprStr{V: "hello world!"}, // james says hi
			// TODO: change to a func when we can change hostname dynamically!
			"hostname": &ExprStr{V: ""}, // NOTE: empty b/c not used
		},
		// all the built-in top-level, core functions enter here...
		Functions: funcs.LookupPrefix(""),
	}

	type errs struct {
		fail1 bool
		fail2 bool
		fail3 bool
		fail4 bool
		fail5 bool
	}
	type test struct { // an individual test
		name string
		path string // relative sub directory path inside tests dir
		fail bool
		//graph *pgraph.Graph
		expstr string // expected output graph in string format
		errs   errs
	}
	testCases := []test{}
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	testCases = append(testCases, test{
	//		name:   "simple hello world",
	//		path:   "hello0/",
	//		fail:   false,
	//		expstr: graph.Sprint(),
	//	})
	//}

	// build test array automatically from reading the dir
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Errorf("FAIL: could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		graphFile := f + ".output" // expected output graph file
		graphFileFull := dir + graphFile
		info, err := os.Stat(graphFileFull)
		if err != nil || info.IsDir() {
			t.Errorf("FAIL: missing: %s", graphFile)
			t.Errorf("(err: %+v)", err)
			continue
		}
		content, err := ioutil.ReadFile(graphFileFull)
		if err != nil {
			t.Errorf("FAIL: could not read graph file: %+v", err)
			return
		}
		str := string(content) // expected graph

		// if the graph file has a magic error string, it's a failure
		errStr := ""
		fail1 := false
		fail2 := false
		fail3 := false
		fail4 := false
		fail5 := false
		if strings.HasPrefix(str, magicError) {
			errStr = strings.TrimPrefix(str, magicError)
			str = errStr

			if strings.HasPrefix(str, magicError1) {
				errStr = strings.TrimPrefix(str, magicError1)
				str = errStr
				fail1 = true
			}
			if strings.HasPrefix(str, magicError2) {
				errStr = strings.TrimPrefix(str, magicError2)
				str = errStr
				fail2 = true
			}
			if strings.HasPrefix(str, magicError3) {
				errStr = strings.TrimPrefix(str, magicError3)
				str = errStr
				fail3 = true
			}
			if strings.HasPrefix(str, magicError4) {
				errStr = strings.TrimPrefix(str, magicError4)
				str = errStr
				fail4 = true
			}
			if strings.HasPrefix(str, magicError5) {
				errStr = strings.TrimPrefix(str, magicError5)
				str = errStr
				fail5 = true
			}
		}

		// add automatic test case
		testCases = append(testCases, test{
			name:   fmt.Sprintf("dir: %s", f),
			path:   f + "/",
			fail:   errStr != "",
			expstr: str,
			errs: errs{
				fail1: fail1,
				fail2: fail2,
				fail3: fail3,
				fail4: fail4,
				fail5: fail5,
			},
		})
		//t.Logf("adding: %s", f + "/")
	}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple operators" {
		//	continue
		//}

		t.Run(fmt.Sprintf("test #%d (%s)", index, tc.name), func(t *testing.T) {
			name, path, fail, expstr, errs := tc.name, tc.path, tc.fail, strings.Trim(tc.expstr, "\n"), tc.errs
			src := dir + path // location of the test
			fail1 := errs.fail1
			fail2 := errs.fail2
			fail3 := errs.fail3
			fail4 := errs.fail4
			fail5 := errs.fail5

			t.Logf("\n\ntest #%d (%s) ----------------\npath: %s\n\n", index, name, src)

			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			}
			mmFs := afero.NewMemMapFs()
			afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
			fs := &util.Fs{Afero: afs}

			// use this variant, so that we don't copy the dir name
			// this is the equivalent to running `rsync -a src/ /`
			if err := util.CopyDiskContentsToFs(fs, src, "/", false); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: CopyDiskContentsToFs failed: %+v", index, err)
				return
			}

			// this shows us what we pulled in from the test dir:
			tree0, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree0)

			input := "/"
			logf("input: %s", input)

			output, err := parseInput(input, fs) // raw code can be passed in
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: parseInput failed: %+v", index, err)
				return
			}
			for _, fn := range output.Workers {
				if err := fn(fs); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: worker execution failed: %+v", index, err)
					return
				}
			}
			tree, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree)

			logf("main:\n%s", output.Main) // debug

			reader := bytes.NewReader(output.Main)
			ast, err := LexParse(reader)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			if fail1 && err != nil {
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if fail1 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse passed, expected fail", index)
				return
			}

			t.Logf("test #%d: AST: %+v", index, ast)

			importGraph, err := pgraph.NewGraph("importGraph")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not create graph: %+v", index, err)
				return
			}
			importVertex := &pgraph.SelfVertex{
				Name:  "",          // first node is the empty string
				Graph: importGraph, // store a reference to ourself
			}
			importGraph.AddVertex(importVertex)

			data := &interfaces.Data{
				Fs:       fs,
				Base:     output.Base,  // base dir (absolute path) the metadata file is in
				Files:    output.Files, // no really needed here afaict
				Imports:  importVertex,
				Metadata: output.Metadata,
				Modules:  "/" + interfaces.ModuleDirectory, // not really needed here afaict

				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			if err := ast.Init(data); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}

			iast, err := ast.Interpolate()
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}

			// propagate the scope down through the AST...
			err = iast.SetScope(scope)
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not set scope: %+v", index, err)
				return
			}
			if fail2 && err != nil {
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during set scope, don't run unification!
			}
			if fail2 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolation passed, expected fail", index)
				return
			}

			// apply type unification
			xlogf := func(format string, v ...interface{}) {
				logf("unification: "+format, v...)
			}
			err = unification.Unify(iast, unification.SimpleInvariantSolverLogger(xlogf))
			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not unify types: %+v", index, err)
				return
			}
			if fail3 && err != nil {
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during unification, don't run Graph!
			}
			if fail3 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification passed, expected fail", index)
				return
			}

			// build the function graph
			graph, err := iast.Graph()

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions failed with: %+v", index, err)
				return
			}
			if fail4 && err != nil { // can't process graph if it's nil
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if fail4 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			if graph.NumVertices() == 0 { // no funcs to load!
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: function graph is empty", index)
				return
			}

			// run the function engine once to get some real output
			funcs := &funcs.Engine{
				Graph:    graph,             // not the same as the output graph!
				Hostname: "",                // NOTE: empty b/c not used
				World:    nil,               // NOTE: nil b/c not used
				Debug:    testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("funcs: "+format, v...)
				},
				Glitch: false, // FIXME: verify this functionality is perfect!
			}

			logf("function engine initializing...")
			if err := funcs.Init(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: init error with func engine: %+v", index, err)
				return
			}

			logf("function engine validating...")
			if err := funcs.Validate(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: validate error with func engine: %+v", index, err)
				return
			}

			logf("function engine starting...")
			// On failure, we expect the caller to run Close() to shutdown all of
			// the currently initialized (and running) funcs... This is needed if
			// we successfully ran `Run` but isn't needed only for Init/Validate.
			if err := funcs.Run(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: run error with func engine: %+v", index, err)
				return
			}
			defer funcs.Close() // cleanup

			// wait for some activity
			logf("stream...")
			stream := funcs.Stream()
			select {
			case err, ok := <-stream:
				if !ok {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: stream closed", index)
					return
				}
				if err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: stream errored", index)
					return
				}
			}

			// run interpret!
			funcs.RLock() // in case something is actually changing
			ograph, err := interpret(iast)
			funcs.RUnlock()

			if !fail && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpret failed with: %+v", index, err)
				return
			}
			if fail5 && err != nil { // can't process graph if it's nil
				// TODO: %+v instead?
				s := fmt.Sprintf("%s", err) // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if fail5 && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpret passed, expected fail", index)
				return
			}

			t.Logf("test #%d: graph: %+v", index, ograph)
			str := strings.Trim(ograph.Sprint(), "\n") // text format of output graph
			if expstr == magicEmpty {
				expstr = ""
			}
			// XXX: something isn't consistent, and I can't figure
			// out what, so workaround this by sorting these :(
			sortHack := func(x string) string {
				l := strings.Split(x, "\n")
				sort.Strings(l)
				return strings.Join(l, "\n")
			}
			str = sortHack(str)
			expstr = sortHack(expstr)
			if expstr != str {
				t.Errorf("test #%d: FAIL\n\n", index)
				t.Logf("test #%d:   actual (g1):\n%s\n\n", index, str)
				t.Logf("test #%d: expected (g2):\n%s\n\n", index, expstr)
				diff := pretty.Compare(str, expstr)
				if diff != "" { // bonus
					t.Logf("test #%d: diff:\n%s", index, diff)
				}
				return
			}

			for i, v := range ograph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range ograph.Adjacency() {
				for v2, e := range ograph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}
		})
	}
}

// TestAstInterpret0 should only be run in limited circumstances. Read the code
// comments below to see how it is run.
func TestAstInterpret0(t *testing.T) {
	type test struct { // an individual test
		name  string
		code  string
		fail  bool
		graph *pgraph.Graph
	}
	testCases := []test{}

	{
		graph, _ := pgraph.NewGraph("g")
		testCases = append(testCases, test{ // 0
			"nil",
			``,
			false,
			graph,
		})
	}
	{
		testCases = append(testCases, test{
			name: "wrong res field type",
			code: `
				test "t1" {
					stringptr => 42,	# int, not str
				}
			`,
			fail: true,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		t1, _ := engine.NewNamedResource("test", "t1")
		x := t1.(*resources.TestRes)
		int64ptr := int64(42)
		x.Int64Ptr = &int64ptr
		str := "okay cool"
		x.StringPtr = &str
		int8ptr := int8(127)
		int8ptrptr := &int8ptr
		int8ptrptrptr := &int8ptrptr
		x.Int8PtrPtrPtr = &int8ptrptrptr
		graph.AddVertex(t1)
		testCases = append(testCases, test{
			name: "resource with three pointer fields",
			code: `
				test "t1" {
					int64ptr => 42,
					stringptr => "okay cool",
					int8ptrptrptr => 127,	# super nested
				}
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		t1, _ := engine.NewNamedResource("test", "t1")
		x := t1.(*resources.TestRes)
		stringptr := "wow"
		x.StringPtr = &stringptr
		graph.AddVertex(t1)
		testCases = append(testCases, test{
			name: "resource with simple string pointer field",
			code: `
				test "t1" {
					stringptr => "wow",
				}
			`,
			graph: graph,
		})
	}
	{
		// FIXME: add a better vertexCmpFn so we can compare send/recv!
		graph, _ := pgraph.NewGraph("g")
		t1, _ := engine.NewNamedResource("test", "t1")
		{
			x := t1.(*resources.TestRes)
			int64Ptr := int64(42)
			x.Int64Ptr = &int64Ptr
			graph.AddVertex(t1)
		}
		t2, _ := engine.NewNamedResource("test", "t2")
		{
			x := t2.(*resources.TestRes)
			int64Ptr := int64(13)
			x.Int64Ptr = &int64Ptr
			graph.AddVertex(t2)
		}
		edge := &engine.Edge{
			Name:   fmt.Sprintf("%s -> %s", t1, t2),
			Notify: false,
		}
		graph.AddEdge(t1, t2, edge)
		testCases = append(testCases, test{
			name: "two resources and send/recv edge",
			code: `
			test "t1" {
				int64ptr => 42,
			}
			test "t2" {
				int64ptr => 13,
			}

			Test["t1"].hello -> Test["t2"].stringptr # send/recv
			`,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		t1, _ := engine.NewNamedResource("test", "t1")
		x := t1.(*resources.TestRes)
		stringptr := "this is meta"
		x.StringPtr = &stringptr
		m := &engine.MetaParams{
			Noop:    true, // overwritten
			Retry:   -1,
			Delay:   0,
			Poll:    5,
			Limit:   4.2,
			Burst:   3,
			Sema:    []string{"foo:1", "bar:3"},
			Rewatch: false,
			Realize: true,
		}
		x.SetMetaParams(m)
		graph.AddVertex(t1)
		testCases = append(testCases, test{
			name: "resource with meta params",
			code: `
				test "t1" {
					stringptr => "this is meta",

					Meta => struct{
						noop => false,
						retry => -1,
						delay => 0,
						poll => 5,
						limit => 4.2,
						burst => 3,
						sema => ["foo:1", "bar:3",],
						rewatch => false,
						realize => true,
						reverse => true,
						autoedge => true,
						autogroup => true,
					},
					Meta:noop => true,
					Meta:reverse => true,
					Meta:autoedge => true,
					Meta:autogroup => true,
				}
			`,
			graph: graph,
		})
	}

	names := []string{}
	for index, tc := range testCases { // run all the tests
		name, code, fail, exp := tc.name, tc.code, tc.fail, tc.graph

		if name == "" {
			name = "<sub test not named>"
		}
		if util.StrInList(name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, name)
			continue
		}
		names = append(names, name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "nil" {
		//	continue
		//}

		t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

		str := strings.NewReader(code)
		ast, err := LexParse(str)
		if err != nil {
			t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
			continue
		}
		t.Logf("test #%d: AST: %+v", index, ast)

		data := &interfaces.Data{
			Debug: testing.Verbose(), // set via the -test.v flag to `go test`
			Logf: func(format string, v ...interface{}) {
				t.Logf("ast: "+format, v...)
			},
		}
		// some of this might happen *after* interpolate in SetScope or Unify...
		if err := ast.Init(data); err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
			return
		}

		// these tests only work in certain cases, since this does not
		// perform type unification, run the function graph engine, and
		// only gives you limited results... don't expect normal code to
		// run and produce meaningful things in this test...
		graph, err := interpret(ast)

		if !fail && err != nil {
			t.Errorf("test #%d: interpret failed with: %+v", index, err)
			continue
		}
		if fail && err == nil {
			t.Errorf("test #%d: interpret passed, expected fail", index)
			continue
		}

		if fail { // can't process graph if it's nil
			// TODO: match against expected error
			t.Logf("test #%d: expected fail, error: %+v", index, err)
			continue
		}

		t.Logf("test #%d: graph: %+v", index, graph)
		// TODO: improve: https://github.com/purpleidea/mgmt/issues/199
		if err := graph.GraphCmp(exp, vertexCmpFn, edgeCmpFn); err != nil {
			t.Logf("test #%d:   actual (g1): %v%s", index, graph, fullPrint(graph))
			t.Logf("test #%d: expected (g2): %v%s", index, exp, fullPrint(exp))
			t.Errorf("test #%d: cmp error:\n%v", index, err)
			continue
		}

		for i, v := range graph.Vertices() {
			t.Logf("test #%d: vertex(%d): %+v", index, i, v)
		}
		for v1 := range graph.Adjacency() {
			for v2, e := range graph.Adjacency()[v1] {
				t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
			}
		}
	}
}
