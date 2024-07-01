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

package corevalue

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// GetFuncName is the name this function is registered as. This variant
	// is the fanciest version, although type unification is much more
	// difficult when using this.
	// XXX: type unification doesn't work perfectly here yet... maybe a bug with returned structs?
	GetFuncName = "get"

	// GetBoolFuncName is the name this function is registered as. This
	// variant can only pull in values of type bool.
	GetBoolFuncName = "get_bool"

	// GetStrFuncName is the name this function is registered as. This
	// variant can only pull in values of type str.
	GetStrFuncName = "get_str"

	// GetIntFuncName is the name this function is registered as. This
	// variant can only pull in values of type int.
	GetIntFuncName = "get_int"

	// GetFloatFuncName is the name this function is registered as. This
	// variant can only pull in values of type float.
	GetFloatFuncName = "get_float"

	// arg names...
	getArgNameKey = "key"
	// struct field names...
	getFieldNameValue = "value"
	getFieldNameReady = "ready"
)

func init() {
	funcs.ModuleRegister(ModuleName, GetFuncName, func() interfaces.Func { return &GetFunc{} })
	funcs.ModuleRegister(ModuleName, GetBoolFuncName, func() interfaces.Func { return &GetFunc{Type: types.TypeBool} })
	funcs.ModuleRegister(ModuleName, GetStrFuncName, func() interfaces.Func { return &GetFunc{Type: types.TypeStr} })
	funcs.ModuleRegister(ModuleName, GetIntFuncName, func() interfaces.Func { return &GetFunc{Type: types.TypeInt} })
	funcs.ModuleRegister(ModuleName, GetFloatFuncName, func() interfaces.Func { return &GetFunc{Type: types.TypeFloat} })
}

// GetFunc is special function which looks up the stored `Any` field in the
// value resource that it gets it from. If it is initialized with a fixed Type
// field, then it becomes a statically typed version that can only return keys
// of that type. It is instead recommended to use the Get* functions that are
// more strictly typed.
type GetFunc struct {
	// Type is the actual type being used for the value we are looking up.
	Type *types.Type

	init *interfaces.Init

	key string

	last   types.Value
	result types.Value // last calculated output

	watchChan chan struct{}
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *GetFunc) String() string {
	return GetFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *GetFunc) ArgGen(index int) (string, error) {
	seq := []string{getArgNameKey}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *GetFunc) sig() *types.Type {
	// func(key str) struct{value ?1; ready bool}
	typ := "?1"
	if obj.Type != nil {
		typ = obj.Type.String()
	}

	// output is a struct with two fields:
	// value is the zero value if not ready. A bool for that in other field.
	return types.NewType(fmt.Sprintf("func(%s str) struct{%s %s; %s bool}", getArgNameKey, getFieldNameValue, typ, getFieldNameReady))
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *GetFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}
	if len(typ.Ord) != 1 {
		return nil, fmt.Errorf("the function needs exactly one arg")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}

	tKey, exists := typ.Map[typ.Ord[0]]
	if !exists || tKey == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	if tKey.Kind != types.KindStr {
		return nil, fmt.Errorf("key must be str kind")
	}

	if typ.Out.Kind != types.KindStruct {
		return nil, fmt.Errorf("return must be kind struct")
	}
	if typ.Out.Map == nil {
		return nil, fmt.Errorf("invalid return type")
	}
	if len(typ.Out.Ord) != 2 {
		return nil, fmt.Errorf("invalid return type")
	}
	tValue, exists := typ.Out.Map[typ.Out.Ord[0]]
	if !exists || tValue == nil {
		return nil, fmt.Errorf("first struct field must be specified")
	}
	tReady, exists := typ.Out.Map[typ.Out.Ord[1]]
	if !exists || tReady == nil {
		return nil, fmt.Errorf("second struct field must be specified")
	}
	if tReady.Kind != types.KindBool {
		return nil, fmt.Errorf("second struct field must be bool kind")
	}

	obj.Type = tValue // type of our value
	return obj.sig(), nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *GetFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *GetFunc) Info() *interfaces.Info {
	var sig *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *GetFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.watchChan = make(chan struct{}) // sender closes this when Stream ends
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *GetFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // important so that we cleanup the watch when exiting
	for {
		select {
		// TODO: should this first chan be run as a priority channel to
		// avoid some sort of glitch? is that even possible? can our
		// hostname check with reality (below) fix that?
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back
				continue             // no more inputs, but don't return!
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			key := input.Struct()[getArgNameKey].Str()
			if key == "" {
				return fmt.Errorf("can't use an empty key")
			}
			if obj.init.Debug {
				obj.init.Logf("key: %s", key)
			}

			// We don't support changing the key over time, since it
			// might cause the type to need to be changed.
			if obj.key == "" {
				obj.key = key // store it
				var err error
				//  Don't send a value right away, wait for the
				// first ValueWatch startup event to get one!
				obj.watchChan, err = obj.init.Local.ValueWatch(ctx, obj.key) // watch for var changes
				if err != nil {
					return err
				}

			} else if obj.key != key {
				return fmt.Errorf("can't change key, previously: `%s`", obj.key)
			}

			continue // we get values on the watch chan, not here!

		case _, ok := <-obj.watchChan:
			if !ok { // closed
				return nil
			}
			//if err != nil {
			//	return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.key)
			//}

			result, err := obj.getValue(ctx) // get the value...
			if err != nil {
				return err
			}

			// if the result is still the same, skip sending an update...
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

// getValue gets the value we're looking for.
func (obj *GetFunc) getValue(ctx context.Context) (types.Value, error) {
	typ, exists := obj.Info().Sig.Out.Map[getFieldNameValue] // type of value field
	if !exists || typ == nil {
		// programming error
		return nil, fmt.Errorf("missing type for %s field", getFieldNameValue)
	}

	// The API will pull from the on-disk stored cache if present... This
	// value comes from the field in the Value resource... We only have an
	// on-disk cache because since functions load before resources do, we'd
	// like to warm the cache with the right value before the resource can
	// issue a new one to our in-memory store. This avoids a re-provisioning
	// step that might be needed if the value started out empty...
	// TODO: We could even add a stored: bool field in the returned struct!
	isReady := true // assume true
	val, err := obj.init.Local.ValueGet(ctx, obj.key)
	if err != nil {
		return nil, errwrap.Wrapf(err, "channel read failed on `%s`", obj.key)
	}
	if val == nil { // val doesn't exist
		isReady = false
	}

	ready := &types.BoolValue{V: isReady}
	value := typ.New() // new zero value of that typ
	if isReady {
		value, err = types.ValueOfGolang(val) // interface{} -> types.Value
		if err != nil {
			// programming error
			return nil, errwrap.Wrapf(err, "invalid value")
		}
		if err := value.Type().Cmp(typ); err != nil {
			// XXX: when we run get_int, but the resource value is
			// an str for example, this error happens... Do we want
			// to: (1) coerce? -- no; (2) error? -- yep for now; (3)
			// improve type unification? -- if it's possible, yes.
			return nil, errwrap.Wrapf(err, "type mismatch, check type in Value[%s]", obj.key)
		}
	}

	st := types.NewStruct(obj.Info().Sig.Out)
	if err := st.Set(getFieldNameValue, value); err != nil {
		return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", getFieldNameValue, value)
	}
	if err := st.Set(getFieldNameReady, ready); err != nil {
		return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", getFieldNameReady, ready)
	}

	return st, nil // put struct into interface type
}
