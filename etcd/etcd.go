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

// TODO: remove race around leader operations
// TODO: fix unstarted member
// TODO: add VIP for servers (incorporate with net resource)
// TODO: auto assign ports/ip's for peers (if possible)
// TODO: check the shutdown ordering, so everything unrolls to a shutdown
// TODO: add the converger Register/Unregister stuff and timers if needed

// Package etcd implements the distributed key value store and fs integration.
// This also takes care of managing and clustering of the embedded etcd server.
// The automatic clustering is considered experimental. If you require a more
// robust, battle-test etcd cluster, then manage your own, and point each mgmt
// agent at it with --seeds and --no-server.
//
// Algorithm
//
// The elastic etcd algorithm works in the following way:
//
// * When you start up mgmt, you can pass it a list of seeds.
//
// * If no seeds are given, then assume you are the first server and startup.
//
// * If a seed is given, connect as a client, and volunteer to be a server.
//
// * All volunteering clients should listen for a message for nomination.
//
// * If a client has been nominated, it should startup a server.
//
// * A server should shutdown if its nomination is removed.
//
// * The elected leader should decide who to nominate/unnominate as needed.
//
// Notes
//
// If you attempt to add a new member to the cluster with a duplicate hostname,
// then the behaviour is undefined, and you could bork your cluster. This is not
// recommended or supported. Please ensure that your hostnames are unique.
//
// A single ^C requests an orderly shutdown, however a third ^C will ask etcd to
// shutdown forcefully. It is not recommended that you use this option, it
// exists as a way to make exit easier if something deadlocked the cluster. If
// this was due to user error (eg: duplicate hostnames) then it was your fault,
// but if the member did not shutdown from a single ^C under normal
// circumstances, then please file a bug.
//
// There are currently some races in this implementation. In practice, this
// should not cause any adverse effects unless you simultaneously add or remove
// members at a high rate. Fixing these races will probably require some
// internal changes to etcd. Help is welcome if you're interested in working on
// this.
//
// Smoke testing
//
// Here is a simple way to test etcd clustering basics...
//
//  ./mgmt run --tmp-prefix --no-pgp --hostname h1 empty
//  ./mgmt run --tmp-prefix --no-pgp --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 empty
//  ./mgmt run --tmp-prefix --no-pgp --hostname h3 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 empty
//  ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/chooser/dynamicsize/idealclustersize 3
//  ./mgmt run --tmp-prefix --no-pgp --hostname h4 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386 empty
//  ./mgmt run --tmp-prefix --no-pgp --hostname h5 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2387 --server-urls http://127.0.0.1:2388 empty
//  ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list
//  ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 put /_mgmt/chooser/dynamicsize/idealclustersize 5
//  ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list
//
// Bugs
//
// A member might occasionally think that an endpoint still exists after it has
// already shutdown. This isn't a major issue, since if that endpoint doesn't
// respond, then it will automatically choose the next available one. To see
// this issue, turn on debugging and start: H1, H2, H3, then stop H2, and you
// might see that H3 still knows about H2.
//
// Shutting down a cluster by setting the idealclustersize to zero is currently
// buggy and not supported. Try this at your own risk.
//
// If a member is nominated, and it doesn't respond to the nominate event and
// startup, and we lost quorum to add it, then we could be in a blocked state.
// This can be improved upon if we can call memberRemove after a timeout.
//
// Adding new cluster members very quickly, might trigger a:
// `runtime error: error validating peerURLs ... member count is unequal` error.
// See: https://github.com/etcd-io/etcd/issues/10626 for more information.
//
// If you use the dynamic size feature to start and stop the server process,
// once it has already started and then stopped, it can't be re-started because
// of a bug in etcd that doesn't free the port. Instead you'll get a:
// `bind: address already in use` error. See:
// https://github.com/etcd-io/etcd/issues/6042 for more information.
package etcd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/etcd/chooser"
	"github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "github.com/coreos/etcd/clientv3" // "clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/coreos/etcd/clientv3/namespace"
	"github.com/coreos/etcd/embed"
	etcdtypes "github.com/coreos/etcd/pkg/types"
)

