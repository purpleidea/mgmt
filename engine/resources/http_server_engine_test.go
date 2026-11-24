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
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph"
	"github.com/purpleidea/mgmt/pgraph"
)

// newTestHTTPEngine builds a running converger and an initialized engine for a
// test. The returned cleanup stops the converger, and must run after the engine
// is shut down.
func newTestHTTPEngine(t *testing.T) (*graph.Engine, func()) {
	t.Helper()

	// Lingering goroutines can log after the test ends, which t.Logf panics
	// on, so we stop logging once the test is over.
	logMutex := &sync.Mutex{}
	logDone := false
	t.Cleanup(func() {
		logMutex.Lock()
		logDone = true
		logMutex.Unlock()
	})
	logf := func(format string, v ...interface{}) {
		logMutex.Lock()
		defer logMutex.Unlock()
		if logDone || !testing.Verbose() {
			return
		}
		t.Logf("test: "+format, v...)
	}

	conv := &converger.Coordinator{
		Timeout: -1, // disabled
		Logf: func(format string, v ...interface{}) {
			logf("converger: "+format, v...)
		},
	}
	if err := conv.Init(); err != nil {
		t.Fatalf("converger Init: %v", err)
	}
	convCtx, convCancel := context.WithCancel(context.Background())
	convWg := &sync.WaitGroup{}
	convWg.Add(1)
	go func() {
		defer convWg.Done()
		_ = conv.Run(convCtx, false) // errors on context cancel
	}()
	cleanup := func() {
		convCancel()
		convWg.Wait()
	}

	ge := &graph.Engine{
		Program:   "mgmt",
		Version:   "0.0.1",
		Hostname:  "localhost",
		Converger: conv,
		Prefix:    t.TempDir(),
		Logf:      logf,
		Cancel: func(err error) {
			t.Errorf("the engine was cancelled: %v", err)
		},
	}
	if err := ge.Init(); err != nil {
		cleanup()
		t.Fatalf("engine Init: %v", err)
	}

	return ge, cleanup
}

// swapTestHTTPGraph loads, validates, pauses, commits, and resumes a graph.
func swapTestHTTPGraph(ge *graph.Engine, g *pgraph.Graph) error {
	if err := ge.Load(g); err != nil {
		return fmt.Errorf("engine Load: %v", err)
	}
	if err := ge.Validate(); err != nil {
		return fmt.Errorf("engine Validate: %v", err)
	}
	if err := ge.Pause(); err != nil { // see the main loop in lib
		return fmt.Errorf("engine Pause: %v", err)
	}
	if err := ge.Commit(context.Background()); err != nil {
		return fmt.Errorf("engine Commit: %v", err)
	}
	if err := ge.Resume(); err != nil {
		return fmt.Errorf("engine Resume: %v", err)
	}
	return nil
}

