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

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	httpServerRecordKind = httpServerKind + ":record"

	// HTTPServerRecordStateExists is the state which makes sure that the
	// store exists.
	HTTPServerRecordStateExists = "exists"

	// httpServerRecordMaxBytes is the largest request body that we read.
	httpServerRecordMaxBytes = 1024 * 1024 // 1 MiB
)

func init() {
	engine.RegisterResource(httpServerRecordKind, func() engine.Res { return &HTTPServerRecordRes{} })
}

var _ HTTPServerGroupableRes = &HTTPServerRecordRes{} // compile time check

// HTTPServerRecordRes is a special path that exists within an http server,
// which stores a record of each request that it receives in a JSON document.
// The name is used as the public path, unless the path field is specified, and
// in that case it is used instead. It autogroups at runtime with an existing
// http server resource, the same way that the http:server:flag resource does.
// Only one of these can respond to a given path, and if any other resource also
// responds to that path, then it is undefined behaviour which of them will get
// the request.
//
// Each POST request which has the form field named by Key is stored in the
// document, under the value of that field, as an object of the form fields that
// are listed in Fields. A later request with the same key replaces that record.
// For example, with a key of "mac" and fields of "ip" and "hostkey", a request
// of mac=00:11:22:33:44:55&ip=192.0.2.13&hostkey=AAAA results in this document:
//
//	{
//		"00:11:22:33:44:55": {
//			"hostkey": "AAAA",
//			"ip": "192.0.2.13"
//		}
//	}
//
// We only reply with a 200 OK once the record has been stored so a client which
// gets one knows that it won't be lost, and a client which gets an error should
// try again. A request without the key gets a 400. Every write goes through the
// Local and World API's, which make it atomic, so the records are never lost if
// another resource, such as json:edit, changes the same store at the same time.
// For an etcd store, this is true even if it's in another process.
//
// The records are kept after this resource is gone, so something else can read
// them, and they can be picked up by another process if this one dies. Whatever
// uses them should remove each one that it's done with, such as with a del edit
// of the json:edit resource. Be careful, because such a consumer might remove a
// newer record that replaced the one that it read, such as when a provisioned
// host is reinstalled and reports in again, because that delete isn't based on
// what it read. A timestamp field that the client sends in each request would
// let it check that it's removing the same record that it used, but json:edit
// can't do a conditional delete like that yet.
//
// Anyone who can reach this path can add or replace any record. Only the listed
// fields are stored, but the number of records is not limited.
type HTTPServerRecordRes struct {
	traits.Base      // add the base methods without re-implementation
	traits.Edgeable  // XXX: add autoedge support
	traits.Groupable // can be grouped into HTTPServerRes

	init *engine.Init

	// Server is the name of the http server resource to group this into. If
	// it is omitted, and there is only a single http resource, then it will
	// be grouped into it automatically. If there is more than one main http
	// resource being used, then the grouping behaviour is *undefined* when
	// this is not specified, and it is not recommended to leave this blank!
	Server string `lang:"server" yaml:"server"`

	// Path is the path that this will present as on the http server. If it
	// is empty, then the name is used instead.
	Path string `lang:"path" yaml:"path"`

	// Store is where the JSON document of records is kept. It accepts an
	// absolute path, a file URI, or an etcd URI, the same as the json:edit
	// resource does. Examples are /var/lib/mgmt/records.json, and
	// file:///var/lib/mgmt/records.json, and etcd:///magic/records.
	Store string `lang:"store" yaml:"store"`

	// State is "exists" to make sure that the store exists, which creates
	// an empty document if it's missing. A new file is only readable by its
	// owner. If this is empty, then we don't manage that, and a request is
	// only stored once the store exists.
	State string `lang:"state" yaml:"state"`

	// Key is the name of the form field that identifies each record.
	Key string `lang:"key" yaml:"key"`

	// Fields are the names of the form fields to store in each record. Any
	// other fields are ignored, and one that isn't in a request isn't in
	// its record.
	Fields []string `lang:"fields" yaml:"fields"`

	store *engineUtil.JSONStore
}

// Default returns some sensible defaults for this resource.
func (obj *HTTPServerRecordRes) Default() engine.Res {
	return &HTTPServerRecordRes{}
}

// getPath returns the actual path we respond to. When Path is not specified, we
// use the Name.
func (obj *HTTPServerRecordRes) getPath() string {
	if obj.Path != "" {
		return obj.Path
	}
	return obj.Name()
}

// ParentName is used to limit which resources autogroup into this one. If it's
// empty then it's ignored, otherwise it must match the Name of the parent to
// get grouped.
func (obj *HTTPServerRecordRes) ParentName() string {
	return obj.Server
}

