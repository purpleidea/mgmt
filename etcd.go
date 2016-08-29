// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// TODO: Add TTL's (eg: volunteering)
// TODO: Remove race around leader operations
// TODO: Fix server reuse issue (bind: address already in use)
// TODO: Fix unstarted member
// TODO: Fix excessive StartLoop/FinishLoop
// TODO: Add VIP for servers (incorporate with net resource)
// TODO: Auto assign ports/ip's for peers (if possible)
// TODO: Fix godoc

// Smoke testing:
// ./mgmt run --file examples/etcd1a.yaml --hostname h1
// ./mgmt run --file examples/etcd1b.yaml --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382
// ./mgmt run --file examples/etcd1c.yaml --hostname h3 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384
// ./mgmt run --file examples/etcd1d.yaml --hostname h4 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/idealClusterSize 3
// ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 put /_mgmt/idealClusterSize 5

// The elastic etcd algorithm works in the following way:
// * When you start up mgmt, you can pass it a list of seeds.
// * If no seeds are given, then assume you are the first server and startup.
// * If a seed is given, connect as a client, and optionally volunteer to be a server.
// * All volunteering clients should listen for a message from the master for nomination.
// * If a client has been nominated, it should startup a server.
// * All servers should list for their nomination to be removed and shutdown if so.
// * The elected leader should decide who to nominate/unnominate to keep the right number of servers.
package main

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

	etcd "github.com/coreos/etcd/clientv3" // "clientv3"
	"github.com/coreos/etcd/embed"
	"github.com/coreos/etcd/etcdserver"
	rpctypes "github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	etcdtypes "github.com/coreos/etcd/pkg/types"
	raft "github.com/coreos/etcd/raft"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	NS                      = "_mgmt" // root namespace for mgmt operations
	seedSentinel            = "_seed" // you must not name your hostname this
	maxStartServerRetries   = 3       // number of times to retry starting the etcd server
	maxClientConnectRetries = 5       // number of times to retry consecutive connect failures
	selfRemoveTimeout       = 3       // give unnominated members a chance to self exit
	exitDelay               = 3       // number of sec of inactivity after exit to clean up
	defaultIdealClusterSize = 5       // default ideal cluster size target for initial seed
	DefaultClientURL        = "127.0.0.1:2379"
	DefaultServerURL        = "127.0.0.1:2380"
)

var (
	ErrApplyDeltaEventsInconsistent = errors.New("Etcd: ApplyDeltaEvents: Inconsistent key!")
)

// AW is a struct for the AddWatcher queue
type AW struct {
	path       string
	opts       []etcd.OpOption
	callback   func(*RE) error
	errCheck   bool
	resp       Resp
	cancelFunc func() // data
}

// RE is a response + error struct since these two values often occur together
// This is now called an event with the move to the etcd v3 API
type RE struct {
	response  etcd.WatchResponse
	path      string
	err       error
	callback  func(*RE) error
	errCheck  bool // should we check the error of the callback?
	retryHint bool // set to true for one event after a watcher failure
	retries   uint // number of times we've retried on error
}

// KV is a key + value struct to hold the two items together
type KV struct {
	key   string
	value string
	opts  []etcd.OpOption
	resp  Resp
}

// GQ is a struct for the get queue
type GQ struct {
	path string
	opts []etcd.OpOption
	resp Resp
	data map[string]string
}

// DL is a struct for the delete queue
type DL struct {
	path string
	opts []etcd.OpOption
	resp Resp
	data int64
}

// TN is a struct for the txn queue
type TN struct {
	ifcmps  []etcd.Cmp
	thenops []etcd.Op
	elseops []etcd.Op
	resp    Resp
	data    *etcd.TxnResponse
}

// EmbdEtcd provides the embedded server and client etcd functionality
type EmbdEtcd struct { // EMBeddeD etcd
	// etcd client connection related
	cLock  sync.Mutex   // client connect lock
	rLock  sync.RWMutex // client reconnect lock
	client *etcd.Client
	cError error // permanent client error
	ctxErr error // permanent ctx error

	// exit and cleanup related
	cancelLock  sync.Mutex // lock for the cancels list
	cancels     []func()   // array of every cancel function for watches
	exiting     bool
	exitchan    chan struct{}
	exitTimeout <-chan time.Time

	hostname   string
	memberId   uint64            // cluster membership id of server if running
	endpoints  etcdtypes.URLsMap // map of servers a client could connect to
	clientURLs etcdtypes.URLs    // locations to listen for clients if i am a server
	serverURLs etcdtypes.URLs    // locations to listen for servers if i am a server (peer)
	noServer   bool              // disable all server peering if true

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

	converger Converger // converged tracking
	prefix    string    // folder prefix to use for misc storage

	// etcd server related
	serverwg sync.WaitGroup // wait for server to shutdown
	server   *embed.Etcd    // technically this contains the server struct
	dataDir  string         // our data dir, prefix + "etcd"
}

// NewEmbdEtcd creates the top level embedded etcd struct client and server obj
func NewEmbdEtcd(hostname string, seeds, clientURLs, serverURLs etcdtypes.URLs, noServer bool, idealClusterSize uint16, converger Converger, prefix string) *EmbdEtcd {
	endpoints := make(etcdtypes.URLsMap)
	if hostname == seedSentinel { // safety
		return nil
	}
	if len(seeds) > 0 {
		endpoints[seedSentinel] = seeds
		idealClusterSize = 0 // unset, get from running cluster
	}
	obj := &EmbdEtcd{
		exitchan:    make(chan struct{}), // exit signal for main loop
		exitTimeout: nil,
		awq:         make(chan *AW),
		wevents:     make(chan *RE),
		setq:        make(chan *KV),
		getq:        make(chan *GQ),
		delq:        make(chan *DL),
		txnq:        make(chan *TN),

		nominated: make(etcdtypes.URLsMap),

		hostname:   hostname,
		endpoints:  endpoints,
		clientURLs: clientURLs,
		serverURLs: serverURLs,
		noServer:   noServer,

		idealClusterSize: idealClusterSize,
		converger:        converger,
		prefix:           prefix,
		dataDir:          path.Join(prefix, "etcd"),
	}
	// TODO: add some sort of auto assign method for picking these defaults
	// add a default so that our local client can connect locally if needed
	if len(obj.LocalhostClientURLs()) == 0 { // if we don't have any localhost URLs
		u := url.URL{Scheme: "http", Host: DefaultClientURL}     // default
		obj.clientURLs = append([]url.URL{u}, obj.clientURLs...) // prepend
	}

	// add a default for local use and testing, harmless and useful!
	if !obj.noServer && len(obj.serverURLs) == 0 {
		if len(obj.endpoints) > 0 {
			obj.noServer = true // we didn't have enough to be a server
		}
		u := url.URL{Scheme: "http", Host: DefaultServerURL} // default
		obj.serverURLs = []url.URL{u}
	}

	return obj
}

