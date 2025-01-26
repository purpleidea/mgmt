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

// Package unification contains the code related to type unification for the mcl
// language.
package unification

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// Unifier holds all the data that the Unify function will need for it to run.
type Unifier struct {
	// AST is the input abstract syntax tree to unify.
	AST interfaces.Stmt

	// Solver is the solver algorithm implementation to use.
	Solver Solver

	// Strategy is a hack to tune unification performance until we have an
	// overall cleaner unification algorithm in place.
	Strategy map[string]string

	// UnifiedState stores a common representation of our unification vars.
	UnifiedState *types.UnifiedState

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Unify takes an AST expression tree and attempts to assign types to every node
// using the specified solver. The expression tree returns a list of invariants
// (or constraints) which must be met in order to find a unique value for the
// type of each expression. This list of invariants is passed into the solver,
// which hopefully finds a solution. If it cannot find a unique solution, then
// it will return an error. The invariants are available in different flavours
// which describe different constraint scenarios. The simplest expresses that a
// a particular node id (it's pointer) must be a certain type. More complicated
// invariants might express that two different node id's must have the same
// type. This function and logic was invented after the author could not find
// any proper literature or examples describing a well-known implementation of
// this process. Improvements and polite recommendations are welcome.
func (obj *Unifier) Unify(ctx context.Context) error {
	if obj.AST == nil {
		return fmt.Errorf("the AST is nil")
	}
	if obj.Solver == nil {
		return fmt.Errorf("the Solver is missing")
	}
	if obj.UnifiedState == nil {
		return fmt.Errorf("the UnifiedState table is missing")
	}
	if obj.Logf == nil {
		return fmt.Errorf("the Logf function is missing")
	}

	init := &Init{
		Strategy:     obj.Strategy,
		UnifiedState: obj.UnifiedState,
		Logf:         obj.Logf,
		Debug:        obj.Debug,
	}
	if err := obj.Solver.Init(init); err != nil {
		return err
	}

	if obj.Debug {
		obj.Logf("tree: %+v", obj.AST)
	}

	// This used to take a map[string]*types.Type type context as in/output.
	unificationInvariants, err := obj.AST.TypeCheck() // ([]*UnificationInvariant, error)
	if err != nil {
		return err
	}

	data := &Data{
		UnificationInvariants: unificationInvariants,
	}
	solved, err := obj.Solver.Solve(ctx, data) // often does union find
	if err != nil {
		return err
	}

	obj.Logf("found a solution of length: %d", len(solved.Solutions))
	if obj.Debug {
		for _, x := range solved.Solutions {
			obj.Logf("> %p %s -- %s", x.Expr, x.Type, x.Expr.String())
		}
	}

	// solver has found a solution, apply it...
	// we're modifying the AST, so code can't error now...
	for _, x := range solved.Solutions {
		if x.Expr == nil {
			// programming error ?
			return fmt.Errorf("unexpected invalid solution at: %p", x)
		}

		if obj.Debug {
			obj.Logf("solution: %p => %+v\t(%+v)", x.Expr, x.Type, x.Expr.String())
		}
		// apply this to each AST node
		if err := x.Expr.SetType(x.Type); err != nil {
			// SetType calls the Build() API, which functions as a
			// "check" step to add additional constraints that were
			// not possible during type unification.
			// TODO: Improve this error message!
			return fmt.Errorf("error setting type: %+v, error: %s", x.Expr, err)
		}
	}
	return nil
}

// InvariantSolution lists a trivial set of EqualsInvariant mappings so that you
// can populate your AST with SetType calls in a simple loop.
type InvariantSolution struct {
	Solutions []*EqualsInvariant // list of trivial solutions for each node
}

// EqualsInvariant is an invariant that symbolizes that the expression has a
// known type. It is used for producing solutions.
type EqualsInvariant struct {
	Expr interfaces.Expr
	Type *types.Type
}
