// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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
	"sort"

	"github.com/purpleidea/mgmt/gapi"
	_ "github.com/purpleidea/mgmt/lang" // import so the GAPI registers
	_ "github.com/purpleidea/mgmt/langpuppet"
	_ "github.com/purpleidea/mgmt/puppet"
	_ "github.com/purpleidea/mgmt/yamlgraph"

	"github.com/urfave/cli/v2"
)

// CLIArgs is a struct of values that we pass to the main CLI function.
type CLIArgs struct {
	Program string
	Version string
	Copying string
	Flags   Flags
}

// CLI is the entry point for using mgmt normally from the CLI.
func CLI(cliArgs *CLIArgs) error {
	// test for sanity
	if cliArgs == nil {
		return fmt.Errorf("this CLI was not run correctly")
	}
	if cliArgs.Program == "" || cliArgs.Version == "" {
		return fmt.Errorf("program was not compiled correctly, see Makefile")
	}
	if cliArgs.Copying == "" {
		return fmt.Errorf("program copyrights we're removed, can't run")
	}

	// All of these flags can be accessed in your GAPI implementation with
	// the `c.Lineage()[1].Type` and `c.Lineage()[1].IsSet` functions. Their own
	// flags can be accessed with `c.Type` and `c.IsSet` directly.
	runFlags := []cli.Flag{
		// common flags which all can use

		// useful for testing multiple instances on same machine
		&cli.StringFlag{
			Name:  "hostname",
			Value: "",
			Usage: "hostname to use",
		},

		&cli.StringFlag{
			Name:    "prefix",
			Usage:   "specify a path to the working prefix directory",
			EnvVars: []string{"MGMT_PREFIX"},
		},
		&cli.BoolFlag{
			Name:  "tmp-prefix",
			Usage: "request a pseudo-random, temporary prefix to be used",
		},
		&cli.BoolFlag{
			Name:  "allow-tmp-prefix",
			Usage: "allow creation of a new temporary prefix if main prefix is unavailable",
		},

		&cli.BoolFlag{
			Name:  "no-watch",
			Usage: "do not update graph under any switch events",
		},
		&cli.BoolFlag{
			Name:  "no-stream-watch",
			Usage: "do not update graph on stream switch events",
		},
		&cli.BoolFlag{
			Name:  "no-deploy-watch",
			Usage: "do not change deploys after an initial deploy",
		},

		&cli.BoolFlag{
			Name:  "noop",
			Usage: "globally force all resources into no-op mode",
		},
		&cli.IntFlag{
			Name:  "sema",
			Value: -1,
			Usage: "globally add a semaphore to all resources with this lock count",
		},
		&cli.StringFlag{
			Name:  "graphviz, g",
			Value: "",
			Usage: "output file for graphviz data",
		},
		&cli.StringFlag{
			Name:  "graphviz-filter, gf",
			Value: "",
			Usage: "graphviz filter to use",
		},
		&cli.Int64Flag{
			Name:    "converged-timeout, t",
			Value:   -1,
			Usage:   "after approximately this many seconds without activity, we're considered to be in a converged state",
			EnvVars: []string{"MGMT_CONVERGED_TIMEOUT"},
		},
		&cli.BoolFlag{
			Name:  "converged-timeout-no-exit",
			Usage: "don't exit on converged-timeout",
		},
		&cli.StringFlag{
			Name:  "converged-status-file",
			Value: "",
			Usage: "file to append the current converged state to, mostly used for testing",
		},
		&cli.IntFlag{
			Name:    "max-runtime",
			Value:   0,
			Usage:   "exit after a maximum of approximately this many seconds",
			EnvVars: []string{"MGMT_MAX_RUNTIME"},
		},

		// if empty, it will startup a new server
		&cli.StringSliceFlag{
			Name:    "seeds, s",
			Value:   &cli.StringSlice{}, // empty slice
			Usage:   "default etc client endpoint",
			EnvVars: []string{"MGMT_SEEDS"},
		},
		// port 2379 and 4001 are common
		&cli.StringSliceFlag{
			Name:    "client-urls",
			Value:   &cli.StringSlice{},
			Usage:   "list of URLs to listen on for client traffic",
			EnvVars: []string{"MGMT_CLIENT_URLS"},
		},
		// port 2380 and 7001 are common
		&cli.StringSliceFlag{
			Name:    "server-urls, peer-urls",
			Value:   &cli.StringSlice{},
			Usage:   "list of URLs to listen on for server (peer) traffic",
			EnvVars: []string{"MGMT_SERVER_URLS"},
		},
		// port 2379 and 4001 are common
		&cli.StringSliceFlag{
			Name:    "advertise-client-urls",
			Value:   &cli.StringSlice{},
			Usage:   "list of URLs to listen on for client traffic",
			EnvVars: []string{"MGMT_ADVERTISE_CLIENT_URLS"},
		},
		// port 2380 and 7001 are common
		&cli.StringSliceFlag{
			Name:    "advertise-server-urls, advertise-peer-urls",
			Value:   &cli.StringSlice{},
			Usage:   "list of URLs to listen on for server (peer) traffic",
			EnvVars: []string{"MGMT_ADVERTISE_SERVER_URLS"},
		},
		&cli.IntFlag{
			Name:    "ideal-cluster-size",
			Value:   -1,
			Usage:   "ideal number of server peers in cluster; only read by initial server",
			EnvVars: []string{"MGMT_IDEAL_CLUSTER_SIZE"},
		},
		&cli.BoolFlag{
			Name:  "no-server",
			Usage: "do not start embedded etcd server (do not promote from client to peer)",
		},
		&cli.BoolFlag{
			Name:    "no-network",
			Usage:   "run single node instance without clustering or opening tcp ports to the outside",
			EnvVars: []string{"MGMT_NO_NETWORK"},
		},
		&cli.BoolFlag{
			Name:  "no-pgp",
			Usage: "don't create pgp keys",
		},
		&cli.StringFlag{
			Name:  "pgp-key-path",
			Value: "",
			Usage: "path for instance key pair",
		},
		&cli.StringFlag{
			Name:  "pgp-identity",
			Value: "",
			Usage: "default identity used for generation",
		},
		&cli.BoolFlag{
			Name:  "prometheus",
			Usage: "start a prometheus instance",
		},
		&cli.StringFlag{
			Name:  "prometheus-listen",
			Value: "",
			Usage: "specify prometheus instance binding",
		},
	}
	deployFlags := []cli.Flag{
		// common flags which all can use
		&cli.StringSliceFlag{
			Name:    "seeds, s",
			Value:   &cli.StringSlice{}, // empty slice
			Usage:   "default etc client endpoint",
			EnvVars: []string{"MGMT_SEEDS"},
		},
		&cli.BoolFlag{
			Name:  "noop",
			Usage: "globally force all resources into no-op mode",
		},
		&cli.IntFlag{
			Name:  "sema",
			Value: -1,
			Usage: "globally add a semaphore to all resources with this lock count",
		},

		&cli.BoolFlag{
			Name:  "no-git",
			Usage: "don't look at git commit id for safe deploys",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "force a new deploy, even if the safety chain would break",
		},
	}
	getFlags := []cli.Flag{
		// common flags which all can use
		&cli.BoolFlag{
			Name:  "noop",
			Usage: "simulate the download (can't recurse)",
		},
		&cli.IntFlag{
			Name:  "sema",
			Value: -1, // maximum parallelism
			Usage: "globally add a semaphore to downloads with this lock count",
		},
		&cli.BoolFlag{
			Name:  "update",
			Usage: "update all dependencies to the latest versions",
		},
	}

	subCommandsRun := []*cli.Command{}    // run sub commands
	subCommandsDeploy := []*cli.Command{} // deploy sub commands
	subCommandsGet := []*cli.Command{}    // get (download) sub commands

	names := []string{}
	for name := range gapi.RegisteredGAPIs {
		names = append(names, name)
	}
	sort.Strings(names) // ensure deterministic order when parsing
	for _, x := range names {
		name := x // create a copy in this scope
		fn := gapi.RegisteredGAPIs[name]
		gapiObj := fn()

		commandRun := &cli.Command{
			Name:  name,
			Usage: fmt.Sprintf("run using the `%s` frontend", name),
			Action: func(c *cli.Context) error {
				if err := run(c, name, gapiObj); err != nil {
					log.Printf("run: error: %v", err)
					//return cli.NewExitError(err.Error(), 1) // TODO: ?
					return cli.NewExitError("", 1)
				}
				return nil
			},
			Flags: gapiObj.CliFlags(gapi.CommandRun),
		}
		subCommandsRun = append(subCommandsRun, commandRun)

		commandDeploy := &cli.Command{
			Name:  name,
			Usage: fmt.Sprintf("deploy using the `%s` frontend", name),
			Action: func(c *cli.Context) error {
				if err := deploy(c, name, gapiObj); err != nil {
					log.Printf("deploy: error: %v", err)
					//return cli.NewExitError(err.Error(), 1) // TODO: ?
					return cli.NewExitError("", 1)
				}
				return nil
			},
			Flags: gapiObj.CliFlags(gapi.CommandDeploy),
		}
		subCommandsDeploy = append(subCommandsDeploy, commandDeploy)

		if _, ok := gapiObj.(gapi.GettableGAPI); ok {
			commandGet := &cli.Command{
				Name:  name,
				Usage: fmt.Sprintf("get (download) using the `%s` frontend", name),
				Action: func(c *cli.Context) error {
					if err := get(c, name, gapiObj); err != nil {
						log.Printf("get: error: %v", err)
						//return cli.NewExitError(err.Error(), 1) // TODO: ?
						return cli.NewExitError("", 1)
					}
					return nil
				},
				Flags: gapiObj.CliFlags(gapi.CommandGet),
			}
			subCommandsGet = append(subCommandsGet, commandGet)
		}
	}

	app := cli.NewApp()
	app.Name = cliArgs.Program // App.name and App.version pass these values through
	app.Version = cliArgs.Version
	app.Usage = "next generation config management"
	app.Metadata = map[string]interface{}{ // additional flags
		"flags": cliArgs.Flags,
	}

	// if no app.Command is specified
	app.Action = func(c *cli.Context) error {
		// print the license
		if c.Bool("license") {
			fmt.Printf("%s", cliArgs.Copying)
			return nil
		}

		// print help if no flags are set
		cli.ShowAppHelp(c)
		return nil
	}

	// global flags
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "license",
			Usage: "prints the software license",
		},
	}

	app.Commands = []*cli.Command{
		//{
		//	Name:    gapi.CommandTODO,
		//	Aliases: []string{"TODO"},
		//	Usage:   "TODO",
		//	Action:  TODO,
		//	Flags:   TODOFlags,
		//},
	}

	// run always requires a frontend to start the engine, but if you don't
	// want a graph, you can use the `empty` frontend. The engine backend is
	// agnostic to which frontend is running, in fact, you can deploy with
	// multiple different frontends, one after another, on the same engine.
	if len(subCommandsRun) > 0 {
		commandRun := &cli.Command{
			Name:        gapi.CommandRun,
			Aliases:     []string{"r"},
			Usage:       "Run code on this machine",
			Subcommands: subCommandsRun,
			Flags:       runFlags,
		}
		app.Commands = append(app.Commands, commandRun)
	}

	if len(subCommandsDeploy) > 0 {
		commandDeploy := &cli.Command{
			Name:        gapi.CommandDeploy,
			Aliases:     []string{"d"},
			Usage:       "Deploy code into the cluster",
			Subcommands: subCommandsDeploy,
			Flags:       deployFlags,
		}
		app.Commands = append(app.Commands, commandDeploy)
	}

	if len(subCommandsGet) > 0 {
		commandGet := &cli.Command{
			Name:        gapi.CommandGet,
			Aliases:     []string{"g"},
			Usage:       "Download code from the internet",
			Subcommands: subCommandsGet,
			Flags:       getFlags,
		}
		app.Commands = append(app.Commands, commandGet)
	}

	app.EnableBashCompletion = true
	return app.Run(os.Args)
}
