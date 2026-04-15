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

// Package scheduler implements a distributed consensus scheduler with etcd.
package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/scheduler"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
	clientv3Util "go.etcd.io/etcd/client/v3/clientv3util"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	// SchedulerTTL is the TTL on the main scheduler.
	SchedulerTTL = 1 // seconds

	// ResignTimeout specifies how long we'll attempt to run the resign
	// operation for. Don't wait longer than this when shutting down.
	ResignTimeout = 5 * time.Second

	hostnameJoinChar = "," // char used to join and split lists of hostnames
)

// Apparatus is a container for all the scheduling function implementations.
type Apparatus struct {
	Client interfaces.Client

	// Prefix is the etcd prefix used, not a file path. For example, we
	// expect this to be `/scheduler/` here, although if for some reason it
	// needed to be namespaced, this would change. The etcd namespace is a
	// prefix that goes even before this one.
	Prefix string

	// Hostname is this hosts unique identifier.
	Hostname string

	Debug bool
	Logf  func(format string, v ...interface{})

	// strategy is the active scheduler strategy struct for each namespace.
	strategy map[string]scheduler.Strategy

	// schedulerLeases is a process lifetime namespace->lease in-memory map.
	//schedulerLeases map[string]etcd.LeaseID

	// schedulerSessions is a process lifetime namespace->session in-memory
	// map.
	schedulerSessions map[string]*concurrency.Session

	// cleanupRequests are pending namespaces that need their sessions
	// closed.
	cleanupRequests map[string]struct{}

	// schedulerExited informs any SchedulerCleanup calls that we exited
	// early. This is a rare scenario, but it could happen.
	schedulerExited bool

	// schedulerMutex guards schedulerSessions, cleanupRequests and
	// schedulerExited.
	schedulerMutex *sync.Mutex
}

// Init prepares the struct for first use and returns it for ergonomics.
func (obj *Apparatus) Init() *Apparatus {

	obj.strategy = make(map[string]scheduler.Strategy)

	//obj.schedulerLeases = make(map[string]etcd.LeaseID)
	obj.schedulerSessions = make(map[string]*concurrency.Session)

	obj.cleanupRequests = make(map[string]struct{})

	obj.schedulerMutex = &sync.Mutex{}

	return obj // for ergonomics, change API if we need an error...
}

