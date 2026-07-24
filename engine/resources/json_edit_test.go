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
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

func TestJSONEditParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		keys  []string
		value interface{}
	}{
		{
			name:  "identifiers",
			input: `.server01.state = "ready"`,
			keys:  []string{"server01", "state"},
			value: "ready",
		},
		{
			name:  "bracket keys",
			input: `.["server-01"]["state"] = true`,
			keys:  []string{"server-01", "state"},
			value: true,
		},
		{
			name:  "object value",
			input: `.server.settings = {"enabled":true}`,
			keys:  []string{"server", "settings"},
			value: map[string]interface{}{"enabled": true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			edit, err := parseJSONEdit(test.input)
			if err != nil {
				t.Fatalf("func parseJSONEdit failed: %+v", err)
			}
			if !reflect.DeepEqual(edit.keys, test.keys) {
				t.Errorf("unexpected keys: got %#v, want %#v", edit.keys, test.keys)
			}
			if edit.operation != jsonEditOperationSet {
				t.Errorf("unexpected operation: got %d, want %d", edit.operation, jsonEditOperationSet)
			}
			if !jsonEditEqual(edit.value, test.value) {
				t.Errorf("unexpected value: got %#v, want %#v", edit.value, test.value)
			}
		})
	}
}

func TestJSONEditParseDelete(t *testing.T) {
	edit, err := parseJSONEdit(`del(.services["api.example.com"].labels.legacy)`)
	if err != nil {
		t.Fatalf("func parseJSONEdit failed: %+v", err)
	}
	expected := []string{"services", "api.example.com", "labels", "legacy"}
	if !reflect.DeepEqual(edit.keys, expected) {
		t.Errorf("unexpected keys: got %#v, want %#v", edit.keys, expected)
	}
	if edit.operation != jsonEditOperationDelete {
		t.Errorf("unexpected operation: got %d, want %d", edit.operation, jsonEditOperationDelete)
	}
}

func TestJSONEditParseDeleteInvalid(t *testing.T) {
	tests := []string{
		`del()`,
		`del(.server`,
		`del(.server) = null`,
		`del(.server) trailing`,
		`del(.servers[0])`,
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			if edit, err := parseJSONEdit(test); err == nil {
				t.Errorf("func parseJSONEdit unexpectedly succeeded: %#v", edit)
			}
		})
	}
}

func TestJSONEditGetStore(t *testing.T) {
	obj := &JSONEditRes{}
	obj.SetName("etcd:///from-name")
	if got, want := obj.getStore(), obj.Name(); got != want {
		t.Errorf("unexpected store: got %q, want %q", got, want)
	}

	obj.Store = "file:///from-store"
	if got, want := obj.getStore(), obj.Store; got != want {
		t.Errorf("unexpected store override: got %q, want %q", got, want)
	}
}

func TestJSONEditStoreParse(t *testing.T) {
	tests := []struct {
		name     string
		handle   string
		scheme   string
		location string
		fail     bool
	}{
		{
			name:     "etcd",
			handle:   "etcd:///foo/hosts",
			scheme:   jsonEditStoreSchemeEtcd,
			location: "foo/hosts",
		},
		{
			name:     "absolute file path",
			handle:   "/tmp/store.json",
			scheme:   jsonEditStoreSchemeFile,
			location: "/tmp/store.json",
		},
		{
			name:     "file URI",
			handle:   "file:///tmp/store.json",
			scheme:   jsonEditStoreSchemeFile,
			location: "/tmp/store.json",
		},
		{
			name:   "relative path",
			handle: "tmp/store.json",
			fail:   true,
		},
		{
			name:   "empty etcd key",
			handle: "etcd://",
			fail:   true,
		},
		{
			name:   "etcd query",
			handle: "etcd://?key=foo%2Fhosts",
			fail:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := parseJSONEditStore(test.handle)
			if test.fail {
				if err == nil {
					t.Fatalf("func parseJSONEditStore unexpectedly succeeded: %#v", store)
				}
				return
			}
			if err != nil {
				t.Fatalf("func parseJSONEditStore failed: %+v", err)
			}
			if store.scheme != test.scheme {
				t.Errorf("unexpected scheme: got %q, want %q", store.scheme, test.scheme)
			}
			if store.location != test.location {
				t.Errorf("unexpected location: got %q, want %q", store.location, test.location)
			}
		})
	}
}

