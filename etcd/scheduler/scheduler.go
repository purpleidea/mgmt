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
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/clientv3util"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	// DefaultStrategy is the strategy to use if none has been specified.
	DefaultStrategy = "rr"

	// DefaultSessionTTL is the number of seconds to wait before a dead or
	// unresponsive host is removed from the scheduled pool.
	DefaultSessionTTL = 10 // seconds

	// DefaultMaxCount is the maximum number of hosts to schedule on if not
	// specified.
	DefaultMaxCount = 1 // TODO: what is the logical value to choose? +Inf?

	hostnameJoinChar = "," // char used to join and split lists of hostnames
)

// ErrEndOfResults is a sentinel that represents no more results will be coming.
var ErrEndOfResults = errors.New("scheduler: end of results")

var schedulerLeases = make(map[string]etcd.LeaseID) // process lifetime in-memory lease store

// ScheduledResult represents output from the scheduler.
type ScheduledResult struct {
	Hosts []string
	Err   error
}

// Result is what is returned when you request a scheduler. You can call methods
// on it, and it stores the necessary state while you're running. When one of
// these is produced, the scheduler has already kicked off running for you
// automatically.
type Result struct {
	results   chan *ScheduledResult
	closeFunc func() // run this when you're done with the scheduler // TODO: replace with an input `context`
}

