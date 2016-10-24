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

package mgmtmain

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/gconfig"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/puppet"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/remote"
	"github.com/purpleidea/mgmt/util"

	etcdtypes "github.com/coreos/etcd/pkg/types"
	"github.com/coreos/pkg/capnslog"
)

// Main is the main struct for running the mgmt logic.
type Main struct {
	Program string // the name of this program, usually set at compile time
	Version string // the version of this program, usually set at compile time

	Prefix         *string // prefix passed in; nil if undefined
	TmpPrefix      bool    // request a pseudo-random, temporary prefix to be used
	AllowTmpPrefix bool    // allow creation of a new temporary prefix if main prefix is unavailable

	Hostname *string // hostname to use; nil if undefined

	File       *string                     // graph file to run; nil if undefined
	Puppet     *string                     // puppet mode to run; nil if undefined
	PuppetConf string                      // the path to an alternate puppet.conf file
	GAPI       func() *gconfig.GraphConfig // graph API; nil if undefined
	Remotes    []string                    // list of remote graph definitions to run

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

	DEBUG   bool
	VERBOSE bool

	seeds            etcdtypes.URLs // processed seeds value
	clientURLs       etcdtypes.URLs // processed client urls value
	serverURLs       etcdtypes.URLs // processed server urls value
	idealClusterSize uint16         // processed ideal cluster size value

	exit       chan error                       // exit signal
	switchChan chan func() *gconfig.GraphConfig // graph switches
}