// Scheduler runs a distributed scheduler.
//
// It does not schedule any hosts directly. It runs once per host (for a given
// prefix) on any participating member that wishes to take part in scheduling
// decisions and which shares the same prefix. It uses the etcd scheduling
// primitives to elect one host out of the set of hosts running this. That host
// then runs the scheduling algorithm that chooses which hosts are picked out of
// the set of inquiring groups made with the schedule add and withdraw
// functions.
//
// To add or withdraw from the scheduled group, keys are place or removed (or
// expired) from special paths which are watched by the scheduling host. (This
// happens in this function.) Different options can be passed in to customize
// the behaviour. The input hostname arg represents the unique identifier for
// the caller. The behaviour is undefined if this is run more than once with the
// same path and hostname simultaneously, or if illogically incompatible options
// are passed in via scheduler add calls.
//
// It's important that if we change the elected host, the new hosts starts by
// looking at the previous scheduling decision and continuing as much as
// possible from that "epoch" (scheduling decision) if it's still a valid one.
// This means there won't be unnecessary churn in scheduled hosts if possible,
// and things stay as persistent as they can. It's entirely up to the specific
// scheduling algorithm to implement this logic if you so desire it.
//
// When multiple different scheduling group requests are made, that algorithm
// needs to do more work. In the future, we could distribute this work across
// different hosts.
func (obj *Apparatus) Scheduler(ctx context.Context, ready chan<- struct{}) (reterr error) {
	// * key structure is /scheduler/election = ??? (shared)
	//	** used for the core (single) top-level scheduler...

	// * key structure is /scheduler/schedule/$namespace = host1,host3,...
	//	** used for the scheduler result for each namespaced decision...

	// * key structure is /scheduler/hostname/$hostname/$namespace = <opts>
	//	** used to add/withdraw ones own hostname to/from a namespace...

	// * key structure is /scheduler/data/$hostname/$namespace/data = data
	//	** used to store scheduling data alongside a hostname/namespace

	// * we need $hostname in the prefix before $namespace so that when we
	// do etcd auth, we can add /scheduler/hostname/$hostname/ as a prefix

	// Previously, as a special case, if we were a single host, and we were
	// trying to withdraw, then there wouldn't have been an elected member
	// to push a new scheduling result to the special key where *Scheduled*
	// would read it. As a result, we'd need some hacks to remove hosts that
	// we're in the exchanged set. This new architecture doesn't have this
	// limitation because when we withdraw, the elected host still watches.

	if !strings.HasSuffix(obj.Prefix, "/") {
		return fmt.Errorf("prefix must end with the slash char")
	}
	if !strings.HasPrefix(obj.Prefix, "/") {
		return fmt.Errorf("prefix must start with the slash char")
	}
	if obj.Hostname == "" {
		return fmt.Errorf("hostname must not be empty")
	}
	if strings.Contains(obj.Hostname, hostnameJoinChar) {
		return fmt.Errorf("hostname must not contain join char: %s", hostnameJoinChar)
	}

	c := obj.Client.GetClient()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// There are no more *options* here since this is the core background
	// scheduler and it's Add/Withdraw that specify the options. (Well "Add"
	// does!) If we do want to get more specific here, we could create a
	// `schedule:options` resource to pass those in, but that would be a lot
	// of work, for a very unlikely benefit.

	sessionOptions := []concurrency.SessionOption{}
	// ttl for key expiry on abrupt disconnection
	sessionOptions = append(sessionOptions, concurrency.WithTTL(SchedulerTTL))
	//sessionOptions = append(sessionOptions, concurrency.WithLease(leaseID))

	// create a session object
	session, err := concurrency.NewSession(c, sessionOptions...)
	if err != nil {
		return errwrap.Wrapf(err, "could not create session")
	}
	defer session.Close() // this revokes the lease in shutdown...
	//leaseID := session.Lease() // if needed somewhere

	// create an election object
	// we need to add on the client namespace here since the special /_mgmt/
	// prefix isn't added when we go directly through this API...
	electionPath := fmt.Sprintf("%s%selection", obj.Client.GetNamespace(), obj.Prefix)
	election := concurrency.NewElection(session, electionPath)

	electionChan := election.Observe(ctx)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), ResignTimeout)
		defer cancel()

		// If we're not the leader, this is a harmless noop.
		if err := election.Resign(ctx); err != nil {
			// lock only needed if we do this elsewhere concurrently
			reterr = errwrap.Append(reterr, err)
		}
	}()

	defer func() {
		// We only need these mutexes for the rare situation when this
		// scheduler prematurely exits on its own before the Watch func
		// of a `schedule` resource exits. In that situation we also set
		// the schedulerExited bool, to tell everyone else that we died.
		obj.schedulerMutex.Lock()
		defer obj.schedulerMutex.Unlock()

		// TODO: deterministic order?
		for namespace := range obj.cleanupRequests {
			session, exists := obj.schedulerSessions[namespace]
			if !exists {
				continue
			}
			// closing the session here expires the attached leaseID
			if err := session.Close(); err != nil {
				// lock only needed if we do this elsewhere concurrently
				reterr = errwrap.Append(reterr, err)
			}
		}
		obj.cleanupRequests = make(map[string]struct{}) // reset (or gc)
		obj.schedulerExited = true
	}()

	// Down here instead of at the top because we don't have an earlier need
	// for waitgroups and this way we make sure the `reterr` can't be set at
	// the same time from more than one place. (Race free now!)
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	campaignFunc := func(ctx context.Context) error {
		obj.Logf("starting campaign...")
		for {
			if err := election.Campaign(ctx, obj.Hostname); err != nil {
				if err == context.Canceled {
					return nil
				}
				return err
			}
		}
	}

	// kick off an initial campaign if none exist already...
	obj.Logf("checking for existing leader...")
	leaderResult, err := election.Leader(ctx)
	if err == concurrency.ErrElectionNoLeader {
		// start up the campaign function
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := campaignFunc(ctx); err != nil { // run
				// lock only needed if we do this elsewhere concurrently
				reterr = errwrap.Append(reterr, err)
				cancel() // important to trigger the shutdown...
			}
		}()

	} else if err != nil {
		obj.Logf("leader information error: %v", err)
		return err
	}
	if obj.Debug {
		obj.Logf("leader information: %v", leaderResult)
	}

	// NOTE: We don't really need to watch here for changes, we just trust.
	// In the odd chance there is a fight to write, let the last one win it.
	//scheduledPathAll := fmt.Sprintf("%sschedule/", obj.Prefix)
	//scheduledChan, err := obj.Client.Watcher(ctx, scheduledPathAll, etcd.WithPrefix())
	//if err != nil {
	//	return errwrap.Wrapf(err, "could not watch schedule path")
	//}

	// Watch the Add/Withdraw changes from here.
	hostnamePathPrefix := fmt.Sprintf("%shostname/", obj.Prefix)

	// NOTE: etcd.WithSort can't be used in a watcher!
	hostnameChan, err := obj.Client.Watcher(ctx, hostnamePathPrefix, etcd.WithPrefix())
	if err != nil {
		return errwrap.Wrapf(err, "could not watch hostname path")
	}

	close(ready)   // signal we've started up correctly!
	defer cancel() // to unblock anything above

	elected := "" // who we "assume" is elected
	for {
		select {
		// new election result
		case val, ok := <-electionChan:
			if obj.Debug {
				obj.Logf("electionChan(%t): %+v", ok, val)
			}
			if !ok {
				obj.Logf("elections stream shutdown...")
				return fmt.Errorf("election shutdown")
			}

			elected = string(val.Kvs[0].Value)
			obj.Logf("elected: %s", elected)
			// XXX: Do we need to start/stop the campaign function
			// here? Do we care if it runs when i'm already elected?
			if elected != obj.Hostname { // not me!
				// This strategy invalidation must not be
				// concurrent with running the doScheduled
				// function below...
				obj.strategy = make(map[string]scheduler.Strategy)
				continue
			}
			// i was voted in to make the scheduling choice!

		// XXX: do we need to watch for tampering in our decisions?
		// watch election result
		//case _, ok := <-scheduledChan:
		//	if !ok {
		//		obj.Logf("scheduled stream shutdown...")
		//		return fmt.Errorf("scheduled stream shutdown")
		//	}

		// watch hostname submissions
		case _, ok := <-hostnameChan:
			if !ok {
				obj.Logf("hostname stream shutdown...")
				return fmt.Errorf("hostname stream shutdown")
			}

		case <-ctx.Done():
			return ctx.Err()
		}

		if elected != obj.Hostname { // not me!
			continue
		}

		// XXX: Can we skip running the scheduler again if we know that
		// there's no new work to do? For example if we were watching
		// the election result channel, and nothing changed.

		// i am the leader, run scheduler and store result
		obj.Logf("i am elected, running scheduler...")

		// Here we need to figure out which schedulers to run...
		// Depending on how many namespaces people have published into.
		data, err := obj.getData(ctx) // get all the data
		if err != nil {
			return err
		}

		// When we previously did a schedule withdraw, the key at:
		// /scheduler/hostname/$hostname/$namespace will get removed,
		// meaning getData would have shown empty, but the result in:
		// /scheduler/schedule/$namespace will still have that hostname!
		// As a result it was updated to look at both prefixes to get
		// the list of all namespaces so that they can be updated to the
		// empty set or removed entirely (equivalent) if so desired!

		// XXX: loop these concurrently?
		// map[namespace] -> map[hostname] -> opts
		for namespace, hm := range data { // a bunch of different namespaces...
			if _, err := obj.doScheduled(ctx, namespace, hm); err != nil { // hm: map[hostname] -> opts
				return err
			}
		}
	}
}

