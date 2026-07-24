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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
)

func init() {
	engine.RegisterResource("json:edit", func() engine.Res { return &JSONEditRes{} })
}

const (
	jsonEditStoreSchemeEtcd = "etcd"
	jsonEditStoreSchemeFile = "file"
)

var (
	jsonEditStoreLock  = &sync.Mutex{}
	jsonEditStoreLocks = make(map[string]*sync.Mutex)
)

// JSONEditRes applies declarative edits to a JSON document. The resource name
// selects where the complete document is kept unless Store overrides it. A file
// URI or absolute path names a local file, and an etcd URI names a key in
// mgmt's shared string store, which is the same storage used by the kv
// resource.
//
// Edits use a deliberately small, jq-like assignment syntax:
//
//	.server01.state = "ready"
//	.["server-01"]["state"] = "ready"
//	.["server01"].settings = {"enabled":true}
//	del(.server01.legacy)
//
// Dot identifiers may contain ASCII letters, digits, and underscores, and
// cannot start with a digit. Bracket keys are JSON strings and can represent
// any non-empty object key. An assignment right-hand side is exactly one JSON
// value. The del form removes one object entry. Only object traversal,
// assignment, and deletion are supported: filters, array traversal, pipes, and
// arbitrary jq expressions are not. Assignments create missing intermediate
// objects; deletion of a missing path is already converged. Edits to one store
// are serialized within this process. The World API doesn't yet have a
// compare-and-swap operation, so edits from separate processes can overwrite
// each other.
type JSONEditRes struct {
	traits.Base

	init *engine.Init

	// Store optionally overrides the resource name as the store handle. It
	// accepts an absolute path, a file URI, or an etcd URI. Examples are
	// /etc/mgmt/hosts.json, file:///etc/mgmt/hosts.json, and
	// etcd:///magic/hosts.
	Store string `lang:"store" yaml:"store"`

	// Edits are the jq-like operations to converge. They are applied in
	// order to one document and written atomically as a batch.
	Edits []string `lang:"edits" yaml:"edits"`

	store *jsonEditStore
	edits []*jsonEdit
}

// Default returns some sensible defaults for this resource.
func (obj *JSONEditRes) Default() engine.Res {
	return &JSONEditRes{}
}

// getStore returns the configured store handle.
func (obj *JSONEditRes) getStore() string {
	if obj.Store != "" {
		return obj.Store
	}
	return obj.Name()
}

// Validate checks the store URI and edit expressions.
func (obj *JSONEditRes) Validate() error {
	if _, err := parseJSONEditStore(obj.getStore()); err != nil {
		return err
	}
	if _, err := parseJSONEdits(obj.Edits); err != nil {
		return err
	}
	return nil
}

// Init saves the parsed store and edits.
func (obj *JSONEditRes) Init(init *engine.Init) error {
	obj.init = init

	store, err := parseJSONEditStore(obj.getStore())
	if err != nil {
		return err
	}
	edits, err := parseJSONEdits(obj.Edits)
	if err != nil {
		return err
	}
	obj.store = store
	obj.edits = edits
	return nil
}

// Cleanup has no persistent state to release.
func (obj *JSONEditRes) Cleanup() error {
	return nil
}

// Watch watches the selected store for external changes.
func (obj *JSONEditRes) Watch(ctx context.Context) error {
	switch obj.store.scheme {
	case jsonEditStoreSchemeFile:
		return obj.fileWatch(ctx)

	case jsonEditStoreSchemeEtcd:
		return obj.etcdWatch(ctx)

	default:
		return fmt.Errorf("unsupported JSON store scheme: %s", obj.store.scheme)
	}
}

