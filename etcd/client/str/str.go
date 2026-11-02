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

package str

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
	etcdUtil "go.etcd.io/etcd/client/v3/clientv3util"
)

const (
	ns = "" // in case we want to add one back in
)

// WatchStr returns a channel which spits out events on key activity.
// FIXME: It should close the channel when it's done, and spit out errors when
// something goes wrong.
// XXX: since the caller of this (via the World API) has no way to tell it it's
// done, does that mean we leak go-routines since it might still be running, but
// perhaps even blocked??? Could this cause a dead-lock? Should we instead
// return some sort of struct which has a close method with it to ask for a
// shutdown?
func WatchStr(ctx context.Context, client interfaces.Client, key string) (chan error, error) {
	// new key structure is $NS/strings/$key = $data
	path := fmt.Sprintf("%s/strings/%s", ns, key)
	return client.Watcher(ctx, path)
}

// GetStr collects the string which matches a global namespace in etcd.
func GetStr(ctx context.Context, client interfaces.Client, key string) (string, error) {
	// new key structure is $NS/strings/$key = $data
	path := fmt.Sprintf("%s/strings/%s", ns, key)
	keyMap, err := client.Get(ctx, path)
	if err != nil {
		return "", errwrap.Wrapf(err, "could not get strings in: %s", key)
	}

	if len(keyMap) == 0 {
		return "", interfaces.ErrNotExist
	}

	if count := len(keyMap); count != 1 {
		return "", fmt.Errorf("returned %d entries", count)
	}

	val, exists := keyMap[path]
	if !exists {
		return "", fmt.Errorf("path `%s` is missing", path)
	}

	return val, nil
}

// SetStr sets a key and hostname pair to a certain value. If the value is nil,
// then it deletes the key. Otherwise the value should point to a string.
// TODO: TTL or delete disconnect?
func SetStr(ctx context.Context, client interfaces.Client, key string, data *string) error {
	// key structure is $NS/strings/$key = $data
	path := fmt.Sprintf("%s/strings/%s", ns, key)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this transaction (then)
	els := []etcd.Op{}  // list of ops in this transaction (else)
	if data == nil {    // perform a delete
		ifs = append(ifs, etcdUtil.KeyExists(path))
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

// StrEdit atomically edits a value in a namespace, with a compare-and-swap
// methodology. This can either add, delete, or change an existing value. The
// current namespace value is passed in to the function. The nil value is sent
// if no value currently exists. The function is then run, and assuming nothing
// goes wrong, the returned value is used during the set. You may return nil to
// express that you wish to delete the key. The function must be pure (it must
// only depend on its input) and it may be run multiple times as needed. This
// mechanism lets multiple writers (even in different processes) share a key
// without losing each others writes, since if the desired edit can't run *from*
// the state it expected, it just retries indefinitely until it gets its turn to
// make forward progress.
func EditStr(ctx context.Context, client interfaces.Client, key string, fn func(*string) (*string, error)) error {
	// key structure is $NS/strings/$key = $data
	path := fmt.Sprintf("%s/strings/%s", ns, key)
	for {
		keyMap, err := client.Get(ctx, path)
		if err != nil {
			return errwrap.Wrapf(err, "could not get strings in: %s", key)
		}
		if count := len(keyMap); count > 1 {
			return fmt.Errorf("returned %d entries", count)
		}
		var value *string // nil if it doesn't exist
		if val, exists := keyMap[path]; exists {
			value = &val
		} else if len(keyMap) == 1 {
			return fmt.Errorf("path `%s` is missing", path)
		}

		data, err := fn(value)
		if err != nil {
			return err
		}
		if value == nil && data == nil {
			return nil // already absent
		}
		if value != nil && data != nil && *value == *data {
			return nil // already set
		}

		ifs := []etcd.Cmp{} // the value must still be what we read
		ops := []etcd.Op{}  // list of ops in this transaction (then)
		if value == nil {
			ifs = append(ifs, etcdUtil.KeyMissing(path))
		} else {
			ifs = append(ifs, etcd.Compare(etcd.Value(path), "=", *value))
		}
		if data == nil { // perform a delete
			ops = append(ops, etcd.OpDelete(path))
		} else {
			ops = append(ops, etcd.OpPut(path, *data))
		}

		resp, err := client.Txn(ctx, ifs, ops, nil)
		if err != nil {
			return errwrap.Wrapf(err, "could not edit strings in: %s", key)
		}
		if resp.Succeeded {
			return nil
		}
		// Someone else changed it after we read it, so try again. They
		// made progress, so this can't loop forever without any writes.
	}
}
