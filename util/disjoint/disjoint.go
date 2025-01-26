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

// Package disjoint implements a "disjoint-set data structure", otherwise known
// as the "union–find data structure" which is commonly used for type
// unification, among other things.
//
// You create new elements with the NewElem function, which returns an Elem that
// has both Union and Find methods available. Each Elem can store some typed
// data associated with it, and as a result, this uses golang generics.
//
// The Union method merges two elements into the same set. The Find method picks
// a representative element for that set. Unless you change the contents of a
// set, and in the steady-state of a set, any elements Find method will always
// return the same representative element for that set, and so this can be used
// to build the IsConnected function quite easily.
//
// The Merge and UnsafeMerge functions can be used to run the Union method for
// two elements, with a way to update the data for the representative element in
// the set. This is useful if you want to keep track of a representative piece
// of data for the set, such as the data type of that set if you were using this
// for type unification.
//
// This package does not attempt to be thread-safe, and as a result, make sure
// to wrap this with the synchronization primitives of your choosing.
//
// This package was built by examining wikipedia and other sources, and contains
// some short excerpts from the wikipedia page.
//
// https://en.wikipedia.org/wiki/Disjoint-set_data_structure
package disjoint

// TODO: This package needs more tests!

// NewElem creates a new set with one element and returns the sole element (the
// representative element) of that set.
func NewElem[T any]() *Elem[T] {
	obj := &Elem[T]{}
	obj.parent = obj // initially point to self
	return obj
}

// Elem is the "node" or "element" type for objects contained in the set. It has
// a single Data field which can be used to store some user data.
type Elem[T any] struct {
	// Data is some data that the user might want to store with this element.
	Data T

	// parent is the parent element that we link to. This points to ourself
	// if we are the root (representative element) of the set.
	parent *Elem[T]

	// rank is used for union by rank, a node stores its rank, which is an
	// upper bound for its height. When a node is initialized, its rank is
	// set to zero. To merge trees with roots x and y, first compare their
	// ranks. If the ranks are different, then the larger rank tree becomes
	// the parent, and the ranks of x and y do not change. If the ranks are
	// the same, then either one can become the parent, but the new parent's
	// rank is incremented by one. While the rank of a node is clearly
	// related to its height, storing ranks is more efficient than storing
	// heights. The height of a node can change during a Find operation, so
	// storing ranks avoids the extra effort of keeping the height correct.
	rank int
}

// Union combines two elements into the same set. If the elements are already
// part of the same set, then nothing changes.
func (obj *Elem[T]) Union(elem *Elem[T]) {
	root1 := obj.Find()
	root2 := elem.Find()
	if root1 == root2 {
		return // already part of the same union, do nothing
	}

	// Create the union based on the relative rank's of the two elements.
	// The larger ranked element becomes the new parent. If two elements
	// have the same rank, then we arbitrarily make one of them the new
	// parent. Keep in mind that this third case *does* change the
	// representative value of the set, this expectation of constancy is
	// only true in the steady state.
	switch {
	case root1.rank < root2.rank:
		root1.parent = root2
	case root1.rank > root2.rank:
		root2.parent = root1
	default:
		root1.rank++ // starts at the zero value of 0 if uninitialized
		root2.parent = root1
	}
}

// Find returns the representative element of the set. The same element will
// always be returned when this is called on any element in that same set. You
// can use the IsConnected function to determine if two elements are in the same
// set.
func (obj *Elem[T]) Find() *Elem[T] {
	for obj != obj.parent { // search until we reach the sentinel root value
		// path compression!
		obj.parent = obj.parent.parent
		obj = obj.parent
	}
	return obj
}

// IsConnected returns true if the two elements are part of the same set. Since
// any set must return the same representative element for it, by comparing this
// value for each element, we can determine if they're connected.
func IsConnected[T any](elem1, elem2 *Elem[T]) bool {
	return elem1.Find() == elem2.Find()
}

// Merge is exactly like UnsafeMerge, except it calls Find on each element first
// so that we merge the Data associated with the representative elements only.
// This is almost always what you want.
func Merge[T any](elem1, elem2 *Elem[T], merge func(T, T) (T, error)) error {
	return UnsafeMerge(elem1.Find(), elem2.Find(), merge)
}

// UnsafeMerge runs the Union operation on two elements. Before this happens, it
// runs a merge function which takes the data from each of those elements and
// gives it the opportunity to determine what the new resultant data should be.
// Note that either of the elements passed in might not be the representative
// element for their sets, so if you want to be sure to use the "already merged"
// data, then run Find on each element before passing it to this function. To do
// this automatically, use the regular Merge function instead.
func UnsafeMerge[T any](elem1, elem2 *Elem[T], merge func(T, T) (T, error)) error {
	data, err := merge(elem1.Data, elem2.Data) // compute the merged data
	if err != nil {
		return err
	}

	elem1.Union(elem2)   // union always succeeds
	elem := elem1.Find() // get the new representative element
	elem.Data = data     // store the previously merged data in that element

	return nil
}
