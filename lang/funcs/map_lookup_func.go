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
	// MapLookupFuncName is the name this function is registered as.
	MapLookupFuncName = "map_lookup"

	// arg names...
	mapLookupArgNameMap = "map"
	mapLookupArgNameKey = "key"
)

func init() {
	Register(MapLookupFuncName, func() interfaces.Func { return &MapLookupFunc{} }) // must register the func and name
}

var _ interfaces.PolyFunc = &MapLookupFunc{} // ensure it meets this expectation

// MapLookupFunc is a key map lookup function. If you provide a missing key,
// then it will return the zero value for that type.
type MapLookupFunc struct {
	Type *types.Type // Kind == Map, that is used as the map we lookup

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *MapLookupFunc) String() string {
	return MapLookupFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *MapLookupFunc) ArgGen(index int) (string, error) {
	seq := []string{mapLookupArgNameMap, mapLookupArgNameKey}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *MapLookupFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(map T1, key T2) T3
	// (map: T2 => T3)

	mapName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	keyName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	dummyMap := &interfaces.ExprAny{} // corresponds to the map type
	dummyKey := &interfaces.ExprAny{} // corresponds to the key type
	dummyOut := &interfaces.ExprAny{} // corresponds to the out string

	// relationship between T1, T2 and T3
	invar = &interfaces.EqualityWrapMapInvariant{
		Expr1:    dummyMap,
		Expr2Key: dummyKey,
		Expr2Val: dummyOut,
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{mapName, keyName}
	mapped[mapName] = dummyMap
	mapped[keyName] = dummyKey

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
				Expr2: dummyMap,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyKey,
			}
			invariants = append(invariants, invar)

			// If we figure out all of these three types, we'll
			// know the full type...
			var t1 *types.Type // map type
			var t2 *types.Type // map key type
			var t3 *types.Type // map val type

			// validateArg0 checks: map T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				// we happen to have a map!
				if k := typ.Kind; k != types.KindMap {
					return fmt.Errorf("unable to build function with 0th arg of kind: %s", k)
				}

				if typ.Key == nil || typ.Val == nil {
					// programming error
					return fmt.Errorf("map is missing type")
				}

				if err := typ.Cmp(t1); t1 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}
				if err := typ.Key.Cmp(t2); t2 != nil && err != nil {
					return errwrap.Wrapf(err, "input key type was inconsistent")
				}
				if err := typ.Val.Cmp(t3); t3 != nil && err != nil {
					return errwrap.Wrapf(err, "input val type was inconsistent")
				}

				// learn!
				t1 = typ
				t2 = typ.Key
				t3 = typ.Val
				return nil
			}

			// validateArg1 checks: map key T2
			validateArg1 := func(typ *types.Type) error {
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

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 and t2 and t3 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first map arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 and t2 and t3 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first map arg type is inconsistent")
				}
			}

			if typ, err := cfavInvar.Args[1].Type(); err == nil { // is it known?
				// this sets t2 (and sometimes t1) on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second key arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[1]]; exists { // alternate way to lookup type
				// this sets t2 (and sometimes t1) on success if it learned
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second key arg type is inconsistent")
				}
			}

			// XXX: if the types aren't know statically?

			if t1 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyMap,
					Type: t1,
				}
				invariants = append(invariants, invar)
			}
			if t2 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyKey,
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

			// XXX: if t{1..3} are missing, we could also return a
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
func (obj *MapLookupFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the maplookup function needs exactly three args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	tMap, exists := typ.Map[typ.Ord[0]]
	if !exists || tMap == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}

	tKey, exists := typ.Map[typ.Ord[1]]
	if !exists || tKey == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}

	if err := tMap.Key.Cmp(tKey); err != nil {
		return nil, errwrap.Wrapf(err, "key must match map key type")
	}

	if err := tMap.Val.Cmp(typ.Out); err != nil {
		return nil, errwrap.Wrapf(err, "return type must match map val type")
	}

	obj.Type = tMap // map type
	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *MapLookupFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindMap {
		return fmt.Errorf("type must be a kind of map")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *MapLookupFunc) Info() *interfaces.Info {
	var sig *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		// TODO: can obj.Type.Key or obj.Type.Val be nil (a partial) ?
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  sig, // func kind
		Err:  obj.Validate(),
	}
}

// helper
func (obj *MapLookupFunc) sig() *types.Type {
	k := obj.Type.Key.String()
	v := obj.Type.Val.String()
	return types.NewType(fmt.Sprintf("func(%s %s, %s %s) %s", mapLookupArgNameMap, obj.Type.String(), mapLookupArgNameKey, k, v))
}

// Init runs some startup code for this function.
func (obj *MapLookupFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *MapLookupFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			m := (input.Struct()[mapLookupArgNameMap]).(*types.MapValue)
			key := input.Struct()[mapLookupArgNameKey]
			zero := m.Type().New() // the zero value

			var result types.Value
			val, exists := m.Lookup(key)
			if exists {
				result = val
			} else {
				result = zero
			}

			// if previous input was `2 + 4`, but now it
			// changed to `1 + 5`, the result is still the
			// same, so we can skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
		case <-ctx.Done():
			return nil
		}
	}
}