// SchedulerAdd registers this host as a candidate for scheduling within this
// namespace.
func (obj *Apparatus) SchedulerAdd(ctx context.Context, namespace string, apply bool, opts ...scheduler.Option) (bool, error) {
	// * key structure is /scheduler/hostname/$hostname/$namespace = <opts>
	//	** used to add/withdraw ones own hostname to/from a namespace...

	if !strings.HasSuffix(obj.Prefix, "/") {
		return false, fmt.Errorf("prefix must end with the slash char")
	}
	if !strings.HasPrefix(obj.Prefix, "/") {
		return false, fmt.Errorf("prefix must start with the slash char")
	}
	if obj.Hostname == "" {
		return false, fmt.Errorf("hostname must not be empty")
	}
	if strings.Contains(obj.Hostname, hostnameJoinChar) {
		return false, fmt.Errorf("hostname must not contain join char: %s", hostnameJoinChar)
	}
	if namespace == "" || strings.Contains(namespace, "/") {
		return false, fmt.Errorf("namespace is invalid")
	}

	phn := fmt.Sprintf("%shostname/%s/%s", obj.Prefix, obj.Hostname, namespace)

	c := obj.Client.GetClient()

	options := scheduler.Defaults()
	for _, optionFunc := range opts { // apply the scheduler options
		if err := optionFunc(options); err != nil {
			return false, err
		}
	}

	if options.Strategy == "" {
		return false, fmt.Errorf("strategy must be specified")
	}
	// XXX: Should we validate more options here?

	obj.schedulerMutex.Lock() // lock early even though this is only for the map

	sessionOptions := []concurrency.SessionOption{}
	// ttl for key expiry on abrupt disconnection or if persist is true!
	if options.SessionTTL > 0 {
		sessionOptions = append(sessionOptions, concurrency.WithTTL(options.SessionTTL))
	}

	// XXX: Is the previous code creating (and leaking) new sessions
	// everytime I run? Should I be caching the session instead of caching
	// the leaseID? Would an etcd reconnect break the cached session? Try
	// caching the session instead for now.
	//leaseID, exists := obj.schedulerLeases[namespace]
	//if exists {
	//	sessionOptions = append(sessionOptions, concurrency.WithLease(leaseID))
	//}
	//// create a session object
	//session, err := concurrency.NewSession(c, sessionOptions...)
	//if err != nil {
	//	return false, errwrap.Wrapf(err, "could not create session")
	//}
	////defer session.Close() // this would revoke the lease...

	isSessionDone := func(session *concurrency.Session) bool { // bonus check
		select {
		case <-session.Done():
			return true
		default:
			return false
		}
	}
	session, exists := obj.schedulerSessions[namespace]
	if !exists || isSessionDone(session) {
		var err error
		session, err = concurrency.NewSession(c, sessionOptions...)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not create session")
		}
		//defer session.Close() // this would revoke the lease...
		obj.schedulerSessions[namespace] = session
	}
	leaseID := session.Lease()

	if options.Persist {
		// If we attach a leaseID then it will *always* expire the key
		// at some point. The only way to persist a value is to not use
		// a leaseID, or to set it to zero (which AFAICT is the same as
		// not setting one) and we use 0 here to simplify the logic.
		leaseID = 0
	}
	if obj.Debug {
		obj.Logf("leaseID: %s", leaseID)
	}

	obj.schedulerMutex.Unlock()

	data := options.Save() // store the scheduler options

	if !apply {
		// TODO: untested
		phnPrefix := fmt.Sprintf("%s%s", obj.Client.GetNamespace(), phn)
		resp, err := c.KV.Get(ctx, phnPrefix)
		//resp, err := obj.Client.Get(ctx, phn)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not get add in `%s`", phn)
		}
		if resp == nil {
			return false, fmt.Errorf("could not get add in `%s`, resp is nil", phn)
		}
		if d := len(resp.Kvs); d > 1 {
			return false, fmt.Errorf("could not get add in `%s`, resp is %d", phn, d)
		}
		if len(resp.Kvs) == 0 {
			return false, nil // not found
		}

		kv := resp.Kvs[0]
		if string(kv.Value) == data && kv.Lease == int64(leaseID) {
			return true, nil // state is correct
		}
		return false, nil // state needs updating
	}

	// NOTE: If we want to store placement data for fancier scheduling, we
	// could do this in a separate key, so as to not overburden the simple
	// schedulers with having to see this extra data.
	// NOTE: With `options.SessionTTL == 0` we must *not* expire the key if
	// the session is dereferenced, but if we *don't* call session.Close(),
	// make sure this key still sticks around. It *doesn't* because if etcd
	// API, so choosing a TTL of 0 sets it to 60. It's also not possible to
	// have *any* lease which doesn't expire without a keepalive. So if you
	// want to prevent a key from being removed, don't specify a leaseID or
	// specify the magic value of 0.

	// We avoid specifying the lease (it's zero) if persist is true...
	ifOps := []etcd.Cmp{
		etcd.Compare(etcd.Value(phn), "=", data),
		etcd.Compare(etcd.LeaseValue(phn), "=", leaseID),
	}
	elsOp := []etcd.Op{
		etcd.OpPut(phn, data, etcd.WithLease(leaseID)),
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	// updating leaseID, or key expiry (deletion) both generate watch events
	txn, err := obj.Client.Txn(ctx, ifOps, nil, elsOp)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not add in `%s`", phn)
	}
	if txn.Succeeded {
		// txn did nothing... (then branch)
		return true, nil
	}

	// txn did an update...
	return false, nil
}