func TestJSONEditValidateEmptyEdits(t *testing.T) {
	obj := &JSONEditRes{Store: "file:///tmp/store.json"}
	obj.SetName("test")
	if err := obj.Validate(); err == nil {
		t.Errorf("func Validate unexpectedly succeeded")
	}
}

func TestJSONEditFileCheckApply(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "store.json")
	initial := []byte("{\"server\":{\"old\":true},\"untouched\":1}\n")
	if err := os.WriteFile(filename, initial, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}
	if err := os.Chmod(filename, 0600); err != nil {
		t.Fatalf("could not set test file mode: %+v", err)
	}

	obj := &JSONEditRes{
		Edits: []string{`.server.state = "ready"`},
	}
	obj.SetName(filename)
	if err := obj.Validate(); err != nil {
		t.Fatalf("func Validate failed: %+v", err)
	}
	if err := obj.Init(&engine.Init{Logf: t.Logf}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checkOK, err := obj.CheckApply(ctx, true)
	if checkOK {
		t.Errorf("cancelled CheckApply returned true")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CheckApply returned unexpected error: %+v", err)
	}

	checkOK, err = obj.CheckApply(context.Background(), false)
	if err != nil {
		t.Fatalf("dry-run CheckApply failed: %+v", err)
	}
	if checkOK {
		t.Errorf("dry-run CheckApply returned true before convergence")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(data) != string(initial) {
		t.Fatalf("dry-run CheckApply changed the file: %s", data)
	}

	checkOK, err = obj.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("apply CheckApply failed: %+v", err)
	}
	if checkOK {
		t.Errorf("apply CheckApply returned true while changing the file")
	}

	data, err = os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	root, err := jsonEditDecode(data)
	if err != nil {
		t.Fatalf("could not decode edited file: %+v", err)
	}
	expected := map[string]interface{}{
		"server": map[string]interface{}{
			"old":   true,
			"state": "ready",
		},
		"untouched": json.Number("1"),
	}
	if !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected JSON document: got %#v, want %#v", root, expected)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("could not stat test file: %+v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Errorf("file mode changed: got %o, want %o", got, want)
	}

	checkOK, err = obj.CheckApply(context.Background(), false)
	if err != nil {
		t.Fatalf("converged CheckApply failed: %+v", err)
	}
	if !checkOK {
		t.Errorf("converged CheckApply returned false")
	}
}

func TestJSONEditFileMultipleCheckApply(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "store.json")
	initial := []byte("{\"service\":{\"labels\":{\"legacy\":true},\"state\":\"pending\"},\"generation\":7}\n")
	if err := os.WriteFile(filename, initial, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	obj := &JSONEditRes{
		Store: (&url.URL{Scheme: jsonEditStoreSchemeFile, Path: filename}).String(),
		Edits: []string{
			`.service.state = "ready"`,
			`.service.labels.owner = "purple"`,
			`del(.service.labels.legacy)`,
		},
	}
	obj.SetName("test")
	if err := obj.Validate(); err != nil {
		t.Fatalf("func Validate failed: %+v", err)
	}
	if err := obj.Init(&engine.Init{Logf: t.Logf}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	checkOK, err := obj.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply failed: %+v", err)
	}
	if checkOK {
		t.Errorf("func CheckApply returned true while changing the file")
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	root, err := jsonEditDecode(data)
	if err != nil {
		t.Fatalf("could not decode edited file: %+v", err)
	}
	expected := map[string]interface{}{
		"service": map[string]interface{}{
			"labels": map[string]interface{}{
				"owner": "purple",
			},
			"state": "ready",
		},
		"generation": json.Number("7"),
	}
	if !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected JSON document: got %#v, want %#v", root, expected)
	}

	checkOK, err = obj.CheckApply(context.Background(), false)
	if err != nil {
		t.Fatalf("converged CheckApply failed: %+v", err)
	}
	if !checkOK {
		t.Errorf("converged CheckApply returned false")
	}
}

