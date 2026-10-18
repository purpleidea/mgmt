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

package client

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const testTimeout = 10 * time.Second

// testFreeURL returns a local URL with a port which was free a moment ago.
func testFreeURL(t *testing.T) url.URL {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("func Listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
	return url.URL{Scheme: "http", Host: addr}
}

// testEtcd starts an embedded etcd server on free ports, and returns a client
// which is connected to it. Both are cleaned up when the test ends.
func testEtcd(t *testing.T) *etcd.Client {
	t.Helper()
	clientURL := testFreeURL(t)
	peerURL := testFreeURL(t)

	cfg := embed.NewConfig()
	cfg.Name = "test"
	cfg.Dir = t.TempDir()
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)
	cfg.Logger = "zap"
	cfg.LogLevel = "error"

	server, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("func StartEtcd: %v", err)
	}
	t.Cleanup(server.Close)
	select {
	case <-server.Server.ReadyNotify():
	case <-time.After(60 * time.Second):
		t.Fatalf("etcd server took too long to start")
	}

	cli, err := etcd.New(etcd.Config{
		Endpoints:   []string{clientURL.String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("func New: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() }) // runs before the server closes
	return cli
}

// testSimple returns a simple client which wraps this client.
func testSimple(t *testing.T, cli *etcd.Client) *Simple {
	t.Helper()
	simple := NewClientFromClient(cli)
	if err := simple.Init(); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	return simple
}

// testClose closes the simple client, which waits for all of its watcher
// goroutines, so if any of them leaked, this times out.
func testClose(t *testing.T, simple *Simple) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- simple.Close()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("func Close: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatalf("func Close: watcher goroutines leaked")
	}
}

// testRecv reads one value from a watch channel.
func testRecv(t *testing.T, ch chan error) (error, bool) {
	t.Helper()
	select {
	case err, ok := <-ch:
		return err, ok
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for a watch event")
	}
	return nil, false // unreachable
}

func TestWatcherEvents(t *testing.T) {
	cli := testEtcd(t)
	simple := testSimple(t, cli)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := simple.Watcher(ctx, "/foo")
	if err != nil {
		t.Fatalf("func Watcher: %v", err)
	}

	if err, ok := testRecv(t, ch); !ok || err != nil {
		t.Fatalf("startup event: got: %v, open: %t", err, ok)
	}

	if _, err := cli.Put(ctx, "/foo", "1"); err != nil {
		t.Fatalf("func Put: %v", err)
	}
	if err, ok := testRecv(t, ch); !ok || err != nil {
		t.Fatalf("change event: got: %v, open: %t", err, ok)
	}

	// Our own cancellation closes the channel without an error.
	cancel()
	for {
		err, ok := testRecv(t, ch)
		if !ok {
			break
		}
		if err != nil {
			t.Errorf("unexpected error after cancel: %v", err)
		}
	}

	testClose(t, simple)
}

func TestWatcherCancelledBeforeStart(t *testing.T) {
	cli := testEtcd(t)
	simple := testSimple(t, cli)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ch, err := simple.Watcher(ctx, "/foo"); err == nil {
		t.Errorf("func Watcher: expected an error, got a channel: %v", ch)
	}

	testClose(t, simple)
}

func TestWatcherCompacted(t *testing.T) {
	cli := testEtcd(t)
	simple := testSimple(t, cli)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rev int64
	for i := 0; i < 3; i++ {
		resp, err := cli.Put(ctx, "/foo", "x")
		if err != nil {
			t.Fatalf("func Put: %v", err)
		}
		rev = resp.Header.GetRevision()
	}
	if _, err := cli.Compact(ctx, rev); err != nil {
		t.Fatalf("func Compact: %v", err)
	}

	// The server creates a watch at a compacted revision, and then cancels
	// it right away. We must see that real error, and then a close.
	ch, err := simple.Watcher(ctx, "/foo", etcd.WithRev(1))
	if err != nil {
		t.Fatalf("func Watcher: %v", err)
	}
	if err, ok := testRecv(t, ch); !ok || err != nil {
		t.Fatalf("startup event: got: %v, open: %t", err, ok)
	}
	err, ok := testRecv(t, ch)
	if !ok || err == nil {
		t.Fatalf("expected a compaction error, got: %v, open: %t", err, ok)
	}
	if !errors.Is(err, rpctypes.ErrCompacted) {
		t.Errorf("expected a compaction error, got: %v", err)
	}
	if _, ok := testRecv(t, ch); ok {
		t.Errorf("expected the channel to close after the error")
	}

	testClose(t, simple)
}

func TestWatcherClientClosed(t *testing.T) {
	cli := testEtcd(t)
	simple := testSimple(t, cli)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := simple.Watcher(ctx, "/foo")
	if err != nil {
		t.Fatalf("func Watcher: %v", err)
	}
	if err, ok := testRecv(t, ch); !ok || err != nil {
		t.Fatalf("startup event: got: %v, open: %t", err, ok)
	}

	// If the watch ends while our ctx is still alive, then we must get an
	// error before the channel closes, and not just a silent close.
	if err := cli.Close(); err != nil {
		t.Fatalf("func Close: %v", err)
	}
	err, ok := testRecv(t, ch)
	if !ok || err == nil {
		t.Fatalf("expected an error, got: %v, open: %t", err, ok)
	}
	if _, ok := testRecv(t, ch); ok {
		t.Errorf("expected the channel to close after the error")
	}

	testClose(t, simple)
}