// SchedulerWithdraw removes this host as a candidate for scheduling within this
// namespace.
func (obj *Apparatus) SchedulerWithdraw(ctx context.Context, namespace string, apply bool) (bool, error) {
	// * key structure is /scheduler/hostname/$hostname/$namespace = <opts>
	//	** used to add/withdraw ones own hostname to/from a namespace...

	if !strings.HasSuffix(obj.Prefix, "/") {
		return false, fmt.Errorf("prefix must end with the slash char")
	}
	if !strings.HasPrefix(obj.Prefix, "/") {
		return false, fmt.Errorf("prefix must start with the slash char")
	}
	if obj.Hostname == "" {
		return false, fmt.Errorf("hostname must not be empty")
	}
	if strings.Contains(obj.Hostname, hostnameJoinChar) {
		return false, fmt.Errorf("hostname must not contain join char: %s", hostnameJoinChar)
	}
	if namespace == "" || strings.Contains(namespace, "/") {
		return false, fmt.Errorf("namespace is invalid")
	}

	phn := fmt.Sprintf("%shostname/%s/%s", obj.Prefix, obj.Hostname, namespace)

	c := obj.Client.GetClient()

	ifOps := []etcd.Cmp{
		clientv3Util.KeyMissing(phn),
	}
	elsOp := []etcd.Op{
		etcd.OpDelete(phn),
	}

	if !apply {
		// TODO: untested
		phnPrefix := fmt.Sprintf("%s%s", obj.Client.GetNamespace(), phn)
		resp, err := c.KV.Get(ctx, phnPrefix)
		//resp, err := obj.Client.Get(ctx, phn)
		if err != nil {
			return false, errwrap.Wrapf(err, "could not get withdraw in `%s`", phn)
		}
		if resp == nil {
			return false, fmt.Errorf("could not get withdraw in `%s`, resp is nil", phn)
		}
		if d := len(resp.Kvs); d > 1 {
			return false, fmt.Errorf("could not get withdraw in `%s`, resp is %d", phn, d)
		}
		if len(resp.Kvs) == 0 {
			return true, nil // state is correct
		}
		return false, nil // state needs updating
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	// updating leaseID, or key expiry (deletion) both generate watch events
	txn, err := obj.Client.Txn(ctx, ifOps, nil, elsOp)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not withdraw in `%s`", phn)
	}
	if txn.Succeeded {
		// txn did nothing... (then branch)
		return true, nil
	}

	// txn did an update...
	return false, nil
}

