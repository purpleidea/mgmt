// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

//go:build !root

package txn

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs/ref"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
)

type testGraphAPI struct {
	graph *pgraph.Graph
}

func (obj *testGraphAPI) AddVertex(f interfaces.Func) error {
	v, ok := f.(pgraph.Vertex)
	if !ok {
		return fmt.Errorf("can't use func as vertex")
	}
	obj.graph.AddVertex(v)
	return nil
}
func (obj *testGraphAPI) AddEdge(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) error {
	v1, ok := f1.(pgraph.Vertex)
	if !ok {
		return fmt.Errorf("can't use func as vertex")
	}
	v2, ok := f2.(pgraph.Vertex)
	if !ok {
		return fmt.Errorf("can't use func as vertex")
	}
	obj.graph.AddEdge(v1, v2, fe)
	return nil
}

func (obj *testGraphAPI) DeleteVertex(f interfaces.Func) error {
	v, ok := f.(pgraph.Vertex)
	if !ok {
		return fmt.Errorf("can't use func as vertex")
	}
	obj.graph.DeleteVertex(v)
	return nil
}

func (obj *testGraphAPI) DeleteEdge(fe *interfaces.FuncEdge) error {
	obj.graph.DeleteEdge(fe)
	return nil
}

//func (obj *testGraphAPI) AddGraph(*pgraph.Graph) error {
//	return fmt.Errorf("not implemented")
//}

//func (obj *testGraphAPI) Adjacency() map[interfaces.Func]map[interfaces.Func]*interfaces.FuncEdge {
//	panic("not implemented")
//}

func (obj *testGraphAPI) HasVertex(f interfaces.Func) bool {
	v, ok := f.(pgraph.Vertex)
	if !ok {
		panic("can't use func as vertex")
	}
	return obj.graph.HasVertex(v)
}

func (obj *testGraphAPI) LookupEdge(fe *interfaces.FuncEdge) (interfaces.Func, interfaces.Func, bool) {
	v1, v2, b := obj.graph.LookupEdge(fe)
	if !b {
		return nil, nil, b
	}

	f1, ok := v1.(interfaces.Func)
	if !ok {
		panic("can't use vertex as func")
	}
	f2, ok := v2.(interfaces.Func)
	if !ok {
		panic("can't use vertex as func")
	}
	return f1, f2, true
}

func (obj *testGraphAPI) FindEdge(f1, f2 interfaces.Func) *interfaces.FuncEdge {
	edge := obj.graph.FindEdge(f1, f2)
	if edge == nil {
		return nil
	}
	fe, ok := edge.(*interfaces.FuncEdge)
	if !ok {
		panic("edge is not a FuncEdge")
	}

	return fe
}

func (obj *testGraphAPI) Graph() *pgraph.Graph {
	return obj.graph.Copy()
}

type testNullFunc struct {
	name string
}

func (obj *testNullFunc) String() string               { return obj.name }
func (obj *testNullFunc) Info() *interfaces.Info       { return nil }
func (obj *testNullFunc) Validate() error              { return nil }
func (obj *testNullFunc) Init(*interfaces.Init) error  { return nil }
func (obj *testNullFunc) Stream(context.Context) error { return nil }

func TestTxn1(t *testing.T) {
	graph, err := pgraph.NewGraph("test")
	if err != nil {
		t.Errorf("err: %+v", err)
		return
	}
	testGraphAPI := &testGraphAPI{graph: graph}
	mutex := &sync.Mutex{}

	graphTxn := &GraphTxn{
		GraphAPI: testGraphAPI,
		Lock:     mutex.Lock,
		Unlock:   mutex.Unlock,
		RefCount: (&ref.Count{}).Init(),
	}
	txn := graphTxn.Init()

	f1 := &testNullFunc{"f1"}

	if err := txn.AddVertex(f1).Commit(); err != nil {
		t.Errorf("commit err: %+v", err)
		return
	}

	if l, i := len(graph.Adjacency()), 1; l != i {
		t.Errorf("got len of: %d", l)
		t.Errorf("exp len of: %d", i)
		return
	}

	if err := txn.Reverse(); err != nil {
		t.Errorf("reverse err: %+v", err)
		return
	}

	if l, i := len(graph.Adjacency()), 0; l != i {
		t.Errorf("got len of: %d", l)
		t.Errorf("exp len of: %d", i)
		return
	}
}

type txnTestOp func(*pgraph.Graph, interfaces.Txn) error

