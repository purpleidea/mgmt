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

package coremap

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/wrapped"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// MapLookupFuncName is the name this function is registered as.
	MapLookupFuncName = "lookup" // map.lookup

	// arg names...
	mapLookupArgNameMap = "map"
	mapLookupArgNameKey = "key"
	mapLookupArgNameDef = "default"
)

func init() {
	funcs.ModuleRegister(ModuleName, MapLookupFuncName, func() interfaces.Func { return &MapLookupFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &MapLookupFunc{} // ensure it meets this expectation

// MapLookupFunc is a key map lookup function. It can take either two or three
// arguments. The first argument is the map to lookup a value by key in. The
// second is the key to use. If you specify a third argument, then this value
// will be returned if the map key is not present. If the third argument is
// omitted, then this function errors if the map key is not present.
type MapLookupFunc struct {
	*wrapped.Func // *wrapped.Func as a type alias to pull in the base impl.

	Type *types.Type // Kind == Map, that is used as the map we lookup

	hasDefault *bool // does this function take a third arg?

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
	seq := []string{mapLookupArgNameMap, mapLookupArgNameKey, mapLookupArgNameDef}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *MapLookupFunc) sig() *types.Type {
	// func(map map{?1: ?2}, key ?1) ?2
	// OR
	// func(map map{?1: ?2}, key ?1, default ?2) ?2

	if obj.hasDefault == nil { // not yet known
		return nil
	}

	k := "?1"
	v := "?2"
	m := fmt.Sprintf("map{%s: %s}", k, v)
	if obj.Type != nil { // don't panic if called speculatively
		k = obj.Type.Key.String()
		v = obj.Type.Val.String()
		m = obj.Type.String()
	}

	if *obj.hasDefault {
		return types.NewType(fmt.Sprintf(
			"func(%s %s, %s %s, %s %s) %s",
			mapLookupArgNameMap, m,
			mapLookupArgNameKey, k,
			mapLookupArgNameDef, v,
			v,
		))
	}

	return types.NewType(fmt.Sprintf(
		"func(%s %s, %s %s) %s",
		mapLookupArgNameMap, m,
		mapLookupArgNameKey, k,
		v,
	))
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *MapLookupFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	if l := len(partialValues); l != 2 && l != 3 {
		return nil, nil, fmt.Errorf("function must have either 2 or 3 args")
	}

	b := false
	//if len(partialValues) == 2 {
	//	b = false
	//}
	if len(partialValues) == 3 {
		b = true
	}
	obj.hasDefault = &b // store for later

	return obj.sig(), []*interfaces.UnificationInvariant{}, nil
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

	if obj.hasDefault == nil {
		return nil, fmt.Errorf("the maplookup function needs exactly two or three args")
	}
	if *obj.hasDefault && len(typ.Ord) != 3 {
		return nil, fmt.Errorf("the maplookup function needs exactly three args")
	}
	if !*obj.hasDefault && len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the maplookup function needs exactly two args")
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

	if *obj.hasDefault {
		tDef, exists := typ.Map[typ.Ord[2]]
		if !exists || tDef == nil {
			return nil, fmt.Errorf("third arg must be specified")
		}
		if err := tMap.Val.Cmp(tDef); err != nil {
			return nil, errwrap.Wrapf(err, "default must match map val type")
		}
	}

	obj.Func = &wrapped.Func{
		Name: obj.String(),
		FuncInfo: &wrapped.Info{
			// TODO: dedup with below Info data
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		Type: typ, // .Copy(),
	}

	obj.Type = tMap // map type
	fn := &types.FuncValue{
		T: typ,
		V: obj.Call, // implementation
	}
	obj.Fn = fn // inside wrapper.Func
	//return obj.Fn.T, nil
	return obj.sig(), nil
}

// Copy is implemented so that the obj.hasDefault value is not lost if we copy
// this function. That value is learned during FuncInfer, and previously would
// have been lost by the time we used it in Build.
func (obj *MapLookupFunc) Copy() interfaces.Func {
	return &MapLookupFunc{
		Type:       obj.Type, // don't copy because we use this after unification
		hasDefault: obj.hasDefault,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Call is the actual implementation here. This is used in lieu of the Stream
// function which we'd have these contents within.
func (obj *MapLookupFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("not enough args")
	}
	m := (args[0]).(*types.MapValue)
	key := args[1]
	//zero := m.Type().Val.New() // the zero value

	val, exists := m.Lookup(key)
	if exists {
		return val, nil
	}
	if len(args) == 3 { // default value since lookup is missing
		return args[2], nil
	}

	return nil, fmt.Errorf("map key not present, got: %v", key)
}

// Validate tells us if the input struct takes a valid form.
func (obj *MapLookupFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindMap {
		return fmt.Errorf("type must be a kind of map")
	}
	if obj.hasDefault == nil {
		return fmt.Errorf("function not built correctly")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *MapLookupFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: true,
		Fast: true,
		Spec: true,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}
