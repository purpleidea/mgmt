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
	"net/url"
	"sort"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/clientv3"
	rpctypes "go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
	pb "go.etcd.io/etcd/etcdserver/etcdserverpb"
	etcdtypes "go.etcd.io/etcd/pkg/types"
)

// addSelfState is used to populate the initial state when I am adding myself.
func (obj *EmbdEtcd) addSelfState() {
	surls, _ := obj.surls() // validated on init
	curls, _ := obj.curls() // validated on init
	obj.membermap[obj.Hostname] = surls
	obj.endpoints[obj.Hostname] = curls
	obj.memberIDs[obj.Hostname] = obj.serverID
}

// addMemberState adds the specific member state to our local caches.
func (obj *EmbdEtcd) addMemberState(member string, id uint64, surls, curls etcdtypes.URLs) {
	obj.stateMutex.Lock()
	defer obj.stateMutex.Unlock()
	if surls != nil {
		obj.membermap[member] = surls
	}
	if curls != nil { // TODO: && len(curls) > 0 ?
		obj.endpoints[member] = curls
	}
	obj.memberIDs[member] = id
}

// rmMemberState removes the state of a given member.
func (obj *EmbdEtcd) rmMemberState(member string) {
	obj.stateMutex.Lock()
	defer obj.stateMutex.Unlock()
	delete(obj.membermap, member) // proactively delete it
	delete(obj.endpoints, member) // proactively delete it
	delete(obj.memberIDs, member) // proactively delete it
}

// updateMemberState updates some of our local state whenever we get new
// information from a response.
// TODO: ideally this would be []*etcd.Member but the types are inconsistent...
// TODO: is it worth computing a delta to see if we need to change this?
func (obj *EmbdEtcd) updateMemberState(members []*pb.Member) error {
	//nominated := make(etcdtypes.URLsMap)
	//volunteers := make(etcdtypes.URLsMap)
	membermap := make(etcdtypes.URLsMap) // map[hostname]URLs
	endpoints := make(etcdtypes.URLsMap) // map[hostname]URLs
	memberIDs := make(map[string]uint64) // map[hostname]memberID

	// URLs is etcdtypes.URLs is []url.URL
	for _, member := range members {
		// member.ID         // uint64
		// member.Name       // string (hostname)
		// member.PeerURLs   // []string (URLs)
		// member.ClientURLs // []string (URLs)

		if member.Name == "" { // not started yet
			continue
		}

		// []string -> etcdtypes.URLs
		purls, err := etcdtypes.NewURLs(member.PeerURLs)
		if err != nil {
			return err
		}
		curls, err := etcdtypes.NewURLs(member.ClientURLs)
		if err != nil {
			return err
		}

		//nominated[member.Name] = member.PeerURLs
		//volunteers[member.Name] = member.PeerURLs
		membermap[member.Name] = purls
		endpoints[member.Name] = curls
		memberIDs[member.Name] = member.ID
	}

	// set
	obj.stateMutex.Lock()
	defer obj.stateMutex.Unlock()
	// can't set these two, because we only have a partial knowledge of them
	//obj.nominated = nominated   // can't get this information (partial)
	//obj.volunteers = volunteers // can't get this information (partial)
	obj.membermap = membermap
	obj.endpoints = endpoints
	obj.memberIDs = memberIDs

	return nil
}

// memberList returns the current list of server peer members in the cluster.
func (obj *EmbdEtcd) memberList(ctx context.Context) (*etcd.MemberListResponse, error) {
	return obj.etcd.MemberList(ctx)
}

// memberAdd adds a member to the cluster.
func (obj *EmbdEtcd) memberAdd(ctx context.Context, peerURLs etcdtypes.URLs) (*etcd.MemberAddResponse, error) {
	resp, err := obj.etcd.MemberAdd(ctx, peerURLs.StringSlice())
	if err == rpctypes.ErrPeerURLExist { // commonly seen at startup
		return nil, nil
	}
	if err == rpctypes.ErrMemberExist { // not seen yet, but plan for it
		return nil, nil
	}
	return resp, err
}

// memberRemove removes a member by ID and returns if it worked, and also if
// there was an error. This is because it might have run without error, but the
// member wasn't found, for example. If a value of zero is used, then it will
// try to remove itself in an idempotent way based on whether we're supposed to
// be running a server or not.
func (obj *EmbdEtcd) memberRemove(ctx context.Context, memberID uint64) (*etcd.MemberRemoveResponse, error) {
	if memberID == 0 {
		// copy value to avoid it changing part way through
		memberID = obj.serverID
	}
	if memberID == 0 {
		return nil, fmt.Errorf("can't remove memberID of zero")
	}

	resp, err := obj.etcd.MemberRemove(ctx, memberID)
	if err == rpctypes.ErrMemberNotFound {
		// if we get this, member already shut itself down :)
		return nil, nil // unchanged, mask this error
	}

	return resp, err // changed
}

