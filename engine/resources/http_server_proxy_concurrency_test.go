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
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
)

// testProxy is a running http:server with one http:server:proxy grouped in.
type testProxy struct {
	url    string // base url of the http:server, no trailing slash
	server *HTTPServerRes
	cancel func()
	done   chan struct{} // closed once the server Watch has returned
	err    error         // what Watch returned, only read it after done
}

// stop cancels the server Watch and waits for it to return. It fails if that
// takes longer than the timeout. It can be called more than once.
func (obj *testProxy) stop(t *testing.T, timeout time.Duration) {
	t.Helper()
	obj.cancel()
	select {
	case <-obj.done:
	case <-time.After(timeout):
		t.Fatalf("server Watch did not return within %s of cancel", timeout)
	}
}

// newTestProxy starts an http:server on a random loopback port, with a proxy at
// /p/ which maps to head, and which caches into the cache dir. The options can
// change the http:server params before it starts.
func newTestProxy(t *testing.T, head, cache string, options ...func(*HTTPServerRes)) *testProxy {
	t.Helper()

	// Lingering handlers can log after the test ends, which t.Logf panics
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
		t.Logf(format, v...)
	}

	started := make(chan struct{})
	once := &sync.Once{}
	init := &engine.Init{
		Event: func(ctx context.Context) error {
			once.Do(func() { close(started) })
			return nil
		},
		Refresh: func() bool { return false },
		Send:    func(interface{}) error { return nil },
		Recv:    func() map[string]*engine.Send { return map[string]*engine.Send{} },
		Debug:   false,
		Logf:    logf,
	}

	proxy := &HTTPServerProxyRes{
		Sub:   "/p/",
		Head:  head,
		Cache: cache,
	}
	proxy.SetKind(httpServerProxyKind)
	proxy.SetName("/p/")

	server := &HTTPServerRes{
		Address: "127.0.0.1:0",
	}
	server.SetKind(httpServerKind)
	server.SetName("server")
	server.SetGroup([]engine.GroupableRes{proxy})
	for _, option := range options {
		option(server)
	}

	if err := proxy.Validate(); err != nil {
		t.Fatalf("proxy Validate: %v", err)
	}
	if err := server.Init(init); err != nil {
		t.Fatalf("server Init: %v", err)
	}
	t.Cleanup(func() { _ = server.Cleanup() })

	ctx, cancel := context.WithCancel(context.Background())
	obj := &testProxy{
		server: server,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go func() {
		defer close(obj.done)
		obj.err = server.Watch(ctx)
	}()
	// This runs before the Cleanup above, so that we don't clean up the
	// resources while the Watch is still using them.
	t.Cleanup(func() { obj.stop(t, 5*time.Second) })

	select {
	case <-started:
	case <-obj.done:
		t.Fatalf("server Watch exited before startup: %v", obj.err)
	case <-time.After(5 * time.Second):
		t.Fatal("server Watch did not report startup")
	}

	obj.url = fmt.Sprintf("http://%s", server.conn.Addr())
	return obj
}

// testProxyGet does a GET and returns the status and the whole body.
func testProxyGet(client *http.Client, url string) (int, []byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

// testReadEventually reads the file, and waits a little while for it to exist
// if it's not there yet.
func testReadEventually(path string) ([]byte, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil || !os.IsNotExist(err) || time.Now().After(deadline) {
			return data, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// testSlowUpstream is an upstream server with a /slow file which sends its
// first half, and then blocks until release is closed before it sends the rest.
// The first half is bigger than the proxy's response buffer, so a client of the
// proxy can see it while the proxy is still in the middle of that download.
type testSlowUpstream struct {
	*httptest.Server
	release chan struct{}
	half    []byte
}

func newTestSlowUpstream(t *testing.T) *testSlowUpstream {
	t.Helper()
	obj := &testSlowUpstream{
		release: make(chan struct{}),
		half:    bytes.Repeat([]byte("x"), 64*1024),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", 2*len(obj.half)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.half)
		w.(http.Flusher).Flush()
		select {
		case <-obj.release:
		case <-req.Context().Done():
			return
		}
		_, _ = w.Write(obj.half)
	})
	mux.HandleFunc("/other", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("other"))
	})
	obj.Server = httptest.NewServer(mux)
	return obj
}