func TestJSONEditFileMultipleNetNoChange(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "store.json")
	initial := []byte("{ \"service\": { \"state\": \"ready\" } }\n")
	if err := os.WriteFile(filename, initial, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	obj := &JSONEditRes{
		Store: (&url.URL{Scheme: jsonEditStoreSchemeFile, Path: filename}).String(),
		Edits: []string{
			`.service.temporary = true`,
			`del(.service.temporary)`,
		},
	}
	obj.SetName("test")
	if err := obj.Init(&engine.Init{Logf: t.Logf}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	checkOK, err := obj.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply failed: %+v", err)
	}
	if !checkOK {
		t.Errorf("net-no-change CheckApply returned false")
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(data) != string(initial) {
		t.Fatalf("net-no-change batch rewrote the file: %s", data)
	}
}

func TestJSONEditFileMultipleAtomicError(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "store.json")
	initial := []byte("{\"service\":{\"state\":\"pending\"}}\n")
	if err := os.WriteFile(filename, initial, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	obj := &JSONEditRes{
		Store: (&url.URL{Scheme: jsonEditStoreSchemeFile, Path: filename}).String(),
		Edits: []string{
			`.service.state = "ready"`,
			`.service.state.value = true`,
		},
	}
	obj.SetName("test")
	if err := obj.Init(&engine.Init{Logf: t.Logf}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	if checkOK, err := obj.CheckApply(context.Background(), true); err == nil {
		t.Errorf("func CheckApply unexpectedly succeeded with checkOK=%t", checkOK)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(data) != string(initial) {
		t.Fatalf("failed batch changed the file: %s", data)
	}
}

func TestJSONEditFileDeleteCheckApply(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "store.json")
	initial := []byte("{\"server\":{\"keep\":true,\"legacy\":\"remove\"},\"untouched\":1}\n")
	if err := os.WriteFile(filename, initial, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	obj := &JSONEditRes{
		Store: (&url.URL{Scheme: jsonEditStoreSchemeFile, Path: filename}).String(),
		Edits: []string{`del(.server.legacy)`},
	}
	obj.SetName("test")
	if err := obj.Validate(); err != nil {
		t.Fatalf("func Validate failed: %+v", err)
	}
	if err := obj.Init(&engine.Init{Logf: t.Logf}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	checkOK, err := obj.CheckApply(context.Background(), false)
	if err != nil {
		t.Fatalf("dry-run CheckApply failed: %+v", err)
	}
	if checkOK {
		t.Errorf("dry-run CheckApply returned true before convergence")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(data) != string(initial) {
		t.Fatalf("dry-run CheckApply changed the file: %s", data)
	}

	checkOK, err = obj.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("apply CheckApply failed: %+v", err)
	}
	if checkOK {
		t.Errorf("apply CheckApply returned true while changing the file")
	}

	data, err = os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	root, err := jsonEditDecode(data)
	if err != nil {
		t.Fatalf("could not decode edited file: %+v", err)
	}
	expected := map[string]interface{}{
		"server": map[string]interface{}{
			"keep": true,
		},
		"untouched": json.Number("1"),
	}
	if !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected JSON document: got %#v, want %#v", root, expected)
	}

	checkOK, err = obj.CheckApply(context.Background(), false)
	if err != nil {
		t.Fatalf("converged CheckApply failed: %+v", err)
	}
	if !checkOK {
		t.Errorf("converged CheckApply returned false")
	}

	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	obj.Edits = []string{`del(.server.missing.value)`}
	if err := obj.Init(&engine.Init{Logf: t.Logf}); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}
	checkOK, err = obj.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("missing-path CheckApply failed: %+v", err)
	}
	if !checkOK {
		t.Errorf("missing-path CheckApply returned false")
	}
	after, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("missing-path CheckApply changed the file: %s", after)
	}
}