// Init initializes the main struct after it performs some validation.
func (obj *Main) Init() error {

	if obj.Program == "" || obj.Version == "" {
		return fmt.Errorf("You must set the Program and Version strings!")
	}

	if obj.Prefix != nil && obj.TmpPrefix {
		return fmt.Errorf("Choosing a prefix and the request for a tmp prefix is illogical!")
	}

	if obj.File != nil && obj.Puppet != nil {
		return fmt.Errorf("The File and Puppet parameters cannot be used together!")
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
	obj.switchChan = make(chan func() *gconfig.GraphConfig)
	return nil
}

// Exit causes a safe shutdown. This is often attached to the ^C signal handler.
func (obj *Main) Exit(err error) {
	obj.exit <- err // trigger an exit!
}

// Switch causes mgmt try to switch the currently running graph to a new one.
// The function passed in will usually be called immediately, but it can also
// happen after a delay, and more often than this Switch function is called!
func (obj *Main) Switch(f func() *gconfig.GraphConfig) {
	obj.switchChan <- f
	// TODO: should we get an ACK() and pass back a return value ?
}

// Run is the main execution entrypoint to run mgmt.
func (obj *Main) Run() error {

	var start = time.Now().UnixNano()

	var flags int
	if obj.DEBUG || true { // TODO: remove || true
		flags = log.LstdFlags | log.Lshortfile
	}
	flags = (flags - log.Ldate) // remove the date for now
	log.SetFlags(flags)

	// un-hijack from capnslog...
	log.SetOutput(os.Stderr)
	if obj.VERBOSE {
		capnslog.SetFormatter(capnslog.NewLogFormatter(os.Stderr, "(etcd) ", flags))
	} else {
		capnslog.SetFormatter(capnslog.NewNilFormatter())
	}

	log.Printf("This is: %s, version: %s", obj.Program, obj.Version)
	log.Printf("Main: Start: %v", start)

	var hostname, _ = os.Hostname()
	// allow passing in the hostname, instead of using --hostname
	if obj.File != nil {
		if config := gconfig.ParseConfigFromFile(*obj.File); config != nil {
			if h := config.Hostname; h != "" {
				hostname = h
			}
		}
	}
	if obj.Hostname != nil { // override by cli
		if h := obj.Hostname; *h != "" {
			hostname = *h
		}
	}

	var prefix = fmt.Sprintf("/var/lib/%s/", obj.Program) // default prefix
	if p := obj.Prefix; p != nil {
		prefix = *p
	}
	// make sure the working directory prefix exists
	if obj.TmpPrefix || os.MkdirAll(prefix, 0770) != nil {
		if obj.TmpPrefix || obj.AllowTmpPrefix {
			var err error
			if prefix, err = ioutil.TempDir("", obj.Program+"-"); err != nil {
				return fmt.Errorf("Main: Error: Can't create temporary prefix!")
			}
			log.Println("Main: Warning: Working prefix directory is temporary!")

		} else {
			return fmt.Errorf("Main: Error: Can't create prefix!")
		}
	}
	log.Printf("Main: Working prefix is: %s", prefix)

	var wg sync.WaitGroup
	var G, fullGraph *pgraph.Graph

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

	exitchan := make(chan struct{}) // exit on close
	go func() {
		startchan := make(chan struct{}) // start signal
		go func() { startchan <- struct{}{} }()
		var configChan chan error
		var puppetChan <-chan time.Time
		var customFunc = obj.GAPI // default
		if !obj.NoWatch && obj.File != nil {
			configChan = recwatch.ConfigWatch(*obj.File)
		} else if obj.Puppet != nil {
			interval := puppet.PuppetInterval(obj.PuppetConf)
			puppetChan = time.Tick(time.Duration(interval) * time.Second)
		}
		log.Println("Etcd: Starting...")
		etcdchan := etcd.EtcdWatch(EmbdEtcd)
		first := true // first loop or not
		for {
			log.Println("Main: Waiting...")
			select {
			case <-startchan: // kick the loop once at start
				// pass

			case b := <-etcdchan:
				if !b { // ignore the message
					continue
				}
				// everything else passes through to cause a compile!

			case customFunc = <-obj.switchChan:
				// handle a graph switch with a new custom function
				obj.GAPI = customFunc

			case <-puppetChan:
				// nothing, just go on

			case e := <-configChan:
				if obj.NoWatch {
					continue // not ready to read config
				}
				if e != nil {
					obj.Exit(e) // trigger exit
					continue
					//return // TODO: return or wait for exitchan?
				}
			// XXX: case compile_event: ...
			// ...
			case <-exitchan:
				return
			}

			var config *gconfig.GraphConfig
			if obj.File != nil {
				config = gconfig.ParseConfigFromFile(*obj.File)
			} else if obj.Puppet != nil {
				config = puppet.ParseConfigFromPuppet(*obj.Puppet, obj.PuppetConf)
			} else if obj.GAPI != nil {
				config = obj.GAPI()
			}

			if config == nil {
				log.Printf("Config: Parse failure")
				continue
			}

			if config.Hostname != "" && config.Hostname != hostname {
				log.Printf("Config: Hostname changed, ignoring config!")
				continue
			}
			config.Hostname = hostname // set it in case it was ""

			// run graph vertex LOCK...
			if !first { // TODO: we can flatten this check out I think
				converger.Pause() // FIXME: add sync wait?
				G.Pause()         // sync
			}

			// build graph from config struct on events, eg: etcd...
			// we need the vertices to be paused to work on them
			if newFullgraph, err := config.NewGraphFromConfig(fullGraph, EmbdEtcd, obj.Noop); err == nil { // keep references to all original elements
				fullGraph = newFullgraph
			} else {
				log.Printf("Config: Error making new graph from config: %v", err)
				// unpause!
				if !first {
					G.Start(&wg, first) // sync
					converger.Start()   // after G.Start()
				}
				continue
			}

			G = fullGraph.Copy() // copy to active graph
			// XXX: do etcd transaction out here...
			G.AutoEdges() // add autoedges; modifies the graph
			G.AutoGroup() // run autogroup; modifies the graph
			// TODO: do we want to do a transitive reduction?

			log.Printf("Graph: %v", G) // show graph
			err := G.ExecGraphviz(obj.GraphvizFilter, obj.Graphviz)
			if err != nil {
				log.Printf("Graphviz: %v", err)
			} else {
				log.Printf("Graphviz: Successfully generated graph!")
			}
			G.AssociateData(converger)
			// G.Start(...) needs to be synchronous or wait,
			// because if half of the nodes are started and
			// some are not ready yet and the EtcdWatch
			// loops, we'll cause G.Pause(...) before we
			// even got going, thus causing nil pointer errors
			G.Start(&wg, first) // sync
			converger.Start()   // after G.Start()
			first = false
		}
	}()

	configWatcher := recwatch.NewConfigWatcher()
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
		obj.Program,
	)

	// TODO: is there any benefit to running the remotes above in the loop?
	// wait for etcd to be running before we remote in, which we do above!
	go remotes.Run()

	if obj.File == nil && obj.Puppet == nil && obj.GAPI == nil {
		converger.Start() // better start this for empty graphs
	}
	log.Println("Main: Running...")

	err := <-obj.exit // wait for exit signal

	log.Println("Destroy...")

	configWatcher.Close() // stop sending file changes to remotes
	remotes.Exit()        // tell all the remote connections to shutdown; waits!

	G.Exit() // tell all the children to exit

	// tell inner main loop to exit
	close(exitchan)

	// cleanup etcd main loop last so it can process everything first
	if err := EmbdEtcd.Destroy(); err != nil { // shutdown and cleanup etcd
		log.Printf("Etcd exited poorly with: %v", err)
	}

	if obj.DEBUG {
		log.Printf("Graph: %v", G)
	}

	wg.Wait() // wait for primary go routines to exit

	// TODO: wait for each vertex to exit...
	log.Println("Goodbye!")
	return err
}