const (
	// TODO: figure out a trailing slash convention...

	// NominatedPath is the unprefixed path under which nominated hosts are
	// stored. This is public so that other consumers can know to avoid this
	// key prefix.
	NominatedPath    = "/nominated/"
	nominatedPathFmt = NominatedPath + "%s" // takes a hostname on the end

	// VolunteerPath is the unprefixed path under which volunteering hosts
	// are stored. This is public so that other consumers can know to avoid
	// this key prefix.
	VolunteerPath    = "/volunteer/"
	volunteerPathFmt = VolunteerPath + "%s" // takes a hostname on the end

	// EndpointsPath is the unprefixed path under which the advertised host
	// endpoints are stored. This is public so that other consumers can know
	// to avoid this key prefix.
	EndpointsPath    = "/endpoints/"
	endpointsPathFmt = EndpointsPath + "%s" // takes a hostname on the end

	// ChooserPath is the unprefixed path under which the chooser algorithm
	// may store data. This is public so that other consumers can know to
	// avoid this key prefix.
	ChooserPath = "/chooser" // all hosts share the same namespace

	// ConvergedPath is the unprefixed path under which the converger
	// may store data. This is public so that other consumers can know to
	// avoid this key prefix.
	ConvergedPath    = "/converged/"
	convergedPathFmt = ConvergedPath + "%s" // takes a hostname on the end

	// SchedulerPath is the unprefixed path under which the scheduler
	// may store data. This is public so that other consumers can know to
	// avoid this key prefix.
	SchedulerPath    = "/scheduler/"
	schedulerPathFmt = SchedulerPath + "%s" // takes a namespace on the end

	// DefaultClientURL is the default value that is used for client URLs.
	// It is pulled from the upstream etcd package.
	DefaultClientURL = embed.DefaultListenClientURLs // 127.0.0.1:2379

	// DefaultServerURL is the default value that is used for server URLs.
	// It is pulled from the upstream etcd package.
	DefaultServerURL = embed.DefaultListenPeerURLs // 127.0.0.1:2380

	// DefaultMaxTxnOps is the maximum number of operations to run in a
	// single etcd transaction. If you exceed this limit, it is possible
	// that you have either an extremely large code base, or that you have
	// some code which is possibly not as efficient as it could be. Let us
	// know so that we can analyze the situation, and increase this if
	// necessary.
	DefaultMaxTxnOps = 512

	// RunStartupTimeout is the amount of time we will wait for regular run
	// startup before cancelling it all.
	RunStartupTimeout = 30 * time.Second

	// ClientDialTimeout is the DialTimeout option in the client config.
	ClientDialTimeout = 5 * time.Second

	// ClientDialKeepAliveTime is the DialKeepAliveTime config value for the
	// etcd client. It is recommended that you use this so that dead
	// endpoints don't block any cluster operations.
	ClientDialKeepAliveTime = 2 * time.Second // from etcdctl
	// ClientDialKeepAliveTimeout is the DialKeepAliveTimeout config value
	// for the etcd client. It is recommended that you use this so that dead
	// endpoints don't block any cluster operations.
	ClientDialKeepAliveTimeout = 6 * time.Second // from etcdctl

	// MemberChangeInterval is the polling interval to use when watching for
	// member changes during add or remove.
	MemberChangeInterval = 500 * time.Millisecond

	// SelfRemoveTimeout gives unnominated members a chance to self exit.
	SelfRemoveTimeout = 10 * time.Second

	// ForceExitTimeout is the amount of time we will wait for a force exit
	// to occur before cancelling it all.
	ForceExitTimeout = 15 * time.Second

	// SessionTTL is the number of seconds to wait before a dead or
	// unresponsive host has their volunteer keys removed from the cluster.
	// This should be an integer multiple of seconds, since one second is
	// the TTL precision used in etcd.
	SessionTTL = 10 * time.Second // seconds

	// RequireLeaderCtx specifies whether the volunteer loop should use the
	// WithRequireLeader ctx wrapper. It is unknown at this time if this
	// would cause occasional events to be lost, more extensive testing is
	// needed.
	RequireLeaderCtx = false

	// ConvergerHostnameNamespace is a unique key used in the converger.
	ConvergerHostnameNamespace = "etcd-hostname"
)

// EmbdEtcd provides the embedded server and client etcd functionality.
type EmbdEtcd struct { // EMBeddeD etcd
	Hostname string

	// Seeds is the list of servers that this client could connect to.
	Seeds etcdtypes.URLs

	// ClientURLs are the locations to listen for clients if i am a server.
	ClientURLs etcdtypes.URLs
	// ServerURLs are the locations to listen for servers (peers) if i am a
	// server (peer).
	ServerURLs etcdtypes.URLs
	// AClientURLs are the client urls to advertise.
	AClientURLs etcdtypes.URLs
	// AServerURLscare the server (peer) urls to advertise.
	AServerURLs etcdtypes.URLs

	// NoServer disables all server peering for this host.
	// TODO: allow changing this at runtime with some function call?
	NoServer bool
	// NoNetwork causes this to use unix:// sockets instead of TCP for
	// connections.
	NoNetwork bool

	// Chooser is the implementation of the algorithm that decides which
	// hosts to add or remove to grow and shrink the cluster.
	Chooser chooser.Chooser

	// Converger is a converged coordinator object that can be used to
	// track the converged state.
	Converger *converger.Coordinator

	// NS is a string namespace that we prefix to every key operation.
	NS string

	// Prefix is the directory where any etcd related state is stored. It
	// must be an absolute directory path.
	Prefix string

	Debug bool
	Logf  func(format string, v ...interface{})

	wg       *sync.WaitGroup
	exit     *util.EasyExit // exit signal
	closing  bool           // are we closing ?
	hardexit *util.EasyExit // hard exit signal (to unblock borked things)

	errChan chan error // global error chan, closes when Run is done

	// errExit1 ... errExitN all must get closed for errChan to close.
	errExit1 chan struct{} // connect
	errExit2 chan struct{} // chooser
	errExit3 chan struct{} // nominate
	errExit4 chan struct{} // volunteer
	errExit5 chan struct{} // endpoints
	errExitN chan struct{} // special signal for server closing (starts/stops)

	// coordinate an organized exit so we wait for everyone without blocking
	activeExit1   bool
	activeExit2   bool
	activeExit3   bool
	activeExit4   bool
	activeExit5   bool
	activateExit1 *util.EasyAckOnce
	activateExit2 *util.EasyAckOnce
	activateExit3 *util.EasyAckOnce
	activateExit4 *util.EasyAckOnce
	activateExit5 *util.EasyAckOnce

	readySignal chan struct{} // closes when we're up and running
	exitsSignal chan struct{} // closes when run exits

	// locally tracked state

	// nominated is a local cache of who's been nominated. This contains
	// values for where a *server* would connect to. It gets updated
	// primarily in the nominateCb watcher loop.
	// TODO: maybe this should just be a list?
	// TODO: is there a difference here between ServerURLs and AServerURLs ?
	nominated etcdtypes.URLsMap // map[hostname]URLs

	// volunteers is a local cache of who's volunteered. This contains
	// values for where a *server* would connect to. It gets updated
	// primarily in the volunteerCb watcher loop.
	// TODO: maybe this should just be a list?
	// TODO: is there a difference here between ServerURLs and AServerURLs ?
	volunteers etcdtypes.URLsMap // map[hostname]URLs

	// membermap is a local cache of server endpoints. This contains values
	// for where a *server* (peer) would connect to. It gets updated in the
	// membership state functions.
	membermap etcdtypes.URLsMap // map[hostname]URLs

	// endpoints is a local cache of server endpoints. It differs from the
	// config value which is a flattened representation of the same. That
	// value can be seen via client.Endpoints() and client.SetEndpoints().
	// This contains values for where a *client* would connect to. It gets
	// updated in the membership state functions.
	endpoints etcdtypes.URLsMap // map[hostname]URLs

	// memberIDs is a local cache of which cluster servers (peers) are
	// associated with each memberID. It gets updated in the membership
	// state functions. Note that unstarted members have an ID, but no name
	// yet, so they aren't included here, since that key would be the empty
	// string.
	memberIDs map[string]uint64 // map[hostname]memberID

	// behaviour mutexes
	stateMutex     *sync.RWMutex // lock around all locally tracked state
	orderingMutex  *sync.Mutex   // lock around non-concurrent changes
	nominatedMutex *sync.Mutex   // lock around nominatedCb
	volunteerMutex *sync.Mutex   // lock around volunteerCb

	// client related
	etcd          *etcd.Client
	connectSignal chan struct{}        // TODO: use a SubscribedSignal instead?
	client        *client.Simple       // provides useful helper methods
	clients       []*client.Simple     // list of registered clients
	session       *concurrency.Session // session that expires on disconnect
	leaseID       etcd.LeaseID         // the leaseID used by this session

	// server related
	server            *embed.Etcd            // contains the server struct
	serverID          uint64                 // uint64 because memberRemove uses that
	serverwg          *sync.WaitGroup        // wait for server to shutdown
	servermu          *sync.Mutex            // lock around destroy server
	serverExit        *util.EasyExit         // exit signal
	serverReadySignal *util.SubscribedSignal // signals when server is up and running
	serverExitsSignal *util.SubscribedSignal // signals when runServer exits

	// task queue state
	taskQueue        []*task
	taskQueueWg      *sync.WaitGroup
	taskQueueLock    *sync.Mutex
	taskQueueRunning bool
	taskQueueID      int
}

