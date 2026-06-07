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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

// watchErrorRes is a test-only groupable resource whose Watch fires its initial
// startup Event and then immediately returns an error. It's used to exercise
// how the http server reacts when one of its grouped children fails *after*
// startup, while a sibling child is still happily blocked in its own Watch.
type watchErrorRes struct {
	traits.Base
	traits.Groupable
	traits.Refreshable

	init *engine.Init

	// watchErr is returned by Watch. By default it is returned right after
	// the startup Event fires, unless beforeEvent is set.
	watchErr error

	// beforeEvent makes Watch return watchErr *before* it ever fires its
	// startup Event, modelling a child that fails to initialize its watch.
	beforeEvent bool
}

func (obj *watchErrorRes) Default() engine.Res { return &watchErrorRes{} }

func (obj *watchErrorRes) Validate() error { return nil }

func (obj *watchErrorRes) Init(init *engine.Init) error {
	obj.init = init
	return nil
}

func (obj *watchErrorRes) Cleanup() error { return nil }

func (obj *watchErrorRes) Watch(ctx context.Context) error {
	if obj.beforeEvent {
		return obj.watchErr // fail before ever firing a startup Event
	}
	if err := obj.init.Event(ctx); err != nil {
		return err
	}
	return obj.watchErr // fail after startup
}

func (obj *watchErrorRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	return true, nil
}

func (obj *watchErrorRes) Cmp(r engine.Res) error {
	if _, ok := r.(*watchErrorRes); !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	return nil
}

func (obj *watchErrorRes) UIDs() []engine.ResUID { return nil }

func (obj *watchErrorRes) GroupCmp(r engine.GroupableRes) error { return nil }

// TestHTTPServerChildWatchErrorDoesNotDeadlock verifies that when a grouped
// child's Watch fails after startup, the http server's Watch returns that error
// promptly instead of deadlocking. The server must cancel the (derived) context
// that its other grouped children are blocked on *before* it waits for them to
// exit, otherwise a sibling stuck in `<-ctx.Done()` will hang the WaitGroup.
func TestHTTPServerChildWatchErrorDoesNotDeadlock(t *testing.T) {
	events := make(chan struct{}, 4)
	init := &engine.Init{
		Event: func(ctx context.Context) error {
			select {
			case events <- struct{}{}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Refresh: func() bool { return false },
		Send:    func(interface{}) error { return nil },
		Recv:    func() map[string]*engine.Send { return map[string]*engine.Send{} },
		Debug:   testing.Verbose(),
		Logf:    func(format string, v ...interface{}) { t.Logf(format, v...) },
	}

	// The failing child errors right after startup...
	boom := fmt.Errorf("boom")
	errChild := &watchErrorRes{watchErr: boom}
	errChild.SetKind("noop")
	errChild.SetName("errchild")

	// ...while this sibling just blocks in its Watch on ctx, exactly the
	// resource we need ctx-cancelled in order to unblock during teardown.
	sibling := &NoopRes{}
	sibling.SetKind("noop")
	sibling.SetName("sibling")

	server := &HTTPServerRes{
		Address: "127.0.0.1:0",
	}
	server.SetKind(httpServerKind)
	server.SetName("server")
	server.SetGroup([]engine.GroupableRes{errChild, sibling})

	if err := server.Init(init); err != nil {
		t.Fatalf("server Init: %v", err)
	}
	defer server.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- server.Watch(ctx) }()

	// Drain the server's startup Event.
	select {
	case <-events:
	case err := <-done:
		t.Fatalf("func Watch exited before startup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not report startup")
	}

	// The child's failure must surface as a prompt Watch error, not a hang.
	select {
	case err := <-done:
		if err != boom {
			t.Fatalf("func Watch returned %v, want %v", err, boom)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("func Watch deadlocked after a grouped child's Watch errored")
	}
}

// TestHTTPServerChildWatchErrorBeforeStartupDoesNotDeadlock verifies that when
// a grouped child's Watch fails *before* ever firing its startup Event (e.g. a
// long poll file whose watcher couldn't be created), the http server's Watch
// surfaces that error instead of hanging forever in its per-child startup
// handshake.
func TestHTTPServerChildWatchErrorBeforeStartupDoesNotDeadlock(t *testing.T) {
	events := make(chan struct{}, 4)
	init := &engine.Init{
		Event: func(ctx context.Context) error {
			select {
			case events <- struct{}{}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Refresh: func() bool { return false },
		Send:    func(interface{}) error { return nil },
		Recv:    func() map[string]*engine.Send { return map[string]*engine.Send{} },
		Debug:   testing.Verbose(),
		Logf:    func(format string, v ...interface{}) { t.Logf(format, v...) },
	}

	boom := fmt.Errorf("boom")
	errChild := &watchErrorRes{watchErr: boom, beforeEvent: true}
	errChild.SetKind("noop")
	errChild.SetName("errchild")

	server := &HTTPServerRes{
		Address: "127.0.0.1:0",
	}
	server.SetKind(httpServerKind)
	server.SetName("server")
	server.SetGroup([]engine.GroupableRes{errChild})

	if err := server.Init(init); err != nil {
		t.Fatalf("server Init: %v", err)
	}
	defer server.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- server.Watch(ctx) }()

	select {
	case err := <-done:
		if err != boom {
			t.Fatalf("func Watch returned %v, want %v", err, boom)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("func Watch deadlocked when a grouped child errored before startup")
	}
}

func TestHTTPServerFileEmptyAndMissingContent(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		longpoll   bool
		wantStatus int
	}{
		{
			name:       "empty long poll inline file",
			longpoll:   true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing path",
			path:       filepath.Join(t.TempDir(), "missing"),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &HTTPServerFileRes{
				Path:     tc.path,
				Longpoll: tc.longpoll,
			}
			obj.SetName("/event")
			if err := obj.Init(&engine.Init{
				Logf: func(format string, v ...interface{}) {
					t.Logf(format, v...)
				},
			}); err != nil {
				t.Fatalf("func Init: %v", err)
			}
			defer obj.Cleanup()

			req := httptest.NewRequest(http.MethodGet, "/event", nil)
			rec := httptest.NewRecorder()

			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("func ServeHTTP panicked: %v", r)
					}
				}()
				obj.ServeHTTP(rec, req)
			}()

			resp := rec.Result()
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("func ReadAll: %v", err)
				}
				if len(body) != 0 {
					t.Errorf("body = %q, want empty", body)
				}
			}
		})
	}
}

