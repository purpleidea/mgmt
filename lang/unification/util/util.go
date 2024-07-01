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

// Package util contains some utility functions and algorithms which are useful
// for type unification.
package util

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/types"
)

// Unify takes two types and tries to make them equivalent. If they are both
// basic types without any unification variables, then we compare them directly,
// and this is equivalent to running the *types.Type Cmp method. This function
// works by drawing conclusions from the assertion that the two sides are equal:
// that a variable on the left must be equal to the sub-trees at the same
// position on the right, and similarly for variables on the right. This may
// modify the input types, copy them before use if this is an issue. If you only
// want to do a compare, you can safely use UnifyCmp.
func Unify(typ1, typ2 *types.Type) error {
	if typ1 == nil || typ2 == nil {
		return fmt.Errorf("nil type")
	}

	// Both types are real and don't contain any unification variables, so
	// we just compare them directly. -- Actually don't, these could contain
	// unification variables, so leave that to the recursive call below.
	//if typ1.Uni == nil && typ2.Uni == nil {
	//	return typ1.Cmp(typ2)
	//}

	// Here we have one type that is a ?1 type, and the other one *might* be
	// a full type or it might even contain a ?2 for example. It could be a
	// [?2] or [[?2]] for example.
	if typ1.Uni != nil && typ2.Uni == nil { // aka && typ2.Kind != nil
		root := typ1.Uni.Find()

		// We don't yet know anything about this unification variable.
		if root.Data == nil {
			if err := OccursCheck(root, typ2); err != nil {
				return err
			}

			root.Data = typ2 // learn!
			return nil
		}
		// otherwise, cmp root.Data with typ2

		return Unify(root.Data, typ2)
	}

	// This is the same case as above, except it's the opposite scenario.
	if typ1.Uni == nil && typ2.Uni != nil {
		root := typ2.Uni.Find()

		// We don't yet know anything about this unification variable.
		if root.Data == nil {
			if err := OccursCheck(root, typ1); err != nil {
				return err
			}

			root.Data = typ1 // learn!
			return nil
		}
		// otherwise, cmp root.Data with typ1

		return Unify(root.Data, typ1)
	}

	// Both of these are of the form ?1 and ?2 so we compare them directly.
	if typ1.Uni != nil && typ2.Uni != nil {
		root1 := typ1.Uni.Find()
		root2 := typ2.Uni.Find()

		if root1.Data == nil && root2.Data == nil {
			// We don't need a merge function to wrap the Union call
			// because in this scenario, both data fields are empty!
			root1.Union(root2) // merge!
			return nil
		}

		if root1.Data == nil && root2.Data != nil {
			return Unify(typ1, root2.Data)
		}

		if root1.Data != nil && root2.Data == nil {
			return Unify(root1.Data, typ2)
		}

		// root1.Data != nil && root2.Data != nil
		return Unify(root1.Data, root2.Data)
	}

	// At this point, we've handled all the special cases with the ?1, ?2
	// unification variables, so we now expect the kind's to match to unify.
	if k1, k2 := typ1.Kind, typ2.Kind; k1 != k2 {
		return fmt.Errorf("type error: %v != %v", k1, k2)
	}

	if typ1.Kind == types.KindBool && typ2.Kind == types.KindBool {
		return nil
	}
	if typ1.Kind == types.KindStr && typ2.Kind == types.KindStr {
		return nil
	}
	if typ1.Kind == types.KindInt && typ2.Kind == types.KindInt {
		return nil
	}
	if typ1.Kind == types.KindFloat && typ2.Kind == types.KindFloat {
		return nil
	}

	if typ1.Kind == types.KindList && typ2.Kind == types.KindList {
		if err := Unify(typ1.Val, typ2.Val); err != nil {
			return err
		}
		//Unpack(typ1)
		//Unpack(typ2)
		//if typ1.Val.Uni != nil { // example of what Unpack() does
		//	typ1.Val = typ1.Val.Uni.Data // TODO: Should we find?
		//}
		//if typ2.Val.Uni != nil {
		//	typ2.Val = typ2.Val.Uni.Data // TODO: Should we find?
		//}
		return nil
	}

	if typ1.Kind == types.KindMap && typ2.Kind == types.KindMap {
		if err := Unify(typ1.Key, typ2.Key); err != nil {
			return err
		}
		if err := Unify(typ1.Val, typ2.Val); err != nil {
			return err
		}
		//Unpack(typ1)
		//Unpack(typ2)
		return nil
	}

	if typ1.Kind == types.KindStruct && typ2.Kind == types.KindStruct {
		if typ1.Map == nil || typ2.Map == nil {
			panic("malformed struct type")
		}
		if len(typ1.Ord) != len(typ2.Ord) {
			return fmt.Errorf("struct field count differs")
		}
		for i, k := range typ1.Ord {
			if k != typ2.Ord[i] {
				return fmt.Errorf("struct fields differ")
			}
		}
		for _, k := range typ1.Ord { // loop map in deterministic order
			t1, ok := typ1.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			t2, ok := typ2.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			//if t1 == nil || t2 == nil { // checked at the top
			//	panic("malformed struct field")
			//}
			if err := Unify(t1, t2); err != nil {
				return err
			}
		}
		//Unpack(typ1)
		//Unpack(typ2)
		return nil
	}

	if typ1.Kind == types.KindFunc && typ2.Kind == types.KindFunc {
		if typ1.Map == nil || typ2.Map == nil {
			panic("malformed func type")
		}
		if len(typ1.Ord) != len(typ2.Ord) {
			return fmt.Errorf("func arg count differs")
		}

		// needed for strict cmp only...
		//for i, k := range typ1.Ord {
		//	if k != typ2.Ord[i] {
		//		return fmt.Errorf("func arg differs")
		//	}
		//}
		//for _, k := range typ1.Ord { // loop map in deterministic order
		//	t1, ok := typ1.Map[k]
		//	if !ok {
		//		panic("malformed func order")
		//	}
		//	t2, ok := typ2.Map[k]
		//	if !ok {
		//		panic("malformed func order")
		//	}
		//	//if t1 == nil || t2 == nil { // checked at the top
		//	//	panic("malformed func arg")
		//	//}
		//	if err := Unify(t1, t2); err != nil {
		//		return err
		//	}
		//}

		// if we're not comparing arg names, get the two lists of types
		for i := 0; i < len(typ1.Ord); i++ {
			t1, ok := typ1.Map[typ1.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			if t1 == nil {
				panic("malformed func arg")
			}

			t2, ok := typ2.Map[typ2.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			if t2 == nil {
				panic("malformed func arg")
			}

			if err := Unify(t1, t2); err != nil {
				return err
			}
		}

		if err := Unify(typ1.Out, typ2.Out); err != nil {
			return err
		}
		//Unpack(typ1)
		//Unpack(typ2)
		return nil
	}

	// Unsure if this case is ever used.
	if typ1.Kind == types.KindVariant && typ2.Kind == types.KindVariant {
		// TODO: should we Unify typ1.Var with typ2.Var ?
		if err := Unify(typ1.Var, typ2.Var); err != nil {
			return err
		}
		//Unpack(typ1)
		//Unpack(typ2)
		return nil
	}

	// programming error
	return fmt.Errorf("unhandled type case")
}

// OccursCheck determines if elem exists inside of this type. This is important
// so that we can avoid infinite self-referential types. If we find that it does
// occur, then we error. This: `?1 occurs-in [[?2]]` is what we're doing here!
// This function panics if you pass in a nil type, a malformed type, or an elem
// is nil or that has a .Data field that is populated. This is because if ?0 is
// equal to []?1, then to prevent infinite types, it does not suffice to check
// that ?0 does not occur in the list ([]?1), we must also check that []?1 does
// not occur. That check is harder (and slower) to implement. So we just don't
// implement it, and we ask the caller to only call OccursCheck in the easy case
// where ?0 is not yet equal to anything else.
func OccursCheck(elem *types.Elem, typ *types.Type) error {
	if elem == nil {
		panic("nil elem")
	}
	if typ == nil {
		panic("nil type")
	}

	if typ.Uni != nil {
		root1 := elem.Find()
		root2 := typ.Uni.Find()

		if root1 == root2 {
			return fmt.Errorf("directly in the same set")
		}

		// We don't look at root1.Data as it's not important because we
		// only call OccursCheck if root1.Data (which is the single
		// representative for the elem set by union find) is nil.
		// TODO: Move this check to the top of the function for safety?
		if root1.Data != nil {
			// programming error
			panic("unexpected non-nil data")
		}

		if root2.Data == nil {
			return nil // We don't occur again, we are done!
		}

		return OccursCheck(root1, root2.Data)
	}
	// Now we know that `typ.Uni == nil`. There could still be a ?1 inside
	// of a recursive type .Val, .Key, etc field. Check through all of them.

	if typ.Kind == types.KindBool {
		return nil
	}
	if typ.Kind == types.KindStr {
		return nil
	}
	if typ.Kind == types.KindInt {
		return nil
	}
	if typ.Kind == types.KindFloat {
		return nil
	}

	if typ.Kind == types.KindList {
		return OccursCheck(elem, typ.Val)
	}

	if typ.Kind == types.KindMap {
		if err := OccursCheck(elem, typ.Key); err != nil {
			return err
		}
		return OccursCheck(elem, typ.Val)
	}

	if typ.Kind == types.KindStruct {
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			//if t == nil { // checked at the top
			//	panic("malformed struct field")
			//}
			if err := OccursCheck(elem, t); err != nil {
				return err
			}
		}
		return nil
	}

	if typ.Kind == types.KindFunc {
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed func order")
			}
			//if t == nil { // checked at the top
			//	panic("malformed func field")
			//}
			if err := OccursCheck(elem, t); err != nil {
				return err
			}
		}
		return OccursCheck(elem, typ.Out)
	}

	// Unsure if this case is ever used.
	if typ.Kind == types.KindVariant {
		return OccursCheck(elem, typ.Var)
	}

	// programming error
	panic("malformed type")
}

