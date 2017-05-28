// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
)

// NE is a helper function to make testing easier. It creates a new noop edge.
func NE(s string) pgraph.Edge {
	obj := &Edge{Name: s}
	return obj
}

type testGrouper struct {
	// TODO: this algorithm may not be correct in all cases. replace if needed!
	NonReachabilityGrouper // "inherit" what we want, and reimplement the rest
}

func (ag *testGrouper) name() string {
	return "testGrouper"
}

func (ag *testGrouper) vertexMerge(v1, v2 pgraph.Vertex) (v pgraph.Vertex, err error) {
	if err := VtoR(v1).GroupRes(VtoR(v2)); err != nil { // group them first
		return nil, err
	}
	// HACK: update the name so it matches full list of self+grouped
	obj := VtoR(v1)
	names := strings.Split(obj.GetName(), ",") // load in stored names
	for _, n := range obj.GetGroup() {
		names = append(names, n.GetName()) // add my contents
	}
	names = util.StrRemoveDuplicatesInList(names) // remove duplicates
	sort.Strings(names)
	obj.SetName(strings.Join(names, ","))
	return // success or fail, and no need to merge the actual vertices!
}

func (ag *testGrouper) edgeMerge(e1, e2 pgraph.Edge) pgraph.Edge {
	edge1 := e1.(*Edge) // panic if wrong
	edge2 := e2.(*Edge) // panic if wrong
	// HACK: update the name so it makes a union of both names
	n1 := strings.Split(edge1.Name, ",") // load
	n2 := strings.Split(edge2.Name, ",") // load
	names := append(n1, n2...)
	names = util.StrRemoveDuplicatesInList(names) // remove duplicates
	sort.Strings(names)
	return &Edge{Name: strings.Join(names, ",")}
}

// helper function
func runGraphCmp(t *testing.T, g1, g2 *pgraph.Graph) {
	AutoGroup(g1, &testGrouper{}) // edits the graph
	err := GraphCmp(g1, g2)
	if err != nil {
		t.Logf("  actual (g1): %v%v", g1, fullPrint(g1))
		t.Logf("expected (g2): %v%v", g2, fullPrint(g2))
		t.Logf("Cmp error:")
		t.Errorf("%v", err)
	}
}

type NoopResTest struct {
	NoopRes
}

func (obj *NoopResTest) GroupCmp(r Res) bool {
	res, ok := r.(*NoopResTest)
	if !ok {
		return false
	}

	// TODO: implement this in vertexCmp for *testGrouper instead?
	if strings.Contains(res.Name, ",") { // HACK
		return false // element to be grouped is already grouped!
	}

	// group if they start with the same letter! (helpful hack for testing)
	return obj.Name[0] == res.Name[0]
}

func NewNoopResTest(name string) *NoopResTest {
	obj := &NoopResTest{
		NoopRes: NoopRes{
			BaseRes: BaseRes{
				Name: name,
				MetaParams: MetaParams{
					AutoGroup: true, // always autogroup
				},
			},
		},
	}
	return obj
}

