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
	"net/url"
	"strconv"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources/http_ui/common"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	httpUIInputKind = httpUIKind + ":input"

	httpUIInputStoreKey         = "key"
	httpUIInputStoreSchemeLocal = "local"
	httpUIInputStoreSchemeWorld = "world"

	httpUIInputTypeText  = common.HTTPUIInputTypeText  // "text"
	httpUIInputTypeRange = common.HTTPUIInputTypeRange // "range"
)

func init() {
	engine.RegisterResource(httpUIInputKind, func() engine.Res { return &HTTPUIInputRes{} })
}

// HTTPUIInputRes is a form element that exists within a http:ui resource, which
// exists within an http server. The name is used as the unique id of the field,
// unless the id field is specified, and in that case it is used instead. The
// way this works is that it autogroups at runtime with an existing http:ui
// resource, and in doing so makes the form field associated with this resource
// available as part of that ui which is itself grouped and served from the http
// server resource.
type HTTPUIInputRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPUIRes
	traits.Sendable

	init *engine.Init

	// Path is the name of the http ui resource to group this into. If it is
	// omitted, and there is only a single http ui resource, then it will
	// be grouped into it automatically. If there is more than one main http
	// ui resource being used, then the grouping behaviour is *undefined*
	// when this is not specified, and it is not recommended to leave this
	// blank!
	Path string `lang:"path" yaml:"path"`

	// ID is the unique id for this element. It is used in form fields and
	// should not be a private identifier. It must be unique within a given
	// http ui.
	ID string `lang:"id" yaml:"id"`

	// Value is the default value to use for the form field. If you change
	// it, then the resource graph will change and we'll rebuild and have
	// the new value visible. You can use either this or the Store field.
	// XXX: If we ever add our resource mutate API, we might not need to
	// swap to a new resource graph, and maybe Store is not needed?
	Value string `lang:"value" yaml:"value"`

	// Store the data in this source. It will also read in a default value
	// from there if one is present. It will watch it for changes as well,
	// and update the displayed value if it's changed from another source.
	// This cannot be used at the same time as the Value field.
	Store string `lang:"store" yaml:"store"`

	// Type specifies the type of input field this is, and some information
	// about it.
	// XXX: come up with a format such as "multiline://?max=60&style=foo"
	Type string `lang:"type" yaml:"type"`

	// Sort is a string that you can use to determine the global sorted
	// display order of all the elements in a ui.
	Sort string `lang:"sort" yaml:"sort"`

	scheme        string        // the scheme we're using with Store, cached for later
	key           string        // the key we're using with Store, cached for later
	typeURL       *url.URL      // the type data, cached for later
	typeURLValues url.Values    // the type data, cached for later
	last          *string       // the last value we sent
	value         string        // what we've last received from SetValue
	storeEvent    bool          // did a store event happen?
	mutex         *sync.Mutex   // guards storeEvent and value
	event         chan struct{} // local event that the setValue sends
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPUIInputRes) Default() engine.Res {
	return &HTTPUIInputRes{
		Type: "text://",
	}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPUIInputRes) Validate() error {
	if obj.GetID() == "" {
		return fmt.Errorf("empty id")
	}

	if obj.Value != "" && obj.Store != "" {
		return fmt.Errorf("may only use either Value or Store")
	}

	if obj.Value != "" {
		if err := obj.checkValue(obj.Value); err != nil {
			return errwrap.Wrapf(err, "the Value field is invalid")
		}
	}

	if obj.Store != "" {
		// XXX: check the URI format
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPUIInputRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	u, err := url.Parse(obj.Type)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("can't parse Type")
	}
	if u.Scheme != httpUIInputTypeText && u.Scheme != httpUIInputTypeRange {
		return fmt.Errorf("unknown scheme: %s", u.Scheme)
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return err
	}
	obj.typeURL = u
	obj.typeURLValues = values

	if obj.Store != "" {
		u, err := url.Parse(obj.Store)
		if err != nil {
			return err
		}
		if u == nil {
			return fmt.Errorf("can't parse Store")
		}
		if u.Scheme != httpUIInputStoreSchemeLocal && u.Scheme != httpUIInputStoreSchemeWorld {
			return fmt.Errorf("unknown scheme: %s", u.Scheme)
		}
		values, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return err
		}

		obj.scheme = u.Scheme // cache for later
		obj.key = obj.Name()  // default

		x, exists := values[httpUIInputStoreKey]
		if exists && len(x) > 0 && x[0] != "" { // ignore absent or broken keys
			obj.key = x[0]
		}
	}

	// populate our obj.value cache somehow, so we don't mutate obj.Value
	obj.value = obj.Value // copy
	obj.mutex = &sync.Mutex{}
	obj.event = make(chan struct{}, 1) // buffer to avoid blocks or deadlock

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPUIInputRes) Cleanup() error {
	return nil
}