// sessionTTLSec transforms the time representation into the nearest number of
// seconds, which is needed by the etcd API.
func sessionTTLSec(d time.Duration) int {
	return int(d.Seconds())
}

// Validate the initial struct. This is called from Init, but can be used if you
// would like to check your configuration is correct.
func (obj *EmbdEtcd) Validate() error {
	s := sessionTTLSec(SessionTTL)
	if s <= 0 {
		return fmt.Errorf("the SessionTTL const of %s (%d sec) must be greater than zero", SessionTTL.String(), s)
	}
	if s > etcd.MaxLeaseTTL {
		return fmt.Errorf("the SessionTTL const of %s (%d sec) must be less than %d sec", SessionTTL.String(), s, etcd.MaxLeaseTTL)
	}

	if obj.Hostname == "" {
		return fmt.Errorf("the Hostname was not specified")
	}

	if obj.NoServer && len(obj.Seeds) == 0 {
		return fmt.Errorf("need at least one seed if NoServer is true")
	}

	if !obj.NoServer { // you don't need a Chooser if there's no server...
		if obj.Chooser == nil {
			return fmt.Errorf("need to specify a Chooser implementation")
		}
		if err := obj.Chooser.Validate(); err != nil {
			return errwrap.Wrapf(err, "the Chooser did not validate")
		}
	}

	if obj.NoNetwork {
		if len(obj.Seeds) != 0 || len(obj.ClientURLs) != 0 || len(obj.ServerURLs) != 0 {
			return fmt.Errorf("NoNetwork is mutually exclusive with Seeds, ClientURLs and ServerURLs")
		}
	}

	if _, err := copyURLs(obj.Seeds); err != nil { // this will validate
		return errwrap.Wrapf(err, "the Seeds are not valid")
	}

	if obj.NS == "/" {
		return fmt.Errorf("the namespace should be empty instead of /")
	}
	if strings.HasSuffix(obj.NS, "/") {
		return fmt.Errorf("the namespace should not end in /")
	}

	if obj.Prefix == "" || obj.Prefix == "/" {
		return fmt.Errorf("the prefix of `%s` is invalid", obj.Prefix)
	}

	if obj.Logf == nil {
		return fmt.Errorf("no Logf function was specified")
	}

	return nil
}

