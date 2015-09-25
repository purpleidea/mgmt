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

	// exit after `exittime` seconds for no reason at all...
	if i := c.Int("exittime"); i > 0 {
		go func() {
			time.Sleep(time.Duration(i) * time.Second)
			exit <- true
		}()
	}

	// build the graph from a config file
	G := GraphFromConfig(c.String("file"))
	log.Printf("Graph: %v\n", G) // show graph

	log.Printf("Start: %v\n", start)

	for x := range G.GetVerticesChan() { // XXX ?
		log.Printf("Main->Starting[%v]\n", x.Name)

		wg.Add(1)
		// must pass in value to avoid races...
		// see: https://ttboj.wordpress.com/2015/07/27/golang-parallelism-issues-causing-too-many-open-files-error/
		go func(v *Vertex) {
			defer wg.Done()
			v.Start()
			log.Printf("Main->Finish[%v]\n", v.Name)
		}(x)

		// generate a startup "poke" so that an initial check happens
		go func(v *Vertex) {
			v.Events <- fmt.Sprintf("Startup(%v)", v.Name)
		}(x)
	}

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