// startSlowDownload starts a client download of /slow through the proxy, and
// returns once that client has received its first bytes, which means the proxy
// is now part way through copying the upstream body. The returned channel gets
// the result of reading the rest of the body.
func startSlowDownload(t *testing.T, client *http.Client, proxy *testProxy) <-chan error {
	t.Helper()
	resp, err := client.Get(proxy.url + "/p/slow")
	if err != nil {
		t.Fatalf("slow GET: %v", err)
	}
	if _, err := io.ReadFull(resp.Body, make([]byte, 1)); err != nil || resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("slow GET first byte: %d %v", resp.StatusCode, err)
	}
	result := make(chan error, 1)
	go func(body io.ReadCloser) {
		defer body.Close()
		_, err := io.Copy(io.Discard, body)
		result <- err
	}(resp.Body)
	return result
}

// TestHTTPServerProxyCacheHitDuringDownload checks that a request for a file
// which is already in the cache is answered while a different file is still
// being downloaded into the cache.
func TestHTTPServerProxyCacheHitDuringDownload(t *testing.T) {
	upstream := newTestSlowUpstream(t)
	defer upstream.Close()

	cache := t.TempDir() + "/"
	if err := os.WriteFile(cache+"hit", []byte("cached"), 0600); err != nil {
		t.Fatalf("func WriteFile: %v", err)
	}
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	defer proxy.stop(t, 5*time.Second)
	defer close(upstream.release) // the stop above waits for the download

	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	slowClient := &http.Client{} // no timeout
	defer slowClient.CloseIdleConnections()

	_ = startSlowDownload(t, slowClient, proxy)

	status, body, err := testProxyGet(client, proxy.url+"/p/hit")
	if err != nil {
		t.Fatalf("cache hit was blocked by a download in progress: %v", err)
	}
	if status != http.StatusOK || string(body) != "cached" {
		t.Fatalf("cache hit got: %d %q", status, body)
	}
}

// TestHTTPServerProxyDownloadsAreIndependent checks that two different files
// can be downloaded through the proxy at the same time, so that a slow one does
// not hold up any of the others.
func TestHTTPServerProxyDownloadsAreIndependent(t *testing.T) {
	upstream := newTestSlowUpstream(t)
	defer upstream.Close()

	cache := t.TempDir() + "/"
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	defer proxy.stop(t, 5*time.Second)
	defer close(upstream.release) // the stop above waits for the download

	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	slowClient := &http.Client{}
	defer slowClient.CloseIdleConnections()

	_ = startSlowDownload(t, slowClient, proxy)

	status, body, err := testProxyGet(client, proxy.url+"/p/other")
	if err != nil {
		t.Fatalf("download was blocked by a different download: %v", err)
	}
	if status != http.StatusOK || string(body) != "other" {
		t.Fatalf("other got: %d %q", status, body)
	}
}

