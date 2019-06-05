// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