// SchedulerCleanup adds a session cleanup request to the Scheduler() main
// processing function if the persist arg is false. This must be run before that
// Scheduler() function terminates. Since it is usually called in the shutdown
// of Watch which *must* happen by engine policy before we Stop Background
// tasks. This lets a session be freed immediately when there are no longer any
// running resources at shutdown, rather than needing to wait for the TTL
// timeout to occur. One important subtlety is that if we graph swap and a
// particular schedule resource goes away (and doesn't change into a withdraw)
// then this will not trigger. For that kind of behaviour, we would need to hook
// into the Reversal API.
//
// If the orphan arg is true, then we free the session memory immediately, and
// this should cause your TTL to eventually run out. If your TTL is 0, then it
// should persist the data indefinitely because we wouldn't have set persist.
func (obj *Apparatus) SchedulerCleanup(namespace string, orphan bool) {
	obj.schedulerMutex.Lock()
	defer obj.schedulerMutex.Unlock()
	if !orphan && !obj.schedulerExited {
		// This tells the shutdown of Scheduler() (which happens because
		// of StopBackground) to Close() the session which expires the
		// attached leaseID immediately.
		obj.cleanupRequests[namespace] = struct{}{} // request to main!
		return
	}
	// All that follows is if persist is true...

	session, exists := obj.schedulerSessions[namespace]
	if !exists {
		// unexpected, but nothing to do!
		return
	}
	defer delete(obj.schedulerSessions, namespace) // don't leak memory!

	if !orphan { // scheduler died prematurely, might as well close these...
		session.Close() // ignore any error
		return
	}

	session.Orphan() // cancel the session goroutine
}

