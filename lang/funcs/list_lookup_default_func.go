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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package funcs

import (
	"context"
	"fmt"
	"math"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ListLookupDefaultFuncName is the name this function is registered as.
	ListLookupDefaultFuncName = "list_lookup_default"

	// arg names...
	listLookupDefaultArgNameList    = "list"
	listLookupDefaultArgNameIndex   = "index"
	listLookupDefaultArgNameDefault = "default"
)

func init() {
	Register(ListLookupDefaultFuncName, func() interfaces.Func { return &ListLookupDefaultFunc{} }) // must register the func and name
}

var _ interfaces.PolyFunc = &ListLookupDefaultFunc{} // ensure it meets this expectation

// ListLookupDefaultFunc is a list index lookup function. If you provide a
// negative index, then it will return the default value you specified for this
// function.
type ListLookupDefaultFunc struct {
	Type *types.Type // Kind == List, that is used as the list we lookup in

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ListLookupDefaultFunc) String() string {
	return ListLookupDefaultFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ListLookupDefaultFunc) ArgGen(index int) (string, error) {
	seq := []string{listLookupDefaultArgNameList, listLookupDefaultArgNameIndex, listLookupDefaultArgNameDefault}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *ListLookupDefaultFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(list T1, index int, default T3) T3
	// (list: []T3 => T3 aka T1 => T3)

	listName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	indexName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	defaultName, err := obj.ArgGen(2)
	if err != nil {
		return nil, err
	}

	dummyList := &interfaces.ExprAny{}    // corresponds to the list type
	dummyIndex := &interfaces.ExprAny{}   // corresponds to the index type
	dummyDefault := &interfaces.ExprAny{} // corresponds to the default type
	dummyOut := &interfaces.ExprAny{}     // corresponds to the out string

	// default type and out are the same
	invar = &interfaces.EqualityInvariant{
		Expr1: dummyDefault,
		Expr2: dummyOut,
	}
	invariants = append(invariants, invar)

	// relationship between T1 and T3
	invar = &interfaces.EqualityWrapListInvariant{
		Expr1:    dummyList,
		Expr2Val: dummyDefault,
	}
	invariants = append(invariants, invar)

	// the index has to be an int
	invar = &interfaces.EqualsInvariant{
		Expr: dummyIndex,
		Type: types.TypeInt,
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{listName, indexName, defaultName}
	mapped[listName] = dummyList
	mapped[indexName] = dummyIndex
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
				Expr2: dummyList,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyIndex,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[2],
				Expr2: dummyDefault,
			}
			invariants = append(invariants, invar)

			// If we figure out either of these types, we'll know
			// the full type...
			var t1 *types.Type // list type
			var t3 *types.Type // list val type

			// validateArg0 checks: list T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				// we happen to have a list!
				if k := typ.Kind; k != types.KindList {
					return fmt.Errorf("unable to build function with 0th arg of kind: %s", k)
				}

				if typ.Val == nil {
					// programming error
					return fmt.Errorf("list is missing type")
				}

				if err := typ.Cmp(t1); t1 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}
				if err := typ.Val.Cmp(t3); t3 != nil && err != nil {
					return errwrap.Wrapf(err, "input val type was inconsistent")
				}

				// learn!
				t1 = typ
				t3 = typ.Val
				return nil
			}

			// validateArg1 checks: list index
			validateArg1 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}
				if typ.Kind != types.KindInt {
					return errwrap.Wrapf(err, "input index type was inconsistent")
				}
				return nil
			}

			// validateArg2 checks: list val T3
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
				t := &types.Type{ // build t1
					Kind: types.KindList,
					Val:  typ, // t3
				}
				if t3 != nil {
					if err := t.Cmp(t1); t1 != nil && err != nil {
						return errwrap.Wrapf(err, "input type was inconsistent")
					}
					//t1 = t // learn!
				}

				// learn!
				t1 = t
				t3 = typ
				return nil
			}

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 and t3 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first list arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 and t3 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first list arg type is inconsistent")
				}
			}

			if typ, err := cfavInvar.Args[1].Type(); err == nil { // is it known?
				// this only checks if this is an int
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second index arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[1]]; exists { // alternate way to lookup type
				// this only checks if this is an int
				if err := validateArg1(typ); err != nil {
					return nil, errwrap.Wrapf(err, "second index arg type is inconsistent")
				}
			}

			if typ, err := cfavInvar.Args[2].Type(); err == nil { // is it known?
				// this sets t1 and t3 on success if it learned
				if err := validateArg2(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third default arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[2]]; exists { // alternate way to lookup type
				// this sets t1 and t3 on success if it learned
				if err := validateArg2(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third default arg type is inconsistent")
				}
			}

			// XXX: if the types aren't know statically?

			if t1 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyList,
					Type: t1,
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
func (obj *ListLookupDefaultFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 3 {
		return nil, fmt.Errorf("the listlookup function needs exactly three args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	tList, exists := typ.Map[typ.Ord[0]]
	if !exists || tList == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}

	tIndex, exists := typ.Map[typ.Ord[1]]
	if !exists || tIndex == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}

	tDefault, exists := typ.Map[typ.Ord[2]]
	if !exists || tDefault == nil {
		return nil, fmt.Errorf("third arg must be specified")
	}

	if tIndex != nil && tIndex.Kind != types.KindInt {
		return nil, fmt.Errorf("index must be int kind")
	}

	if err := tList.Val.Cmp(tDefault); err != nil {
		return nil, errwrap.Wrapf(err, "default must match list val type")
	}

	if err := tList.Val.Cmp(typ.Out); err != nil {
		return nil, errwrap.Wrapf(err, "return type must match list val type")
	}

	obj.Type = tList // list type
	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *ListLookupDefaultFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindList {
		return fmt.Errorf("type must be a kind of list")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ListLookupDefaultFunc) Info() *interfaces.Info {
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
func (obj *ListLookupDefaultFunc) sig() *types.Type {
	v := obj.Type.Val.String()
	return types.NewType(fmt.Sprintf("func(%s %s, %s int, %s %s) %s", listLookupDefaultArgNameList, obj.Type.String(), listLookupDefaultArgNameIndex, listLookupDefaultArgNameDefault, v, v))
}

// Init runs some startup code for this function.
func (obj *ListLookupDefaultFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ListLookupDefaultFunc) Stream(ctx context.Context) error {
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

			l := (input.Struct()[listLookupDefaultArgNameList]).(*types.ListValue)
			index := input.Struct()[listLookupDefaultArgNameIndex].Int()
			def := input.Struct()[listLookupDefaultArgNameDefault]

			// TODO: should we handle overflow by returning default?
			if index > math.MaxInt { // max int size varies by arch
				return fmt.Errorf("list index overflow, got: %d, max is: %d", index, math.MaxInt32)
			}

			// negative index values are "not found" here!
			var result types.Value
			val, exists := l.Lookup(int(index))
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
