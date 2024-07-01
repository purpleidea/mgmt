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

package funcs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// MapLookupFuncName is the name this function is registered as.
	MapLookupFuncName = "map_lookup"

	// arg names...
	mapLookupArgNameMap = "map"
	mapLookupArgNameKey = "key"
)

func init() {
	Register(MapLookupFuncName, func() interfaces.Func { return &MapLookupFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &MapLookupFunc{} // ensure it meets this expectation

// MapLookupFunc is a key map lookup function. If you provide a missing key,
// then it will return the zero value for that type.
type MapLookupFunc struct {
	Type *types.Type // Kind == Map, that is used as the map we lookup

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
	seq := []string{mapLookupArgNameMap, mapLookupArgNameKey}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *MapLookupFunc) sig() *types.Type {
	// func(map map{?1: ?2}, key ?1) ?2
	k := "?1"
	v := "?2"
	m := fmt.Sprintf("map{%s: %s}", k, v)
	if obj.Type != nil { // don't panic if called speculatively
		k = obj.Type.Key.String()
		v = obj.Type.Val.String()
		m = obj.Type.String()
	}
	return types.NewType(fmt.Sprintf(
		"func(%s %s, %s %s) %s",
		mapLookupArgNameMap, m,
		mapLookupArgNameKey, k,
		v,
	))
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

	if len(typ.Ord) != 2 {
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

	obj.Type = tMap // map type
	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *MapLookupFunc) Validate() error {
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
func (obj *MapLookupFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *MapLookupFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *MapLookupFunc) Stream(ctx context.Context) error {
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

			m := (input.Struct()[mapLookupArgNameMap]).(*types.MapValue)
			key := input.Struct()[mapLookupArgNameKey]
			zero := m.Type().New() // the zero value

			var result types.Value
			val, exists := m.Lookup(key)
			if exists {
				result = val
			} else {
				result = zero
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