// getKey returns the key to be used for this resource. If the Store field is
// specified, it will use that parsed part, otherwise it uses the Name.
func (obj *HTTPUIInputRes) getKey() string {
	if obj.Store != "" {
		return obj.key
	}

	return obj.Name()
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPUIInputRes) ParentName() string {
	return obj.Path
}

// GetKind returns the kind of this resource.
func (obj *HTTPUIInputRes) GetKind() string {
	// NOTE: We don't *need* to return such a specific string, and "input"
	// would be enough, but we might as well use this because we have it.
	return httpUIInputKind
}

// GetID returns the actual ID we respond to. When ID is not specified, we use
// the Name.
func (obj *HTTPUIInputRes) GetID() string {
	if obj.ID != "" {
		return obj.ID
	}
	return obj.Name()
}

// SetValue stores the new value field that was obtained from submitting the
// form. This receives the raw, unsafe value that you must validate first.
func (obj *HTTPUIInputRes) SetValue(ctx context.Context, vs []string) error {
	if len(vs) != 1 {
		return fmt.Errorf("unexpected length of %d", len(vs))
	}
	value := vs[0]

	if err := obj.checkValue(value); err != nil {
		return err
	}

	obj.mutex.Lock()
	obj.setValue(ctx, value) // also sends an event
	obj.mutex.Unlock()
	return nil
}

// setValue is the helper version where the caller must provide the mutex.
func (obj *HTTPUIInputRes) setValue(ctx context.Context, val string) error {
	obj.value = val

	select {
	case obj.event <- struct{}{}:
	default:
	}

	return nil
}

func (obj *HTTPUIInputRes) checkValue(value string) error {
	// XXX: validate based on obj.Type
	// XXX: validate what kind of values are allowed, probably no \n, etc...
	return nil
}

// GetValue gets a string representation for the form value, that we'll use in
// our html form.
func (obj *HTTPUIInputRes) GetValue(ctx context.Context) (string, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.storeEvent {
		val, exists, err := obj.storeGet(ctx, obj.getKey())
		if err != nil {
			return "", errwrap.Wrapf(err, "error during get")
		}
		if !exists {
			return "", nil // default
		}
		return val, nil
	}

	return obj.value, nil
}

// GetType returns a map that you can use to build the input field in the ui.
func (obj *HTTPUIInputRes) GetType() map[string]string {
	m := make(map[string]string)

	if obj.typeURL.Scheme == httpUIInputTypeRange {
		m = obj.rangeGetType()
	}

	m[common.HTTPUIInputType] = obj.typeURL.Scheme

	return m
}

func (obj *HTTPUIInputRes) rangeGetType() map[string]string {
	m := make(map[string]string)
	base := 10
	bits := 64

	if sa, exists := obj.typeURLValues[common.HTTPUIInputTypeRangeMin]; exists && len(sa) > 0 {
		if x, err := strconv.ParseInt(sa[0], base, bits); err == nil {
			m[common.HTTPUIInputTypeRangeMin] = strconv.FormatInt(x, base)
		}
	}
	if sa, exists := obj.typeURLValues[common.HTTPUIInputTypeRangeMax]; exists && len(sa) > 0 {
		if x, err := strconv.ParseInt(sa[0], base, bits); err == nil {
			m[common.HTTPUIInputTypeRangeMax] = strconv.FormatInt(x, base)
		}
	}
	if sa, exists := obj.typeURLValues[common.HTTPUIInputTypeRangeStep]; exists && len(sa) > 0 {
		if x, err := strconv.ParseInt(sa[0], base, bits); err == nil {
			m[common.HTTPUIInputTypeRangeStep] = strconv.FormatInt(x, base)
		}
	}

	return m
}

