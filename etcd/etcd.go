// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// TODO: Add TTL's (eg: volunteering)
// TODO: Remove race around leader operations
// TODO: Fix server reuse issue (bind: address already in use)
// TODO: Fix unstarted member
// TODO: Fix excessive StartLoop/FinishLoop
// TODO: Add VIP for servers (incorporate with net resource)
// TODO: Auto assign ports/ip's for peers (if possible)
// TODO: Fix godoc

// Package etcd implements the distributed key value store integration.
// This also takes care of managing and clustering the embedded etcd server.
// The elastic etcd algorithm works in the following way:
// * When you start up mgmt, you can pass it a list of seeds.
// * If no seeds are given, then assume you are the first server and startup.
// * If a seed is given, connect as a client, and optionally volunteer to be a server.
// * All volunteering clients should listen for a message from the master for nomination.
// * If a client has been nominated, it should startup a server.
// * All servers should listen for their nomination to be removed and shutdown if so.
// * The elected leader should decide who to nominate/unnominate to keep the right number of servers.
//
// Smoke testing:
// mkdir /tmp/mgmt{A..E}
// ./mgmt run --hostname h1 --tmp-prefix --no-pgp yaml --yaml examples/yaml/etcd1a.yaml
// ./mgmt run --hostname h2 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 yaml --yaml examples/yaml/etcd1b.yaml
// ./mgmt run --hostname h3 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 yaml --yaml examples/yaml/etcd1c.yaml
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/idealClusterSize 3
// ./mgmt run --hostname h4 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386 yaml --yaml examples/yaml/etcd1d.yaml
// ./mgmt run --hostname h5 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2387 --server-urls http://127.0.0.1:2388 yaml --yaml examples/yaml/etcd1e.yaml
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 put /_mgmt/idealClusterSize 5
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list
package etcd

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/etcd/event"
	"github.com/purpleidea/mgmt/util"

	etcd "github.com/coreos/etcd/clientv3" // "clientv3"
	"github.com/coreos/etcd/embed"
	"github.com/coreos/etcd/etcdserver"
	rpctypes "github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	etcdtypes "github.com/coreos/etcd/pkg/types"
	raft "github.com/coreos/etcd/raft"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
)

// constant parameters which may need to be tweaked or customized
const (
	NS                      = "/_mgmt" // root namespace for mgmt operations
	seedSentinel            = "_seed"  // you must not name your hostname this
	MaxStartServerTimeout   = 60       // max number of seconds to wait for server to start
	MaxStartServerRetries   = 3        // number of times to retry starting the etcd server
	maxClientConnectRetries = 5        // number of times to retry consecutive connect failures
	selfRemoveTimeout       = 3        // give unnominated members a chance to self exit
	exitDelay               = 3        // number of sec of inactivity after exit to clean up
	DefaultIdealClusterSize = 5        // default ideal cluster size target for initial seed

	DefaultClientURL = embed.DefaultListenClientURLs // 127.0.0.1:2379
	DefaultServerURL = embed.DefaultListenPeerURLs   // 127.0.0.1:2380

	// DefaultMaxTxnOps is the maximum number of operations to run in a
	// single etcd transaction. If you exceed this limit, it is possible
	// that you have either an extremely large code base, or that you have
	// some code which is possibly not as efficient as it could be. Let us
	// know so that we can analyze the situation, and increase this if
	// necessary.
	DefaultMaxTxnOps = 512
)

var (
	errApplyDeltaEventsInconsistent = errors.New("inconsistent key in ApplyDeltaEvents")
)

// AW is a struct for the AddWatcher queue.
type AW struct {
	path       string
	opts       []etcd.OpOption
	callback   func(*RE) error
	errCheck   bool
	skipConv   bool // ask event to skip converger updates
	resp       event.Resp
	cancelFunc func() // data
}

// RE is a response + error struct since these two values often occur together.
// This is now called an event with the move to the etcd v3 API.
type RE struct {
	response  etcd.WatchResponse
	path      string
	err       error
	callback  func(*RE) error
	errCheck  bool // should we check the error of the callback?
	skipConv  bool // event skips converger updates
	retryHint bool // set to true for one event after a watcher failure
	retries   uint // number of times we've retried on error
}

// KV is a key + value struct to hold the two items together.
type KV struct {
	key   string
	value string
	opts  []etcd.OpOption
	resp  event.Resp
}

// GQ is a struct for the get queue.
type GQ struct {
	path     string
	skipConv bool
	opts     []etcd.OpOption
	resp     event.Resp
	data     map[string]string
}

// DL is a struct for the delete queue.
type DL struct {
	path string
	opts []etcd.OpOption
	resp event.Resp
	data int64
}

// TN is a struct for the txn queue.
type TN struct {
	ifcmps  []etcd.Cmp
	thenops []etcd.Op
	elseops []etcd.Op
	resp    event.Resp
	data    *etcd.TxnResponse
}

// Flags are some constant flags which are used throughout the program.
type Flags struct {
	Debug   bool // add additional log messages
	Trace   bool // add execution flow log messages
	Verbose bool // add extra log message output
}

// EmbdEtcd provides the embedded server and client etcd functionality.
type EmbdEtcd struct { // EMBeddeD etcd
	// etcd client connection related
	cLock  sync.Mutex   // client connect lock
	rLock  sync.RWMutex // client reconnect lock
	client *etcd.Client
	cError error // permanent client error
	ctxErr error // permanent ctx error

	// exit and cleanup related
	cancelLock sync.Mutex // lock for the cancels list
	cancels    []func()   // array of every cancel function for watches
	exiting    bool
	exitchan   chan struct{}
	exitchanCb chan struct{}
	exitwg     *sync.WaitGroup // wait for main loops to shutdown

	hostname            string
	memberID            uint64            // cluster membership id of server if running
	endpoints           etcdtypes.URLsMap // map of servers a client could connect to
	clientURLs          etcdtypes.URLs    // locations to listen for clients if i am a server
	serverURLs          etcdtypes.URLs    // locations to listen for servers if i am a server (peer)
	advertiseClientURLs etcdtypes.URLs    // client urls to advertise
	advertiseServerURLs etcdtypes.URLs    // server urls to advertise
	noServer            bool              // disable all server peering if true
	noNetwork           bool              // use unix:// sockets instead of TCP for clients/servers

	// local tracked state
	nominated        etcdtypes.URLsMap // copy of who's nominated to locally track state
	lastRevision     int64             // the revision id of message being processed
	idealClusterSize uint16            // ideal cluster size

	// etcd channels
	awq     chan *AW // add watch queue
	wevents chan *RE // response+error
	setq    chan *KV // set queue
	getq    chan *GQ // get queue
	delq    chan *DL // delete queue
	txnq    chan *TN // txn queue

	flags     Flags
	prefix    string                 // folder prefix to use for misc storage
	converger *converger.Coordinator // converged tracking

	// etcd server related
	serverwg    sync.WaitGroup // wait for server to shutdown
	server      *embed.Etcd    // technically this contains the server struct
	dataDir     string         // our data dir, prefix + "etcd"
	serverReady chan struct{}  // closes when ready
}

