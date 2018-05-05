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

	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "github.com/coreos/etcd/clientv3"
)

// setHostnameConverged sets whether a specific hostname is converged.
func (obj *EmbdEtcd) setHostnameConverged(ctx context.Context, hostname string, isConverged bool) error {
	if obj.Debug {
		obj.Logf("setHostnameConverged(%s): %t", hostname, isConverged)
		defer obj.Logf("setHostnameConverged(%s): done!", hostname)
	}

	key := fmt.Sprintf(obj.NS+convergedPathFmt, hostname)
	data := fmt.Sprintf("%t", isConverged)

	// XXX: bug: https://github.com/etcd-io/etcd/issues/10566
	// XXX: reverse things with els to workaround the bug :(
	//ifs := []etcd.Cmp{etcd.Compare(etcd.Value(key), "!=", data)} // desired state
	//ops := []etcd.Op{etcd.OpPut(key, data, etcd.WithLease(obj.leaseID))}
	ifs := []etcd.Cmp{etcd.Compare(etcd.Value(key), "=", data)} // desired state
	ifs = append(ifs, etcd.Compare(etcd.LeaseValue(key), "=", obj.leaseID))
	els := []etcd.Op{etcd.OpPut(key, data, etcd.WithLease(obj.leaseID))}

	_, err := obj.client.Txn(ctx, ifs, nil, els)
	return errwrap.Wrapf(err, "set hostname converged failed")
}
