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

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
)

// testRecordRes returns an initialized record resource with a file store.
func testRecordRes(t *testing.T, init *engine.Init, store, state string) *HTTPServerRecordRes {
	t.Helper()
	obj := &HTTPServerRecordRes{
		Store:  store,
		State:  state,
		Key:    "mac",
		Fields: []string{"ip", "hostkey"},
	}
	obj.SetKind(httpServerRecordKind)
	obj.SetName("/action/report")
	if err := obj.Validate(); err != nil {
		t.Fatalf("func Validate: %v", err)
	}
	if err := obj.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	return obj
}

// testRecordPost sends a form to the resource, and returns the status code.
func testRecordPost(obj *HTTPServerRecordRes, form url.Values) int {
	req := httptest.NewRequest(http.MethodPost, "/action/report", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	obj.ServeHTTP(rec, req)
	return rec.Code
}

// testRecordRead decodes the store file.
func testRecordRead(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("func ReadFile: %v", err)
	}
	root, err := engineUtil.DecodeJSONObject(data)
	if err != nil {
		t.Fatalf("func DecodeJSONObject: %v", err)
	}
	return root
}

// testInode returns the inode of a file, which changes when it's replaced.
func testInode(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("func Stat: %v", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("no inode numbers here")
	}
	return stat.Ino
}

func TestHTTPServerRecordValidate(t *testing.T) {
	valid := func() *HTTPServerRecordRes {
		obj := &HTTPServerRecordRes{
			Store:  "etcd:///records",
			Key:    "mac",
			Fields: []string{"ip"},
		}
		obj.SetName("/report")
		return obj
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("func Validate: %v", err)
	}

	tests := map[string]func(*HTTPServerRecordRes){
		"relative path":   func(obj *HTTPServerRecordRes) { obj.Path = "report" },
		"no store":        func(obj *HTTPServerRecordRes) { obj.Store = "" },
		"relative store":  func(obj *HTTPServerRecordRes) { obj.Store = "records.json" },
		"absent state":    func(obj *HTTPServerRecordRes) { obj.State = "absent" },
		"no key":          func(obj *HTTPServerRecordRes) { obj.Key = "" },
		"empty field":     func(obj *HTTPServerRecordRes) { obj.Fields = []string{"ip", ""} },
		"duplicate field": func(obj *HTTPServerRecordRes) { obj.Fields = []string{"ip", "ip"} },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			obj := valid()
			change(obj)
			if err := obj.Validate(); err == nil {
				t.Errorf("func Validate unexpectedly succeeded")
			}
		})
	}
}

