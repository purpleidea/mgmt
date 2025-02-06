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

// Package fastsolver implements very fast type unification.
package fastsolver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/unification"
	unificationUtil "github.com/purpleidea/mgmt/lang/unification/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// Name is the prefix for our solver log messages.
	Name = "fast"

	// OptimizationNotImplemented is a placeholder magic flag we can use.
	OptimizationNotImplemented = "not-implemented"
)

func init() {
	unification.Register(Name, func() unification.Solver { return &FastInvariantSolver{} })
	unification.Register("", func() unification.Solver { return &FastInvariantSolver{} }) // default
}

// FastInvariantSolver is a fast invariant solver based on union find. It is
// intended to be computationally efficient.
type FastInvariantSolver struct {
	// Strategy is a series of methodologies to heuristically improve the
	// solver.
	Strategy map[string]string

	// UnifiedState stores a common representation of our unification vars.
	UnifiedState *types.UnifiedState

	Debug bool
	Logf  func(format string, v ...interface{})

	// notImplemented tells the solver to behave differently somehow...
	notImplemented bool
}

// Init contains some handles that are used to initialize the solver.
func (obj *FastInvariantSolver) Init(init *unification.Init) error {
	obj.Strategy = init.Strategy
	obj.UnifiedState = init.UnifiedState
	obj.Debug = init.Debug
	obj.Logf = init.Logf

	optimizations, exists := init.Strategy[unification.StrategyOptimizationsKey]
	if !exists {
		return nil
	}
	// TODO: use a query string parser instead?
	for _, x := range strings.Split(optimizations, ",") {
		if x == OptimizationNotImplemented {
			obj.notImplemented = true
			continue
		}
	}

	return nil
}

// Solve runs the invariant solver. It mutates the .Data field in the .Uni
// unification variables, so that each set contains a single type. If each of
// the sets contains a complete type that no longer contains any ?1 type fields,
// then we have succeeded to unify all of our invariants. If not, then our list
// of invariants must be ambiguous. This is O(N*K) where N is the number of
// invariants, and K is the size of the maximum type. Eg a list of list of map
// of int to str would be of size three. (TODO: or is it four?)
func (obj *FastInvariantSolver) Solve(ctx context.Context, data *unification.Data) (*unification.InvariantSolution, error) {
	u := func(typ *types.Type) string {
		return obj.UnifiedState.String(typ)
	}

	// Build a "list" (map) of what we think we need to solve for exactly.
	exprs := make(map[interfaces.Expr]struct{})

	// TODO: Make better padding for debug output if we end up caring a lot!
	pad := strconv.Itoa(len(strconv.Itoa(max(0, len(data.UnificationInvariants)-1)))) // hack
	for i, x := range data.UnificationInvariants {                                    // []*UnificationInvariant
		// TODO: Is this a good break point for ctx?
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// pass
		}

		if x.Expr == nil {
			return nil, fmt.Errorf("unexpected nil expr")
		}
		exprs[x.Expr] = struct{}{} // Add to the set of what I must solve!

		// TODO: Should we pass ctx into Unify?
		if obj.Debug {
			obj.Logf("#%"+pad+"d unify(%s): %s -- %s", i, x.Expr, u(x.Expect), u(x.Actual))
		}
		if err := unificationUtil.Unify(x.Expect, x.Actual); err != nil {
			displayer, ok := x.Node.(interfaces.TextDisplayer)
			if ! ok {
				fmt.Printf("not displayable: %v\n", x.Node)
				return nil, errwrap.Wrapf(err, "unify error with: %s", x.Expr)
			}
			if highlight, e := displayer.HighlightText() ; e != nil {
				return nil, errwrap.Append(err, errwrap.Wrapf(e, "could not look up error location"))
			} else {
				return nil, fmt.Errorf("type unification error here: " + highlight + "\nERROR: %s", err.Error())
			}
		}
		if obj.Debug {
			e1, e2 := unificationUtil.Extract(x.Expect), unificationUtil.Extract(x.Actual)
			obj.Logf("#%"+pad+"d extract(%s): %s -- %s", i, x.Expr, u(e1), u(e2))
		}
	}
	count := len(exprs) // safety check

	// build final solution
	solutions := []*unification.EqualsInvariant{}
	for _, x := range data.UnificationInvariants { // []*UnificationInvariant
		if x.Expect == nil || x.Actual == nil {
			// programming error ?
			return nil, fmt.Errorf("unexpected nil invariant")
		}

		// zonk!
		t1 := unificationUtil.Extract(x.Expect)
		//t2 := unificationUtil.Extract(x.Actual)

		// TODO: collect all of these errors and return them together?
		if t1.HasUni() { // || t2.HasUni()
			return nil, fmt.Errorf("expr: %s is ambiguous: %s", x.Expr, u(t1))
		}

		//if err := t1.Cmp(t2); err != nil { // for development/debugging
		//	return nil, errwrap.Wrapf(err, "inconsistency between expect and actual")
		//}

		if _, exists := exprs[x.Expr]; !exists {
			// TODO: Do we need to check the consistency here?
			continue // already solved
		}
		delete(exprs, x.Expr) // solved!

		invar := &unification.EqualsInvariant{
			Expr: x.Expr,
			Type: t1, // || t2
		}
		solutions = append(solutions, invar)
	}

	// Determine that our solver produced a solution for every expr that
	// we're interested in. If it didn't, and it didn't error, then it's a
	// bug. We check for this because we care about safety, this ensures
	// that our AST will get fully populated with the correct types!
	if c := len(exprs); c > 0 { // if there's anything left, it's bad...
		// programming error!
		ptrs := []string{}
		disp := make(map[string]string) // display hack
		for i := range exprs {
			s := fmt.Sprintf("%p", i) // pointer
			ptrs = append(ptrs, s)
			disp[s] = i.String()
		}
		sort.Strings(ptrs)
		s := strings.Join(ptrs, ", ")

		obj.Logf("got %d unbound expr's: %s", c, s)
		for i, s := range ptrs {
			obj.Logf("(%d) %s => %s", i, s, disp[s])
		}
		return nil, fmt.Errorf("got %d unbound expr's: %s", c, s)
	}

	if l := len(solutions); l != count { // safety check
		return nil, fmt.Errorf("got %d expressions and %d solutions", count, l)
	}

	// Return a list instead of a map, to keep this ordering deterministic!
	return &unification.InvariantSolution{
		Solutions: solutions,
	}, nil
}
