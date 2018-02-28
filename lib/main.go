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

package lib

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgp"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/prometheus"
	"github.com/purpleidea/mgmt/resources"
	"github.com/purpleidea/mgmt/util"

	etcdtypes "github.com/coreos/etcd/pkg/types"
	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// Flags are some constant flags which are used throughout the program.
type Flags struct {
	Debug   bool // add additional log messages
	Trace   bool // add execution flow log messages
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
	DeployFs resources.Fs // used for static deploys

	NoWatch       bool // do not change graph under any circumstances
	NoConfigWatch bool // do not update graph due to config changes
	NoStreamWatch bool // do not update graph due to stream changes

	Noop             bool   // globally force all resources into no-op mode
	Sema             int    // add a semaphore with this lock count to each resource
	Graphviz         string // output file for graphviz data
	GraphvizFilter   string // graphviz filter to use
	ConvergedTimeout int    // exit after approximately this many seconds in a converged state; -1 to disable
	MaxRuntime       uint   // exit after a maximum of approximately this many seconds

	Seeds               []string // default etc client endpoint
	ClientURLs          []string // list of URLs to listen on for client traffic
	ServerURLs          []string // list of URLs to listen on for server (peer) traffic
	AdvertiseClientURLs []string // list of URLs to advertise for client traffic
	AdvertiseServerURLs []string // list of URLs to advertise for server (peer) traffic
	IdealClusterSize    int      // ideal number of server peers in cluster; only read by initial server
	NoServer            bool     // do not let other servers peer with me

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

	exit chan error // exit signal
}

