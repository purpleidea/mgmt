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

package resources

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("kv", func() engine.Res { return &KVRes{} })
}

// KVResSkipCmpStyle represents the different styles of comparison when using
// SkipLessThan.
type KVResSkipCmpStyle int

// These are the different allowed comparison styles. Most folks will want
// SkipCmpStyleInt.
const (
	SkipCmpStyleInt KVResSkipCmpStyle = iota
	SkipCmpStyleString
)

const (
	kvCheckApplyTimeout = 5 * time.Second
)

// KVRes is a resource which writes a key/value pair into cluster wide storage.
// It will ensure that the key is set to the requested value. The one exception
// is that if you use the SkipLessThan parameter, then it will only replace the
// stored value with the requested value if it is greater than that stored one.
// This allows the KV resource to be used in fast acting, finite state machines
// which have monotonically increasing state values that represent progression.
// The one exception is that when this resource receives a refresh signal, then
// it will set the value to be the exact one if they are not identical already.
type KVRes struct {
	traits.Base // add the base methods without re-implementation
	//traits.Groupable // TODO: it could be useful to group our writes and watches!
	traits.Refreshable
	traits.Recvable

	init *engine.Init

	// Key represents the key to set. If it is not specified, the Name value
	// is used instead.
	Key string `lang:"key" yaml:"key"`

	// Value represents the string value to set. If this value is nil or,
	// undefined, then this will delete that key.
	Value *string `lang:"value" yaml:"value"`

	// Mapped specifies that we will store the value in a map with each
	// hostname as part of the key. This is very useful for exchanging
	// values when running this resource on multiple nodes simultaneously.
	// To read/write/watch a single, global key, this value should be false.
	// Note that resources may fight if more than one uses this. The `world`
	// functions like `exchange`, require this to be true, since they're
	// pulling values out of a pool that each node sets. The `world`
	// functions like `getval`, require this to be false, since they're
	// pulling values directly out of the same namespace that is shared by
	// all nodes.
	Mapped bool `lang:"mapped" yaml:"mapped"`

	// SkipLessThan causes the value to be updated as long as it is greater.
	SkipLessThan bool `lang:"skiplessthan" yaml:"skiplessthan"`

	// SkipCmpStyle is the type of compare function used when determining if
	// the value is greater when using the SkipLessThan parameter.
	SkipCmpStyle KVResSkipCmpStyle `lang:"skipcmpstyle" yaml:"skipcmpstyle"`

	interruptChan chan struct{}

	// TODO: does it make sense to have different backends here? (eg: local)
}

// getKey returns the key to be used for this resource. If the Key field is
// specified, it will use that, otherwise it uses the Name.
func (obj *KVRes) getKey() string {
	if obj.Key != "" {
		return obj.Key
	}
	return obj.Name()
}

func (obj *KVRes) kvWatch(ctx context.Context, key string) (chan error, error) {
	if obj.Mapped {
		return obj.init.World.StrMapWatch(ctx, key)
	}
	return obj.init.World.StrWatch(ctx, key)
}