// Init initializes the struct after it has been populated as desired. You must
// not use the struct if this returns an error.
func (obj *EmbdEtcd) Init() error {
	if err := obj.Validate(); err != nil {
		return errwrap.Wrapf(err, "validate error")
	}

	if obj.ClientURLs == nil {
		obj.ClientURLs = []url.URL{} // initialize
	}
	if obj.ServerURLs == nil {
		obj.ServerURLs = []url.URL{}
	}
	if obj.AClientURLs == nil {
		obj.AClientURLs = []url.URL{}
	}
	if obj.AServerURLs == nil {
		obj.AServerURLs = []url.URL{}
	}

	curls, err := obj.curls()
	if err != nil {
		return err
	}
	surls, err := obj.surls()
	if err != nil {
		return err
	}
	if !obj.NoServer {
		// add a default
		if len(curls) == 0 {
			u, err := url.Parse(DefaultClientURL)
			if err != nil {
				return err
			}
			obj.ClientURLs = []url.URL{*u}
		}
		// add a default for local use and testing, harmless and useful!
		if len(surls) == 0 {
			u, err := url.Parse(DefaultServerURL) // default
			if err != nil {
				return err
			}
			obj.ServerURLs = []url.URL{*u}
		}

		// TODO: if we don't have any localhost URLs, should we warn so
		// that our local client can be able to connect more easily?
		if len(localhostURLs(obj.ClientURLs)) == 0 {
			u, err := url.Parse(DefaultClientURL)
			if err != nil {
				return err
			}
			obj.ClientURLs = append([]url.URL{*u}, obj.ClientURLs...) // prepend
		}
	}

	if obj.NoNetwork {
		var err error
		// FIXME: convince etcd to store these files in our obj.Prefix!
		obj.ClientURLs, err = etcdtypes.NewURLs([]string{"unix://clients.sock:0"})
		if err != nil {
			return err
		}
		obj.ServerURLs, err = etcdtypes.NewURLs([]string{"unix://servers.sock:0"})
		if err != nil {
			return err
		}
	}

	if obj.Chooser != nil {
		data := &chooser.Data{
			Hostname: obj.Hostname,
			Debug:    obj.Debug,
			Logf: func(format string, v ...interface{}) {
				obj.Logf("chooser: "+format, v...)
			},
		}
		if err := obj.Chooser.Init(data); err != nil {
			return errwrap.Wrapf(err, "error initializing chooser")
		}
	}

	if err := os.MkdirAll(obj.Prefix, 0770); err != nil {
		return errwrap.Wrapf(err, "couldn't mkdir: %s", obj.Prefix)
	}

	obj.wg = &sync.WaitGroup{}
	obj.exit = util.NewEasyExit()
	obj.hardexit = util.NewEasyExit()

	obj.errChan = make(chan error)

	obj.errExit1 = make(chan struct{})
	obj.errExit2 = make(chan struct{})
	obj.errExit3 = make(chan struct{})
	obj.errExit4 = make(chan struct{})
	obj.errExit5 = make(chan struct{})
	obj.errExitN = make(chan struct{}) // done before call to runServer!
	close(obj.errExitN)                // starts closed

	//obj.activeExit1 = false
	//obj.activeExit2 = false
	//obj.activeExit3 = false
	//obj.activeExit4 = false
	//obj.activeExit5 = false
	obj.activateExit1 = util.NewEasyAckOnce()
	obj.activateExit2 = util.NewEasyAckOnce()
	obj.activateExit3 = util.NewEasyAckOnce()
	obj.activateExit4 = util.NewEasyAckOnce()
	obj.activateExit5 = util.NewEasyAckOnce()

	obj.readySignal = make(chan struct{})
	obj.exitsSignal = make(chan struct{})

	// locally tracked state
	obj.nominated = make(etcdtypes.URLsMap)
	obj.volunteers = make(etcdtypes.URLsMap)
	obj.membermap = make(etcdtypes.URLsMap)
	obj.endpoints = make(etcdtypes.URLsMap)
	obj.memberIDs = make(map[string]uint64)

	// behaviour mutexes
	obj.stateMutex = &sync.RWMutex{}
	// TODO: I'm not sure if orderingMutex is actually required or not...
	obj.orderingMutex = &sync.Mutex{}
	obj.nominatedMutex = &sync.Mutex{}
	obj.volunteerMutex = &sync.Mutex{}

	// client related
	obj.connectSignal = make(chan struct{})
	obj.clients = []*client.Simple{}

	// server related
	obj.serverwg = &sync.WaitGroup{}
	obj.servermu = &sync.Mutex{}
	obj.serverExit = util.NewEasyExit() // is reset after destroyServer exit
	obj.serverReadySignal = &util.SubscribedSignal{}
	obj.serverExitsSignal = &util.SubscribedSignal{}

	// task queue state
	obj.taskQueue = []*task{}
	obj.taskQueueWg = &sync.WaitGroup{}
	obj.taskQueueLock = &sync.Mutex{}

	return nil
}

// Close cleans up after you are done using the struct.
func (obj *EmbdEtcd) Close() error {
	var reterr error

	if obj.Chooser != nil {
		reterr = errwrap.Append(reterr, obj.Chooser.Close())
	}

	return reterr
}

// curls returns the client urls that we should use everywhere except for
// locally, where we prefer to use the non-advertised perspective.
func (obj *EmbdEtcd) curls() (etcdtypes.URLs, error) {
	// TODO: do we need the copy?
	if len(obj.AClientURLs) > 0 {
		return copyURLs(obj.AClientURLs)
	}
	return copyURLs(obj.ClientURLs)
}

// surls returns the server (peer) urls that we should use everywhere except for
// locally, where we prefer to use the non-advertised perspective.
func (obj *EmbdEtcd) surls() (etcdtypes.URLs, error) {
	// TODO: do we need the copy?
	if len(obj.AServerURLs) > 0 {
		return copyURLs(obj.AServerURLs)
	}
	return copyURLs(obj.ServerURLs)
}

// err is an error helper that sends to the errChan.
func (obj *EmbdEtcd) err(err error) {
	select {
	case obj.errChan <- err:
	}
}

