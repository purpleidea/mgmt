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
	"sync"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/clientv3" // "clientv3"
	pb "go.etcd.io/etcd/etcdserver/etcdserverpb"
)

// nominateApply applies the changed watcher data onto our local caches.
func (obj *EmbdEtcd) nominateApply(data *interfaces.WatcherData) error {
	if data == nil { // ignore empty data
		return nil
	}

	// If we tried to lookup the nominated members here (in etcd v3) this
	// would sometimes block because we would lose the cluster leader once
	// the current leader calls the MemberAdd API and it steps down trying
	// to form a two host cluster. Instead, we can look at the event
	// response data to read the nominated values! Since we only see what
	// has *changed* in the response data, we have to keep track of the
	// original state and apply the deltas. This must be idempotent in case
	// it errors and is called again. If we're retrying and we get a data
	// format error, it's probably not the end of the world.
	nominated, err := applyDeltaEvents(data, obj.nominated) // map[hostname]URLs (URLsMap)
	if err != nil && err != errInconsistentApply {          // allow missing deletes
		return err // unexpected error, fail
	}
	// TODO: do we want to sort this if it becomes a list instead of a map?
	//sort.Strings(nominated) // deterministic order
	obj.nominated = nominated
	return nil
}

// volunteerApply applies the changed watcher data onto our local caches.
func (obj *EmbdEtcd) volunteerApply(data *interfaces.WatcherData) error {
	if data == nil { // ignore empty data
		return nil
	}
	volunteers, err := applyDeltaEvents(data, obj.volunteers) // map[hostname]URLs (URLsMap)
	if err != nil && err != errInconsistentApply {            // allow missing deletes
		return err // unexpected error, fail
	}
	// TODO: do we want to sort this if it becomes a list instead of a map?
	//sort.Strings(volunteers) // deterministic order
	obj.volunteers = volunteers
	return nil
}

// endpointApply applies the changed watcher data onto our local caches. In this
// particular apply function, it also sets our client with the new endpoints.
func (obj *EmbdEtcd) endpointApply(data *interfaces.WatcherData) error {
	if data == nil { // ignore empty data
		return nil
	}
	endpoints, err := applyDeltaEvents(data, obj.endpoints) // map[hostname]URLs (URLsMap)
	if err != nil && err != errInconsistentApply {          // allow missing deletes
		return err // unexpected error, fail
	}

	// is the endpoint list different?
	if err := cmpURLsMap(obj.endpoints, endpoints); err != nil {
		obj.endpoints = endpoints // set
		// can happen if a server drops out for example
		obj.Logf("endpoint list changed to: %+v", endpoints)
		obj.setEndpoints()
	}
	return nil
}

