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

package util

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

const (
	// JSONStoreSchemeEtcd is the scheme of a JSON store in etcd.
	JSONStoreSchemeEtcd = "etcd"

	// JSONStoreSchemeFile is the scheme of a JSON store in a local file.
	JSONStoreSchemeFile = "file"

	// JSONStoreSchemeLocal is the scheme of a JSON store in the Local Value
	// API, which keeps it in the mgmt prefix dir of this host.
	JSONStoreSchemeLocal = "local"
)

// ParseJSONStore parses a store handle. It accepts an absolute path, a file
// URI, an etcd URI, or a local URI. Examples are /etc/mgmt/hosts.json,
// file:///etc/mgmt/hosts.json, etcd:///magic/hosts and local:///hosts. A local
// key is a file name in the mgmt prefix dir, so it can't have a slash in it.
func ParseJSONStore(handle string) (*JSONStore, error) {
	if handle == "" {
		return nil, fmt.Errorf("store handle is empty")
	}
	if filepath.IsAbs(handle) {
		return &JSONStore{
			Scheme:   JSONStoreSchemeFile,
			Location: handle,
		}, nil
	}

	uri, err := url.Parse(handle)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not parse store handle")
	}

	switch uri.Scheme {
	case JSONStoreSchemeFile:
		if uri.Host != "" {
			return nil, fmt.Errorf("file store host is not supported")
		}
		if uri.RawQuery != "" || uri.Fragment != "" {
			return nil, fmt.Errorf("file store query and fragment are not supported")
		}
		if !filepath.IsAbs(uri.Path) {
			return nil, fmt.Errorf("file store path is not absolute")
		}
		return &JSONStore{
			Scheme:   JSONStoreSchemeFile,
			Location: uri.Path,
		}, nil

	case JSONStoreSchemeEtcd:
		if uri.Host != "" {
			return nil, fmt.Errorf("etcd store host is not supported")
		}
		if uri.RawQuery != "" || uri.Fragment != "" {
			return nil, fmt.Errorf("etcd store query and fragment are not supported")
		}
		if uri.Opaque != "" {
			return nil, fmt.Errorf("etcd store path is not absolute")
		}

		location := strings.TrimPrefix(uri.Path, "/")
		if location == "" {
			return nil, fmt.Errorf("etcd store key is empty")
		}
		return &JSONStore{
			Scheme:   JSONStoreSchemeEtcd,
			Location: location,
		}, nil

	case JSONStoreSchemeLocal:
		if uri.Host != "" {
			return nil, fmt.Errorf("local store host is not supported")
		}
		if uri.RawQuery != "" || uri.Fragment != "" {
			return nil, fmt.Errorf("local store query and fragment are not supported")
		}
		if uri.Opaque != "" {
			return nil, fmt.Errorf("local store path is not absolute")
		}

		location := strings.TrimPrefix(uri.Path, "/")
		if location == "" {
			return nil, fmt.Errorf("local store key is empty")
		}
		if strings.Contains(location, "/") || location == "." || location == ".." {
			return nil, fmt.Errorf("local store key must be a file name")
		}
		return &JSONStore{
			Scheme:   JSONStoreSchemeLocal,
			Location: location,
		}, nil

	default:
		return nil, fmt.Errorf("unknown store scheme: %s", uri.Scheme)
	}
}

// JSONStore is where a resource keeps a JSON document, either in a local file,
// in etcd, or in the Local Value API. Its edits go through the Local and World
// API's, which make them atomic, so a resource never needs to coordinate with
// any others that share its store.
type JSONStore struct {
	// Scheme is JSONStoreSchemeFile, JSONStoreSchemeEtcd or
	// JSONStoreSchemeLocal.
	Scheme string

	// Location is the absolute path of the file, the etcd key, or the local
	// key.
	Location string
}

// String returns the store as a URI, such as file:///etc/mgmt/hosts.json or
// etcd:///magic/hosts. It's escaped, so that ParseJSONStore always gets this
// same store back from it.
func (obj *JSONStore) String() string {
	path := obj.Location // an absolute file path
	if obj.Scheme == JSONStoreSchemeEtcd || obj.Scheme == JSONStoreSchemeLocal {
		path = "/" + obj.Location // an etcd key may start with a slash
	}
	return (&url.URL{Scheme: obj.Scheme, Path: path}).String()
}

// Watch sends an event on startup, and then whenever the store changes, until
// the context is cancelled.
func (obj *JSONStore) Watch(ctx context.Context, init *engine.Init) error {
	switch obj.Scheme {
	case JSONStoreSchemeFile:
		return obj.fileWatch(ctx, init)

	case JSONStoreSchemeEtcd:
		return obj.etcdWatch(ctx, init)

	case JSONStoreSchemeLocal:
		return obj.localWatch(ctx, init)

	default:
		return fmt.Errorf("unsupported JSON store scheme: %s", obj.Scheme)
	}
}

