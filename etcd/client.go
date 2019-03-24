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
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "github.com/coreos/etcd/clientv3" // "clientv3"
	context "golang.org/x/net/context"
)

// ClientEtcd provides a simple etcd client for deploy and status operations.
type ClientEtcd struct {
	Seeds []string // list of endpoints to try to connect

	client *etcd.Client
}

// GetClient returns a handle to the raw etcd client object.
func (obj *ClientEtcd) GetClient() *etcd.Client {
	return obj.client
}

// GetConfig returns the config struct to be used for the etcd client connect.
func (obj *ClientEtcd) GetConfig() etcd.Config {
	cfg := etcd.Config{
		Endpoints: obj.Seeds,
		// RetryDialer chooses the next endpoint to use
		// it comes with a default dialer if unspecified
		DialTimeout: 5 * time.Second,
	}
	return cfg
}

// Connect connects the client to a server, and then builds the *API structs.
// If reconnect is true, it will force a reconnect with new config endpoints.
func (obj *ClientEtcd) Connect() error {
	if obj.client != nil { // memoize
		return nil
	}

	var err error
	cfg := obj.GetConfig()
	obj.client, err = etcd.New(cfg) // connect!
	if err != nil {
		return errwrap.Wrapf(err, "client connect error")
	}
	return nil
}

// Destroy cleans up the entire etcd client connection.
func (obj *ClientEtcd) Destroy() error {
	err := obj.client.Close()
	//obj.wg.Wait()
	return err
}

// Get runs a get on the client connection. This has the same signature as our
// EmbdEtcd Get function.
func (obj *ClientEtcd) Get(path string, opts ...etcd.OpOption) (map[string]string, error) {
	resp, err := obj.client.Get(context.TODO(), path, opts...)
	if err != nil || resp == nil {
		return nil, err
	}

	// TODO: write a resp.ToMap() function on https://godoc.org/github.com/coreos/etcd/etcdserver/etcdserverpb#RangeResponse
	result := make(map[string]string)
	for _, x := range resp.Kvs {
		result[string(x.Key)] = string(x.Value)
	}
	return result, nil
}

// Txn runs a transaction on the client connection. This has the same signature
// as our EmbdEtcd Txn function.
func (obj *ClientEtcd) Txn(ifcmps []etcd.Cmp, thenops, elseops []etcd.Op) (*etcd.TxnResponse, error) {
	return obj.client.KV.Txn(context.TODO()).If(ifcmps...).Then(thenops...).Else(elseops...).Commit()
}