// NewEmbdEtcd creates the top level embedded etcd struct client and server obj.
func NewEmbdEtcd(hostname string, seeds, clientURLs, serverURLs, advertiseClientURLs, advertiseServerURLs etcdtypes.URLs, noServer bool, noNetwork bool, idealClusterSize uint16, flags Flags, prefix string, converger *converger.Coordinator) *EmbdEtcd {
	endpoints := make(etcdtypes.URLsMap)
	if hostname == seedSentinel { // safety
		return nil
	}
	if noServer && len(seeds) == 0 {
		log.Printf("Etcd: need at least one seed if running with --no-server!")
		return nil
	}
	if noNetwork {
		if len(clientURLs) != 0 || len(serverURLs) != 0 || len(seeds) != 0 {
			log.Printf("--no-network is mutual exclusive with --seeds, --client-urls and --server-urls")
			return nil
		}
		clientURLs, _ = etcdtypes.NewURLs([]string{"unix://clients.sock:0"})
		serverURLs, _ = etcdtypes.NewURLs([]string{"unix://servers.sock:0"})
	}

	if len(seeds) > 0 {
		endpoints[seedSentinel] = seeds
		idealClusterSize = 0 // unset, get from running cluster
	}
	obj := &EmbdEtcd{
		exitchan:   make(chan struct{}), // exit signal for main loop
		exitchanCb: make(chan struct{}),
		exitwg:     &sync.WaitGroup{},
		awq:        make(chan *AW),
		wevents:    make(chan *RE),
		setq:       make(chan *KV),
		getq:       make(chan *GQ),
		delq:       make(chan *DL),
		txnq:       make(chan *TN),

		nominated: make(etcdtypes.URLsMap),

		hostname:            hostname,
		endpoints:           endpoints,
		clientURLs:          clientURLs,
		serverURLs:          serverURLs,
		advertiseClientURLs: advertiseClientURLs,
		advertiseServerURLs: advertiseServerURLs,
		noServer:            noServer,
		noNetwork:           noNetwork,

		idealClusterSize: idealClusterSize,
		converger:        converger,
		flags:            flags,
		prefix:           prefix,
		dataDir:          path.Join(prefix, "etcd"),
		serverReady:      make(chan struct{}),
	}
	// TODO: add some sort of auto assign method for picking these defaults
	// add a default so that our local client can connect locally if needed
	if len(obj.LocalhostClientURLs()) == 0 { // if we don't have any localhost URLs
		u, err := url.Parse(DefaultClientURL)
		if err != nil {
			return nil // TODO: change interface to return an error
		}
		obj.clientURLs = append([]url.URL{*u}, obj.clientURLs...) // prepend
	}

	// add a default for local use and testing, harmless and useful!
	if !obj.noServer && len(obj.serverURLs) == 0 {
		if len(obj.endpoints) > 0 {
			obj.noServer = true // we didn't have enough to be a server
		}
		u, err := url.Parse(DefaultServerURL) // default
		if err != nil {
			return nil // TODO: change interface to return an error
		}
		obj.serverURLs = []url.URL{*u}
	}

	if converger != nil {
		converger.AddStateFn("etcd-hostname", func(converged bool) error {
			// send our individual state into etcd for others to see
			return SetHostnameConverged(obj, hostname, converged) // TODO: what should happen on error?
		})
	}

	return obj
}

// GetClient returns a handle to the raw etcd client object for those scenarios.
func (obj *EmbdEtcd) GetClient() *etcd.Client {
	return obj.client
}

// GetConfig returns the config struct to be used for the etcd client connect.
func (obj *EmbdEtcd) GetConfig() etcd.Config {
	endpoints := []string{}
	// XXX: filter out any urls which wouldn't resolve here ?
	for _, eps := range obj.endpoints { // flatten map
		for _, u := range eps {
			endpoints = append(endpoints, u.String()) // use full url including scheme
		}
	}
	sort.Strings(endpoints) // sort for determinism
	cfg := etcd.Config{
		Endpoints: endpoints,
		// RetryDialer chooses the next endpoint to use
		// it comes with a default dialer if unspecified
		DialTimeout: 5 * time.Second,
	}
	return cfg
}

// Connect connects the client to a server, and then builds the *API structs.
// If reconnect is true, it will force a reconnect with new config endpoints.
func (obj *EmbdEtcd) Connect(reconnect bool) error {
	if obj.flags.Debug {
		log.Println("Etcd: Connect...")
	}
	obj.cLock.Lock()
	defer obj.cLock.Unlock()
	if obj.cError != nil { // stop on permanent error
		return obj.cError
	}
	if obj.client != nil { // memoize
		if reconnect {
			// i think this requires the rLock when using it concurrently
			err := obj.client.Close()
			if err != nil {
				log.Printf("Etcd: (Re)Connect: Close: Error: %+v", err)
			}
			obj.client = nil // for kicks
		} else {
			return nil
		}
	}
	var emax uint16 // = 0
	for {           // loop until connect
		var err error
		cfg := obj.GetConfig()
		if eps := obj.endpoints; len(eps) > 0 {
			log.Printf("Etcd: Connect: Endpoints: %v", eps)
		} else {
			log.Printf("Etcd: Connect: Endpoints: []")
		}
		obj.client, err = etcd.New(cfg) // connect!
		if err == etcd.ErrNoAvailableEndpoints {
			emax++
			if emax > maxClientConnectRetries {
				log.Printf("Etcd: The dataDir (%s) might be inconsistent or corrupt.", obj.dataDir)
				log.Printf("Etcd: Please see: %s", "https://github.com/purpleidea/mgmt/blob/master/docs/faq.md#what-does-the-error-message-about-an-inconsistent-datadir-mean")
				obj.cError = fmt.Errorf("can't find an available endpoint")
				return obj.cError
			}
			err = &CtxDelayErr{time.Duration(emax) * time.Second, "No endpoints available yet!"} // retry with backoff...
		}
		if err != nil {
			log.Printf("Etcd: Connect: CtxError...")
			if _, e := obj.CtxError(context.TODO(), err); e != nil {
				log.Printf("Etcd: Connect: CtxError: Fatal: %v", e)
				obj.cError = e
				return e // fatal error
			}
			continue
		}
		// check if we're actually connected here, because this must
		// block if we're not connected
		if obj.client == nil {
			log.Printf("Etcd: Connect: Is nil!")
			continue
		}
		break
	}
	return nil
}

// Startup is the main entry point to kick off the embedded etcd client & server.
func (obj *EmbdEtcd) Startup() error {
	bootstrapping := len(obj.endpoints) == 0 // because value changes after start

	// connect but don't block here, because servers might not be up yet...
	go func() {
		if err := obj.Connect(false); err != nil {
			log.Printf("Etcd: Startup: Error: %v", err)
			// XXX: Now cause Startup() to exit with error somehow!
		}
	}()

	go obj.CbLoop() // start callback loop
	go obj.Loop()   // start main loop

	// TODO: implement native etcd watcher method on member API changes
	path := fmt.Sprintf("%s/nominated/", NS)
	go obj.AddWatcher(path, obj.nominateCallback, true, false, etcd.WithPrefix()) // no block

	// setup ideal cluster size watcher
	key := fmt.Sprintf("%s/idealClusterSize", NS)
	go obj.AddWatcher(key, obj.idealClusterSizeCallback, true, false) // no block

	// if we have no endpoints, it means we are bootstrapping...
	if !bootstrapping {
		log.Println("Etcd: Startup: Getting initial values...")
		if nominated, err := Nominated(obj); err == nil {
			obj.nominated = nominated // store a local copy
		} else {
			log.Printf("Etcd: Startup: Nominate lookup error.")
			obj.Destroy()
			return fmt.Errorf("Etcd: Startup: Error: %v", err)
		}

		// get initial ideal cluster size
		if idealClusterSize, err := GetClusterSize(obj); err == nil {
			obj.idealClusterSize = idealClusterSize
			log.Printf("Etcd: Startup: Ideal cluster size is: %d", idealClusterSize)
		} else {
			// perhaps the first server didn't set it yet. it's ok,
			// we can get it from the watcher if it ever gets set!
			log.Printf("Etcd: Startup: Ideal cluster size lookup error.")
		}
	}

	if !obj.noServer {
		path := fmt.Sprintf("%s/volunteers/", NS)
		go obj.AddWatcher(path, obj.volunteerCallback, true, false, etcd.WithPrefix()) // no block
	}

	// if i am alone and will have to be a server...
	if !obj.noServer && bootstrapping {
		log.Printf("Etcd: Bootstrapping...")
		surls := obj.serverURLs
		if len(obj.advertiseServerURLs) > 0 {
			surls = obj.advertiseServerURLs
		}
		// give an initial value to the obj.nominate map we keep in sync
		// this emulates Nominate(obj, obj.hostname, obj.serverURLs)
		obj.nominated[obj.hostname] = surls // initial value
		// NOTE: when we are stuck waiting for the server to start up,
		// it is probably happening on this call right here...
		obj.nominateCallback(nil) // kick this off once
	}

	// self volunteer
	if !obj.noServer && len(obj.serverURLs) > 0 {
		// we run this in a go routine because it blocks waiting for server
		surls := obj.serverURLs
		if len(obj.advertiseServerURLs) > 0 {
			surls = obj.advertiseServerURLs
		}
		log.Printf("Etcd: Startup: Volunteering...")
		go Volunteer(obj, surls)
	}

	if bootstrapping {
		if err := SetClusterSize(obj, obj.idealClusterSize); err != nil {
			log.Printf("Etcd: Startup: Ideal cluster size storage error.")
			obj.Destroy()
			return fmt.Errorf("Etcd: Startup: Error: %v", err)
		}
	}

	go obj.AddWatcher(fmt.Sprintf("%s/endpoints/", NS), obj.endpointCallback, true, false, etcd.WithPrefix())

	if err := obj.Connect(false); err != nil { // don't exit from this Startup function until connected!
		return err
	}
	return nil
}

