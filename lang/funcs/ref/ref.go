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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package ref implements reference counting for the graph API and function
// engine.
package ref

import (
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Count keeps track of vertex and edge references across the entire graph. Make
// sure to lock access somehow, ideally with the provided Locker interface.
type Count struct {
	// mutex locks this database for read or write.
	mutex *sync.Mutex

	// vertices is a reference count of the number of vertices used.
	vertices map[interfaces.Func]int64

	// edges is a reference count of the number of edges used.
	edges map[*CountEdge]int64 // TODO: hash *CountEdge as a key instead
}

// CountEdge is a virtual "hash" entry for the Count edges map key.
type CountEdge struct {
	f1  interfaces.Func
	f2  interfaces.Func
	arg string
}

// String prints a representation of the references held.
func (obj *Count) String() string {
	s := ""
	s += fmt.Sprintf("vertices (%d):\n", len(obj.vertices))
	for vertex, count := range obj.vertices {
		s += fmt.Sprintf("\tvertex (%d): %p %s\n", count, vertex, vertex)
	}
	s += fmt.Sprintf("edges (%d):\n", len(obj.edges))
	for edge, count := range obj.edges {
		s += fmt.Sprintf("\tedge (%d): %p %s -> %p %s # %s\n", count, edge.f1, edge.f1, edge.f2, edge.f2, edge.arg)
	}
	return s
}

// Init must be called to initialized the struct before first use.
func (obj *Count) Init() *Count {
	obj.mutex = &sync.Mutex{}
	obj.vertices = make(map[interfaces.Func]int64)
	obj.edges = make(map[*CountEdge]int64)
	return obj // return self so it can be called in a chain
}

// Lock the mutex that should be used when reading or writing from this.
func (obj *Count) Lock() { obj.mutex.Lock() }

// Unlock the mutex that should be used when reading or writing from this.
func (obj *Count) Unlock() { obj.mutex.Unlock() }

// VertexInc increments the reference count for the input vertex. It returns
// true if the reference count for this vertex was previously undefined or zero.
// True usually means we'd want to actually add this vertex now. If you attempt
// to increment a vertex which already has a less than zero count, then this
// will panic. This situation is likely impossible unless someone modified the
// reference counting struct directly.
func (obj *Count) VertexInc(f interfaces.Func) bool {
	count, _ := obj.vertices[f]
	obj.vertices[f] = count + 1
	if count == -1 { // unlikely, but catch any bugs
		panic("negative reference count")
	}
	return count == 0
}

// VertexDec decrements the reference count for the input vertex. It returns
// true if the reference count for this vertex is now zero. True usually means
// we'd want to actually remove this vertex now. If you attempt to decrement a
// vertex which already has a zero count, then this will panic.
func (obj *Count) VertexDec(f interfaces.Func) bool {
	count, _ := obj.vertices[f]
	obj.vertices[f] = count - 1
	if count == 0 {
		panic("negative reference count")
	}
	return count == 1 // now it's zero
}

// EdgeInc increments the reference count for the input edge. It adds a
// reference for each arg name in the edge. Since this also increments the
// references for the two input vertices, it returns the corresponding two
// boolean values for these calls. (This function makes two calls to VertexInc.)
func (obj *Count) EdgeInc(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) (bool, bool) {
	for _, arg := range fe.Args { // ref count each arg
		r := obj.makeEdge(f1, f2, arg)
		count := obj.edges[r]
		obj.edges[r] = count + 1
		if count == -1 { // unlikely, but catch any bugs
			panic("negative reference count")
		}
	}

	return obj.VertexInc(f1), obj.VertexInc(f2)
}

// EdgeDec decrements the reference count for the input edge. It removes a
// reference for each arg name in the edge. Since this also decrements the
// references for the two input vertices, it returns the corresponding two
// boolean values for these calls. (This function makes two calls to VertexDec.)
func (obj *Count) EdgeDec(f1, f2 interfaces.Func, fe *interfaces.FuncEdge) (bool, bool) {
	for _, arg := range fe.Args { // ref count each arg
		r := obj.makeEdge(f1, f2, arg)
		count := obj.edges[r]
		obj.edges[r] = count - 1
		if count == 0 {
			panic("negative reference count")
		}
	}

	return obj.VertexDec(f1), obj.VertexDec(f2)
}

// FreeVertex removes exactly one entry from the Vertices list or it errors.
func (obj *Count) FreeVertex(f interfaces.Func) error {
	if count, exists := obj.vertices[f]; !exists || count != 0 {
		return fmt.Errorf("no vertex of count zero found")
	}
	delete(obj.vertices, f)
	return nil
}

// FreeEdge removes exactly one entry from the Edges list or it errors.
func (obj *Count) FreeEdge(f1, f2 interfaces.Func, arg string) error {
	found := []*CountEdge{}
	for k, count := range obj.edges {
		//if k == nil { // programming error
		//	continue
		//}
		if k.f1 == f1 && k.f2 == f2 && k.arg == arg && count == 0 {
			found = append(found, k)
		}
	}
	if len(found) > 1 {
		return fmt.Errorf("inconsistent ref count for edge")
	}
	if len(found) == 0 {
		return fmt.Errorf("no edge of count zero found")
	}
	delete(obj.edges, found[0]) // delete from map
	return nil
}

// GC runs the garbage collector on any zeroed references. Note the distinction
// between count == 0 (please delete now) and absent from the map.
func (obj *Count) GC(graphAPI interfaces.GraphAPI) error {
	// debug
	//fmt.Printf("start refs\n%s", obj.String())
	//defer func() { fmt.Printf("end refs\n%s", obj.String()) }()
	free := make(map[interfaces.Func]map[interfaces.Func][]string) // f1 -> f2
	for x, count := range obj.edges {
		if count != 0 { // we only care about freed things
			continue
		}
		if _, exists := free[x.f1]; !exists {
			free[x.f1] = make(map[interfaces.Func][]string)
		}
		if _, exists := free[x.f1][x.f2]; !exists {
			free[x.f1][x.f2] = []string{}
		}
		free[x.f1][x.f2] = append(free[x.f1][x.f2], x.arg) // exists as refcount zero
	}

	// These edges have a refcount of zero.
	for f1, x := range free {
		for f2, args := range x {
			for _, arg := range args {
				edge := graphAPI.FindEdge(f1, f2)
				// any errors here are programming errors
				if edge == nil {
					return fmt.Errorf("missing edge from %p %s -> %p %s", f1, f1, f2, f2)
				}

				once := false // sanity check
				newArgs := []string{}
				for _, a := range edge.Args {
					if arg == a {
						if once {
							// programming error, duplicate arg
							return fmt.Errorf("duplicate arg (%s) in edge", arg)
						}
						once = true
						continue
					}
					newArgs = append(newArgs, a)
				}

				if len(edge.Args) == 1 { // edge gets deleted
					if a := edge.Args[0]; a != arg { // one arg
						return fmt.Errorf("inconsistent arg: %s != %s", a, arg)
					}

					if err := graphAPI.DeleteEdge(edge); err != nil {
						return errwrap.Wrapf(err, "edge deletion error")
					}
				} else {
					// just remove the one arg for now
					edge.Args = newArgs
				}

				// always free the database entry
				if err := obj.FreeEdge(f1, f2, arg); err != nil {
					return err
				}
			}
		}
	}

	// Now check the vertices...
	vs := []interfaces.Func{}
	for vertex, count := range obj.vertices {
		if count != 0 {
			continue
		}

		// safety check, vertex is still in use by an edge
		for x := range obj.edges {
			if x.f1 == vertex || x.f2 == vertex {
				// programming error
				return fmt.Errorf("vertex unexpectedly still in use: %p %s", vertex, vertex)
			}
		}

		vs = append(vs, vertex)
	}

	for _, vertex := range vs {
		if err := graphAPI.DeleteVertex(vertex); err != nil {
			return errwrap.Wrapf(err, "vertex deletion error")
		}
		// free the database entry
		if err := obj.FreeVertex(vertex); err != nil {
			return err
		}
	}

	return nil
}

// makeEdge looks up an edge with the "hash" input we are seeking. If it doesn't
// find a match, it returns a new one with those fields.
func (obj *Count) makeEdge(f1, f2 interfaces.Func, arg string) *CountEdge {
	for k := range obj.edges {
		//if k == nil { // programming error
		//	continue
		//}
		if k.f1 == f1 && k.f2 == f2 && k.arg == arg {
			return k
		}
	}
	return &CountEdge{ // not found, so make a new one!
		f1:  f1,
		f2:  f2,
		arg: arg,
	}
}
