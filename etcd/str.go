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
	"errors"
	"fmt"

	etcd "github.com/coreos/etcd/clientv3"
	errwrap "github.com/pkg/errors"
)

// ErrNotExist is returned when GetStr can not find the requested key.
// TODO: https://dave.cheney.net/2016/04/07/constant-errors
var ErrNotExist = errors.New("errNotExist")

// WatchStr returns a channel which spits out events on key activity.
// FIXME: It should close the channel when it's done, and spit out errors when
// something goes wrong.
// XXX: since the caller of this (via the World API) has no way to tell it it's
// done, does that mean we leak go-routines since it might still be running, but
// perhaps even blocked??? Could this cause a dead-lock? Should we instead return
// some sort of struct which has a close method with it to ask for a shutdown?
func WatchStr(obj *EmbdEtcd, key string) chan error {
	// new key structure is $NS/strings/$key = $data
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

// GetStr collects the string which matches a global namespace in etcd.
func GetStr(obj *EmbdEtcd, key string) (string, error) {
	// new key structure is $NS/strings/$key = $data
	path := fmt.Sprintf("%s/strings/%s", NS, key)
	keyMap, err := obj.Get(path, etcd.WithPrefix())
	if err != nil {
		return "", errwrap.Wrapf(err, "could not get strings in: %s", key)
	}

	if len(keyMap) == 0 {
		return "", ErrNotExist
	}

	if count := len(keyMap); count != 1 {
		return "", fmt.Errorf("returned %d entries", count)
	}

	val, exists := keyMap[path]
	if !exists {
		return "", fmt.Errorf("path `%s` is missing", path)
	}

	//log.Printf("Etcd: GetStr(%s): %s", key, val)
	return val, nil
}

// SetStr sets a key and hostname pair to a certain value. If the value is
// nil, then it deletes the key. Otherwise the value should point to a string.
// TODO: TTL or delete disconnect?
func SetStr(obj *EmbdEtcd, key string, data *string) error {
	// key structure is $NS/strings/$key = $data
	path := fmt.Sprintf("%s/strings/%s", NS, key)
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
