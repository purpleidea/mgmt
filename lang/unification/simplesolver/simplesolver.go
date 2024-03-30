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

package simplesolver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// Name is the prefix for our solver log messages.
	Name = "simple"

	// OptimizationSkipFuncCmp is the magic flag name to include the skip
	// func cmp optimization which can speed up some simple programs. If
	// this is specified, then OptimizationHeuristicalDrop is redundant.
	OptimizationSkipFuncCmp = "skip-func-cmp"

	// OptimizationHeuristicalDrop is the magic flag name to include to tell
	// the solver to drop some func compares. This is a less aggressive form
	// of OptimizationSkipFuncCmp. This is redundant if
	// OptimizationSkipFuncCmp is true.
	OptimizationHeuristicalDrop = "heuristical-drop"

	// AllowRecursion specifies whether we're allowed to use the recursive
	// solver or not. It uses an absurd amount of memory, and might hang
	// your system if a simple solution doesn't exist.
	AllowRecursion = false

	// RecursionDepthLimit specifies the max depth that is allowed.
	// FIXME: RecursionDepthLimit is not currently implemented
	RecursionDepthLimit = 5 // TODO: pick a better value ?

	// RecursionInvariantLimit specifies the max number of invariants we can
	// recurse into.
	RecursionInvariantLimit = 5 // TODO: pick a better value ?
)

func init() {
	unification.Register(Name, func() unification.Solver { return &SimpleInvariantSolver{} })
}

// SimpleInvariantSolver is an iterative invariant solver for AST expressions.
// It is intended to be very simple, even if it's computationally inefficient.
// TODO: Move some of the global solver constants into this struct as params.
type SimpleInvariantSolver struct {
	// Strategy is a series of methodologies to heuristically improve the
	// solver.
	Strategy map[string]string

	Debug bool
	Logf  func(format string, v ...interface{})

	// skipFuncCmp tells the solver to skip the slow loop entirely. This may
	// prevent some correct programs from completing type unification.
	skipFuncCmp bool

	// heuristicalDrop tells the solver to drop some func compares. This was
	// determined heuristically and needs checking to see if it's even a
	// sensible algorithmic approach. This is a less aggressive form of
	// skipFuncCmp. This is redundant if skipFuncCmp is true.
	heuristicalDrop bool

	// zTotal is a heuristic counter to measure the size of the slow loop.
	zTotal int
}

// Init contains some handles that are used to initialize the solver.
func (obj *SimpleInvariantSolver) Init(init *unification.Init) error {
	obj.Strategy = init.Strategy

	obj.Debug = init.Debug
	obj.Logf = init.Logf

	optimizations, exists := init.Strategy[unification.StrategyOptimizationsKey]
	if !exists {
		return nil
	}
	// TODO: use a query string parser instead?
	for _, x := range strings.Split(optimizations, ",") {
		if x == OptimizationSkipFuncCmp {
			obj.skipFuncCmp = true
			continue
		}
		if x == OptimizationHeuristicalDrop {
			obj.heuristicalDrop = true
			continue
		}
	}

	return nil
}

