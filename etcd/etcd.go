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
// agent at it with --seeds.
//
// # Algorithm
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
// # Notes
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
// # Smoke testing
//
// Here is a simple way to test etcd clustering basics...
//
//	./mgmt run --tmp-prefix --no-pgp --hostname h1 empty
//	./mgmt run --tmp-prefix --no-pgp --hostname h2 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381 --server-urls=http://127.0.0.1:2382 empty
//	./mgmt run --tmp-prefix --no-pgp --hostname h3 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2383 --server-urls=http://127.0.0.1:2384 empty
//	ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/chooser/dynamicsize/idealclustersize 3
//	./mgmt run --tmp-prefix --no-pgp --hostname h4 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2385 --server-urls=http://127.0.0.1:2386 empty
//	./mgmt run --tmp-prefix --no-pgp --hostname h5 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2387 --server-urls=http://127.0.0.1:2388 empty
//	ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list
//	ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 put /_mgmt/chooser/dynamicsize/idealclustersize 5
//	ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list
//
// # Bugs
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
	etcdUtil "github.com/purpleidea/mgmt/etcd/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
	etcd "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const (
	// TODO: figure out a trailing slash convention...

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

	// SessionTTL is the number of seconds to wait before a dead or
	// unresponsive host has their volunteer keys removed from the cluster.
	// This should be an integer multiple of seconds, since one second is
	// the TTL precision used in etcd.
	SessionTTL = 10 * time.Second // seconds

	// ConvergerHostnameNamespace is a unique key used in the converger.
	ConvergerHostnameNamespace = "etcd-hostname"
)