// Destroy cleans up the entire embedded etcd system. Use DestroyServer if you
// only want to shutdown the embedded server portion.
func (obj *EmbdEtcd) Destroy() error {

	// this should also trigger an unnominate, which should cause a shutdown
	log.Printf("Etcd: Destroy: Unvolunteering...")
	if err := Volunteer(obj, nil); err != nil { // unvolunteer so we can shutdown...
		log.Printf("Etcd: Destroy: Error: %v", err) // we have a problem
	}

	obj.serverwg.Wait() // wait for server shutdown signal

	obj.exiting = true // must happen before we run the cancel functions!

	// clean up any watchers which might want to continue
	obj.cancelLock.Lock() // TODO: do we really need the lock here on exit?
	log.Printf("Etcd: Destroy: Cancelling %d operations...", len(obj.cancels))
	for _, cancelFunc := range obj.cancels {
		cancelFunc()
	}
	obj.cancelLock.Unlock()

	close(obj.exitchan) // cause main loop to exit
	close(obj.exitchanCb)

	obj.rLock.Lock()
	if obj.client != nil {
		obj.client.Close()
	}
	obj.client = nil
	obj.rLock.Unlock()

	// this happens in response to the unnominate callback. not needed here!
	//if obj.server != nil {
	//	return obj.DestroyServer()
	//}
	obj.exitwg.Wait()
	return nil
}

// CtxDelayErr requests a retry in Delta duration.
type CtxDelayErr struct {
	Delta   time.Duration
	Message string
}

func (obj *CtxDelayErr) Error() string {
	return fmt.Sprintf("CtxDelayErr(%v): %s", obj.Delta, obj.Message)
}

// CtxRetriesErr lets you retry as long as you have retries available.
// TODO: consider combining this with CtxDelayErr
type CtxRetriesErr struct {
	Retries uint
	Message string
}

func (obj *CtxRetriesErr) Error() string {
	return fmt.Sprintf("CtxRetriesErr(%v): %s", obj.Retries, obj.Message)
}

// CtxPermanentErr is a permanent failure error to notify about borkage.
type CtxPermanentErr struct {
	Message string
}

func (obj *CtxPermanentErr) Error() string {
	return fmt.Sprintf("CtxPermanentErr: %s", obj.Message)
}

// CtxReconnectErr requests a client reconnect to the new endpoint list.
type CtxReconnectErr struct {
	Message string
}

func (obj *CtxReconnectErr) Error() string {
	return fmt.Sprintf("CtxReconnectErr: %s", obj.Message)
}

// CancelCtx adds a tracked cancel function around an existing context.
func (obj *EmbdEtcd) CancelCtx(ctx context.Context) (context.Context, func()) {
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	obj.cancelLock.Lock()
	obj.cancels = append(obj.cancels, cancelFunc) // not thread-safe, needs lock
	obj.cancelLock.Unlock()
	return cancelCtx, cancelFunc
}

// TimeoutCtx adds a tracked cancel function with timeout around an existing context.
func (obj *EmbdEtcd) TimeoutCtx(ctx context.Context, t time.Duration) (context.Context, func()) {
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, t)
	obj.cancelLock.Lock()
	obj.cancels = append(obj.cancels, cancelFunc) // not thread-safe, needs lock
	obj.cancelLock.Unlock()
	return timeoutCtx, cancelFunc
}

// CtxError is called whenever there is a connection or other client problem
// that needs to be resolved before we can continue, eg: connection disconnected,
// change of server to connect to, etc... It modifies the context if needed.
func (obj *EmbdEtcd) CtxError(ctx context.Context, err error) (context.Context, error) {
	if obj.ctxErr != nil { // stop on permanent error
		return ctx, obj.ctxErr
	}
	type ctxKey string // use a non-basic type as ctx key (str can conflict)
	const ctxErr ctxKey = "ctxErr"
	const ctxIter ctxKey = "ctxIter"
	expBackoff := func(tmin, texp, iter, tmax int) time.Duration {
		// https://en.wikipedia.org/wiki/Exponential_backoff
		// tmin <= texp^iter - 1 <= tmax // TODO: check my math
		return time.Duration(math.Min(math.Max(math.Pow(float64(texp), float64(iter))-1.0, float64(tmin)), float64(tmax))) * time.Millisecond
	}
	var isTimeout bool
	var iter int // = 0
	if ctxerr, ok := ctx.Value(ctxErr).(error); ok {
		if obj.flags.Debug {
			log.Printf("Etcd: CtxError: err(%v), ctxerr(%v)", err, ctxerr)
		}
		if i, ok := ctx.Value(ctxIter).(int); ok {
			iter = i + 1 // load and increment
			if obj.flags.Debug {
				log.Printf("Etcd: CtxError: Iter: %v", iter)
			}
		}
		isTimeout = err == context.DeadlineExceeded
		if obj.flags.Debug {
			log.Printf("Etcd: CtxError: isTimeout: %v", isTimeout)
		}
		if !isTimeout {
			iter = 0 // reset timer
		}
		err = ctxerr // restore error
	} else if obj.flags.Debug {
		log.Printf("Etcd: CtxError: No value found")
	}
	ctxHelper := func(tmin, texp, tmax int) context.Context {
		t := expBackoff(tmin, texp, iter, tmax)
		if obj.flags.Debug {
			log.Printf("Etcd: CtxError: Timeout: %v", t)
		}

		ctxT, _ := obj.TimeoutCtx(ctx, t)
		ctxV := context.WithValue(ctxT, ctxIter, iter) // save iter
		ctxF := context.WithValue(ctxV, ctxErr, err)   // save err
		return ctxF
	}
	_ = ctxHelper // TODO

	isGrpc := func(e error) bool { // helper function
		return grpc.ErrorDesc(err) == e.Error()
	}

	if err == nil {
		log.Fatal("Etcd: CtxError: Error: Unexpected lack of error!")
	}
	if obj.exiting {
		obj.ctxErr = fmt.Errorf("exit in progress")
		return ctx, obj.ctxErr
	}

	// happens when we trigger the cancels during reconnect
	if err == context.Canceled {
		// TODO: do we want to create a fresh ctx here for all cancels?
		//ctx = context.Background()
		ctx, _ = obj.CancelCtx(ctx) // add a new one
		return ctx, nil             // we should retry, reconnect probably happened
	}

	if delayErr, ok := err.(*CtxDelayErr); ok { // custom delay error
		log.Printf("Etcd: CtxError: Reason: %s", delayErr.Error())
		time.Sleep(delayErr.Delta) // sleep the amount of time requested
		return ctx, nil
	}

	if retriesErr, ok := err.(*CtxRetriesErr); ok { // custom retry error
		log.Printf("Etcd: CtxError: Reason: %s", retriesErr.Error())
		if retriesErr.Retries == 0 {
			obj.ctxErr = fmt.Errorf("no more retries due to CtxRetriesErr")
			return ctx, obj.ctxErr
		}
		return ctx, nil
	}

	if permanentErr, ok := err.(*CtxPermanentErr); ok { // custom permanent error
		obj.ctxErr = fmt.Errorf("error due to CtxPermanentErr: %s", permanentErr.Error())
		return ctx, obj.ctxErr // quit
	}

	if err == etcd.ErrNoAvailableEndpoints { // etcd server is probably starting up
		// TODO: tmin, texp, tmax := 500, 2, 16000 // ms, exp base, ms
		// TODO: return ctxHelper(tmin, texp, tmax), nil
		log.Printf("Etcd: CtxError: No endpoints available yet!")
		time.Sleep(500 * time.Millisecond) // a ctx timeout won't help!
		return ctx, nil                    // passthrough
	}

	// etcd server is apparently still starting up...
	if err == rpctypes.ErrNotCapable { // isGrpc(rpctypes.ErrNotCapable) also matches
		log.Printf("Etcd: CtxError: Server is starting up...")
		time.Sleep(500 * time.Millisecond) // a ctx timeout won't help!
		return ctx, nil                    // passthrough
	}

	if err == grpc.ErrClientConnTimeout { // sometimes caused by "too many colons" misconfiguration
		return ctx, fmt.Errorf("misconfiguration: %v", err) // permanent failure?
	}

	// this can happen if my client connection shuts down, but without any
	// available alternatives. in this case, rotate it off to someone else
	reconnectErr, isReconnectErr := err.(*CtxReconnectErr) // custom reconnect error
	switch {
	case isReconnectErr:
		log.Printf("Etcd: CtxError: Reason: %s", reconnectErr.Error())
		fallthrough
	case err == raft.ErrStopped: // TODO: does this ever happen?
		fallthrough
	case err == etcdserver.ErrStopped: // TODO: does this ever happen?
		fallthrough
	case isGrpc(raft.ErrStopped):
		fallthrough
	case isGrpc(etcdserver.ErrStopped):
		fallthrough
	case isGrpc(grpc.ErrClientConnClosing):

		if obj.flags.Debug {
			log.Printf("Etcd: CtxError: Error(%T): %+v", err, err)
			log.Printf("Etcd: Endpoints are: %v", obj.client.Endpoints())
			log.Printf("Etcd: Client endpoints are: %v", obj.endpoints)
		}

		if obj.flags.Debug {
			log.Printf("Etcd: CtxError: Locking...")
		}
		obj.rLock.Lock()
		// TODO: should this really be nested inside the other lock?
		obj.cancelLock.Lock()
		// we need to cancel any WIP connections like Txn()'s and so on
		// we run the cancel()'s that are stored up so they don't block
		log.Printf("Etcd: CtxError: Cancelling %d operations...", len(obj.cancels))
		for _, cancelFunc := range obj.cancels {
			cancelFunc()
		}
		obj.cancels = []func(){} // reset
		obj.cancelLock.Unlock()

		log.Printf("Etcd: CtxError: Reconnecting...")
		if err := obj.Connect(true); err != nil {
			defer obj.rLock.Unlock()
			obj.ctxErr = fmt.Errorf("permanent connect error: %v", err)
			return ctx, obj.ctxErr
		}
		if obj.flags.Debug {
			log.Printf("Etcd: CtxError: Unlocking...")
		}
		obj.rLock.Unlock()
		log.Printf("Etcd: CtxError: Reconnected!")
		return ctx, nil
	}

	// FIXME: we might be one of the members in a two member cluster that
	// had the other member crash.. hmmm bork?!
	if isGrpc(context.DeadlineExceeded) {
		log.Printf("Etcd: CtxError: DeadlineExceeded(%T): %+v", err, err) // TODO
	}

	if err == rpctypes.ErrDuplicateKey {
		log.Fatalf("Etcd: CtxError: Programming error: %+v", err)
	}

	// if you hit this code path here, please report the unmatched error!
	log.Printf("Etcd: CtxError: Unknown error(%T): %+v", err, err)
	time.Sleep(1 * time.Second)
	obj.ctxErr = fmt.Errorf("unknown CtxError")
	return ctx, obj.ctxErr
}