// GraphCmp compares the topology of two graphs and returns nil if they're equal
// It also compares if grouped element groups are identical
func GraphCmp(g1, g2 *pgraph.Graph) error {
	if n1, n2 := g1.NumVertices(), g2.NumVertices(); n1 != n2 {
		return fmt.Errorf("graph g1 has %d vertices, while g2 has %d", n1, n2)
	}
	if e1, e2 := g1.NumEdges(), g2.NumEdges(); e1 != e2 {
		return fmt.Errorf("graph g1 has %d edges, while g2 has %d", e1, e2)
	}

	var m = make(map[pgraph.Vertex]pgraph.Vertex) // g1 to g2 vertex correspondence
Loop:
	// check vertices
	for v1 := range g1.Adjacency() { // for each vertex in g1

		l1 := strings.Split(VtoR(v1).GetName(), ",") // make list of everyone's names...
		for _, x1 := range VtoR(v1).GetGroup() {
			l1 = append(l1, x1.GetName()) // add my contents
		}
		l1 = util.StrRemoveDuplicatesInList(l1) // remove duplicates
		sort.Strings(l1)

		// inner loop
		for v2 := range g2.Adjacency() { // does it match in g2 ?

			l2 := strings.Split(VtoR(v2).GetName(), ",")
			for _, x2 := range VtoR(v2).GetGroup() {
				l2 = append(l2, x2.GetName())
			}
			l2 = util.StrRemoveDuplicatesInList(l2) // remove duplicates
			sort.Strings(l2)

			// does l1 match l2 ?
			if ListStrCmp(l1, l2) { // cmp!
				m[v1] = v2
				continue Loop
			}
		}
		return fmt.Errorf("graph g1, has no match in g2 for: %v", VtoR(v1).GetName())
	}
	// vertices (and groups) match :)

	// check edges
	for v1 := range g1.Adjacency() { // for each vertex in g1
		v2 := m[v1] // lookup in map to get correspondance
		// g1.Adjacency()[v1] corresponds to g2.Adjacency()[v2]
		if e1, e2 := len(g1.Adjacency()[v1]), len(g2.Adjacency()[v2]); e1 != e2 {
			return fmt.Errorf("graph g1, vertex(%v) has %d edges, while g2, vertex(%v) has %d", VtoR(v1).GetName(), e1, VtoR(v2).GetName(), e2)
		}

		for vv1, ee1 := range g1.Adjacency()[v1] {
			vv2 := m[vv1]
			ee1 := ee1.(*Edge)
			ee2 := g2.Adjacency()[v2][vv2].(*Edge)

			// these are edges from v1 -> vv1 via ee1 (graph 1)
			// to cmp to edges from v2 -> vv2 via ee2 (graph 2)

			// check: (1) vv1 == vv2 ? (we've already checked this!)
			l1 := strings.Split(VtoR(vv1).GetName(), ",") // make list of everyone's names...
			for _, x1 := range VtoR(vv1).GetGroup() {
				l1 = append(l1, x1.GetName()) // add my contents
			}
			l1 = util.StrRemoveDuplicatesInList(l1) // remove duplicates
			sort.Strings(l1)

			l2 := strings.Split(VtoR(vv2).GetName(), ",")
			for _, x2 := range VtoR(vv2).GetGroup() {
				l2 = append(l2, x2.GetName())
			}
			l2 = util.StrRemoveDuplicatesInList(l2) // remove duplicates
			sort.Strings(l2)

			// does l1 match l2 ?
			if !ListStrCmp(l1, l2) { // cmp!
				return fmt.Errorf("graph g1 and g2 don't agree on: %v and %v", VtoR(vv1).GetName(), VtoR(vv2).GetName())
			}

			// check: (2) ee1 == ee2
			if ee1.Name != ee2.Name {
				return fmt.Errorf("graph g1 edge(%v) doesn't match g2 edge(%v)", ee1.Name, ee2.Name)
			}
		}
	}

	// check meta parameters
	for v1 := range g1.Adjacency() { // for each vertex in g1
		for v2 := range g2.Adjacency() { // does it match in g2 ?
			s1, s2 := VtoR(v1).Meta().Sema, VtoR(v2).Meta().Sema
			sort.Strings(s1)
			sort.Strings(s2)
			if !reflect.DeepEqual(s1, s2) {
				return fmt.Errorf("vertex %s and vertex %s have different semaphores", VtoR(v1).GetName(), VtoR(v2).GetName())
			}
		}
	}

	return nil // success!
}

