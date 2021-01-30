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

package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util"

	etcd "go.etcd.io/etcd/clientv3"
)

const (
	ns = "" // in case we want to add one back in
)

// WatchResources returns a channel that outputs events when exported resources
// change.
// TODO: Filter our watch (on the server side if possible) based on the
// collection prefixes and filters that we care about...
func WatchResources(ctx context.Context, client interfaces.Client) (chan error, error) {
	path := fmt.Sprintf("%s/exported/", ns)
	return client.Watcher(ctx, path, etcd.WithPrefix())
}

// SetResources exports all of the resources which we pass in to etcd.
func SetResources(ctx context.Context, client interfaces.Client, hostname string, resourceList []engine.Res) error {
	// key structure is $NS/exported/$hostname/resources/$uid = $data

	var kindFilter []string // empty to get from everyone
	hostnameFilter := []string{hostname}
	// this is not a race because we should only be reading keys which we
	// set, and there should not be any contention with other hosts here!
	originals, err := GetResources(ctx, client, hostnameFilter, kindFilter)
	if err != nil {
		return err
	}

	if len(originals) == 0 && len(resourceList) == 0 { // special case of no add or del
		return nil
	}

	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this transaction
	for _, res := range resourceList {
		if res.Kind() == "" {
			return fmt.Errorf("empty kind: %s", res.Name())
		}
		uid := fmt.Sprintf("%s/%s", res.Kind(), res.Name())
		path := fmt.Sprintf("%s/exported/%s/resources/%s", ns, hostname, uid)
		if data, err := engineUtil.ResToB64(res); err == nil {
			ifs = append(ifs, etcd.Compare(etcd.Value(path), "=", data)) // desired state
			ops = append(ops, etcd.OpPut(path, data))
		} else {
			return fmt.Errorf("can't convert to B64: %v", err)
		}
	}

	match := func(res engine.Res, resourceList []engine.Res) bool { // helper lambda
		for _, x := range resourceList {
			if res.Kind() == x.Kind() && res.Name() == x.Name() {
				return true
			}
		}
		return false
	}

	hasDeletes := false
	// delete old, now unused resources here...
	for _, res := range originals {
		if res.Kind() == "" {
			return fmt.Errorf("empty kind: %s", res.Name())
		}
		uid := fmt.Sprintf("%s/%s", res.Kind(), res.Name())
		path := fmt.Sprintf("%s/exported/%s/resources/%s", ns, hostname, uid)

		if match(res, resourceList) { // if we match, no need to delete!
			continue
		}

		ops = append(ops, etcd.OpDelete(path))

		hasDeletes = true
	}

	// if everything is already correct, do nothing, otherwise, run the ops!
	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	if hasDeletes { // always run, ifs don't matter
		_, err = client.Txn(ctx, nil, ops, nil) // TODO: does this run? it should!
	} else {
		_, err = client.Txn(ctx, ifs, nil, ops) // TODO: do we need to look at response?
	}
	return err
}

// GetResources collects all of the resources which match a filter from etcd. If
// the kindfilter or hostnameFilter is empty, then it assumes no filtering...
// TODO: Expand this with a more powerful filter based on what we eventually
// support in our collect DSL. Ideally a server side filter like WithFilter()
// could do this if the pattern was $NS/exported/$kind/$hostname/$uid = $data.
func GetResources(ctx context.Context, client interfaces.Client, hostnameFilter, kindFilter []string) ([]engine.Res, error) {
	// key structure is $NS/exported/$hostname/resources/$uid = $data
	path := fmt.Sprintf("%s/exported/", ns)
	resourceList := []engine.Res{}
	keyMap, err := client.Get(ctx, path, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
	if err != nil {
		return nil, fmt.Errorf("could not get resources: %v", err)
	}
	for key, val := range keyMap {
		if !strings.HasPrefix(key, path) { // sanity check
			continue
		}

		str := strings.Split(key[len(path):], "/")
		if len(str) != 4 {
			return nil, fmt.Errorf("unexpected chunk count")
		}
		hostname, r, kind, name := str[0], str[1], str[2], str[3]
		if r != "resources" {
			return nil, fmt.Errorf("unexpected chunk pattern")
		}
		if kind == "" {
			return nil, fmt.Errorf("unexpected kind chunk")
		}
		if name == "" { // TODO: should I check this?
			return nil, fmt.Errorf("unexpected empty name")
		}
		// FIXME: ideally this would be a server side filter instead!
		if len(hostnameFilter) > 0 && !util.StrInList(hostname, hostnameFilter) {
			continue
		}

		// FIXME: ideally this would be a server side filter instead!
		if len(kindFilter) > 0 && !util.StrInList(kind, kindFilter) {
			continue
		}

		if res, err := engineUtil.B64ToRes(val); err == nil {
			//obj.Logf("Get: (Hostname, Kind, Name): (%s, %s, %s)", hostname, kind, name)
			resourceList = append(resourceList, res)
		} else {
			return nil, fmt.Errorf("can't convert from B64: %v", err)
		}
	}
	return resourceList, nil
}
