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

package disjoint

import (
	"sort"
	"testing"
)

func TestUnionFind0(t *testing.T) {
	s1 := NewElem[bool]()
	s2 := NewElem[bool]()
	s3 := NewElem[bool]()

	s1.Union(s2)

	f1 := s1.Find()
	f2 := s2.Find()
	f3 := s3.Find()

	if f1 != f2 || !IsConnected(f1, f2) {
		t.Errorf("f1 and f2 are not in the same set")
	}
	if f2 == f3 || IsConnected(f2, f3) {
		t.Errorf("f1 and f2 should not be in the same set")
	}
}

func TestMerge0(t *testing.T) {
	s1 := NewElem[[]string]()
	s1.Data = []string{"a"}
	s2 := NewElem[[]string]()
	s2.Data = []string{"b"}
	s3 := NewElem[[]string]()
	s3.Data = []string{"c"}

	//s1.Union(s2)
	//s2.Union(s3)
	merge := func(a, b []string) ([]string, error) {
		t.Logf("merge: `%s` and `%s`", a, b)
		c := []string{}
		c = append(c, a...)
		c = append(c, b...)
		return c, nil
	}
	if err := UnsafeMerge(s1, s2, merge); err != nil {
		t.Errorf("merge error: %v", err)
		return
	}

	// If this is UnsafeMerge, we should correctly fail!
	if err := Merge(s2, s3, merge); err != nil {
		t.Errorf("merge error: %v", err)
		return
	}

	x := s3.Find().Data
	sort.Strings(x)

	// TODO: use compare in golang 1.21
	//if !slices.Compare(x, []string{"a", "b", "c"}) {
	//	t.Errorf("wrong data")
	//}
	if x[0] != "a" || x[1] != "b" || x[2] != "c" {
		t.Errorf("wrong data, got: %v", x)
	}
}
