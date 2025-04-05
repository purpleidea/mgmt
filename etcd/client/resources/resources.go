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

package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
	clientv3Util "go.etcd.io/etcd/client/v3/clientv3util"
	//pb "go.etcd.io/etcd/api/v3/etcdserverpb"
)

const (
	ns = "" // in case we want to add one back in
)

// WatchResources returns a channel that outputs events when exported resources
// change.
// TODO: Filter our watch (on the server side if possible) based on the
// collection prefixes and filters that we care about...
// XXX: filter based on kind as well, we don't do that currently... See:
// https://github.com/etcd-io/etcd/issues/19667
// TODO: do the star (*) hostname matching catch-all if we have WithStar option.
func WatchResources(ctx context.Context, client interfaces.Client, hostname, kind string) (chan error, error) {
	// key structure is $NS/exported/$hostname:to/$hostname:from/$kind/$name = $data
	ctx, cancel := context.WithCancel(ctx) // wrap

	path := fmt.Sprintf("%s/exported/%s/", ns, hostname)
	ch1, err := client.Watcher(ctx, path, etcd.WithPrefix())
	if err != nil {
		cancel()
		return nil, err
	}

	star := fmt.Sprintf("%s/exported/%s/", ns, "*")
	ch2, err := client.Watcher(ctx, star, etcd.WithPrefix())
	if err != nil {
		cancel()
		return nil, err
	}

	// multiplex the two together
	ch := make(chan error)
	go func() {
		defer cancel()
		var e error
		var ok bool
		for {
			select {
			case e, ok = <-ch1:
				if !ok {
					ch1 = nil
					if ch2 == nil {
						return
					}
				}
			case e, ok = <-ch2:
				if !ok {
					ch2 = nil
					if ch1 == nil {
						return
					}
				}
			}

			select {
			case ch <- e:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// GetResources reads the resources sent to the input hostname, and also applies
// the filters to ensure we get a limited selection.
// XXX: We'd much rather filter server side if etcd had better filtering API's.
// See: https://github.com/etcd-io/etcd/issues/19667
func GetResources(ctx context.Context, client interfaces.Client, hostname string, filters []*engine.ResFilter) ([]*engine.ResOutput, error) {
	// key structure is $NS/exported/$hostname:to/$hostname:from/$kind/$name = $data
	path := fmt.Sprintf("%s/exported/", ns)
	output := []*engine.ResOutput{}
	keyMap, err := client.Get(ctx, path, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not get resources")
	}
	for key, val := range keyMap {
		if !strings.HasPrefix(key, path) { // sanity check
			continue
		}

		str := strings.Split(key[len(path):], "/")
		if len(str) < 4 {
			return nil, fmt.Errorf("unexpected chunk count of: %d", len(str))
		}
		// The name may contain slashes, so join all those pieces back!
		hostnameTo, hostnameFrom, kind, name := str[0], str[1], str[2], strings.Join(str[3:], "/")
		if hostnameTo == "" || hostnameFrom == "" {
			return nil, fmt.Errorf("unexpected empty hostname")
		}
		if kind == "" {
			return nil, fmt.Errorf("unexpected empty kind")
		}
		if name == "" {
			return nil, fmt.Errorf("unexpected empty name")
		}

		// XXX: Do we want to include this catch-all match?
		if hostnameTo != hostname && hostnameTo != "*" { // star is any
			continue
		}

		// TODO: I'd love to avoid this O(N^2) matching if possible...
		for _, filter := range filters {
			if err := filter.Match(kind, name, hostnameFrom); err != nil {
				continue // did not match
			}
		}

		ro := &engine.ResOutput{
			Kind: kind,
			Name: name,
			Host: hostnameFrom, // from this host
			Data: val,          // encoded res data
		}
		output = append(output, ro)
	}

	return output, nil
}

// SetResources stores some resource data for export in etcd. It returns an
// error if anything goes wrong. If it didn't need to make a changes because the
// data was already correct in the database, it returns (true, nil). Otherwise
// it returns (false, nil).
func SetResources(ctx context.Context, client interfaces.Client, hostname string, resourceExports []*engine.ResExport) (bool, error) {
	// key structure is $NS/exported/$hostname:to/$hostname:from/$kind/$name = $data

	// XXX: We run each export one at a time, because there's a bug if we
	// group them, See: https://github.com/etcd-io/etcd/issues/19678
	b := true
	for _, re := range resourceExports {
		ifs := []etcd.Cmp{} // list matching the desired state
		thn := []etcd.Op{}  // list of ops in this transaction (then)
		els := []etcd.Op{}  // list of ops in this transaction (else)

		host := re.Host
		if host == "" {
			host = "*" // XXX: use whatever means "all"
		}

		path := fmt.Sprintf("%s/exported/%s/%s/%s/%s", ns, host, hostname, re.Kind, re.Name)
		ifs = append(ifs, etcd.Compare(etcd.Value(path), "=", re.Data))
		els = append(els, etcd.OpPut(path, re.Data))

		// it's important to do this in one transaction, and atomically, because
		// this way, we only generate one watch event, and only when it's needed
		out, err := client.Txn(ctx, ifs, thn, els)
		if err != nil {
			return false, err
		}

		b = b && !out.Succeeded // collect the true/false responses...
	}

	// false means something changed
	return b, nil
}

// DelResources deletes some exported resource data from etcd. It returns an
// error if anything goes wrong. If it didn't need to make a changes because the
// data was already correct in the database, it returns (true, nil). Otherwise
// it returns (false, nil).
func DelResources(ctx context.Context, client interfaces.Client, hostname string, resourceDeletes []*engine.ResDelete) (bool, error) {
	// key structure is $NS/exported/$hostname:to/$hostname:from/$kind/$name = $data

	// XXX: We run each delete one at a time, because there's a bug if we
	// group them, See: https://github.com/etcd-io/etcd/issues/19678
	b := true
	for _, rd := range resourceDeletes {

		ifs := []etcd.Cmp{} // list matching the desired state
		thn := []etcd.Op{}  // list of ops in this transaction (then)
		els := []etcd.Op{}  // list of ops in this transaction (else)

		host := rd.Host
		if host == "" {
			host = "*" // XXX: use whatever means "all"
		}

		path := fmt.Sprintf("%s/exported/%s/%s/%s/%s", ns, host, hostname, rd.Kind, rd.Name)
		ifs = append(ifs, clientv3Util.KeyExists(path))
		thn = append(thn, etcd.OpDelete(path))

		// it's important to do this in one transaction, and atomically, because
		// this way, we only generate one watch event, and only when it's needed
		out, err := client.Txn(ctx, ifs, thn, els)
		if err != nil {
			return false, err
		}

		b = b && out.Succeeded // collect the true/false responses...
	}

	// false means something changed
	return b, nil
}
