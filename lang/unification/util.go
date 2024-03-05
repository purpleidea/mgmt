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

package unification

import (
	"github.com/purpleidea/mgmt/lang/interfaces"
)

// ExprListToExprMap converts a list of expressions to a map that has the unique
// expr pointers as the keys. This is just an alternate representation of the
// same data structure. If you have any duplicate values in your list, they'll
// get removed when stored as a map.
func ExprListToExprMap(exprList []interfaces.Expr) map[interfaces.Expr]struct{} {
	exprMap := make(map[interfaces.Expr]struct{})
	for _, x := range exprList {
		exprMap[x] = struct{}{}
	}
	return exprMap
}

// ExprMapToExprList converts a map of expressions to a list that has the unique
// expr pointers as the values. This is just an alternate representation of the
// same data structure.
func ExprMapToExprList(exprMap map[interfaces.Expr]struct{}) []interfaces.Expr {
	exprList := []interfaces.Expr{}
	// TODO: sort by pointer address for determinism ?
	for x := range exprMap {
		exprList = append(exprList, x)
	}
	return exprList
}

// UniqueExprList returns a unique list of expressions with no duplicates. It
// does this my converting it to a map and then back. This isn't necessarily the
// most efficient way, and doesn't preserve list ordering.
func UniqueExprList(exprList []interfaces.Expr) []interfaces.Expr {
	exprMap := ExprListToExprMap(exprList)
	return ExprMapToExprList(exprMap)
}

// ExprContains is an "in array" function to test for an expr in a slice of
// expressions.
func ExprContains(needle interfaces.Expr, haystack []interfaces.Expr) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

// pairs is a simple list of pairs of expressions which can be used as a simple
// undirected graph structure, or as a simple list of equalities.
type pairs []*interfaces.EqualityInvariant

// Vertices returns the list of vertices that the input expr is directly
// connected to.
func (obj pairs) Vertices(expr interfaces.Expr) []interfaces.Expr {
	m := make(map[interfaces.Expr]struct{})
	for _, x := range obj {
		if x.Expr1 == x.Expr2 { // skip circular
			continue
		}
		if x.Expr1 == expr {
			m[x.Expr2] = struct{}{}
		}
		if x.Expr2 == expr {
			m[x.Expr1] = struct{}{}
		}
	}

	out := []interfaces.Expr{}
	// FIXME: can we do this loop in a deterministic, sorted way?
	for k := range m {
		out = append(out, k)
	}

	return out
}

// DFS returns a depth first search for the graph, starting at the input vertex.
func (obj pairs) DFS(start interfaces.Expr) []interfaces.Expr {
	var d []interfaces.Expr // discovered
	var s []interfaces.Expr // stack
	found := false
	for _, x := range obj { // does the start exist?
		if x.Expr1 == start || x.Expr2 == start {
			found = true
			break
		}
	}
	if !found {
		return nil // TODO: error
	}
	v := start
	s = append(s, v)
	for len(s) > 0 {
		v, s = s[len(s)-1], s[:len(s)-1] // s.pop()

		if !ExprContains(v, d) { // if not discovered
			d = append(d, v) // label as discovered

			for _, w := range obj.Vertices(v) {
				s = append(s, w)
			}
		}
	}
	return d
}
