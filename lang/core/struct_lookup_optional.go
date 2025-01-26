// Mgmt
// Copyright (C) James Shubin and the project contributors
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

package core

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// StructLookupOptionalFuncName is the name this function is registered
	// as. This starts with an underscore so that it cannot be used from the
	// lexer.
	StructLookupOptionalFuncName = funcs.StructLookupOptionalFuncName

	// arg names...
	structLookupOptionalArgNameStruct   = "struct"
	structLookupOptionalArgNameField    = "field"
	structLookupOptionalArgNameOptional = "optional"
)

func init() {
	funcs.Register(StructLookupOptionalFuncName, func() interfaces.Func { return &StructLookupOptionalFunc{} }) // must register the func and name
}

var _ interfaces.InferableFunc = &StructLookupOptionalFunc{} // ensure it meets this expectation

// StructLookupOptionalFunc is a struct field lookup function. It does a special
// trick in that it will unify on a struct that doesn't have the specified field
// in it, but in that case, it will always return the optional value. This is a
// bit different from the "default" mechanism that is used by list and map
// lookup functions.
type StructLookupOptionalFunc struct {
	Type *types.Type // Kind == Struct, that is used as the struct we lookup
	Out  *types.Type // type of field we're extracting (also the type of optional)

	built bool // was this function built yet?

	init  *interfaces.Init
	last  types.Value // last value received to use for diff
	field string

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *StructLookupOptionalFunc) String() string {
	return StructLookupOptionalFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *StructLookupOptionalFunc) ArgGen(index int) (string, error) {
	seq := []string{structLookupOptionalArgNameStruct, structLookupOptionalArgNameField, structLookupOptionalArgNameOptional}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *StructLookupOptionalFunc) sig() *types.Type {
	st := "?1"
	out := "?2"
	if obj.Type != nil {
		st = obj.Type.String()
	}
	if obj.Out != nil {
		out = obj.Out.String()
	}

	return types.NewType(fmt.Sprintf(
		"func(%s %s, %s str, %s %s) %s",
		structLookupOptionalArgNameStruct, st,
		structLookupOptionalArgNameField,
		structLookupOptionalArgNameOptional, out,
		out,
	))
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *StructLookupOptionalFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// func(struct ?1, field str, optional ?2) ?2

	// This particular function should always get called with a known string
	// for the second argument. Without it being known statically, we refuse
	// to build this function.

	if l := 3; len(partialValues) != l {
		return nil, nil, fmt.Errorf("function must have %d args", l)
	}
	if err := partialValues[1].Type().Cmp(types.TypeStr); err != nil {
		return nil, nil, errwrap.Wrapf(err, "function field name must be a str")
	}
	s := partialValues[1].Str() // must not panic
	if s == "" {
		return nil, nil, fmt.Errorf("function must not have an empty field name")
	}
	// This can happen at runtime too, but we save it here for Build()!
	//obj.field = s // don't store for this optional lookup version!

	// Figure out more about the sig if any information is known statically.
	if len(partialType.Ord) > 0 && partialType.Map[partialType.Ord[0]] != nil {
		obj.Type = partialType.Map[partialType.Ord[0]] // assume this
		if obj.Type.Kind == types.KindStruct && obj.Type.Map != nil {
			if typ, exists := obj.Type.Map[s]; exists {
				obj.Out = typ
			}
		}
	}

	// This isn't precise enough because we must guarantee that the field is
	// in the struct and that ?1 is actually a struct, but that's okay it is
	// something that we'll verify at build time! (Or skip it for optional!)

	return obj.sig(), []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *StructLookupOptionalFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 3 {
		return nil, fmt.Errorf("the structlookup function needs exactly three args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	tStruct, exists := typ.Map[typ.Ord[0]]
	if !exists || tStruct == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}

	tField, exists := typ.Map[typ.Ord[1]]
	if !exists || tField == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}
	if err := tField.Cmp(types.TypeStr); err != nil {
		return nil, errwrap.Wrapf(err, "field must be an str")
	}

	tOptional, exists := typ.Map[typ.Ord[2]]
	if !exists || tOptional == nil {
		return nil, fmt.Errorf("third arg must be specified")
	}
	if err := tOptional.Cmp(typ.Out); err != nil {
		return nil, errwrap.Wrapf(err, "optional arg must match return type")
	}

	// NOTE: We actually don't know which field this is yet, only its type!
	// We don't care, because that's a runtime issue and doesn't need to be
	// our problem as long as this is a struct. The only optimization we can
	// add is to know statically if we're returning the optional value.
	if tStruct.Kind != types.KindStruct {
		return nil, fmt.Errorf("first arg must be of kind struct, got: %s", tStruct.Kind)
	}

	obj.Type = tStruct // struct type
	obj.Out = typ.Out  // type of return value
	obj.built = true

	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *StructLookupOptionalFunc) Validate() error {
	if !obj.built {
		return fmt.Errorf("function wasn't built yet")
	}
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindStruct {
		return fmt.Errorf("type must be a kind of struct")
	}
	if obj.Out == nil {
		return fmt.Errorf("return type must be specified")
	}

	// TODO: can we do better and validate more aspects here?

	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *StructLookupOptionalFunc) Info() *interfaces.Info {
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	var sig *types.Type
	if obj.built {
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *StructLookupOptionalFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *StructLookupOptionalFunc) Stream(ctx context.Context) error {
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

			st := (input.Struct()[structLookupOptionalArgNameStruct]).(*types.StructValue)
			field := input.Struct()[structLookupOptionalArgNameField].Str()
			optional := input.Struct()[structLookupOptionalArgNameOptional]

			if field == "" {
				return fmt.Errorf("received empty field")
			}
			if obj.field == "" {
				// This can happen at compile time too. Bonus!
				obj.field = field // store first field
			}
			if field != obj.field {
				return fmt.Errorf("input field changed from: `%s`, to: `%s`", obj.field, field)
			}

			// We know the result of this lookup statically at
			// compile time, but for simplicity we check each time
			// here anyways. Maybe one day there will be a fancy
			// reason why this might vary over time.
			var result types.Value
			val, exists := st.Lookup(obj.field)
			if exists {
				result = val
			} else {
				result = optional
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
