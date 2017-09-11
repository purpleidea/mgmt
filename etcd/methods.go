// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
	"log"
	"strconv"
	"strings"

	etcd "github.com/coreos/etcd/clientv3"
	rpctypes "github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	etcdtypes "github.com/coreos/etcd/pkg/types"
	context "golang.org/x/net/context"
)

// TODO: Could all these Etcd*(obj *EmbdEtcd, ...) functions which deal with the
// interface between etcd paths and behaviour be grouped into a single struct ?

// Nominate nominates a particular client to be a server (peer).
func Nominate(obj *EmbdEtcd, hostname string, urls etcdtypes.URLs) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: Nominate(%v): %v", hostname, urls.String())
		defer log.Printf("Trace: Etcd: Nominate(%v): Finished!", hostname)
	}
	// nominate someone to be a server
	nominate := fmt.Sprintf("/%s/nominated/%s", NS, hostname)
	ops := []etcd.Op{} // list of ops in this txn
	if urls != nil {
		ops = append(ops, etcd.OpPut(nominate, urls.String())) // TODO: add a TTL? (etcd.WithLease)

	} else { // delete message if set to erase
		ops = append(ops, etcd.OpDelete(nominate))
	}

	if _, err := obj.Txn(nil, ops, nil); err != nil {
		return fmt.Errorf("nominate failed") // exit in progress?
	}
	return nil
}

// Nominated returns a urls map of nominated etcd server volunteers.
// NOTE: I know 'nominees' might be more correct, but is less consistent here
func Nominated(obj *EmbdEtcd) (etcdtypes.URLsMap, error) {
	path := fmt.Sprintf("/%s/nominated/", NS)
	keyMap, err := obj.Get(path, etcd.WithPrefix()) // map[string]string, bool
	if err != nil {
		return nil, fmt.Errorf("nominated isn't available: %v", err)
	}
	nominated := make(etcdtypes.URLsMap)
	for key, val := range keyMap { // loop through directory of nominated
		if !strings.HasPrefix(key, path) {
			continue
		}
		name := key[len(path):] // get name of nominee
		if val == "" {          // skip "erased" values
			continue
		}
		urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
		if err != nil {
			return nil, fmt.Errorf("nominated data format error: %v", err)
		}
		nominated[name] = urls // add to map
		if obj.flags.Debug {
			log.Printf("Etcd: Nominated(%v): %v", name, val)
		}
	}
	return nominated, nil
}

// Volunteer offers yourself up to be a server if needed.
func Volunteer(obj *EmbdEtcd, urls etcdtypes.URLs) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: Volunteer(%v): %v", obj.hostname, urls.String())
		defer log.Printf("Trace: Etcd: Volunteer(%v): Finished!", obj.hostname)
	}
	// volunteer to be a server
	volunteer := fmt.Sprintf("/%s/volunteers/%s", NS, obj.hostname)
	ops := []etcd.Op{} // list of ops in this txn
	if urls != nil {
		// XXX: adding a TTL is crucial! (i think)
		ops = append(ops, etcd.OpPut(volunteer, urls.String())) // value is usually a peer "serverURL"

	} else { // delete message if set to erase
		ops = append(ops, etcd.OpDelete(volunteer))
	}

	if _, err := obj.Txn(nil, ops, nil); err != nil {
		return fmt.Errorf("volunteering failed") // exit in progress?
	}
	return nil
}

// Volunteers returns a urls map of available etcd server volunteers.
func Volunteers(obj *EmbdEtcd) (etcdtypes.URLsMap, error) {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: Volunteers()")
		defer log.Printf("Trace: Etcd: Volunteers(): Finished!")
	}
	path := fmt.Sprintf("/%s/volunteers/", NS)
	keyMap, err := obj.Get(path, etcd.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("volunteers aren't available: %v", err)
	}
	volunteers := make(etcdtypes.URLsMap)
	for key, val := range keyMap { // loop through directory of volunteers
		if !strings.HasPrefix(key, path) {
			continue
		}
		name := key[len(path):] // get name of volunteer
		if val == "" {          // skip "erased" values
			continue
		}
		urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
		if err != nil {
			return nil, fmt.Errorf("volunteers data format error: %v", err)
		}
		volunteers[name] = urls // add to map
		if obj.flags.Debug {
			log.Printf("Etcd: Volunteer(%v): %v", name, val)
		}
	}
	return volunteers, nil
}

