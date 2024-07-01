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

package unification

import (
	"context"
	"fmt"
	"sort"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ErrAmbiguous means we couldn't find a solution, but we weren't
	// inconsistent.
	ErrAmbiguous = interfaces.Error("can't unify, no equalities were consumed, we're ambiguous")

	// StrategyNameKey is the string key used when choosing a solver name.
	StrategyNameKey = "name"

	// StrategyOptimizationsKey is the string key used to tell the solver
	// about the specific optimizations you'd like to request. The format
	// can be specific to each solver.
	StrategyOptimizationsKey = "optimizations"
)

// Init contains some handles that are used to initialize every solver. Each
// individual solver can choose to omit using some of the fields.
type Init struct {
	// Strategy is a hack to tune unification performance until we have an
	// overall cleaner unification algorithm in place.
	Strategy map[string]string

	// UnifiedState stores a common representation of our unification vars.
	UnifiedState *types.UnifiedState

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Solver is the general interface that any solver needs to implement.
type Solver interface {
	// Init initializes the solver struct before first use.
	Init(*Init) error

	// Solve performs the actual solving. It must return as soon as possible
	// if the context is closed.
	Solve(ctx context.Context, invariants []interfaces.Invariant, expected []interfaces.Expr) (*InvariantSolution, error)
}

// registeredSolvers is a global map of all possible unification solvers which
// can be used. You should never touch this map directly. Use methods like
// Register instead.
var registeredSolvers = make(map[string]func() Solver) // must initialize

// Register takes a solver and its name and makes it available for use. It is
// commonly called in the init() method of the solver at program startup. There
// is no matching Unregister function.
func Register(name string, solver func() Solver) {
	if _, exists := registeredSolvers[name]; exists {
		panic(fmt.Sprintf("a solver named %s is already registered", name))
	}

	//gob.Register(solver())
	registeredSolvers[name] = solver
}

// Lookup returns a pointer to the solver's struct.
func Lookup(name string) (Solver, error) {
	solver, exists := registeredSolvers[name]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return solver(), nil
}

// LookupDefault attempts to return a "default" solver.
func LookupDefault() (Solver, error) {
	if len(registeredSolvers) == 0 {
		return nil, fmt.Errorf("no registered solvers")
	}
	if len(registeredSolvers) == 1 {
		for _, solver := range registeredSolvers {
			return solver(), nil // return the first and only one
		}
	}

	// TODO: Should we remove this empty string feature?
	// If one was registered with no name, then use that as the default.
	if solver, exists := registeredSolvers[""]; exists { // empty name
		return solver(), nil
	}

	return nil, fmt.Errorf("no registered default solver")
}

// DebugSolverState helps us in understanding the state of the type unification
// solver in a more mainstream format.
// Example:
//
// solver state:
//
// *	str("foo") :: str
// *	call:f(str("foo")) [0xc000ac9f10] :: ?1
// *	var(x) [0xc00088d840] :: ?2
// *	param(x) [0xc00000f950] :: ?3
// *	func(x) { var(x) } [0xc0000e9680] :: ?4
// *	?2 = ?3
// *	?4 = func(arg0 str) ?1
// *	?4 = func(x str) ?2
// *	?1 = ?2
func DebugSolverState(solved map[interfaces.Expr]*types.Type, equalities []interfaces.Invariant) string {
	s := ""

	// all the relevant Exprs
	count := 0
	exprs := make(map[interfaces.Expr]int)
	for _, equality := range equalities {
		for _, expr := range equality.ExprList() {
			count++
			exprs[expr] = count // for sorting
		}
	}

	// print the solved Exprs first
	for expr, typ := range solved {
		s += fmt.Sprintf("%v :: %v\n", expr, typ)
		delete(exprs, expr)
	}

	sortedExprs := []interfaces.Expr{}
	for k := range exprs {
		sortedExprs = append(sortedExprs, k)
	}
	sort.Slice(sortedExprs, func(i, j int) bool { return exprs[sortedExprs[i]] < exprs[sortedExprs[j]] })

	// for each remaining expr, generate a shorter name than the full pointer
	nextVar := 1
	shortNames := map[interfaces.Expr]string{}
	for _, expr := range sortedExprs {
		shortNames[expr] = fmt.Sprintf("?%d", nextVar)
		nextVar++
		s += fmt.Sprintf("%p %v :: %s\n", expr, expr, shortNames[expr])
	}

	// print all the equalities using the short names
	for _, equality := range equalities {
		switch e := equality.(type) {
		case *interfaces.EqualsInvariant:
			_, ok := solved[e.Expr]
			if !ok {
				s += fmt.Sprintf("%s = %v\n", shortNames[e.Expr], e.Type)
			} else {
				// if solved, then this is redundant, don't print anything
			}

		case *interfaces.EqualityInvariant:
			type1, ok1 := solved[e.Expr1]
			type2, ok2 := solved[e.Expr2]
			if !ok1 && !ok2 {
				s += fmt.Sprintf("%s = %s\n", shortNames[e.Expr1], shortNames[e.Expr2])
			} else if ok1 && !ok2 {
				s += fmt.Sprintf("%s = %s\n", type1, shortNames[e.Expr2])
			} else if !ok1 && ok2 {
				s += fmt.Sprintf("%s = %s\n", shortNames[e.Expr1], type2)
			} else {
				// if completely solved, then this is redundant, don't print anything
			}

		case *interfaces.EqualityWrapFuncInvariant:
			funcType, funcOk := solved[e.Expr1]

			args := ""
			argsOk := true
			for i, argName := range e.Expr2Ord {
				if i > 0 {
					args += ", "
				}
				argExpr := e.Expr2Map[argName]
				argType, ok := solved[argExpr]
				if !ok {
					args += fmt.Sprintf("%s %s", argName, shortNames[argExpr])
					argsOk = false
				} else {
					args += fmt.Sprintf("%s %s", argName, argType)
				}
			}

			outType, outOk := solved[e.Expr2Out]

			if !funcOk || !argsOk || !outOk {
				if !funcOk && !outOk {
					s += fmt.Sprintf("%s = func(%s) %s\n", shortNames[e.Expr1], args, shortNames[e.Expr2Out])
				} else if !funcOk && outOk {
					s += fmt.Sprintf("%s = func(%s) %s\n", shortNames[e.Expr1], args, outType)
				} else if funcOk && !outOk {
					s += fmt.Sprintf("%s = func(%s) %s\n", funcType, args, shortNames[e.Expr2Out])
				} else {
					s += fmt.Sprintf("%s = func(%s) %s\n", funcType, args, outType)
				}
			}

		case *interfaces.CallFuncArgsValueInvariant:
			// skip, not used in the examples I care about

		case *interfaces.AnyInvariant:
			// skip, not used in the examples I care about

		case *interfaces.SkipInvariant:
			// we don't care about this one

		default:
			s += fmt.Sprintf("%v\n", equality)
		}
	}

	return s
}