// Init initializes the main struct after it performs some validation.
func (obj *Main) Init() error {

	if obj.Program == "" || obj.Version == "" {
		return fmt.Errorf("you must set the Program and Version strings")
	}

	if obj.Prefix != nil && obj.TmpPrefix {
		return fmt.Errorf("choosing a prefix and the request for a tmp prefix is illogical")
	}

	// if we've turned off watching, then be explicit and disable them all!
	// if all the watches are disabled, then it's equivalent to no watching
	if obj.NoWatch {
		obj.NoConfigWatch = true
		obj.NoStreamWatch = true
	} else if obj.NoConfigWatch && obj.NoStreamWatch {
		obj.NoWatch = true
	}

	obj.idealClusterSize = uint16(obj.IdealClusterSize)
	if obj.IdealClusterSize < 0 { // value is undefined, set to the default
		obj.idealClusterSize = etcd.DefaultIdealClusterSize
	}

	if obj.idealClusterSize < 1 {
		return fmt.Errorf("the IdealClusterSize should be at least one")
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

	obj.exit = make(chan error)
	return nil
}

// Exit causes a safe shutdown. This is often attached to the ^C signal handler.
func (obj *Main) Exit(err error) {
	obj.exit <- err // trigger an exit!
}

// Run is the main execution entrypoint to run mgmt.
func (obj *Main) Run() error {

	hello(obj.Program, obj.Version, obj.Flags) // say hello!

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

	var prefix = fmt.Sprintf("/var/lib/%s/", obj.Program) // default prefix
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
			log.Println("Main: Warning: Working prefix directory is temporary!")

		} else {
			return fmt.Errorf("can't create prefix")
		}
	}
	log.Printf("Main: Working prefix is: %s", prefix)
	pgraphPrefix := fmt.Sprintf("%s/", path.Join(prefix, "pgraph")) // pgraph namespace
	if err := os.MkdirAll(pgraphPrefix, 0770); err != nil {
		return errwrap.Wrapf(err, "can't create pgraph prefix")
	}

	var prom *prometheus.Prometheus
	if obj.Prometheus {
		prom = &prometheus.Prometheus{
			Listen: obj.PrometheusListen,
		}
		if err := prom.Init(); err != nil {
			return errwrap.Wrapf(err, "can't create initiate Prometheus instance")
		}

		log.Printf("Main: Prometheus: Starting instance on %s", prom.Listen)
		if err := prom.Start(); err != nil {
			return errwrap.Wrapf(err, "can't start initiate Prometheus instance")
		}

		if err := prom.InitKindMetrics(resources.RegisteredResourcesNames()); err != nil {
			return errwrap.Wrapf(err, "can't initialize kind-specific prometheus metrics")
		}
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

	oldGraph := &pgraph.Graph{}
	graph := &resources.MGraph{}
	// pass in the information we need
	graph.Debug = obj.Flags.Debug
	graph.Init()

	// exit after `max-runtime` seconds for no reason at all...
	if i := obj.MaxRuntime; i > 0 {
		go func() {
			time.Sleep(time.Duration(i) * time.Second)
			obj.Exit(nil)
		}()
	}

	// setup converger
	converger := converger.NewConverger(
		obj.ConvergedTimeout,
		nil, // stateFn gets added in by EmbdEtcd
	)
	go converger.Loop(true) // main loop for converger, true to start paused
	// TODO: will this shut things down prematurely?
	converger.Start() // better start this anyways...

	// embedded etcd
	if len(obj.seeds) == 0 {
		log.Printf("Main: Seeds: No seeds specified!")
	} else {
		log.Printf("Main: Seeds(%d): %v", len(obj.seeds), obj.seeds)
	}
	EmbdEtcd := etcd.NewEmbdEtcd(
		hostname,
		obj.seeds,
		obj.clientURLs,
		obj.serverURLs,
		obj.advertiseClientURLs,
		obj.advertiseServerURLs,
		obj.NoServer,
		obj.idealClusterSize,
		etcd.Flags{
			Debug:   obj.Flags.Debug,
			Trace:   obj.Flags.Trace,
			Verbose: obj.Flags.Verbose,
		},
		prefix,
		converger,
	)
	if EmbdEtcd == nil {
		// TODO: verify EmbdEtcd is not nil below...
		obj.Exit(fmt.Errorf("Main: Etcd: Creation failed"))
	} else if err := EmbdEtcd.Startup(); err != nil { // startup (returns when etcd main loop is running)
		obj.Exit(fmt.Errorf("Main: Etcd: Startup failed: %v", err))
	}

	// wait for etcd server to be ready before continuing...
	// XXX: this is wrong if we're not going to be a server! we'll block!!!
	//	select {
	//	case <-EmbdEtcd.ServerReady():
	//		log.Printf("Main: Etcd: Server: Ready!")
	//		// pass
	//	case <-time.After(((etcd.MaxStartServerTimeout * etcd.MaxStartServerRetries) + 1) * time.Second):
	//		obj.Exit(fmt.Errorf("Main: Etcd: Startup timeout"))
	//	}

	time.Sleep(1 * time.Second) // XXX: temporary workaround

	convergerStateFn := func(b bool) error {
		// exit if we are using the converged timeout and we are the
		// root node. otherwise, if we are a child node in a remote
		// execution hierarchy, we should only notify our converged
		// state and wait for the parent to trigger the exit.
		if t := obj.ConvergedTimeout; t >= 0 {
			if b {
				log.Printf("Main: Converged for %d seconds, exiting!", t)
				obj.Exit(nil) // trigger an exit!
			}
			return nil
		}
		// send our individual state into etcd for others to see
		return etcd.SetHostnameConverged(EmbdEtcd, hostname, b) // TODO: what should happen on error?
	}
	if EmbdEtcd != nil {
		converger.SetStateFn(convergerStateFn)
	}

	// implementation of the World API (alternates can be substituted in)
	world := &etcd.World{
		Hostname:       hostname,
		EmbdEtcd:       EmbdEtcd,
		MetadataPrefix: MetadataPrefix,
		StoragePrefix:  StoragePrefix,
		StandaloneFs:   obj.DeployFs, // used for static deploys
		Debug:          obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("world: etcd: "+format, v...)
		},
	}

	graph.Data = &resources.ResData{
		Hostname:   hostname,
		Converger:  converger,
		Prometheus: prom,
		World:      world,
		Prefix:     pgraphPrefix,
		Debug:      obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("resources: "+format, v...)
		},
	}

	exitchan := make(chan struct{}) // exit on close
	wg := &sync.WaitGroup{}         // waitgroup for inner loop (go routine)

	deployChan := make(chan *gapi.Deploy)
	var gapiImpl gapi.GAPI // active GAPI implementation
	gapiImpl = nil         // starts off missing

	var gapiChan chan gapi.Next // stream events contain some instructions!
	gapiChan = nil              // starts off blocked
	wg.Add(1)
	go func() {
		defer wg.Done()
		first := true // first loop or not
		var mainDeploy *gapi.Deploy
		for {
			log.Println("Main: Waiting...")
			// The GAPI should always kick off an event on Next() at
			// startup when (and if) it indeed has a graph to share!
			fastPause := false
			select {
			case deploy, ok := <-deployChan:
				if !ok { // channel closed
					if obj.Flags.Debug {
						log.Printf("Main: Deploy: exited")
					}
					deployChan = nil // disable it

					if gapiImpl != nil { // currently running...
						gapiChan = nil
						if err := gapiImpl.Close(); err != nil {
							err = errwrap.Wrapf(err, "the GAPI closed poorly")
							log.Printf("Main: Deploy: GAPI: Final close failed: %+v", err)
						}
					}
					return // this is the only place we exit
				}
				if deploy == nil {
					log.Printf("Main: Deploy: Received empty deploy")
					continue
				}
				mainDeploy = deploy // save this one
				gapiObj := mainDeploy.GAPI
				if gapiObj == nil {
					log.Printf("Main: Deploy: Received empty GAPI")
					continue
				}

				if gapiImpl != nil { // currently running...
					gapiChan = nil
					if err := gapiImpl.Close(); err != nil {
						err = errwrap.Wrapf(err, "the GAPI closed poorly")
						log.Printf("Main: Deploy: GAPI: Close failed: %+v", err)
					}
				}
				gapiImpl = gapiObj // copy it to active

				data := gapi.Data{
					Program:  obj.Program,
					Hostname: hostname,
					World:    world,
					Noop:     mainDeploy.Noop,
					// FIXME: should the below flags come from the deploy struct?
					//NoWatch:  obj.NoWatch,
					NoConfigWatch: obj.NoConfigWatch,
					NoStreamWatch: obj.NoStreamWatch,
					Debug:         obj.Flags.Debug,
					Logf: func(format string, v ...interface{}) {
						log.Printf("gapi: "+format, v...)
					},
				}
				if obj.Flags.Debug {
					log.Printf("Main: GAPI: Init...")
				}
				if err := gapiImpl.Init(data); err != nil {
					log.Printf("Main: GAPI: Init failed: %+v", err)
					// TODO: consider running previous GAPI?
				} else {
					if obj.Flags.Debug {
						log.Printf("Main: GAPI: Next...")
					}
					// this must generate at least one event for it to work
					gapiChan = gapiImpl.Next() // stream of graph switch events!
				}
				continue

			case next, ok := <-gapiChan:
				if !ok { // channel closed
					if obj.Flags.Debug {
						log.Printf("Main: GAPI exited")
					}
					gapiChan = nil // disable it
					continue
				}

				// if we've been asked to exit...
				// TODO: do we want to block exits and wait?
				// TODO: we might want to wait for the next GAPI
				if next.Exit {
					obj.Exit(next.Err) // trigger exit
					continue           // wait for exitchan
				}

				// the gapi lets us send an error to the channel
				// this means there was a failure, but not fatal
				if err := next.Err; err != nil {
					log.Printf("Main: Error with graph stream: %v", err)
					continue // wait for another event
				}
				// everything else passes through to cause a compile!

				fastPause = next.Fast // should we pause fast?

				//case <-exitchan: // we only exit on deployChan close!
				//	return
			}

			if gapiImpl == nil { // TODO: can this ever happen anymore?
				log.Printf("Main: GAPI is empty!")
				continue
			}

			if first {
				converger.Pause() // it's already started!
			}
			// we need the vertices to be paused to work on them, so
			// run graph vertex LOCK...
			if !first { // TODO: we can flatten this check out I think
				converger.Pause()      // FIXME: add sync wait?
				graph.Pause(fastPause) // sync

				//graph.UnGroup() // FIXME: implement me if needed!
			}

			// make the graph from yaml, lib, puppet->yaml, or dsl!
			newGraph, err := gapiImpl.Graph() // generate graph!
			if err != nil {
				log.Printf("Main: Error creating new graph: %v", err)
				// unpause!
				if !first {
					graph.Start(first) // sync
					converger.Start()  // after Start()
				}
				continue
			}
			if obj.Flags.Debug {
				log.Printf("Main: New Graph: %v", newGraph)
			}

			// this edits the paused vertices, but it is safe to do
			// so even if we don't use this new graph, since those
			// value should be the same for existing vertices...
			for _, v := range newGraph.Vertices() {
				m := resources.VtoR(v).Meta()
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

			// We don't have to "UnGroup()" to compare, since we
			// save the old graph to use when we compare.
			// TODO: Does this hurt performance or graph changes ?
			log.Printf("Main: GraphSync...")
			vertexCmpFn := func(v1, v2 pgraph.Vertex) (bool, error) {
				return resources.VtoR(v1).Compare(resources.VtoR(v2)), nil
			}
			vertexAddFn := func(v pgraph.Vertex) error {
				err := resources.VtoR(v).Validate()
				return errwrap.Wrapf(err, "could not Validate() resource")
			}
			vertexRemoveFn := func(v pgraph.Vertex) error {
				// wait for exit before starting new graph!
				resources.VtoR(v).Exit() // sync
				return nil
			}
			edgeCmpFn := func(e1, e2 pgraph.Edge) (bool, error) {
				edge1 := e1.(*resources.Edge) // panic if wrong
				edge2 := e2.(*resources.Edge) // panic if wrong
				return edge1.Compare(edge2), nil
			}
			// on success, this updates the receiver graph...
			if err := oldGraph.GraphSync(newGraph, vertexCmpFn, vertexAddFn, vertexRemoveFn, edgeCmpFn); err != nil {
				log.Printf("Main: Error running graph sync: %v", err)
				// unpause!
				if !first {
					graph.Start(first) // sync
					converger.Start()  // after Start()
				}
				continue
			}

			//savedGraph := oldGraph.Copy() // save a copy for errors

			// TODO: should we call each Res.Setup() here instead?

			// add autoedges; modifies the graph only if no error
			if err := resources.AutoEdges(oldGraph); err != nil {
				log.Printf("Main: Error running auto edges: %v", err)
				// unpause!
				if !first {
					graph.Start(first) // sync
					converger.Start()  // after Start()
				}
				continue
			}

			// at this point, any time we error after a destructive
			// modification of the graph we need to restore the old
			// graph that was previously running, eg:
			//
			//	oldGraph = savedGraph.Copy()
			//
			// which we are (luckily) able to avoid testing for now

			resources.AutoGroup(oldGraph, &resources.NonReachabilityGrouper{}) // run autogroup; modifies the graph
			// TODO: do we want to do a transitive reduction?
			// FIXME: run a type checker that verifies all the send->recv relationships

			graph.Update(oldGraph) // copy in structure of new graph

			// Call this here because at this point the graph does
			// not know anything about the prometheus instance.
			if err := prom.UpdatePgraphStartTime(); err != nil {
				log.Printf("Main: Prometheus.UpdatePgraphStartTime() errored: %v", err)
			}
			// Start() needs to be synchronous or wait,
			// because if half of the nodes are started and
			// some are not ready yet and the EtcdWatch
			// loops, we'll cause Pause() before we
			// even got going, thus causing nil pointer errors
			graph.Start(first) // sync
			converger.Start()  // after Start()

			log.Printf("Main: Graph: %v", graph) // show graph
			if obj.Graphviz != "" {
				filter := obj.GraphvizFilter
				if filter == "" {
					filter = "dot" // directed graph default
				}
				if err := graph.ExecGraphviz(filter, obj.Graphviz, hostname); err != nil {
					log.Printf("Main: Graphviz: %v", err)
				} else {
					log.Printf("Main: Graphviz: Successfully generated graph!")
				}
			}
			first = false
		}
	}()

	if obj.Deploy != nil {
		deploy := obj.Deploy
		// redundant
		deploy.Noop = obj.Noop
		deploy.Sema = obj.Sema

		select {
		case deployChan <- deploy:
			// send
		case <-exitchan:
			// pass
		}

		// don't inline this, because when we close the deployChan it's
		// the signal to tell the engine to actually shutdown...
		go func() {
			defer close(deployChan) // no more are coming ever!
			select {                // wait until we're ready to shutdown
			case <-exitchan:
				return
			}
		}()
	} else {
		// etcd based deploy
		go func() {
			defer close(deployChan)
			startChan := make(chan struct{}) // start signal
			close(startChan)                 // kick it off!
			for {
				select {
				case <-startChan: // kick the loop once at start
					startChan = nil // disable

				case err, ok := <-etcd.WatchDeploy(EmbdEtcd):
					if !ok {
						obj.Exit(nil) // regular shutdown
						return
					}
					if err != nil {
						// TODO: it broke, can we restart?
						obj.Exit(fmt.Errorf("Main: Deploy: Watch error"))
						return
					}

				case <-exitchan:
					return
				}

				if obj.Flags.Debug {
					log.Printf("Main: Deploy: Got activity")
				}
				str, err := etcd.GetDeploy(EmbdEtcd, 0) // 0 means get the latest one
				if err != nil {
					log.Printf("Main: Deploy: Error getting deploy %+v", err)
					continue
				}
				if str == "" { // no available deploys exist yet
					continue
				}

				// decode the deploy (incl. GAPI) and send it!
				deploy, err := gapi.NewDeployFromB64(str)
				if err != nil {
					log.Printf("Main: Deploy: Error decoding deploy %+v", err)
					continue
				}

				select {
				case deployChan <- deploy:
					// send
					if obj.Flags.Debug {
						log.Printf("Main: Deploy: Sending new GAPI")
					}

				case <-exitchan:
					return
				}
			}
		}()
	}

	log.Println("Main: Running...")

	reterr := <-obj.exit // wait for exit signal

	log.Println("Main: Destroy...")

	// tell inner main loop to exit
	close(exitchan)
	wg.Wait()

	graph.Exit() // tells all the children to exit, and waits for them to do so

	// cleanup etcd main loop last so it can process everything first
	if err := EmbdEtcd.Destroy(); err != nil { // shutdown and cleanup etcd
		err = errwrap.Wrapf(err, "embedded Etcd exited poorly")
		reterr = multierr.Append(reterr, err) // list of errors
	}

	if obj.Prometheus {
		log.Printf("Main: Prometheus: Stopping instance")
		if err := prom.Stop(); err != nil {
			err = errwrap.Wrapf(err, "the Prometheus instance exited poorly")
			reterr = multierr.Append(reterr, err)
		}
	}

	if obj.Flags.Debug {
		log.Printf("Main: Graph: %v", graph)
	}
	if reterr != nil {
		log.Printf("Main: Error: %v", reterr)
	}
	log.Println("Goodbye!")
	return reterr
}
