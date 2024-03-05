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

package txn

import (
	"sync"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"
)

var _ interfaces.GraphAPI = &Graph{} // ensure it meets this expectation

// Graph is a simple pgraph wrapper that implements the GraphAPI interface. That
// interface is also implemented by *dage.Engine and the code is very similar.
type Graph struct {
	Debug bool
	Logf  func(format string, v ...interface{})

	graph      *pgraph.Graph // guarded by graphMutex
	graphMutex *sync.Mutex   // TODO: &sync.RWMutex{} ?
}

// Init initializes the struct before first use and returns it for ergonomics.
func (obj *Graph) Init() *Graph {
	var err error
	obj.graph, err = pgraph.NewGraph("graph")
	if err != nil {
		panic("graph was not built")
	}
	obj.graphMutex = &sync.Mutex{} // TODO: &sync.RWMutex{} ?

	return obj
}

// AddVertex adds a vertex to the graph. It takes a lock.
func (obj *Graph) AddVertex(f interfaces.Func) error {
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		obj.Logf("AddVertex: %p %s", f, f)
	}

	obj.graph.AddVertex(f)
	return nil
}

// AddEdge adds an edge to the graph. It takes a lock.
func (obj *Graph) AddEdge(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) error {
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		obj.Logf("AddEdge %p %s: %p %s -> %p %s", fe, fe, f1, f1, f2, f2)
	}

	// safety check to avoid cycles
	g := obj.graph.Copy()
	g.AddEdge(f1, f2, fe)
	if _, err := g.TopologicalSort(); err != nil {
		return err // not a dag
	}
	// if we didn't cycle, we can modify the real graph safely...

	obj.graph.AddEdge(f1, f2, fe) // replaces any existing edge here

	// This shouldn't error, since the test graph didn't find a cycle.
	if _, err := obj.graph.TopologicalSort(); err != nil {
		// programming error
		panic(err) // not a dag
	}

	return nil
}

// DeleteVertex deletes a vertex from the graph. It takes a lock.
func (obj *Graph) DeleteVertex(f interfaces.Func) error {
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		obj.Logf("DeleteVertex: %p %s", f, f)
	}

	obj.graph.DeleteVertex(f)
	return nil
}

// DeleteEdge deletes an edge from the graph. It takes a lock.
func (obj *Graph) DeleteEdge(fe *interfaces.FuncEdge) error {
	obj.graphMutex.Lock()
	defer obj.graphMutex.Unlock()
	if obj.Debug {
		f1, f2, found := obj.graph.LookupEdge(fe)
		if found {
			obj.Logf("DeleteEdge: %p %s -> %p %s", f1, f1, f2, f2)
		} else {
			obj.Logf("DeleteEdge: not found %p %s", fe, fe)
		}
	}

	// Don't bother checking if edge exists first and don't error if it
	// doesn't because it might have gotten deleted when a vertex did, and
	// so there's no need to complain for nothing.
	obj.graph.DeleteEdge(fe)
	return nil
}

// HasVertex checks if a vertex exists in the graph. It takes a lock.
func (obj *Graph) HasVertex(f interfaces.Func) bool {
	obj.graphMutex.Lock()         // XXX: should this be a RLock?
	defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

	return obj.graph.HasVertex(f)
}

// LookupEdge checks which vertices (if any) exist between an edge in the graph.
// It takes a lock.
func (obj *Graph) LookupEdge(fe *interfaces.FuncEdge) (interfaces.Func, interfaces.Func, bool) {
	obj.graphMutex.Lock()         // XXX: should this be a RLock?
	defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

	v1, v2, found := obj.graph.LookupEdge(fe)
	if !found {
		return nil, nil, found
	}
	f1, ok := v1.(interfaces.Func)
	if !ok {
		panic("not a Func")
	}
	f2, ok := v2.(interfaces.Func)
	if !ok {
		panic("not a Func")
	}
	return f1, f2, found
}

// FindEdge checks which edge (if any) exists between two vertices in the graph.
// It takes a lock. This is an important method in edge removal, because it's
// what you really need to know for DeleteEdge to work. Requesting a specific
// deletion isn't very sensical in this library when specified as the edge
// pointer, since we might replace it with a new edge that has new arg names.
// Instead, use this to look up what relationship you want, and then DeleteEdge
// to remove it.
func (obj *Graph) FindEdge(f1, f2 interfaces.Func) *interfaces.FuncEdge {
	obj.graphMutex.Lock()         // XXX: should this be a RLock?
	defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

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

// Graph returns a copy of the contained graph. It takes a lock.
func (obj *Graph) Graph() *pgraph.Graph {
	obj.graphMutex.Lock()         // XXX: should this be a RLock?
	defer obj.graphMutex.Unlock() // XXX: should this be an RUnlock?

	return obj.graph.Copy()
}
