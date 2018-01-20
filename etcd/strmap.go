// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/util"

	etcd "github.com/coreos/etcd/clientv3"
	errwrap "github.com/pkg/errors"
)

// WatchStrMap returns a channel which spits out events on key activity.
// FIXME: It should close the channel when it's done, and spit out errors when
// something goes wrong.
func WatchStrMap(obj *EmbdEtcd, key string) chan error {
	// new key structure is $NS/strings/$key/$hostname = $data
	path := fmt.Sprintf("%s/strings/%s", NS, key)
	ch := make(chan error, 1)
	// FIXME: fix our API so that we get a close event on shutdown.
	callback := func(re *RE) error {
		// TODO: is this even needed? it used to happen on conn errors
		//log.Printf("Etcd: Watch: Path: %v", path) // event
		if re == nil || re.response.Canceled {
			return fmt.Errorf("watch is empty") // will cause a CtxError+retry
		}
		if len(ch) == 0 { // send event only if one isn't pending
			ch <- nil // event
		}
		return nil
	}
	_, _ = obj.AddWatcher(path, callback, true, false, etcd.WithPrefix()) // no need to check errors
	return ch
}

// GetStrMap collects all of the strings which match a namespace in etcd.
func GetStrMap(obj *EmbdEtcd, hostnameFilter []string, key string) (map[string]string, error) {
	// old key structure is $NS/strings/$hostname/$key = $data
	// new key structure is $NS/strings/$key/$hostname = $data
	// FIXME: if we have the $key as the last token (old key structure), we
	// can allow the key to contain the slash char, otherwise we need to
	// verify that one isn't present in the input string.
	path := fmt.Sprintf("%s/strings/%s", NS, key)
	keyMap, err := obj.Get(path, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
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
func SetStrMap(obj *EmbdEtcd, hostname, key string, data *string) error {
	// key structure is $NS/strings/$key/$hostname = $data
	path := fmt.Sprintf("%s/strings/%s/%s", NS, key, hostname)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this transaction (then)
	els := []etcd.Op{}  // list of ops in this transaction (else)
	if data == nil {    // perform a delete
		// TODO: use https://github.com/coreos/etcd/pull/7417 if merged
		//ifs = append(ifs, etcd.KeyExists(path))
		ifs = append(ifs, etcd.Compare(etcd.Version(path), ">", 0))
		ops = append(ops, etcd.OpDelete(path))
	} else {
		data := *data                                                // get the real value
		ifs = append(ifs, etcd.Compare(etcd.Value(path), "=", data)) // desired state
		els = append(els, etcd.OpPut(path, data))
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	_, err := obj.Txn(ifs, ops, els) // TODO: do we need to look at response?
	return errwrap.Wrapf(err, "could not set strings in: %s", key)
}