func TestHTTPServerLongpollShutdownUnblocksHeldClient(t *testing.T) {
	events := make(chan struct{}, 4)
	init := &engine.Init{
		Event: func(ctx context.Context) error {
			select {
			case events <- struct{}{}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Refresh: func() bool { return false },
		Send:    func(interface{}) error { return nil },
		Recv:    func() map[string]*engine.Send { return map[string]*engine.Send{} },
		Debug:   testing.Verbose(),
		Logf:    func(format string, v ...interface{}) { t.Logf(format, v...) },
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "event")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatalf("func WriteFile: %v", err)
	}

	file := &HTTPServerFileRes{
		Path:     path,
		Longpoll: true,
	}
	file.SetKind(httpServerFileKind)
	file.SetName("/event")

	server := &HTTPServerRes{
		Address: "127.0.0.1:0",
	}
	server.SetKind(httpServerKind)
	server.SetName("server")
	server.SetGroup([]engine.GroupableRes{file})

	if err := server.Init(init); err != nil {
		t.Fatalf("server Init: %v", err)
	}
	defer server.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Watch(ctx)
	}()

	select {
	case <-events:
	case err := <-done:
		t.Fatalf("func Watch exited before startup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not report startup")
	}

	url := fmt.Sprintf("http://%s/event", server.conn.Addr())
	client := &http.Client{}
	resp := httpGetEventually(t, client, url)
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatalf("reading initial response: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("closing initial response: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("func NewRequest: %v", err)
	}
	req.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))

	type httpResult struct {
		status     int
		retryAfter string
		err        error
	}
	held := make(chan httpResult, 1)
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			held <- httpResult{err: err}
			return
		}
		io.Copy(io.Discard, resp.Body)
		held <- httpResult{
			status:     resp.StatusCode,
			retryAfter: resp.Header.Get("Retry-After"),
			err:        resp.Body.Close(),
		}
	}()

	select {
	case result := <-held:
		t.Fatalf("long poll request returned before shutdown: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("func Watch returned unexpected error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		if err := server.Interrupt(); err != nil {
			t.Logf("func Interrupt after shutdown timeout: %v", err)
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("func Watch stayed blocked even after Interrupt")
		}
		t.Fatal("func Watch did not return after context cancellation with held long poll client")
	}

	select {
	case result := <-held:
		if result.err != nil {
			t.Fatalf("held long poll request returned error: %v", result.err)
		}
		if result.status != http.StatusServiceUnavailable {
			t.Fatalf("held long poll status = %d, want %d", result.status, http.StatusServiceUnavailable)
		}
		if result.retryAfter == "" {
			t.Fatal("held long poll response did not include Retry-After")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("held long poll request did not unblock")
	}
}

func httpGetEventually(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		resp, err := client.Get(url)
		if err == nil {
			return resp
		}
		lastErr = err
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("http GET %s did not succeed: %v", url, lastErr)
		}
	}
}

