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
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
)

// testFreeAddress returns a loopback address with a port which was free just
// now, for the tests which need to know the address before anything binds it.
func testFreeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("func Listen: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
	return address
}

// startTestSocketBackground runs the http:server background worker for this
// address, like the engine does, and returns a func which stops it.
func startTestSocketBackground(t *testing.T, address string) func() {
	t.Helper()
	handle := &engine.BackgroundHandle{
		Key:  address,
		Logf: t.Logf,
	}
	fn := (&HTTPServerRes{}).Background(handle)

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- fn(ctx, ready)
	}()
	select {
	case <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("background exited before it was ready: %v", err)
	}

	return func() {
		t.Helper()
		cancel()
		if err := <-done; err != context.Canceled {
			t.Errorf("background returned: %v", err)
		}
	}
}

// startTestSocketServer runs the Watch of an http:server on this address, which
// serves data at /f. If the Watch fails before it has started up, then this
// returns that error. Otherwise it returns a func which stops it.
func startTestSocketServer(t *testing.T, address, data string) (*HTTPServerRes, func(), error) {
	t.Helper()
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
		Logf:    t.Logf,
	}

	file := &HTTPServerFileRes{
		Data: data,
	}
	file.SetKind(httpServerFileKind)
	file.SetName("/f")

	server := &HTTPServerRes{
		Address: address,
	}
	server.SetKind(httpServerKind)
	server.SetName("server")
	server.SetGroup([]engine.GroupableRes{file})
	if err := server.Init(init); err != nil {
		t.Fatalf("server Init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Watch(ctx)
	}()

	select {
	case <-started:
	case err := <-done:
		cancel()
		_ = server.Cleanup()
		return nil, nil, err
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("server Watch did not report startup")
	}

	stopOnce := &sync.Once{}
	stop := func() { // it's okay to run this more than once
		t.Helper()
		stopOnce.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("server Watch did not return after cancel")
			}
			_ = server.Cleanup()
		})
	}
	return server, stop, nil
}

// TestHTTPServerSocketAcrossWatches checks that while the background worker for
// an address runs, its socket stays open between the Watch of one http:server
// and the next one with that address, like in a graph swap. A connection which
// is made in between waits, and the next one serves it. Only one Watch at a
// time can serve the address, and the socket closes when the background stops.
func TestHTTPServerSocketAcrossWatches(t *testing.T) {
	address := testFreeAddress(t)
	stopBackground := startTestSocketBackground(t, address)
	stopped := false
	defer func() {
		if !stopped {
			stopBackground()
		}
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()

	server1, stop1, err := startTestSocketServer(t, address, "one")
	if err != nil {
		t.Fatalf("first server: %v", err)
	}
	addr := server1.conn.Addr().String()
	status, body, err := testProxyGet(client, "http://"+addr+"/f")
	if err != nil || status != http.StatusOK || string(body) != "one" {
		t.Fatalf("first server got: %d %q %v", status, body, err)
	}

	if _, _, err := startTestSocketServer(t, address, "dup"); err == nil {
		t.Fatal("two servers are serving the same address")
	}

	stop1()
	client.CloseIdleConnections()

	// Nothing is serving the address now, but the socket is still open.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("the socket closed between the two servers: %v", err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "GET /f HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", addr); err != nil {
		t.Fatalf("request Write: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("func SetReadDeadline: %v", err)
	}
	if n, err := conn.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Fatalf("something answered between the two servers: %d %v", n, err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("func SetReadDeadline: %v", err)
	}

	server2, stop2, err := startTestSocketServer(t, address, "two")
	if err != nil {
		t.Fatalf("second server: %v", err)
	}
	defer stop2()
	if a := server2.conn.Addr().String(); a != addr {
		t.Errorf("second server is on %s, not on %s", a, addr)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("the waiting request got no response: %v", err)
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || string(body) != "two" {
		t.Fatalf("the waiting request got: %d %q %v", resp.StatusCode, body, err)
	}

	stop2()
	stopBackground()
	stopped = true
	if conn, err := net.Dial("tcp", addr); err == nil {
		_ = conn.Close()
		t.Fatal("the socket is still open after the background stopped")
	}
}

// TestHTTPServerSocketPortZero checks that when the port is zero, the
// background worker doesn't keep a socket, so that two http:servers with that
// address can run at once, each on a random port of its own.
func TestHTTPServerSocketPortZero(t *testing.T) {
	const address = "127.0.0.1:0"
	stopBackground := startTestSocketBackground(t, address)
	defer stopBackground()

	server1, stop1, err := startTestSocketServer(t, address, "one")
	if err != nil {
		t.Fatalf("first server: %v", err)
	}
	defer stop1()
	server2, stop2, err := startTestSocketServer(t, address, "two")
	if err != nil {
		t.Fatalf("second server: %v", err)
	}
	defer stop2()

	addr1 := server1.conn.Addr().String()
	addr2 := server2.conn.Addr().String()
	if addr1 == addr2 {
		t.Fatalf("both servers are on %s", addr1)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	for addr, want := range map[string]string{addr1: "one", addr2: "two"} {
		status, body, err := testProxyGet(client, "http://"+addr+"/f")
		if err != nil || status != http.StatusOK || string(body) != want {
			t.Errorf("server on %s got: %d %q %v", addr, status, body, err)
		}
	}
}

// TestHTTPServerSocketBindError checks that the background worker doesn't bind
// the socket, so a port which is in use is an error of the Watch, and not one
// of the background, which would stop the whole engine. A later Watch can
// retry.
func TestHTTPServerSocketBindError(t *testing.T) {
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("func Listen: %v", err)
	}
	address := occupier.Addr().String()

	stopBackground := startTestSocketBackground(t, address)
	defer stopBackground()

	if _, stop, err := startTestSocketServer(t, address, "x"); err == nil {
		stop()
		t.Fatal("the server bound a port which is in use")
	}

	if err := occupier.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
	_, stop, err := startTestSocketServer(t, address, "x")
	if err != nil {
		t.Fatalf("the retry couldn't bind the port: %v", err)
	}
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	status, body, err := testProxyGet(client, "http://"+address+"/f")
	if err != nil || status != http.StatusOK || string(body) != "x" {
		t.Fatalf("got: %d %q %v", status, body, err)
	}
}