// Extract takes a solved type out of a unification variable data slot, and
// returns it from the input container type. If there is no contained
// unification variable, or no type in the unification variable data field, then
// this simply returns the input type without modification. Fields in the type
// tree that do not need to be copied, are not. If in any doubt copy the result.
// We *never* copy the actual unification variable itself. We keep the pointer!
// This public function looks at the whole type recursively, and will first
// extract any child unification variables before finishing at the top. This was
// written based on the realization that something of this shape was needed
// after looking at post-unification unification variables and realizing that
// the data needed to be "bubbled upwards". It turns out the GHC Haskell has a
// similar function for this which is called "zonk". "zonk" is the name which it
// uses for this transformation, whimsically claiming that "zonk" is named
// "after the sound it makes". (Thanks to Sam for the fun trivia!)
// TODO: Untested alternate version that copies everything if anything changes.
func Extract(typ *types.Type) *types.Type {
	if typ == nil {
		return typ // or panic?
	}
	if typ.Uni != nil {
		return extract(typ) // call private helper, not a recursion!
	}

	switch typ.Kind {
	case types.KindBool:
		return typ
	case types.KindStr:
		return typ
	case types.KindInt:
		return typ
	case types.KindFloat:
		return typ

	case types.KindList:
		if typ.Val == nil {
			panic("malformed list type")
		}
		val := Extract(typ.Val)
		if val != typ.Val {
			//t := UnifyCopy(typ) // don't copy at all for normal zonk
			t := typ // bypass UnifyCopy for now
			t.Val = val
			return t
		}
		return typ

	case types.KindMap:
		if typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}
		key := Extract(typ.Key)
		val := Extract(typ.Val)
		if key != typ.Key || val != typ.Val {
			//t := UnifyCopy(typ) // don't copy at all for normal zonk
			t := typ // bypass UnifyCopy for now
			t.Key = key
			t.Val = val
			return t
		}

		// Alternate version that copies everything if anything changes.
		//key := Extract(typ.Key)
		//val := Extract(typ.Val)
		//if copied := key != typ.Key || val != typ.Val; copied {
		//	t := UnifyCopy(typ) // don't copy at all for normal zonk
		//	t.Key = key // assume
		//	t.Val = val
		//
		//	if key == typ.Key { // val changed, so copy key
		//		t.Key = UnifyCopy(key)
		//	}
		//	if val == typ.Val { // key changed, so copy val
		//		t.Val = UnifyCopy(val)
		//	}
		//	return t
		//}
		return typ

	case types.KindStruct: // {a bool; b int}
		if typ.Map == nil {
			panic("malformed struct type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed struct length")
		}
		copied := false
		m := make(map[string]*types.Type)
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}
			m[k] = Extract(t)
			if m[k] != t {
				copied = true
			}
		}
		if copied {
			//t := UnifyCopy(typ) // don't copy at all for normal zonk
			t := typ // bypass UnifyCopy for now
			for _, k := range typ.Ord {
				t.Map[k] = m[k]
				// Alternate version that copies everything if
				// anything changes.
				//if m[k] == typ.Map[k] { // field changed, so copy
				//	t.Map[k] = UnifyCopy(m[k])
				//}
			}
			return t
		}
		return typ

	case types.KindFunc:
		if typ.Map == nil {
			panic("malformed func type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed func length")
		}
		copied := false
		m := make(map[string]*types.Type)
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func field")
			}
			m[k] = Extract(t)
			if m[k] != t {
				copied = true
			}
		}
		//if typ.Out != nil {
		out := Extract(typ.Out)
		//}
		if copied || out != typ.Out {
			//t := UnifyCopy(typ) // don't copy at all for normal zonk
			t := typ // bypass UnifyCopy for now
			for _, k := range typ.Ord {
				t.Map[k] = m[k]
				// Alternate version that copies everything if
				// anything changes.
				//if m[k] == typ.Map[k] { // arg changed, so copy
				//	t.Map[k] = UnifyCopy(m[k])
				//}
			}
			t.Out = out
			// Alternate version that copies everything if anything
			// changes.
			//if out == typ.Out {
			//	t.Out = UnifyCopy(out) // out changed, so copy
			//}
			return t
		}
		return typ

	case types.KindVariant:
		v := Extract(typ.Var)
		if v != typ.Var {
			//t := UnifyCopy(typ) // don't copy at all for normal zonk
			t := typ // bypass UnifyCopy for now
			t.Var = v
			return t
		}
		return typ

	case types.KindUnification:
		return extract(typ) // duplicate case that is handled at the top
	}

	panic("malformed type")
}

