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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
)

// serverResp is one scripted response from the test server.
type serverResp struct {
	status       int               // 0 -> 200
	body         string            // raw bytes; binary is fine
	headers      map[string]string // extra response headers
	lastModified time.Time         // zero -> no Last-Modified header
}

// testServer is an httptest.Server whose handler blocks on each request,
// pushing the *http.Request onto `requests` and waiting on `responses` for the
// test driver to hand back what to reply with. This gives us exact control over
// long poll timing.
type testServer struct {
	*httptest.Server
	requests  chan *http.Request
	responses chan serverResp
	done      chan struct{}
}

func newTestServer() *testServer {
	obj := &testServer{
		requests:  make(chan *http.Request),
		responses: make(chan serverResp),
		done:      make(chan struct{}),
	}
	obj.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case obj.requests <- r:
		case <-obj.done:
			return
		}
		var resp serverResp
		select {
		case resp = <-obj.responses:
		case <-obj.done:
			return
		}
		for k, v := range resp.headers {
			w.Header().Set(k, v)
		}
		if !resp.lastModified.IsZero() {
			w.Header().Set("Last-Modified", resp.lastModified.UTC().Format(http.TimeFormat))
		}
		if resp.status == 0 {
			resp.status = http.StatusOK
		}
		w.WriteHeader(resp.status)
		// Don't write a body for 304-- net/http will drop it anyway,
		// but skipping it makes intent explicit.
		if resp.status != http.StatusNotModified {
			w.Write([]byte(resp.body))
		}
	}))
	return obj
}

// Close unblocks any in-flight handler and tears down the server.
func (obj *testServer) Close() {
	close(obj.done)
	obj.Server.Close()
}

// fakeInit builds an *engine.Init suitable for driving a resource in tests,
// with channels exposing the side effects (Event firings, Send payloads) and an
// atomic toggle for Refresh().
type fakeInit struct {
	init    *engine.Init
	events  chan struct{}
	sent    chan *HTTPClientSends
	refresh atomic.Bool
}

