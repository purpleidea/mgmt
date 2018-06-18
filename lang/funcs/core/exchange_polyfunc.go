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

package core // TODO: should this be in its own individual package?

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"

	errwrap "github.com/pkg/errors"
)

func init() {
	funcs.Register("exchange", func() interfaces.Func { return &ExchangeFunc{} }) // must register the func and name
}

// ExchangeFunc is special function which returns all the values of a given key
// in the exposed world, and sets it's own.
type ExchangeFunc struct {
	init *interfaces.Init

	namespace string
	value     string

	last   types.Value
	result types.Value // last calculated output

	watchChan chan error
	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ExchangeFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ExchangeFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		// TODO: do we want to allow this to be statically polymorphic,
		// and have value be any type we might want?
		// output is map of: hostname => value
		Sig: types.NewType("func(namespace str, value str) map{str: str}"),
		Err: obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ExchangeFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.watchChan = make(chan error) // XXX: sender should close this, but did I implement that part yet???
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ExchangeFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
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

			namespace := input.Struct()["namespace"].Str()
			if namespace == "" {
				return fmt.Errorf("can't use an empty namespace")
			}
			if obj.init.Debug {
				obj.init.Logf("namespace: %s", namespace)
			}

			// TODO: support changing the namespace over time...
			// TODO: possibly removing our stored value there first!
			if obj.namespace == "" {
				obj.namespace = namespace                                 // store it
				obj.watchChan = obj.init.World.StrMapWatch(obj.namespace) // watch for var changes
			} else if obj.namespace != namespace {
				return fmt.Errorf("can't change namespace, previously: `%s`", obj.namespace)
			}

			value := input.Struct()["value"].Str()
			if obj.init.Debug {
				obj.init.Logf("value: %+v", value)
			}

			if err := obj.init.World.StrMapSet(obj.namespace, value); err != nil {
				return errwrap.Wrapf(err, "namespace write error of `%s` to `%s`", value, obj.namespace)
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

			keyMap, err := obj.init.World.StrMapGet(obj.namespace)
			if err != nil {
				return errwrap.Wrapf(err, "channel read failed on `%s`", obj.namespace)
			}

			var result types.Value

			d := types.NewMap(obj.Info().Sig.Out)
			for k, v := range keyMap {
				key := &types.StrValue{V: k}
				val := &types.StrValue{V: v}
				if err := d.Add(key, val); err != nil {
					return errwrap.Wrapf(err, "map could not add key `%s`, val: `%s`", k, v)
				}
			}
			result = d // put map into interface type

			// if the result is still the same, skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *ExchangeFunc) Close() error {
	close(obj.closeChan)
	return nil
}
