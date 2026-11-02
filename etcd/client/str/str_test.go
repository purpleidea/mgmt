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

package str

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/etcd/interfaces"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

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

// testClient starts an embedded etcd server on free ports, and returns a simple
// client which is connected to it. Both are cleaned up when the test ends.
func testClient(t *testing.T) interfaces.Client {
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

	simple := client.NewClientFromClient(cli)
	if err := simple.Init(); err != nil {
		t.Fatalf("func Init: %v", err)
	}
	t.Cleanup(func() { _ = simple.Close() }) // runs before the client closes
	return simple
}

// testGet returns the value of a key, or nil if it doesn't exist.
func testGet(t *testing.T, c interfaces.Client, key string) *string {
	t.Helper()
	val, err := GetStr(context.Background(), c, key)
	if err == interfaces.ErrNotExist {
		return nil
	}
	if err != nil {
		t.Fatalf("func GetStr: %v", err)
	}
	return &val
}

func TestEditStr(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	key := "edit"

	// A missing key comes in as nil, and returning nil keeps it missing.
	calls := 0
	if err := EditStr(ctx, c, key, func(value *string) (*string, error) {
		calls++
		if value != nil {
			return nil, fmt.Errorf("unexpected value: %s", *value)
		}
		return nil, nil
	}); err != nil {
		t.Fatalf("func EditStr: %v", err)
	}
	if calls != 1 {
		t.Errorf("edit ran %d times, want 1", calls)
	}
	if value := testGet(t, c, key); value != nil {
		t.Fatalf("key exists with: %s", *value)
	}

	// Create it.
	if err := EditStr(ctx, c, key, func(value *string) (*string, error) {
		s := "hello"
		return &s, nil
	}); err != nil {
		t.Fatalf("func EditStr: %v", err)
	}
	if value := testGet(t, c, key); value == nil || *value != "hello" {
		t.Fatalf("unexpected value: %v", value)
	}

	// An error from the edit function doesn't change anything.
	if err := EditStr(ctx, c, key, func(value *string) (*string, error) {
		return nil, fmt.Errorf("nope")
	}); err == nil {
		t.Errorf("func EditStr: expected an error")
	}
	if value := testGet(t, c, key); value == nil || *value != "hello" {
		t.Fatalf("unexpected value: %v", value)
	}

	// Returning nil deletes it.
	if err := EditStr(ctx, c, key, func(value *string) (*string, error) {
		if value == nil || *value != "hello" {
			return nil, fmt.Errorf("unexpected value: %v", value)
		}
		return nil, nil
	}); err != nil {
		t.Fatalf("func EditStr: %v", err)
	}
	if value := testGet(t, c, key); value != nil {
		t.Fatalf("key exists with: %s", *value)
	}
}

// TestEditStrConflict changes the key while an edit is running, so that its
// write must fail and retry with the new value, instead of overwriting it.
func TestEditStrConflict(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	key := "conflict"

	for _, initial := range []*string{nil, func() *string { s := "a"; return &s }()} {
		if err := SetStr(ctx, c, key, initial); err != nil {
			t.Fatalf("func SetStr: %v", err)
		}

		seen := []string{}
		if err := EditStr(ctx, c, key, func(value *string) (*string, error) {
			if value == nil {
				seen = append(seen, "<nil>")
			} else {
				seen = append(seen, *value)
			}
			if len(seen) == 1 { // someone else writes before we do
				s := "b"
				if err := SetStr(ctx, c, key, &s); err != nil {
					return nil, err
				}
			}
			s := "c"
			if value != nil {
				s = *value + "c"
			}
			return &s, nil
		}); err != nil {
			t.Fatalf("func EditStr: %v", err)
		}

		if len(seen) != 2 || seen[1] != "b" {
			t.Errorf("unexpected edit inputs: %v", seen)
		}
		if value := testGet(t, c, key); value == nil || *value != "bc" {
			t.Errorf("unexpected value: %v", value)
		}
	}
}

// TestEditStrConcurrent runs many edits at once, and none of them can be lost.
func TestEditStrConcurrent(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	key := "counter"
	count := 20

	wg := &sync.WaitGroup{}
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- EditStr(ctx, c, key, func(value *string) (*string, error) {
				n := 0
				if value != nil {
					var err error
					if n, err = strconv.Atoi(*value); err != nil {
						return nil, err
					}
				}
				s := strconv.Itoa(n + 1)
				return &s, nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("func EditStr: %v", err)
		}
	}

	if value := testGet(t, c, key); value == nil || *value != strconv.Itoa(count) {
		t.Errorf("unexpected value: %v", value)
	}
}