// EmbdEtcd provides the embedded server and client etcd functionality. The
// "elastic" functionality has been removed for the time being because it was
// too complicated and not stable. Much better to use mcl to build that system.
type EmbdEtcd struct { // EMBeddeD etcd
	// Hostname is the unique identifier for this host.
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

	// NoNetwork causes this to use unix:// sockets instead of TCP for
	// connections.
	NoNetwork bool

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

	wg       *sync.WaitGroup // sync group for tunnel go routines
	err      error
	errMutex *sync.Mutex // guards err

	readySignal chan struct{} // closes when we're up and running
	exitsSignal chan struct{} // closes when run exits

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

	// server related
	server            *embed.Etcd            // contains the server struct
	serverID          uint64                 // uint64 because memberRemove uses that
	serverReadySignal *util.SubscribedSignal // signals when server is up and running
	serverExitsSignal *util.SubscribedSignal // signals when runServer exits
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

	if obj.NoNetwork {
		if len(obj.Seeds) != 0 || len(obj.ClientURLs) != 0 || len(obj.ServerURLs) != 0 {
			return fmt.Errorf("option NoNetwork is mutually exclusive with Seeds, ClientURLs and ServerURLs")
		}
	}

	if obj.NS == "/" {
		return fmt.Errorf("the namespace should be empty instead of /")
	}
	if strings.HasSuffix(obj.NS, "/") {
		return fmt.Errorf("the namespace should not end in /")
	}

	// Check that ClientURLs and ServerURLs don't share the same host:port,
	// since both services need their own unique socket to listen on.
	clientHosts := make(map[string]struct{})
	for _, u := range obj.ClientURLs {
		clientHosts[u.Host] = struct{}{}
	}
	for _, u := range obj.ServerURLs {
		if _, exists := clientHosts[u.Host]; exists {
			return fmt.Errorf("the --client-urls and --server-urls share the same host:port (%s), each needs a unique socket", u.Host)
		}
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
	if len(etcdUtil.LocalhostURLs(obj.ClientURLs)) == 0 {
		u, err := url.Parse(DefaultClientURL)
		if err != nil {
			return err
		}
		obj.ClientURLs = append([]url.URL{*u}, obj.ClientURLs...) // prepend
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

	if err := os.MkdirAll(obj.Prefix, 0770); err != nil {
		return errwrap.Wrapf(err, "couldn't mkdir: %s", obj.Prefix)
	}

	obj.wg = &sync.WaitGroup{}

	obj.readySignal = make(chan struct{})
	obj.exitsSignal = make(chan struct{})

	// locally tracked state
	obj.membermap = make(etcdtypes.URLsMap)
	obj.endpoints = make(etcdtypes.URLsMap)
	obj.memberIDs = make(map[string]uint64)

	// server related
	obj.serverReadySignal = &util.SubscribedSignal{}
	obj.serverExitsSignal = &util.SubscribedSignal{}

	return nil
}

// Cleanup cleans up after you are done using the struct.
func (obj *EmbdEtcd) Cleanup() error {
	var reterr error

	return reterr
}

// curls returns the client urls that we should use everywhere except for
// locally, where we prefer to use the non-advertised perspective.
func (obj *EmbdEtcd) curls() (etcdtypes.URLs, error) {
	// TODO: do we need the copy?
	if len(obj.AClientURLs) > 0 {
		return etcdUtil.CopyURLs(obj.AClientURLs)
	}
	return etcdUtil.CopyURLs(obj.ClientURLs)
}

// surls returns the server (peer) urls that we should use everywhere except for
// locally, where we prefer to use the non-advertised perspective.
func (obj *EmbdEtcd) surls() (etcdtypes.URLs, error) {
	// TODO: do we need the copy?
	if len(obj.AServerURLs) > 0 {
		return etcdUtil.CopyURLs(obj.AServerURLs)
	}
	return etcdUtil.CopyURLs(obj.ServerURLs)
}

// Run is the main entry point to kick off the embedded etcd server. It blocks
// until we've exited for shutdown. The shutdown can be triggered by cancelling
// the context.
func (obj *EmbdEtcd) Run(ctx context.Context) error {
	//curls, err := obj.curls()
	//if err != nil {
	//	return err
	//}
	surls, err := obj.surls()
	if err != nil {
		return err
	}
	peerURLsMap := make(etcdtypes.URLsMap)
	peerURLsMap[obj.Hostname] = surls // XXX: is this right?

	obj.Logf("running...")
	defer obj.Logf("exited!")
	defer close(obj.exitsSignal)
	obj.wg = &sync.WaitGroup{}
	defer obj.wg.Wait()

	serverReady, ackReady := obj.ServerReady()    // must call ack!
	serverExited, ackExited := obj.ServerExited() // must call ack!

	//var sendError = false
	var serverErr error
	obj.Logf("waiting for server...")

	wg := &sync.WaitGroup{}
	wg.Add(1)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer wg.Done()

		// blocks until server exits
		newCluster := true
		serverErr = obj.runServer(ctx, newCluster, peerURLsMap)
		if serverErr != nil && serverErr != context.Canceled {
			// TODO: why isn't this error seen elsewhere?
			// TODO: shouldn't it get propagated somewhere?
			obj.Logf("runServer exited with: %+v", serverErr)
		}
		// in case this exits on its own instead of with destroy

		//if sendError && serverErr != nil { // exited with an error
		//	select {
		//	//case obj.errChan <- errwrap.Wrapf(serverErr, "runServer errored"): // ???
		//	case <-ctx.Done():
		//	}
		//}
	}()

	// block until either server is ready or an early exit occurs
	// we *don't* have a ctx here since we expect ctx closing to cause this!
	select {
	case <-serverReady:
		// detach from our local return of errors from an early
		// server exit (pre server ready) and switch to channel
		//sendError = true // gets set before the ackReady() does
		ackReady()  // must be called
		ackExited() // must be called
		// pass

	case <-serverExited:
		ackExited() // must be called
		ackReady()  // must be called

		wg.Wait() // wait for server to finish to get early err
		return serverErr
	}

	close(obj.readySignal) // we're ready to be used now...
	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Err will contain the last error when Next shuts down. It waits for all the
// running processes to exit before it returns.
func (obj *EmbdEtcd) Err() error {
	obj.wg.Wait()
	return obj.err
}

// errAppend is a simple helper function.
func (obj *EmbdEtcd) errAppend(err error) {
	obj.errMutex.Lock()
	obj.err = errwrap.Append(obj.err, err)
	obj.errMutex.Unlock()
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
	endpoints := etcdUtil.FromURLsMapToStringList(obj.endpoints) // flatten map
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