// fileWatch watches a JSON document stored in a local file.
func (obj *JSONEditRes) fileWatch(ctx context.Context) error {
	watcher, err := recwatch.NewRecWatcher(obj.store.location, false)
	if err != nil {
		return errwrap.Wrapf(err, "could not watch JSON store")
	}
	defer watcher.Close()

	if err := obj.init.Event(ctx); err != nil {
		return err
	}
	for {
		select {
		case event, ok := <-watcher.Events():
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if event == nil {
				return fmt.Errorf("the JSON store watch returned a nil event")
			}
			if event.Error != nil {
				return errwrap.Wrapf(event.Error, "the JSON store watch failed")
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// etcdWatch watches a JSON document stored in the World string store.
func (obj *JSONEditRes) etcdWatch(ctx context.Context) error {
	events, err := obj.init.World.StrWatch(ctx, obj.store.location)
	if err != nil {
		return errwrap.Wrapf(err, "could not watch JSON store")
	}

	if err := obj.init.Event(ctx); err != nil {
		return err
	}
	for {
		select {
		case err, ok := <-events:
			if !ok {
				return fmt.Errorf("unexpected close")
			}
			if err != nil {
				return errwrap.Wrapf(err, "the JSON store watch failed")
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply converges the requested edits.
func (obj *JSONEditRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	lock := jsonEditLock(obj.store.scheme + "://" + obj.store.location)
	lock.Lock()
	defer lock.Unlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	data, err := obj.storeRead(ctx)
	if err != nil {
		return false, err
	}
	root, err := jsonEditDecode(data)
	if err != nil {
		return false, err
	}

	before, err := json.Marshal(root)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not encode current JSON store")
	}
	for i, edit := range obj.edits {
		if err := applyJSONEdit(root, edit); err != nil {
			return false, errwrap.Wrapf(err, "could not apply edit at index %d", i)
		}
	}
	after, err := json.Marshal(root)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not encode desired JSON store")
	}
	if bytes.Equal(before, after) {
		return true, nil
	}
	if !apply {
		return false, nil
	}

	output, err := json.MarshalIndent(root, "", "\t")
	if err != nil {
		return false, errwrap.Wrapf(err, "could not encode JSON store")
	}
	output = append(output, '\n')
	if err := obj.storeWrite(ctx, output); err != nil {
		return false, err
	}
	for _, edit := range obj.Edits {
		obj.init.Logf("applied edit: %s", edit)
	}
	return false, nil
}

// storeRead reads the complete JSON document.
func (obj *JSONEditRes) storeRead(ctx context.Context) ([]byte, error) {
	switch obj.store.scheme {
	case jsonEditStoreSchemeFile:
		data, err := os.ReadFile(obj.store.location)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not read JSON store")
		}
		return data, nil

	case jsonEditStoreSchemeEtcd:
		value, err := obj.init.World.StrGet(ctx, obj.store.location)
		if err != nil && obj.init.World.StrIsNotExist(err) {
			return []byte("{}"), nil
		}
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not read JSON store")
		}
		return []byte(value), nil

	default:
		return nil, fmt.Errorf("unsupported JSON store scheme: %s", obj.store.scheme)
	}
}

// storeWrite writes the complete JSON document.
func (obj *JSONEditRes) storeWrite(ctx context.Context, data []byte) error {
	switch obj.store.scheme {
	case jsonEditStoreSchemeFile:
		return jsonEditWriteFile(ctx, obj.store.location, data)

	case jsonEditStoreSchemeEtcd:
		// XXX: Add compare-and-swap to the World string API and use it
		// across storeRead and storeWrite so separate processes cannot
		// lose concurrent edits.
		if err := obj.init.World.StrSet(ctx, obj.store.location, string(data)); err != nil {
			return errwrap.Wrapf(err, "could not write JSON store")
		}
		return nil

	default:
		return fmt.Errorf("unsupported JSON store scheme: %s", obj.store.scheme)
	}
}

// Cmp compares two resources and returns an error if they differ.
func (obj *JSONEditRes) Cmp(r engine.Res) error {
	res, ok := r.(*JSONEditRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	if obj.Store != res.Store {
		return fmt.Errorf("the Store differs")
	}
	if len(obj.Edits) != len(res.Edits) {
		return fmt.Errorf("the Edits differ")
	}
	for i, edit := range obj.Edits {
		if edit != res.Edits[i] {
			return fmt.Errorf("the Edits differ")
		}
	}
	return nil
}

// jsonEditStore is a parsed storage URI.
type jsonEditStore struct {
	scheme   string
	location string
}

// parseJSONEditStore parses a store handle.
func parseJSONEditStore(handle string) (*jsonEditStore, error) {
	if handle == "" {
		return nil, fmt.Errorf("store handle is empty")
	}
	if filepath.IsAbs(handle) {
		return &jsonEditStore{
			scheme:   jsonEditStoreSchemeFile,
			location: handle,
		}, nil
	}

	uri, err := url.Parse(handle)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not parse store handle")
	}

	switch uri.Scheme {
	case jsonEditStoreSchemeFile:
		if uri.Host != "" {
			return nil, fmt.Errorf("file store host is not supported")
		}
		if uri.RawQuery != "" || uri.Fragment != "" {
			return nil, fmt.Errorf("file store query and fragment are not supported")
		}
		if !filepath.IsAbs(uri.Path) {
			return nil, fmt.Errorf("file store path is not absolute")
		}
		return &jsonEditStore{
			scheme:   jsonEditStoreSchemeFile,
			location: uri.Path,
		}, nil

	case jsonEditStoreSchemeEtcd:
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
		return &jsonEditStore{
			scheme:   jsonEditStoreSchemeEtcd,
			location: location,
		}, nil

	default:
		return nil, fmt.Errorf("unknown store scheme: %s", uri.Scheme)
	}
}

// jsonEditOperation is the operation performed by an edit expression.
type jsonEditOperation uint8

const (
	jsonEditOperationSet jsonEditOperation = iota
	jsonEditOperationDelete
)

// jsonEdit is a parsed edit expression.
type jsonEdit struct {
	operation jsonEditOperation
	keys      []string
	value     interface{}
}

// parseJSONEdits parses a non-empty list of edit expressions.
func parseJSONEdits(inputs []string) ([]*jsonEdit, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("edits is empty")
	}

	edits := make([]*jsonEdit, 0, len(inputs))
	for i, input := range inputs {
		edit, err := parseJSONEdit(input)
		if err != nil {
			return nil, errwrap.Wrapf(err, "invalid edit at index %d", i)
		}
		edits = append(edits, edit)
	}
	return edits, nil
}

// parseJSONEdit parses the supported jq-like assignment syntax.
func parseJSONEdit(input string) (*jsonEdit, error) {
	parser := &jsonEditParser{input: strings.TrimSpace(input)}
	if parser.takeString("del") {
		return parser.parseDelete()
	}

	keys, err := parser.parsePath()
	if err != nil {
		return nil, err
	}
	parser.skipSpace()
	if !parser.take('=') {
		return nil, fmt.Errorf("expected `=`")
	}
	parser.skipSpace()
	if parser.done() {
		return nil, fmt.Errorf("missing JSON value")
	}

	decoder := json.NewDecoder(strings.NewReader(parser.input[parser.pos:]))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, errwrap.Wrapf(err, "could not parse JSON value")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("the JSON value contains trailing data")
		}
		return nil, errwrap.Wrapf(err, "could not finish parsing JSON value")
	}

	return &jsonEdit{
		operation: jsonEditOperationSet,
		keys:      keys,
		value:     value,
	}, nil
}

// jsonEditParser parses the object path portion of an edit.
type jsonEditParser struct {
	input string
	pos   int
}

// parseDelete parses a del(path) expression.
func (obj *jsonEditParser) parseDelete() (*jsonEdit, error) {
	obj.skipSpace()
	if !obj.take('(') {
		return nil, fmt.Errorf("expected `(` after `del`")
	}
	obj.skipSpace()
	keys, err := obj.parsePath()
	if err != nil {
		return nil, err
	}
	obj.skipSpace()
	if !obj.take(')') {
		return nil, fmt.Errorf("expected `)` after delete path")
	}
	obj.skipSpace()
	if !obj.done() {
		return nil, fmt.Errorf("delete expression contains trailing data")
	}

	return &jsonEdit{
		operation: jsonEditOperationDelete,
		keys:      keys,
	}, nil
}

// parsePath parses a dot path followed by dot identifiers or bracket keys.
func (obj *jsonEditParser) parsePath() ([]string, error) {
	if !obj.take('.') {
		return nil, fmt.Errorf("edit must start with `.`")
	}

	keys := []string{}
	for {
		obj.skipSpace()
		var key string
		var err error
		if obj.peek() == '[' {
			key, err = obj.parseBracketKey()
		} else {
			key, err = obj.parseIdentifier()
		}
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)

		obj.skipSpace()
		switch obj.peek() {
		case '.':
			obj.pos++
		case '[':
			continue
		default:
			return keys, nil
		}
	}
}

