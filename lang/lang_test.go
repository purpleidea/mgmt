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
	_ "github.com/purpleidea/mgmt/lang/funcs/core" // import so the funcs register
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// TODO: unify with the other function like this...
// TODO: where should we put our test helpers?
func runGraphCmp(t *testing.T, g1, g2 *pgraph.Graph) {
	err := g1.GraphCmp(g2, vertexCmpFn, edgeCmpFn)
	if err != nil {
		t.Logf("  actual (g1): %v%s", g1, fullPrint(g1))
		t.Logf("expected (g2): %v%s", g2, fullPrint(g2))
		t.Errorf("Cmp error:\n%v", err)
	}
}

// TODO: unify with the other function like this...
func fullPrint(g *pgraph.Graph) (str string) {
	if g == nil {
		return "<nil>"
	}
	str += "\n"
	for v := range g.Adjacency() {
		str += fmt.Sprintf("* v: %s\n", v)
	}
	for v1 := range g.Adjacency() {
		for v2, e := range g.Adjacency()[v1] {
			str += fmt.Sprintf("* e: %s -> %s # %s\n", v1, v2, e)
		}
	}
	return
}

func vertexCmpFn(v1, v2 pgraph.Vertex) (bool, error) {
	if v1.String() == "" || v2.String() == "" {
		return false, fmt.Errorf("oops, empty vertex")
	}

	r1, r2 := v1.(engine.Res), v2.(engine.Res)
	if err := r1.Cmp(r2); err != nil {
		//fmt.Printf("r1: %+v\n", *(r1.(*resources.TestRes).Int64Ptr))
		//fmt.Printf("r2: %+v\n", *(r2.(*resources.TestRes).Int64Ptr))
		return false, nil
	}

	return v1.String() == v2.String(), nil
}

func edgeCmpFn(e1, e2 pgraph.Edge) (bool, error) {
	if e1.String() == "" || e2.String() == "" {
		return false, fmt.Errorf("oops, empty edge")
	}
	return e1.String() == e2.String(), nil
}

func runInterpret(t *testing.T, code string) (*pgraph.Graph, error) {
	str := strings.NewReader(code)
	logf := func(format string, v ...interface{}) {
		t.Logf("test: lang: "+format, v...)
	}
	lang := &Lang{
		Input: str, // string as an interface that satisfies io.Reader
		Debug: true,
		Logf:  logf,
	}
	if err := lang.Init(); err != nil {
		return nil, errwrap.Wrapf(err, "init failed")
	}
	closeFn := func() error {
		return errwrap.Wrapf(lang.Close(), "close failed")
	}

	// we only wait for the first event, instead of the continuous stream
	select {
	case err, ok := <-lang.Stream():
		if !ok {
			return nil, errwrap.Wrapf(closeFn(), "stream closed without event")
		}
		if err != nil {
			return nil, errwrap.Wrapf(err, "stream failed, close: %+v", closeFn())
		}
	}

	// run artificially without the entire GAPI loop
	graph, err := lang.Interpret()
	if err != nil {
		err := errwrap.Wrapf(err, "interpret failed")
		if e := closeFn(); e != nil {
			err = multierr.Append(err, e) // list of errors
		}
		return nil, err
	}

	return graph, closeFn()
}