// CbLoop is the loop where callback execution is serialized.
func (obj *EmbdEtcd) CbLoop() {
	obj.exitwg.Add(1)
	defer obj.exitwg.Done()
	cuid := obj.converger.Register()
	defer cuid.Unregister()
	if e := obj.Connect(false); e != nil {
		return // fatal
	}
	var exitTimeout <-chan time.Time // = nil is implied
	// we use this timer because when we ignore un-converge events and loop,
	// we reset the ConvergedTimer case statement, ruining the timeout math!
	cuid.StartTimer()
	for {
		ctx := context.Background() // TODO: inherit as input argument?
		select {
		// etcd watcher event
		case re := <-obj.wevents:
			if !re.skipConv { // if we want to count it...
				cuid.ResetTimer() // activity!
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: CbLoop: Event: StartLoop")
			}
			for {
				if obj.exiting { // the exit signal has been sent!
					//re.resp.NACK() // nope!
					break
				}
				if obj.flags.Trace {
					log.Printf("Trace: Etcd: CbLoop: rawCallback()")
				}
				err := rawCallback(ctx, re)
				if obj.flags.Trace {
					log.Printf("Trace: Etcd: CbLoop: rawCallback(): %v", err)
				}
				if err == nil {
					//re.resp.ACK() // success
					break
				}
				re.retries++ // increment error retry count
				if ctx, err = obj.CtxError(ctx, err); err != nil {
					break // TODO: it's bad, break or return?
				}
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: CbLoop: Event: FinishLoop")
			}

		// exit loop signal
		case <-obj.exitchanCb:
			obj.exitchanCb = nil
			log.Println("Etcd: Exiting loop shortly...")
			// activate exitTimeout switch which only opens after N
			// seconds of inactivity in this select switch, which
			// lets everything get bled dry to avoid blocking calls
			// which would otherwise block us from exiting cleanly!
			exitTimeout = util.TimeAfterOrBlock(exitDelay)

		// exit loop commit
		case <-exitTimeout:
			log.Println("Etcd: Exiting callback loop!")
			cuid.StopTimer() // clean up nicely
			return
		}
	}
}

// Loop is the main loop where everything is serialized.
func (obj *EmbdEtcd) Loop() {
	obj.exitwg.Add(1) // TODO: add these to other go routines?
	defer obj.exitwg.Done()
	cuid := obj.converger.Register()
	defer cuid.Unregister()
	if e := obj.Connect(false); e != nil {
		return // fatal
	}
	var exitTimeout <-chan time.Time // = nil is implied
	cuid.StartTimer()
	for {
		ctx := context.Background() // TODO: inherit as input argument?
		// priority channel...
		select {
		case aw := <-obj.awq:
			cuid.ResetTimer() // activity!
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: PriorityAW: StartLoop")
			}
			obj.loopProcessAW(ctx, aw)
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: PriorityAW: FinishLoop")
			}
			continue // loop to drain the priority channel first!
		default:
			// passthrough to normal channel
		}

		select {
		// add watcher
		case aw := <-obj.awq:
			cuid.ResetTimer() // activity!
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: AW: StartLoop")
			}
			obj.loopProcessAW(ctx, aw)
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: AW: FinishLoop")
			}

		// set kv pair
		case kv := <-obj.setq:
			cuid.ResetTimer() // activity!
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Set: StartLoop")
			}
			for {
				if obj.exiting { // the exit signal has been sent!
					kv.resp.NACK() // nope!
					break
				}
				err := obj.rawSet(ctx, kv)
				if err == nil {
					kv.resp.ACK() // success
					break
				}
				if ctx, err = obj.CtxError(ctx, err); err != nil { // try to reconnect, etc...
					break // TODO: it's bad, break or return?
				}
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Set: FinishLoop")
			}

		// get value
		case gq := <-obj.getq:
			if !gq.skipConv {
				cuid.ResetTimer() // activity!
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Get: StartLoop")
			}
			for {
				if obj.exiting { // the exit signal has been sent!
					gq.resp.NACK() // nope!
					break
				}
				data, err := obj.rawGet(ctx, gq)
				if err == nil {
					gq.data = data // update struct
					gq.resp.ACK()  // success
					break
				}
				if ctx, err = obj.CtxError(ctx, err); err != nil {
					break // TODO: it's bad, break or return?
				}
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Get: FinishLoop")
			}

		// delete value
		case dl := <-obj.delq:
			cuid.ResetTimer() // activity!
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Delete: StartLoop")
			}
			for {
				if obj.exiting { // the exit signal has been sent!
					dl.resp.NACK() // nope!
					break
				}
				data, err := obj.rawDelete(ctx, dl)
				if err == nil {
					dl.data = data // update struct
					dl.resp.ACK()  // success
					break
				}
				if ctx, err = obj.CtxError(ctx, err); err != nil {
					break // TODO: it's bad, break or return?
				}
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Delete: FinishLoop")
			}

		// run txn
		case tn := <-obj.txnq:
			cuid.ResetTimer() // activity!
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Txn: StartLoop")
			}
			for {
				if obj.exiting { // the exit signal has been sent!
					tn.resp.NACK() // nope!
					break
				}
				data, err := obj.rawTxn(ctx, tn)
				if err == nil {
					tn.data = data // update struct
					tn.resp.ACK()  // success
					break
				}
				if ctx, err = obj.CtxError(ctx, err); err != nil {
					break // TODO: it's bad, break or return?
				}
			}
			if obj.flags.Trace {
				log.Printf("Trace: Etcd: Loop: Txn: FinishLoop")
			}

		// exit loop signal
		case <-obj.exitchan:
			obj.exitchan = nil
			log.Println("Etcd: Exiting loop shortly...")
			// activate exitTimeout switch which only opens after N
			// seconds of inactivity in this select switch, which
			// lets everything get bled dry to avoid blocking calls
			// which would otherwise block us from exiting cleanly!
			exitTimeout = util.TimeAfterOrBlock(exitDelay)

		// exit loop commit
		case <-exitTimeout:
			log.Println("Etcd: Exiting loop!")
			cuid.StopTimer() // clean up nicely
			return
		}
	}
}