// extract takes a solved type out of a unification variable data slot, and
// returns it from the input container type. If there is no contained
// unification variable, or no type in the unification variable data field, then
// this simply returns the input type without modification. Fields in the type
// tree that do not need to be copied, are not. If in any doubt copy the result.
// This helper function only looks at the direct unification variable. This is
// harmless, because it would only extract over a nested unification variable if
// this types data field was non-nil, however the recursive version named
// Extract is probably what you actually want to call.
func extract(typ *types.Type) *types.Type {
	if typ.Uni == nil {
		return typ
	}

	// Remember, the types of every Elem in the set are known to be equal to
	// each other, so there is only one .Data for the whole set, not one per
	// Elem. Some Union-Find libraries only allow you to store one piece of
	// data per set, but our library allows each Elem to hold a different
	// .Data value; that's not what we want, so we follow the convention
	// that only the root of each set has meaningful .Data, the rest have
	// junk, outdated .Data values.
	root := typ.Uni.Find() // We should definitely call find.
	//root := typ.Uni // wrong!
	if root.Data == nil {
		//return typ // normal implementation
		// Returning this way instead makes it easier for debugging, and
		// is legal to leave in forever too. (Says Sam!)
		return &types.Type{
			Kind: types.KindUnification,
			Uni:  root,
		}
	}
	// NOTE: Not doing the recursion here was an earlier major bug of mine!
	return Extract(root.Data) // return it (but it's important we recurse!)
}