func TestTxnTable(t *testing.T) {

	type test struct { // an individual test
		name    string
		actions []txnTestOp
	}
	testCases := []test{}
	{
		f1 := &testNullFunc{"f1"}

		testCases = append(testCases, test{
			name: "simple add vertex",
			actions: []txnTestOp{
				//func(g *pgraph.Graph, txn interfaces.Txn) error {
				//	txn.AddVertex(f1)
				//	return nil
				//},
				//func(g *pgraph.Graph, txn interfaces.Txn) error {
				//	return txn.Commit()
				//},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.AddVertex(f1).Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 1; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Reverse()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 0; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testNullFunc{"f1"}
		f2 := &testNullFunc{"f2"}
		e1 := testEdge("e1")

		testCases = append(testCases, test{
			name: "simple add edge",
			actions: []txnTestOp{
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.AddEdge(f1, f2, e1).Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 2; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 1; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Reverse()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 0; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 0; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testNullFunc{"f1"}
		f2 := &testNullFunc{"f2"}
		e1 := testEdge("e1")

		testCases = append(testCases, test{
			name: "simple add edge two-step",
			actions: []txnTestOp{
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.AddVertex(f1).Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.AddEdge(f1, f2, e1).Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 2; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 1; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				// Reverse only undoes what happened since the
				// previous commit, so only one of the nodes is
				// left at the end.
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Reverse()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 1; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 0; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testNullFunc{"f1"}
		f2 := &testNullFunc{"f2"}
		e1 := testEdge("e1")

		testCases = append(testCases, test{
			name: "simple two add edge, reverse",
			actions: []txnTestOp{
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.AddVertex(f1).AddVertex(f2).Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.AddEdge(f1, f2, e1).Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 2; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 1; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				// Reverse only undoes what happened since the
				// previous commit, so only one of the nodes is
				// left at the end.
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Reverse()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 2; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 0; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
			},
		})
	}
	{
		f1 := &testNullFunc{"f1"}
		f2 := &testNullFunc{"f2"}
		f3 := &testNullFunc{"f3"}
		f4 := &testNullFunc{"f4"}
		e1 := testEdge("e1")
		e2 := testEdge("e2")
		e3 := testEdge("e3")
		e4 := testEdge("e4")

		testCases = append(testCases, test{
			name: "simple add/delete",
			actions: []txnTestOp{
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					txn.AddVertex(f1).AddEdge(f1, f2, e1)
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					txn.AddVertex(f1).AddEdge(f1, f3, e2)
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 3; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 2; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					txn.AddEdge(f2, f4, e3)
					txn.AddEdge(f3, f4, e4)
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Commit()
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 4; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 4; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				// debug
				//func(g *pgraph.Graph, txn interfaces.Txn) error {
				//	fileName := "/tmp/graphviz-txn1.dot"
				//	if err := g.ExecGraphviz(fileName); err != nil {
				//		return fmt.Errorf("writing graph failed at: %s, err: %+v", fileName, err)
				//	}
				//	return nil
				//},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					return txn.Reverse()
				},
				// debug
				//func(g *pgraph.Graph, txn interfaces.Txn) error {
				//	fileName := "/tmp/graphviz-txn2.dot"
				//	if err := g.ExecGraphviz(fileName); err != nil {
				//		return fmt.Errorf("writing graph failed at: %s, err: %+v", fileName, err)
				//	}
				//	return nil
				//},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Adjacency()), 3; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
				func(g *pgraph.Graph, txn interfaces.Txn) error {
					if l, i := len(g.Edges()), 2; l != i {
						return fmt.Errorf("got len of: %d, exp len of: %d", l, i)
					}
					return nil
				},
			},
		})
	}
	if testing.Short() {
		t.Logf("available tests:")
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
		//if tc.name != "simple txn" {
		//	continue
		//}

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			name, actions := tc.name, tc.actions

			t.Logf("\n\ntest #%d (%s) ----------------\n\n", index, name)

			//logf := func(format string, v ...interface{}) {
			//	t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			//}

			graph, err := pgraph.NewGraph("test")
			if err != nil {
				t.Errorf("err: %+v", err)
				return
			}
			testGraphAPI := &testGraphAPI{graph: graph}
			mutex := &sync.Mutex{}

			graphTxn := &GraphTxn{
				GraphAPI: testGraphAPI,
				Lock:     mutex.Lock,
				Unlock:   mutex.Unlock,
				RefCount: (&ref.Count{}).Init(),
			}
			txn := graphTxn.Init()

			// Run a list of actions, passing the returned txn (if
			// any) to the next action. Any error kills it all.
			for i, action := range actions {
				if err := action(graph, txn); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: action #%d failed with: %+v", index, i, err)
					return
				}
			}
		})
	}

	if testing.Short() {
		t.Skip("skipping all tests...")
	}
}
