// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"encoding/gob"
	"fmt"
	"log"
	"strconv"

	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("kv", func() Res { return &KVRes{} })
	gob.Register(&KVRes{})
}

// KVResSkipCmpStyle represents the different styles of comparison when using SkipLessThan.
type KVResSkipCmpStyle int

// These are the different allowed comparison styles. Most folks will want SkipCmpStyleInt.
const (
	SkipCmpStyleInt KVResSkipCmpStyle = iota
	SkipCmpStyleString
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
	BaseRes      `yaml:",inline"`
	Key          string            `yaml:"key"`          // key to set
	Value        *string           `yaml:"value"`        // value to set (nil to delete)
	SkipLessThan bool              `yaml:"skiplessthan"` // skip updates as long as stored value is greater
	SkipCmpStyle KVResSkipCmpStyle `yaml:"skipcmpstyle"` // how to do the less than cmp
	// TODO: does it make sense to have different backends here? (eg: local)
}

// Default returns some sensible defaults for this resource.
func (obj *KVRes) Default() Res {
	return &KVRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
// FIXME: This will catch most issues unless data is passed in after Init with
// the Send/Recv mechanism. Should the engine re-call Validate after Send/Recv?
func (obj *KVRes) Validate() error {
	if obj.Key == "" {
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
	return obj.BaseRes.Validate()
}

// Init initializes the resource.
func (obj *KVRes) Init() error {
	obj.BaseRes.Kind = "kv"
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *KVRes) Watch() error {

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	ch := obj.Data().World.StrMapWatch(obj.Key) // get possible events!

	var send = false // send event?
	var exit *error
	for {
		select {
		// NOTE: this part is very similar to the file resource code
		case err, ok := <-ch:
			if !ok { // channel shutdown
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			if obj.Data().Debug {
				log.Printf("%s: Event!", obj)
			}
			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

// lessThanCheck checks for less than validity.
func (obj *KVRes) lessThanCheck(value string) (checkOK bool, err error) {

	v := *obj.Value
	if value == v { // redundant check for safety
		return true, nil
	}

	var refresh = obj.Refresh()       // do we have a pending reload to apply?
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
func (obj *KVRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: CheckApply(%t)", obj, apply)

	if val, exists := obj.Recv["Value"]; exists && val.Changed {
		// if we received on Value, and it changed, wooo, nothing to do.
		log.Printf("CheckApply: `Value` was updated!")
	}

	hostname := obj.Data().Hostname // me
	keyMap, err := obj.Data().World.StrMapGet(obj.Key)
	if err != nil {
		return false, errwrap.Wrapf(err, "check error during StrGet")
	}

	if value, ok := keyMap[hostname]; ok && obj.Value != nil {
		if value == *obj.Value {
			return true, nil
		}

		if c, err := obj.lessThanCheck(value); err != nil {
			return false, err
		} else if c {
			return true, nil
		}

	} else if !ok && obj.Value == nil {
		return true, nil // nothing to delete, we're good!

	} else if ok && obj.Value == nil { // delete
		err := obj.Data().World.StrMapDel(obj.Key)
		return false, errwrap.Wrapf(err, "apply error during StrDel")
	}

	if !apply {
		return false, nil
	}

	if err := obj.Data().World.StrMapSet(obj.Key, *obj.Value); err != nil {
		return false, errwrap.Wrapf(err, "apply error during StrSet")
	}

	return false, nil
}

// KVUID is the UID struct for KVRes.
type KVUID struct {
	BaseUID
	name string
}

// AutoEdges returns the AutoEdge interface. In this case no autoedges are used.
func (obj *KVRes) AutoEdges() AutoEdge {
	return nil
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *KVRes) UIDs() []ResUID {
	x := &KVUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *KVRes) GroupCmp(r Res) bool {
	_, ok := r.(*KVRes)
	if !ok {
		return false
	}
	return false // TODO: this is doable!
	// TODO: it could be useful to group our writes and watches!
}

// Compare two resources and return if they are equivalent.
func (obj *KVRes) Compare(r Res) bool {
	// we can only compare KVRes to others of the same resource kind
	res, ok := r.(*KVRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}

	if obj.Key != res.Key {
		return false
	}
	if (obj.Value == nil) != (res.Value == nil) { // xor
		return false
	}
	if obj.Value != nil && res.Value != nil {
		if *obj.Value != *res.Value { // compare the strings
			return false
		}
	}
	if obj.SkipLessThan != res.SkipLessThan {
		return false
	}
	if obj.SkipCmpStyle != res.SkipCmpStyle {
		return false
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
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
