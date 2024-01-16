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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package coreworld

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// GetValFuncName is the name this function is registered as.
	GetValFuncName = "getval"

	// arg names...
	getValArgNameKey = "key"

	// struct field names...
	getValFieldNameValue  = "value"
	getValFieldNameExists = "exists"
)

func init() {
	funcs.ModuleRegister(ModuleName, GetValFuncName, func() interfaces.Func { return &GetValFunc{} })
}

// GetValFunc is special function which returns the value of a given key in the
// exposed world.
type GetValFunc struct {
	init *interfaces.Init

	key string

	last   types.Value
	result types.Value // last calculated output

	watchChan chan error
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *GetValFunc) String() string {
	return GetValFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *GetValFunc) ArgGen(index int) (string, error) {
	seq := []string{getValArgNameKey}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *GetValFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *GetValFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		// output is a struct with two fields:
		// value is the zero value if not exists. A bool for that in other field.
		Sig: types.NewType(fmt.Sprintf("func(%s str) struct{%s str; %s bool}", getValArgNameKey, getValFieldNameValue, getValFieldNameExists)),
		Err: obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *GetValFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.watchChan = make(chan error) // XXX: sender should close this, but did I implement that part yet???
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *GetValFunc) Stream(ctx context.Context) error {
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

			key := input.Struct()[getValArgNameKey].Str()
			if key == "" {
				return fmt.Errorf("can't use an empty key")
			}
			if obj.init.Debug {
				obj.init.Logf("key: %s", key)
			}

			// TODO: support changing the key over time...
			if obj.key == "" {
				obj.key = key // store it
				var err error
				//  Don't send a value right away, wait for the
				// first ValueWatch startup event to get one!
				obj.watchChan, err = obj.init.World.StrWatch(ctx, obj.key) // watch for var changes
				if err != nil {
					return err
				}

			} else if obj.key != key {
				return fmt.Errorf("can't change key, previously: `%s`", obj.key)
			}

			continue // we get values on the watch chan, not here!

		case err, ok := <-obj.watchChan:
			if !ok { // closed
				// XXX: if we close, perhaps the engine is
				// switching etcd hosts and we should retry?
				// maybe instead we should get an "etcd
				// reconnect" signal, and the lang will restart?
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.key)
			}

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
func (obj *GetValFunc) getValue(ctx context.Context) (types.Value, error) {
	exists := true // assume true
	val, err := obj.init.World.StrGet(ctx, obj.key)
	if err != nil && obj.init.World.StrIsNotExist(err) {
		exists = false // val doesn't exist
	} else if err != nil {
		return nil, errwrap.Wrapf(err, "channel read failed on `%s`", obj.key)
	}

	s := &types.StrValue{V: val}
	b := &types.BoolValue{V: exists}
	st := types.NewStruct(obj.Info().Sig.Out)
	if err := st.Set(getValFieldNameValue, s); err != nil {
		return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", getValFieldNameValue, s)
	}
	if err := st.Set(getValFieldNameExists, b); err != nil {
		return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", getValFieldNameExists, b)
	}

	return st, nil // put struct into interface type
}