// parseIdentifier parses one dot identifier.
func (obj *jsonEditParser) parseIdentifier() (string, error) {
	start := obj.pos
	if obj.done() || !isJSONEditIdentifierStart(obj.input[obj.pos]) {
		return "", fmt.Errorf("expected object identifier")
	}
	obj.pos++
	for !obj.done() && isJSONEditIdentifierPart(obj.input[obj.pos]) {
		obj.pos++
	}
	return obj.input[start:obj.pos], nil
}

// parseBracketKey parses one bracketed JSON string.
func (obj *jsonEditParser) parseBracketKey() (string, error) {
	if !obj.take('[') {
		return "", fmt.Errorf("expected `[`")
	}
	obj.skipSpace()
	if obj.peek() != '"' {
		return "", fmt.Errorf("bracket key must be a JSON string")
	}

	start := obj.pos
	obj.pos++
	escaped := false
	terminated := false
	for !obj.done() {
		ch := obj.input[obj.pos]
		obj.pos++
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			terminated = true
			break
		}
	}
	if !terminated {
		return "", fmt.Errorf("unterminated bracket key")
	}

	var key string
	if err := json.Unmarshal([]byte(obj.input[start:obj.pos]), &key); err != nil {
		return "", errwrap.Wrapf(err, "invalid bracket key")
	}
	if key == "" {
		return "", fmt.Errorf("object key is empty")
	}
	obj.skipSpace()
	if !obj.take(']') {
		return "", fmt.Errorf("expected `]`")
	}
	return key, nil
}