// Run is the main entry point to kick off the embedded etcd client and server.
// It blocks until we've exited for shutdown. The shutdown can be triggered by
// calling Destroy.
func (obj *EmbdEtcd) Run() error {
	curls, err := obj.curls()
	if err != nil {
		return err
	}
	surls, err := obj.surls()
	if err != nil {
		return err
	}

	exitCtx := obj.exit.Context() // local exit signal
	obj.Logf("running...")
	defer obj.Logf("exited!")
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer close(obj.exitsSignal)
	defer obj.wg.Wait()
	defer obj.exit.Done(nil) // unblock anything waiting for exit...
	startupCtx, cancel := context.WithTimeout(exitCtx, RunStartupTimeout)
	defer cancel()
	defer obj.Logf("waiting for exit cleanup...") // TODO: is this useful?

	// After we trigger a hardexit, wait for the ForceExitTimeout and then
	// cancel any remaining stuck context's. This helps prevent angry users.
	unblockCtx, runTimeout := context.WithCancel(context.Background())
	defer runTimeout()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer runTimeout()
		select {
		case <-obj.hardexit.Signal(): // bork unblocker
		case <-obj.exitsSignal:
		}

		select {
		case <-time.After(ForceExitTimeout):
		case <-obj.exitsSignal:
		}
	}()

	// main loop exit signal
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		// when all the senders on errChan have exited, we can exit too
		defer close(obj.errChan)
		// these signals guard around the errChan close operation
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			// We wait here until we're notified to know whether or
			// not this particular exit signal will be relevant...
			// This is because during some runs, we might not use
			// all of the signals, therefore we don't want to wait
			// for them!
			select {
			case <-obj.activateExit1.Wait():
			case <-exitCtx.Done():
			}
			if !obj.activeExit1 {
				return
			}
			select {
			case <-obj.errExit1:
				if obj.Debug {
					obj.Logf("exited connect loop (1)")
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-obj.activateExit2.Wait():
			case <-exitCtx.Done():
			}
			if !obj.activeExit2 {
				return
			}
			select {
			case <-obj.errExit2:
				if obj.Debug {
					obj.Logf("exited chooser loop (2)")
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-obj.activateExit3.Wait():
			case <-exitCtx.Done():
			}
			if !obj.activeExit3 {
				return
			}
			select {
			case <-obj.errExit3:
				if obj.Debug {
					obj.Logf("exited nominate loop (3)")

				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-obj.activateExit4.Wait():
			case <-exitCtx.Done():
			}
			if !obj.activeExit4 {
				return
			}
			select {
			case <-obj.errExit4:
				if obj.Debug {
					obj.Logf("exited volunteer loop (4)")
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-obj.activateExit5.Wait():
			case <-exitCtx.Done():
			}
			if !obj.activeExit5 {
				return
			}
			select {
			case <-obj.errExit5:
				if obj.Debug {
					obj.Logf("exited endpoints loop (5)")
				}
			}
		}()
		wg.Wait() // wait for all the other exit signals before this one
		select {
		case <-obj.errExitN: // last one is for server (it can start/stop)
			if obj.Debug {
				obj.Logf("exited server loop (0)")
			}
		}
	}()

	// main loop
	var reterr error
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
	Loop:
		for {
			select {
			case err, ok := <-obj.errChan:
				if !ok { // when this closes, we can shutdown
					break Loop
				}
				if err == nil {
					err = fmt.Errorf("unexpected nil error")
				}
				obj.Logf("runtime error: %+v", err)
				if reterr == nil { // happens only once
					obj.exit.Done(err) // trigger an exit in Run!
				}
				reterr = errwrap.Append(reterr, err)
			}
		}
	}()

	bootstrapping := len(obj.Seeds) == 0 // we're the first, start a server!
	canServer := !obj.NoServer

	// Opportunistic "connect events" system, so that we can connect
	// promiscuously when it's needed, instead of needing to linearize code.
	obj.activeExit1 = true // activate errExit1
	obj.activateExit1.Ack()
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.errExit1) // multi-signal for errChan close op
		if bootstrapping {
			serverReady, ackReady := obj.ServerReady()    // must call ack!
			serverExited, ackExited := obj.ServerExited() // must call ack!
			select {
			case <-serverReady:
				ackReady()  // must be called
				ackExited() // must be called

			case <-serverExited:
				ackExited() // must be called
				ackReady()  // must be called
				// send an error in case server doesn't
				// TODO: do we want this error to be sent?
				obj.err(fmt.Errorf("server exited early"))
				return

			case <-obj.exit.Signal(): // exit early on exit signal
				ackReady()  // must be called
				ackExited() // must be called
				return
			}
		}

		// Connect here. If we're bootstrapping, the server came up
		// right above us. No need to add to our endpoints manually,
		// that is done for us in the server start method.
		if err := obj.connect(); err != nil {
			obj.err(errwrap.Wrapf(err, "error during client connect"))
			return
		}
		obj.client = client.NewClientFromClient(obj.etcd)
		obj.client.Debug = obj.Debug
		obj.client.Logf = func(format string, v ...interface{}) {
			obj.Logf("client: "+format, v...)
		}
		if err := obj.client.Init(); err != nil {
			obj.err(errwrap.Wrapf(err, "error during client init"))
			return
		}

		// Build a session for making leases that expire on disconnect!
		options := []concurrency.SessionOption{
			concurrency.WithTTL(sessionTTLSec(SessionTTL)),
		}
		if obj.leaseID > 0 { // in the odd chance we ever do reconnects
			options = append(options, concurrency.WithLease(obj.leaseID))
		}
		obj.session, err = concurrency.NewSession(obj.etcd, options...)
		if err != nil {
			obj.err(errwrap.Wrapf(err, "could not create session"))
			return
		}
		obj.leaseID = obj.session.Lease()

		obj.Logf("connected!")
		if !bootstrapping { // new clients need an initial state sync...
			if err := obj.memberStateFromList(startupCtx); err != nil {
				obj.err(errwrap.Wrapf(err, "error during initial state sync"))
				return
			}
		}
		close(obj.connectSignal)
	}()
	defer func() {
		if obj.session != nil {
			obj.session.Close() // this revokes the lease...
		}

		// run cleanup functions in reverse order
		for i := len(obj.clients) - 1; i >= 0; i-- {
			obj.clients[i].Close() // ignore errs
		}
		if obj.client != nil { // in case we bailed out early
			obj.client.Close() // ignore err, but contains wg.Wait()
		}
		if obj.etcd == nil { // in case we bailed out early
			return
		}
		obj.disconnect()
		obj.Logf("disconnected!")
		//close(obj.disconnectSignal)
	}()

	obj.Logf("watching chooser...")
	chooserChan := make(chan error)
	obj.activeExit2 = true // activate errExit2
	obj.activateExit2.Ack()
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.errExit2) // multi-signal for errChan close op
		if obj.Chooser == nil {
			return
		}

		// wait till we're connected
		select {
		case <-obj.connectSignal:
		case <-exitCtx.Done():
			return // run exited early
		}

		p := obj.NS + ChooserPath
		c, err := obj.MakeClientFromNamespace(p)
		if err != nil {
			obj.err(errwrap.Wrapf(err, "error during chooser init"))
			return
		}
		if err := obj.Chooser.Connect(exitCtx, c); err != nil {
			obj.err(errwrap.Wrapf(err, "error during chooser connect"))
			return
		}

		ch, err := obj.Chooser.Watch()
		if err != nil {
			obj.err(errwrap.Wrapf(err, "error running chooser watch"))
			return
		}
		chooserChan = ch // watch it
	}()
	defer func() {
		if obj.Chooser == nil {
			return
		}
		obj.Chooser.Disconnect() // ignore error if any
	}()

	// call this once to start the server so we'll be able to connect
	if bootstrapping {
		obj.Logf("bootstrapping...")
		obj.volunteers[obj.Hostname] = surls // bootstrap this!
		obj.nominated[obj.Hostname] = surls
		// alternatively we can bootstrap like this if we add more stuff...
		//data := bootstrapWatcherData(obj.Hostname, surls) // server urls
		//if err := obj.nominateApply(data); err != nil { // fake apply
		//	return err
		//}
		// server starts inside here if bootstrapping!
		if err := obj.nominateCb(startupCtx); err != nil {
			// If while bootstrapping a new server, an existing one
			// is running on the same port, then we error this here.
			return err
		}

		// wait till we're connected
		select {
		case <-obj.connectSignal:
		case <-exitCtx.Done():
			// TODO: should this return an error?
			return nil // run exited early
		}

		// advertise our new endpoint (comes paired after nominateCb)
		if err := obj.advertise(startupCtx, obj.Hostname, curls); err != nil { // endpoints
			return errwrap.Wrapf(err, "error with endpoints advertise")
		}

		// run to add entry into our public nominated datastructure
		// FIXME: this might be redundant, but probably not harmful in
		// our bootstrapping process... it will get done in volunteerCb
		if err := obj.nominate(startupCtx, obj.Hostname, surls); err != nil {
			return errwrap.Wrapf(err, "error nominating self")
		}
	}

	// If obj.NoServer, then we don't need to start up the nominate watcher,
	// unless we're the first server... But we check that both are not true!
	if bootstrapping || canServer {
		if !bootstrapping && canServer { // wait for client!
			select {
			case <-obj.connectSignal:
			case <-exitCtx.Done():
				return nil // just exit
			}
		}

		ctx, cancel := context.WithCancel(unblockCtx)
		defer cancel()
		info, err := obj.client.ComplexWatcher(ctx, obj.NS+NominatedPath, etcd.WithPrefix())
		if err != nil {
			obj.activateExit3.Ack()
			return errwrap.Wrapf(err, "error adding nominated watcher")
		}
		obj.Logf("watching nominees...")
		obj.activeExit3 = true // activate errExit3
		obj.activateExit3.Ack()
		obj.wg.Add(1)
		go func() {
			defer obj.wg.Done()
			defer close(obj.errExit3) // multi-signal for errChan close op
			defer cancel()
			for {
				var event *interfaces.WatcherData
				var ok bool
				select {
				case event, ok = <-info.Events:
					if !ok {
						return
					}
				}

				if err := event.Err; err != nil {
					obj.err(errwrap.Wrapf(err, "nominated watcher errored"))
					continue
				}

				// on the initial created event, we populate...
				if !bootstrapping && event.Created && len(event.Events) == 0 {
					obj.Logf("populating nominated list...")
					nominated, err := obj.getNominated(ctx)
					if err != nil {
						obj.err(errwrap.Wrapf(err, "get nominated errored"))
						continue
					}
					obj.nominated = nominated

				} else if err := obj.nominateApply(event); err != nil {
					obj.err(errwrap.Wrapf(err, "nominate apply errored"))
					continue
				}

				// decide the desired state before we change it
				doStop := obj.serverAction(serverActionStop)
				doStart := obj.serverAction(serverActionStart)

				// server is running, but it should not be
				if doStop { // stop?
					// first, un advertise client urls
					// TODO: should this cause destroy server instead? does it already?
					if err := obj.advertise(ctx, obj.Hostname, nil); err != nil { // remove me
						obj.err(errwrap.Wrapf(err, "error with endpoints unadvertise"))
						continue
					}
				}

				// runServer gets started in a goroutine here...
				err := obj.nominateCb(ctx)
				if obj.Debug {
					obj.Logf("nominateCb: %+v", err)
				}

				if doStart { // start?
					if err := obj.advertise(ctx, obj.Hostname, curls); err != nil { // add one
						obj.err(errwrap.Wrapf(err, "error with endpoints advertise"))
						continue
					}
				}

				if err == interfaces.ErrShutdown {
					if obj.Debug {
						obj.Logf("nominated watcher shutdown")
					}
					return
				}
				if err == nil {
					continue
				}
				obj.err(errwrap.Wrapf(err, "nominated watcher callback errored"))
				continue
			}
		}()
		defer func() {
			// wait for unnominate of self to be seen...
			select {
			case <-obj.errExit3:
			case <-obj.hardexit.Signal(): // bork unblocker
				obj.Logf("unblocked unnominate signal")
				// now unblock the server in case it's running!
				if err := obj.destroyServer(); err != nil { // sync until exited
					obj.err(errwrap.Wrapf(err, "destroyServer errored"))
					return
				}
			}
		}()
		defer func() {
			// wait for volunteer loop to exit
			select {
			case <-obj.errExit4:
			}
		}()
	}
	obj.activateExit3.Ack()

	// volunteering code (volunteer callback and initial volunteering)
	if !obj.NoServer && len(obj.ServerURLs) > 0 {
		ctx, cancel := context.WithCancel(unblockCtx)
		defer cancel() // cleanup on close...
		info, err := obj.client.ComplexWatcher(ctx, obj.NS+VolunteerPath, etcd.WithPrefix())
		if err != nil {
			obj.activateExit4.Ack()
			return errwrap.Wrapf(err, "error adding volunteer watcher")
		}
		unvolunteered := make(chan struct{})
		obj.Logf("watching volunteers...")
		obj.wg.Add(1)
		obj.activeExit4 = true // activate errExit4
		obj.activateExit4.Ack()
		go func() {
			defer obj.wg.Done()
			defer close(obj.errExit4) // multi-signal for errChan close op
			for {
				var event *interfaces.WatcherData
				var ok bool
				select {
				case event, ok = <-info.Events:
					if !ok {
						return
					}
					if err := event.Err; err != nil {
						obj.err(errwrap.Wrapf(err, "volunteer watcher errored"))
						continue
					}

				case chooserEvent, ok := <-chooserChan:
					if !ok {
						obj.Logf("got chooser shutdown...")
						chooserChan = nil // done here!
						continue
					}
					if chooserEvent != nil {
						obj.err(errwrap.Wrapf(err, "chooser watcher errored"))
						continue
					}
					obj.Logf("got chooser event...")
					event = nil // pass through the apply...
					// chooser events should poke volunteerCb
				}

				_, exists1 := obj.volunteers[obj.Hostname] // before

				// on the initial created event, we populate...
				if !bootstrapping && event != nil && event.Created && len(event.Events) == 0 {
					obj.Logf("populating volunteers list...")
					volunteers, err := obj.getVolunteers(ctx)
					if err != nil {
						obj.err(errwrap.Wrapf(err, "get volunteers errored"))
						continue
					}
					// TODO: do we need to add ourself?
					//_, exists := volunteers[obj.Hostname]
					//if !exists {
					//	volunteers[obj.Hostname] = surls
					//}
					obj.volunteers = volunteers

				} else if err := obj.volunteerApply(event); event != nil && err != nil {
					obj.err(errwrap.Wrapf(err, "volunteer apply errored"))
					continue
				}
				_, exists2 := obj.volunteers[obj.Hostname] // after

				err := obj.volunteerCb(ctx)
				if err == nil {
					// it was there, and it got removed
					if exists1 && !exists2 {
						close(unvolunteered)
					}
					continue
				}
				obj.err(errwrap.Wrapf(err, "volunteer watcher callback errored"))
				continue
			}
		}()
		defer func() {
			// wait for unvolunteer of self to be seen...
			select {
			case <-unvolunteered:
			case <-obj.hardexit.Signal(): // bork unblocker
				obj.Logf("unblocked unvolunteer signal")
			}
		}()

		// self volunteer
		obj.Logf("volunteering...")
		surls, err := obj.surls()
		if err != nil {
			return err
		}
		if err := obj.volunteer(ctx, surls); err != nil {
			return err
		}
		defer obj.volunteer(ctx, nil) // unvolunteer
		defer obj.Logf("unvolunteering...")
		defer func() {
			// Move the leader if I'm it, so that the member remove
			// chooser operation happens on a different member than
			// myself. A leaving member should not decide its fate.
			member, err := obj.moveLeaderSomewhere(ctx)
			if err != nil {
				// TODO: use obj.err ?
				obj.Logf("move leader failed with: %+v", err)
				return
			}
			if member != "" {
				obj.Logf("moved leader to: %s", member)
			}
		}()
	}
	obj.activateExit4.Ack()

	// startup endpoints watcher (to learn about other servers)
	ctx, cancel := context.WithCancel(unblockCtx)
	defer cancel() // cleanup on close...
	if err := obj.runEndpoints(ctx); err != nil {
		obj.activateExit5.Ack()
		return err
	}
	obj.activateExit5.Ack()
	// We don't set state, we only watch others, so nothing to defer close!

	if obj.Converger != nil {
		obj.Converger.AddStateFn(ConvergerHostnameNamespace, func(converged bool) error {
			// send our individual state into etcd for others to see
			// TODO: what should happen on error?
			return obj.setHostnameConverged(exitCtx, obj.Hostname, converged)
		})
		defer obj.Converger.RemoveStateFn(ConvergerHostnameNamespace)
	}

	// NOTE: Add anything else we want to start up here...

	// If we get all the way down here, *and* we're connected, we're ready!
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		select {
		case <-obj.connectSignal:
			close(obj.readySignal) // we're ready to be used now...
		case <-exitCtx.Done():
		}
	}()

	select {
	case <-exitCtx.Done():
	}
	obj.closing = true // flag to let nominateCb know we're shutting down...
	// kick off all the defer()'s....
	return reterr
}

