// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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
	"fmt"
	"github.com/codegangsta/cli"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// set at compile time
var (
	version string
	program string
)

const (
	DEBUG = false
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
			fmt.Println() // put ^C char from terminal on its own line
			log.Println("Interrupted by ^C")
		} else {
			log.Println("Interrupted by signal")
		}
	case <-exit: // or a manual signal
		log.Println("Interrupted by exit signal")
	}
}

func run(c *cli.Context) {
	var start int64 = time.Now().UnixNano()
	var wg sync.WaitGroup
	exit := make(chan bool) // exit signal
	log.Printf("This is: %v, version: %v\n", program, version)
	log.Printf("Start: %v\n", start)
	G := NewGraph("Graph") // give graph a default name

	// exit after `exittime` seconds for no reason at all...
	if i := c.Int("exittime"); i > 0 {
		go func() {
			time.Sleep(time.Duration(i) * time.Second)
			exit <- true
		}()
	}

	// initial etcd peer endpoint
	seed := c.String("seed")
	if seed == "" {
		// XXX: start up etcd server, others will join me!
		seed = "http://127.0.0.1:2379" // thus we use the local server!
	}
	// then, connect to `seed` as a client

	// FIXME: validate seed, or wait for it to fail in etcd init?

	// etcd
	hostname := c.String("hostname")
	if hostname == "" {
		hostname, _ = os.Hostname() // etcd watch key // XXX: this is not the correct key name this is the set key name... WOOPS
	}
	go func() {
		startchan := make(chan struct{}) // start signal
		go func() { startchan <- struct{}{} }()
		file := c.String("file")
		configchan := ConfigWatch(file)
		log.Printf("Starting etcd...\n")
		kapi := EtcdGetKAPI(seed)
		etcdchan := EtcdWatch(kapi)
		first := true // first loop or not
		for {
			select {
			case _ = <-startchan: // kick the loop once at start
				// pass
			case msg := <-etcdchan:
				switch msg {
				// some types of messages we ignore...
				case etcdFoo, etcdBar:
					continue
				// while others passthrough and cause a compile!
				case etcdStart, etcdEvent:
					// pass
				default:
					log.Fatal("Etcd: Unhandled message: %v", msg)
				}
			case msg := <-configchan:
				if c.Bool("no-watch") || !msg {
					continue // not ready to read config
				}

				//case compile_event: XXX
			}

			config := ParseConfigFromFile(file)
			if config == nil {
				log.Printf("Config parse failure")
				continue
			}

			// run graph vertex LOCK...
			if !first { // XXX: we can flatten this check out I think
				G.SetState(graphPausing)
				log.Printf("State: %v", G.State())
				G.Pause() // sync
				G.SetState(graphPaused)
				log.Printf("State: %v", G.State())
			}

			// build the graph from a config file
			// build the graph on events (eg: from etcd)
			UpdateGraphFromConfig(config, hostname, G, kapi)
			log.Printf("Graph: %v\n", G) // show graph
			err := G.ExecGraphviz(c.String("graphviz-filter"), c.String("graphviz"))
			if err != nil {
				log.Printf("Graphviz: %v", err)
			} else {
				log.Printf("Graphviz: Successfully generated graph!")
			}
			G.SetVertex()
			// G.Start(...) needs to be synchronous or wait,
			// because if half of the nodes are started and
			// some are not ready yet and the EtcdWatch
			// loops, we'll cause G.Pause(...) before we
			// even got going, thus causing nil pointer errors
			G.SetState(graphStarting)
			log.Printf("State: %v", G.State())
			G.Start(&wg) // sync
			G.SetState(graphStarted)
			log.Printf("State: %v", G.State())

			first = false
		}
	}()

	log.Println("Running...")

	waitForSignal(exit) // pass in exit channel to watch

	G.Exit() // tell all the children to exit

	if DEBUG {
		for i := range G.GetVerticesChan() {
			fmt.Printf("Vertex: %v\n", i)
		}
		fmt.Printf("Graph: %v\n", G)
	}

	wg.Wait() // wait for primary go routines to exit

	// TODO: wait for each vertex to exit...
	log.Println("Goodbye!")
}

func main() {
	//if DEBUG {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	//}
	log.SetFlags(log.Flags() - log.Ldate) // remove the date for now
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
					Name:  "file, f",
					Value: "",
					Usage: "graph definition to run",
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
				cli.StringFlag{
					Name:  "seed, s",
					Value: "",
					Usage: "default etc peer endpoint",
				},
				cli.IntFlag{
					Name:  "exittime",
					Value: 0,
					Usage: "exit after a maximum of approximately this many seconds",
				},
			},
		},
	}
	app.EnableBashCompletion = true
	app.Run(os.Args)
}