// memberChange polls the member list API and runs a function on each iteration.
// If that function returns nil, then it closes the output channel to signal an
// event. Between iterations, it sleeps for a given interval. Since this polls
// and doesn't watch events, it could miss changes if they happen rapidly. It
// does not send results on the channel, since results could be captured in the
// fn callback. It will send an error on the channel if something goes wrong.
// TODO: https://github.com/etcd-io/etcd/issues/5277
func (obj *EmbdEtcd) memberChange(ctx context.Context, fn func([]*pb.Member) error, d time.Duration) (chan error, error) {
	ch := make(chan error)
	go func() {
		defer close(ch)
		for {
			resp, err := obj.etcd.MemberList(ctx)
			if err != nil {
				select {
				case ch <- err: // send error
				case <-ctx.Done():
				}
				return
			}
			result := fn(resp.Members)
			if result == nil { // done!
				return
			}
			select {
			case <-time.After(d): // sleep before retry
				// pass
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// memberStateFromList does a member list, and applies the state to our cache.
func (obj *EmbdEtcd) memberStateFromList(ctx context.Context) error {
	resp, err := obj.memberList(ctx)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("empty response")
	}
	reterr := obj.updateMemberState(resp.Members)
	if reterr == nil {
		obj.setEndpoints() // sync client with new endpoints
	}
	return reterr
}

// isLeader returns true if I'm the leader from the first sane perspective (pov)
// that I can arbitrarily pick.
func (obj *EmbdEtcd) isLeader(ctx context.Context) (bool, error) {
	if obj.server == nil {
		return false, nil // if i'm not a server, i'm not a leader, return
	}

	var ep, backup *url.URL
	if len(obj.ClientURLs) > 0 {
		// heuristic, but probably correct
		addresses := localhostURLs(obj.ClientURLs)
		if len(addresses) > 0 {
			ep = &addresses[0] // arbitrarily pick the first one
		}
		backup = &obj.ClientURLs[0] // backup
	}
	if ep == nil && len(obj.AClientURLs) > 0 {
		addresses := localhostURLs(obj.AClientURLs)
		if len(addresses) > 0 {
			ep = &addresses[0]
		}
		backup = &obj.AClientURLs[0] // backup
	}
	if ep == nil {
		ep = backup
	}
	if ep == nil { // programming error?
		return false, fmt.Errorf("no available endpoints")
	}

	// Ask for one perspective...
	// TODO: are we supposed to use ep.Host instead?
	resp, err := obj.etcd.Maintenance.Status(ctx, ep.String()) // this perspective
	if err != nil {
		return false, err
	}
	if resp == nil {
		return false, fmt.Errorf("empty response")
	}
	if resp.Leader != obj.serverID { // i am not the leader
		return false, nil
	}

	return true, nil
}

// moveLeaderSomewhere tries to transfer the leader to the alphanumerically
// lowest member if the caller is the current leader. This contains races. If it
// succeeds, it returns the member hostname that it transferred to. If it can't
// transfer, but doesn't error, it returns an empty string. Any error condition
// returns an error.
func (obj *EmbdEtcd) moveLeaderSomewhere(ctx context.Context) (string, error) {
	//if isLeader, err := obj.isLeader(ctx); err != nil { // race!
	//	return "", errwrap.Wrapf(err, "error determining leader")
	//} else if !isLeader {
	//	if obj.Debug {
	//		obj.Logf("we are not the leader...")
	//	}
	//	return "", nil
	//}
	// assume i am the leader!

	memberList, err := obj.memberList(ctx)
	if err != nil {
		return "", err
	}

	var transfereeID uint64
	m := make(map[string]uint64)
	names := []string{}
	for _, x := range memberList.Members {
		m[x.Name] = x.ID
		if x.Name != obj.Hostname {
			names = append(names, x.Name)
		}
	}
	if len(names) == 0 {
		return "", nil // can't transfer to self, last remaining host
	}
	if len(names) == 1 && names[0] == obj.Hostname { // does this happen?
		return "", nil // can't transfer to self
	}
	sort.Strings(names)
	if len(names) > 0 {
		// transfer to alphanumerically lowest ID for consistency...
		transfereeID = m[names[0]]
	}

	if transfereeID == 0 { // safety
		return "", fmt.Errorf("got memberID of zero")
	}
	if transfereeID == obj.serverID {
		return "", nil // can't transfer to self
	}

	// do the move
	if _, err := obj.etcd.MoveLeader(ctx, transfereeID); err == rpctypes.ErrNotLeader {
		if obj.Debug {
			obj.Logf("we are not the leader...")
		}
		return "", nil // we are not the leader
	} else if err != nil {
		return "", errwrap.Wrapf(err, "error moving leader")
	}
	return names[0], nil
}
