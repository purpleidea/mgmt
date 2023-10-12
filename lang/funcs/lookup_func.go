// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

package funcs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// LookupFuncName is the name this function is registered as.
	// This starts with an underscore so that it cannot be used from the
	// lexer.
	LookupFuncName = "_lookup"

	// arg names...
	lookupArgNameListOrMap  = "listormap"
	lookupArgNameIndexOrKey = "indexorkey"
)

func init() {
	Register(LookupFuncName, func() interfaces.Func { return &LookupFunc{} }) // must register the func and name
}

var _ interfaces.PolyFunc = &LookupFunc{} // ensure it meets this expectation

// LookupFunc is a list index or map key lookup function. It does both because
// the current syntax in the parser is identical, so it's convenient to mix the
// two together. This calls out to some of the code in the ListLookupFunc and
// MapLookupFunc implementations. If the index or key for this input doesn't
// exist, then it will return the zero value for that type.
type LookupFunc struct {
	Type *types.Type // Kind == List OR Map, that is used as the list/map we lookup in

	//init *interfaces.Init
	fn interfaces.PolyFunc // handle to ListLookupFunc or MapLookupFunc
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *LookupFunc) String() string {
	return LookupFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *LookupFunc) ArgGen(index int) (string, error) {
	seq := []string{lookupArgNameListOrMap, lookupArgNameIndexOrKey}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *LookupFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(list T1, index int) T3
	// (list: []T3 => T3 aka T1 => T3)
	// OR
	// func(map T1, key T2) T3
	// (map: T2 => T3)

	listOrMapName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	indexOrKeyName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	dummyListOrMap := &interfaces.ExprAny{}  // corresponds to the list or map type
	dummyIndexOrKey := &interfaces.ExprAny{} // corresponds to the index or key type
	dummyOut := &interfaces.ExprAny{}        // corresponds to the out string

	ors := []interfaces.Invariant{} // solve only one from this list

	var listInvariants []interfaces.Invariant

	// relationship between T1 and T3
	invar = &interfaces.EqualityWrapListInvariant{
		Expr1:    dummyListOrMap,
		Expr2Val: dummyOut,
	}
	listInvariants = append(listInvariants, invar)

	// the index has to be an int
	invar = &interfaces.EqualsInvariant{
		Expr: dummyIndexOrKey,
		Type: types.TypeInt,
	}
	listInvariants = append(listInvariants, invar)

	// all of these need to be true together
	and := &interfaces.ConjunctionInvariant{
		Invariants: listInvariants,
	}
	ors = append(ors, and) // one solution added!

	// OR

	// relationship between T1, T2 and T3
	mapInvariant := &interfaces.EqualityWrapMapInvariant{
		Expr1:    dummyListOrMap,
		Expr2Key: dummyIndexOrKey,
		Expr2Val: dummyOut,
	}
	ors = append(ors, mapInvariant) // one solution added!

	invar = &interfaces.ExclusiveInvariant{
		Invariants: ors, // one and only one of these should be true
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{listOrMapName, indexOrKeyName}
	mapped[listOrMapName] = dummyListOrMap
	mapped[indexOrKeyName] = dummyIndexOrKey

	invar = &interfaces.EqualityWrapFuncInvariant{
		Expr1:    expr, // maps directly to us!
		Expr2Map: mapped,
		Expr2Ord: ordered,
		Expr2Out: dummyOut,
	}
	invariants = append(invariants, invar)

	// generator function
	fn := func(fnInvariants []interfaces.Invariant, solved map[interfaces.Expr]*types.Type) ([]interfaces.Invariant, error) {
		for _, invariant := range fnInvariants {
			// search for this special type of invariant
			cfavInvar, ok := invariant.(*interfaces.CallFuncArgsValueInvariant)
			if !ok {
				continue
			}
			// did we find the mapping from us to ExprCall ?
			if cfavInvar.Func != expr {
				continue
			}
			// cfavInvar.Expr is the ExprCall! (the return pointer)
			// cfavInvar.Args are the args that ExprCall uses!
			if l := len(cfavInvar.Args); l != 2 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// add the relationship to the returned value
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Expr,
				Expr2: dummyOut,
			}
			invariants = append(invariants, invar)

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyListOrMap,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyIndexOrKey,
			}
			invariants = append(invariants, invar)

			// If we figure out all of these three types, we'll
			// know the full type...
			var t1 *types.Type // list or map type
			var t2 *types.Type // list or map index/key type
			var t3 *types.Type // list or map val type

			// validateArg0 checks: list or map T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				// we happen to have a list or a map!
				if k := typ.Kind; k != types.KindList && k != types.KindMap {
					return fmt.Errorf("unable to build function with 0th arg of kind: %s", k)
				}
				//isList := typ.Kind == types.KindList
				isMap := typ.Kind == types.KindMap

				if isMap && typ.Key == nil {
					// programming error
					return fmt.Errorf("map is missing type")
				}
				if typ.Val == nil { // used for list or map
					// programming error
					return fmt.Errorf("map/list is missing type")
				}

				if err := typ.Cmp(t1); t1 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}
				if isMap {
					if err := typ.Key.Cmp(t2); t2 != nil && err != nil {
						return errwrap.Wrapf(err, "input key type was inconsistent")
					}
				}
				if err := typ.Val.Cmp(t3); t3 != nil && err != nil {
					return errwrap.Wrapf(err, "input val type was inconsistent")
				}

				// learn!
				t1 = typ
				if isMap {
					t2 = typ.Key
				} else if t1 != nil && t3 != nil {
					t2 = types.TypeInt
				}
				t3 = typ.Val
				return nil
			}

			// validateArg1 checks: list index
			validateListArg1 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}
				if typ.Kind != types.KindInt {
					return errwrap.Wrapf(err, "input index type was inconsistent")
				}

				// learn!
				t2 = typ
				return nil
			}

			// validateArg1 checks: map key T2
			validateMapArg1 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				if err := typ.Cmp(t2); t2 != nil && err != nil {
					return errwrap.Wrapf(err, "input key type was inconsistent")
				}
				if t1 != nil {
					if err := typ.Cmp(t1.Key); err != nil {
						return errwrap.Wrapf(err, "input key type was inconsistent")
					}
				}
				if t3 != nil {
					t := &types.Type{ // build t1
						Kind: types.KindMap,
						Key:  typ, // t2
						Val:  t3,
					}

					if err := t.Cmp(t1); t1 != nil && err != nil {
						return errwrap.Wrapf(err, "input type was inconsistent")
					}
					t1 = t // learn!
				}

				// learn!
				t2 = typ
				return nil
			}

			// validateArg1 checks: list index
			validateArg1 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}
				isList := typ.Kind == types.KindList
				isMap := typ.Kind == types.KindMap

				if isList {
					return validateListArg1(typ)
				}
				if isMap {
					return validateMapArg1(typ)
				}

				return nil
			}

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 and t3 on success (and sometimes t2) if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 and t3 on success (and sometimes t2) if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first arg type is inconsistent")
				}
			}

			if typ, err := cfavInvar.Args[1].Type(); err == nil { // is it known?
				// this sets t2 (and sometimes t1) on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[1]]; exists { // alternate way to lookup type
				// this sets t2 (and sometimes t1) on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second arg type is inconsistent")
				}
			}

			// XXX: if the types aren't know statically?

			if t1 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyListOrMap,
					Type: t1,
				}
				invariants = append(invariants, invar)
			}
			if t2 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyIndexOrKey,
					Type: t2,
				}
				invariants = append(invariants, invar)
			}
			if t3 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyOut,
					Type: t3,
				}
				invariants = append(invariants, invar)
			}

			// XXX: if t{1..2} are missing, we could also return a
			// new generator for later if we learn new information,
			// but we'd have to be careful to not do it infinitely.

			// TODO: do we return this relationship with ExprCall?
			invar = &interfaces.EqualityWrapCallInvariant{
				// TODO: should Expr1 and Expr2 be reversed???
				Expr1: cfavInvar.Expr,
				//Expr2Func: cfavInvar.Func, // same as below
				Expr2Func: expr,
			}
			invariants = append(invariants, invar)

			// TODO: are there any other invariants we should build?
			return invariants, nil // generator return
		}
		// We couldn't tell the solver anything it didn't already know!
		return nil, fmt.Errorf("couldn't generate new invariants")
	}
	invar = &interfaces.GeneratorInvariant{
		Func: fn,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *LookupFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) < 1 {
		return nil, fmt.Errorf("the lookup function needs at least one arg") // actually 2 or 3
	}
	tListOrMap, exists := typ.Map[typ.Ord[0]]
	if !exists || tListOrMap == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	if tListOrMap == nil {
		return nil, fmt.Errorf("first arg must have a type")
	}

	if tListOrMap.Kind == types.KindList {
		obj.fn = &ListLookupFunc{} // set it
		return obj.fn.Build(typ)
	}
	if tListOrMap.Kind == types.KindMap {
		obj.fn = &MapLookupFunc{} // set it
		return obj.fn.Build(typ)
	}

	return nil, fmt.Errorf("we must lookup from either a list or a map")
}

// Validate tells us if the input struct takes a valid form.
func (obj *LookupFunc) Validate() error {
	if obj.fn == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return obj.fn.Validate()
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *LookupFunc) Info() *interfaces.Info {
	if obj.fn == nil {
		return &interfaces.Info{
			Pure: true,
			Memo: false,
			Sig:  nil, // func kind
			Err:  obj.Validate(),
		}
	}
	return obj.fn.Info()
}

// Init runs some startup code for this function.
func (obj *LookupFunc) Init(init *interfaces.Init) error {
	if obj.fn == nil {
		return fmt.Errorf("function not built correctly")
	}
	//obj.init = init
	return obj.fn.Init(init)
}

// Stream returns the changing values that this func has over time.
func (obj *LookupFunc) Stream(ctx context.Context) error {
	if obj.fn == nil {
		return fmt.Errorf("function not built correctly")
	}
	return obj.fn.Stream(ctx)
}