// runEndpoints is a helper function to move all of this code into a new block.
func (obj *EmbdEtcd) runEndpoints(ctx context.Context) error {
	bootstrapping := len(obj.Seeds) == 0
	select {
	case <-obj.connectSignal:
	case <-ctx.Done():
		return nil // TODO: just exit ?
	}
	info, err := obj.client.ComplexWatcher(ctx, obj.NS+EndpointsPath, etcd.WithPrefix())
	if err != nil {
		obj.activateExit5.Ack()
		return errwrap.Wrapf(err, "error adding endpoints watcher")
	}
	obj.Logf("watching endpoints...")
	obj.wg.Add(1)
	obj.activeExit5 = true // activate errExit5
	obj.activateExit5.Ack()
	go func() {
		defer obj.wg.Done()
		defer close(obj.errExit5) // multi-signal for errChan close op
		for {
			var event *interfaces.WatcherData
			var ok bool
			select {
			case event, ok = <-info.Events:
				if !ok {
					return
				}
				if err := event.Err; err != nil {
					obj.err(errwrap.Wrapf(err, "endpoints watcher errored"))
					continue
				}
			}

			// on the initial created event, we populate...
			if !bootstrapping && event.Created && len(event.Events) == 0 {
				obj.Logf("populating endpoints list...")
				endpoints, err := obj.getEndpoints(ctx)
				if err != nil {
					obj.err(errwrap.Wrapf(err, "get endpoints errored"))
					continue
				}
				obj.endpoints = endpoints
				obj.setEndpoints()

			} else if err := obj.endpointApply(event); err != nil {
				obj.err(errwrap.Wrapf(err, "endpoint apply errored"))
				continue
			}

			// there is no endpoint callback necessary

			// TODO: do we need this member state sync?
			if err := obj.memberStateFromList(ctx); err != nil {
				obj.err(errwrap.Wrapf(err, "error during endpoint state sync"))
				continue
			}
		}
	}()

	return nil
}

