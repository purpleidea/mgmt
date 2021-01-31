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

package strmap

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/clientv3"
	etcdutil "go.etcd.io/etcd/clientv3/clientv3util"
)

const (
	ns = "" // in case we want to add one back in
)

// WatchStrMap returns a channel which spits out events on key activity.
// FIXME: It should close the channel when it's done, and spit out errors when
// something goes wrong.
func WatchStrMap(ctx context.Context, client interfaces.Client, key string) (chan error, error) {
	// new key structure is $NS/strings/$key/$hostname = $data
	path := fmt.Sprintf("%s/strings/%s", ns, key)
	return client.Watcher(ctx, path, etcd.WithPrefix())
}

// GetStrMap collects all of the strings which match a namespace in etcd.
func GetStrMap(ctx context.Context, client interfaces.Client, hostnameFilter []string, key string) (map[string]string, error) {
	// old key structure is $NS/strings/$hostname/$key = $data
	// new key structure is $NS/strings/$key/$hostname = $data
	// FIXME: if we have the $key as the last token (old key structure), we
	// can allow the key to contain the slash char, otherwise we need to
	// verify that one isn't present in the input string.
	path := fmt.Sprintf("%s/strings/%s", ns, key)
	keyMap, err := client.Get(ctx, path, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not get strings in: %s", key)
	}
	result := make(map[string]string)
	for key, val := range keyMap {
		if !strings.HasPrefix(key, path) { // sanity check
			continue
		}

		str := strings.Split(key[len(path):], "/")
		if len(str) != 2 {
			return nil, fmt.Errorf("unexpected chunk count of %d", len(str))
		}
		_, hostname := str[0], str[1]

		if hostname == "" {
			return nil, fmt.Errorf("unexpected chunk length of %d", len(hostname))
		}

		// FIXME: ideally this would be a server side filter instead!
		if len(hostnameFilter) > 0 && !util.StrInList(hostname, hostnameFilter) {
			continue
		}
		//log.Printf("Etcd: GetStr(%s): (Hostname, Data): (%s, %s)", key, hostname, val)
		result[hostname] = val
	}
	return result, nil
}

// SetStrMap sets a key and hostname pair to a certain value. If the value is
// nil, then it deletes the key. Otherwise the value should point to a string.
// TODO: TTL or delete disconnect?
func SetStrMap(ctx context.Context, client interfaces.Client, hostname, key string, data *string) error {
	// key structure is $NS/strings/$key/$hostname = $data
	path := fmt.Sprintf("%s/strings/%s/%s", ns, key, hostname)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this transaction (then)
	els := []etcd.Op{}  // list of ops in this transaction (else)
	if data == nil {    // perform a delete
		ifs = append(ifs, etcdutil.KeyExists(path))
		//ifs = append(ifs, etcd.Compare(etcd.Version(path), ">", 0))
		ops = append(ops, etcd.OpDelete(path))
	} else {
		data := *data                                                // get the real value
		ifs = append(ifs, etcd.Compare(etcd.Value(path), "=", data)) // desired state
		els = append(els, etcd.OpPut(path, data))
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	_, err := client.Txn(ctx, ifs, ops, els) // TODO: do we need to look at response?
	return errwrap.Wrapf(err, "could not set strings in: %s", key)
}