// ScheduledGet gets the scheduled results.
func (obj *Apparatus) ScheduledGet(ctx context.Context, namespace string) (*scheduler.ScheduledResult, error) {

	if !strings.HasSuffix(obj.Prefix, "/") {
		return nil, fmt.Errorf("prefix must end with the slash char")
	}
	if !strings.HasPrefix(obj.Prefix, "/") {
		return nil, fmt.Errorf("prefix must start with the slash char")
	}
	if namespace == "" || strings.Contains(namespace, "/") {
		return nil, fmt.Errorf("namespace is invalid")
	}

	hosts, err := obj.getScheduled(ctx, namespace)
	if err != nil {
		return &scheduler.ScheduledResult{
			Hosts: hosts,
			Err:   err, // department of redundancy department
		}, err
	}

	// Send off that data...
	return &scheduler.ScheduledResult{
		Hosts: hosts,
		Err:   nil,
	}, nil
}

// Scheduled returns the stream of scheduled results without participating.
func (obj *Apparatus) Scheduled(ctx context.Context, namespace string) (chan *scheduler.ScheduledResult, error) {
	// * key structure is /scheduler/schedule/$namespace = host1,host3,...
	//	** used for the scheduler result for each namespaced decision...

	if !strings.HasSuffix(obj.Prefix, "/") {
		return nil, fmt.Errorf("prefix must end with the slash char")
	}
	if !strings.HasPrefix(obj.Prefix, "/") {
		return nil, fmt.Errorf("prefix must start with the slash char")
	}
	if namespace == "" || strings.Contains(namespace, "/") {
		return nil, fmt.Errorf("namespace is invalid")
	}

	ctx, cancel := context.WithCancel(ctx) // wrap so we can cancel watcher!

	// stored scheduler results
	scheduledPath := fmt.Sprintf("%sschedule/%s", obj.Prefix, namespace)

	// XXX: Does this send an initial value on startup? It needs to!
	ch, err := obj.Client.Watcher(ctx, scheduledPath)
	if err != nil {
		cancel()
		return nil, err
	}

	// XXX: What about waitgroups? Does the caller need to know?
	result := make(chan *scheduler.ScheduledResult)
	go func() {
		defer cancel() // if something errors, make sure Watcher exits!
		defer close(result)
		for {
			var err error
			var hosts []string

			select {
			case event, ok := <-ch:
				if !ok {
					err = scheduler.ErrSchedulerShutdown // we shut down I guess
				} else {
					err = event // it may be an error or not
				}

			case <-ctx.Done():
				return
			}

			// We had an event!

			// XXX: It's more efficient to parse the data we receive
			// from the event, instead of re-fetching this data now,
			// but this code is simpler and more reliable like this.

			// Get data to send...
			if err == nil { // did we error above?
				hosts, err = obj.getScheduled(ctx, namespace)
			}

			// Send off that data...
			select {
			case result <- &scheduler.ScheduledResult{
				Hosts: hosts,
				Err:   err,
			}:

			case <-ctx.Done():
				return
			}

			if err != nil {
				return
			}
		}
	}()

	return result, nil
}

// getScheduled gets the list of hosts which are scheduled for a given namespace
// in etcd. This *is* the scheduling decisions, and *not* the add/withdraw data.
func (obj *Apparatus) getScheduled(ctx context.Context, namespace string) ([]string, error) {
	// * key structure is /scheduler/schedule/$namespace = host1,host3,...
	//	** used for the scheduler result for each namespaced decision...

	// This function is only called internally from functions which have
	// already done this kind of validation, so we can skip these here...
	//if !strings.HasSuffix(obj.Prefix, "/") {
	//	return nil, fmt.Errorf("prefix must end with the slash char")
	//}
	//if !strings.HasPrefix(obj.Prefix, "/") {
	//	return nil, fmt.Errorf("prefix must start with the slash char")
	//}
	//if namespace == "" || strings.Contains(namespace, "/") {
	//	return nil, fmt.Errorf("namespace is invalid")
	//}

	// stored scheduler results
	scheduledPath := fmt.Sprintf("%sschedule/%s", obj.Prefix, namespace)

	resp, err := obj.Client.Get(ctx, scheduledPath)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not get scheduled in `%s`", scheduledPath)
	}
	if resp == nil {
		return nil, fmt.Errorf("could not get scheduled in `%s`, resp is nil", scheduledPath)
	}

	if len(resp) == 0 {
		// nobody scheduled yet
		//return nil, interfaces.ErrNotExist
		return []string{}, nil
	}

	if count := len(resp); count != 1 {
		return nil, fmt.Errorf("returned %d entries", count)
	}

	val, exists := resp[scheduledPath]
	if !exists {
		return nil, fmt.Errorf("path `%s` is missing", scheduledPath)
	}
	val = strings.TrimSpace(val)

	if val == "" {
		return []string{}, nil
	}
	// don't split an empty string, we'd get [""] of length 1 which is bad!
	hosts := strings.Split(val, hostnameJoinChar)

	sort.Strings(hosts) // for consistency

	return hosts, nil
}