// TestHTTPServerFileLongpollDataCursorAdvancesPerUpdate verifies that several
// long poll content updates landing within the same wall-clock second still
// advance the version cursor (mtime) by at least a whole second each. HTTP's
// Last-Modified / If-Modified-Since headers only carry one second resolution,
// so if two same-second updates shared a cursor a client that reconnected
// between them would never be told about the second one (a lost update).
func TestHTTPServerFileLongpollDataCursorAdvancesPerUpdate(t *testing.T) {
	recv := map[string]*engine.Send{"data": {Changed: true}}
	obj := &HTTPServerFileRes{
		Data:     "v1",
		Longpoll: true,
	}
	obj.SetName("/event")
	if err := obj.Init(&engine.Init{
		Recv:    func() map[string]*engine.Send { return recv },
		Refresh: func() bool { return false },
		Logf:    func(format string, v ...interface{}) { t.Logf(format, v...) },
	}); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer obj.Cleanup()

	ctx := context.Background()

	m0 := obj.mtime

	obj.Data = "v2"
	if _, err := obj.CheckApply(ctx, true); err != nil {
		t.Fatalf("func CheckApply v2: %v", err)
	}
	m1 := obj.mtime

	obj.Data = "v3"
	if _, err := obj.CheckApply(ctx, true); err != nil {
		t.Fatalf("func CheckApply v3: %v", err)
	}
	m2 := obj.mtime

	if !m1.After(m0) || m1.Sub(m0) < time.Second {
		t.Errorf("cursor did not advance a full second on first update: m0=%v m1=%v (delta %v)", m0, m1, m1.Sub(m0))
	}
	if !m2.After(m1) || m2.Sub(m1) < time.Second {
		t.Errorf("cursor did not advance a full second on second update: m1=%v m2=%v (delta %v)", m1, m2, m2.Sub(m1))
	}
}

// TestHTTPServerFileLongpollPathServesLogicalMtime verifies that a long poll
// file backed by a Path serves our monotonic version cursor as Last-Modified,
// not the file's own on-disk mtime. The on-disk mtime only has one second
// resolution once it round-trips through HTTP, so it can't safely be used as
// the long poll cursor.
func TestHTTPServerFileLongpollPathServesLogicalMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "event")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatalf("func WriteFile: %v", err)
	}

	obj := &HTTPServerFileRes{
		Path:     path,
		Longpoll: true,
	}
	obj.SetName("/event")
	if err := obj.Init(&engine.Init{
		Logf: func(format string, v ...interface{}) { t.Logf(format, v...) },
	}); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer obj.Cleanup()

	// Force a logical cursor that is clearly distinct from the file's real
	// mtime (which is ~now), so we can tell which one gets served.
	sentinel := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	obj.mutex.Lock()
	obj.mtime = sentinel
	obj.mutex.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/event", nil)
	rec := httptest.NewRecorder()
	obj.ServeHTTP(rec, req) // no If-Modified-Since -> serves immediately

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	got := resp.Header.Get("Last-Modified")
	want := sentinel.UTC().Format(http.TimeFormat)
	if got != want {
		t.Errorf("header Last-Modified = %q, want logical cursor %q (not the file mtime)", got, want)
	}
}
