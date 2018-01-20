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

package unification

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

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
func Unify(ast interfaces.Stmt, solver func([]interfaces.Invariant) (*InvariantSolution, error)) error {
	//log.Printf("unification: tree: %+v", ast) // debug
	if ast == nil {
		return fmt.Errorf("AST is nil")
	}

	invariants, err := ast.Unify()
	if err != nil {
		return err
	}

	solved, err := solver(invariants)
	if err != nil {
		return err
	}

	// TODO: ideally we would know how many different expressions need their
	// types set in the AST and then ensure we have this many unique
	// solutions, and if not, then fail. This would ensure we don't have an
	// AST that is only partially populated with the correct types.

	//log.Printf("unification: found a solution!") // TODO: get a logf function passed in...
	// solver has found a solution, apply it...
	// we're modifying the AST, so code can't error now...
	for _, x := range solved.Solutions {
		//log.Printf("unification: solution: %p => %+v\t(%+v)", x.Expr, x.Type, x.Expr.String()) // debug
		// apply this to each AST node
		if err := x.Expr.SetType(x.Type); err != nil {
			// programming error!
			panic(fmt.Sprintf("error setting type: %+v, error: %+v", x.Expr, err))
		}
	}
	return nil
}

// EqualsInvariant is an invariant that symbolizes that the expression has a
// known type.
// TODO: is there a better name than EqualsInvariant
type EqualsInvariant struct {
	Expr interfaces.Expr
	Type *types.Type
}

// String returns a representation of this invariant.
func (obj *EqualsInvariant) String() string {
	return fmt.Sprintf("%p == %s", obj.Expr, obj.Type)
}

// EqualityInvariant is an invariant that symbolizes that the two expressions
// must have equivalent types.
// TODO: is there a better name than EqualityInvariant
type EqualityInvariant struct {
	Expr1 interfaces.Expr
	Expr2 interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *EqualityInvariant) String() string {
	return fmt.Sprintf("%p == %p", obj.Expr1, obj.Expr2)
}

// EqualityInvariantList is an invariant that symbolizes that all the
// expressions listed must have equivalent types.
type EqualityInvariantList struct {
	Exprs []interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *EqualityInvariantList) String() string {
	var a []string
	for _, x := range obj.Exprs {
		a = append(a, fmt.Sprintf("%p", x))
	}
	return fmt.Sprintf("[%s]", strings.Join(a, ", "))
}

// EqualityWrapListInvariant expresses that a list in Expr1 must have elements
// that have the same type as the expression in Expr2Val.
type EqualityWrapListInvariant struct {
	Expr1    interfaces.Expr
	Expr2Val interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapListInvariant) String() string {
	return fmt.Sprintf("%p == [%p]", obj.Expr1, obj.Expr2Val)
}

// EqualityWrapMapInvariant expresses that a map in Expr1 must have keys that
// match the type of the expression in Expr2Key and values that match the type
// of the expression in Expr2Val.
type EqualityWrapMapInvariant struct {
	Expr1    interfaces.Expr
	Expr2Key interfaces.Expr
	Expr2Val interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapMapInvariant) String() string {
	return fmt.Sprintf("%p == {%p: %p}", obj.Expr1, obj.Expr2Key, obj.Expr2Val)
}

// EqualityWrapStructInvariant expresses that a struct in Expr1 must have fields
// that match the type of the expressions listed in Expr2Map.
type EqualityWrapStructInvariant struct {
	Expr1    interfaces.Expr
	Expr2Map map[string]interfaces.Expr
	Expr2Ord []string
}

// String returns a representation of this invariant.
func (obj *EqualityWrapStructInvariant) String() string {
	var s = make([]string, len(obj.Expr2Ord))
	for i, k := range obj.Expr2Ord {
		t, ok := obj.Expr2Map[k]
		if !ok {
			panic("malformed struct order")
		}
		if t == nil {
			panic("malformed struct field")
		}
		s[i] = fmt.Sprintf("%s %p", k, t)
	}
	return fmt.Sprintf("%p == struct{%s}", obj.Expr1, strings.Join(s, "; "))
}