// TestHTTPServerProxyNoPartialCacheEntry checks that a file which is still
// being downloaded is never visible at its path in the cache. Anyone reading
// the cache dir (another proxy, an http:server:file or a new mgmt after a
// crash) must only ever see whole files there.
func TestHTTPServerProxyNoPartialCacheEntry(t *testing.T) {
	upstream := newTestSlowUpstream(t)
	defer upstream.Close()

	cache := t.TempDir() + "/"
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	defer proxy.stop(t, 5*time.Second)

	slowClient := &http.Client{}
	defer slowClient.CloseIdleConnections()

	result := startSlowDownload(t, slowClient, proxy)

	_, err := os.Stat(cache + "slow")
	if err == nil {
		close(upstream.release)
		t.Fatal("a partial download is visible at its cache path")
	}
	if !os.IsNotExist(err) {
		close(upstream.release)
		t.Fatalf("func Stat: %v", err)
	}

	close(upstream.release)
	if err := <-result; err != nil {
		t.Fatalf("slow download: %v", err)
	}

	// The client gets the end of the body before it's put in the cache.
	data, err := testReadEventually(cache + "slow")
	if err != nil {
		t.Fatalf("finished download is not in the cache: %v", err)
	}
	if len(data) != 2*len(upstream.half) {
		t.Fatalf("cache entry has %d bytes, want %d", len(data), 2*len(upstream.half))
	}
}

// TestHTTPServerProxyTruncatedUpstream checks that a body which the upstream
// cut short never becomes a cache entry.
func TestHTTPServerProxyTruncatedUpstream(t *testing.T) {
	full := bytes.Repeat([]byte("y"), 32*1024)
	truncate := true
	mutex := &sync.Mutex{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mutex.Lock()
		cut := truncate
		mutex.Unlock()
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(full)))
		if !cut {
			_, _ = w.Write(full)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(full[:len(full)/2])
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close() // cut it short
		}
	}))
	defer upstream.Close()

	cache := t.TempDir() + "/"
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	defer proxy.stop(t, 5*time.Second)

	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()

	_, body, err := testProxyGet(client, proxy.url+"/p/f")
	if err == nil && len(body) == len(full) {
		t.Fatal("client got a whole body from a truncated upstream")
	}
	if _, err := os.Stat(cache + "f"); !os.IsNotExist(err) {
		t.Fatalf("truncated download left a cache entry: %v", err)
	}

	mutex.Lock()
	truncate = false
	mutex.Unlock()

	status, body, err := testProxyGet(client, proxy.url+"/p/f")
	if err != nil || status != http.StatusOK || !bytes.Equal(body, full) {
		t.Fatalf("retry got: %d, %d bytes, %v", status, len(body), err)
	}
	data, err := testReadEventually(cache + "f")
	if err != nil || !bytes.Equal(data, full) {
		t.Fatalf("retry cache entry: %d bytes, %v", len(data), err)
	}
}

// TestHTTPServerProxySameFileConcurrently checks that many clients can ask for
// the same uncached file at the same time, and that they all get the whole file
// and leave one whole cache entry behind, with nothing else in the cache dir.
func TestHTTPServerProxySameFileConcurrently(t *testing.T) {
	const clients = 8
	full := bytes.Repeat([]byte("0123456789abcdef"), 64*1024) // 1MiB

	arrived := make(chan struct{}, clients)
	gate := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		arrived <- struct{}{}
		select { // hold the first few so they all overlap
		case <-gate:
		case <-time.After(500 * time.Millisecond):
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(full)))
		_, _ = w.Write(full)
	}))
	defer upstream.Close()

	cache := t.TempDir() + "/"
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	defer proxy.stop(t, 5*time.Second)

	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()

	wg := &sync.WaitGroup{}
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body, err := testProxyGet(client, proxy.url+"/p/f")
			if err != nil {
				errs <- err
				return
			}
			if status != http.StatusOK || !bytes.Equal(body, full) {
				errs <- fmt.Errorf("got: %d, %d bytes", status, len(body))
			}
		}()
	}
	go func() { // release once a few are in, or after the timeout
		for i := 0; i < 2; i++ {
			<-arrived
		}
		close(gate)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent client: %v", err)
	}

	// Each of the downloads is committed after its client is done, so we
	// stop the server to wait for all of them before we look at the cache.
	proxy.stop(t, 5*time.Second)

	data, err := os.ReadFile(cache + "f")
	if err != nil || !bytes.Equal(data, full) {
		t.Fatalf("cache entry: %d bytes, %v", len(data), err)
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatalf("func ReadDir: %v", err)
	}
	for _, x := range entries {
		if x.Name() != "f" {
			t.Errorf("unexpected file left in cache: %q", x.Name())
		}
	}
}