// nominateCb runs to respond to the nomination list change events.
// Functionally, it controls the starting and stopping of the server process. If
// a nominate message is received for this machine, then it means it is already
// being added to the cluster with member add and the cluster is now waiting for
// it to start up. When a nominate entry is removed, it's up to this function to
// run the member remove right before it shuts its server down.
func (obj *EmbdEtcd) nominateCb(ctx context.Context) error {
	// Ensure that only one copy of this function is run simultaneously.
	// This is because we don't want to cause runServer to race with
	// destroyServer. Let us completely start up before we can cancel it. As
	// a special case, destroyServer itself can race against itself. I don't
	// think it's possible for contention on this mutex, but we'll leave it
	// in for safety.
	obj.nominatedMutex.Lock()
	defer obj.nominatedMutex.Unlock()
	// This ordering mutex is being added for safety, since there is no good
	// reason for this function and volunteerCb to run simultaneously, and
	// it might be preventing a race condition that was happening.
	obj.orderingMutex.Lock()
	defer obj.orderingMutex.Unlock()
	if obj.Debug {
		obj.Logf("nominateCb")
		defer obj.Logf("nominateCb: done!")
	}

	// check if i have actually volunteered first of all...
	if obj.NoServer || len(obj.ServerURLs) == 0 {
		obj.Logf("inappropriately nominated, rogue or stale server?")
		// TODO: should we un-nominate ourself?
		return nil // we've done our job successfully
	}

	// This can happen when we're shutting down, build the nominated value.
	if len(obj.nominated) == 0 {
		obj.Logf("list of nominations is empty")
		//return nil // don't exit, we might want to shutdown the server
	} else {
		obj.Logf("nominated: %v", obj.nominated)
	}

	// if there are no other peers, we create a new server
	// TODO: do we need an || len(obj.nominated) == 0 if we're the first?
	_, exists := obj.nominated[obj.Hostname] // am i nominated?
	newCluster := len(obj.nominated) == 1 && exists
	if obj.Debug {
		obj.Logf("nominateCb: newCluster: %t; exists: %t; obj.server == nil: %t", newCluster, exists, obj.server == nil)
	}

	// TODO: server start retries should be handled inside of runServer...
	if obj.serverAction(serverActionStart) { // start
		// no server is running, but it should be
		wg := &sync.WaitGroup{}
		serverReady, ackReady := obj.ServerReady()    // must call ack!
		serverExited, ackExited := obj.ServerExited() // must call ack!

		var sendError = false
		var serverErr error
		obj.Logf("waiting for server...")
		nominated, err := copyURLsMap(obj.nominated)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj.errExitN = make(chan struct{})
			defer close(obj.errExitN) // multi-signal for errChan close op
			// blocks until server exits
			serverErr = obj.runServer(newCluster, nominated)
			// in case this exits on its own instead of with destroy
			defer obj.destroyServer()          // run to reset some values
			if sendError && serverErr != nil { // exited with an error
				select {
				case obj.errChan <- errwrap.Wrapf(serverErr, "runServer errored"):
				}
			}
		}()

		// block until either server is ready or an early exit occurs
		select {
		case <-serverReady:
			// detach from our local return of errors from an early
			// server exit (pre server ready) and switch to channel
			sendError = true // gets set before the ackReady() does
			ackReady()       // must be called
			ackExited()      // must be called
			// pass

		case <-serverExited:
			ackExited() // must be called
			ackReady()  // must be called

			wg.Wait() // wait for server to finish to get early err
			return serverErr
		}

		// Once the server is online, we *must* publish this information
		// so that (1) others know where to connect to us (2) we provide
		// an "event" for member add since there is not any event that's
		// currently built-in to etcd and (3) so we have a key to expire
		// when we shutdown or crash to give us the member remove event.
		// please see issue: https://github.com/etcd-io/etcd/issues/5277

	} else if obj.serverAction(serverActionStop) { // stop?
		// server is running, but it should not be

		// i have been un-nominated, remove self and shutdown server!
		// we don't need to do a member remove if i'm the last one...
		if len(obj.nominated) != 0 { // don't call if nobody left but me!
			// work around: https://github.com/etcd-io/etcd/issues/5482
			// and it might make sense to avoid it if we're the last
			obj.Logf("member remove: removing self: %d", obj.serverID)
			resp, err := obj.memberRemove(ctx, obj.serverID)
			if err != nil {
				if obj.Debug {
					obj.Logf("error with member remove: %v", err)
				}
				return errwrap.Wrapf(err, "member self remove error")
			}
			if resp != nil {
				obj.Logf("member removed (self): %s (%d)", obj.Hostname, obj.serverID)
				if err := obj.updateMemberState(resp.Members); err != nil {
					return err
				}
			}
		}

		// FIXME: if we fail on destroy should we try to run some of the
		// other cleanup tasks that usually afterwards (below) anyways ?
		if err := obj.destroyServer(); err != nil { // sync until exited
			return errwrap.Wrapf(err, "destroyServer errored")
		}

		// We close with this special sentinel only during destroy/exit.
		if obj.closing {
			return interfaces.ErrShutdown
		}
	}

	return nil
}