func TestHTTPServerRecordState(t *testing.T) {
	ctx := context.Background()
	init := testJSONStoreInit(t)
	path := filepath.Join(t.TempDir(), "records.json")

	// Without the state, we don't make the store, and can't store anything.
	obj := testRecordRes(t, init, path, "")
	if checkOK, err := obj.CheckApply(ctx, true); err != nil || !checkOK {
		t.Errorf("func CheckApply: %t, %v", checkOK, err)
	}
	if code := testRecordPost(obj, url.Values{"mac": {"m1"}}); code != http.StatusInternalServerError {
		t.Errorf("unexpected status: %d", code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("store exists, or other error: %v", err)
	}

	// With it, we make the store, but not in a dry run.
	obj = testRecordRes(t, init, path, HTTPServerRecordStateExists)
	if checkOK, err := obj.CheckApply(ctx, false); err != nil || checkOK {
		t.Errorf("dry-run CheckApply: %t, %v", checkOK, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("store exists, or other error: %v", err)
	}
	if checkOK, err := obj.CheckApply(ctx, true); err != nil || checkOK {
		t.Errorf("func CheckApply: %t, %v", checkOK, err)
	}
	if root := testRecordRead(t, path); len(root) != 0 {
		t.Errorf("unexpected store: %v", root)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("func Stat: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Errorf("store mode: got %o, want %o", got, want)
	}
	if checkOK, err := obj.CheckApply(ctx, true); err != nil || !checkOK {
		t.Errorf("converged CheckApply: %t, %v", checkOK, err)
	}

	// The records stay when it already exists.
	if code := testRecordPost(obj, url.Values{"mac": {"m1"}}); code != http.StatusOK {
		t.Errorf("unexpected status: %d", code)
	}
	if checkOK, err := obj.CheckApply(ctx, true); err != nil || !checkOK {
		t.Errorf("converged CheckApply: %t, %v", checkOK, err)
	}
	if root := testRecordRead(t, path); len(root) != 1 {
		t.Errorf("unexpected store: %v", root)
	}
}

func TestHTTPServerRecordServe(t *testing.T) {
	ctx := context.Background()
	init := testJSONStoreInit(t)
	path := filepath.Join(t.TempDir(), "records.json")
	if err := os.WriteFile(path, []byte("{\"other\": {\"n\": 1.50}}\n"), 0600); err != nil {
		t.Fatalf("func WriteFile: %v", err)
	}

	//nolint:gosec // G302: We're testing Chmod functionality
	if err := os.Chmod(path, 0640); err != nil { // not limited by the umask
		t.Fatalf("func Chmod: %v", err)
	}
	obj := testRecordRes(t, init, path, "")

	if err := obj.AcceptHTTP(httptest.NewRequest(http.MethodGet, "/action/report", nil)); err == nil {
		t.Errorf("func AcceptHTTP accepted a GET")
	}
	if err := obj.AcceptHTTP(httptest.NewRequest(http.MethodPost, "/action/other", nil)); err == nil {
		t.Errorf("func AcceptHTTP accepted another path")
	}
	if err := obj.AcceptHTTP(httptest.NewRequest(http.MethodPost, "/action/report", nil)); err != nil {
		t.Errorf("func AcceptHTTP: %v", err)
	}

	rec := httptest.NewRecorder()
	obj.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/action/report", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("unexpected status for a GET: %d", rec.Code)
	}
	if code := testRecordPost(obj, url.Values{"ip": {"192.0.2.13"}}); code != http.StatusBadRequest {
		t.Errorf("unexpected status without the key: %d", code)
	}

	// Only the listed fields are stored, and only if they're sent.
	if code := testRecordPost(obj, url.Values{
		"mac":   {"00:11:22:33:44:55"},
		"ip":    {"192.0.2.13"},
		"extra": {"ignored"},
	}); code != http.StatusOK {
		t.Errorf("unexpected status: %d", code)
	}
	expected := map[string]interface{}{
		"other": map[string]interface{}{"n": json.Number("1.50")},
		"00:11:22:33:44:55": map[string]interface{}{
			"ip": "192.0.2.13",
		},
	}
	if root := testRecordRead(t, path); !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected store: got %#v, want %#v", root, expected)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("func Stat: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0640); got != want {
		t.Errorf("store mode: got %o, want %o", got, want)
	}

	// The same record again doesn't rewrite the store.
	inode := testInode(t, path)
	if code := testRecordPost(obj, url.Values{"mac": {"00:11:22:33:44:55"}, "ip": {"192.0.2.13"}}); code != http.StatusOK {
		t.Errorf("unexpected status: %d", code)
	}
	if testInode(t, path) != inode {
		t.Errorf("the same record rewrote the store")
	}

	// A new one replaces it entirely.
	if code := testRecordPost(obj, url.Values{"mac": {"00:11:22:33:44:55"}, "hostkey": {"AAAA"}}); code != http.StatusOK {
		t.Errorf("unexpected status: %d", code)
	}
	expected["00:11:22:33:44:55"] = map[string]interface{}{
		"hostkey": "AAAA",
	}
	if root := testRecordRead(t, path); !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected store: got %#v, want %#v", root, expected)
	}

	// A consumer removes it with json:edit.
	edit := &JSONEditRes{
		Store: path,
		Edits: []string{`del(.["00:11:22:33:44:55"])`},
	}
	edit.SetName("consumer")
	if err := edit.Validate(); err != nil {
		t.Fatalf("func Validate: %v", err)
	}
	if err := edit.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	if checkOK, err := edit.CheckApply(ctx, true); err != nil || checkOK {
		t.Errorf("func CheckApply: %t, %v", checkOK, err)
	}
	delete(expected, "00:11:22:33:44:55")
	if root := testRecordRead(t, path); !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected store: got %#v, want %#v", root, expected)
	}
}

