// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
		Ord:  []string{"map", "key", "default"},
		Out:  nil,
	}
	typFunc.Map["map"] = typ
	typFunc.Map["key"] = typ.Key
	typFunc.Map["default"] = typ.Val
	typFunc.Out = typ.Val

	// TODO: don't include partial internal func map's for now, allow in future?
	if typ.Key == nil || typ.Val == nil {
		typFunc.Map = make(map[string]*types.Type) // erase partial
		typFunc.Map["map"] = types.TypeVariant
		typFunc.Map["key"] = types.TypeVariant
		typFunc.Map["default"] = types.TypeVariant
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
	typ := types.NewType(fmt.Sprintf("func(map %s, key %s, default %s) %s", obj.Type.String(), obj.Type.Key.String(), obj.Type.Val.String(), obj.Type.Val.String()))
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

			m := (input.Struct()["map"]).(*types.MapValue)
			key := input.Struct()["key"]
			def := input.Struct()["default"]

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
