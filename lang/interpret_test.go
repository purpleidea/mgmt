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
	"fmt"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/pgraph"
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
	}

	type test struct { // an individual test
		name  string
		code  string
		fail  bool
		scope *interfaces.Scope
		graph *pgraph.Graph
	}
	values := []test{}

	{
		graph, _ := pgraph.NewGraph("g")
		values = append(values, test{ // 0
			"nil",
			``,
			false,
			nil,
			graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
			name: "simple operator",
			code: `
				test "t" {
					int64ptr => 42 + 13,
				}
			`,
			fail:  false,
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
		values = append(values, test{
			name: "simple operators",
			code: `
				test "t" {
					int64ptr => 42 + 13 - 99,
				}
			`,
			fail:  false,
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
		values = append(values, test{
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
			graph: graph,
		})
	}
	{
		values = append(values, test{
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
		values = append(values, test{
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

		values = append(values, test{
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

		values = append(values, test{
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
	//	values = append(values, test{
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
	{
		values = append(values, test{
			name: "variable re-declaration and type change error",
			code: `
				# this should fail b/c of variable re-declaration
				$x = "wow"
				$x = 99	# woops, but also a change of type :P
			`,
			fail: true,
		})
	}

	for index, test := range values { // run all the tests
		name, code, fail, scope, exp := test.name, test.code, test.fail, test.scope, test.graph

		if name == "" {
			name = "<sub test not named>"
		}

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if test.name != "simple operators" {
		//	continue
		//}

		t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)
		str := strings.NewReader(code)
		ast, err := LexParse(str)
		if err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
			continue
		}
		t.Logf("test #%d: AST: %+v", index, ast)

		iast, err := ast.Interpolate()
		if err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: interpolate failed with: %+v", index, err)
			continue
		}

		// propagate the scope down through the AST...
		err = iast.SetScope(scope)
		if !fail && err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: could not set scope: %+v", index, err)
			continue
		}
		if fail && err != nil {
			continue // fail happened during set scope, don't run unification!
		}

		// apply type unification
		logf := func(format string, v ...interface{}) {
			t.Logf(fmt.Sprintf("test #%d", index)+": unification: "+format, v...)
		}
		err = unification.Unify(iast, unification.SimpleInvariantSolverLogger(logf))
		if !fail && err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: could not unify types: %+v", index, err)
			continue
		}
		// maybe it will fail during graph below instead?
		//if fail && err == nil {
		//	t.Errorf("test #%d: FAIL", index)
		//	t.Errorf("test #%d: unification passed, expected fail", index)
		//	continue
		//}
		if fail && err != nil {
			continue // fail happened during unification, don't run Graph!
		}

		// build the function graph
		graph, err := iast.Graph()

		if !fail && err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: functions failed with: %+v", index, err)
			continue
		}
		if fail && err == nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: functions passed, expected fail", index)
			continue
		}

		if fail { // can't process graph if it's nil
			// TODO: match against expected error
			t.Logf("test #%d: error: %+v", index, err)
			continue
		}

		t.Logf("test #%d: graph: %+v", index, graph)
		// TODO: improve: https://github.com/purpleidea/mgmt/issues/199
		if err := graph.GraphCmp(exp, vertexAstCmpFn, edgeAstCmpFn); err != nil {
			t.Errorf("test #%d: FAIL\n\n", index)
			t.Logf("test #%d:   actual (g1): %v%s\n\n", index, graph, fullPrint(graph))
			t.Logf("test #%d: expected (g2): %v%s\n\n", index, exp, fullPrint(exp))
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

// TestAstInterpret0 should only be run in limited circumstances. Read the code
// comments below to see how it is run.
func TestAstInterpret0(t *testing.T) {
	type test struct { // an individual test
		name  string
		code  string
		fail  bool
		graph *pgraph.Graph
	}
	values := []test{}

	{
		graph, _ := pgraph.NewGraph("g")
		values = append(values, test{ // 0
			"nil",
			``,
			false,
			graph,
		})
	}
	{
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
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
		values = append(values, test{
			name: "two resources and send/recv edge",
			code: `
			test "t1" {
				int64ptr => 42,
			}
			test "t2" {
				int64ptr => 13,
			}

			Test["t1"].foosend -> Test["t2"].barrecv # send/recv
			`,
			graph: graph,
		})
	}

	for index, test := range values { // run all the tests
		name, code, fail, exp := test.name, test.code, test.fail, test.graph

		if name == "" {
			name = "<sub test not named>"
		}

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if test.name != "nil" {
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
