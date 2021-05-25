// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/namespace"
)

// method represents the method we used to build the simple client.
type method uint8

const (
	methodError method = iota
	methodSeeds
	methodClient
	methodNamespace
)

// NewClientFromSeeds builds a new simple client by connecting to a list of
// seeds.
func NewClientFromSeeds(seeds []string) *Simple {
	return &Simple{
		method: methodSeeds,
		wg:     &sync.WaitGroup{},

		seeds: seeds,
	}
}

// NewClientFromSeedsNamespace builds a new simple client by connecting to a
// list of seeds and ensuring all key access is prefixed with a namespace.
func NewClientFromSeedsNamespace(seeds []string, ns string) *Simple {
	return &Simple{
		method: methodSeeds,
		wg:     &sync.WaitGroup{},

		seeds:     seeds,
		namespace: ns,
	}
}

// NewClientFromClient builds a new simple client by taking an existing client
// struct. It does not disconnect this when Close is called, as that is up to
// the parent, which is the owner of that client input struct.
func NewClientFromClient(client *etcd.Client) *Simple {
	return &Simple{
		method: methodClient,
		wg:     &sync.WaitGroup{},

		client: client,
	}
}

// NewClientFromNamespaceStr builds a new simple client by taking an existing
// client and a string namespace. Warning, this doesn't properly nest the
// namespaces.
func NewClientFromNamespaceStr(client *etcd.Client, ns string) *Simple {
	if client == nil {
		return &Simple{
			method: methodError,
			err:    fmt.Errorf("client is nil"),
		}
	}
	kv := client.KV
	w := client.Watcher
	if ns != "" { // only layer if not empty
		kv = namespace.NewKV(client.KV, ns)
		w = namespace.NewWatcher(client.Watcher, ns)
	}

	return &Simple{
		method: methodClient, // similar enough to this one to share it!
		wg:     &sync.WaitGroup{},

		client: client, // store for GetClient()
		kv:     kv,
		w:      w,
	}
}

// NewClientFromSimple builds a simple client from an existing client interface
// which must be a simple client. This awkward method is required so that
// namespace nesting works properly, because the *etcd.Client doesn't directly
// pass through the namespace. I'd love to nuke this function, but it's good
// enough for now.
func NewClientFromSimple(client interfaces.Client, ns string) *Simple {
	if client == nil {
		return &Simple{
			method: methodError,
			err:    fmt.Errorf("client is nil"),
		}
	}

	simple, ok := client.(*Simple)
	if !ok {
		return &Simple{
			method: methodError,
			err:    fmt.Errorf("client is not simple"),
		}
	}
	kv := simple.kv
	w := simple.w
	if ns != "" { // only layer if not empty
		kv = namespace.NewKV(simple.kv, ns)
		w = namespace.NewWatcher(simple.w, ns)
	}

	return &Simple{
		method: methodNamespace,
		wg:     &sync.WaitGroup{},

		client: client.GetClient(), // store for GetClient()
		kv:     kv,
		w:      w,
	}
}

// NewClientFromNamespace builds a new simple client by taking an existing set
// of interface API's that we might use.
func NewClientFromNamespace(client *etcd.Client, kv etcd.KV, w etcd.Watcher) *Simple {
	return &Simple{
		method: methodNamespace,
		wg:     &sync.WaitGroup{},

		client: client, // store for GetClient()
		kv:     kv,
		w:      w,
	}
}

// Simple provides a simple etcd client for deploy and status operations. You
// can set Debug and Logf after you've built this with one of the NewClient*
// methods.
type Simple struct {
	Debug bool
	Logf  func(format string, v ...interface{})

	method method
	wg     *sync.WaitGroup

	// err is the error we set when using methodError
	err error

	// seeds is the list of endpoints to try to connect to.
	seeds     []string
	namespace string

	// client is the etcd client connection.
	client *etcd.Client

	// kv and w are the namespaced interfaces that we got passed.
	kv etcd.KV
	w  etcd.Watcher
}

// logf is a safe wrapper around the Logf parameter that doesn't panic if the
// user didn't pass a logger in.
func (obj *Simple) logf(format string, v ...interface{}) {
	if obj.Logf == nil {
		return
	}
	obj.Logf(format, v...)
}

