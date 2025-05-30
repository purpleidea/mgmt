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

package structs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// CompositeFuncName is the unique name identifier for this function.
	CompositeFuncName = "composite"
)

// CompositeFunc is a function that passes through the value it receives. It is
// used to take a series of inputs to a list, map or struct, and return that
// value as a stream that depends on those inputs. It helps the list, map, and
// struct's that fulfill the Expr interface but expressing a Func method.
type CompositeFunc struct {
	Type *types.Type // this is the type of the composite value we hold
	Len  int         // length of list or map (if used)

	init   *interfaces.Init
	last   types.Value // last value received to use for diff
	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *CompositeFunc) String() string {
	if obj.Type != nil {
		return fmt.Sprintf("%s: %s", CompositeFuncName, obj.Type.String())
	}
	return CompositeFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *CompositeFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	switch obj.Type.Kind {
	case types.KindList:
		fallthrough
	case types.KindMap:
		fallthrough
	case types.KindStruct:
		return nil
	}

	return fmt.Errorf("can't compose type `%s`", obj.Type.String())
}

// Info returns some static info about itself.
func (obj *CompositeFunc) Info() *interfaces.Info {
	typ := &types.Type{
		Kind: types.KindFunc, // function type
		Map:  make(map[string]*types.Type),
		Ord:  []string{},
		Out:  obj.Type, // this is the output type for the expression
	}

	// This populates the .Map and .Ord fields of the above type.
	obj.makeStructType(typ) // hack =D

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// makeStructType is a helper that adds the map/ord properties onto a type. This
// should match the type we expect for this composite struct.
func (obj *CompositeFunc) makeStructType(typ *types.Type) {

	switch obj.Type.Kind {
	case types.KindList: // wrapped in a struct with `length` many keys
		for i := 0; i < obj.Len; i++ {
			// FIXME: should we .Title the fields or add a prefix?
			key := fmt.Sprintf("%d", i)
			typ.Map[key] = obj.Type.Val // type of each list element
			typ.Ord = append(typ.Ord, key)
		}

	case types.KindMap: // wrapped in a struct with named keys
		for i := 0; i < obj.Len; i++ {
			// each key and val has a value to pass in, and we have
			// a known number of kv pairs, so we pass each in with
			// the index of the kv pair as found in the parse order
			key1 := fmt.Sprintf("key:%d", i)
			typ.Map[key1] = obj.Type.Key // type of each map key
			typ.Ord = append(typ.Ord, key1)

			key2 := fmt.Sprintf("val:%d", i)
			typ.Map[key2] = obj.Type.Val // type of each map val
			typ.Ord = append(typ.Ord, key2)
		}

	case types.KindStruct:
		// map it directly, each key is the right input!
		typ.Map = obj.Type.Map
		typ.Ord = obj.Type.Ord
	}
}

// Init runs some startup code for this composite function.
func (obj *CompositeFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *CompositeFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back

				if obj.last == nil {
					result, err := obj.StructCall(ctx, obj.last)
					if err != nil {
						return err
					}

					obj.result = result
					select {
					case obj.init.Output <- result: // send
						// pass
					case <-ctx.Done():
						return nil
					}
				}

				return nil // can't output any more
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}
			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			// TODO: use the normal Call interface instead?
			//args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			//if err != nil {
			//	return err
			//}
			//result, err := obj.Call(ctx, args)
			//if err != nil {
			//	return err
			//}
			result, err := obj.StructCall(ctx, input)
			if err != nil {
				return err
			}

			// skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-ctx.Done():
			return nil
		}
	}
}

// StructCall is a different Call API which is sometimes easier to implement.
func (obj *CompositeFunc) StructCall(ctx context.Context, st types.Value) (types.Value, error) {
	if st == nil {
		// FIXME: can we get an empty struct?
		result := obj.Type.New() // new list or map
		return result, nil
	}

	var result types.Value
	switch obj.Type.Kind {
	case types.KindList:
		// XXX: this duplicates the same logic that exists in Value() as implemented on *ExprList
		// XXX: have this call that function to get the result?
		result = obj.Type.New()          // new list
		input := st.(*types.StructValue) // must be!
		for i := 0; i < obj.Len; i++ {   // build it
			value, exists := input.Lookup(fmt.Sprintf("%d", i)) // argNames as integers!
			if !exists {
				return nil, fmt.Errorf("missing input index `%d`", i)
			}
			if err := result.(*types.ListValue).Add(value); err != nil {
				return nil, errwrap.Wrapf(err, "can't build list index `%d`", i)
			}
		}

	case types.KindMap:
		result = obj.Type.New()                     // new map
		input := (st.(*types.StructValue)).Struct() // must be!
		l := len(input)
		if l%2 != 0 {
			return nil, fmt.Errorf("expected even number of inputs for a map, got: %d", l)
		}

		// each key should be named `key:0`, `val:0`, `key:1`, `val:1`,
		// and so on for as many key pairs as we have... remember that
		// the number of keys pairs is known statically in this case!
		for i := 0; i < l/2; i++ { // build it
			key, exists := input[fmt.Sprintf("key:%d", i)]
			if !exists {
				return nil, fmt.Errorf("missing input key `key:%d`", i)
			}
			val, exists := input[fmt.Sprintf("val:%d", i)]
			if !exists {
				return nil, fmt.Errorf("missing input val `val:%d`", i)
			}

			if err := result.(*types.MapValue).Add(key, val); err != nil {
				return nil, errwrap.Wrapf(err, "can't build map key with index `%d`", i)
			}
		}

	case types.KindStruct:
		result = st
	}

	return result, nil
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *CompositeFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {

	typ := &types.Type{
		Kind: types.KindStruct,
		Map:  make(map[string]*types.Type),
		Ord:  []string{},
	}
	obj.makeStructType(typ) // hack =D

	st := (typ.New().(*types.StructValue)) // new struct

	switch obj.Type.Kind {
	case types.KindList:
		for i, arg := range args {
			key := fmt.Sprintf("%d", i)
			st.V[key] = arg
		}

	case types.KindMap:
		for i, arg := range args {
			if i%2 == 0 {
				key1 := fmt.Sprintf("key:%d", i)
				st.V[key1] = arg
			} else {
				key2 := fmt.Sprintf("val:%d", i)
				st.V[key2] = arg
			}
		}

	case types.KindStruct:
		for i, arg := range args {
			key := obj.Type.Ord[i]
			st.V[key] = arg
		}
	}

	return obj.StructCall(ctx, st)
}
