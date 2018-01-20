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

package lang

import (
	"fmt"
	"strings"
	"testing"

	_ "github.com/purpleidea/mgmt/lang/funcs/facts/core" // load facts
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"

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

	r1, r2 := v1.(resources.Res), v2.(resources.Res)
	if !r1.Compare(r2) {
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

func runInterpret(code string) (*pgraph.Graph, error) {
	str := strings.NewReader(code)
	lang := &Lang{
		Input: str, // string as an interface that satisfies io.Reader
		Debug: true,
	}
	if err := lang.Init(); err != nil {
		return nil, errwrap.Wrapf(err, "init failed")
	}
	closeFn := func() error {
		return errwrap.Wrapf(lang.Close(), "close failed")
	}

	select {
	case err, ok := <-lang.Stream():
		if !ok {
			return nil, errwrap.Wrapf(closeFn(), "stream closed without event")
		}
		if err != nil {
			return nil, errwrap.Wrapf(err, "stream failed, close: %+v", closeFn())
		}
	}

	// run artificially without the entire engine
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
	graph, err := runInterpret(code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	expected := &pgraph.Graph{}

	runGraphCmp(t, graph, expected)
}

func TestInterpret1(t *testing.T) {
	code := `noop "n1" {}`
	graph, err := runInterpret(code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	n1, _ := resources.NewNamedResource("noop", "n1")

	expected := &pgraph.Graph{}
	expected.AddVertex(n1)

	runGraphCmp(t, graph, expected)
}

func TestInterpret2(t *testing.T) {
	code := `
		noop "n1" {}
		noop "n2" {}
	`
	graph, err := runInterpret(code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	n1, _ := resources.NewNamedResource("noop", "n1")
	n2, _ := resources.NewNamedResource("noop", "n2")

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
	_, err := runInterpret(code)
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
	graph, err := runInterpret(code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	t1, _ := resources.NewNamedResource("test", "t1")
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
	graph, err := runInterpret(code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	t1, _ := resources.NewNamedResource("test", "t1")
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
	graph, err := runInterpret(code)
	if err != nil {
		t.Errorf("runInterpret failed: %+v", err)
		return
	}

	expected := &pgraph.Graph{}

	{
		r, _ := resources.NewNamedResource("test", "t1")
		x := r.(*resources.TestRes)
		x.Int64 = 42
		str := "hello"
		x.StringPtr = &str
		expected.AddVertex(x)
	}
	{
		r, _ := resources.NewNamedResource("test", "t2")
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
		graph, _ := pgraph.NewGraph("g")
		values = append(values, test{ // 1
			name:  "empty",
			code:  ``,
			fail:  false,
			graph: graph,
		})
	}
	{
		graph, _ := pgraph.NewGraph("g")
		r, _ := resources.NewNamedResource("test", "t")
		x := r.(*resources.TestRes)
		i := int64(42 + 13)
		x.Int64Ptr = &i
		graph.AddVertex(x)
		values = append(values, test{
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
		r, _ := resources.NewNamedResource("test", "t")
		x := r.(*resources.TestRes)
		i := int64(42 + 13 + 99)
		x.Int64Ptr = &i
		graph.AddVertex(x)
		values = append(values, test{
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
		r, _ := resources.NewNamedResource("test", "t")
		x := r.(*resources.TestRes)
		i := int64(42 + 13 - 99)
		x.Int64Ptr = &i
		graph.AddVertex(x)
		values = append(values, test{
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

		graph, err := runInterpret(code)
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
