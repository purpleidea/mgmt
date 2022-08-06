// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// MapLookupFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	// XXX: change to _maplookup and add syntax in the lexer/parser
	MapLookupFuncName = "maplookup"

	argNameMap = "map"
	argNameKey = "key"
	argNameDef = "default"
)

func init() {
	Register(MapLookupFuncName, func() interfaces.Func { return &MapLookupPolyFunc{} }) // must register the func and name
}

// MapLookupPolyFunc is a key map lookup function.
type MapLookupPolyFunc struct {
	Type *types.Type // Kind == Map, that is used as the map we lookup

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *MapLookupPolyFunc) ArgGen(index int) (string, error) {
	seq := []string{argNameMap, argNameKey, argNameDef}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *MapLookupPolyFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(map T1, key T2, default T3) T3
	// (map: T2 => T3)

	mapName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	keyName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	defaultName, err := obj.ArgGen(2)
	if err != nil {
		return nil, err
	}

	dummyMap := &interfaces.ExprAny{}     // corresponds to the map type
	dummyKey := &interfaces.ExprAny{}     // corresponds to the key type
	dummyDefault := &interfaces.ExprAny{} // corresponds to the default type
	dummyOut := &interfaces.ExprAny{}     // corresponds to the out string

	// default type and out are the same
	invar = &interfaces.EqualityInvariant{
		Expr1: dummyDefault,
		Expr2: dummyOut,
	}
	invariants = append(invariants, invar)

	// relationship between T1, T2 and T3
	invar = &interfaces.EqualityWrapMapInvariant{
		Expr1:    dummyMap,
		Expr2Key: dummyKey,
		Expr2Val: dummyDefault,
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{mapName, keyName, defaultName}
	mapped[mapName] = dummyMap
	mapped[keyName] = dummyKey
	mapped[defaultName] = dummyDefault

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
			if l := len(cfavInvar.Args); l != 3 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}

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

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[2],
				Expr2: dummyDefault,
			}
			invariants = append(invariants, invar)

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

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

			// validateArg2 checks: map val T3
			validateArg2 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				if err := typ.Cmp(t3); t3 != nil && err != nil {
					return errwrap.Wrapf(err, "input val type was inconsistent")
				}
				if t1 != nil {
					if err := typ.Cmp(t1.Val); err != nil {
						return errwrap.Wrapf(err, "input val type was inconsistent")
					}
				}
				if t2 != nil {
					t := &types.Type{ // build t1
						Kind: types.KindMap,
						Key:  t2,
						Val:  typ, // t3
					}

					if err := t.Cmp(t1); t1 != nil && err != nil {
						return errwrap.Wrapf(err, "input type was inconsistent")
					}
					t1 = t // learn!
				}

				// learn!
				t3 = typ
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

			if typ, err := cfavInvar.Args[2].Type(); err == nil { // is it known?
				// this sets t3 (and sometimes t1) on success if it learned
				if err := validateArg2(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third default arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[2]]; exists { // alternate way to lookup type
				// this sets t3 (and sometimes t1) on success if it learned
				if err := validateArg2(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third default arg type is inconsistent")
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
					Expr: dummyDefault,
					Type: t3,
				}
				invariants = append(invariants, invar)
			}

			// XXX: if t{1..3} are missing, we could also return a
			// new generator for later if we learn new information,
			// but we'd have to be careful to not do the infinitely

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

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *MapLookupPolyFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: return `variant` as arg for now -- maybe there's a better way?
	variant := []*types.Type{types.NewType("func(map variant, key variant, default variant) variant")}

	if partialType == nil {
		return variant, nil
	}

	// what's the map type of the first argument?
	typ := &types.Type{
		Kind: types.KindMap,
		//Key: ???,
		//Val: ???,
	}

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) != 3 {
			return nil, fmt.Errorf("must have exactly three args in maplookup func")
		}
		if tMap, exists := partialType.Map[ord[0]]; exists && tMap != nil {
			if tMap.Kind != types.KindMap {
				return nil, fmt.Errorf("first arg for maplookup must be a map")
			}
			typ.Key = tMap.Key
			typ.Val = tMap.Val
		}
		if tKey, exists := partialType.Map[ord[1]]; exists && tKey != nil {
			if typ.Key != nil && typ.Key.Cmp(tKey) != nil {
				return nil, fmt.Errorf("second arg for maplookup must match map's key type")
			}
			typ.Key = tKey
		}
		if tDef, exists := partialType.Map[ord[2]]; exists && tDef != nil {
			if typ.Val != nil && typ.Val.Cmp(tDef) != nil {
				return nil, fmt.Errorf("third arg for maplookup must match map's val type")
			}
			typ.Val = tDef

			// add this for better error messages
			if tOut := partialType.Out; tOut != nil {
				if tDef.Cmp(tOut) != nil {
					return nil, fmt.Errorf("third arg for maplookup must match return type")
				}
			}
		}
		if tOut := partialType.Out; tOut != nil {
			if typ.Val != nil && typ.Val.Cmp(tOut) != nil {
				return nil, fmt.Errorf("return type for maplookup must match map's val type")
			}
			typ.Val = tOut
		}
	}

	// TODO: are we okay adding just the map val type and not the map key type?
	//if tOut := partialType.Out; tOut != nil {
	//	if typ.Val != nil && typ.Val.Cmp(tOut) != nil {
	//		return nil, fmt.Errorf("return type for maplookup must match map's val type")
	//	}
	//	typ.Val = tOut
	//}

	typFunc := &types.Type{
		Kind: types.KindFunc, // function type
		Map:  make(map[string]*types.Type),
		Ord:  []string{argNameMap, argNameKey, argNameDef},
		Out:  nil,
	}
	typFunc.Map[argNameMap] = typ
	typFunc.Map[argNameKey] = typ.Key
	typFunc.Map[argNameDef] = typ.Val
	typFunc.Out = typ.Val

	// TODO: don't include partial internal func map's for now, allow in future?
	if typ.Key == nil || typ.Val == nil {
		typFunc.Map = make(map[string]*types.Type) // erase partial
		typFunc.Map[argNameMap] = types.TypeVariant
		typFunc.Map[argNameKey] = types.TypeVariant
		typFunc.Map[argNameDef] = types.TypeVariant
	}
	if typ.Val == nil {
		typFunc.Out = types.TypeVariant
	}

	// just returning nothing for now, in case we can't detect a partial map
	if typ.Key == nil || typ.Val == nil {
		return []*types.Type{typFunc}, nil
	}

	// TODO: type check that the partialValues are compatible

	return []*types.Type{typFunc}, nil // solved!
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *MapLookupPolyFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 3 {
		return fmt.Errorf("the maplookup function needs exactly three args")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return fmt.Errorf("invalid input type")
	}

	tMap, exists := typ.Map[typ.Ord[0]]
	if !exists || tMap == nil {
		return fmt.Errorf("first arg must be specified")
	}

	tKey, exists := typ.Map[typ.Ord[1]]
	if !exists || tKey == nil {
		return fmt.Errorf("second arg must be specified")
	}

	tDef, exists := typ.Map[typ.Ord[2]]
	if !exists || tDef == nil {
		return fmt.Errorf("third arg must be specified")
	}

	if err := tMap.Key.Cmp(tKey); err != nil {
		return errwrap.Wrapf(err, "key must match map key type")
	}

	if err := tMap.Val.Cmp(tDef); err != nil {
		return errwrap.Wrapf(err, "default must match map val type")
	}

	if err := tMap.Val.Cmp(typ.Out); err != nil {
		return errwrap.Wrapf(err, "return type must match map val type")
	}

	obj.Type = tMap // map type
	return nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *MapLookupPolyFunc) Validate() error {
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
func (obj *MapLookupPolyFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		// TODO: can obj.Type.Key or obj.Type.Val be nil (a partial) ?
		k := obj.Type.Key.String()
		v := obj.Type.Val.String()
		typ = types.NewType(fmt.Sprintf("func(map %s, key %s, default %s) %s", obj.Type.String(), k, v, v))
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  typ, // func kind
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *MapLookupPolyFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *MapLookupPolyFunc) Stream() error {
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

			m := (input.Struct()[argNameMap]).(*types.MapValue)
			key := input.Struct()[argNameKey]
			def := input.Struct()[argNameDef]

			var result types.Value
			val, exists := m.Lookup(key)
			if exists {
				result = val
			} else {
				result = def
			}

			// if previous input was `2 + 4`, but now it
			// changed to `1 + 5`, the result is still the
			// same, so we can skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *MapLookupPolyFunc) Close() error {
	close(obj.closeChan)
	return nil
}