// Destroy cleans up the entire embedded etcd system. Use DestroyServer if you
// only want to shutdown the embedded server portion.
func (obj *EmbdEtcd) Destroy() error {
	obj.Logf("destroy...")
	obj.exit.Done(nil) // trigger an exit in Run!

	reterr := obj.exit.Error() // wait for exit signal (block until arrival)

	obj.wg.Wait()
	return reterr
}

// Interrupt causes this member to force shutdown. It does not safely wait for
// an ordered shutdown. It is not recommended to use this unless you're borked.
func (obj *EmbdEtcd) Interrupt() error {
	obj.Logf("interrupt...")
	wg := &sync.WaitGroup{}
	var err error
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = obj.Destroy() // set return error
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-obj.exit.Signal(): // wait for Destroy to run first
		}
		obj.hardexit.Done(nil) // trigger a hard exit
	}()

	wg.Wait()
	return err
}

// Ready returns a channel that closes when we're up and running. This process
// happens when calling Run. If Run is never called, this will never happen. Our
// main startup must be running, and our client must be connected to get here.
func (obj *EmbdEtcd) Ready() <-chan struct{} { return obj.readySignal }

// Exited returns a channel that closes when we've destroyed. This process
// happens after Run exits. If Run is never called, this will never happen.
func (obj *EmbdEtcd) Exited() <-chan struct{} { return obj.exitsSignal }