// getData returns scheduled data of a map[namespace] -> map[hostname] -> opts
// which is used in various functions when we want to know the state of things.
// This is *not* the scheduling decisions, this is the add/withdraw data.
// XXX: We return the parsed options, but it might be more useful to only return
// the string representation and parse them later in doScheduled only if needed.
func (obj *Apparatus) getData(ctx context.Context) (map[string]map[string]*scheduler.Options, error) {
	// * key structure is /scheduler/hostname/$hostname/$namespace = <opts>
	//	** used to add/withdraw ones own hostname to/from a namespace...

	// This function is only called internally from functions which have
	// already done this kind of validation, so we can skip these here...
	//if !strings.HasSuffix(obj.Prefix, "/") {
	//	return nil, fmt.Errorf("prefix must end with the slash char")
	//}
	//if !strings.HasPrefix(obj.Prefix, "/") {
	//	return nil, fmt.Errorf("prefix must start with the slash char")
	//}

	// list of previously scheduled namespaces
	scheduledPathAll := fmt.Sprintf("%sschedule/", obj.Prefix)

	// stored add/withdraw data
	hostnamePathPrefix := fmt.Sprintf("%shostname/", obj.Prefix)

	noopOp := []etcd.Cmp{} // always true
	thenOp := []etcd.Op{
		// TODO: I'm not sure the sort options are even needed here...
		etcd.OpGet(scheduledPathAll, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend)),
		etcd.OpGet(hostnamePathPrefix, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend)),
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we get consistent data from the exact same moment...
	txn, err := obj.Client.Txn(ctx, noopOp, thenOp, nil)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not get data in txn")
	}
	if txn == nil {
		return nil, fmt.Errorf("could not get data in txn, resp is nil")
	}
	if !txn.Succeeded {
		// programming error
		return nil, fmt.Errorf("unexpected branch")
	}
	resps := txn.Responses
	if len(resps) != 2 {
		// programming error?
		return nil, fmt.Errorf("expected two responses")
	}
	r0, r1 := resps[0], resps[1]
	if r0 == nil || r1 == nil {
		// programming error?
		return nil, fmt.Errorf("unexpected nil response")
	}
	respSchedule := r0.GetResponseRange()
	respHostname := r1.GetResponseRange()

	m := make(map[string]map[string]*scheduler.Options) // map[namespace]map[hostname]<opts>

	// We need to read the previously scheduled namespaces list so that we
	// can tell our caller that they exist, even though there might not be
	// any data (in the second resp below) for that now because that tells
	// us that the namespace still needs to be updated and we may not have
	// anything left in it right now. This can happen when we withdraw the
	// host from being scheduled, and it's important that we show an empty
	// set instead of leaving a single stale host hanging around in there!
	for _, kv := range respSchedule.Kvs {
		ns := string(kv.Key)
		//v := string(kv.Value) // currently scheduled joined hostnames
		if !strings.HasPrefix(ns, scheduledPathAll) {
			continue
		}
		ns = ns[len(scheduledPathAll):] // strip

		if _, exists := m[ns]; !exists {
			m[ns] = make(map[string]*scheduler.Options)
		}
	}

	// FIXME: the value key could instead be host specific information which
	// is used for some purpose, eg: seconds active, and other data?

	for _, kv := range respHostname.Kvs {
		k, v := string(kv.Key), string(kv.Value)

		if !strings.HasPrefix(k, hostnamePathPrefix) {
			continue
		}
		k = k[len(hostnamePathPrefix):] // strip

		// k is now $hostname/$namespace
		sp := strings.Split(k, "/")
		if len(sp) != 2 { // invalid data
			obj.Logf("invalid data split")
			continue
		}
		h := sp[0]  // hostname
		ns := sp[1] // namespace
		if len(h) == 0 || len(ns) == 0 {
			obj.Logf("invalid data length")
			continue
		}

		if _, exists := m[ns]; !exists {
			m[ns] = make(map[string]*scheduler.Options)
		}
		opts := scheduler.Defaults()
		if err := opts.Load(v); err != nil {
			//return nil, err
			obj.Logf("invalid data load")
			m[ns][h] = scheduler.Defaults() // we might have partial parsing
			continue
		}
		m[ns][h] = opts // store as parsed opts?
	}

	return m, nil
}