// GetConfig returns the config struct to be used for the etcd client connect
func (obj *EmbdEtcd) GetConfig() etcd.Config {
	endpoints := []string{}
	// XXX: filter out any urls which wouldn't resolve here ?
	for _, eps := range obj.endpoints { // flatten map
		for _, u := range eps {
			endpoints = append(endpoints, u.Host) // remove http:// prefix
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
	if DEBUG {
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
	var emax uint16 = 0
	for { // loop until connect
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
				log.Printf("Etcd: Please see: %s", "https://github.com/purpleidea/mgmt/blob/master/DOCUMENTATION.md#what-does-the-error-message-about-an-inconsistent-datadir-mean")
				obj.cError = fmt.Errorf("Can't find an available endpoint.")
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

// Startup is the main entry point to kick off the embedded etcd client & server
func (obj *EmbdEtcd) Startup() error {
	bootstrapping := len(obj.endpoints) == 0 // because value changes after start

	// connect but don't block here, because servers might not be up yet...
	go func() {
		if err := obj.Connect(false); err != nil {
			log.Printf("Etcd: Startup: Error: %v", err)
			// XXX: Now cause Startup() to exit with error somehow!
		}
	}()

	go obj.CallbackLoop() // start callback loop
	go obj.Loop()         // start main loop

	// TODO: implement native etcd watcher method on member API changes
	path := fmt.Sprintf("/%s/nominated/", NS)
	go obj.AddWatcher(path, obj.nominateCallback, true, etcd.WithPrefix()) // no block

	// setup ideal cluster size watcher
	key := fmt.Sprintf("/%s/idealClusterSize", NS)
	go obj.AddWatcher(key, obj.idealClusterSizeCallback, true) // no block

	// if we have no endpoints, it means we are bootstrapping...
	if !bootstrapping {
		log.Println("Etcd: Startup: Getting initial values...")
		if nominated, err := EtcdNominated(obj); err == nil {
			obj.nominated = nominated // store a local copy
		} else {
			log.Printf("Etcd: Startup: Nominate lookup error.")
			obj.Destroy()
			return fmt.Errorf("Etcd: Startup: Error: %v", err)
		}

		// get initial ideal cluster size
		if idealClusterSize, err := EtcdGetClusterSize(obj); err == nil {
			obj.idealClusterSize = idealClusterSize
			log.Printf("Etcd: Startup: Ideal cluster size is: %d", idealClusterSize)
		} else {
			// perhaps the first server didn't set it yet. it's ok,
			// we can get it from the watcher if it ever gets set!
			log.Printf("Etcd: Startup: Ideal cluster size lookup error.")
		}
	}

	if !obj.noServer {
		path := fmt.Sprintf("/%s/volunteers/", NS)
		go obj.AddWatcher(path, obj.volunteerCallback, true, etcd.WithPrefix()) // no block
	}

	// if i am alone and will have to be a server...
	if !obj.noServer && bootstrapping {
		log.Printf("Etcd: Bootstrapping...")
		// give an initial value to the obj.nominate map we keep in sync
		// this emulates EtcdNominate(obj, obj.hostname, obj.serverURLs)
		obj.nominated[obj.hostname] = obj.serverURLs // initial value
		// NOTE: when we are stuck waiting for the server to start up,
		// it is probably happening on this call right here...
		obj.nominateCallback(nil) // kick this off once
	}

	// self volunteer
	if !obj.noServer && len(obj.serverURLs) > 0 {
		// we run this in a go routine because it blocks waiting for server
		log.Printf("Etcd: Startup: Volunteering...")
		go EtcdVolunteer(obj, obj.serverURLs)
	}

	if bootstrapping {
		if err := EtcdSetClusterSize(obj, obj.idealClusterSize); err != nil {
			log.Printf("Etcd: Startup: Ideal cluster size storage error.")
			obj.Destroy()
			return fmt.Errorf("Etcd: Startup: Error: %v", err)
		}
	}

	go obj.AddWatcher(fmt.Sprintf("/%s/endpoints/", NS), obj.endpointCallback, true, etcd.WithPrefix())

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
	if err := EtcdVolunteer(obj, nil); err != nil { // unvolunteer so we can shutdown...
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

	obj.exitchan <- struct{}{} // cause main loop to exit

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
	return nil
}

// CtxDelayErr requests a retry in Delta duration
type CtxDelayErr struct {
	Delta   time.Duration
	Message string
}

func (obj *CtxDelayErr) Error() string {
	return fmt.Sprintf("CtxDelayErr(%v): %s", obj.Delta, obj.Message)
}

// CtxRetriesErr lets you retry as long as you have retries available
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

// CtxReconnectErr requests a client reconnect to the new endpoint list
type CtxReconnectErr struct {
	Message string
}

func (obj *CtxReconnectErr) Error() string {
	return fmt.Sprintf("CtxReconnectErr: %s", obj.Message)
}

// CancelCtx adds a tracked cancel function around an existing context
func (obj *EmbdEtcd) CancelCtx(ctx context.Context) (context.Context, func()) {
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	obj.cancelLock.Lock()
	obj.cancels = append(obj.cancels, cancelFunc) // not thread-safe, needs lock
	obj.cancelLock.Unlock()
	return cancelCtx, cancelFunc
}

// TimeoutCtx adds a tracked cancel function with timeout around an existing context
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
	const ctxErr = "ctxErr"
	const ctxIter = "ctxIter"
	expBackoff := func(tmin, texp, iter, tmax int) time.Duration {
		// https://en.wikipedia.org/wiki/Exponential_backoff
		// tmin <= texp^iter - 1 <= tmax // TODO: check my math
		return time.Duration(math.Min(math.Max(math.Pow(float64(texp), float64(iter))-1.0, float64(tmin)), float64(tmax))) * time.Millisecond
	}
	var isTimeout bool = false
	var iter int = 0
	if ctxerr, ok := ctx.Value(ctxErr).(error); ok {
		if DEBUG {
			log.Printf("Etcd: CtxError: err(%v), ctxerr(%v)", err, ctxerr)
		}
		if i, ok := ctx.Value(ctxIter).(int); ok {
			iter = i + 1 // load and increment
			if DEBUG {
				log.Printf("Etcd: CtxError: Iter: %v", iter)
			}
		}
		isTimeout = err == context.DeadlineExceeded
		if DEBUG {
			log.Printf("Etcd: CtxError: isTimeout: %v", isTimeout)
		}
		if !isTimeout {
			iter = 0 // reset timer
		}
		err = ctxerr // restore error
	} else if DEBUG {
		log.Printf("Etcd: CtxError: No value found")
	}
	ctxHelper := func(tmin, texp, tmax int) context.Context {
		t := expBackoff(tmin, texp, iter, tmax)
		if DEBUG {
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
		obj.ctxErr = fmt.Errorf("Etcd: CtxError: Exit in progress!")
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
			obj.ctxErr = fmt.Errorf("Etcd: CtxError: CtxRetriesErr: No more retries!")
			return ctx, obj.ctxErr
		}
		return ctx, nil
	}

	if permanentErr, ok := err.(*CtxPermanentErr); ok { // custom permanent error
		obj.ctxErr = fmt.Errorf("Etcd: CtxError: Reason: %s", permanentErr.Error())
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
		return ctx, fmt.Errorf("Etcd: Error: Misconfiguration: %v", err) // permanent failure?
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
	case isGrpc(etcdserver.ErrStopped):
		fallthrough
	case isGrpc(grpc.ErrClientConnClosing):

		if DEBUG {
			log.Printf("Etcd: CtxError: Error(%T): %+v", err, err)
			log.Printf("Etcd: Endpoints are: %v", obj.client.Endpoints())
			log.Printf("Etcd: Client endpoints are: %v", obj.endpoints)
		}

		if DEBUG {
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
			obj.ctxErr = fmt.Errorf("Etcd: Permanent connect error: %v", err)
			return ctx, obj.ctxErr
		}
		if DEBUG {
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
	obj.ctxErr = fmt.Errorf("Etcd: CtxError: Unknown error!")
	return ctx, obj.ctxErr
}

// CallbackLoop is the loop where callback execution is serialized
func (obj *EmbdEtcd) CallbackLoop() {
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()
	if e := obj.Connect(false); e != nil {
		return // fatal
	}
	for {
		ctx := context.Background() // TODO: inherit as input argument?
		select {
		// etcd watcher event
		case re := <-obj.wevents:
			cuuid.SetConverged(false) // activity!
			if TRACE {
				log.Printf("Trace: Etcd: Loop: Event: StartLoop")
			}
			for {
				if obj.exiting { // the exit signal has been sent!
					//re.resp.NACK() // nope!
					break
				}
				if TRACE {
					log.Printf("Trace: Etcd: Loop: rawCallback()")
				}
				err := rawCallback(ctx, re)
				if TRACE {
					log.Printf("Trace: Etcd: Loop: rawCallback(): %v", err)
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
			if TRACE {
				log.Printf("Trace: Etcd: Loop: Event: FinishLoop")
			}

		// converged timeout
		case _ = <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!

		// exit loop commit
		case <-obj.exitTimeout:
			log.Println("Etcd: Exiting callback loop!")
			return
		}
	}
}

// Loop is the main loop where everything is serialized
func (obj *EmbdEtcd) Loop() {
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()
	if e := obj.Connect(false); e != nil {
		return // fatal
	}
	for {
		ctx := context.Background() // TODO: inherit as input argument?
		// priority channel...
		select {
		case aw := <-obj.awq:
			cuuid.SetConverged(false) // activity!
			if TRACE {
				log.Printf("Trace: Etcd: Loop: PriorityAW: StartLoop")
			}
			obj.loopProcessAW(ctx, aw)
			if TRACE {
				log.Printf("Trace: Etcd: Loop: PriorityAW: FinishLoop")
			}
			continue // loop to drain the priority channel first!
		default:
			// passthrough to normal channel
		}

		select {
		// add watcher
		case aw := <-obj.awq:
			cuuid.SetConverged(false) // activity!
			if TRACE {
				log.Printf("Trace: Etcd: Loop: AW: StartLoop")
			}
			obj.loopProcessAW(ctx, aw)
			if TRACE {
				log.Printf("Trace: Etcd: Loop: AW: FinishLoop")
			}

		// set kv pair
		case kv := <-obj.setq:
			cuuid.SetConverged(false) // activity!
			if TRACE {
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
			if TRACE {
				log.Printf("Trace: Etcd: Loop: Set: FinishLoop")
			}

		// get value
		case gq := <-obj.getq:
			cuuid.SetConverged(false) // activity!
			if TRACE {
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
			if TRACE {
				log.Printf("Trace: Etcd: Loop: Get: FinishLoop")
			}

		// delete value
		case dl := <-obj.delq:
			cuuid.SetConverged(false) // activity!
			if TRACE {
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
			if TRACE {
				log.Printf("Trace: Etcd: Loop: Delete: FinishLoop")
			}

		// run txn
		case tn := <-obj.txnq:
			cuuid.SetConverged(false) // activity!
			if TRACE {
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
			if TRACE {
				log.Printf("Trace: Etcd: Loop: Txn: FinishLoop")
			}

		// converged timeout
		case _ = <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!

		// exit loop signal
		case <-obj.exitchan:
			log.Println("Etcd: Exiting loop shortly...")
			// activate exitTimeout switch which only opens after N
			// seconds of inactivity in this select switch, which
			// lets everything get bled dry to avoid blocking calls
			// which would otherwise block us from exiting cleanly!
			obj.exitTimeout = TimeAfterOrBlock(exitDelay)

		// exit loop commit
		case <-obj.exitTimeout:
			log.Println("Etcd: Exiting loop!")
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

// Set queues up a set operation to occur using our mainloop
func (obj *EmbdEtcd) Set(key, value string, opts ...etcd.OpOption) error {
	resp := NewResp()
	obj.setq <- &KV{key: key, value: value, opts: opts, resp: resp}
	if !resp.Wait() { // wait for ack/nack
		return fmt.Errorf("Etcd: Set: Probably received an exit...")
	}
	return nil
}

// rawSet actually implements the key set operation
func (obj *EmbdEtcd) rawSet(ctx context.Context, kv *KV) error {
	if TRACE {
		log.Printf("Trace: Etcd: rawSet()")
	}
	// key is the full key path
	// TODO: should this be : obj.client.KV.Put or obj.client.Put ?
	obj.rLock.RLock() // these read locks need to wrap any use of obj.client
	response, err := obj.client.KV.Put(ctx, kv.key, kv.value, kv.opts...)
	obj.rLock.RUnlock()
	log.Printf("Etcd: Set(%s): %v", kv.key, response) // w00t... bonus
	if TRACE {
		log.Printf("Trace: Etcd: rawSet(): %v", err)
	}
	return err
}

// Get performs a get operation and waits for an ACK to continue
func (obj *EmbdEtcd) Get(path string, opts ...etcd.OpOption) (map[string]string, error) {
	resp := NewResp()
	gq := &GQ{path: path, opts: opts, resp: resp, data: nil}
	obj.getq <- gq    // send
	if !resp.Wait() { // wait for ack/nack
		return nil, fmt.Errorf("Etcd: Get: Probably received an exit...")
	}
	return gq.data, nil
}

func (obj *EmbdEtcd) rawGet(ctx context.Context, gq *GQ) (result map[string]string, err error) {
	if TRACE {
		log.Printf("Trace: Etcd: rawGet()")
	}
	obj.rLock.RLock()
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

	if TRACE {
		log.Printf("Trace: Etcd: rawGet(): %v", result)
	}
	return
}

// Delete performs a delete operation and waits for an ACK to continue
func (obj *EmbdEtcd) Delete(path string, opts ...etcd.OpOption) (int64, error) {
	resp := NewResp()
	dl := &DL{path: path, opts: opts, resp: resp, data: -1}
	obj.delq <- dl    // send
	if !resp.Wait() { // wait for ack/nack
		return -1, fmt.Errorf("Etcd: Delete: Probably received an exit...")
	}
	return dl.data, nil
}

func (obj *EmbdEtcd) rawDelete(ctx context.Context, dl *DL) (count int64, err error) {
	if TRACE {
		log.Printf("Trace: Etcd: rawDelete()")
	}
	count = -1
	obj.rLock.RLock()
	response, err := obj.client.KV.Delete(ctx, dl.path, dl.opts...)
	obj.rLock.RUnlock()
	if err == nil {
		count = response.Deleted
	}
	if TRACE {
		log.Printf("Trace: Etcd: rawDelete(): %v", err)
	}
	return
}

// Txn performs a transaction and waits for an ACK to continue
func (obj *EmbdEtcd) Txn(ifcmps []etcd.Cmp, thenops, elseops []etcd.Op) (*etcd.TxnResponse, error) {
	resp := NewResp()
	tn := &TN{ifcmps: ifcmps, thenops: thenops, elseops: elseops, resp: resp, data: nil}
	obj.txnq <- tn    // send
	if !resp.Wait() { // wait for ack/nack
		return nil, fmt.Errorf("Etcd: Txn: Probably received an exit...")
	}
	return tn.data, nil
}

func (obj *EmbdEtcd) rawTxn(ctx context.Context, tn *TN) (*etcd.TxnResponse, error) {
	if TRACE {
		log.Printf("Trace: Etcd: rawTxn()")
	}
	obj.rLock.RLock()
	response, err := obj.client.KV.Txn(ctx).If(tn.ifcmps...).Then(tn.thenops...).Else(tn.elseops...).Commit()
	obj.rLock.RUnlock()
	if TRACE {
		log.Printf("Trace: Etcd: rawTxn(): %v, %v", response, err)
	}
	return response, err
}

// AddWatcher queues up an add watcher request and returns a cancel function
// Remember to add the etcd.WithPrefix() option if you want to watch recursively
func (obj *EmbdEtcd) AddWatcher(path string, callback func(re *RE) error, errCheck bool, opts ...etcd.OpOption) (func(), error) {
	resp := NewResp()
	awq := &AW{path: path, opts: opts, callback: callback, errCheck: errCheck, cancelFunc: nil, resp: resp}
	obj.awq <- awq    // send
	if !resp.Wait() { // wait for ack/nack
		return nil, fmt.Errorf("Etcd: AddWatcher: Got NACK!")
	}
	return awq.cancelFunc, nil
}

// rawAddWatcher adds a watcher and returns a cancel function to call to end it
func (obj *EmbdEtcd) rawAddWatcher(ctx context.Context, aw *AW) (func(), error) {
	cancelCtx, cancelFunc := obj.CancelCtx(ctx)
	go func(ctx context.Context) {
		defer cancelFunc() // it's safe to cancelFunc() more than once!
		obj.rLock.RLock()
		rch := obj.client.Watcher.Watch(ctx, aw.path, aw.opts...)
		obj.rLock.RUnlock()
		var rev int64
		var useRev bool = false
		var retry, locked bool = false, false
		for {
			response := <-rch // read
			err := response.Err()
			isCanceled := response.Canceled || err == context.Canceled
			if response.Header.Revision == 0 { // by inspection
				if DEBUG {
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
				rev = response.Header.Revision // TODO +1 ?
				useRev = true
				if !locked {
					retry = false
				}
				locked = false
			} else {
				if DEBUG {
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
			obj.wevents <- &RE{response: response, path: aw.path, err: err, callback: aw.callback, errCheck: aw.errCheck, retryHint: retry} // send event
		}
	}(cancelCtx)
	return cancelFunc, nil
}

// rawCallback is the companion to AddWatcher which runs the callback processing
func rawCallback(ctx context.Context, re *RE) error {
	var err error = re.err // the watch event itself might have had an error
	if err == nil {
		if callback := re.callback; callback != nil {
			// TODO: we could add an async option if needed
			// NOTE: the callback must *not* block!
			// FIXME: do we need to pass ctx in via *RE, or in the callback signature ?
			err = callback(re) // run the callback
			if TRACE {
				log.Printf("Trace: Etcd: rawCallback(): %v", err)
			}
			if !re.errCheck || err == nil {
				return nil
			}
		} else {
			return nil
		}
	}
	return err
}

// volunteerCallback runs to respond to the volunteer list change events
// functionally, it controls the adding and removing of members
// FIXME: we might need to respond to member change/disconnect/shutdown events,
// see: https://github.com/coreos/etcd/issues/5277
func (obj *EmbdEtcd) volunteerCallback(re *RE) error {
	if TRACE {
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

	membersMap, err := EtcdMembers(obj) // map[uint64]string
	if err != nil {
		return fmt.Errorf("Etcd: Members: Error: %+v", err)
	}
	members := StrMapValuesUint64(membersMap) // get values
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

	leader, err := EtcdLeader(obj) // XXX: race!
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
	volunteersMap, err := EtcdVolunteers(obj)
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
	quitters := StrFilterElementsInList(volunteers, members)
	log.Printf("Etcd: Quitters: %v", quitters)

	// if we're the only member left, just shutdown...
	if len(members) == 1 && members[0] == obj.hostname && len(quitters) == 1 && quitters[0] == obj.hostname {
		log.Printf("Etcd: Quitters: Shutting down self...")
		if err := EtcdNominate(obj, obj.hostname, nil); err != nil { // unnominate myself
			return &CtxDelayErr{1 * time.Second, fmt.Sprintf("error shutting down self: %v", err)}
		}
		return nil
	}

	candidates := StrFilterElementsInList(members, volunteers)
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
		EtcdNominate(obj, chosen, peerURLs)
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
		mresp, err := EtcdMemberAdd(obj, peerURLs)
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
		if mID, ok := Uint64KeyFromStrInMap(quitter, membersMap); ok {
			EtcdNominate(obj, quitter, nil) // unnominate
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
			removed, err := EtcdMemberRemove(obj, mID)
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

		} else {
			// programming error
			log.Fatalf("Etcd: Member Remove: Error: %v(%v) not in members list!", quitter, mID)
		}
	}

	return nil
}

// nominateCallback runs to respond to the nomination list change events
// functionally, it controls the starting and stopping of the server process
func (obj *EmbdEtcd) nominateCallback(re *RE) error {
	if TRACE {
		log.Printf("Trace: Etcd: nominateCallback()")
		defer log.Printf("Trace: Etcd: nominateCallback(): Finished!")
	}
	bootstrapping := len(obj.endpoints) == 0
	var revision int64 = 0
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
		//nominated, err = EtcdNominated(obj) // nope, won't always work
		// since we only see what has *changed* in the response data, we
		// have to keep track of the original state and apply the deltas
		// this must be idempotent in case it errors and is called again
		// if we're retrying and we get a data format error, it's normal
		nominated := obj.nominated
		if nominated, err := ApplyDeltaEvents(re, nominated); err == nil {
			obj.nominated = nominated
		} else if !re.retryHint || err != ErrApplyDeltaEventsInconsistent {
			log.Fatal(err)
		}

	} else {
		// TODO: should we just use the above delta method for everything?
		//nominated, err := EtcdNominated(obj) // just get it
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
	if DEBUG {
		log.Printf("Etcd: nominateCallback(): newCluster: %v; exists: %v; obj.server == nil: %t", newCluster, exists, obj.server == nil)
	}
	// XXX check if i have actually volunteered first of all...
	if obj.server == nil && (newCluster || exists) {

		log.Printf("Etcd: StartServer(newCluster: %t): %+v", newCluster, obj.nominated)
		err := obj.StartServer(
			newCluster,    // newCluster
			obj.nominated, // other peer members and urls or empty map
		)
		if err != nil {
			// retry maxStartServerRetries times, then permanently fail
			return &CtxRetriesErr{maxStartServerRetries - re.retries, fmt.Sprintf("Etcd: StartServer: Error: %+v", err)}
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

			// XXX: just put this wherever for now so we don't block
			// nominate self so "member" list is correct for peers to see
			EtcdNominate(obj, obj.hostname, obj.serverURLs)
			// XXX if this fails, where will we retry this part ?
		}

		// advertise client urls
		if curls := obj.clientURLs; len(curls) > 0 {
			// XXX: don't advertise local addresses! 127.0.0.1:2381 doesn't really help remote hosts
			// XXX: but sometimes this is what we want... hmmm how do we decide? filter on callback?
			EtcdAdvertiseEndpoints(obj, curls)
			// XXX if this fails, where will we retry this part ?

			// force this to remove sentinel before we reconnect...
			obj.endpointCallback(nil)
		}

		return &CtxReconnectErr{"local server is running"} // trigger reconnect to self

	} else if obj.server != nil && !exists {
		// un advertise client urls
		EtcdAdvertiseEndpoints(obj, nil)

		// i have been un-nominated, remove self and shutdown server!
		if len(obj.nominated) != 0 { // don't call if nobody left but me!
			// this works around: https://github.com/coreos/etcd/issues/5482,
			// and it probably makes sense to avoid calling if we're the last
			log.Printf("Etcd: Member Remove: Removing self: %v", obj.memberId)
			removed, err := EtcdMemberRemove(obj, obj.memberId)
			if err != nil {
				return fmt.Errorf("Etcd: Member Remove: Error: %+v", err)
			}
			if removed {
				log.Printf("Etcd: Member Removed (self): %v(%v)", obj.hostname, obj.memberId)
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

// endpointCallback runs to respond to the endpoint list change events
func (obj *EmbdEtcd) endpointCallback(re *RE) error {
	if TRACE {
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
		endpoints, err = EtcdEndpoints(obj)
		if err != nil {
			return err
		}
	}

	// change detection
	var changed bool = false // do we need to update?
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

// idealClusterSizeCallback runs to respond to the ideal cluster size changes
func (obj *EmbdEtcd) idealClusterSizeCallback(re *RE) error {
	if TRACE {
		log.Printf("Trace: Etcd: idealClusterSizeCallback()")
		defer log.Printf("Trace: Etcd: idealClusterSizeCallback(): Finished!")
	}
	path := fmt.Sprintf("/%s/idealClusterSize", NS)
	for _, event := range re.response.Events {
		key := bytes.NewBuffer(event.Kv.Key).String()
		if key != path {
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

// LocalhostClientURLs returns the most localhost like URLs for direct connection
// this gets clients to talk to the local servers first before searching remotely
func (obj *EmbdEtcd) LocalhostClientURLs() etcdtypes.URLs {
	// look through obj.clientURLs and return the localhost ones
	urls := etcdtypes.URLs{}
	for _, x := range obj.clientURLs {
		// "localhost" or anything in 127.0.0.0/8 is valid!
		if s := x.Host; strings.HasPrefix(s, "localhost") || strings.HasPrefix(s, "127.") {
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

	// embed etcd
	cfg := embed.NewConfig()
	cfg.Name = memberName // hostname
	cfg.Dir = obj.dataDir
	cfg.ACUrls = obj.clientURLs
	cfg.APUrls = peerURLs
	cfg.LCUrls = obj.clientURLs
	cfg.LPUrls = peerURLs

	cfg.InitialCluster = initialPeerURLsMap.String() // including myself!
	if newCluster {
		cfg.ClusterState = embed.ClusterStateFlagNew
	} else {
		cfg.ClusterState = embed.ClusterStateFlagExisting
	}
	//cfg.ForceNewCluster = newCluster // TODO ?

	log.Printf("Etcd: StartServer: Starting server...")
	obj.server, err = embed.StartEtcd(cfg)                 // we hang here if things are bad
	log.Printf("Etcd: StartServer: Done starting server!") // it didn't hang!
	if err != nil {
		return err
	}
	//log.Fatal(<-obj.server.Err())	XXX
	log.Printf("Etcd: StartServer: Server running...")
	obj.memberId = uint64(obj.server.Server.ID()) // store member id for internal use

	obj.serverwg.Add(1)
	return nil
}

// DestroyServer shuts down the embedded etcd server portion
func (obj *EmbdEtcd) DestroyServer() error {
	var err error
	log.Printf("Etcd: DestroyServer: Destroying...")
	if obj.server != nil {
		obj.server.Close() // this blocks until server has stopped
	}
	log.Printf("Etcd: DestroyServer: Done closing...")

	obj.memberId = 0
	if obj.server == nil { // skip the .Done() below because we didn't .Add(1) it.
		return err
	}
	obj.server = nil // important because this is used as an isRunning flag
	log.Printf("Etcd: DestroyServer: Unlocking server...")
	obj.serverwg.Done() // -1
	return err
}

// TODO: Could all these Etcd*(obj *EmbdEtcd, ...) functions which deal with the
// interface between etcd paths and behaviour be grouped into a single struct ?

// EtcdNominate nominates a particular client to be a server (peer)
func EtcdNominate(obj *EmbdEtcd, hostname string, urls etcdtypes.URLs) error {
	if TRACE {
		log.Printf("Trace: Etcd: EtcdNominate(%v): %v", hostname, urls.String())
		defer log.Printf("Trace: Etcd: EtcdNominate(%v): Finished!", hostname)
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
		return fmt.Errorf("Etcd: Nominate failed!") // exit in progress?
	}
	return nil
}

// EtcdNominated returns a urls map of nominated etcd server volunteers
// NOTE: I know 'nominees' might be more correct, but is less consistent here
func EtcdNominated(obj *EmbdEtcd) (etcdtypes.URLsMap, error) {
	path := fmt.Sprintf("/%s/nominated/", NS)
	keyMap, err := obj.Get(path, etcd.WithPrefix()) // map[string]string, bool
	if err != nil {
		return nil, fmt.Errorf("Etcd: Nominated isn't available: %v", err)
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
			return nil, fmt.Errorf("Etcd: Nominated: Data format error!: %v", err)
		}
		nominated[name] = urls // add to map
		if DEBUG {
			log.Printf("Etcd: Nominated(%v): %v", name, val)
		}
	}
	return nominated, nil
}

// EtcdVolunteer offers yourself up to be a server if needed
func EtcdVolunteer(obj *EmbdEtcd, urls etcdtypes.URLs) error {
	if TRACE {
		log.Printf("Trace: Etcd: EtcdVolunteer(%v): %v", obj.hostname, urls.String())
		defer log.Printf("Trace: Etcd: EtcdVolunteer(%v): Finished!", obj.hostname)
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
		return fmt.Errorf("Etcd: Volunteering failed!") // exit in progress?
	}
	return nil
}

// EtcdVolunteers returns a urls map of available etcd server volunteers
func EtcdVolunteers(obj *EmbdEtcd) (etcdtypes.URLsMap, error) {
	if TRACE {
		log.Printf("Trace: Etcd: EtcdVolunteers()")
		defer log.Printf("Trace: Etcd: EtcdVolunteers(): Finished!")
	}
	path := fmt.Sprintf("/%s/volunteers/", NS)
	keyMap, err := obj.Get(path, etcd.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("Etcd: Volunteers aren't available: %v", err)
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
			return nil, fmt.Errorf("Etcd: Volunteers: Data format error!: %v", err)
		}
		volunteers[name] = urls // add to map
		if DEBUG {
			log.Printf("Etcd: Volunteer(%v): %v", name, val)
		}
	}
	return volunteers, nil
}

// EtcdAdvertiseEndpoints advertises the list of available client endpoints
func EtcdAdvertiseEndpoints(obj *EmbdEtcd, urls etcdtypes.URLs) error {
	if TRACE {
		log.Printf("Trace: Etcd: EtcdAdvertiseEndpoints(%v): %v", obj.hostname, urls.String())
		defer log.Printf("Trace: Etcd: EtcdAdvertiseEndpoints(%v): Finished!", obj.hostname)
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
		return fmt.Errorf("Etcd: Endpoint advertising failed!") // exit in progress?
	}
	return nil
}

// EtcdEndpoints returns a urls map of available etcd server endpoints
func EtcdEndpoints(obj *EmbdEtcd) (etcdtypes.URLsMap, error) {
	if TRACE {
		log.Printf("Trace: Etcd: EtcdEndpoints()")
		defer log.Printf("Trace: Etcd: EtcdEndpoints(): Finished!")
	}
	path := fmt.Sprintf("/%s/endpoints/", NS)
	keyMap, err := obj.Get(path, etcd.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("Etcd: Endpoints aren't available: %v", err)
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
			return nil, fmt.Errorf("Etcd: Endpoints: Data format error!: %v", err)
		}
		endpoints[name] = urls // add to map
		if DEBUG {
			log.Printf("Etcd: Endpoint(%v): %v", name, val)
		}
	}
	return endpoints, nil
}

// EtcdSetClusterSize sets the ideal target cluster size of etcd peers
func EtcdSetClusterSize(obj *EmbdEtcd, value uint16) error {
	if TRACE {
		log.Printf("Trace: Etcd: EtcdSetClusterSize(): %v", value)
		defer log.Printf("Trace: Etcd: EtcdSetClusterSize(): Finished!")
	}
	key := fmt.Sprintf("/%s/idealClusterSize", NS)

	if err := obj.Set(key, strconv.FormatUint(uint64(value), 10)); err != nil {
		return fmt.Errorf("Etcd: SetClusterSize failed!") // exit in progress?
	}
	return nil
}

// EtcdGetClusterSize gets the ideal target cluster size of etcd peers
func EtcdGetClusterSize(obj *EmbdEtcd) (uint16, error) {
	key := fmt.Sprintf("/%s/idealClusterSize", NS)
	keyMap, err := obj.Get(key)
	if err != nil {
		return 0, fmt.Errorf("Etcd: GetClusterSize failed: %v", err)
	}

	val, exists := keyMap[key]
	if !exists || val == "" {
		return 0, fmt.Errorf("Etcd: GetClusterSize failed: %v", err)
	}

	v, err := strconv.ParseUint(val, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("Etcd: GetClusterSize failed: %v", err)
	}
	return uint16(v), nil
}

func EtcdMemberAdd(obj *EmbdEtcd, peerURLs etcdtypes.URLs) (*etcd.MemberAddResponse, error) {
	//obj.Connect(false) // TODO ?
	ctx := context.Background()
	var response *etcd.MemberAddResponse
	var err error
	for {
		if obj.exiting { // the exit signal has been sent!
			return nil, fmt.Errorf("Exiting...")
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

// EtcdMemberRemove removes a member by mID and returns if it worked, and also
// if there was an error. This is because It might have run without error, but
// the member wasn't found, for example.
func EtcdMemberRemove(obj *EmbdEtcd, mID uint64) (bool, error) {
	//obj.Connect(false) // TODO ?
	ctx := context.Background()
	for {
		if obj.exiting { // the exit signal has been sent!
			return false, fmt.Errorf("Exiting...")
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

// EtcdMembers returns information on cluster membership.
// The member ID's are the keys, because an empty names means unstarted!
// TODO: consider queueing this through the main loop with CtxError(ctx, err)
func EtcdMembers(obj *EmbdEtcd) (map[uint64]string, error) {
	//obj.Connect(false) // TODO ?
	ctx := context.Background()
	var response *etcd.MemberListResponse
	var err error
	for {
		if obj.exiting { // the exit signal has been sent!
			return nil, fmt.Errorf("Exiting...")
		}
		obj.rLock.RLock()
		if TRACE {
			log.Printf("Trace: Etcd: EtcdMembers(): Endpoints are: %v", obj.client.Endpoints())
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

// EtcdLeader returns the current leader of the etcd server cluster
func EtcdLeader(obj *EmbdEtcd) (string, error) {
	//obj.Connect(false) // TODO ?
	var err error
	membersMap := make(map[uint64]string)
	if membersMap, err = EtcdMembers(obj); err != nil {
		return "", err
	}
	addresses := obj.LocalhostClientURLs() // heuristic, but probably correct
	if len(addresses) == 0 {
		// probably a programming error...
		return "", fmt.Errorf("Etcd: Leader: Programming error!")
	}
	endpoint := addresses[0].Host // FIXME: arbitrarily picked the first one

	// part two
	ctx := context.Background()
	var response *etcd.StatusResponse
	for {
		if obj.exiting { // the exit signal has been sent!
			return "", fmt.Errorf("Exiting...")
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
	return "", fmt.Errorf("Etcd: Members map is not current!") // not found
}

// EtcdWatch returns a channel that outputs a true bool when activity occurs
// TODO: Filter our watch (on the server side if possible) based on the
// collection prefixes and filters that we care about...
func EtcdWatch(obj *EmbdEtcd) chan bool {
	ch := make(chan bool, 1) // buffer it so we can measure it
	path := fmt.Sprintf("/%s/exported/", NS)
	callback := func(re *RE) error {
		// TODO: is this even needed? it used to happen on conn errors
		log.Printf("Etcd: Watch: Path: %v", path) // event
		if re == nil || re.response.Canceled {
			return fmt.Errorf("Etcd: Watch is empty!") // will cause a CtxError+retry
		}
		// we normally need to check if anything changed since the last
		// event, since a set (export) with no changes still causes the
		// watcher to trigger and this would cause an infinite loop. we
		// don't need to do this check anymore because we do the export
		// transactionally, and only if a change is needed. since it is
		// atomic, all the changes arrive together which avoids dupes!!
		if len(ch) == 0 { // send event only if one isn't pending
			// this check avoids multiple events all queueing up and then
			// being released continuously long after the changes stopped
			// do not block!
			ch <- true // event
		}
		return nil
	}
	_, _ = obj.AddWatcher(path, callback, true, etcd.WithPrefix()) // no need to check errors
	return ch
}

// EtcdSetResources exports all of the resources which we pass in to etcd
func EtcdSetResources(obj *EmbdEtcd, hostname string, resources []Res) error {
	// key structure is /$NS/exported/$hostname/resources/$uuid = $data

	var kindFilter []string // empty to get from everyone
	hostnameFilter := []string{hostname}
	// this is not a race because we should only be reading keys which we
	// set, and there should not be any contention with other hosts here!
	originals, err := EtcdGetResources(obj, hostnameFilter, kindFilter)
	if err != nil {
		return err
	}

	if len(originals) == 0 && len(resources) == 0 { // special case of no add or del
		return nil
	}

	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this transaction
	for _, res := range resources {
		if res.Kind() == "" {
			log.Fatalf("Etcd: SetResources: Error: Empty kind: %v", res.GetName())
		}
		uuid := fmt.Sprintf("%s/%s", res.Kind(), res.GetName())
		path := fmt.Sprintf("/%s/exported/%s/resources/%s", NS, hostname, uuid)
		if data, err := ResToB64(res); err == nil {
			ifs = append(ifs, etcd.Compare(etcd.Value(path), "=", data)) // desired state
			ops = append(ops, etcd.OpPut(path, data))
		} else {
			return fmt.Errorf("Etcd: SetResources: Error: Can't convert to B64: %v", err)
		}
	}

	match := func(res Res, resources []Res) bool { // helper lambda
		for _, x := range resources {
			if res.Kind() == x.Kind() && res.GetName() == x.GetName() {
				return true
			}
		}
		return false
	}

	hasDeletes := false
	// delete old, now unused resources here...
	for _, res := range originals {
		if res.Kind() == "" {
			log.Fatalf("Etcd: SetResources: Error: Empty kind: %v", res.GetName())
		}
		uuid := fmt.Sprintf("%s/%s", res.Kind(), res.GetName())
		path := fmt.Sprintf("/%s/exported/%s/resources/%s", NS, hostname, uuid)

		if match(res, resources) { // if we match, no need to delete!
			continue
		}

		ops = append(ops, etcd.OpDelete(path))

		hasDeletes = true
	}

	// if everything is already correct, do nothing, otherwise, run the ops!
	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	if hasDeletes { // always run, ifs don't matter
		_, err = obj.Txn(nil, ops, nil) // TODO: does this run? it should!
	} else {
		_, err = obj.Txn(ifs, nil, ops) // TODO: do we need to look at response?
	}
	return err
}

// EtcdGetResources collects all of the resources which match a filter from etcd
// If the kindfilter or hostnameFilter is empty, then it assumes no filtering...
// TODO: Expand this with a more powerful filter based on what we eventually
// support in our collect DSL. Ideally a server side filter like WithFilter()
// We could do this if the pattern was /$NS/exported/$kind/$hostname/$uuid = $data
func EtcdGetResources(obj *EmbdEtcd, hostnameFilter, kindFilter []string) ([]Res, error) {
	// key structure is /$NS/exported/$hostname/resources/$uuid = $data
	path := fmt.Sprintf("/%s/exported/", NS)
	resources := []Res{}
	keyMap, err := obj.Get(path, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
	if err != nil {
		return nil, fmt.Errorf("Etcd: GetResources: Error: Could not get resources: %v", err)
	}
	for key, val := range keyMap {
		if !strings.HasPrefix(key, path) { // sanity check
			continue
		}

		str := strings.Split(key[len(path):], "/")
		if len(str) != 4 {
			return nil, fmt.Errorf("Etcd: GetResources: Error: Unexpected chunk count!")
		}
		hostname, r, kind, name := str[0], str[1], str[2], str[3]
		if r != "resources" {
			return nil, fmt.Errorf("Etcd: GetResources: Error: Unexpected chunk pattern!")
		}
		if kind == "" {
			return nil, fmt.Errorf("Etcd: GetResources: Error: Unexpected kind chunk!")
		}

		// FIXME: ideally this would be a server side filter instead!
		if len(hostnameFilter) > 0 && !StrInList(hostname, hostnameFilter) {
			continue
		}

		// FIXME: ideally this would be a server side filter instead!
		if len(kindFilter) > 0 && !StrInList(kind, kindFilter) {
			continue
		}

		if obj, err := B64ToRes(val); err == nil {
			obj.setKind(kind) // cheap init
			log.Printf("Etcd: Get: (Hostname, Kind, Name): (%s, %s, %s)", hostname, kind, name)
			resources = append(resources, obj)
		} else {
			return nil, fmt.Errorf("Etcd: GetResources: Error: Can't convert from B64: %v", err)
		}
	}
	return resources, nil
}

//func UrlRemoveScheme(urls etcdtypes.URLs) []string {
//	strs := []string{}
//	for _, u := range urls {
//		strs = append(strs, u.Host) // remove http:// prefix
//	}
//	return strs
//}

// ApplyDeltaEvents modifies a URLsMap with the deltas from a WatchResponse
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
				return nil, fmt.Errorf("Etcd: ApplyDeltaEvents: Value is empty!")
			}
			urls, err := etcdtypes.NewURLs(strings.Split(val, ","))
			if err != nil {
				return nil, fmt.Errorf("Etcd: ApplyDeltaEvents: Format error: %v", err)
			}
			urlsmap[key] = urls // add to map

		// expiry cases are seen as delete in v3 for now
		//case etcd.EventTypeExpire: // doesn't exist right now
		//	fallthrough
		case etcd.EventTypeDelete:
			if _, exists := urlsmap[key]; !exists {
				// this can happen if we retry an operation b/w
				// a reconnect so ignore if we are reconnecting
				if DEBUG {
					log.Printf("Etcd: ApplyDeltaEvents: Inconsistent key: %v", key)
				}
				return nil, ErrApplyDeltaEventsInconsistent
			}
			delete(urlsmap, key)

		default:
			return nil, fmt.Errorf("Etcd: ApplyDeltaEvents: Error: Unknown event: %+v", event.Type)
		}
	}
	return urlsmap, nil
}