// TestHTTPServerProxyUnwritableCache checks that the proxy still serves the
// client when it can't write the file into its cache.
func TestHTTPServerProxyUnwritableCache(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	defer upstream.Close()

	// A file where the cache dir should be can't be written into, and
	// unlike a read-only dir, that's true for root too.
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/cache", nil, 0600); err != nil {
		t.Fatalf("func WriteFile: %v", err)
	}
	cache := dir + "/cache/"

	proxy := newTestProxy(t, upstream.URL+"/", cache)
	defer proxy.stop(t, 5*time.Second)

	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()

	status, body, err := testProxyGet(client, proxy.url+"/p/f")
	if err != nil || status != http.StatusOK || string(body) != "data" {
		t.Fatalf("got: %d %q %v", status, body, err)
	}
}

// testStalledClient sends a GET over a raw connection, reads a little of the
// response and then stops reading, but keeps the connection open, like a client
// which has hung. It returns the connection, which the caller must close.
func testStalledClient(t *testing.T, proxy *testProxy, path string) net.Conn {
	t.Helper()
	addr := proxy.url[len("http://"):]
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("func Dial: %v", err)
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(4096) // fill up sooner
	}
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", path, addr)
	if _, err := conn.Write([]byte(request)); err != nil {
		_ = conn.Close()
		t.Fatalf("request Write: %v", err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 1024)); err != nil {
		_ = conn.Close()
		t.Fatalf("response Read: %v", err)
	}
	return conn
}

// testConnCut checks that the server closed a stalled connection, instead of
// sending the rest or leaving it open. It returns how many bytes it read. We
// don't wait for the end of what the server had queued before its close, since
// after the small read buffer of testStalledClient, some kernels only send that
// on the zero window probes, which takes ages. Instead we send the server some
// data, which a closed socket answers with a reset (RFC 1122), but which an
// open one just takes.
func testConnCut(t *testing.T, conn net.Conn) int64 {
	t.Helper()
	_, _ = conn.Write([]byte("x")) // errors if the reset is here already
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("func SetReadDeadline: %v", err)
	}
	n, err := io.Copy(io.Discard, conn) // the reset, or maybe an EOF
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatalf("the server didn't close the stalled connection, %d bytes later", n)
	}
	return n
}