func newFakeInit(t *testing.T) *fakeInit {
	t.Helper()
	tmpdir := t.TempDir()
	fi := &fakeInit{
		events: make(chan struct{}, 8),
		sent:   make(chan *HTTPClientSends, 8),
	}
	fi.init = &engine.Init{
		VarDir: func(p string) (string, error) {
			return filepath.Join(tmpdir, p), nil
		},
		Event: func(ctx context.Context) error {
			select {
			case fi.events <- struct{}{}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Refresh: func() bool { return fi.refresh.Load() },
		Send: func(v interface{}) error {
			s, ok := v.(*HTTPClientSends)
			if !ok {
				return fmt.Errorf("unexpected Send payload %T", v)
			}
			// Copy so callers can't observe later mutation.
			cp := *s
			if s.Content != nil {
				c := *s.Content
				cp.Content = &c
			}
			fi.sent <- &cp
			return nil
		},
		Debug: testing.Verbose(),
		Logf:  func(format string, v ...interface{}) { t.Logf("res: "+format, v...) },
	}
	fi.init.Local = (&local.API{
		Prefix: filepath.Join(tmpdir, "local"),
		Debug:  testing.Verbose(),
		Logf:   func(format string, v ...interface{}) { t.Logf("local: "+format, v...) },
	}).Init()
	return fi
}

// drainSent pulls a Send payload non-blockingly. Returns nil if none pending.
func (obj *fakeInit) drainSent() *HTTPClientSends {
	select {
	case s := <-obj.sent:
		return s
	default:
		return nil
	}
}

// step is one row in a long poll table-driven test. Each step represents
// exactly one server round-trip and one corresponding CheckApply.
type step struct {
	// What the server returns for the next incoming request.
	serve serverResp

	// Headers we assert are present on the incoming request.
	wantReqHeader map[string]string

	// CheckApply expectations.
	wantOK        bool   // CheckApply's bool return
	wantErrSubstr string // non-empty -> expect this substring in CheckApply's error

	// On-disk and Send expectations.
	wantFile *string // nil = don't check
	wantSent *string // nil = don't check (use noSend to assert no Send happened)
	noSend   bool    // assert that CheckApply did NOT Send anything
}

func TestHTTPClientLongpoll(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	binary := string([]byte{0x00, 0x01, 0xff, 0xfe, 0xde, 0xad, 0xbe, 0xef})

	tests := []struct {
		name  string
		setup func(*HTTPClientRes)
		steps []step
	}{
		{
			name: "happy path: initial fetch then one update",
			steps: []step{
				{serve: serverResp{body: "v1"}, wantFile: strp("v1"), wantSent: strp("v1")},
				{serve: serverResp{body: "v2"}, wantFile: strp("v2"), wantSent: strp("v2")},
			},
		},
		{
			name: "conditional: only requests after initial fetch wait",
			setup: func(r *HTTPClientRes) {
				r.LongpollConditional = true
			},
			steps: []step{
				{
					serve:         serverResp{body: "v1"},
					wantReqHeader: map[string]string{"Prefer": ""},
					wantFile:      strp("v1"),
					wantSent:      strp("v1"),
				},
				{
					serve:         serverResp{body: "v2"},
					wantReqHeader: map[string]string{"Prefer": "wait=60"},
					wantFile:      strp("v2"),
					wantSent:      strp("v2"),
				},
				{
					serve:         serverResp{body: "v3"},
					wantReqHeader: map[string]string{"Prefer": "wait=60"},
					wantFile:      strp("v3"),
					wantSent:      strp("v3"),
				},
			},
		},
		{
			name:  "MtimeCheck: server returns 304, file is preserved and sent",
			setup: func(r *HTTPClientRes) { r.MtimeCheck = true },
			steps: []step{
				{serve: serverResp{body: "v1", lastModified: t0}, wantFile: strp("v1"), wantSent: strp("v1")},
				{
					serve:         serverResp{status: http.StatusNotModified, lastModified: t0},
					wantReqHeader: map[string]string{"If-Modified-Since": t0.UTC().Format(http.TimeFormat)},
					wantOK:        true,
					wantFile:      strp("v1"),
					wantSent:      strp("v1"),
				},
			},
		},
		{
			name:  "sha256 mismatch: CheckApply errors, Watch keeps going, recovery succeeds",
			setup: func(r *HTTPClientRes) { r.Sha256 = sha256Hex("v1") },
			steps: []step{
				{serve: serverResp{body: "v1"}, wantFile: strp("v1"), wantSent: strp("v1")},
				{
					serve:         serverResp{body: "junk"},
					wantErrSubstr: "sha256 mismatch",
					wantFile:      strp("v1"), // dst untouched
					noSend:        true,
				},
				{serve: serverResp{body: "v1"}, wantOK: true, wantFile: strp("v1"), wantSent: strp("v1")},
			},
		},
		{
			name: "non-200 status errors but Watch continues",
			steps: []step{
				{serve: serverResp{body: "v1"}, wantFile: strp("v1"), wantSent: strp("v1")},
				{
					serve:         serverResp{status: http.StatusInternalServerError, body: "boom"},
					wantErrSubstr: "unexpected status",
					wantFile:      strp("v1"),
					noSend:        true,
				},
				{serve: serverResp{body: "v2"}, wantFile: strp("v2"), wantSent: strp("v2")},
			},
		},
		{
			name: "binary payload round-trips through Send and file",
			steps: []step{
				{serve: serverResp{body: binary}, wantFile: strp(binary), wantSent: strp(binary)},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runLongpollSteps(t, tc.setup, tc.steps)
		})
	}
}

// runLongpollSteps is the shared driver for the long poll table. It wires up
// the server + resource, starts Watch, drains the readiness event, then
// executes each step as: (await Watch's request) -> (assert headers) -> (send
// scripted response) -> (await data event) -> (CheckApply) -> (assert).
func runLongpollSteps(t *testing.T, setup func(*HTTPClientRes), steps []step) {
	t.Helper()

	srv := newTestServer()
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "dst")
	fi := newFakeInit(t)

	res := &HTTPClientRes{
		URL:      srv.URL,
		File:     dst,
		Longpoll: true,
	}
	res.SetName("client")
	if setup != nil {
		setup(res)
	}

	if err := res.Init(fi.init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer res.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchDone := make(chan error, 1)
	go func() { watchDone <- res.Watch(ctx) }()

	// Drain the initial readiness Event.
	select {
	case <-fi.events:
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch never fired the initial readiness Event")
	}

	for i, s := range steps {
		// Wait for Watch's long poll request to arrive.
		var req *http.Request
		select {
		case req = <-srv.requests:
		case <-time.After(2 * time.Second):
			t.Fatalf("step %d: timed out waiting for server request", i)
		}
		for k, want := range s.wantReqHeader {
			if got := req.Header.Get(k); got != want {
				t.Errorf("step %d: header %q = %q, want %q", i, k, got, want)
			}
		}

		// Release the scripted response.
		srv.responses <- s.serve

		waitForPendingResponse(t, res, i)

		// CheckApply will pull from respCh, process, ack Watch.
		ok, err := res.CheckApply(ctx, true)

		if s.wantErrSubstr != "" {
			if err == nil || !strings.Contains(err.Error(), s.wantErrSubstr) {
				t.Errorf("step %d: want error containing %q, got %v", i, s.wantErrSubstr, err)
			}
		} else if err != nil {
			t.Errorf("step %d: unexpected error: %v", i, err)
		}
		if ok != s.wantOK {
			t.Errorf("step %d: ok = %v, want %v", i, ok, s.wantOK)
		}

		if s.wantFile != nil {
			b, readErr := os.ReadFile(dst)
			if readErr != nil {
				t.Errorf("step %d: reading %s: %v", i, dst, readErr)
			} else if string(b) != *s.wantFile {
				t.Errorf("step %d: file = %q, want %q", i, b, *s.wantFile)
			}
		}

		got := fi.drainSent()
		switch {
		case s.noSend:
			if got != nil {
				t.Errorf("step %d: expected no Send, got %q", i, derefStr(got.Content))
			}
		case s.wantSent != nil:
			if got == nil {
				t.Errorf("step %d: expected Send %q, none happened", i, *s.wantSent)
			} else if derefStr(got.Content) != *s.wantSent {
				t.Errorf("step %d: sent = %q, want %q", i, derefStr(got.Content), *s.wantSent)
			}
		}
	}

	cancel()
	select {
	case err := <-watchDone:
		if err != nil && err != context.Canceled {
			t.Errorf("func Watch returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("func Watch did not exit after ctx cancel")
	}
}

// TestHTTPClientLongpollCheckApplyGoodStateSkipsRequest verifies the common
// case where a long poll CheckApply runs while Watch is still blocked in a poll
// (so nothing is waiting on respCh) and the destination is already in the
// expected state (its contents match Sha256). It must make no HTTP request of
// its own, re-send the cached content, and report converged.
func TestHTTPClientLongpollCheckApplyGoodStateSkipsRequest(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(dst, []byte("v1"), 0600); err != nil {
		t.Fatalf("seeding dst: %v", err)
	}

	fi := newFakeInit(t)
	res := &HTTPClientRes{
		URL:      srv.URL,
		File:     dst,
		Longpoll: true,
		Sha256:   sha256Hex("v1"),
	}
	res.SetName("client")
	if err := res.Init(fi.init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer res.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ok, err := res.CheckApply(ctx, true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if !ok {
		t.Fatal("func CheckApply ok = false, want true (dst already matches Sha256)")
	}

	// No request should have been issued, since the on-disk state is good.
	select {
	case <-srv.requests:
		t.Fatal("func CheckApply issued an HTTP request despite a good local state")
	case <-time.After(100 * time.Millisecond):
	}

	// The cached content should still be sent downstream.
	if got := fi.drainSent(); got == nil || derefStr(got.Content) != "v1" {
		t.Errorf("sent = %v, want %q", got, "v1")
	}
}

// TestHTTPClientLongpollCheckApplyFetchesWhenNotConverged verifies the rarer
// case where a long poll CheckApply runs with nothing waiting on respCh AND the
// destination is not in the expected state. With no live response from Watch to
// consume, it must fall back to fetching the body itself rather than doing
// nothing, since it can't otherwise reach the correct state.
func TestHTTPClientLongpollCheckApplyFetchesWhenNotConverged(t *testing.T) {
	tests := []struct {
		name   string
		sha256 string
		seed   *string // pre-existing dst contents; nil means the file is absent
	}{
		{
			name: "destination file missing",
		},
		{
			name:   "destination has wrong sha256",
			sha256: sha256Hex("v1"),
			seed:   strp("junk"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer()
			defer srv.Close()

			dst := filepath.Join(t.TempDir(), "dst")
			if tc.seed != nil {
				if err := os.WriteFile(dst, []byte(*tc.seed), 0600); err != nil {
					t.Fatalf("seeding dst: %v", err)
				}
			}

			fi := newFakeInit(t)
			res := &HTTPClientRes{
				URL:      srv.URL,
				File:     dst,
				Longpoll: true,
				Sha256:   tc.sha256,
			}
			res.SetName("client")
			if err := res.Init(fi.init); err != nil {
				t.Fatalf("func Init: %v", err)
			}
			defer res.Cleanup()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Nothing is waiting on respCh and the state is not good,
			// so CheckApply must make its own request to recover.
			done := make(chan struct{})
			go func() {
				defer close(done)
				ok, err := res.CheckApply(ctx, true)
				if err != nil {
					t.Errorf("func CheckApply: %v", err)
				}
				if ok {
					t.Errorf("func CheckApply ok = true, want false (state changed)")
				}
			}()

			select {
			case <-srv.requests:
			case <-time.After(2 * time.Second):
				t.Fatal("func CheckApply did not make its own request")
			}
			srv.responses <- serverResp{body: "v1"}
			<-done

			b, err := os.ReadFile(dst)
			if err != nil {
				t.Fatalf("reading dst: %v", err)
			}
			if string(b) != "v1" {
				t.Errorf("file = %q, want %q", b, "v1")
			}
			if got := fi.drainSent(); got == nil || derefStr(got.Content) != "v1" {
				t.Errorf("sent = %v, want %q", got, "v1")
			}
		})
	}
}

// TestHTTPClientPollRefreshesExistingFile verifies that poll mode contacts the
// server even when the destination file already exists. The local file mtime is
// only a conditional request cursor, not a reason to skip the request entirely.
func TestHTTPClientPollRefreshesExistingFile(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "dst")
	fi := newFakeInit(t)
	t0 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC)

	res := &HTTPClientRes{
		URL:        srv.URL,
		File:       dst,
		MtimeCheck: true,
	}
	res.SetName("client")
	res.MetaParams().Poll = -1
	if err := res.Init(fi.init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer res.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Seed: first CheckApply downloads "v1".
	done := make(chan struct{})
	go func() {
		defer close(done)
		ok, err := res.CheckApply(ctx, true)
		if err != nil {
			t.Errorf("first CheckApply: %v", err)
		}
		if ok {
			t.Errorf("first CheckApply ok = true, want false (state changed)")
		}
	}()
	select {
	case <-srv.requests:
	case <-time.After(2 * time.Second):
		t.Fatal("first CheckApply did not contact the server")
	}
	srv.responses <- serverResp{body: "v1", lastModified: t0}
	<-done

	if got := fi.drainSent(); got == nil || derefStr(got.Content) != "v1" {
		t.Errorf("first CheckApply: sent = %v, want %q", got, "v1")
	}

	// A second poll should still contact the server. The mtime of the
	// existing destination file should be sent as If-Modified-Since so the
	// server can decide whether there is anything new.
	done = make(chan struct{})
	go func() {
		defer close(done)
		ok, err := res.CheckApply(ctx, true)
		if err != nil {
			t.Errorf("second CheckApply: %v", err)
		}
		if ok {
			t.Errorf("second CheckApply ok = true, want false (state changed)")
		}
	}()
	select {
	case req := <-srv.requests:
		if got, want := req.Header.Get("If-Modified-Since"), t0.UTC().Format(http.TimeFormat); got != want {
			t.Errorf("header If-Modified-Since = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second CheckApply did not contact the server")
	}
	srv.responses <- serverResp{body: "v2", lastModified: t1}
	<-done

	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(b) != "v2" {
		t.Errorf("file = %q, want %q", b, "v2")
	}
	if got := fi.drainSent(); got == nil || derefStr(got.Content) != "v2" {
		t.Errorf("second CheckApply: sent = %v, want %q", got, "v2")
	}
}

func TestHTTPClientLongpollRetriesAfterServerRequest(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "dst")
	fi := newFakeInit(t)
	res := &HTTPClientRes{
		URL:      srv.URL,
		File:     dst,
		Longpoll: true,
	}
	res.SetName("client")
	if err := res.Init(fi.init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer res.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchDone := make(chan error, 1)
	go func() { watchDone <- res.Watch(ctx) }()

	select {
	case <-fi.events:
	case err := <-watchDone:
		t.Fatalf("func Watch exited before startup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch never fired the initial readiness Event")
	}

	select {
	case <-srv.requests:
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch never made initial long poll request")
	}
	srv.responses <- serverResp{
		status: http.StatusServiceUnavailable,
		headers: map[string]string{
			"Retry-After": "0",
		},
		body: "server is restarting",
	}

	select {
	case <-fi.events:
		t.Fatal("func Watch fired an event for reconnect response")
	case err := <-watchDone:
		t.Fatalf("func Watch exited after reconnect response: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case <-srv.requests:
	case err := <-watchDone:
		t.Fatalf("func Watch exited before reconnecting: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not reconnect after Retry-After")
	}
	srv.responses <- serverResp{body: "v1"}

	select {
	case <-fi.events:
	case err := <-watchDone:
		t.Fatalf("func Watch exited after reconnect: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not fire event for real response")
	}

	ok, err := res.CheckApply(ctx, true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if ok {
		t.Fatalf("func CheckApply ok = true, want false")
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("func ReadFile: %v", err)
	}
	if string(b) != "v1" {
		t.Fatalf("file = %q, want %q", b, "v1")
	}

	cancel()
	select {
	case err := <-watchDone:
		if err != nil && err != context.Canceled {
			t.Fatalf("func Watch returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not stop after cancellation")
	}
}

func TestHTTPClientLongpollRetryAfterGraceCoversTransportFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net Listen: %v", err)
	}
	addr := ln.Addr().String()

	firstReq := make(chan struct{}, 1)
	firstRespSent := make(chan struct{}, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case firstReq <- struct{}{}:
			default:
			}
			w.Header().Set("Retry-After", "0")
			http.Error(w, "server is restarting", http.StatusServiceUnavailable)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case firstRespSent <- struct{}{}:
			default:
			}
		}),
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(ln) }()

	dst := filepath.Join(t.TempDir(), "dst")
	fi := newFakeInit(t)
	res := &HTTPClientRes{
		URL:      "http://" + addr + "/event",
		File:     dst,
		Longpoll: true,
	}
	res.SetName("client")
	if err := res.Init(fi.init); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	defer res.Cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchDone := make(chan error, 1)
	go func() { watchDone <- res.Watch(ctx) }()

	select {
	case <-fi.events:
	case err := <-watchDone:
		t.Fatalf("func Watch exited before startup: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch never fired the initial readiness Event")
	}

	select {
	case <-firstReq:
	case err := <-watchDone:
		t.Fatalf("func Watch exited before Retry-After response: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch never made initial long poll request")
	}

	select {
	case <-firstRespSent:
	case err := <-watchDone:
		t.Fatalf("func Watch exited before receiving Retry-After response: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not send Retry-After response")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("first server Close: %v", err)
	}
	<-serverDone

	select {
	case <-fi.events:
		t.Fatal("func Watch fired an event for Retry-After response")
	case err := <-watchDone:
		t.Fatalf("func Watch exited during Retry-After grace period: %v", err)
	case <-time.After(200 * time.Millisecond):
		// The client has had time to reconnect once and hit a transport error.
	}

	ln, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("second Listen: %v", err)
	}
	server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("v1"))
		}),
	}
	serverDone = make(chan error, 1)
	go func() { serverDone <- server.Serve(ln) }()
	defer func() {
		server.Close()
		<-serverDone
	}()

	select {
	case <-fi.events:
	case err := <-watchDone:
		t.Fatalf("func Watch exited after server returned: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("func Watch did not reconnect during Retry-After grace period")
	}

	ok, err := res.CheckApply(ctx, true)
	if err != nil {
		t.Fatalf("func CheckApply: %v", err)
	}
	if ok {
		t.Fatalf("func CheckApply ok = true, want false")
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("func ReadFile: %v", err)
	}
	if string(b) != "v1" {
		t.Fatalf("file = %q, want %q", b, "v1")
	}

	cancel()
	select {
	case err := <-watchDone:
		if err != nil && err != context.Canceled {
			t.Fatalf("func Watch returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("func Watch did not stop after cancellation")
	}
}

func strp(s string) *string { return &s }

func waitForPendingResponse(t *testing.T, res *HTTPClientRes, step int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(2 * time.Second)

	for {
		if len(res.respCh) > 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-timeout:
			t.Fatalf("step %d: timed out waiting for long poll response", step)
		}
	}
}

func derefStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