// AdvertiseEndpoints advertises the list of available client endpoints.
func AdvertiseEndpoints(obj *EmbdEtcd, urls etcdtypes.URLs) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: AdvertiseEndpoints(%v): %v", obj.hostname, urls.String())
		defer log.Printf("Trace: Etcd: AdvertiseEndpoints(%v): Finished!", obj.hostname)
	}
	// advertise endpoints
	endpoints := fmt.Sprintf("/%s/endpoints/%s", NS, obj.hostname)
	ops := []etcd.Op{} // list of ops in this txn
	if urls != nil {
		// TODO: add a TTL? (etcd.WithLease)
		ops = append(ops, etcd.OpPut(endpoints, urls.String())) // value is usually a "clientURL"

	} else { // delete message if set to erase
		ops = append(ops, etcd.OpDelete(endpoints))
	}

	if _, err := obj.Txn(nil, ops, nil); err != nil {
		return fmt.Errorf("endpoint advertising failed") // exit in progress?
	}
	return nil
}

// Endpoints returns a urls map of available etcd server endpoints.
func Endpoints(obj *EmbdEtcd) (etcdtypes.URLsMap, error) {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: Endpoints()")
		defer log.Printf("Trace: Etcd: Endpoints(): Finished!")
	}
	path := fmt.Sprintf("/%s/endpoints/", NS)
	keyMap, err := obj.Get(path, etcd.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("endpoints aren't available: %v", err)
	}
	endpoints := make(etcdtypes.URLsMap)
	for key, val := range keyMap { // loop through directory of endpoints
		if !strings.HasPrefix(key, path) {
			continue
		}
		name := key[len(path):] // get name of volunteer
		if val == "" {          // skip "erased" values
			continue
		}
		urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
		if err != nil {
			return nil, fmt.Errorf("endpoints data format error: %v", err)
		}
		endpoints[name] = urls // add to map
		if obj.flags.Debug {
			log.Printf("Etcd: Endpoint(%v): %v", name, val)
		}
	}
	return endpoints, nil
}

// SetHostnameConverged sets whether a specific hostname is converged.
func SetHostnameConverged(obj *EmbdEtcd, hostname string, isConverged bool) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: SetHostnameConverged(%s): %v", hostname, isConverged)
		defer log.Printf("Trace: Etcd: SetHostnameConverged(%v): Finished!", hostname)
	}
	converged := fmt.Sprintf("/%s/converged/%s", NS, hostname)
	op := []etcd.Op{etcd.OpPut(converged, fmt.Sprintf("%t", isConverged))}
	if _, err := obj.Txn(nil, op, nil); err != nil { // TODO: do we need a skipConv flag here too?
		return fmt.Errorf("set converged failed") // exit in progress?
	}
	return nil
}

// HostnameConverged returns a map of every hostname's converged state.
func HostnameConverged(obj *EmbdEtcd) (map[string]bool, error) {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: HostnameConverged()")
		defer log.Printf("Trace: Etcd: HostnameConverged(): Finished!")
	}
	path := fmt.Sprintf("/%s/converged/", NS)
	keyMap, err := obj.ComplexGet(path, true, etcd.WithPrefix()) // don't un-converge
	if err != nil {
		return nil, fmt.Errorf("converged values aren't available: %v", err)
	}
	converged := make(map[string]bool)
	for key, val := range keyMap { // loop through directory...
		if !strings.HasPrefix(key, path) {
			continue
		}
		name := key[len(path):] // get name of key
		if val == "" {          // skip "erased" values
			continue
		}
		b, err := strconv.ParseBool(val)
		if err != nil {
			return nil, fmt.Errorf("converged data format error: %v", err)
		}
		converged[name] = b // add to map
	}
	return converged, nil
}

// AddHostnameConvergedWatcher adds a watcher with a callback that runs on
// hostname state changes.
func AddHostnameConvergedWatcher(obj *EmbdEtcd, callbackFn func(map[string]bool) error) (func(), error) {
	path := fmt.Sprintf("/%s/converged/", NS)
	internalCbFn := func(re *RE) error {
		// TODO: get the value from the response, and apply delta...
		// for now, just run a get operation which is easier to code!
		m, err := HostnameConverged(obj)
		if err != nil {
			return err
		}
		return callbackFn(m) // call my function
	}
	return obj.AddWatcher(path, internalCbFn, true, true, etcd.WithPrefix()) // no block and no converger reset
}

// SetClusterSize sets the ideal target cluster size of etcd peers.
func SetClusterSize(obj *EmbdEtcd, value uint16) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: SetClusterSize(): %v", value)
		defer log.Printf("Trace: Etcd: SetClusterSize(): Finished!")
	}
	key := fmt.Sprintf("/%s/idealClusterSize", NS)

	if err := obj.Set(key, strconv.FormatUint(uint64(value), 10)); err != nil {
		return fmt.Errorf("function SetClusterSize failed: %v", err) // exit in progress?
	}
	return nil
}

