// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
	"strings"

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
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

// EqualsInvariant is an invariant that symbolizes that the expression has a
// known type.
// TODO: is there a better name than EqualsInvariant
type EqualsInvariant struct {
	Expr Expr
	Type *types.Type
}

// String returns a representation of this invariant.
func (obj *EqualsInvariant) String() string {
	return fmt.Sprintf("%p == %s", obj.Expr, obj.Type)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualsInvariant) ExprList() []Expr {
	return []Expr{obj.Expr}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualsInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualsInvariant) Possible(partials []Invariant) error {
	// TODO: we could pass in a solver here
	//set := []Invariant{}
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
	solved := map[Expr]*types.Type{
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
	Expr1 Expr
	Expr2 Expr
}

// String returns a representation of this invariant.
func (obj *EqualityInvariant) String() string {
	return fmt.Sprintf("%p == %p", obj.Expr1, obj.Expr2)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityInvariant) ExprList() []Expr {
	return []Expr{obj.Expr1, obj.Expr2}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityInvariant) Possible(partials []Invariant) error {
	// The idea here is that we look for the expression pointers in the list
	// of partial invariants. It's only impossible if we (1) find both of
	// them, and (2) that they relate to each other. The second part is
	// harder.
	var one, two bool
	exprs := []Invariant{}
	for _, x := range partials {
		for _, y := range x.ExprList() { // []Expr
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
	Exprs []Expr
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
func (obj *EqualityInvariantList) ExprList() []Expr {
	return obj.Exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityInvariantList) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityInvariantList) Possible(partials []Invariant) error {
	// The idea here is that we look for the expression pointers in the list
	// of partial invariants. It's only impossible if we (1) find two or
	// more, and (2) that any of them relate to each other. The second part
	// is harder.
	inList := func(needle Expr, haystack []Expr) bool {
		for _, x := range haystack {
			if x == needle {
				return true
			}
		}
		return false
	}

	exprs := []Invariant{}
	for _, x := range partials {
		for _, y := range x.ExprList() { // []Expr
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
	Expr1    Expr
	Expr2Val Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapListInvariant) String() string {
	return fmt.Sprintf("%p == [%p]", obj.Expr1, obj.Expr2Val)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapListInvariant) ExprList() []Expr {
	return []Expr{obj.Expr1, obj.Expr2Val}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapListInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityWrapListInvariant) Possible(partials []Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}

// EqualityWrapMapInvariant expresses that a map in Expr1 must have keys that
// match the type of the expression in Expr2Key and values that match the type
// of the expression in Expr2Val.
type EqualityWrapMapInvariant struct {
	Expr1    Expr
	Expr2Key Expr
	Expr2Val Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapMapInvariant) String() string {
	return fmt.Sprintf("%p == {%p: %p}", obj.Expr1, obj.Expr2Key, obj.Expr2Val)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapMapInvariant) ExprList() []Expr {
	return []Expr{obj.Expr1, obj.Expr2Key, obj.Expr2Val}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapMapInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityWrapMapInvariant) Possible(partials []Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}

// EqualityWrapStructInvariant expresses that a struct in Expr1 must have fields
// that match the type of the expressions listed in Expr2Map.
type EqualityWrapStructInvariant struct {
	Expr1    Expr
	Expr2Map map[string]Expr
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
func (obj *EqualityWrapStructInvariant) ExprList() []Expr {
	exprs := []Expr{obj.Expr1}
	for _, x := range obj.Expr2Map {
		exprs = append(exprs, x)
	}
	return exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapStructInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityWrapStructInvariant) Possible(partials []Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}

// EqualityWrapFuncInvariant expresses that a func in Expr1 must have args that
// match the type of the expressions listed in Expr2Map and a return value that
// matches the type of the expression in Expr2Out.
// TODO: should this be named EqualityWrapCallInvariant or not?
type EqualityWrapFuncInvariant struct {
	Expr1    Expr
	Expr2Map map[string]Expr
	Expr2Ord []string
	Expr2Out Expr
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
func (obj *EqualityWrapFuncInvariant) ExprList() []Expr {
	exprs := []Expr{obj.Expr1}
	for _, x := range obj.Expr2Map {
		exprs = append(exprs, x)
	}
	exprs = append(exprs, obj.Expr2Out)
	return exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapFuncInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityWrapFuncInvariant) Possible(partials []Invariant) error {
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
	Expr1     Expr
	Expr2Func Expr
}

// String returns a representation of this invariant.
func (obj *EqualityWrapCallInvariant) String() string {
	return fmt.Sprintf("%p == call(%p)", obj.Expr1, obj.Expr2Func)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *EqualityWrapCallInvariant) ExprList() []Expr {
	return []Expr{obj.Expr1, obj.Expr2Func}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *EqualityWrapCallInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *EqualityWrapCallInvariant) Possible(partials []Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}

// GeneratorInvariant is an experimental type of new invariant. The idea is that
// this is a special invariant that the solver knows how to use; the solver runs
// all the easy bits first, and then passes the current solution state into the
// function, and in response, it runs some user-defined code and builds some new
// invariants that are added to the solver! This is not without caveats... This
// should only be used sparingly, and with care. It can suffer from the
// confluence problem, if the generator code that was provided is incorrect.
// What this means is that it could generate different results (and a different
// final solution) depending on the order in which it is called. Since this is
// undesirable, you must only use it for straight-forward situations. As an
// extreme example, if it generated different invariants depending on the time
// of day, this would be very problematic, and evil. Alternatively, it could be
// a pure function, but that returns wildly different results depending on what
// invariants were passed in. Use it wisely. It was added to make the printf
// function (which can have an infinite number of signatures) possible to
// express in terms of "normal" invariants. Lastly, if you wanted to use this to
// add-in partial progress, you could have it generate a list of invariants and
// include a new generator invariant in this list. Be sure to only do this if
// you are making progress on each invocation, and make sure to avoid infinite
// looping which isn't something we can currently detect or prevent. One special
// bit about generators and returning a partial: you must always return the
// minimum set of expressions that need to be solved in the first Unify() call
// that also returns the very first generator. This is because you must not rely
// on the generator to tell the solver about new expressions that it *also*
// wants solved. This is because after the initial (pre-generator-running)
// collection of the invariants, we need to be able to build a list of all the
// expressions that need to be solved for us to consider the problem "done". If
// a new expression only appeared after we ran a generator, then this would
// require our solver be far more complicated than it needs to be and currently
// is. Besides, there's no reason (that I know of at the moment) that needs this
// sort of invariant that only appears after the solver is running.
//
// NOTE: We might *consider* an optimization where we return a different kind of
// error that represents a response of "impossible". This would mean that there
// is no way to reconcile the current world-view with what is know about things.
// However, it would be easier and better to just return your invariants and let
// the normal solver run its course, although future research might show that it
// could maybe help in some cases.
// XXX: solver question: Can our solver detect `expr1 == str` AND `expr1 == int`
// and fail the whole thing when we know of a case like this that is impossible?
type GeneratorInvariant struct {
	// Func is a generator function that takes the state of the world, and
	// returns new invariants that should be added to this world view. The
	// state of the world includes both the currently unsolved invariants,
	// as well as the known solution map that has been solved so far. If
	// this returns nil, we add the invariants it returned and we remove it
	// from the list. If we error, it's because we don't have any new
	// information to provide at this time...
	Func func(invariants []Invariant, solved map[Expr]*types.Type) ([]Invariant, error)
}

// String returns a representation of this invariant.
func (obj *GeneratorInvariant) String() string {
	return fmt.Sprintf("gen(%p)", obj.Func) // TODO: improve this
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *GeneratorInvariant) ExprList() []Expr {
	return []Expr{}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *GeneratorInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
	// XXX: not implemented (don't panic though)
	//return false, err // inconsistent!
	//return false, nil // not matched yet
	//return true, nil  // matched!
	return false, nil // not matched yet

	// If we error, it's because we don't have any new information to
	// provide at this time... If it's nil, it's because the invariants
	// could have worked with this solution.
	//invariants, err := obj.Func(?, solved)
	//if err != nil {
	//}
}

// Possible is currently not implemented!
func (obj *GeneratorInvariant) Possible(partials []Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}

// ConjunctionInvariant represents a list of invariants which must all be true
// together. In other words, it's a grouping construct for a set of invariants.
type ConjunctionInvariant struct {
	Invariants []Invariant
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
func (obj *ConjunctionInvariant) ExprList() []Expr {
	exprs := []Expr{}
	for _, x := range obj.Invariants {
		exprs = append(exprs, x.ExprList()...)
	}
	return exprs
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *ConjunctionInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *ConjunctionInvariant) Possible(partials []Invariant) error {
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
	Invariants []Invariant
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
func (obj *ExclusiveInvariant) ExprList() []Expr {
	// XXX: We should do this if we assume that exclusives don't have some
	// sort of transient expr to satisfy that doesn't disappear depending on
	// which choice in the exclusive is chosen...
	//exprs := []Expr{}
	//for _, x := range obj.Invariants {
	//	exprs = append(exprs, x.ExprList()...)
	//}
	//return exprs
	// XXX: But if we ever specify an expr in this exclusive that isn't
	// referenced anywhere else, then we'd need to use the above so that our
	// type unification algorithm knows not to stop too early.
	return []Expr{} // XXX: Do we want to the set instead?
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors. Because this partial invariant requires only
// one to be true, it will mask children errors, since it's normal for only one
// to be consistent.
func (obj *ExclusiveInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *ExclusiveInvariant) Possible(partials []Invariant) error {
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

// Simplify attempts to reduce the exclusive invariant to eliminate any
// possibilities based on the list of known partials at this time. Hopefully,
// this will weed out some of the function polymorphism possibilities so that we
// can solve the problem without recursive, combinatorial permutation, which is
// very, very slow.
func (obj *ExclusiveInvariant) Simplify(partials []Invariant) ([]Invariant, error) {
	if len(obj.Invariants) == 0 { // unexpected case
		return []Invariant{}, nil // we don't need anything!
	}

	possible := []Invariant{}
	var reasons error
	for _, invar := range obj.Invariants { // []Invariant
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
	return []Invariant{invar}, nil
}

// AnyInvariant is an invariant that symbolizes that the expression can be any
// type. It is sometimes used to ensure that an expr actually gets a solution
// type so that it is not left unreferenced, and as a result, unsolved.
// TODO: is there a better name than AnyInvariant
type AnyInvariant struct {
	Expr Expr
}

// String returns a representation of this invariant.
func (obj *AnyInvariant) String() string {
	return fmt.Sprintf("%p == *", obj.Expr)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *AnyInvariant) ExprList() []Expr {
	return []Expr{obj.Expr}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *AnyInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
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
func (obj *AnyInvariant) Possible([]Invariant) error {
	// keep it simple, even though we don't technically check the inputs...
	return nil
}

// ValueInvariant is an invariant that stores the value associated with an expr
// if it happens to be known statically at unification/compile time. This must
// only be used for static/pure values. For example, in `$x = 42`, we know that
// $x is 42. It's useful here because for `printf("hello %d times", 42)` we can
// get both the format string, and the other args as these new invariants, and
// we'd store those separately into this invariant, where they can eventually be
// passed into the generator invariant, where it can parse the format string and
// we'd be able to produce a precise type for the printf function, since it's
// nearly impossible to do otherwise since the number of possibilities is
// infinite! One special note: these values are typically not consumed, by the
// solver, because they need to be around for the generator invariant to use, so
// make sure your solver implementation can still terminate with unused
// invariants!
type ValueInvariant struct {
	Expr  Expr
	Value types.Value // pointer
}

// String returns a representation of this invariant.
func (obj *ValueInvariant) String() string {
	return fmt.Sprintf("%p == %s", obj.Expr, obj.Value)
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *ValueInvariant) ExprList() []Expr {
	return []Expr{obj.Expr}
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *ValueInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
	typ, exists := solved[obj.Expr]
	if !exists {
		return false, nil
	}
	if err := typ.Cmp(obj.Value.Type()); err != nil {
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
func (obj *ValueInvariant) Possible(partials []Invariant) error {
	// XXX: Double check this is logical. It was modified from EqualsInvariant.
	solved := map[Expr]*types.Type{
		obj.Expr: obj.Value.Type(),
	}
	for _, invar := range partials { // check each one
		_, err := invar.Matches(solved)
		if err != nil { // inconsistent, so it's not possible
			return errwrap.Wrapf(err, "not possible")
		}
	}

	return nil
}

// CallFuncArgsValueInvariant expresses that a func call is associated with a
// particular func, and that it is called with a specific list of args. Expr
// must match the function call expression, Func must match the actual function
// expression, and Args matches the args used in the call to run the func.
// TODO: should this be named FuncCallArgsValueInvariant or something different
// or not?
type CallFuncArgsValueInvariant struct {
	// Expr represents the pointer to the ExprCall.
	Expr Expr

	// Func represents the pointer to the ExprFunc that ExprCall is using.
	Func Expr

	// Args represents the list of args that the ExprCall is using to call
	// the ExprFunc. A solver might speculatively call Value() on each of
	// these in the hopes of doing something useful if a value happens to be
	// known statically at compile time. One such solver that might do this
	// is the GeneratorInvariant inside of a difficult function like printf.
	Args []Expr
}

// String returns a representation of this invariant.
func (obj *CallFuncArgsValueInvariant) String() string {
	return fmt.Sprintf("%p == callfuncargs(%p) %p", obj.Expr, obj.Func, obj.Args) // TODO: improve this
}

// ExprList returns the list of valid expressions in this invariant.
func (obj *CallFuncArgsValueInvariant) ExprList() []Expr {
	return []Expr{obj.Expr} // XXX: add obj.Func or each obj.Args ?
}

// Matches returns whether an invariant matches the existing solution. If it is
// inconsistent, then it errors.
func (obj *CallFuncArgsValueInvariant) Matches(solved map[Expr]*types.Type) (bool, error) {
	// XXX: not implemented (don't panic though)
	//return false, err // inconsistent!
	//return false, nil // not matched yet
	//return true, nil  // matched!
	return false, nil // not matched yet
}

// Possible returns an error if it is certain that it is NOT possible to get a
// solution with this invariant and the set of partials. In certain cases, it
// might not be able to determine that it's not possible, while simultaneously
// not being able to guarantee a possible solution either. In this situation, it
// should return nil, since this is used as a filtering mechanism, and the nil
// result of possible is preferred over eliminating a tricky, but possible one.
// This particular implementation is currently not implemented!
func (obj *CallFuncArgsValueInvariant) Possible(partials []Invariant) error {
	// XXX: not implemented
	return nil // safer to return nil than error
}