// TestHTTPServerProxyStalledClientCacheHitShutdown checks that a client which
// stops reading part way through a big cache hit can only hold up the shutdown
// of the http:server for as long as the ShutdownTimeout. Then its connection is
// closed, and the Watch returns, since no handler may outlive it.
func TestHTTPServerProxyStalledClientCacheHitShutdown(t *testing.T) {
	const size = 64 * 1024 * 1024
	cache := t.TempDir() + "/"
	f, err := os.Create(cache + "big")
	if err != nil {
		t.Fatalf("func Create: %v", err)
	}
	if err := f.Truncate(size); err != nil { // sparse
		t.Fatalf("func Truncate: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}

	timeout := uint64(1) // seconds
	proxy := newTestProxy(t, "", cache, func(server *HTTPServerRes) {
		server.ShutdownTimeout = &timeout
	}) // cache only, there's no head
	conn := testStalledClient(t, proxy, "/p/big")
	defer conn.Close()

	start := time.Now()
	proxy.cancel()
	select {
	case <-proxy.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled client held up the shutdown past its timeout")
	}
	if d := time.Since(start); d < 900*time.Millisecond {
		t.Errorf("the shutdown didn't wait for the timeout, it took %s", d)
	}

	if n := testConnCut(t, conn); n >= size {
		t.Errorf("the stalled client got the whole file")
	}
}

// TestHTTPServerProxyStalledClientDownloadShutdown checks that a client which
// stops reading part way through a download that we're caching doesn't block
// other requests. With no ShutdownTimeout, the shutdown waits for it, until an
// interrupt cuts it. Then its connection is closed, and nothing is cached.
func TestHTTPServerProxyStalledClientDownloadShutdown(t *testing.T) {
	chunk := make([]byte, 64*1024)
	sent := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/big" {
			_, _ = w.Write([]byte("other"))
			return
		}
		const n = 4096 // 256MiB
		w.Header().Set("Content-Length", fmt.Sprintf("%d", n*len(chunk)))
		for i := 0; i < n; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			sent.Add(int64(len(chunk)))
		}
	}))
	defer upstream.Close()

	cache := t.TempDir() + "/"
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	conn := testStalledClient(t, proxy, "/p/big")
	defer conn.Close()

	// Wait until the upstream can't send any more. That only happens once
	// the proxy is stuck writing to our stalled client, and so it's not
	// reading from the upstream anymore.
	last, still := int64(-1), 0
	for deadline := time.Now().Add(10 * time.Second); still < 5; {
		if time.Now().After(deadline) {
			t.Fatalf("the upstream never stalled, it sent: %d", sent.Load())
		}
		time.Sleep(50 * time.Millisecond)
		if n := sent.Load(); n != last {
			last, still = n, 0
			continue
		}
		still++
	}

	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	status, body, err := testProxyGet(client, proxy.url+"/p/other")
	if err != nil || status != http.StatusOK || string(body) != "other" {
		t.Errorf("a stalled client blocked another download: %d %q %v", status, body, err)
	}

	proxy.cancel()
	select {
	case <-proxy.done:
		t.Fatalf("the shutdown didn't wait for the stalled client: %v", proxy.err)
	case <-time.After(time.Second):
	}

	// The interrupt cuts the wait short.
	if err := proxy.server.Interrupt(); err != nil {
		t.Fatalf("func Interrupt: %v", err)
	}
	select {
	case <-proxy.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the interrupt didn't end the shutdown")
	}
	_ = testConnCut(t, conn)

	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatalf("func ReadDir: %v", err)
	}
	for _, x := range entries {
		if x.Name() != "other" {
			t.Errorf("unexpected file left in cache: %q", x.Name())
		}
	}
}

// TestHTTPServerProxyDownloadCompletesDuringDrain checks that a download which
// is running when the http:server starts to shut down, eg: for a graph swap,
// keeps going until it's done. The Watch only returns after that, so by then,
// the download is also in the cache.
func TestHTTPServerProxyDownloadCompletesDuringDrain(t *testing.T) {
	upstream := newTestSlowUpstream(t)
	defer upstream.Close()

	cache := t.TempDir() + "/"
	proxy := newTestProxy(t, upstream.URL+"/", cache)
	released := false
	defer func() { // the server waits for the download, so do this first
		if !released {
			close(upstream.release)
		}
	}()

	slowClient := &http.Client{}
	defer slowClient.CloseIdleConnections()
	result := startSlowDownload(t, slowClient, proxy)

	proxy.cancel() // the drain starts
	select {
	case <-proxy.done:
		t.Fatalf("the shutdown didn't wait for the download: %v", proxy.err)
	case <-time.After(500 * time.Millisecond):
	}

	close(upstream.release)
	released = true
	if err := <-result; err != nil {
		t.Fatalf("the shutdown cut the download: %v", err)
	}
	select {
	case <-proxy.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the shutdown didn't end after the download did")
	}

	data, err := os.ReadFile(cache + "slow")
	if err != nil {
		t.Fatalf("the download isn't in the cache: %v", err)
	}
	if len(data) != 2*len(upstream.half) {
		t.Fatalf("cache entry has %d bytes, want %d", len(data), 2*len(upstream.half))
	}
}