// fileWatch watches a JSON document stored in a local file.
func (obj *JSONStore) fileWatch(ctx context.Context, init *engine.Init) error {
	watcher, err := recwatch.NewRecWatcher(ctx, obj.Location, false)
	if err != nil {
		return errwrap.Wrapf(err, "could not watch JSON store")
	}
	defer watcher.Cleanup()

	if err := init.Event(ctx); err != nil {
		return err
	}
	for {
		select {
		case event, ok := <-watcher.Events():
			if ctx.Err() != nil {
				return ctx.Err() // engine is shutting us down
			}
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if event == nil {
				return fmt.Errorf("the JSON store watch returned a nil event")
			}
			if event.Error != nil {
				return event.Error
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		if err := init.Event(ctx); err != nil {
			return err
		}
	}
}

// etcdWatch watches a JSON document stored in the World string store.
func (obj *JSONStore) etcdWatch(ctx context.Context, init *engine.Init) error {
	events, err := init.World.StrWatch(ctx, obj.Location)
	if err != nil {
		return errwrap.Wrapf(err, "could not watch JSON store")
	}

	if err := init.Event(ctx); err != nil {
		return err
	}
	for {
		select {
		case err, ok := <-events:
			if ctx.Err() != nil {
				return ctx.Err() // engine is shutting us down
			}
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if err != nil {
				return errwrap.Wrapf(err, "the JSON store watch failed")
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		if err := init.Event(ctx); err != nil {
			return err
		}
	}
}

// localWatch watches a JSON document stored in the Local Value API.
func (obj *JSONStore) localWatch(ctx context.Context, init *engine.Init) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops the watch

	events, err := init.Local.ValueWatch(ctx, obj.Location)
	if err != nil {
		return errwrap.Wrapf(err, "could not watch JSON store")
	}

	// The watch sends its first event at startup, which is our startup event.
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return fmt.Errorf("unexpected close")
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		if err := init.Event(ctx); err != nil {
			return err
		}
	}
}

// Get returns the whole document, or nil if there isn't one.
func (obj *JSONStore) Get(ctx context.Context, init *engine.Init) (*string, error) {
	switch obj.Scheme {
	case JSONStoreSchemeFile:
		data, err := os.ReadFile(obj.Location)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		value := string(data)
		return &value, nil

	case JSONStoreSchemeEtcd:
		value, err := init.World.StrGet(ctx, obj.Location)
		if err != nil && init.World.StrIsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &value, nil

	case JSONStoreSchemeLocal:
		v, err := init.Local.ValueGet(ctx, obj.Location)
		if err != nil {
			return nil, err
		}
		return jsonStoreLocalString(v)

	default:
		return nil, fmt.Errorf("unsupported JSON store scheme: %s", obj.Scheme)
	}
}

// Edit atomically edits the whole document. The edit function gets the current
// document, or nil if there isn't one, and returns the new one, or nil to
// remove it. Nothing is written if it returns the same document. It can run
// more than once so it must be a pure function which only depends on its input.
func (obj *JSONStore) Edit(ctx context.Context, init *engine.Init, fn func(*string) (*string, error)) error {
	switch obj.Scheme {
	case JSONStoreSchemeFile:
		return init.Local.FileEdit(ctx, obj.Location, fn)

	case JSONStoreSchemeEtcd:
		return init.World.StrEdit(ctx, obj.Location, fn)

	case JSONStoreSchemeLocal:
		return init.Local.ValueEdit(ctx, obj.Location, func(v interface{}) (interface{}, error) {
			value, err := jsonStoreLocalString(v)
			if err != nil {
				return nil, err
			}
			output, err := fn(value)
			if err != nil {
				return nil, err
			}
			if output == nil {
				return nil, nil // remove it
			}
			return *output, nil
		})

	default:
		return fmt.Errorf("unsupported JSON store scheme: %s", obj.Scheme)
	}
}

// jsonStoreLocalString returns the document which is stored in the Local Value
// API, or nil if there isn't one. It's stored as a string.
func jsonStoreLocalString(v interface{}) (*string, error) {
	if v == nil {
		return nil, nil
	}
	value, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("the JSON store value is not a string")
	}
	return &value, nil
}

// DecodeJSONObject decodes exactly one JSON object. Numbers are kept as their
// json.Number strings, so that they're stored again exactly as they were.
func DecodeJSONObject(data []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	root := make(map[string]interface{})
	if err := decoder.Decode(&root); err != nil {
		return nil, errwrap.Wrapf(err, "could not parse JSON object")
	}
	if root == nil {
		return nil, fmt.Errorf("the JSON root is not an object")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("the JSON contains multiple values")
		}
		return nil, errwrap.Wrapf(err, "could not finish parsing JSON")
	}
	return root, nil
}

// EncodeJSONObject encodes a JSON object the way that a JSONStore keeps it.
// It's indented with tabs, and it ends with a newline.
func EncodeJSONObject(root map[string]interface{}) (string, error) {
	output, err := json.MarshalIndent(root, "", "\t")
	if err != nil {
		return "", errwrap.Wrapf(err, "could not encode JSON object")
	}
	return string(output) + "\n", nil
}
