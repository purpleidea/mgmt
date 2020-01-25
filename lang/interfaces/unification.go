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

package interfaces

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/types"
)

// Invariant represents a constraint that is described by the Expr's and Stmt's,
// and which is passed into the unification solver to describe what is known by
// the AST.
type Invariant interface {
	// TODO: should we add any other methods to this type?
	fmt.Stringer

	// ExprList returns the list of valid expressions in this invariant.
	ExprList() []Expr

	// Matches returns whether an invariant matches the existing solution.
	// If it is inconsistent, then it errors.
	Matches(solved map[Expr]*types.Type) (bool, error)

	// Possible returns an error if it is certain that it is NOT possible to
	// get a solution with this invariant and the set of partials. In
	// certain cases, it might not be able to determine that it's not
	// possible, while simultaneously not being able to guarantee a possible
	// solution either. In this situation, it should return nil, since this
	// is used as a filtering mechanism, and the nil result of possible is
	// preferred over eliminating a tricky, but possible one.
	Possible(partials []Invariant) error
}
