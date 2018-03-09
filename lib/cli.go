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
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/purpleidea/mgmt/bindata"
	"github.com/purpleidea/mgmt/gapi"

	"github.com/spf13/afero"
	"github.com/urfave/cli"
)

// Fs is a simple wrapper to a memory backed file system to be used for
// standalone deploys. This is basically a pass-through so that we fulfill the
// same interface that the deploy mechanism uses.
type Fs struct {
	*afero.Afero
}

// URI returns the unique URI of this filesystem. It returns the root path.
func (obj *Fs) URI() string { return fmt.Sprintf("%s://"+"/", obj.Name()) }

// run is the main run target.
func run(c *cli.Context) error {

	obj := &Main{}

	obj.Program = c.App.Name
	obj.Version = c.App.Version
	if val, exists := c.App.Metadata["flags"]; exists {
		if flags, ok := val.(Flags); ok {
			obj.Flags = flags
		}
	}

	if h := c.String("hostname"); c.IsSet("hostname") && h != "" {
		obj.Hostname = &h
	}

	if s := c.String("prefix"); c.IsSet("prefix") && s != "" {
		obj.Prefix = &s
	}
	obj.TmpPrefix = c.Bool("tmp-prefix")
	obj.AllowTmpPrefix = c.Bool("allow-tmp-prefix")

	// add the versions GAPIs
	names := []string{}
	for name := range gapi.RegisteredGAPIs {
		names = append(names, name)
	}
	sort.Strings(names) // ensure deterministic order when parsing

	// create a memory backed temporary filesystem for storing runtime data
	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
	standaloneFs := &Fs{afs}
	obj.DeployFs = standaloneFs

	for _, name := range names {
		fn := gapi.RegisteredGAPIs[name]
		deployObj, err := fn().Cli(c, standaloneFs)
		if err != nil {
			log.Printf("GAPI cli parse error: %v", err)
			//return cli.NewExitError(err.Error(), 1) // TODO: ?
			return cli.NewExitError("", 1)
		}
		if deployObj == nil { // not used
			continue
		}
		if obj.Deploy != nil { // already set one
			return fmt.Errorf("can't combine `%s` GAPI with existing GAPI", name)
		}
		obj.Deploy = deployObj
	}

	obj.NoWatch = c.Bool("no-watch")
	obj.NoConfigWatch = c.Bool("no-config-watch")
	obj.NoStreamWatch = c.Bool("no-stream-watch")

	obj.Noop = c.Bool("noop")
	obj.Sema = c.Int("sema")
	obj.Graphviz = c.String("graphviz")
	obj.GraphvizFilter = c.String("graphviz-filter")
	obj.ConvergedTimeout = c.Int("converged-timeout")
	obj.ConvergedTimeoutNoExit = c.Bool("converged-timeout-no-exit")
	obj.ConvergedStatusFile = c.String("converged-status-file")
	obj.MaxRuntime = uint(c.Int("max-runtime"))

	obj.Seeds = c.StringSlice("seeds")
	obj.ClientURLs = c.StringSlice("client-urls")
	obj.ServerURLs = c.StringSlice("server-urls")
	obj.AdvertiseClientURLs = c.StringSlice("advertise-client-urls")
	obj.AdvertiseServerURLs = c.StringSlice("advertise-server-urls")
	obj.IdealClusterSize = c.Int("ideal-cluster-size")
	obj.NoServer = c.Bool("no-server")

	obj.NoPgp = c.Bool("no-pgp")

	if kp := c.String("pgp-key-path"); c.IsSet("pgp-key-path") {
		obj.PgpKeyPath = &kp
	}

	if us := c.String("pgp-identity"); c.IsSet("pgp-identity") {
		obj.PgpIdentity = &us
	}

	if err := obj.Init(); err != nil {
		return err
	}

	obj.Prometheus = c.Bool("prometheus")
	obj.PrometheusListen = c.String("prometheus-listen")

	// install the exit signal handler
	exit := make(chan struct{})
	defer close(exit)
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt) // catch ^C
		//signal.Notify(signals, os.Kill) // catch signals
		signal.Notify(signals, syscall.SIGTERM)

		select {
		case sig := <-signals: // any signal will do
			if sig == os.Interrupt {
				log.Println("Interrupted by ^C")
				obj.Exit(nil)
				return
			}
			log.Println("Interrupted by signal")
			obj.Exit(fmt.Errorf("killed by %v", sig))
			return
		case <-exit:
			return
		}
	}()

	if err := obj.Run(); err != nil {
		// log the error message returned
		log.Printf("Main: Error: %v", err)
		//return cli.NewExitError(err.Error(), 1) // TODO: ?
		return cli.NewExitError("", 1)
	}
	return nil
}