// volunteerCb runs to respond to the volunteer list change events.
// Functionally, it controls the nominating and adding of members. It typically
// nominates a peer so that it knows it will get to be a server, which causes it
// to start up its server. It also runs the member add operation so that the
// cluster gets quorum safely. The member remove operation is typically run in
// the nominateCb of that server when it is asked to shutdown. This occurs when
// the nominate entry for that server is removed. If a server removes its
// volunteer entry we must respond by removing the nomination so that it can
// receive that message and shutdown.
// FIXME: we might need to respond to member change/disconnect/shutdown events,
// see: https://github.com/etcd-io/etcd/issues/5277
// XXX: Don't allow this function to partially run if it is canceled part way
// through... We don't want an inconsistent state where we did unnominate, but
// didn't remove a member...
// XXX: If the leader changes, do we need to kick the volunteerCb or anything
// else that might have required a leader and which returned because it did not
// have one, thus loosing an event?
func (obj *EmbdEtcd) volunteerCb(ctx context.Context) error {
	// Ensure that only one copy of this function is run simultaneously.
	// It's not entirely clear if this can ever happen or if it's needed,
	// but it's an inexpensive safety check that we can add in for now.
	obj.volunteerMutex.Lock()
	defer obj.volunteerMutex.Unlock()
	// This ordering mutex is being added for safety, since there is no good
	// reason for this function and nominateCb to run simultaneously, and it
	// might be preventing a race condition that was happening.
	obj.orderingMutex.Lock()
	defer obj.orderingMutex.Unlock()
	if obj.Debug {
		obj.Logf("volunteerCb")
		defer obj.Logf("volunteerCb: done!")
	}

	// FIXME: are there any situations where we don't want to short circuit
	// here, such as if i'm the last node?
	if obj.server == nil {
		if obj.Debug {
			obj.Logf("i'm not a server yet...")
		}
		return nil // if i'm not a server, i'm not a leader, return
	}

	// FIXME: Instead of checking this, assume yes, and use the
	// `WithRequireLeader` wrapper, and just ignore the error from that if
	// it's wrong... Combined with events that poke this volunteerCb when
	// the leader changes, we shouldn't miss any events...
	if isLeader, err := obj.isLeader(ctx); err != nil { // XXX: race!
		return errwrap.Wrapf(err, "error determining leader")
	} else if !isLeader {
		if obj.Debug {
			obj.Logf("we are not the leader...")
		}
		return nil
	}
	// i am the leader!

	// Remember that the member* operations return the membership, so this
	// means we don't need to run an extra memberList in those scenarios...
	// However, this can get out of sync easily, so ensure that our member
	// information is very recent.
	if err := obj.memberStateFromList(ctx); err != nil {
		return errwrap.Wrapf(err, "error during state sync")
	}
	// XXX: If we have any unstarted members here, do we want to reschedule
	// this volunteerCb in a moment? Or will we get another event anyways?

	// NOTE: There used to be an is_leader check right here...
	// FIXME: Should we use WithRequireLeader instead? Here? Elsewhere?
	// https://godoc.org/github.com/etcd-io/etcd/clientv3#WithRequireLeader

	// FIXME: can this happen, and if so, is it an error or a pass-through?
	if len(obj.volunteers) == 0 {
		obj.Logf("list of volunteers is empty")
		//return fmt.Errorf("volunteer list is empty")
	} else {
		obj.Logf("volunteers: %+v", obj.volunteers)
	}

	// TODO: do we really need to check these errors?
	m, err := copyURLsMap(obj.membermap) // list of members...
	if err != nil {
		return err
	}
	v, err := copyURLsMap(obj.volunteers)
	if err != nil {
		return err
	}
	// Unnominate anyone that unvolunteers, so they can shutdown cleanly...
	// FIXME: one step at a time... do we trigger subsequent steps somehow?
	obj.Logf("chooser: (%+v)/(%+v)", m, v)
	nominate, unnominate, err := obj.Chooser.Choose(m, v)
	if err != nil {
		return errwrap.Wrapf(err, "chooser error")
	}

	// Ensure that we are the *last* in the list if we're unnominating, and
	// the *first* in the list if we're nominating. This way, we self-remove
	// last, and we self-add first. This is least likely to hurt quorum.
	headFn := func(x string) bool {
		return x != obj.Hostname
	}
	tailFn := func(x string) bool {
		return x == obj.Hostname
	}
	nominate = util.PriorityStrSliceSort(nominate, headFn)
	unnominate = util.PriorityStrSliceSort(unnominate, tailFn)
	obj.Logf("chooser result(+/-): %+v/%+v", nominate, unnominate)
	var reterr error
	leaderCtx := ctx // default ctx to use
	if RequireLeaderCtx {
		leaderCtx = etcd.WithRequireLeader(ctx) // FIXME: Is this correct?
	}

	for i := range nominate {
		member := nominate[i]
		peerURLs, exists := obj.volunteers[member] // comma separated list of urls
		if !exists {
			// if this happens, do we have an update race?
			return fmt.Errorf("could not find member `%s` in volunteers map", member)
		}

		// NOTE: storing peerURLs when they're already in volunteers/ is
		// redundant, but it seems to be necessary for a sane algorithm.
		// nominate before we call the API so that members see it first!
		if err := obj.nominate(leaderCtx, member, peerURLs); err != nil {
			return errwrap.Wrapf(err, "error nominating: %s", member)
		}
		// XXX: can we add a ttl here, because once we nominate someone,
		// we need to give them up to N seconds to start up after we run
		// the MemberAdd API because if they don't, in some situations
		// such as if we're adding the second node to the cluster, then
		// we've lost quorum until a second member joins! If the TTL
		// expires, we need to MemberRemove! In this special case, we
		// need to forcefully remove the second member if we don't add
		// them, because we'll be in a lack of quorum state and unable
		// to do anything... As a result, we should always only add ONE
		// member at a time!

		// XXX: After we memberAdd, can we wait a timeout, and then undo
		// the add if the member doesn't come up? We'd also need to run
		// an unnominate too, and mark the node as temporarily failed...
		obj.Logf("member add: %s: %v", member, peerURLs)
		resp, err := obj.memberAdd(leaderCtx, peerURLs)
		if err != nil {
			// FIXME: On on error this function needs to run again,
			// because we need to make sure to add the member here!
			return errwrap.Wrapf(err, "member add error")
		}
		if resp != nil { // if we're already the right state, we get nil
			obj.Logf("member added: %s (%d): %v", member, resp.Member.ID, peerURLs)
			if err := obj.updateMemberState(resp.Members); err != nil {
				return err
			}
			if resp.Member.Name == "" { // not started instantly ;)
				obj.addMemberState(member, resp.Member.ID, peerURLs, nil)
			}
			// TODO: would this ever happen or be necessary?
			//if member == obj.Hostname {
			//	obj.addSelfState()
			//}
		}
	}

	// we must remove them from the members API or it will look like a crash
	if l := len(unnominate); l > 0 {
		obj.Logf("unnominated: shutting down %d members...", l)
	}
	for i := range unnominate {
		member := unnominate[i]
		memberID, exists := obj.memberIDs[member] // map[string]uint64
		if !exists {
			// if this happens, do we have an update race?
			return fmt.Errorf("could not find member `%s` in memberIDs map", member)
		}

		// start a watcher to know if member was added
		cancelCtx, cancel := context.WithCancel(leaderCtx)
		defer cancel()
		timeout := util.CloseAfter(cancelCtx, SelfRemoveTimeout) // chan closes
		fn := func(members []*pb.Member) error {
			for _, m := range members {
				if m.Name == member || m.ID == memberID {
					return fmt.Errorf("still present")
				}
			}

			return nil // not found!
		}
		ch, err := obj.memberChange(cancelCtx, fn, MemberChangeInterval)
		if err != nil {
			return errwrap.Wrapf(err, "error watching for change of: %s", member)
		}
		if err := obj.nominate(leaderCtx, member, nil); err != nil { // unnominate
			return errwrap.Wrapf(err, "error unnominating: %s", member)
		}
		// Once we issue the above unnominate, that peer will
		// shutdown, and this might cause us to loose quorum,
		// therefore, let that member remove itself, and then
		// double check that it did happen in case delinquent.
		// TODO: get built-in transactional member Add/Remove
		// functionality to avoid a separate nominate list...

		// If we're removing ourself, then let the (un)nominate callback
		// do it. That way it removes itself cleanly on server shutdown.
		if member == obj.Hostname { // remove in unnominate!
			cancel()
			obj.Logf("unnominate: removing self...")
			continue
		}

		// cancel remove sleep and unblock early on event...
		obj.Logf("waiting %s for %s to self remove...", SelfRemoveTimeout.String(), member)
		select {
		case <-timeout:
			// pass
		case err, ok := <-ch:
			if ok {
				select {
				case <-timeout:
					// wait until timeout finishes
				}
				reterr = errwrap.Append(reterr, err)
			}
			// removed quickly!
		}
		cancel()

		// In case the removed member doesn't remove itself, do it!
		resp, err := obj.memberRemove(leaderCtx, memberID)
		if err != nil {
			return errwrap.Wrapf(err, "member remove error")
		}
		if resp != nil {
			obj.Logf("member removed (forced): %s (%d)", member, memberID)
			if err := obj.updateMemberState(resp.Members); err != nil {
				return err
			}
			// Do this I guess, but the TTL will eventually get it.
			// Remove the other member to avoid client connections.
			if err := obj.advertise(leaderCtx, member, nil); err != nil {
				return err
			}
		}

		// Remove the member from our lists to avoid blocking future
		// possible MemberList calls which would try and connect to a
		// missing member... The lists should get updated from the
		// member exiting safely if it doesn't crash, but if it did
		// and/or since it's a race to see if the update event will get
		// seen before we need the new data, just do it now anyways.
		// TODO: Is the above comment still true?
		obj.rmMemberState(member) // proactively delete it

		obj.Logf("member %s (%d) removed successfully!", member, memberID)
	}

	// NOTE: We could ensure that etcd reconnects here, but we can just wait
	// for the endpoints callback which should see the state change instead.

	obj.setEndpoints() // sync client with new endpoints
	return reterr
}
