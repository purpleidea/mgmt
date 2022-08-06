// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package chooser

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/purpleidea/mgmt/etcd/interfaces"

	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
	etcd "go.etcd.io/etcd/client/v3"
)

// XXX: Test causing cluster shutdowns with:
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/chooser/dynamicsize/idealclustersize 0
// It is currently broken.

const (
	// DefaultIdealDynamicSize is the default target ideal dynamic cluster
	// size used for the initial cluster.
	DefaultIdealDynamicSize = 5

	// IdealDynamicSizePath is the path key used for the chooser. It usually
	// gets used with a namespace prefix.
	IdealDynamicSizePath = "/dynamicsize/idealclustersize"
)

// DynamicSize is a simple implementation of the Chooser interface. This helps
// select which machines to add and remove as we elastically grow and shrink our
// cluster.
// TODO: think of a better name
type DynamicSize struct {
	// IdealClusterSize is the ideal target size for this cluster. If it is
	// set to zero, then it will use DefaultIdealDynamicSize as the value.
	IdealClusterSize uint16

	data   *Data // save for later
	client interfaces.Client

	ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup
}

// Validate validates the struct.
func (obj *DynamicSize) Validate() error {
	// TODO: if changed to zero, treat as a cluster shutdown signal
	if obj.IdealClusterSize < 0 {
		return fmt.Errorf("must choose a positive IdealClusterSize value")
	}
	return nil
}

// Init accepts some useful data and handles.
func (obj *DynamicSize) Init(data *Data) error {
	if data.Hostname == "" {
		return fmt.Errorf("can't Init with empty Hostname value")
	}
	if data.Logf == nil {
		return fmt.Errorf("no Logf function was specified")
	}

	if obj.IdealClusterSize == 0 {
		obj.IdealClusterSize = DefaultIdealDynamicSize
	}

	obj.data = data
	obj.wg = &sync.WaitGroup{}
	return nil
}

// Close runs some cleanup routines.
func (obj *DynamicSize) Close() error {
	return nil
}

// Connect is called to accept an etcd.KV namespace that we can use.
func (obj *DynamicSize) Connect(ctx context.Context, client interfaces.Client) error {
	obj.client = client
	obj.ctx, obj.cancel = context.WithCancel(ctx)
	size, err := DynamicSizeGet(obj.ctx, obj.client)
	if err == interfaces.ErrNotExist || (err == nil && size <= 0) {
		// unset, set in running cluster
		changed, err := DynamicSizeSet(obj.ctx, obj.client, obj.IdealClusterSize)
		if err == nil && changed {
			obj.data.Logf("set dynamic cluster size to: %d", obj.IdealClusterSize)
		}
		return err
	} else if err == nil && size >= 1 {
		// unset, get from running cluster (use the valid cluster value)
		if obj.IdealClusterSize != size {
			obj.data.Logf("using dynamic cluster size of: %d", size)
		}
		obj.IdealClusterSize = size // get from exiting cluster...
	}

	return err
}

// Disconnect is called to cancel our use of the etcd.KV connection.
func (obj *DynamicSize) Disconnect() error {
	if obj.client != nil { // if connect was not called, don't call this...
		obj.cancel()
	}
	obj.wg.Wait()
	return nil
}

// Watch is called to send events anytime we might want to change membership. It
// is also used to watch for changes so that when we get an event, we know to
// honour the change in Choose.
func (obj *DynamicSize) Watch() (chan error, error) {
	// NOTE: The body of this function is very similar to the logic in the
	// simple client.Watcher implementation that wraps ComplexWatcher.
	path := IdealDynamicSizePath
	cancelCtx, cancel := context.WithCancel(obj.ctx)
	info, err := obj.client.ComplexWatcher(cancelCtx, path)
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

			size := obj.IdealClusterSize
			for _, event := range data.Events { // apply each event
				if event.Type != etcd.EventTypePut {
					continue
				}
				key := string(event.Kv.Key)
				key = key[len(data.Path):] // remove path prefix
				val := string(event.Kv.Value)
				if val == "" {
					continue // ignore empty values
				}
				i, err := strconv.Atoi(val)
				if err != nil {
					continue // ignore bad values
				}
				size = uint16(i) // save
			}
			if size == obj.IdealClusterSize {
				continue // no change
			}
			// set before sending the signal
			obj.IdealClusterSize = size

			if size == 0 { // zero means shutdown
				obj.data.Logf("impending cluster shutdown...")
			} else {
				obj.data.Logf("got new dynamic cluster size of: %d", size)
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

// Choose accepts a list of current membership, and a list of volunteers. From
// that we can decide who we should add and remove. We return a list of those
// nominated, and unnominated users respectively.
func (obj *DynamicSize) Choose(membership, volunteers etcdtypes.URLsMap) ([]string, []string, error) {
	// Possible nominees include anyone that has volunteered, but that
	// isn't a member.
	if obj.data.Debug {
		obj.data.Logf("goal: %d members", obj.IdealClusterSize)
	}
	nominees := []string{}
	for hostname := range volunteers {
		if _, exists := membership[hostname]; !exists {
			nominees = append(nominees, hostname)
		}
	}

	// Possible quitters include anyone that is a member, but that is not a
	// volunteer. (They must have unvolunteered.)
	quitters := []string{}
	for hostname := range membership {
		if _, exists := volunteers[hostname]; !exists {
			quitters = append(quitters, hostname)
		}
	}

	// What we want to know...
	nominated := []string{}
	unnominated := []string{}

	// We should always only add ONE member at a time!
	// TODO: is it okay to remove multiple members at the same time?
	if len(nominees) > 0 && len(membership)-len(quitters) < int(obj.IdealClusterSize) {
		//unnominated = []string{} // only do one operation at a time
		nominated = []string{nominees[0]} // FIXME: use a better picker algorithm

	} else if len(quitters) == 0 && len(membership) > int(obj.IdealClusterSize) { // too many members
		//nominated = []string{} // only do one operation at a time
		for kicked := range membership {
			// don't kick ourself unless we are the only one left...
			if kicked != obj.data.Hostname || (obj.IdealClusterSize == 0 && len(membership) == 1) {
				unnominated = []string{kicked} // FIXME: use a better picker algorithm
				break
			}
		}
	} else if len(quitters) > 0 { // must do these before new unvolunteers
		unnominated = quitters // get rid of the quitters
	}

	return nominated, unnominated, nil // perform these changes
}

// DynamicSizeGet gets the currently set dynamic size set in the cluster.
func DynamicSizeGet(ctx context.Context, client interfaces.Client) (uint16, error) {
	key := IdealDynamicSizePath
	m, err := client.Get(ctx, key) // (map[string]string, error)
	if err != nil {
		return 0, err
	}
	val, exists := m[IdealDynamicSizePath]
	if !exists {
		return 0, interfaces.ErrNotExist
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("bad value")
	}
	return uint16(i), nil
}

// DynamicSizeSet sets the dynamic size in the cluster. It returns true if it
// changed or set the value.
func DynamicSizeSet(ctx context.Context, client interfaces.Client, size uint16) (bool, error) {
	key := IdealDynamicSizePath
	val := strconv.FormatUint(uint64(size), 10) // fmt.Sprintf("%d", size)

	ifCmps := []etcd.Cmp{
		etcd.Compare(etcd.Value(key), "=", val), // desired state
	}
	elseOps := []etcd.Op{etcd.OpPut(key, val)}

	resp, err := client.Txn(ctx, ifCmps, nil, elseOps)
	if err != nil {
		return false, err
	}
	// succeeded is set to true if the compare evaluated to true
	changed := !resp.Succeeded

	return changed, err
}