// AcceptHTTP determines whether we will respond to this request. Return nil to
// accept, or any error to pass.
func (obj *HTTPServerRecordRes) AcceptHTTP(req *http.Request) error {
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

// ServeHTTP is the standard HTTP handler that will be used here. It stores the
// record before it replies.
func (obj *HTTPServerRecordRes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// We only allow POST at the moment.
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Limit the request body size before parsing the form, to avoid memory
	// exhaustion from an oversized POST.
	req.Body = http.MaxBytesReader(w, req.Body, httpServerRecordMaxBytes)
	if err := req.ParseMultipartForm(httpServerRecordMaxBytes); err != nil && err != http.ErrNotMultipart {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	key := req.PostForm.Get(obj.Key)
	if key == "" {
		http.Error(w, fmt.Sprintf("missing %s", obj.Key), http.StatusBadRequest)
		return
	}
	record := make(map[string]interface{})
	for _, field := range obj.Fields {
		if values, exists := req.PostForm[field]; exists && len(values) > 0 {
			record[field] = values[0]
		}
	}

	// If the client gives up, then it will have to try again anyways.
	if err := obj.store.Edit(req.Context(), obj.init, func(value *string) (*string, error) {
		return obj.add(value, key, record)
	}); err != nil {
		obj.init.Logf("could not store record %s: %v", key, err)
		http.Error(w, "could not store the record", http.StatusInternalServerError)
		return
	}
	obj.init.Logf("stored record: %s", key)

	w.WriteHeader(http.StatusOK) // 200
}

// add returns the document with this record in it. If the record is already
// there, then it returns the same document so it doesn't get rewritten.
func (obj *HTTPServerRecordRes) add(value *string, key string, record map[string]interface{}) (*string, error) {
	if value == nil && obj.State != HTTPServerRecordStateExists {
		return nil, fmt.Errorf("the store doesn't exist")
	}

	root := make(map[string]interface{})
	if value != nil {
		var err error
		if root, err = engineUtil.DecodeJSONObject([]byte(*value)); err != nil {
			return nil, errwrap.Wrapf(err, "could not decode the store")
		}
	}
	if existing, exists := root[key]; exists && jsonEditEqual(existing, record) {
		return value, nil
	}

	root[key] = record
	output, err := engineUtil.EncodeJSONObject(root)
	if err != nil {
		return nil, err
	}
	return &output, nil
}

// Validate checks if the resource data structure was populated correctly.
func (obj *HTTPServerRecordRes) Validate() error {
	if obj.getPath() == "" {
		return fmt.Errorf("empty path")
	}
	if !strings.HasPrefix(obj.getPath(), "/") {
		return fmt.Errorf("the path must be absolute")
	}

	if _, err := engineUtil.ParseJSONStore(obj.Store); err != nil {
		return errwrap.Wrapf(err, "invalid store")
	}
	if obj.State != "" && obj.State != HTTPServerRecordStateExists {
		return fmt.Errorf("the state must be %s or empty", HTTPServerRecordStateExists)
	}

	if obj.Key == "" {
		return fmt.Errorf("the key is empty")
	}
	fields := make(map[string]struct{})
	for _, field := range obj.Fields {
		if field == "" {
			return fmt.Errorf("a field is empty")
		}
		if _, exists := fields[field]; exists {
			return fmt.Errorf("duplicate field: %s", field)
		}
		fields[field] = struct{}{}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *HTTPServerRecordRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	store, err := engineUtil.ParseJSONStore(obj.Store)
	if err != nil {
		return err
	}
	obj.store = store

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. The
// records stay in the store.
func (obj *HTTPServerRecordRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. The
// requests don't cause any events, since they're stored as they come in. If we
// manage the state of the store, then we watch it, so that we can make it again
// if it gets removed.
func (obj *HTTPServerRecordRes) Watch(ctx context.Context) error {
	if obj.State == HTTPServerRecordStateExists {
		return obj.store.Watch(ctx, obj.init)
	}

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	return ctx.Err()
}

// CheckApply makes sure that the store exists if we manage its state. Otherwise
// it has nothing to do, since the records are stored as they come in.
func (obj *HTTPServerRecordRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	if obj.State != HTTPServerRecordStateExists {
		return true, nil
	}

	checkOK := true
	if err := obj.store.Edit(ctx, obj.init, func(value *string) (*string, error) {
		checkOK = value != nil // this can run more than once
		if value != nil || !apply {
			return value, nil
		}
		output, err := engineUtil.EncodeJSONObject(map[string]interface{}{})
		if err != nil {
			return nil, err
		}
		return &output, nil
	}); err != nil {
		return false, err
	}

	if !checkOK && apply {
		obj.init.Logf("created store: %s", obj.store)
	}
	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *HTTPServerRecordRes) Cmp(r engine.Res) error {
	// we can only compare HTTPServerRecordRes to others of the same kind
	res, ok := r.(*HTTPServerRecordRes)
	if !ok {
		return fmt.Errorf("res is not the same kind")
	}

	if obj.Server != res.Server {
		return fmt.Errorf("the Server field differs")
	}
	if obj.Path != res.Path {
		return fmt.Errorf("the Path differs")
	}
	if obj.Store != res.Store {
		return fmt.Errorf("the Store differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Key != res.Key {
		return fmt.Errorf("the Key differs")
	}
	if err := engineUtil.StrListCmp(obj.Fields, res.Fields); err != nil {
		return errwrap.Wrapf(err, "the Fields differ")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *HTTPServerRecordRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes HTTPServerRecordRes // indirection to avoid infinite recursion

	def := obj.Default()                  // get the default
	res, ok := def.(*HTTPServerRecordRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to HTTPServerRecordRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = HTTPServerRecordRes(raw) // restore from indirection with type conversion!
	return nil
}