// config returns the config struct to be used for the etcd client connect.
func (obj *Simple) config() etcd.Config {
	cfg := etcd.Config{
		Endpoints: obj.seeds,
		// RetryDialer chooses the next endpoint to use
		// it comes with a default dialer if unspecified
		DialTimeout: 5 * time.Second,
	}
	return cfg
}

// connect connects the client to a server, and then builds the *API structs.
func (obj *Simple) connect() error {
	if obj.client != nil { // memoize
		return nil
	}

	var err error
	cfg := obj.config()
	obj.client, err = etcd.New(cfg) // connect!
	if err != nil {
		return errwrap.Wrapf(err, "client connect error")
	}
	obj.kv = obj.client.KV
	obj.w = obj.client.Watcher
	if obj.namespace != "" { // bonus feature of seeds method
		obj.kv = namespace.NewKV(obj.client.KV, obj.namespace)
		obj.w = namespace.NewWatcher(obj.client.Watcher, obj.namespace)
	}
	return nil
}

// Init starts up the struct.
func (obj *Simple) Init() error {
	// By the end of this, we must have obj.kv and obj.w available for use.
	switch obj.method {
	case methodError:
		return obj.err // use the error we set

	case methodSeeds:
		if len(obj.seeds) <= 0 {
			return fmt.Errorf("zero seeds")
		}
		return obj.connect()

	case methodClient:
		if obj.client == nil {
			return fmt.Errorf("no client")
		}
		if obj.kv == nil { // overwrite if not specified!
			obj.kv = obj.client.KV
		}
		if obj.w == nil {
			obj.w = obj.client.Watcher
		}
		return nil

	case methodNamespace:
		if obj.kv == nil || obj.w == nil {
			return fmt.Errorf("empty namespace")
		}
		return nil
	}

	return fmt.Errorf("unknown method: %+v", obj.method)
}

// Close cleans up the struct after we're finished.
func (obj *Simple) Close() error {
	defer obj.wg.Wait()
	switch obj.method {
	case methodError: // for consistency
		return fmt.Errorf("did not Init")

	case methodSeeds:
		return obj.client.Close()

	case methodClient:
		// we we're given a client, so we don't own it or close it
		return nil

	case methodNamespace:
		return nil
	}

	return fmt.Errorf("unknown method: %+v", obj.method)
}

// GetClient returns a handle to an open etcd Client. This is needed for certain
// upstream API's that don't support passing in KV and Watcher instead.
func (obj *Simple) GetClient() *etcd.Client {
	return obj.client
}

// Set runs a set operation. If you'd like more information about whether a
// value changed or not, use Txn instead.
func (obj *Simple) Set(ctx context.Context, key, value string, opts ...etcd.OpOption) error {
	// key is the full key path
	resp, err := obj.kv.Put(ctx, key, value, opts...)
	if obj.Debug {
		obj.logf("set(%s): %v", key, resp) // bonus
	}
	return err
}

// Get runs a get operation.
func (obj *Simple) Get(ctx context.Context, path string, opts ...etcd.OpOption) (map[string]string, error) {
	resp, err := obj.kv.Get(ctx, path, opts...)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("empty response")
	}

	// TODO: write a resp.ToMap() function on https://godoc.org/github.com/etcd-io/etcd/api/etcdserverpb#RangeResponse
	result := make(map[string]string)
	for _, x := range resp.Kvs {
		result[string(x.Key)] = string(x.Value)
	}
	return result, nil
}

// Del runs a delete operation.
func (obj *Simple) Del(ctx context.Context, path string, opts ...etcd.OpOption) (int64, error) {
	resp, err := obj.kv.Delete(ctx, path, opts...)
	if err == nil {
		return resp.Deleted, nil
	}
	return -1, err
}

// Txn runs a transaction.
func (obj *Simple) Txn(ctx context.Context, ifCmps []etcd.Cmp, thenOps, elseOps []etcd.Op) (*etcd.TxnResponse, error) {
	resp, err := obj.kv.Txn(ctx).If(ifCmps...).Then(thenOps...).Else(elseOps...).Commit()
	if obj.Debug {
		obj.logf("txn: %v", resp) // bonus
	}
	return resp, err
}