// skipSpace skips ASCII whitespace.
func (obj *jsonEditParser) skipSpace() {
	for !obj.done() {
		switch obj.input[obj.pos] {
		case ' ', '\t', '\n', '\r':
			obj.pos++
		default:
			return
		}
	}
}

// takeString consumes one expected string.
func (obj *jsonEditParser) takeString(value string) bool {
	if !strings.HasPrefix(obj.input[obj.pos:], value) {
		return false
	}
	obj.pos += len(value)
	return true
}

// take consumes one expected byte.
func (obj *jsonEditParser) take(ch byte) bool {
	if obj.done() || obj.input[obj.pos] != ch {
		return false
	}
	obj.pos++
	return true
}

// peek returns the next byte, or zero at the end.
func (obj *jsonEditParser) peek() byte {
	if obj.done() {
		return 0
	}
	return obj.input[obj.pos]
}

// done reports whether the parser consumed all input.
func (obj *jsonEditParser) done() bool {
	return obj.pos >= len(obj.input)
}

// isJSONEditIdentifierStart reports whether ch can start a dot identifier.
func isJSONEditIdentifierStart(ch byte) bool {
	return ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

// isJSONEditIdentifierPart reports whether ch can continue a dot identifier.
func isJSONEditIdentifierPart(ch byte) bool {
	return isJSONEditIdentifierStart(ch) || ch >= '0' && ch <= '9'
}

// applyJSONEdit applies one parsed edit to a decoded JSON object.
func applyJSONEdit(root map[string]interface{}, edit *jsonEdit) error {
	parent := root
	for _, key := range edit.keys[:len(edit.keys)-1] {
		child, exists := parent[key]
		if !exists {
			if edit.operation == jsonEditOperationDelete {
				return nil
			}
			child = make(map[string]interface{})
			parent[key] = child
		}
		next, ok := child.(map[string]interface{})
		if !ok {
			return fmt.Errorf("edit key `%s` is not a JSON object", key)
		}
		parent = next
	}

	leaf := edit.keys[len(edit.keys)-1]
	switch edit.operation {
	case jsonEditOperationSet:
		parent[leaf] = edit.value

	case jsonEditOperationDelete:
		delete(parent, leaf)

	default:
		return fmt.Errorf("unknown JSON edit operation: %d", edit.operation)
	}
	return nil
}

// jsonEditDecode decodes exactly one JSON object.
func jsonEditDecode(data []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	root := make(map[string]interface{})
	if err := decoder.Decode(&root); err != nil {
		return nil, errwrap.Wrapf(err, "could not parse JSON store")
	}
	if root == nil {
		return nil, fmt.Errorf("the JSON store root is not an object")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("the JSON store contains multiple values")
		}
		return nil, errwrap.Wrapf(err, "could not finish parsing JSON store")
	}
	return root, nil
}

// jsonEditEqual compares values by their JSON representation.
func jsonEditEqual(a, b interface{}) bool {
	aa, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(aa, bb)
}

// jsonEditLock returns the shared mutex for one store.
func jsonEditLock(store string) *sync.Mutex {
	jsonEditStoreLock.Lock()
	defer jsonEditStoreLock.Unlock()

	if lock, exists := jsonEditStoreLocks[store]; exists {
		return lock
	}
	lock := &sync.Mutex{}
	jsonEditStoreLocks[store] = lock
	return lock
}

// jsonEditWriteFile atomically replaces a JSON file while preserving its
// permissions and ownership.
func jsonEditWriteFile(ctx context.Context, path string, data []byte) (retErr error) {
	info, err := os.Lstat(path)
	if err != nil {
		return errwrap.Wrapf(err, "could not stat JSON file")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("the JSON path is not a regular file")
	}

	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return errwrap.Wrapf(err, "could not create temporary JSON file")
	}
	tmp := file.Name()
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil && retErr == nil {
				retErr = errwrap.Wrapf(err, "could not close temporary JSON file")
			}
		}
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) && retErr == nil {
			retErr = errwrap.Wrapf(err, "could not remove temporary JSON file")
		}
	}()

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := file.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			return errwrap.Wrapf(err, "could not preserve JSON file ownership")
		}
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return errwrap.Wrapf(err, "could not preserve JSON file mode")
	}
	if _, err := file.Write(data); err != nil {
		return errwrap.Wrapf(err, "could not write temporary JSON file")
	}
	if err := file.Sync(); err != nil {
		return errwrap.Wrapf(err, "could not sync temporary JSON file")
	}
	err = file.Close()
	closed = true
	if err != nil {
		return errwrap.Wrapf(err, "could not close temporary JSON file")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errwrap.Wrapf(err, "could not replace JSON file")
	}

	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return errwrap.Wrapf(err, "could not open JSON directory")
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return errwrap.Wrapf(err, "could not sync JSON directory")
	}
	return nil
}