// GetClusterSize gets the ideal target cluster size of etcd peers.
func GetClusterSize(obj *EmbdEtcd) (uint16, error) {
	key := fmt.Sprintf("/%s/idealClusterSize", NS)
	keyMap, err := obj.Get(key)
	if err != nil {
		return 0, fmt.Errorf("function GetClusterSize failed: %v", err)
	}

	val, exists := keyMap[key]
	if !exists || val == "" {
		return 0, fmt.Errorf("function GetClusterSize failed: %v", err)
	}

	v, err := strconv.ParseUint(val, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("function GetClusterSize failed: %v", err)
	}
	return uint16(v), nil
}

// MemberAdd adds a member to the cluster.
func MemberAdd(obj *EmbdEtcd, peerURLs etcdtypes.URLs) (*etcd.MemberAddResponse, error) {
	//obj.Connect(false) // TODO: ?
	ctx := context.Background()
	var response *etcd.MemberAddResponse
	var err error
	for {
		if obj.exiting { // the exit signal has been sent!
			return nil, fmt.Errorf("exiting etcd")
		}
		obj.rLock.RLock()
		response, err = obj.client.MemberAdd(ctx, peerURLs.StringSlice())
		obj.rLock.RUnlock()
		if err == nil {
			break
		}
		if ctx, err = obj.CtxError(ctx, err); err != nil {
			return nil, err
		}
	}
	return response, nil
}

// MemberRemove removes a member by mID and returns if it worked, and also
// if there was an error. This is because it might have run without error, but
// the member wasn't found, for example.
func MemberRemove(obj *EmbdEtcd, mID uint64) (bool, error) {
	//obj.Connect(false) // TODO: ?
	ctx := context.Background()
	for {
		if obj.exiting { // the exit signal has been sent!
			return false, fmt.Errorf("exiting etcd")
		}
		obj.rLock.RLock()
		_, err := obj.client.MemberRemove(ctx, mID)
		obj.rLock.RUnlock()
		if err == nil {
			break
		} else if err == rpctypes.ErrMemberNotFound {
			// if we get this, member already shut itself down :)
			return false, nil
		}
		if ctx, err = obj.CtxError(ctx, err); err != nil {
			return false, err
		}
	}
	return true, nil
}

// Members returns information on cluster membership.
// The member ID's are the keys, because an empty names means unstarted!
// TODO: consider queueing this through the main loop with CtxError(ctx, err)
func Members(obj *EmbdEtcd) (map[uint64]string, error) {
	//obj.Connect(false) // TODO: ?
	ctx := context.Background()
	var response *etcd.MemberListResponse
	var err error
	for {
		if obj.exiting { // the exit signal has been sent!
			return nil, fmt.Errorf("exiting etcd")
		}
		obj.rLock.RLock()
		if obj.flags.Trace {
			log.Printf("Trace: Etcd: Members(): Endpoints are: %v", obj.client.Endpoints())
		}
		response, err = obj.client.MemberList(ctx)
		obj.rLock.RUnlock()
		if err == nil {
			break
		}
		if ctx, err = obj.CtxError(ctx, err); err != nil {
			return nil, err
		}
	}

	members := make(map[uint64]string)
	for _, x := range response.Members {
		members[x.ID] = x.Name // x.Name will be "" if unstarted!
	}
	return members, nil
}

// Leader returns the current leader of the etcd server cluster.
func Leader(obj *EmbdEtcd) (string, error) {
	//obj.Connect(false) // TODO: ?
	var err error
	membersMap := make(map[uint64]string)
	if membersMap, err = Members(obj); err != nil {
		return "", err
	}
	addresses := obj.LocalhostClientURLs() // heuristic, but probably correct
	if len(addresses) == 0 {
		// probably a programming error...
		return "", fmt.Errorf("programming error")
	}
	endpoint := addresses[0].Host // FIXME: arbitrarily picked the first one

	// part two
	ctx := context.Background()
	var response *etcd.StatusResponse
	for {
		if obj.exiting { // the exit signal has been sent!
			return "", fmt.Errorf("exiting etcd")
		}

		obj.rLock.RLock()
		response, err = obj.client.Maintenance.Status(ctx, endpoint)
		obj.rLock.RUnlock()
		if err == nil {
			break
		}
		if ctx, err = obj.CtxError(ctx, err); err != nil {
			return "", err
		}
	}

	// isLeader: response.Header.MemberId == response.Leader
	for id, name := range membersMap {
		if id == response.Leader {
			return name, nil
		}
	}
	return "", fmt.Errorf("members map is not current") // not found
}
