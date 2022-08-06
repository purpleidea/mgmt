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

package etcd

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
	etcd "go.etcd.io/etcd/client/v3"
	etcdutil "go.etcd.io/etcd/client/v3/clientv3util"
)

// volunteer offers yourself up to be a server if needed. If you specify a nil
// value for urls, then this will unvolunteer yourself.
func (obj *EmbdEtcd) volunteer(ctx context.Context, urls etcdtypes.URLs) error {
	if obj.Debug {
		if urls == nil {
			obj.Logf("unvolunteer...")
			defer obj.Logf("unvolunteer: done!")
		} else {
			obj.Logf("volunteer: %s", urls.String())
			defer obj.Logf("volunteer: done!")
		}
	}
	// volunteer to be a server
	key := fmt.Sprintf(obj.NS+volunteerPathFmt, obj.Hostname)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this txn
	els := []etcd.Op{}
	if urls != nil {
		data := urls.String() // value is usually a peer "serverURL"
		// XXX: bug: https://github.com/etcd-io/etcd/issues/10566
		// XXX: reverse things with els to workaround the bug :(
		//ifs = append(ifs, etcd.Compare(etcd.Value(key), "!=", data)) // desired state
		//ops = append(ops, etcd.OpPut(key, data, etcd.WithLease(obj.leaseID)))
		ifs = append(ifs, etcd.Compare(etcd.Value(key), "=", data)) // desired state
		ifs = append(ifs, etcd.Compare(etcd.LeaseValue(key), "=", obj.leaseID))
		els = append(ops, etcd.OpPut(key, data, etcd.WithLease(obj.leaseID)))

	} else { // delete message if set to erase
		ifs = append(ifs, etcdutil.KeyExists(key)) // desired state
		ops = append(ops, etcd.OpDelete(key))
	}

	_, err := obj.client.Txn(ctx, ifs, ops, els)
	msg := "volunteering failed"
	if urls == nil {
		msg = "unvolunteering failed"
	}
	return errwrap.Wrapf(err, msg)
}

// nominate nominates a particular client to be a server (peer). If you specify
// a nil value for urls, then this will unnominate that member.
func (obj *EmbdEtcd) nominate(ctx context.Context, hostname string, urls etcdtypes.URLs) error {
	if obj.Debug {
		if urls == nil {
			obj.Logf("unnominate(%s)...", hostname)
			defer obj.Logf("unnominate(%s): done!", hostname)
		} else {
			obj.Logf("nominate(%s): %s", hostname, urls.String())
			defer obj.Logf("nominate(%s): done!", hostname)
		}
	}
	// nominate someone to be a server
	key := fmt.Sprintf(obj.NS+nominatedPathFmt, hostname)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this txn
	els := []etcd.Op{}
	if urls != nil {
		data := urls.String()
		// XXX: bug: https://github.com/etcd-io/etcd/issues/10566
		// XXX: reverse things with els to workaround the bug :(
		//ifs = append(ifs, etcd.Compare(etcd.Value(key), "!=", data)) // desired state
		//ops = append(ops, etcd.OpPut(key, data)) // TODO: add a TTL? (etcd.WithLease)
		ifs = append(ifs, etcd.Compare(etcd.Value(key), "=", data)) // desired state
		els = append(ops, etcd.OpPut(key, data))                    // TODO: add a TTL? (etcd.WithLease)

	} else { // delete message if set to erase
		ifs = append(ifs, etcdutil.KeyExists(key)) // desired state
		ops = append(ops, etcd.OpDelete(key))
	}

	_, err := obj.client.Txn(ctx, ifs, ops, els)
	msg := "nominate failed"
	if urls == nil {
		msg = "unnominate failed"
	}
	return errwrap.Wrapf(err, msg)
}

