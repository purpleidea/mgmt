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

package resources

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

const (
	httpFlagKind = httpKind + ":flag"
)

func init() {
	engine.RegisterResource(httpFlagKind, func() engine.Res { return &HTTPFlagRes{} })
}

// HTTPFlagRes is a special path that exists within an http server. The name is
// used as the public path of the flag, unless the path field is specified, and
// in that case it is used instead. The way this works is that it autogroups at
// runtime with an existing http resource, and in doing so makes the flag
// associated with this resource available to cause actions when it receives a
// request on that http server. If you create a flag which responds to the same
// type of request as an http:file resource or any other kind of resource, it is
// undefined behaviour which will answer the request. The most common clash will
// happen if both are present at the same path.
type HTTPFlagRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPServerRes
	traits.Sendable

	init *engine.Init

	// Server is the name of the http server resource to group this into. If
	// it is omitted, and there is only a single http resource, then it will
	// be grouped into it automatically. If there is more than one main http
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// Path is the path that this will present as on the http server.
	Path string `lang:"path" yaml:"path"`

	// Key is the querystring name that is used to capture a value as.
	Key string `lang:"key" yaml:"key"`

	// TODO: consider adding a method selection field
	//Method string `lang:"method" yaml:"method"`

	mutex         *sync.Mutex // guard the value
	value         *string     // cached value
	previousValue *string
	eventStream   chan error
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPFlagRes) Default() engine.Res {
	return &HTTPFlagRes{}
}

// getPath returns the actual path we respond to. When Path is not specified, we
// use the Name.
func (obj *HTTPFlagRes) getPath() string {
	if obj.Path != "" {
		return obj.Path
	}
	return obj.Name()
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPFlagRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPFlagRes) AcceptHTTP(req *http.Request) error {
	requestPath := req.URL.Path // TODO: is this what we want here?
	if requestPath != obj.getPath() {
		return fmt.Errorf("unhandled path")
	}

	// We only allow POST at the moment.
	if req.Method != http.MethodPost {
		return fmt.Errorf("unhandled method")
	}

	return nil
}

// ServeHTTP is the standard HTTP handler that will be used here.
func (obj *HTTPFlagRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow POST at the moment.
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	//requestPath := req.URL.Path
	//if err := req.ParseForm(); err != nil { // needed to access querystring
	//	msg, httpStatus := toHTTPError(err)
	//	http.Error(w, msg, httpStatus)
	//	return
	//}
	if obj.Key != "" {
		val := req.PostFormValue(obj.Key) // string
		if obj.init.Debug || true {       // XXX: maybe we should always do this?
			obj.init.Logf("Got val: %s", val)
		}

		obj.mutex.Lock()
		if val == "" {
			obj.value = nil // erase
		} else {
			obj.value = &val // store
		}
		obj.mutex.Unlock()
		// TODO: Should we diff the new value with the previous one to
		// decide if we should send a new event or not?
	}

	// Trigger a Watch() event so that CheckApply() calls Send/Recv, so our
	// newly received POST value gets sent through the graph.
	select {
	case obj.eventStream <- nil: // send an event (non-blocking)
	default:
	}

	w.WriteHeader(http.StatusOK) // 200
	return
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPFlagRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("empty filename")
	}
	// FIXME: does getPath need to start with a slash?
	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("the path must be absolute")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPFlagRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.mutex = &sync.Mutex{}
	obj.eventStream = make(chan error, 1) // non-blocking

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPFlagRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one does absolutely nothing but block until we've received a done
// signal.
func (obj *HTTPFlagRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	startupChan := make(chan struct{})
	close(startupChan) // send one initial signal

	var send = false // send event?
	for {
		if obj.init.Debug {
			obj.init.Logf("Looping...")
		}

		select {
		case <-startupChan:
			startupChan = nil
			send = true

		case err, ok := <-obj.eventStream:
			if !ok { // shouldn't happen
				obj.eventStream = nil
				continue
			}
			if err != nil {
				return err
			}
			send = true

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *HTTPFlagRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.init.Debug {
		obj.init.Logf("CheckApply")
	}

	if obj.init.Debug || true { // XXX: maybe we should always do this?
		obj.init.Logf("CheckApply: value: %+v", obj.value)
	}

	// TODO: can we send an empty (nil) value to show it has been removed?

	value := "" // not a ptr, because we don't/can't? send a nil value
	obj.mutex.Lock()

	// first compute if different...
	different := false
	if (obj.value == nil) != (obj.previousValue == nil) { // xor
		different = true
	} else if obj.value != nil && obj.previousValue != nil {
		if *obj.value != *obj.previousValue {
			different = true
		}
	}

	// now store in previous
	if obj.value == nil {
		obj.previousValue = nil

	} else { // a value has been set
		v := *obj.value
		obj.previousValue = &v // value to cache for future compare

		value = *obj.value // value for send/recv
	}
	obj.mutex.Unlock()

	// Previously, if we graph swapped, as is quite common, we'd loose
	// obj.value because the swap would destroy and then re-create and then
	// re-autogroup, all because the Cmp function looked at whatever value
	// we received from send/recv when comparing to the brand-new resource.
	// As a result, we need to run send/recv on the new graph after
	// autogrouping, so that we compare apples to apples, when we do the
	// graphsync!
	if err := obj.init.Send(&HTTPFlagSends{
		Value: &value,
	}); err != nil {
		return false, err
	}

	// TODO: should we always return true?
	return !different, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPFlagRes) Cmp(r engine.Res) error {
	// we can only compare HTTPFlagRes to others of the same resource kind
	res, ok := r.(*HTTPFlagRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}
	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}
	if obj.Key != res.Key {
		return fmt.Errorf("the Key differs")
	}

	return nil
}

// HTTPFlagSends is the struct of data which is sent after a successful Apply.
type HTTPFlagSends struct {
	// Value is the received value being sent.
	Value *string `lang:"value"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *HTTPFlagRes) Sends() interface{} {
	return &HTTPFlagSends{
		Value: nil,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPFlagRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPFlagRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*HTTPFlagRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPFlagRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPFlagRes(raw) // restore from indirection with type conversion!
	return nil
}