func (obj *KVRes) kvGet(ctx context.Context, key string) (string, bool, error) {
	if obj.Mapped {
		hostname := obj.init.Hostname // me
		keyMap, err := obj.init.World.StrMapGet(ctx, obj.getKey())
		if err != nil {
			return "", false, err
		}
		val, exists := keyMap[hostname]
		return val, exists, nil
	}

	val, err := obj.init.World.StrGet(ctx, key)
	if err != nil && obj.init.World.StrIsNotExist(err) {
		return "", false, nil // val doesn't exist
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (obj *KVRes) kvSet(ctx context.Context, key, val string) error {
	if obj.Mapped {
		return obj.init.World.StrMapSet(ctx, key, val)
	}
	return obj.init.World.StrSet(ctx, key, val)
}

func (obj *KVRes) kvDel(ctx context.Context, key string) error {
	if obj.Mapped {
		return obj.init.World.StrMapDel(ctx, key)
	}
	return obj.init.World.StrDel(ctx, key)
}

// Default returns some sensible defaults for this resource.
func (obj *KVRes) Default() engine.Res {
	return &KVRes{}
}

// Validate if the params passed in are valid data.
func (obj *KVRes) Validate() error {
	if obj.getKey() == "" {
		return fmt.Errorf("key must not be empty")
	}
	if obj.SkipLessThan {
		if obj.SkipCmpStyle != SkipCmpStyleInt && obj.SkipCmpStyle != SkipCmpStyleString {
			return fmt.Errorf("the SkipCmpStyle of %v is invalid", obj.SkipCmpStyle)
		}

		if v := obj.Value; obj.SkipCmpStyle == SkipCmpStyleInt && v != nil {
			if _, err := strconv.Atoi(*v); err != nil {
				return fmt.Errorf("the set value of %v can't convert to int", v)
			}
		}
	}
	return nil
}

// Init initializes the resource.
func (obj *KVRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.interruptChan = make(chan struct{})

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *KVRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *KVRes) Watch(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := obj.kvWatch(ctx, obj.getKey()) // get possible events!
	if err != nil {
		return errwrap.Wrapf(err, "error during watch")
	}

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case err, ok := <-ch:
			if !ok { // channel shutdown
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.init.Debug {
				obj.init.Logf("event!")
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// lessThanCheck checks for less than validity.
func (obj *KVRes) lessThanCheck(value string) (bool, error) {
	v := *obj.Value
	if value == v { // redundant check for safety
		return true, nil
	}

	var refresh = obj.init.Refresh()  // do we have a pending reload to apply?
	if !obj.SkipLessThan || refresh { // update lessthan on refresh
		return false, nil
	}

	switch obj.SkipCmpStyle {
	case SkipCmpStyleInt:
		intValue, err := strconv.Atoi(value)
		if err != nil {
			// NOTE: We don't error here since we're going to write
			// over the value anyways. It could be from an old run!
			return false, nil // value is bad (old/corrupt), fix it
		}
		if vint, err := strconv.Atoi(v); err != nil {
			return false, errwrap.Wrapf(err, "can't convert %v to int", v)
		} else if vint < intValue {
			return true, nil
		}

	case SkipCmpStyleString:
		if v < value { // weird way to cmp, but valid
			return true, nil
		}

	default:
		return false, fmt.Errorf("unmatches SkipCmpStyle style %v", obj.SkipCmpStyle)
	}

	return false, nil
}

// CheckApply method for Password resource. Does nothing, returns happy!
func (obj *KVRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	wg := &sync.WaitGroup{}
	defer wg.Wait() // this must be above the defer cancel() call
	ctx, cancel := context.WithTimeout(ctx, kvCheckApplyTimeout)
	defer cancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-obj.interruptChan:
			cancel()
		case <-ctx.Done():
			// let this exit
		}
	}()

	if val, exists := obj.init.Recv()["value"]; exists && val.Changed {
		// if we received on Value, and it changed, wooo, nothing to do.
		if obj.Value == nil {
			obj.init.Logf("nil `value` was received!")
		} else {
			obj.init.Logf("`value` (%s) was received!", *obj.Value)
		}
	}

	value, exists, err := obj.kvGet(ctx, obj.getKey())
	if err != nil {
		return false, errwrap.Wrapf(err, "error during kv get")
	}
	if exists && obj.Value != nil {
		if value == *obj.Value {
			return true, nil
		}

		if c, err := obj.lessThanCheck(value); err != nil {
			return false, err
		} else if c {
			return true, nil
		}

	} else if !exists && obj.Value == nil {
		return true, nil // nothing to delete, we're good!

	} else if exists && obj.Value == nil { // delete
		if !apply {
			return false, nil
		}
		if err := obj.kvDel(ctx, obj.getKey()); err != nil {
			return false, errwrap.Wrapf(err, "error during del")
		}
		obj.init.Logf("`value` was deleted!")
		return false, nil
	}

	if !apply {
		return false, nil
	}

	if err := obj.kvSet(ctx, obj.getKey(), *obj.Value); err != nil {
		return false, errwrap.Wrapf(err, "error during set")
	}
	obj.init.Logf("`value` was changed!")

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *KVRes) Cmp(r engine.Res) error {
	// we can only compare KVRes to others of the same resource kind
	res, ok := r.(*KVRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.getKey() != res.getKey() {
		return fmt.Errorf("the Key differs")
	}
	if (obj.Value == nil) != (res.Value == nil) { // xor
		return fmt.Errorf("the Value differs")
	}
	if obj.Value != nil && res.Value != nil {
		if *obj.Value != *res.Value { // compare the strings
			return fmt.Errorf("the contents of Value differs")
		}
	}
	if obj.Mapped != res.Mapped {
		return fmt.Errorf("the Mapped param differs")
	}
	if obj.SkipLessThan != res.SkipLessThan {
		return fmt.Errorf("the SkipLessThan param differs")
	}
	if obj.SkipCmpStyle != res.SkipCmpStyle {
		return fmt.Errorf("the SkipCmpStyle param differs")
	}

	return nil
}

// Interrupt is called to ask the execution of this resource to end early.
func (obj *KVRes) Interrupt() error {
	close(obj.interruptChan)
	return nil
}

// KVUID is the UID struct for KVRes.
type KVUID struct {
	engine.BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *KVRes) UIDs() []engine.ResUID {
	x := &KVUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *KVRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes KVRes // indirection to avoid infinite recursion

	def := obj.Default()    // get the default
	res, ok := def.(*KVRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to KVRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = KVRes(raw) // restore from indirection with type conversion!
	return nil
}