// EqualityWrapFuncInvariant expresses that a func in Expr1 must have args that
// match the type of the expressions listed in Expr2Map and a return value that
// matches the type of the expression in Expr2Out.
// TODO: should this be named EqualityWrapCallInvariant or not?
type EqualityWrapFuncInvariant struct {
	Expr1    interfaces.Expr
	Expr2Map map[string]interfaces.Expr
	Expr2Ord []string
	Expr2Out interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapFuncInvariant) String() string {
	var s = make([]string, len(obj.Expr2Ord))
	for i, k := range obj.Expr2Ord {
		t, ok := obj.Expr2Map[k]
		if !ok {
			panic("malformed func order")
		}
		if t == nil {
			panic("malformed func field")
		}
		s[i] = fmt.Sprintf("%s %p", k, t)
	}
	return fmt.Sprintf("%p == func{%s} %p", obj.Expr1, strings.Join(s, "; "), obj.Expr2Out)
}

// ConjunctionInvariant represents a list of invariants which must all be true
// together. In other words, it's a grouping construct for a set of invariants.
type ConjunctionInvariant struct {
	Invariants []interfaces.Invariant
}

// String returns a representation of this invariant.
func (obj *ConjunctionInvariant) String() string {
	var a []string
	for _, x := range obj.Invariants {
		s := x.String()
		a = append(a, s)
	}
	return fmt.Sprintf("[%s]", strings.Join(a, ", "))
}

// ExclusiveInvariant represents a list of invariants where one and *only* one
// should hold true. To combine multiple invariants in one of the list elements,
// you can group multiple invariants together using a ConjunctionInvariant. Do
// note that the solver might not verify that only one of the invariants in the
// list holds true, as it might choose to be lazy and pick the first solution
// found.
type ExclusiveInvariant struct {
	Invariants []interfaces.Invariant
}

// String returns a representation of this invariant.
func (obj *ExclusiveInvariant) String() string {
	var a []string
	for _, x := range obj.Invariants {
		s := x.String()
		a = append(a, s)
	}
	return fmt.Sprintf("[%s]", strings.Join(a, ", "))
}

// exclusivesProduct returns a list of different products produced from the
// combinatorial product of the list of exclusives. Each ExclusiveInvariant
// must contain between one and more Invariants. This takes every combination of
// Invariants (choosing one from each ExclusiveInvariant) and returns that list.
// In other words, if you have three exclusives, with invariants named (A1, B1),
// (A2), and (A3, B3, C3) you'll get: (A1, A2, A3), (A1, A2, B3), (A1, A2, C3),
// (B1, A2, A3), (B1, A2, B3), (B1, A2, C3) as results for this function call.
func exclusivesProduct(exclusives []*ExclusiveInvariant) [][]interfaces.Invariant {
	if len(exclusives) == 0 {
		return nil
	}

	length := func(i int) int { return len(exclusives[i].Invariants) }

	// NextIx sets ix to the lexicographically next value,
	// such that for each i > 0, 0 <= ix[i] < length(i).
	NextIx := func(ix []int) {
		for i := len(ix) - 1; i >= 0; i-- {
			ix[i]++
			if i == 0 || ix[i] < length(i) {
				return
			}
			ix[i] = 0
		}
	}

	results := [][]interfaces.Invariant{}

	for ix := make([]int, len(exclusives)); ix[0] < length(0); NextIx(ix) {
		x := []interfaces.Invariant{}
		for j, k := range ix {
			x = append(x, exclusives[j].Invariants[k])
		}
		results = append(results, x)
	}

	return results
}

// AnyInvariant is an invariant that symbolizes that the expression can be any
// type. It is sometimes used to ensure that an expr actually gets a solution
// type so that it is not left unreferenced, and as a result, unsolved.
// TODO: is there a better name than AnyInvariant
type AnyInvariant struct {
	Expr interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *AnyInvariant) String() string {
	return fmt.Sprintf("%p == *", obj.Expr)
}

// InvariantSolution lists a trivial set of EqualsInvariant mappings so that you
// can populate your AST with SetType calls in a simple loop.
type InvariantSolution struct {
	Solutions []*EqualsInvariant // list of trivial solutions for each node
}