// Solve is the actual solve implementation of the solver.
func (obj *SimpleInvariantSolver) Solve(ctx context.Context, invariants []interfaces.Invariant, expected []interfaces.Expr) (*unification.InvariantSolution, error) {
	process := func(invariants []interfaces.Invariant) ([]interfaces.Invariant, []*interfaces.ExclusiveInvariant, error) {
		equalities := []interfaces.Invariant{}
		exclusives := []*interfaces.ExclusiveInvariant{}
		generators := []interfaces.Invariant{}

		for ix := 0; len(invariants) > ix; ix++ { // while
			x := invariants[ix]
			switch invariant := x.(type) {
			case *interfaces.EqualsInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.EqualityInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.EqualityInvariantList:
				// de-construct this list variant into a series
				// of equality variants so that our solver can
				// be implemented more simply...
				if len(invariant.Exprs) < 2 {
					return nil, nil, fmt.Errorf("list invariant needs at least two elements")
				}
				for i := 0; i < len(invariant.Exprs)-1; i++ {
					invar := &interfaces.EqualityInvariant{
						Expr1: invariant.Exprs[i],
						Expr2: invariant.Exprs[i+1],
					}
					equalities = append(equalities, invar)
				}

			case *interfaces.EqualityWrapListInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.EqualityWrapMapInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.EqualityWrapStructInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.EqualityWrapFuncInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.EqualityWrapCallInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.GeneratorInvariant:
				// these are special, note the different list
				generators = append(generators, invariant)

			// contains a list of invariants which this represents
			case *interfaces.ConjunctionInvariant:
				invariants = append(invariants, invariant.Invariants...)

			case *interfaces.ExclusiveInvariant:
				// these are special, note the different list
				if len(invariant.Invariants) > 0 {
					exclusives = append(exclusives, invariant)
				}

			case *interfaces.AnyInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.ValueInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.CallFuncArgsValueInvariant:
				equalities = append(equalities, invariant)

			case *interfaces.SkipInvariant:
				// drop it for now
				//equalities = append(equalities, invariant)

			default:
				return nil, nil, fmt.Errorf("unknown invariant type: %T", x)
			}
		}

		// optimization: if we have zero generator invariants, we can
		// discard the value invariants!
		// NOTE: if exclusives do *not* contain nested generators, then
		// we don't need to check for exclusives here, and the logic is
		// much faster and simpler and can possibly solve more cases...
		if len(generators) == 0 && len(exclusives) == 0 {
			used := []int{}
			for i, x := range equalities {
				_, ok1 := x.(*interfaces.ValueInvariant)
				_, ok2 := x.(*interfaces.CallFuncArgsValueInvariant)
				if !ok1 && !ok2 {
					continue
				}
				used = append(used, i) // mark equality as used up
			}
			obj.Logf("%s: got %d equalities left after %d used up", Name, len(equalities)-len(used), len(used))
			// delete used equalities, in reverse order to preserve indexing!
			for i := len(used) - 1; i >= 0; i-- {
				ix := used[i] // delete index that was marked as used!
				equalities = append(equalities[:ix], equalities[ix+1:]...)
			}
		}

		// append the generators at the end
		// (they can go in any order, but it's more optimal this way)
		equalities = append(equalities, generators...)

		return equalities, exclusives, nil
	}

	obj.Logf("%s: invariants:", Name)
	for i, x := range invariants {
		obj.Logf("invariant(%d): %T: %s", i, x, x)
	}

	solved := make(map[interfaces.Expr]*types.Type)
	// iterate through all invariants, flattening and sorting the list...
	equalities, exclusives, err := process(invariants)
	if err != nil {
		return nil, err
	}

	//skipExprs := make(map[interfaces.Expr]struct{})

	// XXX: if these partials all shared the same variable definition, would
	// it all work??? Maybe we don't even need the first map prefix...
	listPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	mapPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	structPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	funcPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)
	callPartials := make(map[interfaces.Expr]map[interfaces.Expr]*types.Type)

	isSolvedFn := func(solved map[interfaces.Expr]*types.Type) (map[interfaces.Expr]struct{}, bool) {
		unsolved := make(map[interfaces.Expr]struct{})
		result := true
		for _, x := range expected {
			if typ, exists := solved[x]; !exists || typ == nil {
				result = false
				unsolved[x] = struct{}{}
			}
		}
		return unsolved, result
	}

	// list all the expr's connected to expr, use pairs as chains
	listConnectedFn := func(expr interfaces.Expr, exprs []*interfaces.EqualityInvariant) []interfaces.Expr {
		pairsType := unification.Pairs(exprs)
		return pairsType.DFS(expr)
	}

	// does the equality invariant already exist in the set? order of expr1
	// and expr2 doesn't matter
	eqContains := func(eq *interfaces.EqualityInvariant, pairs []*interfaces.EqualityInvariant) bool {
		for _, x := range pairs {
			if eq.Expr1 == x.Expr1 && eq.Expr2 == x.Expr2 {
				return true
			}
			if eq.Expr1 == x.Expr2 && eq.Expr2 == x.Expr1 { // reverse
				return true
			}
		}
		return false
	}

	// build a static list that won't get consumed
	eqInvariants := []*interfaces.EqualityInvariant{}
	fnInvariants := []*interfaces.EqualityWrapFuncInvariant{}
	for _, x := range equalities {
		if eq, ok := x.(*interfaces.EqualityInvariant); ok {
			eqInvariants = append(eqInvariants, eq)
		}

		if eq, ok := x.(*interfaces.EqualityWrapFuncInvariant); ok {
			fnInvariants = append(fnInvariants, eq)
		}
	}

	countGenerators := func() (int, int) {
		active := 0
		total := 0
		for _, x := range equalities {
			gen, ok := x.(*interfaces.GeneratorInvariant)
			if !ok {
				continue
			}
			total++ // total count
			if gen.Inactive {
				continue // skip inactive
			}
			active++ // active
		}
		return total, active
	}
	activeGenerators := func() int {
		_, active := countGenerators()
		return active
	}

	obj.Logf("%s: starting loop with %d equalities", Name, len(equalities))
	// run until we're solved, stop consuming equalities, or type clash
