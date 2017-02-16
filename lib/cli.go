// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/purpleidea/mgmt/puppet"
	"github.com/purpleidea/mgmt/yamlgraph"

	"github.com/urfave/cli"
)

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

	if _ = c.String("code"); c.IsSet("code") {
		if obj.GAPI != nil {
			return fmt.Errorf("Can't combine code GAPI with existing GAPI.")
		}
		// TODO: implement DSL GAPI
		//obj.GAPI = &dsl.GAPI{
		//	Code: &s,
		//}
		return fmt.Errorf("The Code GAPI is not implemented yet!") // TODO: DSL
	}
	if y := c.String("yaml"); c.IsSet("yaml") {
		if obj.GAPI != nil {
			return fmt.Errorf("Can't combine YAML GAPI with existing GAPI.")
		}
		obj.GAPI = &yamlgraph.GAPI{
			File: &y,
		}
	}
	if p := c.String("puppet"); c.IsSet("puppet") {
		if obj.GAPI != nil {
			return fmt.Errorf("Can't combine puppet GAPI with existing GAPI.")
		}
		obj.GAPI = &puppet.GAPI{
			PuppetParam: &p,
			PuppetConf:  c.String("puppet-conf"),
		}
	}
	obj.Remotes = c.StringSlice("remote") // FIXME: GAPI-ify somehow?

	obj.NoWatch = c.Bool("no-watch")
	obj.Noop = c.Bool("noop")
	obj.Graphviz = c.String("graphviz")
	obj.GraphvizFilter = c.String("graphviz-filter")
	obj.ConvergedTimeout = c.Int("converged-timeout")
	obj.MaxRuntime = uint(c.Int("max-runtime"))

	obj.Seeds = c.StringSlice("seeds")
	obj.ClientURLs = c.StringSlice("client-urls")
	obj.ServerURLs = c.StringSlice("server-urls")
	obj.IdealClusterSize = c.Int("ideal-cluster-size")
	obj.NoServer = c.Bool("no-server")

	obj.CConns = uint16(c.Int("cconns"))
	obj.AllowInteractive = c.Bool("allow-interactive")
	obj.SSHPrivIDRsa = c.String("ssh-priv-id-rsa")
	obj.NoCaching = c.Bool("no-caching")
	obj.Depth = uint16(c.Int("depth"))

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
			obj.Exit(fmt.Errorf("Killed by %v", sig))
			return
		case <-exit:
			return
		}
	}()

	if err := obj.Run(); err != nil {
		return err
		//return cli.NewExitError(err.Error(), 1) // TODO: ?
		//return cli.NewExitError("", 1) // TODO: ?
	}
	return nil
}

// CLI is the entry point for using mgmt normally from the CLI.
func CLI(program, version string, flags Flags) error {

	// test for sanity
	if program == "" || version == "" {
		return fmt.Errorf("Program was not compiled correctly. Please see Makefile.")
	}
	app := cli.NewApp()
	app.Name = program // App.name and App.version pass these values through
	app.Version = version
	app.Usage = "next generation config management"
	app.Metadata = map[string]interface{}{ // additional flags
		"flags": flags,
	}
	//app.Action = ... // without a default action, help runs

	app.Commands = []cli.Command{
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run",
			Action:  run,
			Flags: []cli.Flag{
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

				cli.StringFlag{
					Name:  "code, c",
					Value: "",
					Usage: "code definition to run",
				},
				cli.StringFlag{
					Name:  "yaml",
					Value: "",
					Usage: "yaml graph definition to run",
				},
				cli.StringFlag{
					Name:  "puppet, p",
					Value: "",
					Usage: "load graph from puppet, optionally takes a manifest or path to manifest file",
				},
				cli.StringFlag{
					Name:  "puppet-conf",
					Value: "",
					Usage: "the path to an alternate puppet.conf file",
				},
				cli.StringSliceFlag{
					Name:  "remote",
					Value: &cli.StringSlice{},
					Usage: "list of remote graph definitions to run",
				},

				cli.BoolFlag{
					Name:  "no-watch",
					Usage: "do not update graph on stream switch events",
				},
				cli.BoolFlag{
					Name:  "noop",
					Usage: "globally force all resources into no-op mode",
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
				cli.IntFlag{
					Name:   "ideal-cluster-size",
					Value:  -1,
					Usage:  "ideal number of server peers in cluster; only read by initial server",
					EnvVar: "MGMT_IDEAL_CLUSTER_SIZE",
				},
				cli.BoolFlag{
					Name:  "no-server",
					Usage: "do not let other servers peer with me",
				},

				cli.IntFlag{
					Name:   "cconns",
					Value:  0,
					Usage:  "number of maximum concurrent remote ssh connections to run; 0 for unlimited",
					EnvVar: "MGMT_CCONNS",
				},
				cli.BoolFlag{
					Name:  "allow-interactive",
					Usage: "allow interactive prompting, such as for remote passwords",
				},
				cli.StringFlag{
					Name:   "ssh-priv-id-rsa",
					Value:  "~/.ssh/id_rsa",
					Usage:  "default path to ssh key file, set empty to never touch",
					EnvVar: "MGMT_SSH_PRIV_ID_RSA",
				},
				cli.BoolFlag{
					Name:  "no-caching",
					Usage: "don't allow remote caching of remote execution binary",
				},
				cli.IntFlag{
					Name:   "depth",
					Hidden: true, // internal use only
					Value:  0,
					Usage:  "specify depth in remote hierarchy",
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
			},
		},
	}
	app.EnableBashCompletion = true
	return app.Run(os.Args)
}
