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
)

const (
	// LookupFuncName is the name this function is registered as.
	// This starts with an underscore so that it cannot be used from the
	// lexer.
	LookupFuncName = funcs.LookupFuncName

	// arg names...
	lookupArgNameListOrMap  = "listormap"
	lookupArgNameIndexOrKey = "indexorkey"
)

func init() {
	funcs.Register(LookupFuncName, func() interfaces.Func { return &LookupFunc{} }) // must register the func and name
}

var _ interfaces.InferableFunc = &LookupFunc{} // ensure it meets this expectation

// LookupFunc is a list index or map key lookup function. It does both because
// the current syntax in the parser is identical, so it's convenient to mix the
// two together. This calls out to some of the code in the ListLookupFunc and
// MapLookupFunc implementations. If the index or key for this input doesn't
// exist, then it will return the zero value for that type.
// TODO: Eventually we will deprecate this function when the function engine can
// support passing a value for erroring functions. (Bad index could be an err!)
type LookupFunc struct {
	Type *types.Type // Kind == List OR Map, that is used as the list/map we lookup in

	//init *interfaces.Init
	fn interfaces.BuildableFunc // handle to ListLookupFunc or MapLookupFunc
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

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *LookupFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// func(?1, ?2) ?3
	//
	// UNLESS we can be more precise, in which case it's
	//
	// func(list []?1, index int) ?1
	// OR
	// func(map map{?1: ?2}, key ?1) ?2

	// FIXME: We'd instead love to do this during type unification with a
	// callback or similar, but at least for now this handles some cases.

	var sig *types.Type
	listSig := types.NewType("func(list []?1, index int) ?1")
	mapSig := types.NewType("func(map map{?1: ?2}, key ?1) ?2")

	// If first arg is a list or map, then we know which sig to use.
	if len(partialType.Ord) == 2 && partialType.Map[partialType.Ord[0]] != nil {
		typ, exists := partialType.Map[partialType.Ord[0]]
		// don't overwrite earlier determinations
		if exists && typ.Kind == types.KindList && sig == nil {
			sig = listSig
		}
		if exists && typ.Kind == types.KindMap && sig == nil {
			sig = mapSig
		}
	}

	// If second arg is not an int, then it must be a map lookup.
	if len(partialType.Ord) == 2 && partialType.Map[partialType.Ord[1]] != nil {
		typ, exists := partialType.Map[partialType.Ord[1]]
		// don't overwrite earlier determinations
		if exists && typ.Kind != types.KindInt && sig == nil {
			sig = mapSig
		}
	}

	// If second arg is not an int, then it must be a map lookup.
	if len(partialValues) == 2 && partialValues[1] != nil {
		typ := partialValues[1].Type()
		// don't overwrite earlier determinations
		if typ != nil && typ.Kind != types.KindInt && sig == nil {
			sig = mapSig
		}
	}

	// If we haven't found a precise sig, use the less specific type.
	if sig == nil {
		sig = types.NewType("func(?1, ?2) ?3")
	}

	return sig, []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *LookupFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ == nil {
		return nil, fmt.Errorf("nil type") // happens b/c of Copy()
	}
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the lookup function needs two args")
	}
	tListOrMap, exists := typ.Map[typ.Ord[0]]
	if !exists || tListOrMap == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	if tListOrMap == nil {
		return nil, fmt.Errorf("first arg must have a type")
	}

	name := ""
	if tListOrMap.Kind == types.KindList {
		name = ListLookupFuncName
	}
	if tListOrMap.Kind == types.KindMap {
		name = MapLookupFuncName
	}
	if name == "" {
		return nil, fmt.Errorf("we must lookup from either a list or a map")
	}

	f, err := funcs.Lookup(name)
	if err != nil {
		// programming error
		return nil, err
	}

	if _, ok := f.(interfaces.CallableFunc); !ok {
		// programming error
		return nil, fmt.Errorf("not a CallableFunc")
	}

	bf, ok := f.(interfaces.BuildableFunc)
	if !ok {
		// programming error
		return nil, fmt.Errorf("not a BuildableFunc")
	}
	obj.fn = bf

	return obj.fn.Build(typ)
}

// Copy is implemented so that the type value is not lost if we copy this
// function.
func (obj *LookupFunc) Copy() interfaces.Func {
	fn := &LookupFunc{
		Type: obj.Type, // don't copy because we use this after unification

		//init: obj.init, // likely gets overwritten anyways
	}
	if _, err := fn.Build(obj.Type); err != nil {
		// ignore, since we just didn't set the type
	}
	return fn
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
	// func(list []?1, index int) ?1
	// OR
	// func(map map{?1: ?2}, key ?1) ?2
	if obj.fn == nil {
		return &interfaces.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
			Sig:  types.NewType("func(?1, ?2) ?3"), // func kind
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

// Call returns the result of this function.
func (obj *LookupFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.fn == nil {
		return nil, funcs.ErrCantSpeculate
	}
	cf, ok := obj.fn.(interfaces.CallableFunc)
	if !ok {
		// programming error
		return nil, fmt.Errorf("not a CallableFunc")
	}
	return cf.Call(ctx, args)
}