// advertise idempotently advertises the list of available client endpoints for
// the given member. If you specify a nil value for urls, then this will remove
// that member.
func (obj *EmbdEtcd) advertise(ctx context.Context, hostname string, urls etcdtypes.URLs) error {
	if obj.Debug {
		if urls == nil {
			obj.Logf("unadvertise(%s)...", hostname)
			defer obj.Logf("unadvertise(%s): done!", hostname)
		} else {
			obj.Logf("advertise(%s): %s", hostname, urls.String())
			defer obj.Logf("advertise(%s): done!", hostname)
		}
	}
	// advertise endpoints
	key := fmt.Sprintf(obj.NS+endpointsPathFmt, hostname)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this txn
	els := []etcd.Op{}
	if urls != nil {
		data := urls.String() // value is usually a "clientURL"
		// XXX: bug: https://github.com/etcd-io/etcd/issues/10566
		// XXX: reverse things with els to workaround the bug :(
		//ifs = append(ifs, etcd.Compare(etcd.Value(key), "!=", data)) // desired state
		//ops = append(ops, etcd.OpPut(key, data, etcd.WithLease(obj.leaseID)))
		ifs = append(ifs, etcd.Compare(etcd.Value(key), "=", data)) // desired state
		ifs = append(ifs, etcd.Compare(etcd.LeaseValue(key), "=", obj.leaseID))
		els = append(ops, etcd.OpPut(key, data, etcd.WithLease(obj.leaseID)))
	} else { // delete in this case
		ifs = append(ifs, etcdutil.KeyExists(key)) // desired state
		ops = append(ops, etcd.OpDelete(key))
	}

	_, err := obj.client.Txn(ctx, ifs, ops, els)
	msg := "advertising failed"
	if urls == nil {
		msg = "unadvertising failed"
	}
	return errwrap.Wrapf(err, msg)
}

// getVolunteers returns a urls map of available etcd server volunteers.
func (obj *EmbdEtcd) getVolunteers(ctx context.Context) (etcdtypes.URLsMap, error) {
	if obj.Debug {
		obj.Logf("getVolunteers()")
		defer obj.Logf("getVolunteers(): done!")
	}
	p := obj.NS + VolunteerPath
	keyMap, err := obj.client.Get(ctx, p, etcd.WithPrefix())
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't get peer volunteers")
	}
	volunteers := make(etcdtypes.URLsMap)
	for key, val := range keyMap { // loop through directory of volunteers
		if !strings.HasPrefix(key, p) {
			continue
		}
		name := key[len(p):] // get name of volunteer
		if val == "" {       // skip "erased" values
			continue
		}
		urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
		if err != nil {
			return nil, errwrap.Wrapf(err, "data format error")
		}
		volunteers[name] = urls // add to map
	}
	return volunteers, nil
}

// getNominated returns a urls map of nominated etcd server volunteers.
// NOTE: I know 'nominees' might be more correct, but is less consistent here
func (obj *EmbdEtcd) getNominated(ctx context.Context) (etcdtypes.URLsMap, error) {
	if obj.Debug {
		obj.Logf("getNominated()")
		defer obj.Logf("getNominated(): done!")
	}
	p := obj.NS + NominatedPath
	keyMap, err := obj.client.Get(ctx, p, etcd.WithPrefix()) // map[string]string, bool
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't get nominated peers")
	}
	nominated := make(etcdtypes.URLsMap)
	for key, val := range keyMap { // loop through directory of nominated
		if !strings.HasPrefix(key, p) {
			continue
		}
		name := key[len(p):] // get name of nominee
		if val == "" {       // skip "erased" values
			continue
		}
		urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
		if err != nil {
			return nil, errwrap.Wrapf(err, "data format error")
		}
		nominated[name] = urls // add to map
	}
	return nominated, nil
}

// getEndpoints returns a urls map of available endpoints for clients.
func (obj *EmbdEtcd) getEndpoints(ctx context.Context) (etcdtypes.URLsMap, error) {
	if obj.Debug {
		obj.Logf("getEndpoints()")
		defer obj.Logf("getEndpoints(): done!")
	}
	p := obj.NS + EndpointsPath
	keyMap, err := obj.client.Get(ctx, p, etcd.WithPrefix())
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't get client endpoints")
	}
	endpoints := make(etcdtypes.URLsMap)
	for key, val := range keyMap { // loop through directory of endpoints
		if !strings.HasPrefix(key, p) {
			continue
		}
		name := key[len(p):] // get name of volunteer
		if val == "" {       // skip "erased" values
			continue
		}
		urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
		if err != nil {
			return nil, errwrap.Wrapf(err, "data format error")
		}
		endpoints[name] = urls // add to map
	}
	return endpoints, nil
}
