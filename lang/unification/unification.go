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
	"fmt"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Unifier holds all the data that the Unify function will need for it to run.
type Unifier struct {
	// AST is the input abstract syntax tree to unify.
	AST interfaces.Stmt

	// Solver is the solver algorithm implementation to use.
	Solver func([]interfaces.Invariant, []interfaces.Expr) (*InvariantSolution, error)

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
func (obj *Unifier) Unify() error {
	if obj.AST == nil {
		return fmt.Errorf("the AST is nil")
	}
	if obj.Solver == nil {
		return fmt.Errorf("the Solver is missing")
	}
	if obj.Logf == nil {
		return fmt.Errorf("the Logf function is missing")
	}

	if obj.Debug {
		obj.Logf("tree: %+v", obj.AST)
	}
	invariants, err := obj.AST.Unify()
	if err != nil {
		return err
	}

	// build a list of what we think we need to solve for to succeed
	exprs := []interfaces.Expr{}
	for _, x := range invariants {
		exprs = append(exprs, x.ExprList()...)
	}
	exprMap := ExprListToExprMap(exprs)    // makes searching faster
	exprList := ExprMapToExprList(exprMap) // makes it unique (no duplicates)

	solved, err := obj.Solver(invariants, exprList)
	if err != nil {
		return err
	}

	// determine what expr's we need to solve for
	if obj.Debug {
		obj.Logf("expr count: %d", len(exprList))
		//for _, x := range exprList {
		//	obj.Logf("> %p (%+v)", x, x)
		//}
	}

	// XXX: why doesn't `len(exprList)` always == `len(solved.Solutions)` ?
	// XXX: is it due to the extra ExprAny ??? I see an extra function sometimes...

	if obj.Debug {
		obj.Logf("solutions count: %d", len(solved.Solutions))
		//for _, x := range solved.Solutions {
		//	obj.Logf("> %p (%+v) -- %s", x.Expr, x.Type, x.Expr.String())
		//}
	}

	// Determine that our solver produced a solution for every expr that
	// we're interested in. If it didn't, and it didn't error, then it's a
	// bug. We check for this because we care about safety, this ensures
	// that our AST will get fully populated with the correct types!
	for _, x := range solved.Solutions {
		delete(exprMap, x.Expr) // remove everything we know about
	}
	if c := len(exprMap); c > 0 { // if there's anything left, it's bad...
		ptrs := []string{}
		disp := make(map[string]string) // display hack
		for i := range exprMap {
			s := fmt.Sprintf("%p", i) // pointer
			ptrs = append(ptrs, s)
			disp[s] = i.String()
		}
		sort.Strings(ptrs)
		// programming error!
		s := strings.Join(ptrs, ", ")

		obj.Logf("got %d unbound expr's: %s", c, s)
		for i, s := range ptrs {
			obj.Logf("(%d) %s => %s", i, s, disp[s])
		}
		return fmt.Errorf("got %d unbound expr's: %s", c, s)
	}

	if obj.Debug {
		obj.Logf("found a solution!")
	}
	// solver has found a solution, apply it...
	// we're modifying the AST, so code can't error now...
	for _, x := range solved.Solutions {
		if obj.Debug {
			obj.Logf("solution: %p => %+v\t(%+v)", x.Expr, x.Type, x.Expr.String())
		}
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualsInvariant) ExprList() []interfaces.Expr {
	return []interfaces.Expr{obj.Expr}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualsInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	typ, exists := solved[obj.Expr]
	if !exists {
		return false, nil
	}
	if err := typ.Cmp(obj.Type); err != nil {
		return false, err
	}
	return true, nil
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
func (obj *EqualsInvariant) Possible(partials []interfaces.Invariant) error {
	// TODO: we could pass in a solver here
	//set := []interfaces.Invariant{}
	//set = append(set, obj)
	//set = append(set, partials...)
	//_, err := SimpleInvariantSolver(set, ...)
	//if err != nil {
	//	// being ambiguous doesn't guarantee that we're possible
	//	if err == ErrAmbiguous {
	//		return nil // might be possible, might not be...
	//	}
	//	return err
	//}

	// FIXME: This is not right because we want to know if the whole thing
	// works together, and as a result, the above solver is better, however,
	// the goal is to eliminate easy impossible solutions, so allow this!
	// XXX: Double check this is logical.
	solved := map[interfaces.Expr]*types.Type{
		obj.Expr: obj.Type,
	}
	for _, invar := range partials { // check each one
		_, err := invar.Matches(solved)
		if err != nil { // inconsistent, so it's not possible
			return errwrap.Wrapf(err, "not possible")
		}
	}

	return nil
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityInvariant) ExprList() []interfaces.Expr {
	return []interfaces.Expr{obj.Expr1, obj.Expr2}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	t1, exists1 := solved[obj.Expr1]
	t2, exists2 := solved[obj.Expr2]
	if !exists1 || !exists2 {
		return false, nil // not matched yet
	}
	if err := t1.Cmp(t2); err != nil {
		return false, err
	}

	return true, nil // matched!
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
func (obj *EqualityInvariant) Possible(partials []interfaces.Invariant) error {
	// The idea here is that we look for the expression pointers in the list
	// of partial invariants. It's only impossible if we (1) find both of
	// them, and (2) that they relate to each other. The second part is
	// harder.
	var one, two bool
	exprs := []interfaces.Invariant{}
	for _, x := range partials {
		for _, y := range x.ExprList() { // []interfaces.Expr
			if y == obj.Expr1 {
				one = true
				exprs = append(exprs, x)
			}
			if y == obj.Expr2 {
				two = true
				exprs = append(exprs, x)
			}
		}
	}

	if !one || !two {
		return nil // we're unconnected to anything, this is possible!
	}

	// we only need to check the connections in this case...
	// let's keep this simple, and less perfect for now...
	var typ *types.Type
	for _, x := range exprs {
		eq, ok := x.(*EqualsInvariant)
		if !ok {
			// XXX: add support for other kinds in the future...
			continue
		}

		if typ != nil {
			if err := typ.Cmp(eq.Type); err != nil {
				// we found proof it's not possible
				return errwrap.Wrapf(err, "not possible")
			}
		}

		typ = eq.Type // store for next type
	}

	return nil
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityInvariantList) ExprList() []interfaces.Expr {
	return obj.Exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityInvariantList) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	found := true // assume true
	var typ *types.Type
	for _, x := range obj.Exprs {
		t, exists := solved[x]
		if !exists {
			found = false
			continue
		}
		if typ == nil { // set the first time
			typ = t
		}
		if err := typ.Cmp(t); err != nil {
			return false, err
		}
	}
	return found, nil
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
func (obj *EqualityInvariantList) Possible(partials []interfaces.Invariant) error {
	// The idea here is that we look for the expression pointers in the list
	// of partial invariants. It's only impossible if we (1) find two or
	// more, and (2) that any of them relate to each other. The second part
	// is harder.
	inList := func(needle interfaces.Expr, haystack []interfaces.Expr) bool {
		for _, x := range haystack {
			if x == needle {
				return true
			}
		}
		return false
	}

	exprs := []interfaces.Invariant{}
	for _, x := range partials {
		for _, y := range x.ExprList() { // []interfaces.Expr
			if inList(y, obj.Exprs) {
				exprs = append(exprs, x)
			}
		}
	}

	if len(exprs) <= 1 {
		return nil // we're unconnected to anything, this is possible!
	}

	// we only need to check the connections in this case...
	// let's keep this simple, and less perfect for now...
	var typ *types.Type
	for _, x := range exprs {
		eq, ok := x.(*EqualsInvariant)
		if !ok {
			// XXX: add support for other kinds in the future...
			continue
		}

		if typ != nil {
			if err := typ.Cmp(eq.Type); err != nil {
				// we found proof it's not possible
				return errwrap.Wrapf(err, "not possible")
			}
		}

		typ = eq.Type // store for next type
	}

	return nil
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapListInvariant) ExprList() []interfaces.Expr {
	return []interfaces.Expr{obj.Expr1, obj.Expr2Val}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapListInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	t1, exists1 := solved[obj.Expr1] // list type
	t2, exists2 := solved[obj.Expr2Val]
	if !exists1 || !exists2 {
		return false, nil // not matched yet
	}
	if t1.Kind != types.KindList {
		return false, fmt.Errorf("expected list kind")
	}
	if err := t1.Val.Cmp(t2); err != nil {
		return false, err // inconsistent!
	}
	return true, nil // matched!
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *EqualityWrapListInvariant) Possible(partials []interfaces.Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapMapInvariant) ExprList() []interfaces.Expr {
	return []interfaces.Expr{obj.Expr1, obj.Expr2Key, obj.Expr2Val}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapMapInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	t1, exists1 := solved[obj.Expr1] // map type
	t2, exists2 := solved[obj.Expr2Key]
	t3, exists3 := solved[obj.Expr2Val]
	if !exists1 || !exists2 || !exists3 {
		return false, nil // not matched yet
	}
	if t1.Kind != types.KindMap {
		return false, fmt.Errorf("expected map kind")
	}
	if err := t1.Key.Cmp(t2); err != nil {
		return false, err // inconsistent!
	}
	if err := t1.Val.Cmp(t3); err != nil {
		return false, err // inconsistent!
	}
	return true, nil // matched!
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *EqualityWrapMapInvariant) Possible(partials []interfaces.Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapStructInvariant) ExprList() []interfaces.Expr {
	exprs := []interfaces.Expr{obj.Expr1}
	for _, x := range obj.Expr2Map {
		exprs = append(exprs, x)
	}
	return exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapStructInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	t1, exists1 := solved[obj.Expr1] // struct type
	if !exists1 {
		return false, nil // not matched yet
	}
	if t1.Kind != types.KindStruct {
		return false, fmt.Errorf("expected struct kind")
	}

	found := true // assume true
	for _, key := range obj.Expr2Ord {
		_, exists := t1.Map[key]
		if !exists {
			return false, fmt.Errorf("missing invariant struct key of: `%s`", key)
		}
		e, exists := obj.Expr2Map[key]
		if !exists {
			return false, fmt.Errorf("missing matched struct key of: `%s`", key)
		}
		t, exists := solved[e]
		if !exists {
			found = false
			continue
		}
		if err := t1.Map[key].Cmp(t); err != nil {
			return false, err // inconsistent!
		}
	}

	return found, nil // matched!
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *EqualityWrapStructInvariant) Possible(partials []interfaces.Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
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
	return fmt.Sprintf("%p == func(%s) %p", obj.Expr1, strings.Join(s, "; "), obj.Expr2Out)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapFuncInvariant) ExprList() []interfaces.Expr {
	exprs := []interfaces.Expr{obj.Expr1}
	for _, x := range obj.Expr2Map {
		exprs = append(exprs, x)
	}
	exprs = append(exprs, obj.Expr2Out)
	return exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapFuncInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	t1, exists1 := solved[obj.Expr1] // func type
	if !exists1 {
		return false, nil // not matched yet
	}
	if t1.Kind != types.KindFunc {
		return false, fmt.Errorf("expected func kind")
	}

	found := true // assume true
	for _, key := range obj.Expr2Ord {
		_, exists := t1.Map[key]
		if !exists {
			return false, fmt.Errorf("missing invariant struct key of: `%s`", key)
		}
		e, exists := obj.Expr2Map[key]
		if !exists {
			return false, fmt.Errorf("missing matched struct key of: `%s`", key)
		}
		t, exists := solved[e]
		if !exists {
			found = false
			continue
		}
		if err := t1.Map[key].Cmp(t); err != nil {
			return false, err // inconsistent!
		}
	}

	t, exists := solved[obj.Expr2Out]
	if !exists {
		return false, nil
	}
	if err := t1.Out.Cmp(t); err != nil {
		return false, err // inconsistent!
	}

	return found, nil // matched!
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *EqualityWrapFuncInvariant) Possible(partials []interfaces.Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}

// EqualityWrapCallInvariant expresses that a call result that happened in Expr1
// must match the type of the function result listed in Expr2. In this case,
// Expr2 will be a function expression, and the returned expression should match
// with the Expr1 expression, when comparing types.
// TODO: should this be named EqualityWrapFuncInvariant or not?
// TODO: should Expr1 and Expr2 be reversed???
type EqualityWrapCallInvariant struct {
	Expr1     interfaces.Expr
	Expr2Func interfaces.Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapCallInvariant) String() string {
	return fmt.Sprintf("%p == call(%p)", obj.Expr1, obj.Expr2Func)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapCallInvariant) ExprList() []interfaces.Expr {
	return []interfaces.Expr{obj.Expr1, obj.Expr2Func}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapCallInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	t1, exists1 := solved[obj.Expr1] // call type
	t2, exists2 := solved[obj.Expr2Func]
	if !exists1 || !exists2 {
		return false, nil // not matched yet
	}
	//if t1.Kind != types.KindFunc {
	//	return false, fmt.Errorf("expected func kind")
	//}

	if t2.Kind != types.KindFunc {
		return false, fmt.Errorf("expected func kind")
	}
	if err := t1.Cmp(t2.Out); err != nil {
		return false, err // inconsistent!
	}
	return true, nil // matched!
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *EqualityWrapCallInvariant) Possible(partials []interfaces.Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *ConjunctionInvariant) ExprList() []interfaces.Expr {
	exprs := []interfaces.Expr{}
	for _, x := range obj.Invariants {
		exprs = append(exprs, x.ExprList()...)
	}
	return exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *ConjunctionInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	found := true // assume true
	for _, invar := range obj.Invariants {
		match, err := invar.Matches(solved)
		if err != nil {
			return false, nil
		}
		if !match {
			found = false
		}
	}
	return found, nil
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *ConjunctionInvariant) Possible(partials []interfaces.Invariant) error {
	for _, invar := range obj.Invariants {
		if err := invar.Possible(partials); err != nil {
			// we found proof it's not possible
			return errwrap.Wrapf(err, "not possible")
		}
	}
	// XXX: unfortunately we didn't look for them all together with a solver
	return nil
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *ExclusiveInvariant) ExprList() []interfaces.Expr {
	// XXX: We should do this if we assume that exclusives don't have some
	// sort of transient expr to satisfy that doesn't disappear depending on
	// which choice in the exclusive is chosen...
	//exprs := []interfaces.Expr{}
	//for _, x := range obj.Invariants {
	//	exprs = append(exprs, x.ExprList()...)
	//}
	//return exprs
	// XXX: But if we ever specify an expr in this exclusive that isn't
	// referenced anywhere else, then we'd need to use the above so that our
	// type unification algorithm knows not to stop too early.
	return []interfaces.Expr{} // XXX: Do we want to the set instead?
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors. Because this partial invariant requires only
// one to be true, it will mask children errors, since it's normal for only one
// to be consistent.
func (obj *ExclusiveInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	found := false
	reterr := fmt.Errorf("all exclusives errored")
	var errs error
	for _, invar := range obj.Invariants {
		match, err := invar.Matches(solved)
		if err != nil {
			errs = errwrap.Append(errs, err)
			continue
		}
		if !match {
			// at least one was false, so we're not done here yet...
			// we don't want to error yet, since we can't know there
			// won't be a conflict once we get more data about this!
			reterr = nil // clear the error
			continue
		}
		if found { // we already found one
			return false, fmt.Errorf("more than one exclusive solution")
		}
		found = true
	}

	if found { // we got exactly one valid solution
		return true, nil
	}

	return false, errwrap.Wrapf(reterr, errwrap.String(errs))
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *ExclusiveInvariant) Possible(partials []interfaces.Invariant) error {
	var errs error
	for _, invar := range obj.Invariants {
		err := invar.Possible(partials)
		if err == nil {
			// we found proof it's possible
			return nil
		}
		errs = errwrap.Append(errs, err)
	}

	return errwrap.Wrapf(errs, "not possible")
}

// simplify attempts to reduce the exclusive invariant to eliminate any
// possibilities based on the list of known partials at this time. Hopefully,
// this will weed out some of the function polymorphism possibilities so that we
// can solve the problem without recursive, combinatorial permutation, which is
// very, very slow.
func (obj *ExclusiveInvariant) simplify(partials []interfaces.Invariant) ([]interfaces.Invariant, error) {
	if len(obj.Invariants) == 0 { // unexpected case
		return []interfaces.Invariant{}, nil // we don't need anything!
	}

	possible := []interfaces.Invariant{}
	var reasons error
	for _, invar := range obj.Invariants { // []interfaces.Invariant
		if err := invar.Possible(partials); err != nil {
			reasons = errwrap.Append(reasons, err)
			continue
		}
		possible = append(possible, invar)
	}

	if len(possible) == 0 { // nothing was possible
		return nil, errwrap.Wrapf(reasons, "no possible simplifications")
	}
	if len(possible) == 1 { // we flattened out the exclusive!
		return possible, nil
	}

	if len(possible) == len(obj.Invariants) { // nothing changed
		return nil, fmt.Errorf("no possible simplifications, we're unchanged")
	}

	invar := &ExclusiveInvariant{
		Invariants: possible, // hopefully a smaller exclusive!
	}
	return []interfaces.Invariant{invar}, nil
}

// exclusivesProduct returns a list of different products produced from the
// combinatorial product of the list of exclusives. Each ExclusiveInvariant must
// contain between one and more Invariants. This takes every combination of
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

// ExprList returns the list of valid expressions in this invariant.
func (obj *AnyInvariant) ExprList() []interfaces.Expr {
	return []interfaces.Expr{obj.Expr}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *AnyInvariant) Matches(solved map[interfaces.Expr]*types.Type) (bool, error) {
	_, exists := solved[obj.Expr] // we only care that it is found.
	return exists, nil
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation always returns nil.
func (obj *AnyInvariant) Possible([]interfaces.Invariant) error {
	// keep it simple, even though we don't technically check the inputs...
	return nil
}

// InvariantSolution lists a trivial set of EqualsInvariant mappings so that you
// can populate your AST with SetType calls in a simple loop.
type InvariantSolution struct {
	Solutions []*EqualsInvariant // list of trivial solutions for each node
}

// ExprList returns the list of valid expressions. This struct is not part of
// the invariant interface, but it implements this anyways.
func (obj *InvariantSolution) ExprList() []interfaces.Expr {
	exprs := []interfaces.Expr{}
	for _, x := range obj.Solutions {
		exprs = append(exprs, x.ExprList()...)
	}
	return exprs
}
