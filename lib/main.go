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

package lib

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgp"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/prometheus"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/remote"
	"github.com/purpleidea/mgmt/resources"
	"github.com/purpleidea/mgmt/util"

	etcdtypes "github.com/coreos/etcd/pkg/types"
	"github.com/coreos/pkg/capnslog"
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

	GAPI    gapi.GAPI // graph API interface struct
	Remotes []string  // list of remote graph definitions to run

	NoWatch          bool   // do not update graph on watched graph definition file changes
	Noop             bool   // globally force all resources into no-op mode
	Graphviz         string // output file for graphviz data
	GraphvizFilter   string // graphviz filter to use
	ConvergedTimeout int    // exit after approximately this many seconds in a converged state; -1 to disable
	MaxRuntime       uint   // exit after a maximum of approximately this many seconds

	Seeds            []string // default etc client endpoint
	ClientURLs       []string // list of URLs to listen on for client traffic
	ServerURLs       []string // list of URLs to listen on for server (peer) traffic
	IdealClusterSize int      // ideal number of server peers in cluster; only read by initial server
	NoServer         bool     // do not let other servers peer with me

	CConns           uint16 // number of maximum concurrent remote ssh connections to run, 0 for unlimited
	AllowInteractive bool   // allow interactive prompting, such as for remote passwords
	SSHPrivIDRsa     string // default path to ssh key file, set empty to never touch
	NoCaching        bool   // don't allow remote caching of remote execution binary
	Depth            uint16 // depth in remote hierarchy; for internal use only

	seeds            etcdtypes.URLs // processed seeds value
	clientURLs       etcdtypes.URLs // processed client urls value
	serverURLs       etcdtypes.URLs // processed server urls value
	idealClusterSize uint16         // processed ideal cluster size value

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
		return fmt.Errorf("You must set the Program and Version strings!")
	}

	if obj.Prefix != nil && obj.TmpPrefix {
		return fmt.Errorf("Choosing a prefix and the request for a tmp prefix is illogical!")
	}

	obj.idealClusterSize = uint16(obj.IdealClusterSize)
	if obj.IdealClusterSize < 0 { // value is undefined, set to the default
		obj.idealClusterSize = etcd.DefaultIdealClusterSize
	}

	if obj.idealClusterSize < 1 {
		return fmt.Errorf("IdealClusterSize should be at least one!")
	}

	if obj.NoServer && len(obj.Remotes) > 0 {
		// TODO: in this case, we won't be able to tunnel stuff back to
		// here, so if we're okay with every remote graph running in an
		// isolated mode, then this is okay. Improve on this if there's
		// someone who really wants to be able to do this.
		return fmt.Errorf("The Server is required when using Remotes!")
	}

	if obj.CConns < 0 {
		return fmt.Errorf("The CConns value should be at least zero!")
	}

	if obj.ConvergedTimeout >= 0 && obj.CConns > 0 && len(obj.Remotes) > int(obj.CConns) {
		return fmt.Errorf("You can't converge if you have more remotes than available connections!")
	}

	if obj.Depth < 0 { // user should not be using this argument manually
		return fmt.Errorf("Negative values for Depth are not permitted!")
	}

	// transform the url list inputs into etcd typed lists
	var err error
	obj.seeds, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.Seeds, []string{",", ";", " "}),
	)
	if err != nil && len(obj.Seeds) > 0 {
		return fmt.Errorf("Seeds didn't parse correctly!")
	}
	obj.clientURLs, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.ClientURLs, []string{",", ";", " "}),
	)
	if err != nil && len(obj.ClientURLs) > 0 {
		return fmt.Errorf("ClientURLs didn't parse correctly!")
	}
	obj.serverURLs, err = etcdtypes.NewURLs(
		util.FlattenListWithSplit(obj.ServerURLs, []string{",", ";", " "}),
	)
	if err != nil && len(obj.ServerURLs) > 0 {
		return fmt.Errorf("ServerURLs didn't parse correctly!")
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

	var start = time.Now().UnixNano()

	var flags int
	if obj.Flags.Debug || true { // TODO: remove || true
		flags = log.LstdFlags | log.Lshortfile
	}
	flags = (flags - log.Ldate) // remove the date for now
	log.SetFlags(flags)

	// un-hijack from capnslog...
	log.SetOutput(os.Stderr)
	if obj.Flags.Verbose {
		capnslog.SetFormatter(capnslog.NewLogFormatter(os.Stderr, "(etcd) ", flags))
	} else {
		capnslog.SetFormatter(capnslog.NewNilFormatter())
	}

	log.Printf("This is: %s, version: %s", obj.Program, obj.Version)
	log.Printf("Main: Start: %v", start)

	hostname, err := os.Hostname() // a sensible default
	// allow passing in the hostname, instead of using the system setting
	if h := obj.Hostname; h != nil && *h != "" { // override by cli
		hostname = *h
	} else if err != nil {
		return errwrap.Wrapf(err, "Can't get default hostname!")
	}
	if hostname == "" { // safety check
		return fmt.Errorf("Hostname cannot be empty!")
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
				return fmt.Errorf("Main: Error: Can't create temporary prefix!")
			}
			log.Println("Main: Warning: Working prefix directory is temporary!")

		} else {
			return fmt.Errorf("Main: Error: Can't create prefix!")
		}
	}
	log.Printf("Main: Working prefix is: %s", prefix)
	pgraphPrefix := fmt.Sprintf("%s/", path.Join(prefix, "pgraph")) // pgraph namespace
	if err := os.MkdirAll(pgraphPrefix, 0770); err != nil {
		return errwrap.Wrapf(err, "Can't create pgraph prefix")
	}

	var prom *prometheus.Prometheus
	if obj.Prometheus {
		prom = &prometheus.Prometheus{
			Listen: obj.PrometheusListen,
		}
		if err := prom.Init(); err != nil {
			return errwrap.Wrapf(err, "Can't create initiate Prometheus instance")
		}

		log.Printf("Main: Prometheus: Starting instance on %s", prom.Listen)
		if err := prom.Start(); err != nil {
			return errwrap.Wrapf(err, "Can't start initiate Prometheus instance")
		}
	}

	if !obj.NoPgp {
		pgpPrefix := fmt.Sprintf("%s/", path.Join(prefix, "pgp"))
		if err := os.MkdirAll(pgpPrefix, 0770); err != nil {
			return errwrap.Wrapf(err, "Can't create pgp prefix")
		}

		pgpKeyringPath := path.Join(pgpPrefix, pgp.DefaultKeyringFile) // default path

		if p := obj.PgpKeyPath; p != nil {
			pgpKeyringPath = *p
		}

		var err error
		if obj.pgpKeys, err = pgp.Import(pgpKeyringPath); err != nil && !os.IsNotExist(err) {
			return errwrap.Wrapf(err, "Can't import pgp key")
		}

		if obj.pgpKeys == nil {

			identity := fmt.Sprintf("%s <%s> %s", obj.Program, "root@"+hostname, "generated by "+obj.Program)
			if p := obj.PgpIdentity; p != nil {
				identity = *p
			}

			name, comment, email, err := pgp.ParseIdentity(identity)
			if err != nil {
				return errwrap.Wrapf(err, "Can't parse user string")

			}

			// TODO: Make hash configurable
			if obj.pgpKeys, err = pgp.Generate(name, comment, email, nil); err != nil {
				return errwrap.Wrapf(err, "Can't creating pgp key")
			}

			if err := obj.pgpKeys.SaveKey(pgpKeyringPath); err != nil {
				return errwrap.Wrapf(err, "Can't save pgp key")
			}
		}

		// TODO: Import admin key
	}

	var G, oldGraph *pgraph.Graph

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
		obj.Exit(fmt.Errorf("Main: Etcd: Creation failed!"))
	} else if err := EmbdEtcd.Startup(); err != nil { // startup (returns when etcd main loop is running)
		obj.Exit(fmt.Errorf("Main: Etcd: Startup failed: %v", err))
	}
	convergerStateFn := func(b bool) error {
		// exit if we are using the converged timeout and we are the
		// root node. otherwise, if we are a child node in a remote
		// execution hierarchy, we should only notify our converged
		// state and wait for the parent to trigger the exit.
		if t := obj.ConvergedTimeout; obj.Depth == 0 && t >= 0 {
			if b {
				log.Printf("Converged for %d seconds, exiting!", t)
				obj.Exit(nil) // trigger an exit!
			}
			return nil
		}
		// send our individual state into etcd for others to see
		return etcd.EtcdSetHostnameConverged(EmbdEtcd, hostname, b) // TODO: what should happen on error?
	}
	if EmbdEtcd != nil {
		converger.SetStateFn(convergerStateFn)
	}

	var gapiChan chan error // stream events are nil errors
	if obj.GAPI != nil {
		data := gapi.Data{
			Hostname: hostname,
			// NOTE: alternate implementations can be substituted in
			World: &etcd.World{
				Hostname: hostname,
				EmbdEtcd: EmbdEtcd,
			},
			Noop:    obj.Noop,
			NoWatch: obj.NoWatch,
		}
		if err := obj.GAPI.Init(data); err != nil {
			obj.Exit(fmt.Errorf("Main: GAPI: Init failed: %v", err))
		} else if !obj.NoWatch {
			gapiChan = obj.GAPI.Next() // stream of graph switch events!
		}
	}

	exitchan := make(chan struct{}) // exit on close
	go func() {
		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		log.Println("Etcd: Starting...")
		etcdChan := etcd.EtcdWatch(EmbdEtcd)
		first := true // first loop or not
		for {
			log.Println("Main: Waiting...")
			select {
			case <-startChan: // kick the loop once at start
				startChan = nil // disable
				// pass

			case b := <-etcdChan:
				if !b { // ignore the message
					continue
				}
				// everything else passes through to cause a compile!

			case err, ok := <-gapiChan:
				if !ok { // channel closed
					if obj.Flags.Debug {
						log.Printf("Main: GAPI exited")
					}
					gapiChan = nil // disable it
					continue
				}
				if err != nil {
					obj.Exit(err) // trigger exit
					continue
					//return // TODO: return or wait for exitchan?
				}
				if obj.NoWatch { // extra safety for bad GAPI's
					log.Printf("Main: GAPI stream should be quiet with NoWatch!") // fix the GAPI!
					continue                                                      // no stream events should be sent
				}

			case <-exitchan:
				return
			}

			if obj.GAPI == nil {
				log.Printf("Config: GAPI is empty!")
				continue
			}

			// we need the vertices to be paused to work on them, so
			// run graph vertex LOCK...
			if !first { // TODO: we can flatten this check out I think
				converger.Pause() // FIXME: add sync wait?
				G.Pause()         // sync

				//G.UnGroup() // FIXME: implement me if needed!
			}

			// make the graph from yaml, lib, puppet->yaml, or dsl!
			newGraph, err := obj.GAPI.Graph() // generate graph!
			if err != nil {
				log.Printf("Config: Error creating new graph: %v", err)
				// unpause!
				if !first {
					G.Start(first)    // sync
					converger.Start() // after G.Start()
				}
				continue
			}
			newGraph.Flags = pgraph.Flags{Debug: obj.Flags.Debug}
			// pass in the information we need
			newGraph.AssociateData(&resources.Data{
				Converger: converger,
				Prefix:    pgraphPrefix,
				Debug:     obj.Flags.Debug,
			})

			// apply the global noop parameter if requested
			if obj.Noop {
				for _, m := range newGraph.GraphMetas() {
					m.Noop = obj.Noop
				}
			}

			// FIXME: make sure we "UnGroup()" any semi-destructive
			// changes to the resources so our efficient GraphSync
			// will be able to re-use and cmp to the old graph.
			newFullGraph, err := newGraph.GraphSync(oldGraph)
			if err != nil {
				log.Printf("Config: Error running graph sync: %v", err)
				// unpause!
				if !first {
					G.Start(first)    // sync
					converger.Start() // after G.Start()
				}
				continue
			}
			oldGraph = newFullGraph // save old graph
			G = oldGraph.Copy()     // copy to active graph

			G.AutoEdges() // add autoedges; modifies the graph
			G.AutoGroup() // run autogroup; modifies the graph
			// TODO: do we want to do a transitive reduction?
			// FIXME: run a type checker that verifies all the send->recv relationships

			log.Printf("Graph: %v", G) // show graph
			if obj.GraphvizFilter != "" {
				if err := G.ExecGraphviz(obj.GraphvizFilter, obj.Graphviz); err != nil {
					log.Printf("Graphviz: %v", err)
				} else {
					log.Printf("Graphviz: Successfully generated graph!")
				}
			}
			// G.Start(...) needs to be synchronous or wait,
			// because if half of the nodes are started and
			// some are not ready yet and the EtcdWatch
			// loops, we'll cause G.Pause(...) before we
			// even got going, thus causing nil pointer errors
			G.Start(first)    // sync
			converger.Start() // after G.Start()
			first = false
		}
	}()

	configWatcher := recwatch.NewConfigWatcher()
	configWatcher.Flags = recwatch.Flags{Debug: obj.Flags.Debug}
	events := configWatcher.Events()
	if !obj.NoWatch {
		configWatcher.Add(obj.Remotes...) // add all the files...
	} else {
		events = nil // signal that no-watch is true
	}
	go func() {
		select {
		case err := <-configWatcher.Error():
			obj.Exit(err) // trigger an exit!

		case <-exitchan:
			return
		}
	}()

	// initialize the add watcher, which calls the f callback on map changes
	convergerCb := func(f func(map[string]bool) error) (func(), error) {
		return etcd.EtcdAddHostnameConvergedWatcher(EmbdEtcd, f)
	}

	// build remotes struct for remote ssh
	remotes := remote.NewRemotes(
		EmbdEtcd.LocalhostClientURLs().StringSlice(),
		[]string{etcd.DefaultClientURL},
		obj.Noop,
		obj.Remotes, // list of files
		events,      // watch for file changes
		obj.CConns,
		obj.AllowInteractive,
		obj.SSHPrivIDRsa,
		!obj.NoCaching,
		obj.Depth,
		prefix,
		converger,
		convergerCb,
		remote.Flags{
			Program: obj.Program,
			Debug:   obj.Flags.Debug,
		},
	)

	// TODO: is there any benefit to running the remotes above in the loop?
	// wait for etcd to be running before we remote in, which we do above!
	go remotes.Run()

	if obj.GAPI == nil {
		converger.Start() // better start this for empty graphs
	}
	log.Println("Main: Running...")

	reterr := <-obj.exit // wait for exit signal

	log.Println("Destroy...")

	if obj.GAPI != nil {
		if err := obj.GAPI.Close(); err != nil {
			err = errwrap.Wrapf(err, "GAPI closed poorly!")
			reterr = multierr.Append(reterr, err) // list of errors
		}
	}

	configWatcher.Close()                  // stop sending file changes to remotes
	if err := remotes.Exit(); err != nil { // tell all the remote connections to shutdown; waits!
		err = errwrap.Wrapf(err, "Remote exited poorly!")
		reterr = multierr.Append(reterr, err) // list of errors
	}

	// tell inner main loop to exit
	close(exitchan)

	G.Exit() // tells all the children to exit, and waits for them to do so

	// cleanup etcd main loop last so it can process everything first
	if err := EmbdEtcd.Destroy(); err != nil { // shutdown and cleanup etcd
		err = errwrap.Wrapf(err, "Etcd exited poorly!")
		reterr = multierr.Append(reterr, err) // list of errors
	}

	if obj.Prometheus {
		log.Printf("Main: Prometheus: Stopping instance")
		if err := prom.Stop(); err != nil {
			err = errwrap.Wrapf(err, "Prometheus instance exited poorly!")
			reterr = multierr.Append(reterr, err)
		}
	}

	if obj.Flags.Debug {
		log.Printf("Main: Graph: %v", G)
	}

	// TODO: wait for each vertex to exit...
	log.Println("Goodbye!")
	return reterr
}
