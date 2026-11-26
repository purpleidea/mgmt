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

//go:build !root

package lang

import (
	"context"
	"errors"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	etcdInterfaces "github.com/purpleidea/mgmt/etcd/interfaces"
)

// testStrWorld wraps a World, and keeps the str keys in memory instead, so that
// the functions which use them can run in tests which have no etcd. Only the
// StrWatch, StrIsNotExist, StrGet, StrSet and StrDel functions are replaced,
// and everything else, including StrEdit and the StrMap functions, still goes
// to the wrapped World.
type testStrWorld struct {
	engine.World

	mutex   *sync.Mutex
	values  map[string]string
	watches map[chan struct{}]string // one chan for each watch, and its key
}

// newTestStrWorld wraps the World, and starts it with a copy of these values.
func newTestStrWorld(world engine.World, values map[string]string) *testStrWorld {
	obj := &testStrWorld{
		World: world,

		mutex:   &sync.Mutex{},
		values:  make(map[string]string),
		watches: make(map[chan struct{}]string),
	}
	for k, v := range values {
		obj.values[k] = v
	}
	return obj
}

// StrWatch returns a channel which sends one startup event, and then one event
// for every change to the key, like the etcd watch does. It closes the channel
// once the context is cancelled.
func (obj *testStrWorld) StrWatch(ctx context.Context, namespace string) (chan error, error) {
	notify := make(chan struct{}, 1) // buffered, so that set never blocks
	notify <- struct{}{}             // startup event

	obj.mutex.Lock()
	obj.watches[notify] = namespace
	obj.mutex.Unlock()

	ch := make(chan error)
	go func() {
		defer close(ch)
		defer func() {
			obj.mutex.Lock()
			delete(obj.watches, notify)
			obj.mutex.Unlock()
		}()
		for {
			select {
			case <-notify:
			case <-ctx.Done():
				return
			}

			select {
			case ch <- nil: // an event, and not an error
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// StrIsNotExist returns whether the error from StrGet is a key missing error.
func (obj *testStrWorld) StrIsNotExist(err error) bool {
	return errors.Is(err, etcdInterfaces.ErrNotExist)
}

// StrGet returns the value for the key, or the same error as etcd does if the
// key doesn't exist.
func (obj *testStrWorld) StrGet(ctx context.Context, namespace string) (string, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	value, exists := obj.values[namespace]
	if !exists {
		return "", etcdInterfaces.ErrNotExist
	}
	return value, nil
}

// StrSet sets the key to the value.
func (obj *testStrWorld) StrSet(ctx context.Context, namespace, value string) error {
	obj.set(namespace, &value)
	return nil
}

// StrDel deletes the key.
func (obj *testStrWorld) StrDel(ctx context.Context, namespace string) error {
	obj.set(namespace, nil)
	return nil
}

// set sets the key to the value, or deletes it if the value is nil, and then
// notifies every watch of that key.
func (obj *testStrWorld) set(namespace string, value *string) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if value == nil {
		delete(obj.values, namespace)
	} else {
		obj.values[namespace] = *value
	}

	for ch, key := range obj.watches {
		if key != namespace {
			continue
		}
		select {
		case ch <- struct{}{}:
		default:
			// An event is already pending, which coalesces with this.
		}
	}
}
