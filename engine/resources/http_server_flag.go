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
	"net/http"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	httpServerFlagKind = httpServerKind + ":flag"
)

func init() {
	engine.RegisterResource(httpServerFlagKind, func() engine.Res { return &HTTPServerFlagRes{} })
}

var _ HTTPServerGroupableRes = &HTTPServerFlagRes{} // compile time check

// HTTPServerFlagRes is a special path that exists within an http server. The
// name is used as the public path of the flag, unless the path field is
// specified, and in that case it is used instead. The way this works is that it
// autogroups at runtime with an existing http resource, and in doing so makes
// the flag associated with this resource available to cause actions when it
// receives a request on that http server. If you create a flag which responds
// to the same type of request as an http:server:file resource or any other kind
// of resource, it is undefined behaviour which will answer the request. The
// most common clash will happen if both are present at the same path.
type HTTPServerFlagRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPServerRes or itself
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

	mutex       *sync.Mutex // guard the values
	eventStream chan error

	//value     *string // cached value
	//prevValue *string // previous value

	// TODO: do the values need to be pointers?
	mapResKey   map[*HTTPServerFlagRes]string // flagRes not Res
	mapResPrev  map[*HTTPServerFlagRes]*string
	mapResValue map[*HTTPServerFlagRes]*string
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPServerFlagRes) Default() engine.Res {
	return &HTTPServerFlagRes{}
}

