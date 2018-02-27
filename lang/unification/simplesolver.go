// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package unification // TODO: can we put this solver in a sub-package?

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"

	errwrap "github.com/pkg/errors"
)

const (
	// Name is the prefix for our solver log messages.
	Name = "solver: simple"
)

// SimpleInvariantSolverLogger is a wrapper which returns a
// SimpleInvariantSolver with the log parameter of your choice specified. The
// result satisfies the correct signature for the solver parameter of the
// Unification function.
func SimpleInvariantSolverLogger(logf func(format string, v ...interface{})) func([]interfaces.Invariant) (*InvariantSolution, error) {
	return func(invariants []interfaces.Invariant) (*InvariantSolution, error) {
		return SimpleInvariantSolver(invariants, logf)
	}
}

// SimpleInvariantSolver is an iterative invariant solver for AST expressions.
// It is intended to be very simple, even if it's computationally inefficient.
func SimpleInvariantSolver(invariants []interfaces.Invariant, logf func(format string, v ...interface{})) (*InvariantSolution, error) {
	logf("%s: invariants:", Name)
	for i, x := range invariants {
		logf("invariant(%d): %T: %s", i, x, x)
	}

	solved := make(map[interfaces.Expr]*types.Type)
	equalities := []interfaces.Invariant{}
	exclusives := []*ExclusiveInvariant{}
	// iterate through all invariants, flattening and sorting the list...
	for _, x := range invariants {
		switch invariant := x.(type) {
		case *EqualsInvariant:
			equalities = append(equalities, invariant)

		case *EqualityInvariant:
			equalities = append(equalities, invariant)

		case *EqualityInvariantList:
			// de-construct this list variant into a series
			// of equality variants so that our solver can
			// be implemented more simply...
			if len(invariant.Exprs) < 2 {
				return nil, fmt.Errorf("list invariant needs at least two elements")
			}
			for i := 0; i < len(invariant.Exprs)-1; i++ {
				invar := &EqualityInvariant{
					Expr1: invariant.Exprs[i],
					Expr2: invariant.Exprs[i+1],
				}
				equalities = append(equalities, invar)
			}

		case *EqualityWrapListInvariant:
			equalities = append(equalities, invariant)

		case *EqualityWrapMapInvariant:
			equalities = append(equalities, invariant)

		case *EqualityWrapStructInvariant:
			equalities = append(equalities, invariant)

		case *EqualityWrapFuncInvariant:
			equalities = append(equalities, invariant)

		// contains a list of invariants which this represents
		case *ConjunctionInvariant:
			for _, invar := range invariant.Invariants {
				equalities = append(equalities, invar)
			}

		case *ExclusiveInvariant:
			// these are special, note the different list
			if len(invariant.Invariants) > 0 {
				exclusives = append(exclusives, invariant)
			}

		case *AnyInvariant:
			equalities = append(equalities, invariant)

		default:
			return nil, fmt.Errorf("unknown invariant type: %T", x)
		}
	}

	listPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	mapPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	structPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	funcPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)

	logf("%s: starting loop with %d equalities", Name, len(equalities))
	// run until we're solved, stop consuming equalities, or type clash
	for {
		logf("%s: iterate...", Name)
		if len(equalities) == 0 && len(exclusives) == 0 {
			break // we're done, nothing left
		}
		used := []int{}
		for i, x := range equalities {
			logf("%s: match(%T): %+v", Name, x, x)

			// TODO: could each of these cases be implemented as a
			// method on the Invariant type to simplify this code?
			switch eq := x.(type) {
			// trivials
			case *EqualsInvariant:
				typ, exists := solved[eq.Expr]
				if !exists {
					solved[eq.Expr] = eq.Type // yay, we learned something!
					used = append(used, i)    // mark equality as used up
					logf("%s: solved trivial equality", Name)
					continue
				}
				// we already specified this, so check the repeat is consistent
				if err := typ.Cmp(eq.Type); err != nil {
					// this error shouldn't happen unless we purposefully
					// try to trick the solver, or we're in a recursive try
					return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with equals")
				}
				used = append(used, i) // mark equality as duplicate
				logf("%s: duplicate trivial equality", Name)
				continue

			// partials
			case *EqualityWrapListInvariant:
				if _, exists := listPartials[eq.Expr1]; !exists {
					listPartials[eq.Expr1] = make(map[interfaces.Expr]*types.Type)
				}

				if typ, exists := solved[eq.Expr1]; exists {
					// wow, now known, so tell the partials!
					// TODO: this assumes typ is a list, is that guaranteed?
					listPartials[eq.Expr1][eq.Expr2Val] = typ.Val
				}

				// can we add to partials ?
				for _, y := range []interfaces.Expr{eq.Expr2Val} {
					typ, exists := solved[y]
					if !exists {
						continue
					}
					t, exists := listPartials[eq.Expr1][y]
					if !exists {
						listPartials[eq.Expr1][y] = typ // learn!
						continue
					}
					if err := t.Cmp(typ); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial list val")
					}
				}

				// can we solve anything?
				var ready = true // assume ready
				typ := &types.Type{
					Kind: types.KindList,
				}
				valTyp, exists := listPartials[eq.Expr1][eq.Expr2Val]
				if !exists {
					ready = false // nope!
				} else {
					typ.Val = valTyp // build up typ
				}
				if ready {
					if t, exists := solved[eq.Expr1]; exists {
						if err := t.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with list")
						}
					}
					// sub checks
					if t, exists := solved[eq.Expr2Val]; exists {
						if err := t.Cmp(typ.Val); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with list val")
						}
					}

					solved[eq.Expr1] = typ        // yay, we learned something!
					solved[eq.Expr2Val] = typ.Val // yay, we learned something!
					used = append(used, i)        // mark equality as used up
					logf("%s: solved list wrap partial", Name)
					continue
				}

			case *EqualityWrapMapInvariant:
				if _, exists := mapPartials[eq.Expr1]; !exists {
					mapPartials[eq.Expr1] = make(map[interfaces.Expr]*types.Type)
				}

				if typ, exists := solved[eq.Expr1]; exists {
					// wow, now known, so tell the partials!
					// TODO: this assumes typ is a map, is that guaranteed?
					mapPartials[eq.Expr1][eq.Expr2Key] = typ.Key
					mapPartials[eq.Expr1][eq.Expr2Val] = typ.Val
				}

				// can we add to partials ?
				for _, y := range []interfaces.Expr{eq.Expr2Key, eq.Expr2Val} {
					typ, exists := solved[y]
					if !exists {
						continue
					}
					t, exists := mapPartials[eq.Expr1][y]
					if !exists {
						mapPartials[eq.Expr1][y] = typ // learn!
						continue
					}
					if err := t.Cmp(typ); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial map key/val")
					}
				}

				// can we solve anything?
				var ready = true // assume ready
				typ := &types.Type{
					Kind: types.KindMap,
				}
				keyTyp, exists := mapPartials[eq.Expr1][eq.Expr2Key]
				if !exists {
					ready = false // nope!
				} else {
					typ.Key = keyTyp // build up typ
				}
				valTyp, exists := mapPartials[eq.Expr1][eq.Expr2Val]
				if !exists {
					ready = false // nope!
				} else {
					typ.Val = valTyp // build up typ
				}
				if ready {
					if t, exists := solved[eq.Expr1]; exists {
						if err := t.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with map")
						}
					}
					// sub checks
					if t, exists := solved[eq.Expr2Key]; exists {
						if err := t.Cmp(typ.Key); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with map key")
						}
					}
					if t, exists := solved[eq.Expr2Val]; exists {
						if err := t.Cmp(typ.Val); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with map val")
						}
					}

					solved[eq.Expr1] = typ        // yay, we learned something!
					solved[eq.Expr2Key] = typ.Key // yay, we learned something!
					solved[eq.Expr2Val] = typ.Val // yay, we learned something!
					used = append(used, i)        // mark equality as used up
					logf("%s: solved map wrap partial", Name)
					continue
				}

			case *EqualityWrapStructInvariant:
				if _, exists := structPartials[eq.Expr1]; !exists {
					structPartials[eq.Expr1] = make(map[interfaces.Expr]*types.Type)
				}

				if typ, exists := solved[eq.Expr1]; exists {
					// wow, now known, so tell the partials!
					// TODO: this assumes typ is a struct, is that guaranteed?
					if len(typ.Ord) != len(eq.Expr2Ord) {
						return nil, fmt.Errorf("struct field count differs")
					}
					for i, name := range eq.Expr2Ord {
						expr := eq.Expr2Map[name]                            // assume key exists
						structPartials[eq.Expr1][expr] = typ.Map[typ.Ord[i]] // assume key exists
					}
				}

				// can we add to partials ?
				for name, y := range eq.Expr2Map {
					typ, exists := solved[y]
					if !exists {
						continue
					}
					t, exists := structPartials[eq.Expr1][y]
					if !exists {
						structPartials[eq.Expr1][y] = typ // learn!
						continue
					}
					if err := t.Cmp(typ); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial struct field: %s", name)
					}
				}

				// can we solve anything?
				var ready = true // assume ready
				typ := &types.Type{
					Kind: types.KindStruct,
				}
				typ.Map = make(map[string]*types.Type)
				for name, y := range eq.Expr2Map {
					t, exists := structPartials[eq.Expr1][y]
					if !exists {
						ready = false // nope!
						break
					}
					typ.Map[name] = t // build up typ
				}
				if ready {
					typ.Ord = eq.Expr2Ord // known order

					if t, exists := solved[eq.Expr1]; exists {
						if err := t.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with struct")
						}
					}
					// sub checks
					for name, y := range eq.Expr2Map {
						if t, exists := solved[y]; exists {
							if err := t.Cmp(typ.Map[name]); err != nil {
								return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with struct field: %s", name)
							}
						}
					}

					solved[eq.Expr1] = typ // yay, we learned something!
					// we should add the other expr's in too...
					for name, y := range eq.Expr2Map {
						solved[y] = typ.Map[name] // yay, we learned something!
					}
					used = append(used, i) // mark equality as used up
					logf("%s: solved struct wrap partial", Name)
					continue
				}

			case *EqualityWrapFuncInvariant:
				if _, exists := funcPartials[eq.Expr1]; !exists {
					funcPartials[eq.Expr1] = make(map[interfaces.Expr]*types.Type)
				}

				if typ, exists := solved[eq.Expr1]; exists {
					// wow, now known, so tell the partials!
					// TODO: this assumes typ is a func, is that guaranteed?
					if len(typ.Ord) != len(eq.Expr2Ord) {
						return nil, fmt.Errorf("func arg count differs")
					}
					for i, name := range eq.Expr2Ord {
						expr := eq.Expr2Map[name]                          // assume key exists
						funcPartials[eq.Expr1][expr] = typ.Map[typ.Ord[i]] // assume key exists
					}
					funcPartials[eq.Expr1][eq.Expr2Out] = typ.Out
				}

				// can we add to partials ?
				for name, y := range eq.Expr2Map {
					typ, exists := solved[y]
					if !exists {
						continue
					}
					t, exists := funcPartials[eq.Expr1][y]
					if !exists {
						funcPartials[eq.Expr1][y] = typ // learn!
						continue
					}
					if err := t.Cmp(typ); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg: %s", name)
					}
				}
				for _, y := range []interfaces.Expr{eq.Expr2Out} {
					typ, exists := solved[y]
					if !exists {
						continue
					}
					t, exists := funcPartials[eq.Expr1][y]
					if !exists {
						funcPartials[eq.Expr1][y] = typ // learn!
						continue
					}
					if err := t.Cmp(typ); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg")
					}
				}

				// can we solve anything?
				var ready = true // assume ready
				typ := &types.Type{
					Kind: types.KindFunc,
				}
				typ.Map = make(map[string]*types.Type)
				for name, y := range eq.Expr2Map {
					t, exists := funcPartials[eq.Expr1][y]
					if !exists {
						ready = false // nope!
						break
					}
					typ.Map[name] = t // build up typ
				}
				outTyp, exists := funcPartials[eq.Expr1][eq.Expr2Out]
				if !exists {
					ready = false // nope!
				} else {
					typ.Out = outTyp // build up typ
				}
				if ready {
					typ.Ord = eq.Expr2Ord // known order

					if t, exists := solved[eq.Expr1]; exists {
						if err := t.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with func")
						}
					}
					// sub checks
					for name, y := range eq.Expr2Map {
						if t, exists := solved[y]; exists {
							if err := t.Cmp(typ.Map[name]); err != nil {
								return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with func arg: %s", name)
							}
						}
					}
					if t, exists := solved[eq.Expr2Out]; exists {
						if err := t.Cmp(typ.Out); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with func out")
						}
					}

					solved[eq.Expr1] = typ // yay, we learned something!
					// we should add the other expr's in too...
					for name, y := range eq.Expr2Map {
						solved[y] = typ.Map[name] // yay, we learned something!
					}
					solved[eq.Expr2Out] = typ.Out // yay, we learned something!
					used = append(used, i)        // mark equality as used up
					logf("%s: solved func wrap partial", Name)
					continue
				}

			// regular matching
			case *EqualityInvariant:
				typ1, exists1 := solved[eq.Expr1]
				typ2, exists2 := solved[eq.Expr2]

				if !exists1 && !exists2 { // neither equality connects
					// can't learn more from this equality yet
					// nothing is known about either side of it
					continue
				}
				if exists1 && exists2 { // both equalities already connect
					// both sides are already known-- are they the same?
					if err := typ1.Cmp(typ2); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with equality")
					}
					used = append(used, i) // mark equality as used up
					logf("%s: duplicate regular equality", Name)
					continue
				}
				if exists1 && !exists2 { // first equality already connects
					solved[eq.Expr2] = typ1 // yay, we learned something!
					used = append(used, i)  // mark equality as used up
					logf("%s: solved regular equality", Name)
					continue
				}
				if exists2 && !exists1 { // second equality already connects
					solved[eq.Expr1] = typ2 // yay, we learned something!
					used = append(used, i)  // mark equality as used up
					logf("%s: solved regular equality", Name)
					continue
				}

				panic("reached unexpected code")

			// wtf matching
			case *AnyInvariant:
				// this basically ensures that the expr gets solved
				if _, exists := solved[eq.Expr]; exists {
					used = append(used, i) // mark equality as used up
					logf("%s: solved `any` equality", Name)
				}
				continue

			default:
				return nil, fmt.Errorf("unknown invariant type: %T", x)
			}
		} // end inner for loop
		if len(used) == 0 {
			// looks like we're now ambiguous, but if we have any
			// exclusives, recurse into each possibility to see if
			// one of them can help solve this! first one wins. add
			// in the exclusive to the current set of equalities!

			// what have we learned for sure so far?
			partialSolutions := []interfaces.Invariant{}
			logf("%s: %d solved, %d unsolved, and %d exclusives left", Name, len(solved), len(equalities), len(exclusives))
			if len(exclusives) > 0 {
				// FIXME: can we do this loop in a deterministic, sorted way?
				for expr, typ := range solved {
					invar := &EqualsInvariant{
						Expr: expr,
						Type: typ,
					}
					partialSolutions = append(partialSolutions, invar)
					logf("%s: solved: %+v", Name, invar)
				}

				// also include anything that hasn't been solved yet
				for _, x := range equalities {
					partialSolutions = append(partialSolutions, x)
					logf("%s: unsolved: %+v", Name, x)
				}
			}

			// let's try each combination, one at a time...
			for i, ex := range exclusivesProduct(exclusives) { // [][]interfaces.Invariant
				logf("%s: exclusive(%d):\n%+v", Name, i, ex)
				// we could waste a lot of cpu, and start from
				// the beginning, but instead we could use the
				// list of known solutions found and continue!
				// TODO: make sure none of these edit partialSolutions
				recursiveInvariants := []interfaces.Invariant{}
				recursiveInvariants = append(recursiveInvariants, partialSolutions...)
				recursiveInvariants = append(recursiveInvariants, ex...)
				logf("%s: recursing...", Name)
				solution, err := SimpleInvariantSolver(recursiveInvariants, logf)
				if err != nil {
					logf("%s: recursive solution failed: %+v", Name, err)
					continue // no solution found here...
				}
				// solution found!
				logf("%s: recursive solution found!", Name)
				return solution, nil
			}

			// TODO: print ambiguity
			return nil, fmt.Errorf("can't unify, no equalities were consumed, we're ambiguous")
		}
		// delete used equalities, in reverse order to preserve indexing!
		for i := len(used) - 1; i >= 0; i-- {
			ix := used[i] // delete index that was marked as used!
			equalities = append(equalities[:ix], equalities[ix+1:]...)
		}
	} // end giant for loop

	// build final solution
	solutions := []*EqualsInvariant{}
	// FIXME: can we do this loop in a deterministic, sorted way?
	for expr, typ := range solved {
		invar := &EqualsInvariant{
			Expr: expr,
			Type: typ,
		}
		solutions = append(solutions, invar)
	}
	return &InvariantSolution{
		Solutions: solutions,
	}, nil
}