// Next returns the next output from the scheduler when it changes. This blocks
// until a new value is available, which is why you may wish to use a context to
// cancel any read from this. It returns ErrEndOfResults if the scheduler shuts
// down.
func (obj *Result) Next(ctx context.Context) ([]string, error) {
	select {
	case val, ok := <-obj.results:
		if !ok {
			return nil, ErrEndOfResults
		}
		return val.Hosts, val.Err

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Shutdown causes everything to clean up. We no longer need the scheduler.
// TODO: should this be named Close() instead? Should it return an error?
func (obj *Result) Shutdown() {
	obj.closeFunc()
	// XXX: should we have a waitgroup to wait for it all to close?
}

// Schedule returns a scheduler result which can be queried with it's available
// methods. This automatically causes different etcd clients sharing the same
// path to discover each other and be part of the scheduled set. On close the
// keys expire and will get removed from the scheduled set. Different options
// can be passed in to customize the behaviour. Hostname represents the unique
// identifier for the caller. The behaviour is undefined if this is run more
// than once with the same path and hostname simultaneously.
func Schedule(client interfaces.Client, path string, hostname string, opts ...Option) (*Result, error) {
	c := client.GetClient()

	if strings.HasSuffix(path, "/") {
		return nil, fmt.Errorf("scheduler: path must not end with the slash char")
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("scheduler: path must start with the slash char")
	}
	if hostname == "" {
		return nil, fmt.Errorf("scheduler: hostname must not be empty")
	}
	if strings.Contains(hostname, hostnameJoinChar) {
		return nil, fmt.Errorf("scheduler: hostname must not contain join char: %s", hostnameJoinChar)
	}

	// key structure is $path/election = ???
	// key structure is $path/exchange/$hostname = ???
	// key structure is $path/scheduled = ???

	options := &schedulerOptions{ // default scheduler options
		// If reuseLease is false, then on host disconnect, that hosts
		// entry will immediately expire, and the scheduler will react
		// instantly and remove that host entry from the list. If this
		// is true, or if the host closes without a clean shutdown, it
		// will take the TTL number of seconds to remove the key. This
		// can be set using the concurrency.WithTTL option to Session.
		reuseLease: false,
		sessionTTL: DefaultSessionTTL,
		maxCount:   DefaultMaxCount,
		withdraw:   false,
	}
	for _, optionFunc := range opts { // apply the scheduler options
		optionFunc(options)
	}

	if options.strategy == nil {
		return nil, fmt.Errorf("scheduler: strategy must be specified")
	}

	sessionOptions := []concurrency.SessionOption{}

	// here we try to reuse lease between multiple runs of the code
	// TODO: is it a good idea to try and reuse the lease b/w runs?
	if options.reuseLease {
		if leaseID, exists := schedulerLeases[path]; exists {
			sessionOptions = append(sessionOptions, concurrency.WithLease(leaseID))
		}
	}
	// ttl for key expiry on abrupt disconnection or if reuseLease is true!
	if options.sessionTTL > 0 {
		sessionOptions = append(sessionOptions, concurrency.WithTTL(options.sessionTTL))
	}

	//options.debug = true // use this for local debugging
	session, err := concurrency.NewSession(c, sessionOptions...)
	if err != nil {
		return nil, errwrap.Wrapf(err, "scheduler: could not create session")
	}
	leaseID := session.Lease()
	if options.reuseLease {
		// save for next time, otherwise run session.Close() somewhere
		schedulerLeases[path] = leaseID
	}

	ctx, cancel := context.WithCancel(context.Background()) // cancel below
	//defer cancel() // do NOT do this, as it would cause an early cancel!

	// stored scheduler results
	scheduledPath := fmt.Sprintf("%s/scheduled", path)
	//scheduledPath := fmt.Sprintf("%s%s/scheduled", client.GetNamespace(), path)
	//scheduledChan := client.Watcher.Watch(ctx, scheduledPath)
	scheduledChan, err := client.Watcher(ctx, scheduledPath)
	if err != nil {
		cancel()
		return nil, errwrap.Wrapf(err, "scheduler: could not watch scheduled path")
	}

	// exchange hostname, and attach it to session (leaseID) so it expires
	// (gets deleted) when we disconnect...
	exchangePath := fmt.Sprintf("%s/exchange", path)
	//exchangePath := fmt.Sprintf("%s%s/exchange", client.GetNamespace(), path)
	exchangePathHost := fmt.Sprintf("%s/%s", exchangePath, hostname)
	exchangePathPrefix := fmt.Sprintf("%s/", exchangePath)

	// open the watch *before* we set our key so that we can see the change!
	//watchChan := client.Watcher.Watch(ctx, exchangePathPrefix, etcd.WithPrefix())
	watchChan, err := client.Watcher(ctx, exchangePathPrefix, etcd.WithPrefix())
	if err != nil {
		cancel()
		return nil, errwrap.Wrapf(err, "scheduler: could not watch exchange path")
	}

	data := "TODO" // XXX: no data to exchange alongside hostnames yet
	ifops := []etcd.Cmp{
		etcd.Compare(etcd.Value(exchangePathHost), "=", data),
		etcd.Compare(etcd.LeaseValue(exchangePathHost), "=", leaseID),
	}
	elsop := []etcd.Op{
		etcd.OpPut(exchangePathHost, data, etcd.WithLease(leaseID)),
	}

	// XXX: ideally we would get the elected "chosen" host to still edit the
	// scheduled result when there's nobody left, but this might not be
	// plausible or safe to accomplish.
	if options.withdraw {
		ifops = []etcd.Cmp{
			clientv3util.KeyMissing(exchangePathHost),
		}
		elsop = []etcd.Op{
			etcd.OpDelete(exchangePathHost),
		}
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	// updating leaseID, or key expiry (deletion) both generate watch events
	// XXX: context!!!
	if txn, err := client.Txn(context.TODO(), ifops, nil, elsop); err != nil {
		defer cancel() // cancel to avoid leaks if we exit early...
		return nil, errwrap.Wrapf(err, "could not exchange in `%s`", path)
	} else if txn.Succeeded {
		options.logf("txn did nothing...") // then branch
	} else {
		options.logf("txn did an update...")
	}

	// create an election object
	electionPath := fmt.Sprintf("%s%s/election", client.GetNamespace(), path)
	election := concurrency.NewElection(session, electionPath)
	electionChan := election.Observe(ctx)

	elected := "" // who we "assume" is elected
	wg := &sync.WaitGroup{}
	ch := make(chan *ScheduledResult)
	closeChan := make(chan struct{})
	send := func(hosts []string, err error) bool { // helper function for sending
		select {
		case ch <- &ScheduledResult{ // send
			Hosts: hosts,
			Err:   err,
		}:
			return true
		case <-closeChan: // unblock
			return false // not sent
		}
	}

	once := &sync.Once{}
	onceBody := func() { // do not call directly, use closeFunc!
		//cancel() // TODO: is this needed here?
		// request a graceful shutdown, caller must call this to
		// shutdown when they are finished with the scheduler...
		// calling this will cause their hosts channels to close
		close(closeChan) // send a close signal
	}
	closeFunc := func() {
		once.Do(onceBody)
	}
	result := &Result{
		results: ch,
		// TODO: we could accept a context to watch for cancel instead?
		closeFunc: closeFunc,
	}

	mutex := &sync.Mutex{}
	var campaignClose chan struct{}
	campaignRunning := &atomic.Bool{}
	// goroutine to vote for someone as scheduler! each participant must be
	// able to run this or nobody will be around to vote if others are down
	campaignFunc := func() {
		options.logf("starting campaign...")
		// the mutex ensures we don't fly past the wg.Wait() if someone
		// shuts down the scheduler right as we are about to start this
		// campaigning loop up. we do not want to fail unnecessarily...
		mutex.Lock()
		wg.Add(1)
		mutex.Unlock()
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				defer cancel() // run cancel to stop campaigning...
				select {
				case <-campaignClose:
					return
				case <-closeChan:
					return
				}
			}()
			for {
				// TODO: previously, this looped infinitely fast
				// TODO: add some rate limiting here for initial
				// campaigning which occasionally loops a lot...
				if options.debug {
					//fmt.Printf(".") // debug
					options.logf("campaigning...")
				}

				// "Campaign puts a value as eligible for the election.
				// It blocks until it is elected, an error occurs, or
				// the context is cancelled."

				// vote for ourselves, as it's the only host we can
				// guarantee is alive, otherwise we wouldn't be voting!
				// it would be more sensible to vote for the last valid
				// hostname to keep things more stable, but if that
				// information was stale, and that host wasn't alive,
				// then this would defeat the point of picking them!
				if err := election.Campaign(ctx, hostname); err != nil {
					if err != context.Canceled {
						send(nil, errwrap.Wrapf(err, "scheduler: error campaigning"))
					}
					return
				}
			}
		}()
	}

	go func() {
		defer close(ch)
		if !options.reuseLease {
			defer session.Close() // this revokes the lease...
		}

		defer func() {
			// XXX: should we ever resign? why would this block and thus need a context?
			if elected == hostname { // TODO: is it safe to just always do this?
				if err := election.Resign(context.TODO()); err != nil { // XXX: add a timeout?
				}
			}
			elected = "" // we don't care anymore!
		}()

		// this "last" defer (first to run) should block until the other
		// goroutine has closed so we don't Close an in-use session, etc
		defer wg.Wait()

		go func() {
			defer cancel() // run cancel to "free" Observe...

			defer wg.Wait() // also wait here if parent exits first

			select {
			case <-closeChan:
				// we want the above wg.Wait() to work if this
				// close happens. lock with the campaign start
				defer mutex.Unlock()
				mutex.Lock()
				return
			}
		}()
		hostnames := make(map[string]string)
		for {
			select {
			case val, ok := <-electionChan:
				if options.debug {
					options.logf("electionChan(%t): %+v", ok, val)
				}
				if !ok {
					if options.debug {
						options.logf("elections stream shutdown...")
					}
					electionChan = nil
					// done
					// TODO: do we need to send on error channel?
					// XXX: maybe if context was not called to exit us?

					// ensure everyone waiting on closeChan
					// gets cleaned up so we free mem, etc!
					if watchChan == nil && scheduledChan == nil { // all now closed
						closeFunc()
						return
					}
					continue

				}

				elected = string(val.Kvs[0].Value)
				//if options.debug {
				options.logf("elected: %s", elected)
				//}
				if elected != hostname { // not me!
					// start up the campaign function
					if !campaignRunning.Load() {
						campaignClose = make(chan struct{})
						campaignFunc() // run
						campaignRunning.Store(true)
					}
					continue // someone else does the scheduling...
				} else { // campaigning while i am it loops fast
					// shutdown the campaign function
					if campaignRunning.Load() { // XXX: RACE READ
						close(campaignClose)
						wg.Wait()
						campaignRunning.Store(false)
					}
				}

				// i was voted in to make the scheduling choice!

			case watchResp, ok := <-watchChan:
				if options.debug {
					options.logf("watchChan(%t): %+v", ok, watchResp)
				}
				if !ok {
					if options.debug {
						options.logf("watch stream shutdown...")
					}
					watchChan = nil
					// done
					// TODO: do we need to send on error channel?
					// XXX: maybe if context was not called to exit us?

					// ensure everyone waiting on closeChan
					// gets cleaned up so we free mem, etc!
					if electionChan == nil && scheduledChan == nil { // all now closed
						closeFunc()
						return
					}
					continue
				}

				//err := watchResp.Err()
				err := watchResp
				if err == context.Canceled {
					// channel get closed shortly...
					continue
				}
				//if watchResp.Header.Revision == 0 { // by inspection
				//	// received empty message ?
				//	// switched client connection ?
				//	// FIXME: what should we do here ?
				//	continue
				//}
				if err != nil {
					send(nil, errwrap.Wrapf(err, "scheduler: exchange watcher failed"))
					continue
				}

				options.logf("running exchange values get...")
				hosts, err := getExchanged(ctx, client, path)
				if err != nil {
					send(nil, err)
					continue
				}

				// FIXME: the value key could instead be host
				// specific information which is used for some
				// purpose, eg: seconds active, and other data?
				hostnames = hosts // reset
				if options.debug {
					options.logf("available hostnames: %+v", hostnames)
				}

			case scheduledResp, ok := <-scheduledChan:
				if options.debug {
					options.logf("scheduledChan(%t): %+v", ok, scheduledResp)
				}
				if !ok {
					if options.debug {
						options.logf("scheduled stream shutdown...")
					}
					scheduledChan = nil
					// done
					// TODO: do we need to send on error channel?
					// XXX: maybe if context was not called to exit us?

					// ensure everyone waiting on closeChan
					// gets cleaned up so we free mem, etc!
					if electionChan == nil && watchChan == nil { // all now closed
						closeFunc()
						return
					}
					continue
				}
				// event! continue below and get new result...

				// NOTE: not needed, exit this via Observe ctx cancel,
				// which will ultimately cause the chan to shutdown...
				//case <-closeChan:
				//	return
			} // end select

			if len(hostnames) == 0 {
				if options.debug {
					options.logf("zero hosts available")
				}
				continue // not enough hosts available
			}

			// if we're currently elected, make a scheduling decision
			// if not, lookup the existing leader scheduling decision
			if elected != hostname {
				options.logf("i am not the leader, running scheduling result get...")
				resp, err := client.Get(ctx, scheduledPath)
				if err != nil || resp == nil || len(resp) != 1 {
					if err != nil {
						send(nil, errwrap.Wrapf(err, "scheduler: could not get scheduling result in `%s`", path))
					} else if resp == nil {
						send(nil, fmt.Errorf("scheduler: could not get scheduling result in `%s`, resp is nil", path))
					} else if len(resp) > 1 {
						send(nil, fmt.Errorf("scheduler: could not get scheduling result in `%s`, resp kvs: %+v", path, resp))
					}
					// if len(resp) == 0, we shouldn't error
					// in that situation it's just too early...
					continue
				}

				var result string
				for _, v := range resp {
					result = v
					break // get one value
				}
				//result := string(resp.Kvs[0].Value)
				hosts := strings.Split(result, hostnameJoinChar)

				if options.debug {
					options.logf("sending hosts: %+v", hosts)
				}
				// send that on channel!
				if !send(hosts, nil) {
					//return // pass instead, let channels clean up
				}
				continue
			}

			// i am the leader, run scheduler and store result
			options.logf("i am elected, running scheduler...")

			// run actual scheduler and decide who should be chosen
			// TODO: is there any additional data that we can pass
			// to the scheduler so it can make a better decision ?
			hosts, err := options.strategy.Schedule(hostnames, options)
			if err != nil {
				send(nil, errwrap.Wrapf(err, "scheduler: strategy failed"))
				continue
			}
			sort.Strings(hosts) // for consistency

			options.logf("storing scheduling result...")

			data := strings.Join(hosts, hostnameJoinChar)
			ifops := []etcd.Cmp{
				etcd.Compare(etcd.Value(scheduledPath), "=", data),
			}
			elsop := []etcd.Op{
				etcd.OpPut(scheduledPath, data),
			}

			// it's important to do this in one transaction, and atomically, because
			// this way, we only generate one watch event, and only when it's needed
			// updating leaseID, or key expiry (deletion) both generate watch events
			// XXX: context!!!
			if _, err := client.Txn(context.TODO(), ifops, nil, elsop); err != nil {
				send(nil, errwrap.Wrapf(err, "scheduler: could not set scheduling result in `%s`", path))
				continue
			}

			if options.debug {
				options.logf("sending hosts: %+v", hosts)
			}
			// send that on channel!
			if !send(hosts, nil) {
				//return // pass instead, let channels clean up
			}
		}
	}()

	// kick off an initial campaign if none exist already...
	options.logf("checking for existing leader...")
	leaderResult, err := election.Leader(ctx)
	if err == concurrency.ErrElectionNoLeader {
		// start up the campaign function
		if !campaignRunning.Load() {
			campaignClose = make(chan struct{})
			campaignFunc()              // run
			campaignRunning.Store(true) // XXX: RACE WRITE
		}
	}
	if options.debug {
		if err != nil {
			options.logf("leader information error: %+v", err)
		} else {
			options.logf("leader information: %+v", leaderResult)
		}
	}

	return result, nil
}

// Scheduled gets the scheduled results without participating.
func Scheduled(ctx context.Context, client interfaces.Client, path string) (chan *ScheduledResult, error) {
	if strings.HasSuffix(path, "/") {
		return nil, fmt.Errorf("scheduled: path must not end with the slash char")
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("scheduled: path must start with the slash char")
	}

	// key structure is $path/election = ???
	// key structure is $path/exchange/$hostname = ???
	// key structure is $path/scheduled = ???

	// stored scheduler results
	scheduledPath := fmt.Sprintf("%s/scheduled", path)

	exchangePath := fmt.Sprintf("%s/exchange", path)
	exchangePathPrefix := fmt.Sprintf("%s/", exchangePath)

	ch, err := client.Watcher(ctx, scheduledPath)
	if err != nil {
		return nil, err
	}

	// XXX: This second event stream is so that we can detect withdraw
	// events from the exchanged data alone. If we could get the scheduler
	// to always run, then this wouldn't need to happen as we'd only look at
	// the single scheduled path for the actual results.
	watchChan, err := client.Watcher(ctx, exchangePathPrefix, etcd.WithPrefix())
	if err != nil {
		return nil, errwrap.Wrapf(err, "scheduler: could not watch exchange path")
	}

	// XXX: What about waitgroups? Does the caller need to know?
	result := make(chan *ScheduledResult)
	go func() {
		defer close(result)
		for {
			var err error
			var hosts []string

			select {
			case event, ok := <-ch:
				if !ok {
					// XXX: should this be an error?
					err = nil // we shut down I guess
				} else {
					err = event // it may be an error
				}

			case event, ok := <-watchChan:
				if !ok {
					// XXX: should this be an error?
					err = nil // we shut down I guess
				} else {
					err = event // it may be an error
				}

			case <-ctx.Done():
				return
			}

			// We had an event!

			// Get data to send...
			if err == nil { // did we error above?
				hosts, err = getScheduled(ctx, client, path)
			}

			// Send off that data...
			select {
			case result <- &ScheduledResult{
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
// in etcd.
func getScheduled(ctx context.Context, client interfaces.Client, path string) ([]string, error) {
	scheduledPath := fmt.Sprintf("%s/scheduled", path)

	keyMap, err := client.Get(ctx, scheduledPath)
	if err != nil {
		return nil, err
	}

	if len(keyMap) == 0 {
		// nobody scheduled yet
		//return nil, interfaces.ErrNotExist
		return []string{}, nil
	}

	if count := len(keyMap); count != 1 {
		return nil, fmt.Errorf("returned %d entries", count)
	}

	val, exists := keyMap[scheduledPath]
	if !exists {
		return nil, fmt.Errorf("path `%s` is missing", scheduledPath)
	}

	hosts := strings.Split(val, hostnameJoinChar)

	// XXX: special case: if we're a single host, and we're trying to
	// withdraw, then there won't be an elected member to push a new
	// scheduling result to the special key where *Scheduled* would read it.
	// As a result, remove any hosts that aren't also in the exchanged set,
	// because the withdraw *does* cause its hostname to drop from there.
	exchanged, err := getExchanged(ctx, client, path)
	if err != nil {
		return nil, err
	}

	filteredHosts := []string{}
	for _, x := range hosts {
		if _, exists := exchanged[x]; !exists {
			continue
		}

		filteredHosts = append(filteredHosts, x)
	}

	return filteredHosts, nil
}

// getExchanged gets the list of hosts which are actually presenting as
// available for scheduling.
func getExchanged(ctx context.Context, client interfaces.Client, path string) (map[string]string, error) {
	exchangePath := fmt.Sprintf("%s/exchange", path)
	exchangePathPrefix := fmt.Sprintf("%s/", exchangePath)

	resp, err := client.Get(ctx, exchangePathPrefix, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
	if err != nil {
		return nil, errwrap.Wrapf(err, "scheduler: could not get exchange values in `%s`", exchangePathPrefix)
	}
	if resp == nil {
		return nil, fmt.Errorf("scheduler: could not get exchange values in `%s`, resp is nil", exchangePathPrefix)
	}

	// FIXME: the value key could instead be host specific information which
	// is used for some purpose, eg: seconds active, and other data?
	hostnames := make(map[string]string) // reset
	for k, v := range resp {
		if !strings.HasPrefix(k, exchangePathPrefix) {
			continue
		}
		k = k[len(exchangePathPrefix):] // strip
		hostnames[k] = v
	}
	return hostnames, nil
}
