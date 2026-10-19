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

// Package remote implements the etcd side of the handshake between a child,
// which was started by a remote resource, and the parent which started it. Each
// writer always puts its own hostname at the start of the keys it writes. The
// child writes a done record with:
//
//	$NS/remote/done/$child/$parent/$name = $data
//
// which is bound to a lease so that it disappears when the child goes away, and
// the parent acknowledges it with:
//
//	$NS/remote/ack/$parent/$name = $data
//
// which persists until the parent deletes it. The $name is the name of the
// remote resource in the parent, escaped so that it's a single path segment.
package remote

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
	etcdUtil "go.etcd.io/etcd/client/v3/clientv3util"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	ns = "" // in case we want to add one back in
)

// Handshake reads and writes the records of the handshake. It only ever writes
// keys under its own hostname, and it owns the lease which its done records are
// bound to. One session keeps that lease alive for all of them. It's created on
// first use, and Close revokes it, which deletes those records.
type Handshake struct {
	// Client is the etcd client to use.
	Client interfaces.Client

	// Hostname is our own hostname. It's the only one we ever write under.
	Hostname string

	// TTL is the number of seconds that our done records stay around for,
	// after we disappear without running Close.
	TTL int

	session *concurrency.Session
	mutex   *sync.Mutex
}

// Init must be called before any other method. It returns itself so that it can
// be chained after the struct definition.
func (obj *Handshake) Init() *Handshake {
	obj.mutex = &sync.Mutex{}
	return obj
}

// Close revokes the lease of our done records, which deletes them. This is
// bounded by the TTL, after which the lease expires anyways. It's safe to call
// this even if no record was ever written, and to call it more than once.
func (obj *Handshake) Close() error {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if obj.session == nil {
		return nil
	}
	err := obj.session.Close()
	obj.session = nil
	return err
}

// lease returns the lease that our done records are bound to. We grant it with
// the context of the caller, so that a blocked grant can be cancelled, but the
// session keeps it alive with the context of the client, so that it lasts until
// Close, instead of only as long as this caller.
func (obj *Handshake) lease(ctx context.Context) (etcd.LeaseID, error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if session := obj.session; session != nil {
		select {
		case <-session.Done():
			// The keepalive stopped. This can happen on an etcd
			// partition which was longer than the TTL. The lease
			// expires by itself if it hasn't already, so there's
			// nothing to revoke. We make a new one below, and the
			// caller puts its record back.
			obj.session = nil
		default:
			return session.Lease(), nil
		}
	}

	if obj.TTL <= 0 {
		return etcd.NoLease, fmt.Errorf("the TTL must be positive")
	}
	c := obj.Client.GetClient()
	resp, err := c.Grant(ctx, int64(obj.TTL))
	if err != nil {
		return etcd.NoLease, errwrap.Wrapf(err, "could not grant lease")
	}
	session, err := concurrency.NewSession(c, concurrency.WithTTL(obj.TTL), concurrency.WithLease(resp.ID))
	if err != nil {
		// Nothing keeps that lease alive, so it expires by itself.
		return etcd.NoLease, errwrap.Wrapf(err, "could not create session")
	}
	obj.session = session
	return session.Lease(), nil
}

// DoneWatch returns a channel which sends an event on startup, and on any
// change to the done record of the child. Run DoneGet to see what changed.
func (obj *Handshake) DoneWatch(ctx context.Context, child, parent, name string) (chan error, error) {
	path, err := DonePath(child, parent, name)
	if err != nil {
		return nil, err
	}
	return obj.Client.Watcher(ctx, path) // exact key, not a prefix
}

// DoneGet returns the done record of the child, or nil if there isn't one.
func (obj *Handshake) DoneGet(ctx context.Context, child, parent, name string) (*string, error) {
	path, err := DonePath(child, parent, name)
	if err != nil {
		return nil, err
	}
	val, err := obj.get(ctx, path)
	return val, errwrap.Wrapf(err, "could not get done record: %s", path)
}

// DoneSet stores our done record, as a child, for the remote resource with this
// name in the parent. It's bound to our lease, so that it's deleted when we go
// away. If that lease was lost, it is put again with a new one.
func (obj *Handshake) DoneSet(ctx context.Context, parent, name, value string) error {
	path, err := DonePath(obj.Hostname, parent, name)
	if err != nil {
		return err
	}
	lease, err := obj.lease(ctx)
	if err != nil {
		return err
	}
	return errwrap.Wrapf(obj.set(ctx, path, value, lease), "could not set done record: %s", path)
}

