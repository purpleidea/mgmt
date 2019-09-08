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

package lib

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph"
	"github.com/purpleidea/mgmt/engine/graph/autogroup"
	_ "github.com/purpleidea/mgmt/engine/resources" // let register's run
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/etcd/chooser"
	"github.com/purpleidea/mgmt/etcd/deployer"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/gapi/empty"
	"github.com/purpleidea/mgmt/pgp"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/prometheus"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	etcdtypes "github.com/coreos/etcd/pkg/types"
)

const (
	// NS is the root namespace for etcd operations. All keys must use it!
	NS = "/_mgmt" // must not end with a slash!
)

// Flags are some constant flags which are used throughout the program.
type Flags struct {
	Debug   bool // add additional log messages
	Verbose bool // add extra log message output
}

// Main is the main struct for running the mgmt logic.
type Main struct {
	Program string // the name of this program, usually set at compile time
	Version string // the version of this program, usually set at compile time

	Flags Flags // static global flags that are set at compile time

	Hostname *string // hostname to use; nil if undefined

	Prefix         *string // prefix passed in; nil if undefined
	TmpPrefix      bool    // request a pseudo-random, temporary prefix to be used
	AllowTmpPrefix bool    // allow creation of a new temporary prefix if main prefix is unavailable

	Deploy   *gapi.Deploy // deploy object including GAPI for static deploys
	DeployFs engine.Fs    // used for static deploys

	NoWatch       bool // do not change graph under any circumstances
	NoStreamWatch bool // do not update graph due to stream changes
	NoDeployWatch bool // do not change deploys after an initial deploy

	Noop                   bool   // globally force all resources into no-op mode
	Sema                   int    // add a semaphore with this lock count to each resource
	Graphviz               string // output file for graphviz data
	GraphvizFilter         string // graphviz filter to use
	ConvergedTimeout       int64  // approximately this many seconds of inactivity means we're in a converged state; -1 to disable
	ConvergedTimeoutNoExit bool   // don't exit on converged timeout
	ConvergedStatusFile    string // file to append converged status to
	MaxRuntime             uint   // exit after a maximum of approximately this many seconds

	Seeds               []string // default etc client endpoint
	ClientURLs          []string // list of URLs to listen on for client traffic
	ServerURLs          []string // list of URLs to listen on for server (peer) traffic
	AdvertiseClientURLs []string // list of URLs to advertise for client traffic
	AdvertiseServerURLs []string // list of URLs to advertise for server (peer) traffic
	IdealClusterSize    int      // ideal number of server peers in cluster; only read by initial server
	NoServer            bool     // do not let other servers peer with me
	NoNetwork           bool     // run single node instance without clustering or opening tcp ports to the outside

	seeds               etcdtypes.URLs // processed seeds value
	clientURLs          etcdtypes.URLs // processed client urls value
	serverURLs          etcdtypes.URLs // processed server urls value
	advertiseClientURLs etcdtypes.URLs // processed advertise client urls value
	advertiseServerURLs etcdtypes.URLs // processed advertise server urls value
	idealClusterSize    uint16         // processed ideal cluster size value

	NoPgp       bool    // disallow pgp functionality
	PgpKeyPath  *string // import a pre-made key pair
	PgpIdentity *string
	pgpKeys     *pgp.PGP // agent key pair

	Prometheus       bool   // enable prometheus metrics
	PrometheusListen string // prometheus instance bind specification

	embdEtcd *etcd.EmbdEtcd // TODO: can be an interface in the future...
	ge       *graph.Engine

	exit    *util.EasyExit // exit signal
	cleanup []func() error // list of functions to run on close
}

// Validate validates the main structure without making any modifications to it.
func (obj *Main) Validate() error {
	if obj.Program == "" || obj.Version == "" {
		return fmt.Errorf("you must set the Program and Version strings")
	}
	if strings.Contains(obj.Program, " ") {
		return fmt.Errorf("the Program string contains unexpected spaces")
	}

	if obj.Prefix != nil && obj.TmpPrefix {
		return fmt.Errorf("choosing a prefix and the request for a tmp prefix is illogical")
	}

	return nil
}