// GetSort returns a string that you can use to determine the global sorted
// display order of all the elements in a ui.
func (obj *HTTPUIInputRes) GetSort() string {
	return obj.Sort
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPUIInputRes) Watch(ctx context.Context) error {
	if obj.Store != "" && obj.scheme == httpUIInputStoreSchemeLocal {
		return obj.localWatch(ctx)
	}
	if obj.Store != "" && obj.scheme == httpUIInputStoreSchemeWorld {
		return obj.worldWatch(ctx)
	}

	obj.init.Running() // when started, notify engine that we're running

	// XXX: do we need to watch on obj.event for normal .Value stuff?

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

func (obj *HTTPUIInputRes) localWatch(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := obj.init.Local.ValueWatch(ctx, obj.getKey()) // get possible events!
	if err != nil {
		return errwrap.Wrapf(err, "error during watch")
	}

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case _, ok := <-ch:
			if !ok { // channel shutdown
				return nil
			}
			obj.mutex.Lock()
			obj.storeEvent = true
			obj.mutex.Unlock()

		case <-obj.event:

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if obj.init.Debug {
			obj.init.Logf("event!")
		}
		obj.init.Event() // notify engine of an event (this can block)
	}

}

func (obj *HTTPUIInputRes) worldWatch(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch, err := obj.init.World.StrWatch(ctx, obj.getKey()) // get possible events!
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
			obj.mutex.Lock()
			obj.storeEvent = true
			obj.mutex.Unlock()

		case <-obj.event:

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if obj.init.Debug {
			obj.init.Logf("event!")
		}
		obj.init.Event() // notify engine of an event (this can block)
	}

}

// CheckApply performs the send/recv portion of this autogrouped resources. That
// can fail, but only if the send portion fails for some reason. If we're using
// the Store feature, then it also reads and writes to and from that store.
func (obj *HTTPUIInputRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	// If we're in ".Value" mode, we want to look at the incoming value, and
	// send it onwards. This function mostly exists as a stub in this case.
	// The private value gets set by obj.SetValue from the http:ui parent.
	// If we're in ".Store" mode, then we're reconciling between the "World"
	// and the http:ui "Web".

	if obj.Store != "" {
		return obj.storeCheckApply(ctx, apply)
	}

	return obj.valueCheckApply(ctx, apply)

}

func (obj *HTTPUIInputRes) valueCheckApply(ctx context.Context, apply bool) (bool, error) {

	obj.mutex.Lock()
	value := obj.value // gets set by obj.SetValue
	obj.mutex.Unlock()

	if obj.last != nil && *obj.last == value {
		if err := obj.init.Send(&HTTPUIInputSends{
			Value: &value,
		}); err != nil {
			return false, err
		}
		return true, nil // expected value has already been sent
	}

	if !apply {
		if err := obj.init.Send(&HTTPUIInputSends{
			Value: &value, // XXX: arbitrary since we're in noop mode
		}); err != nil {
			return false, err
		}
		return false, nil
	}

	s := value    // copy
	obj.last = &s // cache

	// XXX: This is getting called twice, what's the bug?
	obj.init.Logf("sending: %s", value)

	// send
	if err := obj.init.Send(&HTTPUIInputSends{
		Value: &value,
	}); err != nil {
		return false, err
	}

	return false, nil
	//return true, nil // always succeeds, with nothing to do!
}

