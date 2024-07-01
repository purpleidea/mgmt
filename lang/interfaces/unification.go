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

package interfaces

import (
	"github.com/purpleidea/mgmt/lang/types"
)

// UnificationInvariant is the only type of invariant that we currently support.
// It always lets you specify an `Expr` so that we know what we're referring to.
// It always lets you specify two types which must get unified for a successful
// solution. Those two types are symmetrical in that it doesn't matter which is
// used where, it only affects how we print out error messages.
type UnificationInvariant struct { // formerly the SamInvariant
	// Expr is the expression we are determining the type for. This improves
	// our error messages.
	Expr Expr

	// Expect is one of the two types to unify.
	Expect *types.Type

	// Actual is one of the two types to unify.
	Actual *types.Type
}

// GenericCheck is the generic implementation of the Check Expr interface call.
// It is checking that the input type is equal to the object that Check is
// running on. In doing so, it adds any invariants that are necessary. Check
// must always call Infer to produce the invariant. The implementation can be
// generic for all expressions.
func GenericCheck(obj Expr, typ *types.Type) ([]*UnificationInvariant, error) {
	// Generic implementation of Check:
	// This wants to be inferred, because it always knows its type.
	actual, invariants, err := obj.Infer()
	if err != nil {
		return nil, err
	}

	invar := &UnificationInvariant{
		Expr:   obj,
		Expect: typ, // sam says not backwards
		Actual: actual,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}