Loop:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// pass
		}
		// Once we're done solving everything else except the generators
		// then we can exit, but we want to make sure the generators had
		// a chance to "speak up" and make sure they were part of Unify.
		// Every generator gets to run once, and if that does not change
		// the result, then we mark it as inactive.

		obj.Logf("%s: iterate...", Name)
		if len(equalities) == 0 && len(exclusives) == 0 && activeGenerators() == 0 {
			break // we're done, nothing left
		}
		used := []int{}
		for eqi := 0; eqi < len(equalities); eqi++ {
			eqx := equalities[eqi]
			obj.Logf("%s: match(%T): %+v", Name, eqx, eqx)

			// TODO: could each of these cases be implemented as a
			// method on the Invariant type to simplify this code?
			switch eq := eqx.(type) {
			// trivials
			case *interfaces.EqualsInvariant:
				typ, exists := solved[eq.Expr]
				if !exists {
					solved[eq.Expr] = eq.Type // yay, we learned something!
					used = append(used, eqi)  // mark equality as used up
					obj.Logf("%s: solved trivial equality", Name)
					continue
				}
				// we already specified this, so check the repeat is consistent
				if err := typ.Cmp(eq.Type); err != nil {
					// this error shouldn't happen unless we purposefully
					// try to trick the solver, or we're in a recursive try
					return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with equals")
				}
				used = append(used, eqi) // mark equality as duplicate
				obj.Logf("%s: duplicate trivial equality", Name)
				continue

			// partials
			case *interfaces.EqualityWrapListInvariant:
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

						// Even though this is only a partial learn, we should still add it to the solved information!
						if newTyp, exists := solved[y]; !exists {
							solved[y] = typ // yay, we learned something!
							//used = append(used, i) // mark equality as used up when complete!
							obj.Logf("%s: solved partial list val equality", Name)
						} else if err := newTyp.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial list val equality")
						}

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
					used = append(used, eqi)      // mark equality as used up
					obj.Logf("%s: solved list wrap partial", Name)
					continue
				}

			case *interfaces.EqualityWrapMapInvariant:
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

						// Even though this is only a partial learn, we should still add it to the solved information!
						if newTyp, exists := solved[y]; !exists {
							solved[y] = typ // yay, we learned something!
							//used = append(used, i) // mark equality as used up when complete!
							obj.Logf("%s: solved partial map key/val equality", Name)
						} else if err := newTyp.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial map key/val equality")
						}

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
					used = append(used, eqi)      // mark equality as used up
					obj.Logf("%s: solved map wrap partial", Name)
					continue
				}

			case *interfaces.EqualityWrapStructInvariant:
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

						// Even though this is only a partial learn, we should still add it to the solved information!
						if newTyp, exists := solved[y]; !exists {
							solved[y] = typ // yay, we learned something!
							//used = append(used, i) // mark equality as used up when complete!
							obj.Logf("%s: solved partial struct field equality", Name)
						} else if err := newTyp.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial struct field equality")
						}

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
					used = append(used, eqi) // mark equality as used up
					obj.Logf("%s: solved struct wrap partial", Name)
					continue
				}

			case *interfaces.EqualityWrapFuncInvariant:
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

						// Even though this is only a partial learn, we should still add it to the solved information!
						if newTyp, exists := solved[y]; !exists {
							solved[y] = typ // yay, we learned something!
							//used = append(used, i) // mark equality as used up when complete!
							obj.Logf("%s: solved partial func arg equality", Name)
						} else if err := newTyp.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg equality")
						}

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

						// Even though this is only a partial learn, we should still add it to the solved information!
						if newTyp, exists := solved[y]; !exists {
							solved[y] = typ // yay, we learned something!
							//used = append(used, i) // mark equality as used up when complete!
							obj.Logf("%s: solved partial func return equality", Name)
						} else if err := newTyp.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func return equality")
						}

						continue
					}
					if err := t.Cmp(typ); err != nil {
						return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg")
					}
				}

				equivs := listConnectedFn(eq.Expr1, eqInvariants) // or equivalent!
				if obj.Debug && len(equivs) > 0 {
					obj.Logf("%s: equiv %d: %p %+v", Name, len(equivs), eq.Expr1, eq.Expr1)
					for i, x := range equivs {
						obj.Logf("%s: equiv(%d): %p %+v", Name, i, x, x)
					}
				}
				// This determines if a pointer is equivalent to
				// a pointer we're interested to match against.
				inEquiv := func(needle interfaces.Expr) bool {
					for _, x := range equivs {
						if x == needle {
							return true
						}
					}
					return false
				}
				// is there another EqualityWrapFuncInvariant with the same Expr1 pointer?
				fnDone := make(map[int]struct{})
				for z, fn := range fnInvariants {
					if obj.skipFuncCmp {
						break
					}
					obj.zTotal++
					// XXX: I think we're busy in this loop a lot.
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
						// pass
					}
					// is this fn.Expr1 related by equivalency graph to eq.Expr1 ?
					if (eq.Expr1 != fn.Expr1) && !inEquiv(fn.Expr1) {
						if obj.Debug {
							obj.Logf("%s: equiv skip: %p %+v", Name, fn.Expr1, fn.Expr1)
						}
						continue
					}
					if obj.Debug {
						obj.Logf("%s: equiv used: %p %+v", Name, fn.Expr1, fn.Expr1)
					}
					//if eq.Expr1 != fn.Expr1 { // previously
					//	continue
					//}
					// wow they match or are equivalent

					if len(eq.Expr2Ord) != len(fn.Expr2Ord) {
						return nil, fmt.Errorf("func arg count differs")
					}
					for i := range eq.Expr2Ord {
						lhsName := eq.Expr2Ord[i]
						lhsExpr := eq.Expr2Map[lhsName] // assume key exists
						rhsName := fn.Expr2Ord[i]
						rhsExpr := fn.Expr2Map[rhsName] // assume key exists

						lhsTyp, lhsExists := solved[lhsExpr]
						rhsTyp, rhsExists := solved[rhsExpr]

						// add to eqInvariants if not already there!
						// TODO: If this parent func invariant gets solved,
						// will being unable to add this later be an issue?
						newEq := &interfaces.EqualityInvariant{
							Expr1: lhsExpr,
							Expr2: rhsExpr,
						}
						if !eqContains(newEq, eqInvariants) {
							obj.Logf("%s: new equality: %p %+v <-> %p %+v", Name, newEq.Expr1, newEq.Expr1, newEq.Expr2, newEq.Expr2)
							eqInvariants = append(eqInvariants, newEq)
							// TODO: add it as a generator instead?
							equalities = append(equalities, newEq)
							fnDone[z] = struct{}{} // XXX: heuristical drop
						}

						// both solved or both unsolved we skip
						if lhsExists && !rhsExists { // teach rhs
							typ, exists := funcPartials[eq.Expr1][rhsExpr]
							if !exists {
								funcPartials[eq.Expr1][rhsExpr] = lhsTyp // learn!

								// Even though this is only a partial learn, we should still add it to the solved information!
								if newTyp, exists := solved[rhsExpr]; !exists {
									solved[rhsExpr] = lhsTyp // yay, we learned something!
									//used = append(used, i) // mark equality as used up when complete!
									obj.Logf("%s: solved partial rhs func arg equality", Name)
									fnDone[z] = struct{}{} // XXX: heuristical drop
								} else if err := newTyp.Cmp(lhsTyp); err != nil {
									return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial rhs func arg equality")
								}

								continue
							}
							if err := typ.Cmp(lhsTyp); err != nil {
								return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg")
							}
						}
						if rhsExists && !lhsExists { // teach lhs
							typ, exists := funcPartials[eq.Expr1][lhsExpr]
							if !exists {
								funcPartials[eq.Expr1][lhsExpr] = rhsTyp // learn!

								// Even though this is only a partial learn, we should still add it to the solved information!
								if newTyp, exists := solved[lhsExpr]; !exists {
									solved[lhsExpr] = rhsTyp // yay, we learned something!
									//used = append(used, i) // mark equality as used up when complete!
									obj.Logf("%s: solved partial lhs func arg equality", Name)
									fnDone[z] = struct{}{} // XXX: heuristical drop
								} else if err := newTyp.Cmp(rhsTyp); err != nil {
									return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial lhs func arg equality")
								}

								continue
							}
							if err := typ.Cmp(rhsTyp); err != nil {
								return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg")
							}
						}
					}

					lhsExpr := eq.Expr2Out
					rhsExpr := fn.Expr2Out

					lhsTyp, lhsExists := solved[lhsExpr]
					rhsTyp, rhsExists := solved[rhsExpr]

					// add to eqInvariants if not already there!
					// TODO: If this parent func invariant gets solved,
					// will being unable to add this later be an issue?
					newEq := &interfaces.EqualityInvariant{
						Expr1: lhsExpr,
						Expr2: rhsExpr,
					}
					if !eqContains(newEq, eqInvariants) {
						obj.Logf("%s: new equality: %p %+v <-> %p %+v", Name, newEq.Expr1, newEq.Expr1, newEq.Expr2, newEq.Expr2)
						eqInvariants = append(eqInvariants, newEq)
						// TODO: add it as a generator instead?
						equalities = append(equalities, newEq)
						fnDone[z] = struct{}{} // XXX: heuristical drop
					}

					// both solved or both unsolved we skip
					if lhsExists && !rhsExists { // teach rhs
						typ, exists := funcPartials[eq.Expr1][rhsExpr]
						if !exists {
							funcPartials[eq.Expr1][rhsExpr] = lhsTyp // learn!

							// Even though this is only a partial learn, we should still add it to the solved information!
							if newTyp, exists := solved[rhsExpr]; !exists {
								solved[rhsExpr] = lhsTyp // yay, we learned something!
								//used = append(used, i) // mark equality as used up when complete!
								obj.Logf("%s: solved partial rhs func return equality", Name)
								fnDone[z] = struct{}{} // XXX: heuristical drop
							} else if err := newTyp.Cmp(lhsTyp); err != nil {
								return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial rhs func return equality")
							}

							continue
						}
						if err := typ.Cmp(lhsTyp); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg")
						}
					}
					if rhsExists && !lhsExists { // teach lhs
						typ, exists := funcPartials[eq.Expr1][lhsExpr]
						if !exists {
							funcPartials[eq.Expr1][lhsExpr] = rhsTyp // learn!

							// Even though this is only a partial learn, we should still add it to the solved information!
							if newTyp, exists := solved[lhsExpr]; !exists {
								solved[lhsExpr] = rhsTyp // yay, we learned something!
								//used = append(used, i) // mark equality as used up when complete!
								obj.Logf("%s: solved partial lhs func return equality", Name)
								fnDone[z] = struct{}{} // XXX: heuristical drop
							} else if err := newTyp.Cmp(rhsTyp); err != nil {
								return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial lhs func return equality")
							}

							continue
						}
						if err := typ.Cmp(rhsTyp); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with partial func arg")
						}
					}

				} // end big slow loop
				if obj.heuristicalDrop {
					fnDoneList := []int{}
					for k := range fnDone {
						fnDoneList = append(fnDoneList, k)
					}
					sort.Ints(fnDoneList)

					for z := len(fnDoneList) - 1; z >= 0; z-- {
						zx := fnDoneList[z] // delete index that was marked as done!
						fnInvariants = append(fnInvariants[:zx], fnInvariants[zx+1:]...)
						if obj.Debug {
							obj.Logf("zTotal: %d, had: %d, removing: %d", obj.zTotal, len(fnInvariants), len(fnDoneList))
						}
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
					used = append(used, eqi)      // mark equality as used up
					obj.Logf("%s: solved func wrap partial", Name)
					continue
				}

			case *interfaces.EqualityWrapCallInvariant:
				// the logic is slightly different here, because
				// we can only go from the func type to the call
				// type as we can't do the reverse determination
				if _, exists := callPartials[eq.Expr2Func]; !exists {
					callPartials[eq.Expr2Func] = make(map[interfaces.Expr]*types.Type)
				}

				if typ, exists := solved[eq.Expr2Func]; exists {
					// wow, now known, so tell the partials!
					if typ.Kind != types.KindFunc {
						return nil, fmt.Errorf("expected: %s, got: %s", types.KindFunc, typ.Kind)
					}
					callPartials[eq.Expr2Func][eq.Expr1] = typ.Out
				}

				typ, ready := callPartials[eq.Expr2Func][eq.Expr1]
				if ready { // ready to solve
					if t, exists := solved[eq.Expr1]; exists {
						if err := t.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with call")
						}
					}
					// sub checks
					if t, exists := solved[eq.Expr2Func]; exists {
						if err := t.Out.Cmp(typ); err != nil {
							return nil, errwrap.Wrapf(err, "can't unify, invariant illogicality with call out")
						}
					}

					solved[eq.Expr1] = typ   // yay, we learned something!
					used = append(used, eqi) // mark equality as used up
					obj.Logf("%s: solved call wrap partial", Name)
					continue
				}

			// regular matching
			case *interfaces.EqualityInvariant:
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
					used = append(used, eqi) // mark equality as used up
					obj.Logf("%s: duplicate regular equality", Name)
					continue
				}
				if exists1 && !exists2 { // first equality already connects
					solved[eq.Expr2] = typ1  // yay, we learned something!
					used = append(used, eqi) // mark equality as used up
					obj.Logf("%s: solved regular equality", Name)
					continue
				}
				if exists2 && !exists1 { // second equality already connects
					solved[eq.Expr1] = typ2  // yay, we learned something!
					used = append(used, eqi) // mark equality as used up
					obj.Logf("%s: solved regular equality", Name)
					continue
				}

				panic("reached unexpected code")

			case *interfaces.GeneratorInvariant:
				// this invariant can generate new ones

				// optimization: we want to run the generators
				// last (but before the exclusives) because
				// they take longer to run. So as long as we've
				// made progress this time around, don't run
				// this just yet, there's still time left...
				if len(used) > 0 {
					continue
				}

				// skip if the inactive flag has been set, as it
				// won't produce any new (novel) inequalities we
				// can use to progress to a result at this time.
				if eq.Inactive {
					continue
				}

				// If this returns nil, we add the invariants
				// it returned and we remove it from the list.
				// If we error, it's because we don't have any
				// new information to provide at this time...
				// XXX: should we pass in `invariants` instead?
				gi, err := eq.Func(equalities, solved)
				if err != nil {
					// set the inactive flag of this generator
					eq.Inactive = true
					continue
				}

				eqs, exs, err := process(gi) // process like at the top
				if err != nil {
					// programming error?
					return nil, errwrap.Wrapf(err, "processing error")
				}
				equalities = append(equalities, eqs...)
				exclusives = append(exclusives, exs...)

				used = append(used, eqi) // mark equality as used up
				obj.Logf("%s: solved `generator` equality", Name)
				// reset all other generator equality "inactive" flags
				for _, x := range equalities {
					gen, ok := x.(*interfaces.GeneratorInvariant)
					if !ok {
						continue
					}
					gen.Inactive = false
				}

				continue

			// wtf matching
			case *interfaces.AnyInvariant:
				// this basically ensures that the expr gets solved
				if _, exists := solved[eq.Expr]; exists {
					used = append(used, eqi) // mark equality as used up
					obj.Logf("%s: solved `any` equality", Name)
				}
				continue

			case *interfaces.ValueInvariant:
				// don't consume these, they're stored in case
				// a generator invariant wants to read them...
				continue

			case *interfaces.CallFuncArgsValueInvariant:
				// don't consume these, they're stored in case
				// a generator invariant wants to read them...
				continue

			case *interfaces.SkipInvariant:
				//skipExprs[eq.Expr] = struct{}{} // save
				used = append(used, eqi) // mark equality as used up
				continue

			default:
				return nil, fmt.Errorf("unknown invariant type: %T", eqx)
			}
		} // end inner for loop
		if len(used) == 0 && activeGenerators() == 0 {
			// Looks like we're now ambiguous, but if we have any
			// exclusives, recurse into each possibility to see if
			// one of them can help solve this! first one wins. Add
			// in the exclusive to the current set of equalities!

			// To decrease the problem space, first check if we have
			// enough solutions to solve everything. If so, then we
			// don't need to solve any exclusives, and instead we
			// only need to verify that they don't conflict with the
			// found solution, which reduces the search space...

			// Another optimization that can be done before we run
			// the combinatorial exclusive solver, is we can look at
			// each exclusive, and remove the ones that already
			// match, because they don't tell us any new information
			// that we don't already know. We can also fail early
			// if anything proves we're already inconsistent.

			// These two optimizations turn out to use the exact
			// same algorithm and code, so they're combined here...
			_, isSolved := isSolvedFn(solved)
			if isSolved {
				obj.Logf("%s: solved early with %d exclusives left!", Name, len(exclusives))
			} else {
				obj.Logf("%s: unsolved with %d exclusives left!", Name, len(exclusives))
				if obj.Debug {
					for i, x := range exclusives {
						obj.Logf("%s: exclusive(%d) left: %s", Name, i, x)
					}
				}
			}

			total, active := countGenerators()
			// we still have work to do for consistency
			if active > 0 {
				continue Loop
			}

			if total > 0 {
				return nil, fmt.Errorf("%d unconsumed generators", total)
			}

			// check for consistency against remaining invariants
			obj.Logf("%s: checking for consistency against %d exclusives...", Name, len(exclusives))
			done := []int{}
			for i, invar := range exclusives {
				// test each one to see if at least one works
				match, err := invar.Matches(solved)
				if err != nil {
					obj.Logf("exclusive invar failed: %+v", invar)
					return nil, errwrap.Wrapf(err, "inconsistent exclusive")
				}
				if !match {
					continue
				}
				done = append(done, i)
			}
			obj.Logf("%s: removed %d consistent exclusives...", Name, len(done))

			// Remove exclusives that matched correctly.
			for i := len(done) - 1; i >= 0; i-- {
				ix := done[i] // delete index that was marked as done!
				exclusives = append(exclusives[:ix], exclusives[ix+1:]...)
			}

			// If we removed any exclusives, then we can start over.
			if len(done) > 0 {
				continue Loop
			}

			// If we don't have any exclusives left, then we don't
			// need the Value invariants... This logic is the same
			// as in process() but it's duplicated here because we
			// want it to happen at this stage as well. We can try
			// and clean up the duplication and improve the logic.
			// NOTE: We should probably check that there aren't any
			// generators left in the equalities, but since we have
			// already tried to use them up, it is probably safe to
			// unblock the solver if it's only ValueInvatiant left.
			if len(exclusives) == 0 || isSolved { // either is okay
				used := []int{}
				for i, x := range equalities {
					_, ok1 := x.(*interfaces.ValueInvariant)
					_, ok2 := x.(*interfaces.CallFuncArgsValueInvariant)
					if !ok1 && !ok2 {
						continue
					}
					used = append(used, i) // mark equality as used up
				}
				obj.Logf("%s: got %d equalities left after %d value invariants used up", Name, len(equalities)-len(used), len(used))
				// delete used equalities, in reverse order to preserve indexing!
				for i := len(used) - 1; i >= 0; i-- {
					ix := used[i] // delete index that was marked as used!
					equalities = append(equalities[:ix], equalities[ix+1:]...)
				}

				if len(used) > 0 {
					continue Loop
				}
			}

			if len(exclusives) == 0 && isSolved { // old generators
				used := []int{}
				for i, x := range equalities {
					_, ok := x.(*interfaces.GeneratorInvariant)
					if !ok {
						continue
					}
					used = append(used, i) // mark equality as used up
				}
				obj.Logf("%s: got %d equalities left after %d generators used up", Name, len(equalities)-len(used), len(used))
				// delete used equalities, in reverse order to preserve indexing!
				for i := len(used) - 1; i >= 0; i-- {
					ix := used[i] // delete index that was marked as used!
					equalities = append(equalities[:ix], equalities[ix+1:]...)
				}

				if len(used) > 0 {
					continue Loop
				}
			}

			// what have we learned for sure so far?
			partialSolutions := []interfaces.Invariant{}
			obj.Logf("%s: %d solved, %d unsolved, and %d exclusives left", Name, len(solved), len(equalities), len(exclusives))
			if len(exclusives) > 0 {
				// FIXME: can we do this loop in a deterministic, sorted way?
				for expr, typ := range solved {
					invar := &interfaces.EqualsInvariant{
						Expr: expr,
						Type: typ,
					}
					partialSolutions = append(partialSolutions, invar)
					obj.Logf("%s: solved: %+v", Name, invar)
				}

				// also include anything that hasn't been solved yet
				for _, x := range equalities {
					partialSolutions = append(partialSolutions, x)
					obj.Logf("%s: unsolved: %+v", Name, x)
				}
			}
			obj.Logf("%s: solver state:\n%s", Name, unification.DebugSolverState(solved, equalities))

			// Lastly, we could loop through each exclusive and see
			// if it only has a single, easy solution. For example,
			// if we know that an exclusive is A or B or C, and that
			// B and C are inconsistent, then we can replace the
			// exclusive with a single invariant and then run that
			// through our solver. We can do this iteratively
			// (recursively for accuracy, but in our case via the
			// simplify method) so that if we're lucky, we rarely
			// need to run the raw exclusive combinatorial solver,
			// which is slow.
			obj.Logf("%s: attempting to simplify %d exclusives...", Name, len(exclusives))

			done = []int{} // clear for re-use
			simplified := []interfaces.Invariant{}
			for i, invar := range exclusives {
				// The partialSolutions don't contain any other
				// exclusives... We look at each individually.
				s, err := invar.Simplify(partialSolutions) // XXX: pass in the solver?
				if err != nil {
					obj.Logf("exclusive simplification failed: %+v", invar)
					continue
				}
				done = append(done, i)
				simplified = append(simplified, s...)
			}
			obj.Logf("%s: simplified %d exclusives...", Name, len(done))

			// Remove exclusives that matched correctly.
			for i := len(done) - 1; i >= 0; i-- {
				ix := done[i] // delete index that was marked as done!
				exclusives = append(exclusives[:ix], exclusives[ix+1:]...)
			}

			// Add new equalities and exclusives onto state globals.
			eqs, exs, err := process(simplified) // process like at the top
			if err != nil {
				// programming error?
				return nil, errwrap.Wrapf(err, "processing error")
			}
			equalities = append(equalities, eqs...)
			exclusives = append(exclusives, exs...)

			// If we removed any exclusives, then we can start over.
			if len(done) > 0 {
				continue Loop
			}

			// TODO: We could try and replace our combinatorial
			// exclusive solver with a real SAT solver algorithm.

			if !AllowRecursion || len(exclusives) > RecursionInvariantLimit {
				obj.Logf("%s: %d solved, %d unsolved, and %d exclusives left", Name, len(solved), len(equalities), len(exclusives))
				for i, eq := range equalities {
					obj.Logf("%s: (%d) equality: %s", Name, i, eq)
				}
				for i, ex := range exclusives {
					obj.Logf("%s: (%d) exclusive: %s", Name, i, ex)
				}

				// these can be very slow, so try to avoid them
				return nil, fmt.Errorf("only recursive solutions left")
			}

			// let's try each combination, one at a time...
			for i, ex := range unification.ExclusivesProduct(exclusives) { // [][]interfaces.Invariant
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					// pass
				}
				obj.Logf("%s: exclusive(%d):\n%+v", Name, i, ex)
				// we could waste a lot of cpu, and start from
				// the beginning, but instead we could use the
				// list of known solutions found and continue!
				// TODO: make sure none of these edit partialSolutions
				recursiveInvariants := []interfaces.Invariant{}
				recursiveInvariants = append(recursiveInvariants, partialSolutions...)
				recursiveInvariants = append(recursiveInvariants, ex...)
				// FIXME: implement RecursionDepthLimit
				obj.Logf("%s: recursing...", Name)
				solution, err := obj.Solve(ctx, recursiveInvariants, expected)
				if err != nil {
					obj.Logf("%s: recursive solution failed: %+v", Name, err)
					continue // no solution found here...
				}
				// solution found!
				obj.Logf("%s: recursive solution found!", Name)
				return solution, nil
			}

			// TODO: print ambiguity
			obj.Logf("%s: ================ ambiguity ================", Name)
			unsolved, isSolved := isSolvedFn(solved)
			obj.Logf("%s: isSolved: %+v", Name, isSolved)
			for _, x := range equalities {
				obj.Logf("%s: unsolved equality: %+v", Name, x)
			}
			for x := range unsolved {
				obj.Logf("%s: unsolved expected: (%p) %+v", Name, x, x)
			}
			for expr, typ := range solved {
				obj.Logf("%s: solved: (%p) => %+v", Name, expr, typ)
			}
			return nil, unification.ErrAmbiguous
		}
		// delete used equalities, in reverse order to preserve indexing!
		for i := len(used) - 1; i >= 0; i-- {
			ix := used[i] // delete index that was marked as used!
			equalities = append(equalities[:ix], equalities[ix+1:]...)
		}
	} // end giant for loop

	// build final solution
	solutions := []*interfaces.EqualsInvariant{}
	// FIXME: can we do this loop in a deterministic, sorted way?
	for expr, typ := range solved {
		// Don't do this here, or the current Unifier struct machinery
		// will see it as a bug. Do it there until we change the API.
		//if _, exists := skipExprs[expr]; exists {
		//	continue
		//}

		invar := &interfaces.EqualsInvariant{
			Expr: expr,
			Type: typ,
		}
		solutions = append(solutions, invar)
	}
	obj.Logf("zTotal: %d", obj.zTotal)
	return &unification.InvariantSolution{
		Solutions: solutions,
	}, nil
}
