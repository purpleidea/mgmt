// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package etcd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "github.com/coreos/etcd/clientv3"
	pb "github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/coreos/etcd/mvcc/mvccpb"
	etcdtypes "github.com/coreos/etcd/pkg/types" // generated package
)

// setEndpoints sets the endpoints on the etcd client if it exists. It
// prioritizes local endpoints for performance, and so that if a remote endpoint
// disconnects we aren't affected.
func (obj *EmbdEtcd) setEndpoints() {
	if obj.etcd == nil { // if client doesn't exist, skip!
		return
	}

	eps := fromURLsMapToStringList(obj.endpoints) // get flat list
	sort.Strings(eps)                             // sort for determinism

	curls, _ := obj.curls() // ignore error, was already validated

	// prio sort so we connect locally first
	urls := fromURLsToStringList(curls)
	headFn := func(x string) bool {
		return !util.StrInList(x, urls)
	}
	eps = util.PriorityStrSliceSort(eps, headFn)
	if obj.Debug {
		obj.Logf("set endpoints to: %+v", eps)
	}
	// trigger reconnect with new endpoint list
	// XXX: When a client switches endpoints, do the watches continue from
	// where they last were or do they restart? Add rev restart if needed.
	obj.etcd.SetEndpoints(eps...) // no error to check
}

// ConnectBlock runs a command as soon as the client is connected. When this
// happens, it closes the output channel. In case any error occurs, it sends it
// on that channel.
func (obj *EmbdEtcd) ConnectBlock(ctx context.Context, fn func(context.Context) error) <-chan error {
	ch := make(chan error)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch)
		select {
		case <-obj.connectSignal: // the client is connected!
		case <-ctx.Done():
			return
		}
		if fn == nil {
			return
		}
		if err := fn(ctx); err != nil {
			select {
			case ch <- err:
			case <-ctx.Done():
			}
		}
	}()
	return ch
}

// bootstrapWatcherData returns some a minimal WatcherData struct to simulate an
// initial event for bootstrapping the nominateCb before we've started up.
func bootstrapWatcherData(hostname string, urls etcdtypes.URLs) *interfaces.WatcherData {
	return &interfaces.WatcherData{
		Created: true, // add this flag to hint that we're bootstrapping

		Header: pb.ResponseHeader{}, // not needed
		Events: []*etcd.Event{
			{
				Type: mvccpb.PUT, // or mvccpb.DELETE
				Kv: &mvccpb.KeyValue{
					Key:   []byte(hostname),
					Value: []byte(urls.String()),
				},
			},
		},
	}
}

// applyDeltaEvents applies the WatchResponse deltas to a URLsMap and returns a
// modified copy.
func applyDeltaEvents(data *interfaces.WatcherData, urlsMap etcdtypes.URLsMap) (etcdtypes.URLsMap, error) {
	if err := data.Err; err != nil {
		return nil, errwrap.Wrapf(err, "data contains an error")
	}
	out, err := copyURLsMap(urlsMap)
	if err != nil {
		return nil, err
	}
	if data == nil { // passthrough
		return out, nil
	}
	var reterr error
	for _, event := range data.Events {
		key := string(event.Kv.Key)
		key = key[len(data.Path):] // remove path prefix
		//obj.Logf("applyDeltaEvents: Event(%s): %s", event.Type.String(), key)

		switch event.Type {
		case etcd.EventTypePut:
			val := string(event.Kv.Value)
			if val == "" {
				return nil, fmt.Errorf("value is empty")
			}
			urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
			if err != nil {
				return nil, errwrap.Wrapf(err, "format error")
			}
			urlsMap[key] = urls // add to map

		// expiry cases are seen as delete in v3 for now
		//case etcd.EventTypeExpire: // doesn't exist right now
		//	fallthrough
		case etcd.EventTypeDelete:
			if _, exists := urlsMap[key]; exists {
				delete(urlsMap, key)
				continue
			}

			// this can happen if we retry an operation between a
			// reconnect, so ignore in case we are reconnecting...
			reterr = errInconsistentApply // key not found
			// keep applying in case this is ignored

		default:
			return nil, fmt.Errorf("unknown event: %v", event.Type)
		}
	}
	return urlsMap, reterr
}