// Init initializes the main struct after it performs some validation.
func (obj *Main) Init() error {
	// if we've turned off watching, then be explicit and disable them all!
	// if all the watches are disabled, then it's equivalent to no watching
	if obj.NoWatch {
		obj.NoStreamWatch = true
	} else if obj.NoStreamWatch {
		obj.NoWatch = true
	}

	obj.idealClusterSize = uint16(obj.IdealClusterSize)
	if obj.IdealClusterSize < 0 { // value is undefined, set to the default
		obj.idealClusterSize = chooser.DefaultIdealDynamicSize
	}

	if obj.idealClusterSize < 1 {
		return fmt.Errorf("the IdealClusterSize (%d) should be at least one", obj.idealClusterSize)
	}

	// transform the url list inputs into etcd typed lists
	var err error
	obj.seeds, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.Seeds, []string{",", ";", " "}),
	)
	if err != nil && len(obj.Seeds) > 0 {
		return errwrap.Wrapf(err, "the Seeds didn't parse correctly")
	}
	obj.clientURLs, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.ClientURLs, []string{",", ";", " "}),
	)
	if err != nil && len(obj.ClientURLs) > 0 {
		return errwrap.Wrapf(err, "the ClientURLs didn't parse correctly")
	}
	obj.serverURLs, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.ServerURLs, []string{",", ";", " "}),
	)
	if err != nil && len(obj.ServerURLs) > 0 {
		return errwrap.Wrapf(err, "the ServerURLs didn't parse correctly")
	}
	obj.advertiseClientURLs, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.AdvertiseClientURLs, []string{",", ";", " "}),
	)
	if err != nil && len(obj.AdvertiseClientURLs) > 0 {
		return errwrap.Wrapf(err, "the AdvertiseClientURLs didn't parse correctly")
	}
	obj.advertiseServerURLs, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.AdvertiseServerURLs, []string{",", ";", " "}),
	)
	if err != nil && len(obj.AdvertiseServerURLs) > 0 {
		return errwrap.Wrapf(err, "the AdvertiseServerURLs didn't parse correctly")
	}

	obj.exit = util.NewEasyExit()
	obj.cleanup = []func() error{}
	return nil
}