// loopProcessAW is a helper function to facilitate creating priority channels!
func (obj *EmbdEtcd) loopProcessAW(ctx context.Context, aw *AW) {
	for {
		if obj.exiting { // the exit signal has been sent!
			aw.resp.NACK() // nope!
			return
		}
		// cancelFunc is our data payload
		cancelFunc, err := obj.rawAddWatcher(ctx, aw)
		if err == nil {
			aw.cancelFunc = cancelFunc // update struct
			aw.resp.ACK()              // success
			return
		}
		if ctx, err = obj.CtxError(ctx, err); err != nil {
			return // TODO: do something else ?
		}
	}
}

// Set queues up a set operation to occur using our mainloop.
func (obj *EmbdEtcd) Set(key, value string, opts ...etcd.OpOption) error {
	resp := event.NewResp()
	obj.setq <- &KV{key: key, value: value, opts: opts, resp: resp}
	if err := resp.Wait(); err != nil { // wait for ack/nack
		return fmt.Errorf("Etcd: Set: Probably received an exit: %v", err)
	}
	return nil
}

// rawSet actually implements the key set operation.
func (obj *EmbdEtcd) rawSet(ctx context.Context, kv *KV) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawSet()")
	}
	// key is the full key path
	// TODO: should this be : obj.client.KV.Put or obj.client.Put ?
	obj.rLock.RLock() // these read locks need to wrap any use of obj.client
	response, err := obj.client.KV.Put(ctx, kv.key, kv.value, kv.opts...)
	obj.rLock.RUnlock()
	log.Printf("Etcd: Set(%s): %v", kv.key, response) // w00t... bonus
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawSet(): %v", err)
	}
	return err
}

// Get performs a get operation and waits for an ACK to continue.
func (obj *EmbdEtcd) Get(path string, opts ...etcd.OpOption) (map[string]string, error) {
	return obj.ComplexGet(path, false, opts...)
}

// ComplexGet performs a get operation and waits for an ACK to continue. It can
// accept more arguments that are useful for the less common operations.
// TODO: perhaps a get should never cause an un-converge ?
func (obj *EmbdEtcd) ComplexGet(path string, skipConv bool, opts ...etcd.OpOption) (map[string]string, error) {
	resp := event.NewResp()
	gq := &GQ{path: path, skipConv: skipConv, opts: opts, resp: resp, data: nil}
	obj.getq <- gq                      // send
	if err := resp.Wait(); err != nil { // wait for ack/nack
		return nil, fmt.Errorf("Etcd: Get: Probably received an exit: %v", err)
	}
	return gq.data, nil
}

func (obj *EmbdEtcd) rawGet(ctx context.Context, gq *GQ) (result map[string]string, err error) {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawGet()")
	}
	obj.rLock.RLock()
	// TODO: we're checking if this is nil to workaround a nil ptr bug...
	if obj.client == nil { // bug?
		obj.rLock.RUnlock()
		return nil, fmt.Errorf("client is nil")
	}
	if obj.client.KV == nil { // bug?
		obj.rLock.RUnlock()
		return nil, fmt.Errorf("client.KV is nil")
	}
	response, err := obj.client.KV.Get(ctx, gq.path, gq.opts...)
	obj.rLock.RUnlock()
	if err != nil || response == nil {
		return nil, err
	}

	// TODO: write a response.ToMap() function on https://godoc.org/github.com/coreos/etcd/etcdserver/etcdserverpb#RangeResponse
	result = make(map[string]string)
	for _, x := range response.Kvs {
		result[bytes.NewBuffer(x.Key).String()] = bytes.NewBuffer(x.Value).String()
	}

	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawGet(): %v", result)
	}
	return
}

// Delete performs a delete operation and waits for an ACK to continue.
func (obj *EmbdEtcd) Delete(path string, opts ...etcd.OpOption) (int64, error) {
	resp := event.NewResp()
	dl := &DL{path: path, opts: opts, resp: resp, data: -1}
	obj.delq <- dl                      // send
	if err := resp.Wait(); err != nil { // wait for ack/nack
		return -1, fmt.Errorf("Etcd: Delete: Probably received an exit: %v", err)
	}
	return dl.data, nil
}

func (obj *EmbdEtcd) rawDelete(ctx context.Context, dl *DL) (count int64, err error) {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawDelete()")
	}
	count = -1
	obj.rLock.RLock()
	response, err := obj.client.KV.Delete(ctx, dl.path, dl.opts...)
	obj.rLock.RUnlock()
	if err == nil {
		count = response.Deleted
	}
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawDelete(): %v", err)
	}
	return
}

// Txn performs a transaction and waits for an ACK to continue.
func (obj *EmbdEtcd) Txn(ifcmps []etcd.Cmp, thenops, elseops []etcd.Op) (*etcd.TxnResponse, error) {
	resp := event.NewResp()
	tn := &TN{ifcmps: ifcmps, thenops: thenops, elseops: elseops, resp: resp, data: nil}
	obj.txnq <- tn                      // send
	if err := resp.Wait(); err != nil { // wait for ack/nack
		return nil, fmt.Errorf("Etcd: Txn: Probably received an exit: %v", err)
	}
	return tn.data, nil
}

func (obj *EmbdEtcd) rawTxn(ctx context.Context, tn *TN) (*etcd.TxnResponse, error) {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawTxn()")
	}
	obj.rLock.RLock()
	response, err := obj.client.KV.Txn(ctx).If(tn.ifcmps...).Then(tn.thenops...).Else(tn.elseops...).Commit()
	obj.rLock.RUnlock()
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: rawTxn(): %v, %v", response, err)
	}
	return response, err
}

// AddWatcher queues up an add watcher request and returns a cancel function.
// Remember to add the etcd.WithPrefix() option if you want to watch recursively.
func (obj *EmbdEtcd) AddWatcher(path string, callback func(re *RE) error, errCheck bool, skipConv bool, opts ...etcd.OpOption) (func(), error) {
	resp := event.NewResp()
	awq := &AW{path: path, opts: opts, callback: callback, errCheck: errCheck, skipConv: skipConv, cancelFunc: nil, resp: resp}
	obj.awq <- awq                      // send
	if err := resp.Wait(); err != nil { // wait for ack/nack
		return nil, fmt.Errorf("Etcd: AddWatcher: Got NACK: %v", err)
	}
	return awq.cancelFunc, nil
}

// rawAddWatcher adds a watcher and returns a cancel function to call to end it.
func (obj *EmbdEtcd) rawAddWatcher(ctx context.Context, aw *AW) (func(), error) {
	cancelCtx, cancelFunc := obj.CancelCtx(ctx)
	go func(ctx context.Context) {
		defer cancelFunc() // it's safe to cancelFunc() more than once!
		obj.rLock.RLock()
		rch := obj.client.Watcher.Watch(ctx, aw.path, aw.opts...)
		obj.rLock.RUnlock()
		var rev int64
		var useRev = false
		var retry, locked bool = false, false
		for {
			response := <-rch // read
			err := response.Err()
			isCanceled := response.Canceled || err == context.Canceled
			if response.Header.Revision == 0 { // by inspection
				if obj.flags.Debug {
					log.Printf("Etcd: Watch: Received empty message!") // switched client connection
				}
				isCanceled = true
			}

			if isCanceled {
				if obj.exiting { // if not, it could be reconnect
					return
				}
				err = context.Canceled
			}

			if err == nil { // watch from latest good revision
				rev = response.Header.Revision // TODO: +1 ?
				useRev = true
				if !locked {
					retry = false
				}
				locked = false
			} else {
				if obj.flags.Debug {
					log.Printf("Etcd: Watch: Error: %v", err) // probably fixable
				}
				// this new context is the fix for a tricky set
				// of bugs which were encountered when re-using
				// the existing canceled context! it has state!
				ctx = context.Background() // this is critical!

				if ctx, err = obj.CtxError(ctx, err); err != nil {
					return // TODO: it's bad, break or return?
				}

				// remake it, but add old Rev when valid
				opts := []etcd.OpOption{}
				if useRev {
					opts = append(opts, etcd.WithRev(rev))
				}
				opts = append(opts, aw.opts...)
				rch = nil
				obj.rLock.RLock()
				if obj.client == nil {
					defer obj.rLock.RUnlock()
					return // we're exiting
				}
				rch = obj.client.Watcher.Watch(ctx, aw.path, opts...)
				obj.rLock.RUnlock()
				locked = true
				retry = true
				continue
			}

			// the response includes a list of grouped events, each
			// of which includes one Kv struct. Send these all in a
			// batched group so that they are processed together...
			obj.wevents <- &RE{response: response, path: aw.path, err: err, callback: aw.callback, errCheck: aw.errCheck, skipConv: aw.skipConv, retryHint: retry} // send event
		}
	}(cancelCtx)
	return cancelFunc, nil
}

