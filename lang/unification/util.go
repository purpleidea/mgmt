// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