// config returns the config struct to be used during the etcd client connect.
func (obj *EmbdEtcd) config() etcd.Config {
	// FIXME: filter out any urls which wouldn't resolve ?
	endpoints := fromURLsMapToStringList(obj.endpoints) // flatten map
	// We don't need to do any sort of priority sort here, since for initial
	// connect we'd be the only one, so it doesn't matter, and subsequent
	// changes are made with SetEndpoints, not here, so we never need to
	// prioritize our local endpoint.
	sort.Strings(endpoints) // sort for determinism

	if len(endpoints) == 0 { // initially, we need to use the defaults...
		for _, u := range obj.Seeds {
			endpoints = append(endpoints, u.String())
		}
	}
	// XXX: connect to our local obj.ClientURLs instead of obj.AClientURLs ?
	cfg := etcd.Config{
		Endpoints: endpoints, // eg: []string{"http://254.0.0.1:12345"}
		// RetryDialer chooses the next endpoint to use, and comes with
		// a default dialer if unspecified.
		DialTimeout: ClientDialTimeout,

		// I think the keepalive stuff is needed for endpoint health.
		DialKeepAliveTime:    ClientDialKeepAliveTime,
		DialKeepAliveTimeout: ClientDialKeepAliveTimeout,

		// 0 disables auto-sync. By default auto-sync is disabled.
		AutoSyncInterval: 0, // we do something equivalent ourselves
	}
	return cfg
}

// connect connects the client to a server. If we are the first peer, then that
// server is itself.
func (obj *EmbdEtcd) connect() error {
	obj.Logf("connect...")
	if obj.etcd != nil {
		return fmt.Errorf("already connected")
	}
	cfg := obj.config() // get config
	var err error
	obj.etcd, err = etcd.New(cfg) // connect!
	return err
}

// disconnect closes the etcd connection.
func (obj *EmbdEtcd) disconnect() error {
	obj.Logf("disconnect...")
	if obj.etcd == nil {
		return fmt.Errorf("already disconnected")
	}

	return obj.etcd.Close()
}

// MakeClient returns an etcd Client interface that is suitable for basic tasks.
// Don't run this until the Ready method has acknowledged.
func (obj *EmbdEtcd) MakeClient() (interfaces.Client, error) {
	c := client.NewClientFromClient(obj.etcd)
	if err := c.Init(); err != nil {
		return nil, err
	}
	obj.clients = append(obj.clients, c) // make sure to clean up after...
	return c, nil
}

// MakeClientFromNamespace returns an etcd Client interface that is suitable for
// basic tasks and that has a key namespace prefix. // Don't run this until the
// Ready method has acknowledged.
func (obj *EmbdEtcd) MakeClientFromNamespace(ns string) (interfaces.Client, error) {
	kv := namespace.NewKV(obj.etcd.KV, ns)
	w := namespace.NewWatcher(obj.etcd.Watcher, ns)
	c := client.NewClientFromNamespace(obj.etcd, kv, w)
	if err := c.Init(); err != nil {
		return nil, err
	}
	obj.clients = append(obj.clients, c) // make sure to clean up after...
	return c, nil
}
