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
	// KVLookupFuncName is the name this function is registered as.
	KVLookupFuncName = "kvlookup"

	// arg names...
	kvLookupArgNameNamespace = "namespace"
)

func init() {
	funcs.ModuleRegister(ModuleName, KVLookupFuncName, func() interfaces.Func { return &KVLookupFunc{} })
}

var _ interfaces.CallableFunc = &KVLookupFunc{}

// KVLookupFunc is special function which returns all the values of a given key
// in the exposed world. It is similar to exchange, but it does not set a key.
type KVLookupFunc struct {
	init *interfaces.Init

	namespace string
	args      []types.Value

	last   types.Value
	result types.Value // last calculated output

	watchChan chan error
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *KVLookupFunc) String() string {
	return KVLookupFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *KVLookupFunc) ArgGen(index int) (string, error) {
	seq := []string{kvLookupArgNameNamespace}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *KVLookupFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *KVLookupFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Fast: false,
		Spec: false,
		// output is map of: hostname => value
		Sig: types.NewType(fmt.Sprintf("func(%s str) map{str: str}", kvLookupArgNameNamespace)),
		Err: obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *KVLookupFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.watchChan = make(chan error) // XXX: sender should close this, but did I implement that part yet???
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *KVLookupFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
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

			args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			if err != nil {
				return err
			}
			obj.args = args

			namespace := args[0].Str()
			if namespace == "" {
				return fmt.Errorf("can't use an empty namespace")
			}
			if obj.init.Debug {
				obj.init.Logf("namespace: %s", namespace)
			}

			// TODO: support changing the namespace over time...
			// TODO: possibly removing our stored value there first!
			if obj.namespace == "" {
				obj.namespace = namespace // store it
				var err error
				obj.watchChan, err = obj.init.World.StrMapWatch(ctx, obj.namespace) // watch for var changes
				if err != nil {
					return err
				}

				result, err := obj.Call(ctx, obj.args) // build the map...
				if err != nil {
					return err
				}
				select {
				case obj.init.Output <- result: // send one!
					// pass
				case <-ctx.Done():
					return nil
				}

			} else if obj.namespace != namespace {
				return fmt.Errorf("can't change namespace, previously: `%s`", obj.namespace)
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
				return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.namespace)
			}

			result, err := obj.Call(ctx, obj.args) // build the map...
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

// Call this function with the input args and return the value if it is possible
// to do so at this time. This was previously buildMap, which builds the result
// map which we'll need. It uses struct variables.
func (obj *KVLookupFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	namespace := args[0].Str()
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}
	keyMap, err := obj.init.World.StrMapGet(ctx, namespace)
	if err != nil {
		return nil, errwrap.Wrapf(err, "channel read failed on `%s`", namespace)
	}

	d := types.NewMap(obj.Info().Sig.Out)
	for k, v := range keyMap {
		key := &types.StrValue{V: k}
		val := &types.StrValue{V: v}
		if err := d.Add(key, val); err != nil {
			return nil, errwrap.Wrapf(err, "map could not add key `%s`, val: `%s`", k, v)
		}
	}
	return d, nil // put map into interface type
}