// Run is the main execution entrypoint to run mgmt.
func (obj *Main) Run() error {
	Logf := func(format string, v ...interface{}) {
		log.Printf("main: "+format, v...)
	}

	hello(obj.Program, obj.Version, obj.Flags) // say hello!
	defer Logf("goodbye!")

	exitCtx := obj.exit.Context() // local exit signal
	defer obj.exit.Done(nil)      // ensure this gets called even if Exit doesn't

	hostname, err := os.Hostname() // a sensible default
	// allow passing in the hostname, instead of using the system setting
	if h := obj.Hostname; h != nil && *h != "" { // override by cli
		hostname = *h
	} else if err != nil {
		return errwrap.Wrapf(err, "can't get default hostname")
	}
	if hostname == "" { // safety check
		return fmt.Errorf("hostname cannot be empty")
	}

	user, err := user.Current()
	if err != nil {
		return errwrap.Wrapf(err, "can't get current user")
	}

	// Use systemd StateDirectory if set. If not, use XDG_CACHE_DIR unless user
	// is root, then use /var/lib/mgmt/.
	var prefix = fmt.Sprintf("/var/lib/%s/", obj.Program) // default prefix
	stateDir := os.Getenv("STATE_DIRECTORY")
	// Ensure there is a / at the end of the directory path.
	if stateDir != "" && !strings.HasSuffix(stateDir, "/") {
		stateDir = stateDir + "/"
	}

	xdg := os.Getenv("XDG_CACHE_HOME")
	// Ensure there is a / at the end of the directory path.
	if xdg != "" && !strings.HasSuffix(xdg, "/") {
		xdg = xdg + "/"
	}
	if xdg == "" && user.HomeDir != "" {
		xdg = fmt.Sprintf("%s/.cache/%s/", user.HomeDir, obj.Program)
	}

	if stateDir != "" {
		prefix = stateDir
	} else if user.Uid != "0" {
		prefix = xdg
	}

	if p := obj.Prefix; p != nil {
		prefix = *p
	}
	// make sure the working directory prefix exists
	if obj.TmpPrefix || os.MkdirAll(prefix, 0770) != nil {
		if obj.TmpPrefix || obj.AllowTmpPrefix {
			var err error
			if prefix, err = ioutil.TempDir("", obj.Program+"-"+hostname+"-"); err != nil {
				return fmt.Errorf("can't create temporary prefix")
			}
			Logf("warning: working prefix directory is temporary!")

		} else {
			return fmt.Errorf("can't create prefix: `%s`", prefix)
		}
	}
	Logf("working prefix is: %s", prefix)

	var prom *prometheus.Prometheus
	if obj.Prometheus {
		prom = &prometheus.Prometheus{
			Listen: obj.PrometheusListen,
		}
		if err := prom.Init(); err != nil {
			return errwrap.Wrapf(err, "can't initialize prometheus instance")
		}

		Logf("prometheus: starting instance on: %s", prom.Listen)
		if err := prom.Start(); err != nil {
			return errwrap.Wrapf(err, "can't start prometheus instance")
		}

		if err := prom.InitKindMetrics(engine.RegisteredResourcesNames()); err != nil {
			return errwrap.Wrapf(err, "can't initialize kind-specific prometheus metrics")
		}
		defer func() {
			Logf("prometheus: stopping instance")
			err := errwrap.Wrapf(prom.Stop(), "the prometheus instance exited poorly")
			if err != nil {
				// TODO: cause the final exit code to be non-zero
				Logf("cleanup error: %+v", err)
			}
		}()
	}

	if !obj.NoPgp {
		pgpPrefix := fmt.Sprintf("%s/", path.Join(prefix, "pgp"))
		if err := os.MkdirAll(pgpPrefix, 0770); err != nil {
			return errwrap.Wrapf(err, "can't create pgp prefix")
		}

		pgpKeyringPath := path.Join(pgpPrefix, pgp.DefaultKeyringFile) // default path

		if p := obj.PgpKeyPath; p != nil {
			pgpKeyringPath = *p
		}

		var err error
		if obj.pgpKeys, err = pgp.Import(pgpKeyringPath); err != nil && !os.IsNotExist(err) {
			return errwrap.Wrapf(err, "can't import pgp key")
		}

		if obj.pgpKeys == nil {
			identity := fmt.Sprintf("%s <%s> %s", obj.Program, "root@"+hostname, "generated by "+obj.Program)
			if p := obj.PgpIdentity; p != nil {
				identity = *p
			}

			name, comment, email, err := pgp.ParseIdentity(identity)
			if err != nil {
				return errwrap.Wrapf(err, "can't parse user string")

			}

			// TODO: Make hash configurable
			if obj.pgpKeys, err = pgp.Generate(name, comment, email, nil); err != nil {
				return errwrap.Wrapf(err, "can't create pgp key")
			}

			if err := obj.pgpKeys.SaveKey(pgpKeyringPath); err != nil {
				return errwrap.Wrapf(err, "can't save pgp key")
			}
		}

		// TODO: Import admin key
	}

	exitchan := make(chan struct{}) // exit on close
	wg := &sync.WaitGroup{}         // waitgroup for inner loop & goroutines
	defer wg.Wait()                 // wait in case we have an early exit
	defer obj.exit.Done(nil)        // trigger exit in case something blocks

	// exit after `max-runtime` seconds for no reason at all...
	if i := obj.MaxRuntime; i > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-time.After(time.Duration(i) * time.Second):
				obj.exit.Done(fmt.Errorf("max runtime reached")) // trigger exit signal
			case <-obj.exit.Signal(): // exit early on exit signal
				return
			}
		}()
	}

	// setup converger
	converger := converger.New(
		obj.ConvergedTimeout,
	)
	if obj.ConvergedStatusFile != "" {
		converger.AddStateFn("status-file", func(converged bool) error {
			Logf("converged status is: %t", converged)
			return appendConvergedStatus(obj.ConvergedStatusFile, converged)
		})
	}

	if obj.ConvergedTimeout >= 0 && !obj.ConvergedTimeoutNoExit {
		converger.AddStateFn("converged-exit", func(converged bool) error {
			if converged {
				Logf("converged for %d seconds, exiting!", obj.ConvergedTimeout)
				obj.exit.Done(nil) // trigger an exit!
			}
			return nil
		})
	}

	// XXX: should this be moved to later in the code?
	go converger.Run(true) // main loop for converger, true to start paused
	converger.Ready()      // block until ready
	defer func() {
		// TODO: shutdown converger, but make sure that using it in a
		// still running embdEtcd struct doesn't block waiting on it...
		converger.Shutdown()
	}()

	// embedded etcd
	if len(obj.seeds) == 0 {
		Logf("no seeds specified!")
	} else {
		Logf("seeds(%d): %+v", len(obj.seeds), obj.seeds)
	}
	obj.embdEtcd = &etcd.EmbdEtcd{
		Hostname: hostname,
		Seeds:    obj.seeds,

		ClientURLs:  obj.clientURLs,
		ServerURLs:  obj.serverURLs,
		AClientURLs: obj.advertiseClientURLs,
		AServerURLs: obj.advertiseServerURLs,

		NoServer:  obj.NoServer,
		NoNetwork: obj.NoNetwork,

		Chooser: &chooser.DynamicSize{
			IdealClusterSize: obj.idealClusterSize,
		},

		Converger: converger,

		NS:     NS, // namespace
		Prefix: fmt.Sprintf("%s/", path.Join(prefix, "etcd")),

		Debug: obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("etcd: "+format, v...)
		},
	}
	if err := obj.embdEtcd.Init(); err != nil {
		return errwrap.Wrapf(err, "etcd init failed")
	}
	defer func() {
		// cleanup etcd main loop last so it can process everything first
		err := errwrap.Wrapf(obj.embdEtcd.Close(), "etcd close failed")
		if err != nil {
			// TODO: cause the final exit code to be non-zero
			Logf("cleanup error: %+v", err)
		}
	}()

	var etcdErr error
	// don't add a wait group here, this is done in embdEtcd.Destroy()
	go func() {
		etcdErr = obj.embdEtcd.Run()                             // returns when it shuts down...
		obj.exit.Done(errwrap.Wrapf(etcdErr, "etcd run failed")) // trigger exit
	}()
	// tell etcd to shutdown, blocks until done!
	// TODO: handle/report error?
	defer obj.embdEtcd.Destroy()

	// wait for etcd to be ready before continuing...
	// TODO: do we need to add a timeout here?
	select {
	case <-obj.embdEtcd.Ready():
		Logf("etcd is ready!")
		// pass

	case <-obj.embdEtcd.Exited():
		Logf("etcd was destroyed!")
		err := fmt.Errorf("etcd was destroyed on startup")
		if etcdErr != nil {
			err = etcdErr
		}
		return err
	}
	// TODO: should getting a client from EmbdEtcd already come with the NS?
	etcdClient, err := obj.embdEtcd.MakeClientFromNamespace(NS)
	if err != nil {
		return errwrap.Wrapf(err, "make Client failed")
	}
	simpleDeploy := &deployer.SimpleDeploy{
		Client: etcdClient,
		Debug:  obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("deploy: "+format, v...)
		},
	}
	if err := simpleDeploy.Init(); err != nil {
		return errwrap.Wrapf(err, "deploy Init failed")
	}
	defer func() {
		err := errwrap.Wrapf(simpleDeploy.Close(), "deploy Close failed")
		if err != nil {
			// TODO: cause the final exit code to be non-zero
			Logf("cleanup error: %+v", err)
		}
	}()

	// implementation of the World API (alternatives can be substituted in)
	world := &etcd.World{
		Hostname:       hostname,
		Client:         etcdClient,
		MetadataPrefix: MetadataPrefix,
		StoragePrefix:  StoragePrefix,
		StandaloneFs:   obj.DeployFs, // used for static deploys
		Debug:          obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("world: etcd: "+format, v...)
		},
	}

	obj.ge = &graph.Engine{
		Program:   obj.Program,
		Hostname:  hostname,
		World:     world,
		Prefix:    fmt.Sprintf("%s/", path.Join(prefix, "engine")),
		Converger: converger,
		//Prometheus: prom, // TODO: implement this via a general Status API
		Debug: obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("engine: "+format, v...)
		},
	}

	if err := obj.ge.Init(); err != nil {
		return errwrap.Wrapf(err, "engine Init failed")
	}
	defer func() {
		err := errwrap.Wrapf(obj.ge.Close(), "engine Close failed")
		if err != nil {
			// TODO: cause the final exit code to be non-zero
			Logf("cleanup error: %+v", err)
		}
	}()
	// After this point, the inner "main loop" will run, so that the engine
	// can get closed with the deploy close via the deploy chan shutdown...

	// main loop logic starts here
	deployChan := make(chan *gapi.Deploy)
	var gapiImpl gapi.GAPI // active GAPI implementation
	gapiImpl = nil         // starts off missing

	var gapiChan chan gapi.Next // stream events contain some instructions!
	gapiChan = nil              // starts off blocked
	wg.Add(1)
	go func() {
		defer Logf("loop: exited")
		defer wg.Done()
		started := false // track engine started state
		var mainDeploy *gapi.Deploy
		for {
			Logf("waiting...")
			// The GAPI should always kick off an event on Next() at
			// startup when (and if) it indeed has a graph to share!
			fastPause := false
			select {
			case deploy, ok := <-deployChan:
				if !ok { // channel closed
					Logf("deploy: exited")
					deployChan = nil // disable it

					if gapiImpl != nil { // currently running...
						gapiChan = nil
						if err := gapiImpl.Close(); err != nil {
							err = errwrap.Wrapf(err, "the gapi closed poorly")
							Logf("deploy: gapi: final close failed: %+v", err)
						}
					}

					if started {
						obj.ge.Pause(false)
					}
					// must be paused before this is run
					//obj.ge.Close() // run in defer instead

					return // this is the only place we exit
				}
				if deploy == nil {
					Logf("deploy: received empty deploy")
					continue
				}
				mainDeploy = deploy // save this one
				if id := mainDeploy.ID; id != 0 {
					Logf("deploy: got id: %d", id)
				}
				gapiObj := mainDeploy.GAPI
				if gapiObj == nil {
					Logf("deploy: received empty gapi")
					continue
				}

				if gapiImpl != nil { // currently running...
					gapiChan = nil
					if err := gapiImpl.Close(); err != nil {
						err = errwrap.Wrapf(err, "the gapi closed poorly")
						Logf("deploy: gapi: close failed: %+v", err)
					}
				}
				gapiImpl = gapiObj // copy it to active

				data := &gapi.Data{
					Program:  obj.Program,
					Hostname: hostname,
					World:    world,
					Noop:     mainDeploy.Noop,
					// FIXME: should the below flags come from the deploy struct?
					//NoWatch:  obj.NoWatch,
					NoStreamWatch: obj.NoStreamWatch,
					Prefix:        fmt.Sprintf("%s/", path.Join(prefix, "gapi")),
					Debug:         obj.Flags.Debug,
					Logf: func(format string, v ...interface{}) {
						log.Printf("gapi: "+format, v...)
					},
				}
				if obj.Flags.Debug {
					Logf("gapi: init...")
				}
				if err := gapiImpl.Init(data); err != nil {
					Logf("gapi: init failed: %+v", err)
					// TODO: consider running previous GAPI?
				} else {
					if obj.Flags.Debug {
						Logf("gapi: next...")
					}
					// this must generate at least one event for it to work
					gapiChan = gapiImpl.Next() // stream of graph switch events!
				}
				continue

			case next, ok := <-gapiChan:
				if !ok { // channel closed
					if obj.Flags.Debug {
						Logf("gapi exited")
					}
					gapiChan = nil // disable it
					continue
				}

				// if we've been asked to exit...
				// TODO: do we want to block exits and wait?
				// TODO: we might want to wait for the next GAPI
				if next.Exit {
					obj.exit.Done(next.Err) // trigger exit
					continue                // wait for exitchan
				}

				// the gapi lets us send an error to the channel
				// this means there was a failure, but not fatal
				if err := next.Err; err != nil {
					Logf("error with graph stream: %+v", err)
					continue // wait for another event
				}
				// everything else passes through to cause a compile!

				fastPause = next.Fast // should we pause fast?

				//case <-exitchan: // we only exit on deployChan close!
				//	return
			}

			if gapiImpl == nil { // TODO: can this ever happen anymore?
				Logf("gapi is empty!")
				continue
			}

			// make the graph from yaml, lib, puppet->yaml, or dsl!
			newGraph, err := gapiImpl.Graph() // generate graph!
			if err != nil {
				Logf("error creating new graph: %+v", err)
				continue
			}
			if obj.Flags.Debug {
				Logf("new graph: %+v", newGraph)
			}

			if err := obj.ge.Load(newGraph); err != nil { // copy in new graph
				Logf("error copying in new graph: %+v", err)
				continue
			}

			if err := obj.ge.Validate(); err != nil { // validate the new graph
				obj.ge.Abort() // delete graph
				Logf("error validating the new graph: %+v", err)
				continue
			}

			// apply the global metaparams to the graph
			if err := obj.ge.Apply(func(graph *pgraph.Graph) error {
				var err error
				for _, v := range graph.Vertices() {
					res, ok := v.(engine.Res)
					if !ok {
						e := fmt.Errorf("vertex `%s` is not a Res", v)
						err = errwrap.Append(err, e)
						continue // we'll catch the error later!
					}

					m := res.MetaParams()
					// apply the global noop parameter if requested
					if mainDeploy.Noop {
						m.Noop = mainDeploy.Noop
					}

					// append the semaphore to each resource
					if mainDeploy.Sema > 0 { // NOTE: size == 0 would block
						// a semaphore with an empty id is valid
						m.Sema = append(m.Sema, fmt.Sprintf(":%d", mainDeploy.Sema))
					}
				}
				return err
			}); err != nil { // apply an operation to the new graph
				obj.ge.Abort() // delete graph
				Logf("error applying operation to the new graph: %+v", err)
				continue
			}

			// XXX: can we change this into a ge.Apply operation?
			// add autoedges; modifies the graph only if no error
			if err := obj.ge.AutoEdge(); err != nil {
				obj.ge.Abort() // delete graph
				Logf("error running auto edges: %+v", err)
				continue
			}

			// XXX: can we change this into a ge.Apply operation?
			// run autogroup; modifies the graph
			if err := obj.ge.AutoGroup(&autogroup.NonReachabilityGrouper{}); err != nil {
				obj.ge.Abort() // delete graph
				Logf("error running auto grouping: %+v", err)
				continue
			}

			// XXX: can we change this into a ge.Apply operation?
			// run reversals; modifies the graph
			if err := obj.ge.Reversals(); err != nil {
				obj.ge.Abort() // delete graph
				Logf("error running the reversals: %+v", err)
				continue
			}

			// Double check before we commit.
			if err := obj.ge.Apply(func(graph *pgraph.Graph) error {
				_, e := graph.TopologicalSort() // am i a dag or not?
				return e
			}); err != nil { // apply an operation to the new graph
				obj.ge.Abort() // delete graph
				Logf("error running the TopologicalSort: %+v", err)
				continue
			}

			// TODO: do we want to do a transitive reduction?
			// FIXME: run a type checker that verifies all the send->recv relationships

			// we need the vertices to be paused to work on them, so
			// run graph vertex LOCK...
			if started { // TODO: we can flatten this check out I think
				converger.Pause()       // FIXME: add sync wait?
				obj.ge.Pause(fastPause) // sync
				started = false
			}

			Logf("commit...")
			if err := obj.ge.Commit(); err != nil {
				// If we fail on commit, we have destructively
				// destroyed the graph, so we must not run it.
				// This graph isn't necessarily destroyed, but
				// since an error is not expected here, we can
				// either shutdown or wait for the next deploy.
				obj.ge.Abort() // delete graph
				Logf("error running commit: %+v", err)
				// block gapi until a newDeploy comes in...
				if gapiImpl != nil { // currently running...
					gapiChan = nil
					if err := gapiImpl.Close(); err != nil {
						err = errwrap.Wrapf(err, "the gapi closed poorly")
						Logf("deploy: gapi: close failed: %+v", err)
					}
				}
				continue // stay paused
			}

			// Start needs to be synchronous because we don't want
			// to loop around and cause a pause before we unpaused.
			// Commit already starts things, but we still need to
			// resume anything that was pre-existing and was paused.
			if err := obj.ge.Resume(); err != nil { // sync
				Logf("error resuming graph: %+v", err)
				continue
			}
			converger.Resume() // after Start()
			started = true

			Logf("graph: %+v", obj.ge.Graph()) // show graph
			if obj.Graphviz != "" {
				filter := obj.GraphvizFilter
				if filter == "" {
					filter = "dot" // directed graph default
				}
				if err := obj.ge.Graph().ExecGraphviz(filter, obj.Graphviz, hostname); err != nil {
					Logf("graphviz: %+v", err)
				} else {
					Logf("graphviz: successfully generated graph!")
				}
			}

			// Call this here because at this point the graph does
			// not know anything about the prometheus instance.
			if err := prom.UpdatePgraphStartTime(); err != nil {
				Logf("prometheus: UpdatePgraphStartTime() errored: %+v", err)
			}
		}
	}()

	// get max id (from all the previous deploys)
	// this is what the existing cluster is already running
	// TODO: add a timeout to context?
	max, err := simpleDeploy.GetMaxDeployID(exitCtx)
	if err != nil {
		close(deployChan) // because we won't close it downstream...
		return errwrap.Wrapf(err, "error getting max deploy id")
	}

	// improved etcd based deploy
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(deployChan) // no more are coming ever!

		// we've been asked to deploy, so do that first...
		if obj.Deploy != nil {
			deploy := obj.Deploy
			// redundant
			deploy.Noop = obj.Noop
			deploy.Sema = obj.Sema

			select {
			case deployChan <- deploy:
				// send
				if obj.Flags.Debug {
					Logf("deploy: sending new gapi")
				}
			case <-exitchan:
				return
			}
		}

		// now we can wait for future deploys, but if we already had an
		// initial deploy from run, don't switch to this unless it's new
		ctx, cancel := context.WithCancel(context.Background())
		watchChan, err := simpleDeploy.WatchDeploy(ctx)
		if err != nil {
			cancel()
			Logf("error starting deploy: %+v", err)
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel() // unblock watch deploy
			select {       // wait until we're ready to shutdown
			case <-exitchan:
			}
		}()
		canceled := false

		var last uint64
		for {
			if obj.NoDeployWatch && (obj.Deploy != nil || last > 0) {
				// block here, because when we close the
				// deployChan it's the signal to tell the engine
				// to actually shutdown...
				select { // wait until we're ready to shutdown
				case <-exitchan:
					return
				}
			}

			select {
			// WatchDeploy should send an initial event now...
			case err, ok := <-watchChan:
				if !ok {
					// TODO: is any of this needed in here?
					if !canceled {
						obj.exit.Done(nil) // regular shutdown
					}
					return
				}
				if err == context.Canceled {
					canceled = true
					continue // channel close is coming...
				}
				if err != nil {
					// TODO: it broke, can we restart?
					obj.exit.Done(errwrap.Wrapf(err, "deploy: watch error"))
					continue
				}
				if obj.Flags.Debug {
					Logf("deploy: got activity")
				}

				//case <-exitchan:
				//	return // exit via channel close instead
			}

			latest, err := simpleDeploy.GetMaxDeployID(ctx) // or zero
			if err != nil {
				Logf("error getting max deploy id: %+v", err)
				continue
			}

			// if we already did the built-in one from run, and this
			// new deploy is not newer than when we started, skip it
			if obj.Deploy != nil && latest <= max {
				// if latest and max are zero, it's okay to loop
				continue
			}

			// if we're doing any deploy, don't run the previous one
			// (this might be useful if we get a double event here!)
			if obj.Deploy == nil && latest <= last && latest != 0 {
				// if latest and last are zero, pass through it!
				continue
			}
			// if we already did a deploy, but we're being asked for
			// this again, then skip over it if it's not a newer one
			if obj.Deploy != nil && latest <= last {
				continue
			}

			// 0 passes through an empty deploy without an error...
			// (unless there is some sort of etcd error that occurs)
			str, err := simpleDeploy.GetDeploy(ctx, latest)
			if err != nil {
				Logf("deploy: error getting deploy: %+v", err)
				continue
			}
			if str == "" { // no available deploys exist yet
				// send an empty deploy... this is done
				// to start up the engine so it can run
				// an empty graph and be ready to swap!
				Logf("deploy: empty")
				deploy := &gapi.Deploy{
					Name: empty.Name,
					GAPI: &empty.GAPI{},
				}
				select {
				case deployChan <- deploy:
					// send
					if obj.Flags.Debug {
						Logf("deploy: sending empty deploy")
					}

				case <-exitchan:
					return
				}
				continue
			}

			// decode the deploy (incl. GAPI) and send it!
			deploy, err := gapi.NewDeployFromB64(str)
			if err != nil {
				Logf("deploy: error decoding deploy: %+v", err)
				continue
			}
			deploy.ID = latest // store the ID

			select {
			case deployChan <- deploy:
				last = latest // update last deployed
				// send
				if obj.Flags.Debug {
					Logf("deploy: sent new gapi")
				}

			case <-exitchan:
				return
			}
		}
	}()

	Logf("running...")

	reterr := obj.exit.Error() // wait for exit signal (block until arrival)

	Logf("destroy...")

	// tell inner main loop to exit
	close(exitchan)
	wg.Wait()

	if reterr != nil {
		Logf("error: %+v", reterr)
	}
	return reterr
}

// Close contains a number of methods which must be run after the Run method.
// You must run them to properly clean up after the main program execution.
func (obj *Main) Close() error {
	var err error

	// run cleanup functions in reverse (defer) order
	for i := len(obj.cleanup) - 1; i >= 0; i-- {
		fn := obj.cleanup[i]
		e := fn()
		err = errwrap.Append(err, e) // list of errors
	}

	return err
}

// Exit causes a safe shutdown. This is often attached to the ^C signal handler.
func (obj *Main) Exit(err error) {
	obj.exit.Done(err) // trigger an exit!
}

// FastExit causes a faster shutdown. This is often activated on the second ^C.
func (obj *Main) FastExit(err error) {
	if obj.ge != nil {
		obj.ge.SetFastPause()
	}
	obj.Exit(err)
}

// Interrupt causes the fastest shutdown. The only faster method is a kill -9
// which could cause corruption. This is often activated on the third ^C. This
// might leave some of your resources in a partial or unknown state.
func (obj *Main) Interrupt(err error) {
	// XXX: implement and run Interrupt API for supported resources
	obj.FastExit(err)

	if obj.embdEtcd != nil {
		obj.embdEtcd.Interrupt() // unblock borked clusters
	}
}