// Unpack takes a solved type out of a unification variable data slot, and
// unpacks it over that unification variable to turn the type into a normal one.
// This returns false if no action was taken, or true if it modified the input.
// This public function looks at the whole type recursively, and will first
// unpack any child unification variables before finishing at the top.
func Unpack(typ *types.Type) bool {
	if typ == nil {
		return false
	}
	if typ.Uni != nil {
		return unpack(typ) // call private helper, not a recursion!
	}

	switch typ.Kind {
	case types.KindBool:
		return false
	case types.KindStr:
		return false
	case types.KindInt:
		return false
	case types.KindFloat:
		return false

	case types.KindList:
		if typ.Val == nil {
			panic("malformed list type")
		}
		return Unpack(typ.Val)

	case types.KindMap:
		if typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}
		b := Unpack(typ.Key)
		if Unpack(typ.Val) {
			b = true
		}
		return b

	case types.KindStruct: // {a bool; b int}
		if typ.Map == nil {
			panic("malformed struct type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed struct length")
		}
		b := false
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}
			if Unpack(t) {
				b = true
			}
		}
		return b

	case types.KindFunc:
		if typ.Map == nil {
			panic("malformed func type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed func length")
		}
		b := false
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func field")
			}
			if Unpack(t) {
				b = true
			}
		}
		if typ.Out != nil {
			if Unpack(typ.Out) {
				b = true
			}
		}
		return b

	case types.KindVariant:
		return Unpack(typ.Var)

	case types.KindUnification:
		return unpack(typ) // duplicate case that is handled at the top
	}

	panic("malformed type")
}