// Watcher is a watcher that returns a chan of error's instead of a chan with
// all sorts of watcher data. This is useful when we only want an event signal,
// but we don't care about the specifics.
func (obj *Simple) Watcher(ctx context.Context, path string, opts ...etcd.OpOption) (chan error, error) {
	cancelCtx, cancel := context.WithCancel(ctx)
	info, err := obj.ComplexWatcher(cancelCtx, path, opts...)
	if err != nil {
		defer cancel()
		return nil, err
	}
	ch := make(chan error)
	obj.wg.Add(1) // hook in to global wait group
	go func() {
		defer obj.wg.Done()
		defer close(ch)
		defer cancel()
		var data *interfaces.WatcherData
		var ok bool
		for {
			select {
			case data, ok = <-info.Events: // read
				if !ok {
					return
				}
			case <-cancelCtx.Done():
				continue // wait for ch closure, but don't block
			}

			select {
			case ch <- data.Err: // send (might be nil!)
			case <-cancelCtx.Done():
				continue // wait for ch closure, but don't block
			}
		}
	}()
	return ch, nil
}

// ComplexWatcher is a more capable watcher that also returns data information.
// This starts a watch request. It writes on a channel that you can follow to
// know when an event or an error occurs. It always sends one startup event. It
// will not return until the watch has been started. If it cannot start, then it
// will return an error. Remember to add the WithPrefix() option if you want to
// watch recursively.
// TODO: do we need to support retry and changed client connections?
// XXX: do we need to track last successful revision and retry from there? If so
// use: lastRev := response.Header.Revision // TODO: +1 ? and: etcd.WithRev(rev)
func (obj *Simple) ComplexWatcher(ctx context.Context, path string, opts ...etcd.OpOption) (*interfaces.WatcherInfo, error) {
	if obj.client == nil { // catch bugs, this often means programming error
		return nil, fmt.Errorf("client is nil") // extra safety!
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	eventsChan := make(chan *interfaces.WatcherData) // channel of runtime errors

	var count uint8
	wg := &sync.WaitGroup{}

	// TODO: if we can detect the use of WithCreatedNotify, we don't need to
	// hard-code it down below... https://github.com/etcd-io/etcd/issues/9689
	// XXX: proof of concept patch: https://github.com/etcd-io/etcd/pull/9705
	//for _, op := range opts {
	//	//if op.Cmp(etcd.WithCreatedNotify()) == nil { // would be best
	//	if etcd.OpOptionCmp(op, etcd.WithCreatedNotify()) == nil {
	//		count++
	//		wg.Add(1)
	//		break
	//	}
	//}
	count++
	wg.Add(1)

	wOpts := []etcd.OpOption{
		etcd.WithCreatedNotify(),
	}
	wOpts = append(wOpts, opts...)
	var err error

	obj.wg.Add(1) // hook in to global wait group
	go func() {
		defer obj.wg.Done()
		defer close(eventsChan)
		defer cancel() // it's safe to cancel() more than once!
		ch := obj.w.Watch(cancelCtx, path, wOpts...)
		for {
			var resp etcd.WatchResponse
			var ok bool
			var created bool
			select {
			case resp, ok = <-ch:
				if !ok {
					if count > 0 { // closed before startup
						// set err in parent scope!
						err = fmt.Errorf("watch closed")
						count--
						wg.Done()
					}
					return
				}

				// the watch is now running!
				if count > 0 && resp.Created {
					created = true
					count--
					wg.Done()
				}

				isCanceled := resp.Canceled || resp.Err() == context.Canceled
				// TODO: this might not be needed
				if resp.Header.Revision == 0 { // by inspection
					if obj.Debug {
						obj.logf("watch: received empty message") // switched client connection
					}
					isCanceled = true
				}

				if isCanceled {
					data := &interfaces.WatcherData{
						Err: context.Canceled,
					}
					select { // send the error
					case eventsChan <- data:
					case <-ctx.Done():
						return
					}
					continue // channel should close shortly
				}
			}

			// TODO: consider processing the response data into a
			// more useful form for the callback...
			data := &interfaces.WatcherData{
				Created: created,
				Path:    path,
				Header:  resp.Header,
				Events:  resp.Events,
				Err:     resp.Err(),
			}

			select { // send the event
			case eventsChan <- data:
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait() // wait for created event before we return

	return &interfaces.WatcherInfo{
		Cancel: cancel,
		Events: eventsChan,
	}, err
}