// getPath returns the actual path we respond to. When Path is not specified, we
// use the Name.
func (obj *HTTPServerFlagRes) getPath() string {
	if obj.Path != "" {
		return obj.Path
	}
	return obj.Name()
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPServerFlagRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPServerFlagRes) AcceptHTTP(req *http.Request) error {
	// NOTE: We don't need to look at anyone that might be autogrouped,
	// because for them to autogroup, they must share the same path! The
	// idea is that they're part of the same request of course...

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
func (obj *HTTPServerFlagRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow POST at the moment.
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	//requestPath := req.URL.Path
	//if err := req.ParseForm(); err != nil { // needed to access querystring
	//	sendHTTPError(w, err)
	//	return
	//}
	for res, key := range obj.mapResKey { // TODO: sort deterministically?
		if key == "" {
			continue
		}
		val := req.PostFormValue(key) // string
		if obj.init.Debug || true {   // XXX: maybe we should always do this?
			obj.init.Logf("got %s: %s", key, val)
		}

		obj.mutex.Lock()
		if val == "" {
			//obj.value = nil // erase
			//delete(obj.mapResValue, res)
			obj.mapResValue[res] = nil
		} else {
			//obj.value = &val // store
			obj.mapResValue[res] = &val // store
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
func (obj *HTTPServerFlagRes) Validate() error {
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
func (obj *HTTPServerFlagRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.mutex = &sync.Mutex{}
	obj.eventStream = make(chan error, 1) // non-blocking

	obj.mapResKey = make(map[*HTTPServerFlagRes]string)    // res to key
	obj.mapResPrev = make(map[*HTTPServerFlagRes]*string)  // res to prev value
	obj.mapResValue = make(map[*HTTPServerFlagRes]*string) // res to value
	obj.mapResKey[obj] = obj.Key                           // add "self" res
	obj.mapResPrev[obj] = nil
	obj.mapResValue[obj] = nil

	for _, res := range obj.GetGroup() { // this is a noop if there are none!
		flagRes, ok := res.(*HTTPServerFlagRes) // convert from Res
		if !ok {
			panic(fmt.Sprintf("grouped member %v is not a %s", res, obj.Kind()))
		}

		r := res // bind the variable!

		newInit := &engine.Init{
			Program:  obj.init.Program,
			Version:  obj.init.Version,
			Hostname: obj.init.Hostname,

			// Watch:
			//Running: event,
			//Event:   event,

			// CheckApply:
			//Refresh: func() bool {
			//	innerRes, ok := r.(engine.RefreshableRes)
			//	if !ok {
			//		panic("res does not support the Refreshable trait")
			//	}
			//	return innerRes.Refresh()
			//},
			Send: engine.GenerateSendFunc(r),
			Recv: engine.GenerateRecvFunc(r), // unused

			FilteredGraph: func() (*pgraph.Graph, error) {
				panic("FilteredGraph for HTTP:Server:Flag not implemented")
			},

			Local: obj.init.Local,
			World: obj.init.World,
			//VarDir: obj.init.VarDir, // TODO: wrap this

			Debug: obj.init.Debug,
			Logf: func(format string, v ...interface{}) {
				obj.init.Logf(r.String()+": "+format, v...)
			},
		}

		if err := res.Init(newInit); err != nil {
			return errwrap.Wrapf(err, "autogrouped Init failed")
		}

		obj.mapResKey[flagRes] = flagRes.Key
		obj.mapResPrev[flagRes] = nil // initialize as a bonus
		obj.mapResValue[flagRes] = nil
	}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *HTTPServerFlagRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// particular one listens for events from incoming http requests to the flag,
// and notifies the engine so that CheckApply can then run and return the
// correct value on send/recv.
func (obj *HTTPServerFlagRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	startupChan := make(chan struct{})
	close(startupChan) // send one initial signal

	for {
		if obj.init.Debug {
			obj.init.Logf("Looping...")
		}

		select {
		case <-startupChan:
			startupChan = nil

		case err, ok := <-obj.eventStream:
			if !ok { // shouldn't happen
				obj.eventStream = nil
				continue
			}
			if err != nil {
				return err
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply never has anything to do for this resource, so it always succeeds.
func (obj *HTTPServerFlagRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	checkOK := true
	// run CheckApply on any grouped elements, or just myself
	// TODO: Should we loop in a deterministic order?
	for flagRes, key := range obj.mapResKey { // includes the main parent Res
		if obj.init.Debug {
			obj.init.Logf("key: %+v", key)
		}

		c, err := flagRes.checkApply(ctx, apply, obj)
		if err != nil {
			return false, err
		}
		checkOK = checkOK && c
	}

	return checkOK, nil
}

// checkApply is the actual implementation, but it's used as a helper to make
// the running of autogrouping easier.
func (obj *HTTPServerFlagRes) checkApply(ctx context.Context, apply bool, parentObj *HTTPServerFlagRes) (bool, error) {

	parentObj.mutex.Lock()
	objValue := parentObj.mapResValue[obj] // nil if missing
	objPrevValue := parentObj.mapResPrev[obj]

	if obj.init.Debug {
		obj.init.Logf("value: %+v", objValue)
	}

	// TODO: can we send an empty (nil) value to show it has been removed?

	value := "" // not a ptr, because we don't/can't? send a nil value

	// first compute if different...
	different := false
	if (objValue == nil) != (objPrevValue == nil) { // xor
		different = true
	} else if objValue != nil && objPrevValue != nil {
		if *objValue != *objPrevValue {
			different = true
		}
	}

	// now store in previous
	if objValue == nil {
		//obj.prevValue = nil
		parentObj.mapResPrev[obj] = nil

	} else { // a value has been set
		v := *objValue
		//obj.prevValue = &v // value to cache for future compare
		parentObj.mapResPrev[obj] = &v

		value = *objValue // value for send/recv
	}
	parentObj.mutex.Unlock()

	// Previously, if we graph swapped, as is quite common, we'd loose
	// obj.value because the swap would destroy and then re-create and then
	// re-autogroup, all because the Cmp function looked at whatever value
	// we received from send/recv when comparing to the brand-new resource.
	// As a result, we need to run send/recv on the new graph after
	// autogrouping, so that we compare apples to apples, when we do the
	// graphsync!
	if err := obj.init.Send(&HTTPServerFlagSends{
		Value: &value,
	}); err != nil {
		return false, err
	}

	// TODO: should we always return true?
	return !different, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPServerFlagRes) Cmp(r engine.Res) error {
	// we can only compare HTTPServerFlagRes to others of the same resource kind
	res, ok := r.(*HTTPServerFlagRes)
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

// HTTPServerFlagSends is the struct of data which is sent after a successful
// Apply.
type HTTPServerFlagSends struct {
	// Value is the received value being sent.
	Value *string `lang:"value"`
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *HTTPServerFlagRes) Sends() interface{} {
	return &HTTPServerFlagSends{
		Value: nil,
	}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *HTTPServerFlagRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*HTTPServerFlagRes)
	if !ok {
		return fmt.Errorf("resource is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("resource has a different Server field")
	}

	if obj.getPath() != res.getPath() {
		return fmt.Errorf("resource has a different path")
	}

	//if obj.Method != res.Method {
	//	return fmt.Errorf("resource has a different Method field")
	//}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPServerFlagRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPServerFlagRes // indirection to avoid infinite recursion

	def := obj.Default()                // get the default
	res, ok := def.(*HTTPServerFlagRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPServerFlagRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPServerFlagRes(raw) // restore from indirection with type conversion!
	return nil
}