// TestHTTPServerRecordConcurrent stores many records at once, while a json:edit
// resource which shares the store keeps on changing it, and nothing gets lost.
func TestHTTPServerRecordConcurrent(t *testing.T) {
	ctx := context.Background()
	init := testJSONStoreInit(t)
	path := filepath.Join(t.TempDir(), "records.json")
	obj := testRecordRes(t, init, path, HTTPServerRecordStateExists)
	if _, err := obj.CheckApply(ctx, true); err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}

	count := 50
	wg := &sync.WaitGroup{}
	errs := make(chan error, 2*count)
	for i := 0; i < count; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			mac := fmt.Sprintf("m%d", i)
			if code := testRecordPost(obj, url.Values{"mac": {mac}, "ip": {fmt.Sprintf("192.0.2.%d", i)}}); code != http.StatusOK {
				errs <- fmt.Errorf("unexpected status for %s: %d", mac, code)
			}
		}()
		go func() {
			defer wg.Done()
			edit := &JSONEditRes{
				Store: path,
				Edits: []string{fmt.Sprintf(`.counter["e%d"] = true`, i)},
			}
			edit.SetName(fmt.Sprintf("edit%d", i))
			if err := edit.Init(init); err != nil {
				errs <- err
				return
			}
			if _, err := edit.CheckApply(ctx, true); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	root := testRecordRead(t, path)
	counter, ok := root["counter"].(map[string]interface{})
	if !ok || len(counter) != count {
		t.Errorf("lost some edits: %v", root["counter"])
	}
	if len(root) != count+1 {
		t.Errorf("lost some records: got %d, want %d", len(root)-1, count)
	}
}

// TestHTTPServerRecordServer stores a record that comes in to a real server.
func TestHTTPServerRecordServer(t *testing.T) {
	init := testJSONStoreInit(t)
	events := make(chan struct{}, 4)
	init.Event = func(ctx context.Context) error {
		select {
		case events <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	init.Refresh = func() bool { return false }
	init.Send = func(interface{}) error { return nil }
	init.Recv = func() map[string]*engine.Send { return map[string]*engine.Send{} }
	path := filepath.Join(t.TempDir(), "records.json")

	record := &HTTPServerRecordRes{
		Store: path,
		State: HTTPServerRecordStateExists,
		Key:   "mac",
	}
	record.SetKind(httpServerRecordKind)
	record.SetName("/action/report")

	server := &HTTPServerRes{
		Address: "127.0.0.1:0",
	}
	server.SetKind(httpServerKind)
	server.SetName("server")
	if err := server.GroupCmp(record); err != nil {
		t.Fatalf("func GroupCmp: %v", err)
	}
	server.SetGroup([]engine.GroupableRes{record})

	if err := server.Init(init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer server.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Watch(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select { // the server is listening once it sends its startup event
	case <-events:
	case err := <-done:
		t.Fatalf("func Watch exited before startup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not report startup")
	}

	if _, err := server.CheckApply(ctx, true); err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}

	resp, err := http.PostForm(fmt.Sprintf("http://%s/action/report", server.conn.Addr()), url.Values{"mac": {"m1"}})
	if err != nil {
		t.Fatalf("func PostForm: %v", err)
	}
	resp.Body.Close() //nolint:gosec // G104: ignore for the test
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	expected := map[string]interface{}{
		"m1": map[string]interface{}{},
	}
	if root := testRecordRead(t, path); !reflect.DeepEqual(root, expected) {
		t.Errorf("unexpected store: got %#v, want %#v", root, expected)
	}
}
