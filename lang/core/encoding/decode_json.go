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

package coreencoding

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	jsonUtil "github.com/purpleidea/mgmt/lang/types/json"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// DecodeJSONFuncName is the name this function is registered as.
	DecodeJSONFuncName = "decode_json"

	// arg names...
	decodeJSONArgNameData = "data"
	decodeJSONArgNameType = "type"
)

func init() {
	funcs.ModuleRegister(ModuleName, DecodeJSONFuncName, func() interfaces.Func { return &DecodeJSONFunc{} })
}

var _ interfaces.InferableFunc = &DecodeJSONFunc{} // ensure it meets this expectation

// DecodeJSONFunc is a static polymorphic function that accepts some json data
// and returns an mcl struct representing that data. You currently should pass
// in an mcl type representation in the second arg so that we know what type to
// parse the json as. There are some cases when the type can be inferred, and
// you don't need to specify the second arg. It's your problem if the type
// unification doesn't work though!
type DecodeJSONFunc struct {
	// Type is the type of the type specification (2nd) arg if one is
	// specified. Nil is the special undetermined value that is used before
	// type is known.
	Type *types.Type // type of return value

	length int  // number of arguments
	built  bool // was this function built yet?

	init *interfaces.Init
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *DecodeJSONFunc) String() string {
	return DecodeJSONFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *DecodeJSONFunc) ArgGen(index int) (string, error) {
	seq := []string{decodeJSONArgNameData, decodeJSONArgNameType}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *DecodeJSONFunc) sig() *types.Type {
	if obj.length == 0 { // not yet known
		return nil
	}

	v := "?1"
	if obj.Type != nil { // don't panic if called speculatively
		v = obj.Type.String()
	}

	if obj.length == 1 {
		typ := fmt.Sprintf("func(%s str) %s", decodeJSONArgNameData, v)
		return types.NewType(typ)
	}

	// obj.length == 2
	typ := fmt.Sprintf("func(%s str, %s str) %s", decodeJSONArgNameData, decodeJSONArgNameType, v)
	return types.NewType(typ)
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *DecodeJSONFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// func(data str) ?1
	// OR
	// func(data str, type str) ?1

	if l := len(partialValues); l < 1 || l > 2 {
		return nil, nil, fmt.Errorf("must have at either one or two args")
	}

	obj.length = len(partialValues) // store for later

	// XXX: Add this sort of thing if were ever possible...
	//if len(partialValues) == 1 && partialValues[0] != nil {
	//	typ, err := jsonUtil.TypeOfJSON(partialValues[0].Str())
	//	if err == nil {
	//		if err := typ.Cmp(obj.Type); obj.Type != nil && err != nil {
	//			return nil, nil, errwrap.Wrapf(err, "incompatible types")
	//		}
	//		obj.Type = typ
	//	}
	//}

	if len(partialValues) == 2 && partialValues[1] != nil {
		typ := types.NewType(partialValues[1].Str())
		if typ == nil {
			return nil, nil, fmt.Errorf("invalid type specification")
		}
		if err := typ.Cmp(obj.Type); obj.Type != nil && err != nil {
			return nil, nil, errwrap.Wrapf(err, "incompatible types")
		}
		obj.Type = typ // we know early!
	}

	return obj.sig(), []*interfaces.UnificationInvariant{}, nil
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static.
func (obj *DecodeJSONFunc) Build(typ *types.Type) (*types.Type, error) {
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}
	if obj.length < 1 {
		return nil, fmt.Errorf("the function needs at least one arg")
	}
	if obj.length > 2 {
		return nil, fmt.Errorf("the function needs at most two args")
	}
	if len(typ.Ord) != 2 && len(typ.Ord) != 1 {
		return nil, fmt.Errorf("the function needs exactly one or two args")
	}
	//if len(typ.Ord) != 2 {
	//	return nil, fmt.Errorf("the function needs exactly two args")
	//}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	t0, exists := typ.Map[typ.Ord[0]]
	if !exists || t0 == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	if t0.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("first arg for function must be an str")
	}

	if len(typ.Ord) == 1 { // no args being passed in
		obj.Type = typ.Out // extracted return type is now known!
		obj.built = true
		return obj.sig(), nil
	}

	t1, exists := typ.Map[typ.Ord[1]]
	if !exists || t1 == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}
	if t1.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("second arg for function must be an str")
	}

	obj.Type = typ.Out // extracted return type is now known!
	obj.built = true
	return obj.sig(), nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *DecodeJSONFunc) Validate() error {
	if obj.length == 0 {
		return fmt.Errorf("function not built correctly")
	}
	if !obj.built {
		return fmt.Errorf("function wasn't built yet")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *DecodeJSONFunc) Info() *interfaces.Info {
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	var sig *types.Type
	if obj.built {
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: true, // must be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *DecodeJSONFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Copy is implemented so that the obj.built value is not lost if we copy this
// function.
func (obj *DecodeJSONFunc) Copy() interfaces.Func {
	return &DecodeJSONFunc{
		Type:   obj.Type, // don't copy because we use this after unification
		length: obj.length,
		built:  obj.built,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *DecodeJSONFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	data := args[0].Str()

	var typ *types.Type // nil
	if len(args) == 2 {
		typ = types.NewType(args[1].Str()) // could be nil!
	}

	// We have two types, so check they're compatible!
	if typ != nil && obj.Type != nil {
		if err := obj.Type.Cmp(typ); err != nil {
			return nil, errwrap.Wrapf(err, "incompatible types")
		}
	}

	// Set it if we've got it. We already know it's nil or we're compatible.
	if obj.Type != nil {
		typ = obj.Type
	}

	// If we're early, then this might be nil. Error correctly in this case.
	if obj.init == nil && typ == nil {
		return nil, funcs.ErrCantSpeculate
	}

	// XXX: This particular function, requires typ and can't guess the type!
	// Perhaps a future version with a better markup language can just know!
	return jsonUtil.ValueOfJSON(data, typ)
}
