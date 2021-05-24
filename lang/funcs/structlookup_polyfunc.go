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

package funcs

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// StructLookupFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	// XXX: change to _structlookup and add syntax in the lexer/parser
	StructLookupFuncName = "structlookup"
)

func init() {
	Register(StructLookupFuncName, func() interfaces.Func { return &StructLookupPolyFunc{} }) // must register the func and name
}

// StructLookupPolyFunc is a key map lookup function.
type StructLookupPolyFunc struct {
	Type *types.Type // Kind == Struct, that is used as the struct we lookup
	Out  *types.Type // type of field we're extracting

	init  *interfaces.Init
	last  types.Value // last value received to use for diff
	field string

	result types.Value // last calculated output

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *StructLookupPolyFunc) ArgGen(index int) (string, error) {
	seq := []string{"struct", "field"}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *StructLookupPolyFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(struct T1, field str) T2

	structName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	fieldName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	dummyStruct := &interfaces.ExprAny{} // corresponds to the struct type
	dummyField := &interfaces.ExprAny{}  // corresponds to the field type
	dummyOut := &interfaces.ExprAny{}    // corresponds to the out string

	// field arg type of string
	invar = &interfaces.EqualsInvariant{
		Expr: dummyField,
		Type: types.TypeStr,
	}
	invariants = append(invariants, invar)

	// XXX: we could use this relationship *if* our solver could understand
	// different fields, and partial struct matches. I guess we'll leave it
	// for another day!
	//mapped := make(map[string]interfaces.Expr)
	//ordered := []string{???}
	//mapped[???] = dummyField
	//invar = &interfaces.EqualityWrapStructInvariant{
	//	Expr1:    dummyStruct,
	//	Expr2Map: mapped,
	//	Expr2Ord: ordered,
	//}
	//invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{structName, fieldName}
	mapped[structName] = dummyStruct
	mapped[fieldName] = dummyField

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

			// add the relationship to the returned value
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Expr,
				Expr2: dummyOut,
			}
			invariants = append(invariants, invar)

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyStruct,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyField,
			}
			invariants = append(invariants, invar)

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// second arg must be a string
			invar = &interfaces.EqualsInvariant{
				Expr: cfavInvar.Args[1],
				Type: types.TypeStr,
			}
			invariants = append(invariants, invar)

			value, err := cfavInvar.Args[1].Value() // is it known?
			if err != nil {
				return nil, fmt.Errorf("field string is not known statically")
			}

			if k := value.Type().Kind; k != types.KindStr {
				return nil, fmt.Errorf("unable to build function with 1st arg of kind: %s", k)
			}
			field := value.Str() // must not panic

			// If we figure out both of these two types, we'll know
			// the full type...
			var t1 *types.Type // struct type
			var t2 *types.Type // return type

			// validateArg0 checks: struct T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				// we happen to have a struct!
				if k := typ.Kind; k != types.KindStruct {
					return fmt.Errorf("unable to build function with 0th arg of kind: %s", k)
				}

				// check both Ord and Map for safety
				found := false
				for _, s := range typ.Ord {
					if s == field {
						found = true
						break
					}
				}
				t, exists := typ.Map[field] // type found is T2
				if !exists || !found {
					return fmt.Errorf("struct is missing field: %s", field)
				}

				if err := typ.Cmp(t1); t1 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}

				if err := t.Cmp(t2); t2 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}

				// learn!
				t1 = typ
				t2 = t
				return nil
			}

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first struct arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first struct arg type is inconsistent")
				}
			}

			// XXX: if the struct type/value isn't know statically?

			if t1 != nil {
				invar = &interfaces.EqualsInvariant{
					Expr: dummyStruct,
					Type: t1,
				}
				invariants = append(invariants, invar)

				// We know *some* information about the struct!
				// Let's hope the unusedField expr won't trip
				// up the solver...
				mapped := make(map[string]interfaces.Expr)
				ordered := []string{}
				for _, x := range t1.Ord {
					// We *don't* need to solve unusedField
					unusedField := &interfaces.ExprAny{}
					mapped[x] = unusedField
					if x == field { // the one we care about
						mapped[x] = dummyOut
					}
					ordered = append(ordered, x)
				}
				// We map to dummyOut which is the return type
				// and has the same type of the field we want!
				mapped[field] = dummyOut // redundant =D
				invar = &interfaces.EqualityWrapStructInvariant{
					Expr1:    dummyStruct,
					Expr2Map: mapped,
					Expr2Ord: ordered,
				}
				invariants = append(invariants, invar)
			}
			if t2 != nil {
				invar := &interfaces.EqualsInvariant{
					Expr: dummyOut,
					Type: t2,
				}
				invariants = append(invariants, invar)
			}

			// XXX: if t1 or t2 are missing, we could also return a
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
func (obj *StructLookupPolyFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: return `variant` as arg for now -- maybe there's a better way?
	variant := []*types.Type{types.NewType("func(struct variant, field str) variant")}

	if partialType == nil {
		return variant, nil
	}

	var typ *types.Type // struct type of the first argument
	var out *types.Type // type of the field

	// TODO: if partialValue[0] exists, check it matches the type we expect
	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) != 2 {
			return nil, fmt.Errorf("must have exactly two args in structlookup func")
		}
		if tStruct, exists := partialType.Map[ord[0]]; exists && tStruct != nil {
			if tStruct.Kind != types.KindStruct {
				return nil, fmt.Errorf("first arg for structlookup must be a struct")
			}
			if !tStruct.HasVariant() {
				typ = tStruct // found
			}
		}
		if tField, exists := partialType.Map[ord[1]]; exists && tField != nil {
			if tField.Cmp(types.TypeStr) != nil {
				return nil, fmt.Errorf("second arg for structlookup must be a string")
			}
		}

		if len(partialValues) == 2 && partialValues[1] != nil {
			if types.TypeStr.Cmp(partialValues[1].Type()) != nil {
				return nil, fmt.Errorf("second value must be an str")
			}
			structType, exists := partialType.Map[ord[0]]
			if !exists {
				return nil, fmt.Errorf("missing struct field")
			}
			if structType != nil {
				field := partialValues[1].Str()
				fieldType, exists := structType.Map[field]
				if !exists {
					return nil, fmt.Errorf("field: `%s` does not exist in struct", field)
				}
				if fieldType != nil {
					if partialType.Out != nil && fieldType.Cmp(partialType.Out) != nil {
						return nil, fmt.Errorf("field `%s` must have same type as return type", field)
					}

					out = fieldType // found!
				}
			}
		}

		if tOut := partialType.Out; tOut != nil {
			// TODO: we could check that at least one of the types
			// in struct.Map was our type, but not very useful...
		}
	}

	typFunc := &types.Type{
		Kind: types.KindFunc, // function type
		Map:  make(map[string]*types.Type),
		Ord:  []string{"struct", "field"},
		Out:  out,
	}
	typFunc.Map["struct"] = typ
	typFunc.Map["field"] = types.TypeStr

	// set variant instead of nil
	if typFunc.Map["struct"] == nil {
		typFunc.Map["struct"] = types.TypeVariant
	}
	if out == nil {
		typFunc.Out = types.TypeVariant
	}

	return []*types.Type{typFunc}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *StructLookupPolyFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return fmt.Errorf("the structlookup function needs exactly two args")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return fmt.Errorf("invalid input type")
	}

	tStruct, exists := typ.Map[typ.Ord[0]]
	if !exists || tStruct == nil {
		return fmt.Errorf("first arg must be specified")
	}

	tField, exists := typ.Map[typ.Ord[1]]
	if !exists || tField == nil {
		return fmt.Errorf("second arg must be specified")
	}
	if err := tField.Cmp(types.TypeStr); err != nil {
		return errwrap.Wrapf(err, "field must be an str")
	}

	// NOTE: We actually don't know which field this is, only its type! we
	// could have cached the discovered field during Polymorphisms(), but it
	// turns out it's not actually necessary for us to know it to build the
	// struct.
	obj.Type = tStruct // struct type
	obj.Out = typ.Out  // type of return value
	return nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *StructLookupPolyFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindStruct {
		return fmt.Errorf("type must be a kind of struct")
	}
	if obj.Out == nil {
		return fmt.Errorf("return type must be specified")
	}

	for _, t := range obj.Type.Map {
		if obj.Out.Cmp(t) == nil {
			return nil // found at least one match
		}
	}
	return fmt.Errorf("return type is not in the list of available struct fields")
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *StructLookupPolyFunc) Info() *interfaces.Info {
	var sig *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		// TODO: can obj.Out be nil (a partial) ?
		sig = types.NewType(fmt.Sprintf("func(struct %s, field str) %s", obj.Type.String(), obj.Out.String()))
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  sig, // func kind
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *StructLookupPolyFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *StructLookupPolyFunc) Stream() error {
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

			st := (input.Struct()["struct"]).(*types.StructValue)
			field := input.Struct()["field"].Str()

			if field == "" {
				return fmt.Errorf("received empty field")
			}

			result, exists := st.Lookup(field)
			if !exists {
				return fmt.Errorf("could not lookup field: `%s` in struct", field)
			}

			if obj.field == "" {
				obj.field = field // store first field
			}

			if field != obj.field {
				return fmt.Errorf("input field changed from: `%s`, to: `%s`", obj.field, field)
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
func (obj *StructLookupPolyFunc) Close() error {
	close(obj.closeChan)
	return nil
}