// ListStrCmp compares two lists of strings
func ListStrCmp(a, b []string) bool {
	//fmt.Printf("CMP: %v with %v\n", a, b) // debugging
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fullPrint(g *pgraph.Graph) (str string) {
	str += "\n"
	for v := range g.Adjacency() {
		if semas := VtoR(v).Meta().Sema; len(semas) > 0 {
			str += fmt.Sprintf("* v: %v; sema: %v\n", VtoR(v).GetName(), semas)
		} else {
			str += fmt.Sprintf("* v: %v\n", VtoR(v).GetName())
		}
		// TODO: add explicit grouping data?
	}
	for v1 := range g.Adjacency() {
		for v2, e := range g.Adjacency()[v1] {
			edge := e.(*Edge)
			str += fmt.Sprintf("* e: %v -> %v # %v\n", VtoR(v1).GetName(), VtoR(v2).GetName(), edge.Name)
		}
	}
	return
}

func TestDurationAssumptions(t *testing.T) {
	var d time.Duration
	if (d == 0) != true {
		t.Errorf("empty time.Duration is no longer equal to zero")
	}
	if (d > 0) != false {
		t.Errorf("empty time.Duration is now greater than zero")
	}
}

// all of the following test cases are laid out with the following semantics:
// * vertices which start with the same single letter are considered "like"
// * "like" elements should be merged
// * vertices can have any integer after their single letter "family" type
// * grouped vertices should have a name with a comma separated list of names
// * edges follow the same conventions about grouping

// empty graph
func TestPgraphGrouping1(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	g2, _ := pgraph.NewGraph("g2") // expected result
	runGraphCmp(t, g1, g2)
}

// single vertex
func TestPgraphGrouping2(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{                              // grouping to limit variable scope
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		g1.AddVertex(a1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		g2.AddVertex(a1)
	}
	runGraphCmp(t, g1, g2)
}

// two vertices
func TestPgraphGrouping3(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		g1.AddVertex(a1, b1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		g2.AddVertex(a1, b1)
	}
	runGraphCmp(t, g1, g2)
}

// two vertices merge
func TestPgraphGrouping4(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		g1.AddVertex(a1, a2)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// three vertices merge
func TestPgraphGrouping5(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		a3 := pgraph.NewVertex(NewNoopResTest("a3"))
		g1.AddVertex(a1, a2, a3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2,a3"))
		g2.AddVertex(a)
	}
	runGraphCmp(t, g1, g2)
}

// three vertices, two merge
func TestPgraphGrouping6(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		g1.AddVertex(a1, a2, b1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		g2.AddVertex(a, b1)
	}
	runGraphCmp(t, g1, g2)
}

// four vertices, three merge
func TestPgraphGrouping7(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		a3 := pgraph.NewVertex(NewNoopResTest("a3"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		g1.AddVertex(a1, a2, a3, b1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2,a3"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		g2.AddVertex(a, b1)
	}
	runGraphCmp(t, g1, g2)
}

// four vertices, two&two merge
func TestPgraphGrouping8(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		b2 := pgraph.NewVertex(NewNoopResTest("b2"))
		g1.AddVertex(a1, a2, b1, b2)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b := pgraph.NewVertex(NewNoopResTest("b1,b2"))
		g2.AddVertex(a, b)
	}
	runGraphCmp(t, g1, g2)
}

// five vertices, two&three merge
func TestPgraphGrouping9(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		b2 := pgraph.NewVertex(NewNoopResTest("b2"))
		b3 := pgraph.NewVertex(NewNoopResTest("b3"))
		g1.AddVertex(a1, a2, b1, b2, b3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b := pgraph.NewVertex(NewNoopResTest("b1,b2,b3"))
		g2.AddVertex(a, b)
	}
	runGraphCmp(t, g1, g2)
}

// three unique vertices
func TestPgraphGrouping10(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		g1.AddVertex(a1, b1, c1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		g2.AddVertex(a1, b1, c1)
	}
	runGraphCmp(t, g1, g2)
}

// three unique vertices, two merge
func TestPgraphGrouping11(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		b2 := pgraph.NewVertex(NewNoopResTest("b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		g1.AddVertex(a1, b1, b2, c1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b := pgraph.NewVertex(NewNoopResTest("b1,b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		g2.AddVertex(a1, b, c1)
	}
	runGraphCmp(t, g1, g2)
}

// simple merge 1
// a1   a2         a1,a2
//   \ /     >>>     |     (arrows point downwards)
//    b              b
func TestPgraphGrouping12(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		e1 := NE("e1")
		e2 := NE("e2")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a2, b1, e2)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		e := NE("e1,e2")
		g2.AddEdge(a, b1, e)
	}
	runGraphCmp(t, g1, g2)
}

// simple merge 2
//    b              b
//   / \     >>>     |     (arrows point downwards)
// a1   a2         a1,a2
func TestPgraphGrouping13(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		e1 := NE("e1")
		e2 := NE("e2")
		g1.AddEdge(b1, a1, e1)
		g1.AddEdge(b1, a2, e2)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		e := NE("e1,e2")
		g2.AddEdge(b1, a, e)
	}
	runGraphCmp(t, g1, g2)
}

// triple merge
// a1 a2  a3         a1,a2,a3
//   \ | /     >>>       |      (arrows point downwards)
//     b                 b
func TestPgraphGrouping14(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		a3 := pgraph.NewVertex(NewNoopResTest("a3"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		e1 := NE("e1")
		e2 := NE("e2")
		e3 := NE("e3")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a2, b1, e2)
		g1.AddEdge(a3, b1, e3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2,a3"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		e := NE("e1,e2,e3")
		g2.AddEdge(a, b1, e)
	}
	runGraphCmp(t, g1, g2)
}

// chain merge
//    a1             a1
//   /  \             |
// b1    b2   >>>   b1,b2   (arrows point downwards)
//   \  /             |
//    c1             c1
func TestPgraphGrouping15(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		b2 := pgraph.NewVertex(NewNoopResTest("b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1")
		e2 := NE("e2")
		e3 := NE("e3")
		e4 := NE("e4")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(a1, b2, e2)
		g1.AddEdge(b1, c1, e3)
		g1.AddEdge(b2, c1, e4)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b := pgraph.NewVertex(NewNoopResTest("b1,b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1,e2")
		e2 := NE("e3,e4")
		g2.AddEdge(a1, b, e1)
		g2.AddEdge(b, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// re-attach 1 (outer)
// technically the second possibility is valid too, depending on which order we
// merge edges in, and if we don't filter out any unnecessary edges afterwards!
// a1    a2         a1,a2        a1,a2
//  |   /             |            |  \
// b1  /      >>>    b1     OR    b1  /   (arrows point downwards)
//  | /               |            | /
// c1                c1           c1
func TestPgraphGrouping16(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1")
		e2 := NE("e2")
		e3 := NE("e3")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(b1, c1, e2)
		g1.AddEdge(a2, c1, e3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1,e3")
		e2 := NE("e2,e3") // e3 gets "merged through" to BOTH edges!
		g2.AddEdge(a, b1, e1)
		g2.AddEdge(b1, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// re-attach 2 (inner)
// a1    b2          a1
//  |   /             |
// b1  /      >>>   b1,b2   (arrows point downwards)
//  | /               |
// c1                c1
func TestPgraphGrouping17(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		b2 := pgraph.NewVertex(NewNoopResTest("b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1")
		e2 := NE("e2")
		e3 := NE("e3")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(b1, c1, e2)
		g1.AddEdge(b2, c1, e3)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b := pgraph.NewVertex(NewNoopResTest("b1,b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1")
		e2 := NE("e2,e3")
		g2.AddEdge(a1, b, e1)
		g2.AddEdge(b, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// re-attach 3 (double)
// similar to "re-attach 1", technically there is a second possibility for this
// a2   a1    b2         a1,a2
//   \   |   /             |
//    \ b1  /      >>>   b1,b2   (arrows point downwards)
//     \ | /               |
//      c1                c1
func TestPgraphGrouping18(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		b1 := pgraph.NewVertex(NewNoopResTest("b1"))
		b2 := pgraph.NewVertex(NewNoopResTest("b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1")
		e2 := NE("e2")
		e3 := NE("e3")
		e4 := NE("e4")
		g1.AddEdge(a1, b1, e1)
		g1.AddEdge(b1, c1, e2)
		g1.AddEdge(a2, c1, e3)
		g1.AddEdge(b2, c1, e4)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result
	{
		a := pgraph.NewVertex(NewNoopResTest("a1,a2"))
		b := pgraph.NewVertex(NewNoopResTest("b1,b2"))
		c1 := pgraph.NewVertex(NewNoopResTest("c1"))
		e1 := NE("e1,e3")
		e2 := NE("e2,e3,e4") // e3 gets "merged through" to BOTH edges!
		g2.AddEdge(a, b, e1)
		g2.AddEdge(b, c1, e2)
	}
	runGraphCmp(t, g1, g2)
}

// connected merge 0, (no change!)
// a1            a1
//   \     >>>     \     (arrows point downwards)
//    a2            a2
func TestPgraphGroupingConnected0(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		e1 := NE("e1")
		g1.AddEdge(a1, a2, e1)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result ?
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		e1 := NE("e1")
		g2.AddEdge(a1, a2, e1)
	}
	runGraphCmp(t, g1, g2)
}

// connected merge 1, (no change!)
// a1              a1
//   \               \
//    b      >>>      b      (arrows point downwards)
//     \               \
//      a2              a2
func TestPgraphGroupingConnected1(t *testing.T) {
	g1, _ := pgraph.NewGraph("g1") // original graph
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b := pgraph.NewVertex(NewNoopResTest("b"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		e1 := NE("e1")
		e2 := NE("e2")
		g1.AddEdge(a1, b, e1)
		g1.AddEdge(b, a2, e2)
	}
	g2, _ := pgraph.NewGraph("g2") // expected result ?
	{
		a1 := pgraph.NewVertex(NewNoopResTest("a1"))
		b := pgraph.NewVertex(NewNoopResTest("b"))
		a2 := pgraph.NewVertex(NewNoopResTest("a2"))
		e1 := NE("e1")
		e2 := NE("e2")
		g2.AddEdge(a1, b, e1)
		g2.AddEdge(b, a2, e2)
	}
	runGraphCmp(t, g1, g2)
}