func TestInterpret0(t *testing.T) {
	code := ``
	graph, err := runInterpret(t, code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	expected := &pgraph.Graph{}

	runGraphCmp(t, graph, expected)
}

func TestInterpret1(t *testing.T) {
	code := `noop "n1" {}`
	graph, err := runInterpret(t, code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	n1, _ := engine.NewNamedResource("noop", "n1")

	expected := &pgraph.Graph{}
	expected.AddVertex(n1)

	runGraphCmp(t, graph, expected)
}

func TestInterpret2(t *testing.T) {
	code := `
		noop "n1" {}
		noop "n2" {}
	`
	graph, err := runInterpret(t, code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	n1, _ := engine.NewNamedResource("noop", "n1")
	n2, _ := engine.NewNamedResource("noop", "n2")

	expected := &pgraph.Graph{}
	expected.AddVertex(n1)
	expected.AddVertex(n2)

	runGraphCmp(t, graph, expected)
}

func TestInterpret3(t *testing.T) {
	// should overflow int8
	code := `
		test "t1" {
			int8 => 88888888,
		}
	`
	_, err := runInterpret(t, code)
	if err == nil {
		t.Errorf("expected overflow failure, but it passed")
	}
}

func TestInterpret4(t *testing.T) {
	// str => " !#$%&'()*+,-./0123456790:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\]^_abcdefghijklmnopqrstuvwxyz{|}~",
	code := `
		# comment 1
		test "t1" { # comment 2
			stringptr => " !\"#$%&'()*+,-./0123456790:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_abcdefghijklmnopqrstuvwxyz{|}~",
			int64 => 42,
			boolptr => true,
			# comment 3
			stringptr => "the actual field name is: StringPtr", # comment 4
			int8ptr => 99, # comment 5
			comment => "☺\thello\u263a\nwo\"rld\\2\u263a", # must escape these
		}
	`
	graph, err := runInterpret(t, code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	t1, _ := engine.NewNamedResource("test", "t1")
	x := t1.(*resources.TestRes)
	str := " !\"#$%&'()*+,-./0123456790:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_abcdefghijklmnopqrstuvwxyz{|}~"
	x.StringPtr = &str
	x.Int64 = 42
	b := true
	x.BoolPtr = &b
	stringptr := "the actual field name is: StringPtr"
	x.StringPtr = &stringptr
	int8ptr := int8(99)
	x.Int8Ptr = &int8ptr
	x.Comment = "☺\thello☺\nwo\"rld\\2\u263a" // must escape the escaped chars

	expected := &pgraph.Graph{}
	expected.AddVertex(x)

	runGraphCmp(t, graph, expected)
}

func TestInterpret5(t *testing.T) {
	code := `
		if true {
			test "t1" {
				int64 => 42,
				stringptr => "hello!",
			}
		}
	`
	graph, err := runInterpret(t, code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	t1, _ := engine.NewNamedResource("test", "t1")
	x := t1.(*resources.TestRes)
	x.Int64 = 42
	str := "hello!"
	x.StringPtr = &str

	expected := &pgraph.Graph{}
	expected.AddVertex(x)

	runGraphCmp(t, graph, expected)
}

func TestInterpret6(t *testing.T) {
	code := `
		$b = true
		if $b {
			test "t1" {
				int64 => 42,
				stringptr => "hello",
			}
		}
		if $b {
			test "t2" {
				int64 => 13,
				stringptr => "world",
			}
		}
	`
	graph, err := runInterpret(t, code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	expected := &pgraph.Graph{}

	{
		r, _ := engine.NewNamedResource("test", "t1")
		x := r.(*resources.TestRes)
		x.Int64 = 42
		str := "hello"
		x.StringPtr = &str
		expected.AddVertex(x)
	}
	{
		r, _ := engine.NewNamedResource("test", "t2")
		x := r.(*resources.TestRes)
		x.Int64 = 13
		str := "world"
		x.StringPtr = &str
		expected.AddVertex(x)
	}

	runGraphCmp(t, graph, expected)
}

func TestInterpretMany(t *testing.T) {
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
		graph, _ := pgraph.NewGraph("g")
		testCases = append(testCases, test{ // 1
			name:  "empty",
			code:  ``,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r, _ := engine.NewNamedResource("test", "t")
		x := r.(*resources.TestRes)
		i := int64(42 + 13)
		x.Int64Ptr = &i
		graph.AddVertex(x)
		testCases = append(testCases, test{
			name: "simple addition",
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
		r, _ := engine.NewNamedResource("test", "t")
		x := r.(*resources.TestRes)
		i := int64(42 + 13 + 99)
		x.Int64Ptr = &i
		graph.AddVertex(x)
		testCases = append(testCases, test{
			name: "triple addition",
			code: `
				test "t" {
					int64ptr => 42 + 13 + 99,
				}
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r, _ := engine.NewNamedResource("test", "t")
		x := r.(*resources.TestRes)
		i := int64(42 + 13 - 99)
		x.Int64Ptr = &i
		graph.AddVertex(x)
		testCases = append(testCases, test{
			name: "triple addition/subtraction",
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
		r1, _ := engine.NewNamedResource("test", "t1")
		x1 := r1.(*resources.TestRes)
		s1 := "hello"
		x1.StringPtr = &s1
		graph.AddVertex(x1)
		testCases = append(testCases, test{
			name: "single include",
			code: `
			class c1($a, $b) {
				test $a {
					stringptr => $b,
				}
			}
			include c1("t1", "hello")
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		r2, _ := engine.NewNamedResource("test", "t2")
		x1 := r1.(*resources.TestRes)
		x2 := r2.(*resources.TestRes)
		s1, s2 := "hello", "world"
		x1.StringPtr = &s1
		x2.StringPtr = &s2
		graph.AddVertex(x1, x2)
		testCases = append(testCases, test{
			name: "double include",
			code: `
			class c1($a, $b) {
				test $a {
					stringptr => $b,
				}
			}
			include c1("t1", "hello")
			include c1("t2", "world")
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		//graph, _ := pgraph.NewGraph("g")
		//r1, _ := engine.NewNamedResource("test", "t1")
		//r2, _ := engine.NewNamedResource("test", "t2")
		//x1 := r1.(*resources.TestRes)
		//x2 := r2.(*resources.TestRes)
		//s1, i2 := "hello", int64(42)
		//x1.StringPtr = &s1
		//x2.Int64Ptr = &i2
		//graph.AddVertex(x1, x2)
		testCases = append(testCases, test{
			name: "double include different types error",
			code: `
			class c1($a, $b) {
				if $a == "t1" {
					test $a {
						stringptr => $b,
					}
				} else {
					test $a {
						int64ptr => $b,
					}
				}
			}
			include c1("t1", "hello")
			include c1("t2", 42)
			`,
			fail: true, // should not be able to type check this!
			//graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		r2, _ := engine.NewNamedResource("test", "t2")
		x1 := r1.(*resources.TestRes)
		x2 := r2.(*resources.TestRes)
		s1, s2 := "testing", "testing"
		x1.StringPtr = &s1
		x2.StringPtr = &s2
		graph.AddVertex(x1, x2)
		testCases = append(testCases, test{
			name: "double include different types allowed",
			code: `
			class c1($a, $b) {
				if $b == $b { # for example purposes
					test $a {
						stringptr => "testing",
					}
				}
			}
			include c1("t1", "hello")
			include c1("t2", 42) # this has a different sig
			`,
			fail:  false,
			graph: graph,
		})
	}
	// TODO: add this test once printf supports %v
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	r1, _ := engine.NewNamedResource("test", "t1")
	//	r2, _ := engine.NewNamedResource("test", "t2")
	//	x1 := r1.(*resources.TestRes)
	//	x2 := r2.(*resources.TestRes)
	//	s1, s2 := "value is: hello", "value is: 42"
	//	x1.StringPtr = &s1
	//	x2.StringPtr = &s2
	//	graph.AddVertex(x1, x2)
	//	testCases = append(testCases, test{
	//		name: "double include different printf types allowed",
	//		code: `
	//		import "fmt"
	//		class c1($a, $b) {
	//			test $a {
	//				stringptr => fmt.printf("value is: %v", $b),
	//			}
	//		}
	//		include c1("t1", "hello")
	//		include c1("t2", 42)
	//		`,
	//		fail:  false,
	//		graph: graph,
	//	})
	//}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		r2, _ := engine.NewNamedResource("test", "t2")
		x1 := r1.(*resources.TestRes)
		x2 := r2.(*resources.TestRes)
		s1, s2 := "hey", "hey"
		x1.StringPtr = &s1
		x2.StringPtr = &s2
		graph.AddVertex(x1, x2)
		testCases = append(testCases, test{
			name: "double include with variable in parent scope",
			code: `
			$foo = "hey"
			class c1($a) {
				test $a {
					stringptr => $foo,
				}
			}
			include c1("t1")
			include c1("t2")
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		r2, _ := engine.NewNamedResource("test", "t2")
		x1 := r1.(*resources.TestRes)
		x2 := r2.(*resources.TestRes)
		s1, s2 := "hey", "hey"
		x1.StringPtr = &s1
		x2.StringPtr = &s2
		graph.AddVertex(x1, x2)
		testCases = append(testCases, test{
			name: "double include with out of order variable in parent scope",
			code: `
			include c1("t1")
			include c1("t2")
			class c1($a) {
				test $a {
					stringptr => $foo,
				}
			}
			$foo = "hey"
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		x1 := r1.(*resources.TestRes)
		s1 := "hello"
		x1.StringPtr = &s1
		graph.AddVertex(x1)
		testCases = append(testCases, test{
			name: "duplicate include identical",
			code: `
			include c1("t1", "hello")
			class c1($a, $b) {
				test $a {
					stringptr => $b,
				}
			}
			include c1("t1", "hello") # this is an identical dupe
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		x1 := r1.(*resources.TestRes)
		s1 := "hello"
		x1.StringPtr = &s1
		graph.AddVertex(x1)
		testCases = append(testCases, test{
			name: "duplicate include non-identical",
			code: `
			include c1("t1", "hello")
			class c1($a, $b) {
				if $a == "t1" {
					test $a {
						stringptr => $b,
					}
				} else {
					test "t1" {
						stringptr => $b,
					}
				}
			}
			include c1("t?", "hello") # should cause an identical dupe
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		testCases = append(testCases, test{
			name: "duplicate include incompatible",
			code: `
			include c1("t1", "hello")
			class c1($a, $b) {
				test $a {
					stringptr => $b,
				}
			}
			include c1("t1", "world") # incompatible
			`,
			fail: true, // incompatible resources
		})
	}
	{
		testCases = append(testCases, test{
			name: "class wrong number of args 1",
			code: `
			include c1("hello") # missing second arg
			class c1($a, $b) {
				test $a {
					stringptr => $b,
				}
			}
			`,
			fail: true, // but should NOT panic
		})
	}
	{
		testCases = append(testCases, test{
			name: "class wrong number of args 2",
			code: `
			include c1("hello", 42) # added second arg
			class c1($a) {
				test $a {
					stringptr => "world",
				}
			}
			`,
			fail: true, // but should NOT panic
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r1, _ := engine.NewNamedResource("test", "t1")
		r2, _ := engine.NewNamedResource("test", "t2")
		x1 := r1.(*resources.TestRes)
		x2 := r2.(*resources.TestRes)
		s1, s2 := "hello is 42", "world is 13"
		x1.StringPtr = &s1
		x2.StringPtr = &s2
		graph.AddVertex(x1, x2)
		testCases = append(testCases, test{
			name: "nested classes 1",
			code: `
			import "fmt"

			include c1("t1", "hello") # test["t1"] -> hello is 42
			include c1("t2", "world") # test["t2"] -> world is 13

			class c1($a, $b) {
				# nested class definition
				class c2($c) {
					test $a {
						stringptr => fmt.printf("%s is %d", $b, $c),
					}
				}

				if $a == "t1" {
					include c2(42)
				} else {
					include c2(13)
				}
			}
			`,
			fail:  false,
			graph: graph,
		})
	}
	{
		testCases = append(testCases, test{
			name: "nested classes out of scope 1",
			code: `
			import "fmt"

			include c1("t1", "hello") # test["t1"] -> hello is 42
			include c2(99) # out of scope

			class c1($a, $b) {
				# nested class definition
				class c2($c) {
					test $a {
						stringptr => fmt.printf("%s is %d", $b, $c),
					}
				}

				if $a == "t1" {
					include c2(42)
				} else {
					include c2(13)
				}
			}
			`,
			fail: true,
		})
	}
	// TODO: recursive classes are not currently supported (should they be?)
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	testCases = append(testCases, test{
	//		name: "recursive classes 0",
	//		code: `
	//		include c1(0) # start at zero
	//		class c1($count) {
	//			if $count != 3 {
	//				include c1($count + 1)
	//			}
	//		}
	//		`,
	//		fail:  false,
	//		graph: graph, // produces no output
	//	})
	//}
	// TODO: recursive classes are not currently supported (should they be?)
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	r0, _ := engine.NewNamedResource("test", "done")
	//	x0 := r0.(*resources.TestRes)
	//	s0 := "count is 3"
	//	x0.StringPtr = &s0
	//	graph.AddVertex(x0)
	//	testCases = append(testCases, test{
	//		name: "recursive classes 1",
	//		code: `
	//		import "fmt"
	//		$max = 3
	//		include c1(0) # start at zero
	//		# test["done"] -> count is 3
	//		class c1($count) {
	//			if $count == $max {
	//				test "done" {
	//					stringptr => fmt.printf("count is %d", $count),
	//				}
	//			} else {
	//				include c1($count + 1)
	//			}
	//		}
	//		`,
	//		fail:  false,
	//		graph: graph,
	//	})
	//}
	// TODO: recursive classes are not currently supported (should they be?)
	//{
	//	graph, _ := pgraph.NewGraph("g")
	//	r0, _ := engine.NewNamedResource("test", "zero")
	//	r1, _ := engine.NewNamedResource("test", "ix:1")
	//	r2, _ := engine.NewNamedResource("test", "ix:2")
	//	r3, _ := engine.NewNamedResource("test", "ix:3")
	//	x0 := r0.(*resources.TestRes)
	//	x1 := r1.(*resources.TestRes)
	//	x2 := r2.(*resources.TestRes)
	//	x3 := r3.(*resources.TestRes)
	//	s0, s1, s2, s3 := "count is 0", "count is 1", "count is 2", "count is 3"
	//	x0.StringPtr = &s0
	//	x1.StringPtr = &s1
	//	x2.StringPtr = &s2
	//	x3.StringPtr = &s3
	//	graph.AddVertex(x0, x1, x2, x3)
	//	testCases = append(testCases, test{
	//		name: "recursive classes 2",
	//		code: `
	//		import "fmt"
	//		include c1("ix", 3)
	//		# test["ix:3"] -> count is 3
	//		# test["ix:2"] -> count is 2
	//		# test["ix:1"] -> count is 1
	//		# test["zero"] -> count is 0
	//		class c1($name, $count) {
	//			if $count == 0 {
	//				test "zero" {
	//					stringptr => fmt.printf("count is %d", $count),
	//				}
	//			} else {
	//				include c1($name, $count - 1)
	//				test "${name}:${count}" {
	//					stringptr => fmt.printf("count is %d", $count),
	//				}
	//			}
	//		}
	//		`,
	//		fail:  false,
	//		graph: graph,
	//	})
	//}
	// TODO: remove this test if we ever support recursive classes
	{
		testCases = append(testCases, test{
			name: "recursive classes fail 1",
			code: `
			import "fmt"
			$max = 3
			include c1(0) # start at zero
			class c1($count) {
				if $count == $max {
					test "done" {
						stringptr => fmt.printf("count is %d", $count),
					}
				} else {
					include c1($count + 1) # recursion not supported atm
				}
			}
			`,
			fail: true,
		})
	}
	// TODO: remove this test if we ever support recursive classes
	{
		testCases = append(testCases, test{
			name: "recursive classes fail 2",
			code: `
			import "fmt"
			$max = 5
			include c1(0) # start at zero
			class c1($count) {
				if $count == $max {
					test "done" {
						stringptr => fmt.printf("count is %d", $count),
					}
				} else {
					include c2($count + 1) # recursion not supported atm
				}
			}
			class c2($count) {
				if $count == $max {
					test "done" {
						stringptr => fmt.printf("count is %d", $count),
					}
				} else {
					include c1($count + 1) # recursion not supported atm
				}
			}
			`,
			fail: true,
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

		graph, err := runInterpret(t, code)
		if !fail && err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: runInterpret failed with: %+v", index, err)
			continue
		}
		if fail && err == nil {
			t.Errorf("test #%d: FAIL", index)
			t.Errorf("test #%d: runInterpret passed, expected fail", index)
			continue
		}

		if fail { // can't process graph if it's nil
			continue
		}

		t.Logf("test #%d: graph: %+v", index, graph)
		// TODO: improve: https://github.com/purpleidea/mgmt/issues/199
		if err := graph.GraphCmp(exp, vertexCmpFn, edgeCmpFn); err != nil {
			t.Errorf("test #%d: FAIL", index)
			t.Logf("test #%d:   actual: %v%s", index, graph, fullPrint(graph))
			t.Logf("test #%d: expected: %v%s", index, exp, fullPrint(exp))
			t.Errorf("test #%d: cmp error:\n%v", index, err)
			continue
		}
	}
}
