// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package interfaces

import (
	"context"

	etcd "go.etcd.io/etcd/clientv3" // "clientv3"
	pb "go.etcd.io/etcd/etcdserver/etcdserverpb"
)

// WatcherData is the structure of data passed to a callback from any watcher.
type WatcherData struct {
	// Created is true if this event is the initial event sent on startup.
	Created bool

	// XXX: what goes here... this? or a more processed version?
	Path   string // the path we're watching
	Header pb.ResponseHeader
	Events []*etcd.Event
	Err    error
}

// WatcherInfo is what is returned from a Watcher. It contains everything you
// might need to get information about the running watch.
type WatcherInfo struct {
	// Cancel must be called to shutdown the Watcher when we are done with
	// it. You can alternatively call cancel on the input ctx.
	Cancel func()

	// Events returns a channel of any events that occur. This happens on
	// watch startup, watch event, and watch failure. This channel closes
	// when the Watcher shuts down. If you block on these reads, then you
	// will block the entire Watcher which is usually not what you want.
	Events <-chan *WatcherData
}

// Client provides a simple interface specification for client requests. Both
// EmbdEtcd.MakeClient and client.Simple implement this.
type Client interface {
	GetClient() *etcd.Client
	Set(ctx context.Context, key, value string, opts ...etcd.OpOption) error
	Get(ctx context.Context, path string, opts ...etcd.OpOption) (map[string]string, error)
	Del(ctx context.Context, path string, opts ...etcd.OpOption) (int64, error)
	Txn(ctx context.Context, ifCmps []etcd.Cmp, thenOps, elseOps []etcd.Op) (*etcd.TxnResponse, error)
	Watcher(ctx context.Context, path string, opts ...etcd.OpOption) (chan error, error)
	ComplexWatcher(ctx context.Context, path string, opts ...etcd.OpOption) (*WatcherInfo, error)
}