// rawCallback is the companion to AddWatcher which runs the callback processing.
func rawCallback(ctx context.Context, re *RE) error {
	var err = re.err // the watch event itself might have had an error
	if err == nil {
		if callback := re.callback; callback != nil {
			// TODO: we could add an async option if needed
			// NOTE: the callback must *not* block!
			// FIXME: do we need to pass ctx in via *RE, or in the callback signature ?
			err = callback(re) // run the callback
			if !re.errCheck || err == nil {
				return nil
			}
		} else {
			return nil
		}
	}
	return err
}

// volunteerCallback runs to respond to the volunteer list change events.
// Functionally, it controls the adding and removing of members.
// FIXME: we might need to respond to member change/disconnect/shutdown events,
// see: https://github.com/coreos/etcd/issues/5277
func (obj *EmbdEtcd) volunteerCallback(re *RE) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: volunteerCallback()")
		defer log.Printf("Trace: Etcd: volunteerCallback(): Finished!")
	}
	if err := obj.Connect(false); err != nil {
		log.Printf("Etcd: volunteerCallback(): Connect failed permanently: %v", err)
		// permanently fail...
		return &CtxPermanentErr{fmt.Sprintf("Etcd: volunteerCallback(): Connect error: %s", err)}
	}
	var err error

	// FIXME: if this is running in response to our OWN volunteering offer,
	// skip doing stuff if we're not a server yet because it's pointless,
	// and we might have just lost quorum if we just got nominated! Why the
	// lack of quorum is needed to read data in etcd v3 but not in v2 is a
	// mystery for now, since in v3 this now blocks! Maybe it's that the
	// Maintenance.Status API requires a leader to return? Maybe that's it!
	// FIXME: are there any situations where we don't want to short circuit
	// here, such as if i'm the last node?
	if obj.server == nil {
		return nil // if we're not a server, we're not a leader, return
	}

	membersMap, err := Members(obj) // map[uint64]string
	if err != nil {
		return fmt.Errorf("Etcd: Members: Error: %+v", err)
	}
	members := util.StrMapValuesUint64(membersMap) // get values
	log.Printf("Etcd: Members: List: %+v", members)

	// we only do *one* change operation at a time so that the cluster can
	// advance safely. we ensure this by returning CtxDelayErr any time an
	// operation happens to ensure the function will reschedule itself due
	// to the CtxError processing after this callback "fails". This custom
	// error is caught by CtxError, and lets us specify a retry delay too!

	// check for unstarted members, since we're currently "unhealthy"
	for mID, name := range membersMap {
		if name == "" {
			// reschedule in one second
			// XXX: will the unnominate TTL still happen if we are
			// in an unhealthy state? that's what we're waiting for
			return &CtxDelayErr{2 * time.Second, fmt.Sprintf("unstarted member, mID: %d", mID)}
		}
	}

	leader, err := Leader(obj) // XXX: race!
	if err != nil {
		log.Printf("Etcd: Leader: Error: %+v", err)
		return fmt.Errorf("Etcd: Leader: Error: %+v", err)
	}
	log.Printf("Etcd: Leader: %+v", leader)
	if leader != obj.hostname {
		log.Printf("Etcd: We are not the leader...")
		return nil
	}
	// i am the leader!

	// get the list of available volunteers
	volunteersMap, err := Volunteers(obj)
	if err != nil {
		log.Printf("Etcd: Volunteers: Error: %+v", err)
		return fmt.Errorf("Etcd: Volunteers: Error: %+v", err)
	}

	volunteers := []string{} // get keys
	for k := range volunteersMap {
		volunteers = append(volunteers, k)
	}
	sort.Strings(volunteers) // deterministic order
	log.Printf("Etcd: Volunteers: %v", volunteers)

	// unnominate anyone that unvolunteers, so that they can shutdown cleanly
	quitters := util.StrFilterElementsInList(volunteers, members)
	log.Printf("Etcd: Quitters: %v", quitters)

	// if we're the only member left, just shutdown...
	if len(members) == 1 && members[0] == obj.hostname && len(quitters) == 1 && quitters[0] == obj.hostname {
		log.Printf("Etcd: Quitters: Shutting down self...")
		if err := Nominate(obj, obj.hostname, nil); err != nil { // unnominate myself
			return &CtxDelayErr{1 * time.Second, fmt.Sprintf("error shutting down self: %v", err)}
		}
		return nil
	}

	candidates := util.StrFilterElementsInList(members, volunteers)
	log.Printf("Etcd: Candidates: %v", candidates)

	// TODO: switch to < 0 so that we can shut the whole cluster down with 0
	if obj.idealClusterSize < 1 { // safety in case value is not ready yet
		return &CtxDelayErr{1 * time.Second, "The idealClusterSize is < 1."} // retry in one second
	}

	// do we need more members?
	if len(candidates) > 0 && len(members)-len(quitters) < int(obj.idealClusterSize) {
		chosen := candidates[0]           // XXX: use a better picker algorithm
		peerURLs := volunteersMap[chosen] // comma separated list of urls

		// NOTE: storing peerURLs when they're already in volunteers/ is
		// redundant, but it seems to be necessary for a sane algorithm.
		// nominate before we call the API so that members see it first!
		Nominate(obj, chosen, peerURLs)
		// XXX: add a ttl here, because once we nominate someone, we
		// need to give them up to N seconds to start up after we run
		// the MemberAdd API because if they don't, in some situations
		// such as if we're adding the second node to the cluster, then
		// we've lost quorum until a second member joins! If the TTL
		// expires, we need to MemberRemove! In this special case, we
		// need to forcefully remove the second member if we don't add
		// them, because we'll be in a lack of quorum state and unable
		// to do anything... As a result, we should always only add ONE
		// member at a time!

		log.Printf("Etcd: Member Add: %v", peerURLs)
		mresp, err := MemberAdd(obj, peerURLs)
		if err != nil {
			// on error this function will run again, which is good
			// because we need to make sure to run the below parts!
			return fmt.Errorf("Etcd: Member Add: Error: %+v", err)
		}
		log.Printf("Etcd: Member Add: %+v", mresp.Member.PeerURLs)
		// return and reschedule to check for unstarted members, etc...
		return &CtxDelayErr{1 * time.Second, fmt.Sprintf("Member %s added successfully!", chosen)} // retry asap

	} else if len(quitters) == 0 && len(members) > int(obj.idealClusterSize) { // too many members
		for _, kicked := range members {
			// don't kick ourself unless we are the only one left...
			if kicked != obj.hostname || (obj.idealClusterSize == 0 && len(members) == 1) {
				quitters = []string{kicked} // XXX: use a better picker algorithm
				log.Printf("Etcd: Extras: %v", quitters)
				break
			}
		}
	}

	// we must remove them from the members API or it will look like a crash
	if lq := len(quitters); lq > 0 {
		log.Printf("Etcd: Quitters: Shutting down %d members...", lq)
	}
	for _, quitter := range quitters {
		mID, ok := util.Uint64KeyFromStrInMap(quitter, membersMap)
		if !ok {
			// programming error
			log.Fatalf("Etcd: Member Remove: Error: %v(%v) not in members list!", quitter, mID)
		}
		Nominate(obj, quitter, nil) // unnominate
		// once we issue the above unnominate, that peer will
		// shutdown, and this might cause us to loose quorum,
		// therefore, let that member remove itself, and then
		// double check that it did happen in case delinquent
		// TODO: get built-in transactional member Add/Remove
		// functionality to avoid a separate nominate list...
		if quitter == obj.hostname { // remove in unnominate!
			log.Printf("Etcd: Quitters: Removing self...")
			continue // TODO: CtxDelayErr ?
		}

		log.Printf("Etcd: Waiting %d seconds for %s to self remove...", selfRemoveTimeout, quitter)
		time.Sleep(selfRemoveTimeout * time.Second)
		// in case the removed member doesn't remove itself, do it!
		removed, err := MemberRemove(obj, mID)
		if err != nil {
			return fmt.Errorf("Etcd: Member Remove: Error: %+v", err)
		}
		if removed {
			log.Printf("Etcd: Member Removed (forced): %v(%v)", quitter, mID)
		}

		// Remove the endpoint from our list to avoid blocking
		// future MemberList calls which would try and connect
		// to a missing endpoint... The endpoints should get
		// updated from the member exiting safely if it doesn't
		// crash, but if it did and/or since it's a race to see
		// if the update event will get seen before we need the
		// new data, just do it now anyways, then update the
		// endpoint list and trigger a reconnect.
		delete(obj.endpoints, quitter) // proactively delete it
		obj.endpointCallback(nil)      // update!
		log.Printf("Member %s (%d) removed successfully!", quitter, mID)
		return &CtxReconnectErr{"a member was removed"} // retry asap and update endpoint list
	}

	return nil
}