// DoneDel deletes our done record, as a child.
func (obj *Handshake) DoneDel(ctx context.Context, parent, name string) error {
	path, err := DonePath(obj.Hostname, parent, name)
	if err != nil {
		return err
	}
	return errwrap.Wrapf(obj.del(ctx, path), "could not delete done record: %s", path)
}

// AckWatch returns a channel which sends an event on startup, and on any change
// to the ack record of the parent. Run AckGet to see what changed.
func (obj *Handshake) AckWatch(ctx context.Context, parent, name string) (chan error, error) {
	path, err := AckPath(parent, name)
	if err != nil {
		return nil, err
	}
	return obj.Client.Watcher(ctx, path) // exact key, not a prefix
}

// AckGet returns the ack record of the parent, or nil if there isn't one.
func (obj *Handshake) AckGet(ctx context.Context, parent, name string) (*string, error) {
	path, err := AckPath(parent, name)
	if err != nil {
		return nil, err
	}
	val, err := obj.get(ctx, path)
	return val, errwrap.Wrapf(err, "could not get ack record: %s", path)
}

// AckSet stores our ack record, as a parent, for our remote resource with this
// name. It is not bound to any lease, so it persists until it's deleted.
func (obj *Handshake) AckSet(ctx context.Context, name, value string) error {
	path, err := AckPath(obj.Hostname, name)
	if err != nil {
		return err
	}
	return errwrap.Wrapf(obj.set(ctx, path, value, etcd.NoLease), "could not set ack record: %s", path)
}

// AckDel deletes our ack record, as a parent.
func (obj *Handshake) AckDel(ctx context.Context, name string) error {
	path, err := AckPath(obj.Hostname, name)
	if err != nil {
		return err
	}
	return errwrap.Wrapf(obj.del(ctx, path), "could not delete ack record: %s", path)
}

// get returns the value at this exact key, or nil if it doesn't exist.
func (obj *Handshake) get(ctx context.Context, path string) (*string, error) {
	keyMap, err := obj.Client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if len(keyMap) == 0 {
		return nil, nil // not found
	}
	val, exists := keyMap[path]
	if !exists || len(keyMap) != 1 {
		return nil, fmt.Errorf("unexpected result for key: %s", path)
	}
	return &val, nil
}

// set puts a value at this key, bound to this lease, which may be NoLease. It
// does this in one transaction, so that if the key already has this value and
// lease, nothing is written, and nobody gets a watch event. A key which doesn't
// exist fails both of these comparisons, so it gets written.
func (obj *Handshake) set(ctx context.Context, path, value string, lease etcd.LeaseID) error {
	ifs := []etcd.Cmp{
		etcd.Compare(etcd.Value(path), "=", value),
		etcd.Compare(etcd.LeaseValue(path), "=", lease),
	}
	els := []etcd.Op{
		etcd.OpPut(path, value, etcd.WithLease(lease)),
	}
	_, err := obj.Client.Txn(ctx, ifs, nil, els)
	return err
}

// del deletes this key. It does this in one transaction, so that if the key
// doesn't exist, nobody gets a watch event.
func (obj *Handshake) del(ctx context.Context, path string) error {
	ifs := []etcd.Cmp{
		etcdUtil.KeyExists(path),
	}
	ops := []etcd.Op{
		etcd.OpDelete(path),
	}
	_, err := obj.Client.Txn(ctx, ifs, ops, nil)
	return err
}

// DonePath returns the key of the done record that the child writes for the
// remote resource with this name in the parent.
func DonePath(child, parent, name string) (string, error) {
	if err := util.ValidHostname(child); err != nil {
		return "", errwrap.Wrapf(err, "invalid child")
	}
	if err := util.ValidHostname(parent); err != nil {
		return "", errwrap.Wrapf(err, "invalid parent")
	}
	if name == "" {
		return "", fmt.Errorf("empty name")
	}
	return fmt.Sprintf("%s/remote/done/%s/%s/%s", ns, child, parent, url.PathEscape(name)), nil
}

// AckPath returns the key of the ack record that the parent writes for the
// remote resource with this name.
func AckPath(parent, name string) (string, error) {
	if err := util.ValidHostname(parent); err != nil {
		return "", errwrap.Wrapf(err, "invalid parent")
	}
	if name == "" {
		return "", fmt.Errorf("empty name")
	}
	return fmt.Sprintf("%s/remote/ack/%s/%s", ns, parent, url.PathEscape(name)), nil
}