// TestHTTPServerEngineSwapKeepsSocket runs an http:server in the engine, and
// then swaps it for one with the same address but different files, which
// replaces the resource. A request which connects while the old one is still
// draining waits, and is served by the new one, instead of being refused. Once
// there's no http:server with that address in the graph, the socket is closed.
func TestHTTPServerEngineSwapKeepsSocket(t *testing.T) {
	ge, cleanup := newTestHTTPEngine(t)
	defer cleanup()

	address := testFreeAddress(t)
	url := "http://" + address

	newFile := func(name, data string, longpoll bool) *HTTPServerFileRes {
		file := &HTTPServerFileRes{
			Data:     data,
			Longpoll: longpoll,
		}
		file.SetKind(httpServerFileKind)
		file.SetName(name)
		return file
	}
	newGraph := func(name string, files ...*HTTPServerFileRes) *pgraph.Graph {
		timeout := uint64(1) // the stalled client holds up the drain this long
		server := &HTTPServerRes{
			Address:         address,
			ShutdownTimeout: &timeout,
		}
		server.SetKind(httpServerKind)
		server.SetName("server")
		group := []engine.GroupableRes{}
		for _, file := range files {
			group = append(group, file)
		}
		server.SetGroup(group)

		g, err := pgraph.NewGraph(name)
		if err != nil {
			t.Fatalf("pgraph NewGraph: %v", err)
		}
		g.AddVertex(server)
		return g
	}

	big := strings.Repeat("x", 32*1024*1024)
	if err := swapTestHTTPGraph(ge, newGraph("first",
		newFile("/f", "one", false),
		newFile("/big", big, false),
		newFile("/lp", "old", true),
	)); err != nil {
		t.Fatalf("first graph: %v", err)
	}
	running := true
	defer func() {
		if !running {
			return
		}
		_ = ge.Pause() // errors if we failed while paused, that's okay
		if err := ge.Shutdown(); err != nil {
			t.Errorf("engine Shutdown: %v", err)
		}
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	resp := httpGetEventually(t, client, url+"/f")
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(body) != "one" {
		t.Fatalf("first server got: %q %v", body, err)
	}

	// This stops reading part way through, which holds up the drain of the
	// old server until its ShutdownTimeout cuts it.
	stalled := testStalledClient(t, &testProxy{url: url}, "/big")
	defer stalled.Close()

	// The old server lets go of this long poll as its drain starts, which
	// tells us when that is. It asks us to reconnect at once, which we do
	// like the http:client would, and the new server must answer that. Any
	// version is newer than the one we ask for then, so it's served right
	// away, and its content tells us which server that was.
	type longpollResult struct {
		status     int
		retryAfter string
		body       string
		err        error
	}
	longpoll := make(chan *longpollResult, 2)
	go func() {
		lpClient := &http.Client{}
		defer lpClient.CloseIdleConnections()
		get := func(since time.Time) *longpollResult {
			req, err := http.NewRequest(http.MethodGet, url+"/lp", nil)
			if err != nil {
				return &longpollResult{err: err}
			}
			req.Header.Set("If-Modified-Since", since.UTC().Format(http.TimeFormat))
			resp, err := lpClient.Do(req)
			if err != nil {
				return &longpollResult{err: err}
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			return &longpollResult{
				status:     resp.StatusCode,
				retryAfter: resp.Header.Get("Retry-After"),
				body:       string(body),
				err:        err,
			}
		}
		first := get(time.Now().Add(time.Hour)) // nothing is newer
		longpoll <- first
		if first.err == nil && first.status == http.StatusServiceUnavailable {
			longpoll <- get(time.Unix(0, 0)) // anything is newer
		}
	}()
	select {
	case result := <-longpoll:
		t.Fatalf("the long poll didn't wait: %+v", result)
	case <-time.After(200 * time.Millisecond):
	}

	swapped := make(chan error, 1)
	go func() {
		swapped <- swapTestHTTPGraph(ge, newGraph("second",
			newFile("/f", "two", false),
			newFile("/lp", "new", true),
		))
	}()

	select {
	case result := <-longpoll:
		if result.err != nil || result.status != http.StatusServiceUnavailable || result.retryAfter != "0" {
			t.Fatalf("the long poll got: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the old server didn't start to drain")
	}
	// The old server stopped taking new connections before that.
	select {
	case err := <-swapped:
		t.Fatalf("the swap didn't wait for the drain: %v", err)
	default:
	}

	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("the socket closed during the swap: %v", err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "GET /f HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", address); err != nil {
		t.Fatalf("request Write: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("func SetReadDeadline: %v", err)
	}
	resp, err = http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("the waiting request got no response: %v", err)
	}
	body, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || string(body) != "two" {
		t.Fatalf("the waiting request got: %d %q %v", resp.StatusCode, body, err)
	}

	select {
	case err := <-swapped:
		if err != nil {
			t.Fatalf("second graph: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the swap didn't finish")
	}
	select {
	case result := <-longpoll:
		if result.err != nil || result.status != http.StatusOK || result.body != "new" {
			t.Fatalf("the long poll reconnect got: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the long poll reconnect got no answer")
	}
	if n := testConnCut(t, stalled); n >= int64(len(big)) {
		t.Errorf("the stalled client got the whole file")
	}

	running = false
	if err := ge.Pause(); err != nil {
		t.Fatalf("engine Pause: %v", err)
	}
	if err := ge.Shutdown(); err != nil {
		t.Fatalf("engine Shutdown: %v", err)
	}
	if conn, err := net.Dial("tcp", address); err == nil {
		_ = conn.Close()
		t.Fatal("the socket is still open after the http:server was removed")
	}
}
