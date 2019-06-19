// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package pgraph

import (
    "testing"
//	"fmt"
//	"io/ioutil"
)

// based on pgraph_test.go TestAddVertex() - line 54
func TestGraphViz1(t *testing.T){
	t.Logf("what up")
    G := &Graph{Name: "g2"}
	v1 := NV("v1")
    v2 := NV("v2")
    v3 := NV("v3")
    v4 := NV("v4")
    v5 := NV("v5")
    v6 := NV("v6")
    e1 := NE("e1")
    e2 := NE("e2")
    e3 := NE("e3")
    e4 := NE("e4")
    e5 := NE("e5")
    e6 := NE("e6")

	G.AddEdge(v1, v2, e1)
    G.AddEdge(v2, v3, e2)
    G.AddEdge(v3, v1, e3)

    G.AddEdge(v4, v5, e4)
    G.AddEdge(v5, v6, e5)
	G.AddEdge(v6, v4, e6)

	var expectedResult = [...]string{
        "digraph "g2" {",
        "	label=\"g2\";",
        "	\"0xc00004a510\" [label=\"v1\"];",
        "	\"0xc00004a520\" [label=\"v2\"];",
        "	\"0xc00004a530\" [label=\"v3\"];",
        "	\"0xc00004a540\" [label=\"v4\"];",
        "	\"0xc00004a550\" [label=\"v5\"];",
        "	\"0xc00004a560\" [label=\"v6\"];",
        "	\"0xc00004a510\" -> \"0xc00004a520\" [label=\"e1\"];",
        "	\"0xc00004a520\" -> \"0xc00004a530\" [label=\"e2\"];",
        "	\"0xc00004a530\" -> \"0xc00004a510\" [label=\"e3\"];",
        "	\"0xc00004a540\" -> \"0xc00004a550\" [label=\"e4\"];",
        "	\"0xc00004a550\" -> \"0xc00004a560\" [label=\"e5\"];",
        "	\"0xc00004a560\" -> \"0xc00004a540\" [label=\"e6\"];",
        "}"
    }
    var generated = strings.Split(G.Graphviz(), "\n")

    // if the first line is the same, good!
    // We remove this line for future comparison.
    if(expectedResult[0] == generated[0]){
        // TODO, realistically I don't think this is a smart idea.
    }

    //t.Logf(G.Graphviz())
	//t.Logf(expectedResult)
//
//	mydata := []byte(G.Graphviz())
//
//    // the WriteFile method returns an error if unsuccessful
//    err := ioutil.WriteFile("myfile.data", mydata, 0777)
//    // handle this error
//    if err != nil {
//        // print it out
//        fmt.Println(err)
//    }
//
//    data, err := ioutil.ReadFile("myfile.data")
//    if err != nil {
//        fmt.Println(err)
//    }
//
//    fmt.Print(string(data))



	if (G.Graphviz() != expectedResult){
		t.Error("Conversion to graphviz format done incorrectly")
	}
}
