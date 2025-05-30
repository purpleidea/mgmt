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

package etcd

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
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