// nominateCallback runs to respond to the nomination list change events.
// Functionally, it controls the starting and stopping of the server process.
func (obj *EmbdEtcd) nominateCallback(re *RE) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: nominateCallback()")
		defer log.Printf("Trace: Etcd: nominateCallback(): Finished!")
	}
	bootstrapping := len(obj.endpoints) == 0
	var revision int64 // = 0
	if re != nil {
		revision = re.response.Header.Revision
	}
	if !bootstrapping && (re == nil || revision != obj.lastRevision) {
		// don't reprocess if we've already processed this message
		// this can happen if the callback errors and is re-called
		obj.lastRevision = revision

		// if we tried to lookup the nominated members here (in etcd v3)
		// this would sometimes block because we would loose the cluster
		// leader once the current leader calls the MemberAdd API and it
		// steps down trying to form a two host cluster. Instead, we can
		// look at the event response data to read the nominated values!
		//nominated, err = Nominated(obj) // nope, won't always work
		// since we only see what has *changed* in the response data, we
		// have to keep track of the original state and apply the deltas
		// this must be idempotent in case it errors and is called again
		// if we're retrying and we get a data format error, it's normal
		nominated := obj.nominated
		if nominated, err := ApplyDeltaEvents(re, nominated); err == nil {
			obj.nominated = nominated
		} else if !re.retryHint || err != errApplyDeltaEventsInconsistent {
			log.Fatal(err)
		}

	} else {
		// TODO: should we just use the above delta method for everything?
		//nominated, err := Nominated(obj) // just get it
		//if err != nil {
		//	return fmt.Errorf("Etcd: Nominate: Error: %+v", err)
		//}
		//obj.nominated = nominated // update our local copy
	}
	if n := obj.nominated; len(n) > 0 {
		log.Printf("Etcd: Nominated: %+v", n)
	} else {
		log.Printf("Etcd: Nominated: []")
	}

	// if there are no other peers, we create a new server
	_, exists := obj.nominated[obj.hostname]
	// FIXME: can we get rid of the len(obj.nominated) == 0 ?
	newCluster := len(obj.nominated) == 0 || (len(obj.nominated) == 1 && exists)
	if obj.flags.Debug {
		log.Printf("Etcd: nominateCallback(): newCluster: %v; exists: %v; obj.server == nil: %t", newCluster, exists, obj.server == nil)
	}
	// XXX: check if i have actually volunteered first of all...
	if obj.server == nil && (newCluster || exists) {

		log.Printf("Etcd: StartServer(newCluster: %t): %+v", newCluster, obj.nominated)
		err := obj.StartServer(
			newCluster,    // newCluster
			obj.nominated, // other peer members and urls or empty map
		)
		if err != nil {
			var retries uint
			if re != nil {
				retries = re.retries
			}
			// retry MaxStartServerRetries times, then permanently fail
			return &CtxRetriesErr{MaxStartServerRetries - retries, fmt.Sprintf("Etcd: StartServer: Error: %+v", err)}
		}

		if len(obj.endpoints) == 0 {
			// add server to obj.endpoints list...
			addresses := obj.LocalhostClientURLs()
			if len(addresses) == 0 {
				// probably a programming error...
				log.Fatal("Etcd: No valid clientUrls exist!")
			}
			obj.endpoints[obj.hostname] = addresses // now we have some!
			// client connects to one of the obj.endpoints servers...
			log.Printf("Etcd: Addresses are: %s", addresses)

			surls := obj.serverURLs
			if len(obj.advertiseServerURLs) > 0 {
				surls = obj.advertiseServerURLs
			}
			// XXX: just put this wherever for now so we don't block
			// nominate self so "member" list is correct for peers to see
			Nominate(obj, obj.hostname, surls)
			// XXX: if this fails, where will we retry this part ?
		}

		// advertise client urls
		if curls := obj.clientURLs; len(curls) > 0 {
			if len(obj.advertiseClientURLs) > 0 {
				curls = obj.advertiseClientURLs
			}
			// XXX: don't advertise local addresses! 127.0.0.1:2381 doesn't really help remote hosts
			// XXX: but sometimes this is what we want... hmmm how do we decide? filter on callback?
			AdvertiseEndpoints(obj, curls)
			// XXX: if this fails, where will we retry this part ?

			// force this to remove sentinel before we reconnect...
			obj.endpointCallback(nil)
		}

		return &CtxReconnectErr{"local server is running"} // trigger reconnect to self

	} else if obj.server != nil && !exists {
		// un advertise client urls
		AdvertiseEndpoints(obj, nil)

		// i have been un-nominated, remove self and shutdown server!
		if len(obj.nominated) != 0 { // don't call if nobody left but me!
			// this works around: https://github.com/coreos/etcd/issues/5482,
			// and it probably makes sense to avoid calling if we're the last
			log.Printf("Etcd: Member Remove: Removing self: %v", obj.memberID)
			removed, err := MemberRemove(obj, obj.memberID)
			if err != nil {
				return fmt.Errorf("Etcd: Member Remove: Error: %+v", err)
			}
			if removed {
				log.Printf("Etcd: Member Removed (self): %v(%v)", obj.hostname, obj.memberID)
			}
		}

		log.Printf("Etcd: DestroyServer...")
		obj.DestroyServer()
		// TODO: make sure to think about the implications of
		// shutting down and potentially intercepting signals
		// here after i've removed myself from the nominated!

		// if we are connected to self and other servers exist: trigger
		// if any of the obj.clientURLs are in the endpoints list, then
		// we are stale. it is not likely that the advertised endpoints
		// have been updated because we're still blocking the callback.
		stale := false
		for key, eps := range obj.endpoints {
			if key != obj.hostname && len(eps) > 0 { // other endpoints?
				stale = true // only half true so far
				break
			}
		}

		for _, curl := range obj.clientURLs { // these just got shutdown
			for _, ep := range obj.client.Endpoints() {
				if (curl.Host == ep || curl.String() == ep) && stale {
					// add back the sentinel to force update
					log.Printf("Etcd: Forcing endpoint callback...")
					obj.endpoints[seedSentinel] = nil                    //etcdtypes.URLs{}
					obj.endpointCallback(nil)                            // update!
					return &CtxReconnectErr{"local server has shutdown"} // trigger reconnect
				}
			}
		}
	}
	return nil
}

// endpointCallback runs to respond to the endpoint list change events.
func (obj *EmbdEtcd) endpointCallback(re *RE) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: endpointCallback()")
		defer log.Printf("Trace: Etcd: endpointCallback(): Finished!")
	}

	// if the startup sentinel exists, or delta fails, then get a fresh copy
	endpoints := make(etcdtypes.URLsMap, len(obj.endpoints))
	// this would copy the reference: endpoints := obj.endpoints
	for k, v := range obj.endpoints {
		endpoints[k] = make(etcdtypes.URLs, len(v))
		copy(endpoints[k], v)
	}

	// updating
	_, exists := endpoints[seedSentinel]
	endpoints, err := ApplyDeltaEvents(re, endpoints)
	if err != nil || exists {
		// TODO: we could also lookup endpoints from the maintenance api
		endpoints, err = Endpoints(obj)
		if err != nil {
			return err
		}
	}

	// change detection
	var changed = false // do we need to update?
	if len(obj.endpoints) != len(endpoints) {
		changed = true
	}
	for k, v1 := range obj.endpoints {
		if changed { // catches previous statement and inner loop break
			break
		}
		v2, exists := endpoints[k]
		if !exists {
			changed = true
			break
		}
		if len(v1) != len(v2) {
			changed = true
			break
		}
		for i := range v1 {
			if v1[i] != v2[i] {
				changed = true
				break
			}
		}
	}
	// is the endpoint list different?
	if changed {
		obj.endpoints = endpoints // set
		if eps := endpoints; len(eps) > 0 {
			log.Printf("Etcd: Endpoints: %+v", eps)
		} else {
			log.Printf("Etcd: Endpoints: []")
		}
		// can happen if a server drops out for example
		return &CtxReconnectErr{"endpoint list changed"} // trigger reconnect with new endpoint list
	}

	return nil
}