// CLI is the entry point for using mgmt normally from the CLI.
func CLI(program, version string, flags Flags) error {

	// test for sanity
	if program == "" || version == "" {
		return fmt.Errorf("program was not compiled correctly, see Makefile")
	}

	runFlags := []cli.Flag{
		// useful for testing multiple instances on same machine
		cli.StringFlag{
			Name:  "hostname",
			Value: "",
			Usage: "hostname to use",
		},

		cli.StringFlag{
			Name:   "prefix",
			Usage:  "specify a path to the working prefix directory",
			EnvVar: "MGMT_PREFIX",
		},
		cli.BoolFlag{
			Name:  "tmp-prefix",
			Usage: "request a pseudo-random, temporary prefix to be used",
		},
		cli.BoolFlag{
			Name:  "allow-tmp-prefix",
			Usage: "allow creation of a new temporary prefix if main prefix is unavailable",
		},

		cli.BoolFlag{
			Name:  "no-watch",
			Usage: "do not update graph under any switch events",
		},
		cli.BoolFlag{
			Name:  "no-config-watch",
			Usage: "do not update graph on config switch events",
		},
		cli.BoolFlag{
			Name:  "no-stream-watch",
			Usage: "do not update graph on stream switch events",
		},

		cli.BoolFlag{
			Name:  "noop",
			Usage: "globally force all resources into no-op mode",
		},
		cli.IntFlag{
			Name:  "sema",
			Value: -1,
			Usage: "globally add a semaphore to all resources with this lock count",
		},
		cli.StringFlag{
			Name:  "graphviz, g",
			Value: "",
			Usage: "output file for graphviz data",
		},
		cli.StringFlag{
			Name:  "graphviz-filter, gf",
			Value: "",
			Usage: "graphviz filter to use",
		},
		cli.IntFlag{
			Name:   "converged-timeout, t",
			Value:  -1,
			Usage:  "after approximately this many seconds without activity, we're considered to be in a converged state",
			EnvVar: "MGMT_CONVERGED_TIMEOUT",
		},
		cli.BoolFlag{
			Name:  "converged-timeout-no-exit",
			Usage: "don't exit on converged-timeout",
		},
		cli.StringFlag{
			Name:  "converged-status-file",
			Value: "",
			Usage: "file to append the current converged state to, mostly used for testing",
		},
		cli.IntFlag{
			Name:   "max-runtime",
			Value:  0,
			Usage:  "exit after a maximum of approximately this many seconds",
			EnvVar: "MGMT_MAX_RUNTIME",
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
		// port 2379 and 4001 are common
		cli.StringSliceFlag{
			Name:   "advertise-client-urls",
			Value:  &cli.StringSlice{},
			Usage:  "list of URLs to listen on for client traffic",
			EnvVar: "MGMT_ADVERTISE_CLIENT_URLS",
		},
		// port 2380 and 7001 are common
		cli.StringSliceFlag{
			Name:   "advertise-server-urls, advertise-peer-urls",
			Value:  &cli.StringSlice{},
			Usage:  "list of URLs to listen on for server (peer) traffic",
			EnvVar: "MGMT_ADVERTISE_SERVER_URLS",
		},
		cli.IntFlag{
			Name:   "ideal-cluster-size",
			Value:  -1,
			Usage:  "ideal number of server peers in cluster; only read by initial server",
			EnvVar: "MGMT_IDEAL_CLUSTER_SIZE",
		},
		cli.BoolFlag{
			Name:  "no-server",
			Usage: "do not start embedded etcd server (do not promote from client to peer)",
		},

		cli.BoolFlag{
			Name:  "no-pgp",
			Usage: "don't create pgp keys",
		},
		cli.StringFlag{
			Name:  "pgp-key-path",
			Value: "",
			Usage: "path for instance key pair",
		},
		cli.StringFlag{
			Name:  "pgp-identity",
			Value: "",
			Usage: "default identity used for generation",
		},
		cli.BoolFlag{
			Name:  "prometheus",
			Usage: "start a prometheus instance",
		},
		cli.StringFlag{
			Name:  "prometheus-listen",
			Value: "",
			Usage: "specify prometheus instance binding",
		},
	}

	subCommands := []cli.Command{} // build deploy sub commands

	names := []string{}
	for name := range gapi.RegisteredGAPIs {
		names = append(names, name)
	}
	sort.Strings(names) // ensure deterministic order when parsing
	for _, x := range names {
		name := x // create a copy in this scope
		fn := gapi.RegisteredGAPIs[name]
		gapiObj := fn()
		flags := gapiObj.CliFlags() // []cli.Flag

		runFlags = append(runFlags, flags...)

		command := cli.Command{
			Name:  name,
			Usage: fmt.Sprintf("deploy using the `%s` frontend", name),
			Action: func(c *cli.Context) error {
				if err := deploy(c, name, gapiObj); err != nil {
					log.Printf("Deploy: Error: %v", err)
					//return cli.NewExitError(err.Error(), 1) // TODO: ?
					return cli.NewExitError("", 1)
				}
				return nil
			},
			Flags: flags,
		}
		subCommands = append(subCommands, command)
	}

	app := cli.NewApp()
	app.Name = program // App.name and App.version pass these values through
	app.Version = version
	app.Usage = "next generation config management"
	app.Metadata = map[string]interface{}{ // additional flags
		"flags": flags,
	}

	// if no app.Command is specified
	app.Action = func(c *cli.Context) error {
		// print the license
		if c.Bool("license") {
			license, err := bindata.Asset("../COPYING") // use go-bindata to get the bytes
			if err != nil {
				return err
			}

			fmt.Printf("%s", license)
			return nil
		}

		// print help if no flags are set
		cli.ShowAppHelp(c)
		return nil
	}

	// global flags
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "license",
			Usage: "prints the software license",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run",
			Action:  run,
			Flags:   runFlags,
		},
		{
			Name:        "deploy",
			Aliases:     []string{"d"},
			Usage:       "deploy",
			Subcommands: subCommands,
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:   "seeds, s",
					Value:  &cli.StringSlice{}, // empty slice
					Usage:  "default etc client endpoint",
					EnvVar: "MGMT_SEEDS",
				},

				// common flags which all can use
				cli.BoolFlag{
					Name:  "noop",
					Usage: "globally force all resources into no-op mode",
				},
				cli.IntFlag{
					Name:  "sema",
					Value: -1,
					Usage: "globally add a semaphore to all resources with this lock count",
				},

				cli.BoolFlag{
					Name:  "no-git",
					Usage: "don't look at git commit id for safe deploys",
				},
				cli.BoolFlag{
					Name:  "force",
					Usage: "force a new deploy, even if the safety chain would break",
				},
			},
		},
	}
	app.EnableBashCompletion = true
	return app.Run(os.Args)
}