// storeCheckApply is a tricky function where we attempt to reconcile the state
// between a third-party changing the value in the World database, and a recent
// "http:ui" change by and end user. Basically whoever runs last is the "right"
// value that we want to use. We know who sent the event from reading the
// storeEvent variable, and if it was the World, we want to cache it locally,
// and if it was the Web, then we want to push it up to the store.
func (obj *HTTPUIInputRes) storeCheckApply(ctx context.Context, apply bool) (bool, error) {

	v1, exists, err := obj.storeGet(ctx, obj.getKey())
	if err != nil {
		return false, errwrap.Wrapf(err, "error during get")
	}

	obj.mutex.Lock()
	v2 := obj.value // gets set by obj.SetValue
	storeEvent := obj.storeEvent
	obj.storeEvent = false // reset it
	obj.mutex.Unlock()

	if exists && v1 == v2 { // both sides are happy
		if err := obj.init.Send(&HTTPUIInputSends{
			Value: &v2,
		}); err != nil {
			return false, err
		}
		return true, nil
	}

	if !apply {
		if err := obj.init.Send(&HTTPUIInputSends{
			Value: &v2, // XXX: arbitrary since we're in noop mode
		}); err != nil {
			return false, err
		}
		return false, nil
	}

	obj.mutex.Lock()
	if storeEvent { // event from World, pull down the value
		err = obj.setValue(ctx, v1) // also sends an event
	}
	value := obj.value
	obj.mutex.Unlock()
	if err != nil {
		return false, err
	}

	if !exists || !storeEvent { // event from web, push up the value
		if err := obj.storeSet(ctx, obj.getKey(), value); err != nil {
			return false, errwrap.Wrapf(err, "error during set")
		}
	}

	obj.init.Logf("sending: %s", value)

	// send
	if err := obj.init.Send(&HTTPUIInputSends{
		Value: &value,
	}); err != nil {
		return false, err
	}

	return false, nil
}

func (obj *HTTPUIInputRes) storeGet(ctx context.Context, key string) (string, bool, error) {
	if obj.Store != "" && obj.scheme == httpUIInputStoreSchemeLocal {
		val, err := obj.init.Local.ValueGet(ctx, key)
		if err != nil {
			return "", false, err // real error
		}
		if val == nil { // if val is nil, and no error then it doesn't exist
			return "", false, nil // val doesn't exist
		}
		s, ok := val.(string)
		if !ok {
			// TODO: support different types perhaps?
			return "", false, fmt.Errorf("not a string") // real error
		}
		return s, true, nil
	}

	if obj.Store != "" && obj.scheme == httpUIInputStoreSchemeWorld {
		val, err := obj.init.World.StrGet(ctx, key)
		if err != nil && obj.init.World.StrIsNotExist(err) {
			return "", false, nil // val doesn't exist
		}
		if err != nil {
			return "", false, err // real error
		}
		return val, true, nil
	}

	return "", false, nil // something else
}

func (obj *HTTPUIInputRes) storeSet(ctx context.Context, key, val string) error {

	if obj.Store != "" && obj.scheme == httpUIInputStoreSchemeLocal {
		return obj.init.Local.ValueSet(ctx, key, val)
	}

	if obj.Store != "" && obj.scheme == httpUIInputStoreSchemeWorld {
		return obj.init.World.StrSet(ctx, key, val)
	}

	return nil // something else
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPUIInputRes) Cmp(r engine.Res) error {
	// we can only compare HTTPUIInputRes to others of the same resource kind
	res, ok := r.(*HTTPUIInputRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}
	if obj.ID != res.ID {
		return fmt.Errorf("the ID differs")
	}
	if obj.Value != res.Value {
		return fmt.Errorf("the Value differs")
	}
	if obj.Store != res.Store {
		return fmt.Errorf("the Store differs")
	}
	if obj.Type != res.Type {
		return fmt.Errorf("the Type differs")
	}
	if obj.Sort != res.Sort {
		return fmt.Errorf("the Sort differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPUIInputRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPUIInputRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*HTTPUIInputRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPUIInputRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPUIInputRes(raw) // restore from indirection with type conversion!
	return nil
}

// HTTPUIInputSends is the struct of data which is sent after a successful
// Apply.
type HTTPUIInputSends struct {
	// Value is the text element value being sent.
	Value *string `lang:"value"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *HTTPUIInputRes) Sends() interface{} {
	return &HTTPUIInputSends{
		Value: nil,
	}
}