// unpack takes a solved type out of a unification variable data slot, and
// unpacks it over that unification variable to turn the type into a normal one.
// This returns false if no action was taken, or true if it modified the input.
// This helper function only looks at the direct unification variable. This is
// harmless, because it would only unpack over a nested unification variable if
// this types data field was non-nil, however the recursive version named Unpack
// is probably what you actually want to call.
func unpack(typ *types.Type) bool {
	if typ.Uni == nil {
		return false
	}
	// Remember, the types of every Elem in the set are known to be equal to
	// each other, so there is only one .Data for the whole set, not one per
	// Elem. Some Union-Find libraries only allow you to store one piece of
	// data per set, but our library allows each Elem to hold a different
	// .Data value; that's not what we want, so we follow the convention
	// that only the root of each set has meaningful .Data, the rest have
	// junk, outdated .Data values.
	root := typ.Uni.Find() // We should definitely call find.
	//root := typ.Uni // wrong!
	if root.Data == nil {
		return false
	}
	*typ = *root.Data // squash it
	return true
}

// UnifyCmp is like the standard *types.Type Cmp() except that it allows either
// type to include unification variables. It returns nil if the two types are
// consistent, identical, and don't contain any remaining unification variables.
// This function copies the input types, so this can be use safely without side
// effects from the underlying Unify operation. In the simple case where neither
// type contains unification variables, this is a more expensive version of
// typ1.Cmp(typ2). It is not necessarily valid at this time to call this if both
// sides contain unification variables. In that situation, this implementation
// will panic. It does not matter which side is the one that contains
// unification variables, as long as both of them don't. The textbook
// terminology is that we're checking that the "scrutinee" (the type which
// cannot contain unification variables) "matches" the "type pattern" (the type
// which can contain unification variables). Thanks to Sam for that information.
// TODO: It's possible that this could be useful with both sides containing
// unification variables, but keep this panic in until we find that case and add
// tests for it.
func UnifyCmp(typ1, typ2 *types.Type) error {
	if typ1.HasUni() && typ2.HasUni() {
		panic("both types have unification variables")
	}

	t1, t2 := typ1.Copy(), typ2.Copy()
	if err := Unify(t1, t2); err != nil {
		return err
	}

	e1, e2 := Extract(t1), Extract(t2)
	if e1.HasUni() || e2.HasUni() {
		return fmt.Errorf("ambiguous type cmp")
	}

	return e1.Cmp(e2)
}

// UnifyCopy copies a type, but preserves any unification variables (the .Uni)
// it encounters. It panics if it encounters an invalid or partial type struct.
func UnifyCopy(typ *types.Type) *types.Type {
	ret := &types.Type{ // return at the end
		Kind: typ.Kind,
	}
	switch typ.Kind {
	case types.KindBool:
	case types.KindStr:
	case types.KindInt:
	case types.KindFloat:

	case types.KindList:
		if typ.Val == nil {
			panic("malformed list type")
		}
		ret.Val = UnifyCopy(typ.Val)

	case types.KindMap:
		if typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}
		ret.Key = UnifyCopy(typ.Key)
		ret.Val = UnifyCopy(typ.Val)

	case types.KindStruct: // {a bool; b int}
		if typ.Map == nil {
			panic("malformed struct type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed struct length")
		}
		ret.Map = make(map[string]*types.Type)
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}
			ret.Map[k] = UnifyCopy(t)
			ret.Ord = append(ret.Ord, k)
		}

	case types.KindFunc:
		if typ.Map == nil {
			panic("malformed func type")
		}
		if len(typ.Map) != len(typ.Ord) {
			panic("malformed func length")
		}
		ret.Map = make(map[string]*types.Type)
		for _, k := range typ.Ord {
			t, ok := typ.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func field")
			}
			ret.Map[k] = UnifyCopy(t)
			ret.Ord = append(ret.Ord, k)
		}
		if typ.Out != nil {
			ret.Out = UnifyCopy(typ.Out)
		}

	case types.KindVariant:
		ret.Var = UnifyCopy(typ.Var)

	case types.KindUnification:
		if typ.Uni == nil {
			panic("malformed unification variable")
		}
		ret.Uni = typ.Uni // don't copy! (we want the same pointer)

	default:
		panic("malformed type")
	}

	return ret
}