// doScheduled runs the actual deterministic placement scheduler and stores the
// result.
//
// XXX: is there a better name for this function?
func (obj *Apparatus) doScheduled(ctx context.Context, namespace string, hostnameMap map[string]*scheduler.Options) (bool, error) {
	// * key structure is /scheduler/schedule/$namespace = host1,host3,...
	//	** used for the scheduler result for each namespaced decision...

	hostnames := make(map[string]string)
	options := scheduler.Defaults()

	// Here we have the scheduler.Options from each separate host... Since
	// each contributes the (hopefully identical) set of options, we need to
	// merge them safely if we can, and use the resultant decision. If we
	// have inconsistent options, the behaviour is undefined and we can do
	// whatever we want to.

	for host, opts := range hostnameMap { // map[hostname] -> opts
		hostnames[host] = "TODO" // XXX: add host specific data...
		// NOTE: merge order is stochastic here inside the map loop!
		options.Merge(opts) // mutate options
	}

	// Build the strategy here. We invalidate this elsewhere when we pass it
	// off to another host. This gives it persistent memory as long as it is
	// persistent on this host! We could theoretically transfer the state to
	// another host when we switch, but that's just likely unnecessary work.
	// TODO: Lock here if we make doScheduled concurrent
	strategy, exists := obj.strategy[namespace]
	if !exists {
		var err error
		strategy, err = scheduler.Lookup(options.Strategy)
		if err != nil {
			return false, err
		}
		obj.strategy[namespace] = strategy
	}
	// TODO: Unlock here if we make doScheduled concurrent

	// This is where Debug and Logf are actually passed into the scheduler!
	params := &scheduler.Params{
		Options: options,

		Last: func(ctx context.Context) ([]string, error) {
			return obj.getScheduled(ctx, namespace)
		},

		Debug: obj.Debug,
		Logf:  obj.Logf, // TODO: wrap?
	}

	// run actual scheduler and decide who should be chosen
	// TODO: is there any additional data that we can pass
	// to the scheduler so it can make a better decision ?
	hosts, err := strategy.Schedule(ctx, hostnames, params)
	if err != nil {
		return false, errwrap.Wrapf(err, "strategy failed")
	}
	// Check to make sure the scheduler didn't send an invalid list.
	for _, h := range hosts {
		if _, exists := hostnames[h]; !exists {
			return false, errwrap.Wrapf(err, "invalid or duplicate hostname: %s", h)
		}
		delete(hostnames, h) // so we can check for duplicates!
	}
	sort.Strings(hosts) // for consistency

	obj.Logf("namespace(%s): storing...")

	// stored scheduler results
	scheduledPath := fmt.Sprintf("%sschedule/%s", obj.Prefix, namespace)

	data := strings.Join(hosts, hostnameJoinChar)
	ifOps := []etcd.Cmp{
		etcd.Compare(etcd.Value(scheduledPath), "=", data),
	}
	elsOp := []etcd.Op{
		etcd.OpPut(scheduledPath, data),
	}

	// When the last host in a scheduled namespace is removed, just delete
	// that entry entirely! This has the same effect as read from an empty
	// namespace but it also solves the lifecycle problem of preventing an
	// accumulation of unused entries that would otherwise waste memory...
	if len(hosts) == 0 { // perform a delete
		ifOps = []etcd.Cmp{
			clientv3Util.KeyMissing(scheduledPath),
		}
		elsOp = []etcd.Op{
			etcd.OpDelete(scheduledPath),
		}
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	// updating leaseID, or key expiry (deletion) both generate watch events
	txn, err := obj.Client.Txn(ctx, ifOps, nil, elsOp)
	if err != nil {
		return false, fmt.Errorf("could not set scheduling result in `%s`", scheduledPath)
	}
	if txn.Succeeded {
		// txn did nothing...
		return true, nil
	}

	// txn did an update...
	obj.Logf("namespace(%s): stored: %s", namespace, data)
	return false, nil
}
