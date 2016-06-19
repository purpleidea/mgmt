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

package main

import (
	"github.com/urfave/cli"
	etcdtypes "github.com/coreos/etcd/pkg/types"
	"github.com/coreos/pkg/capnslog"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// set at compile time
var (
	program string
	version string
)

const (
	DEBUG   = false // add additional log messages
	TRACE   = false // add execution flow log messages
	VERBOSE = false // add extra log message output
)

// signal handler
func waitForSignal(exit chan bool) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt) // catch ^C
	//signal.Notify(signals, os.Kill) // catch signals
	signal.Notify(signals, syscall.SIGTERM)

	select {
	case e := <-signals: // any signal will do
		if e == os.Interrupt {
			log.Println("Interrupted by ^C")
		} else {
			log.Println("Interrupted by signal")
		}
	case <-exit: // or a manual signal
		log.Println("Interrupted by exit signal")
	}
}

func run(c *cli.Context) error {
	var start = time.Now().UnixNano()
	log.Printf("This is: %v, version: %v", program, version)
	log.Printf("Main: Start: %v", start)

	hostname := c.String("hostname")
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	noop := c.Bool("noop")

	seeds, err := etcdtypes.NewURLs(
		FlattenListWithSplit(c.StringSlice("seeds"), []string{",", ";", " "}),
	)
	if err != nil && len(c.StringSlice("seeds")) > 0 {
		log.Printf("Main: Error: seeds didn't parse correctly!")
		return cli.NewExitError("", 1)
	}
	clientURLs, err := etcdtypes.NewURLs(
		FlattenListWithSplit(c.StringSlice("client-urls"), []string{",", ";", " "}),
	)
	if err != nil && len(c.StringSlice("client-urls")) > 0 {
		log.Printf("Main: Error: clientURLs didn't parse correctly!")
		return cli.NewExitError("", 1)
	}
	serverURLs, err := etcdtypes.NewURLs(
		FlattenListWithSplit(c.StringSlice("server-urls"), []string{",", ";", " "}),
	)
	if err != nil && len(c.StringSlice("server-urls")) > 0 {
		log.Printf("Main: Error: serverURLs didn't parse correctly!")
		return cli.NewExitError("", 1)
	}

	idealClusterSize := uint16(c.Int("ideal-cluster-size"))
	if idealClusterSize < 1 {
		log.Printf("Main: Error: idealClusterSize should be at least one!")
		return cli.NewExitError("", 1)
	}

	if c.IsSet("file") && c.IsSet("puppet") {
		log.Println("Main: Error: the --file and --puppet parameters cannot be used together!")
		return cli.NewExitError("", 1)
	}

	var wg sync.WaitGroup
	exit := make(chan bool) // exit signal
	var G, fullGraph *Graph

	// exit after `max-runtime` seconds for no reason at all...
	if i := c.Int("max-runtime"); i > 0 {
		go func() {
			time.Sleep(time.Duration(i) * time.Second)
			exit <- true
		}()
	}

	// setup converger
	converger := NewConverger(
		c.Int("converged-timeout"),
		func() { // lambda to run when converged
			log.Printf("Converged for %d seconds, exiting!", c.Int("converged-timeout"))
			exit <- true // trigger an exit!
		},
	)
	go converger.Loop(true) // main loop for converger, true to start paused

	// embedded etcd
	if len(seeds) == 0 {
		log.Printf("Main: Seeds: No seeds specified!")
	} else {
		log.Printf("Main: Seeds(%v): %v", len(seeds), seeds)
	}
	EmbdEtcd := NewEmbdEtcd(
		hostname,
		seeds,
		clientURLs,
		serverURLs,
		c.Bool("no-server"),
		idealClusterSize,
		converger,
	)
	if err := EmbdEtcd.Startup(); err != nil { // startup (returns when etcd main loop is running)
		log.Printf("Main: Etcd: Startup failed: %v", err)
		exit <- true
	}

	exitchan := make(chan Event) // exit event
	go func() {
		startchan := make(chan struct{}) // start signal
		go func() { startchan <- struct{}{} }()
		file := c.String("file")
		var configchan chan bool
		var puppetchan <-chan time.Time
		if !c.Bool("no-watch") && c.IsSet("file") {
			configchan = ConfigWatch(file)
		} else if c.IsSet("puppet") {
			interval := PuppetInterval(c.String("puppet-conf"))
			puppetchan = time.Tick(time.Duration(interval) * time.Second)
		}
		log.Println("Etcd: Starting...")
		etcdchan := EtcdWatch(EmbdEtcd)
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

			case <-puppetchan:
				// nothing, just go on

			case msg := <-configchan:
				if c.Bool("no-watch") || !msg {
					continue // not ready to read config
				}
			// XXX: case compile_event: ...
			// ...
			case msg := <-exitchan:
				msg.ACK()
				return
			}

			var config *GraphConfig
			if c.IsSet("file") {
				config = ParseConfigFromFile(file)
			} else if c.IsSet("puppet") {
				config = ParseConfigFromPuppet(c.String("puppet"), c.String("puppet-conf"))
			}
			if config == nil {
				log.Printf("Config parse failure")
				continue
			}

			// run graph vertex LOCK...
			if !first { // TODO: we can flatten this check out I think
				converger.Pause() // FIXME: add sync wait?
				G.Pause()         // sync
			}

			// build graph from yaml file on events (eg: from etcd)
			// we need the vertices to be paused to work on them
			if newFullgraph, err := fullGraph.NewGraphFromConfig(config, EmbdEtcd, hostname, noop); err == nil { // keep references to all original elements
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
			err := G.ExecGraphviz(c.String("graphviz-filter"), c.String("graphviz"))
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

	log.Println("Main: Running...")

	waitForSignal(exit) // pass in exit channel to watch

	log.Println("Destroy...")

	G.Exit() // tell all the children to exit

	// tell inner main loop to exit
	resp := NewResp()
	go func() { exitchan <- Event{eventExit, resp, "", false} }()

	// cleanup etcd main loop last so it can process everything first
	if err := EmbdEtcd.Destroy(); err != nil { // shutdown and cleanup etcd
		log.Printf("Etcd exited poorly with: %v", err)
	}

	resp.ACKWait() // let inner main loop finish cleanly just in case

	if DEBUG {
		log.Printf("Graph: %v", G)
	}

	wg.Wait() // wait for primary go routines to exit

	// TODO: wait for each vertex to exit...
	log.Println("Goodbye!")
	return nil
}

func main() {
	var flags int
	if DEBUG || true { // TODO: remove || true
		flags = log.LstdFlags | log.Lshortfile
	}
	flags = (flags - log.Ldate) // remove the date for now
	log.SetFlags(flags)

	// un-hijack from capnslog...
	log.SetOutput(os.Stderr)
	if VERBOSE {
		capnslog.SetFormatter(capnslog.NewLogFormatter(os.Stderr, "(etcd) ", flags))
	} else {
		capnslog.SetFormatter(capnslog.NewNilFormatter())
	}

	// test for sanity
	if program == "" || version == "" {
		log.Fatal("Program was not compiled correctly. Please see Makefile.")
	}
	app := cli.NewApp()
	app.Name = program
	app.Usage = "next generation config management"
	app.Version = version
	//app.Action = ... // without a default action, help runs

	app.Commands = []cli.Command{
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run",
			Action:  run,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "file, f",
					Value:  "",
					Usage:  "graph definition to run",
					EnvVar: "MGMT_FILE",
				},
				cli.BoolFlag{
					Name:  "no-watch",
					Usage: "do not update graph on watched graph definition file changes",
				},
				cli.StringFlag{
					Name:  "code, c",
					Value: "",
					Usage: "code definition to run",
				},
				cli.StringFlag{
					Name:  "graphviz, g",
					Value: "",
					Usage: "output file for graphviz data",
				},
				cli.StringFlag{
					Name:  "graphviz-filter, gf",
					Value: "dot", // directed graph default
					Usage: "graphviz filter to use",
				},
				// useful for testing multiple instances on same machine
				cli.StringFlag{
					Name:  "hostname",
					Value: "",
					Usage: "hostname to use",
				},
				// if empty, it will startup a new server
				cli.StringSliceFlag{
					Name:   "seeds, s",
					Value:  &cli.StringSlice{}, // empty slice
					Usage:  "default etc client endpoint",
					EnvVar: "MGMT_SEEDS",
				},
				// port 2379 and 4001 are common
				cli.StringSliceFlag{
					Name:   "client-urls",
					Value:  &cli.StringSlice{},
					Usage:  "list of URLs to listen on for client traffic",
					EnvVar: "MGMT_CLIENT_URLS",
				},
				// port 2380 and 7001 are common
				cli.StringSliceFlag{
					Name:   "server-urls, peer-urls",
					Value:  &cli.StringSlice{},
					Usage:  "list of URLs to listen on for server (peer) traffic",
					EnvVar: "MGMT_SERVER_URLS",
				},
				cli.BoolFlag{
					Name:  "no-server",
					Usage: "do not let other servers peer with me",
				},
				cli.IntFlag{
					Name:   "ideal-cluster-size",
					Value:  defaultIdealClusterSize,
					Usage:  "ideal number of server peers in cluster, only read by initial server",
					EnvVar: "MGMT_IDEAL_CLUSTER_SIZE",
				},
				cli.IntFlag{
					Name:   "converged-timeout, t",
					Value:  -1,
					Usage:  "exit after approximately this many seconds in a converged state",
					EnvVar: "MGMT_CONVERGED_TIMEOUT",
				},
				cli.IntFlag{
					Name:   "max-runtime",
					Value:  0,
					Usage:  "exit after a maximum of approximately this many seconds",
					EnvVar: "MGMT_MAX_RUNTIME",
				},
				cli.BoolFlag{
					Name:  "noop",
					Usage: "globally force all resources into no-op mode",
				},
				cli.StringFlag{
					Name:  "puppet, p",
					Value: "",
					Usage: "load graph from puppet, optionally takes a manifest or path to manifest file",
				},
				cli.StringFlag{
					Name:  "puppet-conf",
					Value: "",
					Usage: "supply the path to an alternate puppet.conf file to use",
				},
			},
		},
	}
	app.EnableBashCompletion = true
	app.Run(os.Args)
}