// idealClusterSizeCallback runs to respond to the ideal cluster size changes.
func (obj *EmbdEtcd) idealClusterSizeCallback(re *RE) error {
	if obj.flags.Trace {
		log.Printf("Trace: Etcd: idealClusterSizeCallback()")
		defer log.Printf("Trace: Etcd: idealClusterSizeCallback(): Finished!")
	}
	path := fmt.Sprintf("%s/idealClusterSize", NS)
	for _, event := range re.response.Events {
		if key := bytes.NewBuffer(event.Kv.Key).String(); key != path {
			continue
		}
		if event.Type != etcd.EventTypePut {
			continue
		}
		val := bytes.NewBuffer(event.Kv.Value).String()
		if val == "" {
			continue
		}
		v, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			continue
		}
		if i := uint16(v); i > 0 {
			log.Printf("Etcd: Ideal cluster size is now: %d", i)
			obj.idealClusterSize = i
			// now, emulate the calling of the volunteerCallback...
			go func() {
				obj.wevents <- &RE{callback: obj.volunteerCallback, errCheck: true} // send event
			}() // don't block
		}
	}
	return nil
}

// LocalhostClientURLs returns the most localhost like URLs for direct connection.
// This gets clients to talk to the local servers first before searching remotely.
func (obj *EmbdEtcd) LocalhostClientURLs() etcdtypes.URLs {
	// look through obj.clientURLs and return the localhost ones
	urls := etcdtypes.URLs{}
	for _, x := range obj.clientURLs {
		// "localhost", ::1 or anything in 127.0.0.0/8 is valid!
		if s := x.Host; strings.HasPrefix(s, "localhost") || strings.HasPrefix(s, "127.") || strings.HasPrefix(s, "[::1]") {
			urls = append(urls, x)
		}
		// or local unix domain socket
		if x.Scheme == "unix" {
			urls = append(urls, x)
		}
	}
	return urls
}

// StartServer kicks of a new embedded etcd server.
func (obj *EmbdEtcd) StartServer(newCluster bool, peerURLsMap etcdtypes.URLsMap) error {
	var err error
	memberName := obj.hostname

	err = os.MkdirAll(obj.dataDir, 0770)
	if err != nil {
		log.Printf("Etcd: StartServer: Couldn't mkdir: %s.", obj.dataDir)
		log.Printf("Etcd: StartServer: Mkdir error: %s.", err)
		obj.DestroyServer()
		return err
	}

	// if no peer URLs exist, then starting a server is mostly only for some
	// testing, but etcd doesn't allow the value to be empty so we use this!
	peerURLs, _ := etcdtypes.NewURLs([]string{"http://localhost:0"})
	if len(obj.serverURLs) > 0 {
		peerURLs = obj.serverURLs
	}
	initialPeerURLsMap := make(etcdtypes.URLsMap)
	for k, v := range peerURLsMap {
		initialPeerURLsMap[k] = v // copy
	}
	if _, exists := peerURLsMap[memberName]; !exists {
		initialPeerURLsMap[memberName] = peerURLs
	}

	aCUrls := obj.clientURLs
	if len(obj.advertiseClientURLs) > 0 {
		aCUrls = obj.advertiseClientURLs
	}
	aPUrls := peerURLs
	if len(obj.advertiseServerURLs) > 0 {
		aPUrls = obj.advertiseServerURLs
	}

	// embed etcd
	cfg := embed.NewConfig()
	cfg.Name = memberName // hostname
	cfg.Dir = obj.dataDir
	cfg.LCUrls = obj.clientURLs
	cfg.LPUrls = peerURLs
	cfg.ACUrls = aCUrls
	cfg.APUrls = aPUrls
	cfg.StrictReconfigCheck = false // XXX: workaround https://github.com/coreos/etcd/issues/6305
	cfg.MaxTxnOps = DefaultMaxTxnOps

	cfg.InitialCluster = initialPeerURLsMap.String() // including myself!
	if newCluster {
		cfg.ClusterState = embed.ClusterStateFlagNew
	} else {
		cfg.ClusterState = embed.ClusterStateFlagExisting
	}
	//cfg.ForceNewCluster = newCluster // TODO: ?

	log.Printf("Etcd: StartServer: Starting server...")
	obj.server, err = embed.StartEtcd(cfg)
	if err != nil {
		return err
	}
	select {
	case <-obj.server.Server.ReadyNotify(): // we hang here if things are bad
		log.Printf("Etcd: StartServer: Done starting server!") // it didn't hang!
	case <-time.After(time.Duration(MaxStartServerTimeout) * time.Second):
		e := fmt.Errorf("timeout of %d seconds reached", MaxStartServerTimeout)
		log.Printf("Etcd: StartServer: %s", e.Error())
		obj.server.Server.Stop() // trigger a shutdown
		obj.serverwg.Add(1)      // add for the DestroyServer()
		obj.DestroyServer()
		return e
	// TODO: should we wait for this notification elsewhere?
	case <-obj.server.Server.StopNotify(): // it's going down now...
		e := fmt.Errorf("received stop notification")
		log.Printf("Etcd: StartServer: %s", e.Error())
		obj.server.Server.Stop() // trigger a shutdown
		obj.serverwg.Add(1)      // add for the DestroyServer()
		obj.DestroyServer()
		return e
	}
	//log.Fatal(<-obj.server.Err())	XXX
	log.Printf("Etcd: StartServer: Server running...")
	obj.memberID = uint64(obj.server.Server.ID()) // store member id for internal use
	close(obj.serverReady)                        // send a signal

	obj.serverwg.Add(1)
	return nil
}

// ServerReady returns on a channel when the server has started successfully.
func (obj *EmbdEtcd) ServerReady() <-chan struct{} { return obj.serverReady }

// DestroyServer shuts down the embedded etcd server portion.
func (obj *EmbdEtcd) DestroyServer() error {
	var err error
	log.Printf("Etcd: DestroyServer: Destroying...")
	if obj.server != nil {
		obj.server.Close() // this blocks until server has stopped
	}
	log.Printf("Etcd: DestroyServer: Done closing...")

	obj.memberID = 0
	if obj.server == nil { // skip the .Done() below because we didn't .Add(1) it.
		return err
	}
	obj.server = nil // important because this is used as an isRunning flag
	log.Printf("Etcd: DestroyServer: Unlocking server...")
	obj.serverReady = make(chan struct{}) // reset the signal
	obj.serverwg.Done()                   // -1
	return err
}

//func UrlRemoveScheme(urls etcdtypes.URLs) []string {
//	strs := []string{}
//	for _, u := range urls {
//		strs = append(strs, u.Host) // remove http:// prefix
//	}
//	return strs
//}

// ApplyDeltaEvents modifies a URLsMap with the deltas from a WatchResponse.
func ApplyDeltaEvents(re *RE, urlsmap etcdtypes.URLsMap) (etcdtypes.URLsMap, error) {
	if re == nil { // passthrough
		return urlsmap, nil
	}
	for _, event := range re.response.Events {
		key := bytes.NewBuffer(event.Kv.Key).String()
		key = key[len(re.path):] // remove path prefix
		log.Printf("Etcd: ApplyDeltaEvents: Event(%s): %s", event.Type.String(), key)

		switch event.Type {
		case etcd.EventTypePut:
			val := bytes.NewBuffer(event.Kv.Value).String()
			if val == "" {
				return nil, fmt.Errorf("value in ApplyDeltaEvents is empty")
			}
			urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
			if err != nil {
				return nil, fmt.Errorf("format error in ApplyDeltaEvents: %v", err)
			}
			urlsmap[key] = urls // add to map

		// expiry cases are seen as delete in v3 for now
		//case etcd.EventTypeExpire: // doesn't exist right now
		//	fallthrough
		case etcd.EventTypeDelete:
			if _, exists := urlsmap[key]; !exists {
				// this can happen if we retry an operation b/w
				// a reconnect so ignore if we are reconnecting
				log.Printf("Etcd: ApplyDeltaEvents: Inconsistent key: %v", key)
				return nil, errApplyDeltaEventsInconsistent
			}
			delete(urlsmap, key)

		default:
			return nil, fmt.Errorf("unknown event in ApplyDeltaEvents: %+v", event.Type)
		}
	}
	return urlsmap, nil
}
