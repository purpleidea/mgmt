// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/gapi"
	emptyGAPI "github.com/purpleidea/mgmt/gapi/empty"
	langGAPI "github.com/purpleidea/mgmt/lang/gapi"
	"github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	yamlGAPI "github.com/purpleidea/mgmt/yamlgraph"

	"github.com/spf13/afero"
)

// RunArgs is the CLI parsing structure and type of the parsed result. This
// particular one contains all the common flags for the `run` subcommand which
// all frontends can use.
type RunArgs struct {

	// useful for testing multiple instances on same machine
	Hostname       *string `arg:"--hostname" help:"hostname to use"`
	Prefix         *string `arg:"--prefix,env:MGMT_PREFIX" help:"specify a path to the working prefix directory"`
	TmpPrefix      bool    `arg:"--tmp-prefix" help:"request a pseudo-random, temporary prefix to be used"`
	AllowTmpPrefix bool    `arg:"--allow-tmp-prefix" help:"allow creation of a new temporary prefix if main prefix is unavailable"`

	NoWatch       bool `arg:"--no-watch" help:"do not update graph under any switch events"`
	NoStreamWatch bool `arg:"--no-stream-watch" help:"do not update graph on stream switch events"`
	NoDeployWatch bool `arg:"--no-deploy-watch" help:"do not change deploys after an initial deploy"`

	Noop bool `arg:"--noop" help:"globally force all resources into no-op mode"`
	Sema int  `arg:"--sema" default:"-1" help:"globally add a semaphore to downloads with this lock count"`

	Graphviz               string `arg:"--graphviz" help:"output file for graphviz data"`
	GraphvizFilter         string `arg:"--graphviz-filter" help:"graphviz filter to use"`
	ConvergedTimeout       int    `arg:"--converged-timeout,env:MGMT_CONVERGED_TIMEOUT" default:"-1" help:"after approximately this many seconds without activity, we're considered to be in a converged state"`
	ConvergedTimeoutNoExit bool   `arg:"--converged-timeout-no-exit" help:"don't exit on converged-timeout"`
	ConvergedStatusFile    string `arg:"--converged-status-file" help:"file to append the current converged state to, mostly used for testing"`
	MaxRuntime             uint   `arg:"--max-runtime,env:MGMT_MAX_RUNTIME" help:"exit after a maximum of approximately this many seconds"`

	// if empty, it will startup a new server
	Seeds []string `arg:"--seeds,env:MGMT_SEEDS" help:"default etc client endpoint"`

	// port 2379 and 4001 are common
	ClientURLs []string `arg:"--client-urls,env:MGMT_CLIENT_URLS" help:"list of URLs to listen on for client traffic"`

	// port 2380 and 7001 are common
	// etcd now uses --peer-urls
	ServerURLs []string `arg:"--server-urls,env:MGMT_SERVER_URLS" help:"list of URLs to listen on for server (peer) traffic"`

	// port 2379 and 4001 are common
	AdvertiseClientURLs []string `arg:"--advertise-client-urls,env:MGMT_ADVERTISE_CLIENT_URLS" help:"list of URLs to listen on for client traffic"`

	// port 2380 and 7001 are common
	// etcd now uses --advertise-peer-urls
	AdvertiseServerURLs []string `arg:"--advertise-server-urls,env:MGMT_ADVERTISE_SERVER_URLS" help:"list of URLs to listen on for server (peer) traffic"`

	IdealClusterSize int  `arg:"--ideal-cluster-size,env:MGMT_IDEAL_CLUSTER_SIZE" default:"-1" help:"ideal number of server peers in cluster; only read by initial server"`
	NoServer         bool `arg:"--no-server" help:"do not start embedded etcd server (do not promote from client to peer)"`
	NoNetwork        bool `arg:"--no-network,env:MGMT_NO_NETWORK" help:"run single node instance without clustering or opening tcp ports to the outside"`

	NoPgp       bool    `arg:"--no-pgp" help:"don't create pgp keys"`
	PgpKeyPath  *string `arg:"--pgp-key-path" help:"path for instance key pair"`
	PgpIdentity *string `arg:"--pgp-identity" help:"default identity used for generation"`

	Prometheus       bool   `arg:"--prometheus" help:"start a prometheus instance"`
	PrometheusListen string `arg:"--prometheus-listen" help:"specify prometheus instance binding"`

	RunEmpty *emptyGAPI.Args `arg:"subcommand:empty" help:"run empty payload"`
	RunLang  *langGAPI.Args  `arg:"subcommand:lang" help:"run lang (mcl) payload"`
	RunYaml  *yamlGAPI.Args  `arg:"subcommand:yaml" help:"run yaml graph payload"`
}

// Run executes the correct subcommand. It errors if there's ever an error. It
// returns true if we did activate one of the subcommands. It returns false if
// we did not. This information is used so that the top-level parser can return
// usage or help information if no subcommand activates. This particular Run is
// the run for the main `run` subcommand. This always requires a frontend to
// start the engine, but if you don't want a graph, you can use the `empty`
// frontend. The engine backend is agnostic to which frontend is running, in
// fact, you can deploy with multiple different frontends, one after another, on
// the same engine.
func (obj *RunArgs) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {
	var name string
	var args interface{}
	if cmd := obj.RunEmpty; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "empty"
		args = cmd
	}
	if cmd := obj.RunLang; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "lang"
		args = cmd
	}
	if cmd := obj.RunYaml; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "yaml"
		args = cmd
	}

	// XXX: workaround https://github.com/alexflint/go-arg/issues/239
	lists := [][]string{
		obj.Seeds,
		obj.ClientURLs,
		obj.ServerURLs,
		obj.AdvertiseClientURLs,
		obj.AdvertiseServerURLs,
	}
	gapiNames := gapi.Names() // list of registered names
	for _, list := range lists {
		if l := len(list); name == "" && l > 1 {
			elem := list[l-2] // second to last element
			if util.StrInList(elem, gapiNames) {
				return false, cliUtil.CliParseError(cliUtil.MissingEquals) // consistent errors
			}
		}
	}

	fn, exists := gapi.RegisteredGAPIs[name]
	if !exists {
		return false, nil // did not activate
	}
	gapiObj := fn()

	main := &lib.Main{}

	main.Program, main.Version = data.Program, data.Version
	main.Flags = lib.Flags{
		Debug:   data.Flags.Debug,
		Verbose: data.Flags.Verbose,
	}
	Logf := func(format string, v ...interface{}) {
		log.Printf("main: "+format, v...)
	}

	cliUtil.Hello(main.Program, main.Version, data.Flags) // say hello!
	defer Logf("goodbye!")

	main.Hostname = obj.Hostname
	main.Prefix = obj.Prefix
	main.TmpPrefix = obj.TmpPrefix
	main.AllowTmpPrefix = obj.AllowTmpPrefix

	// create a memory backed temporary filesystem for storing runtime data
	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
	standaloneFs := &util.AferoFs{Afero: afs}
	main.DeployFs = standaloneFs

	info := &gapi.Info{
		Args: args,
		Flags: &gapi.Flags{
			Hostname: obj.Hostname,
			Noop:     obj.Noop,
			Sema:     obj.Sema,
			//Update: obj.Update,
		},

		Fs:    standaloneFs,
		Debug: main.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("cli: "+format, v...)
		},
	}

	deploy, err := gapiObj.Cli(info)
	if err != nil {
		return false, cliUtil.CliParseError(err) // consistent errors
	}

	if cmd := obj.RunLang; cmd != nil && cmd.OnlyUnify && deploy == nil {
		return true, nil // we end early
	}
	if cmd := obj.RunLang; cmd != nil && cmd.OnlyDownload && deploy == nil {
		return true, nil // we end early
	}
	main.Deploy = deploy
	if main.Deploy == nil {
		// nobody activated, but we'll still watch the etcd deploy chan,
		// and if there is deployed code that's ready to run, we'll run!
		log.Printf("main: no frontend selected (no GAPI activated)")
	}

	main.NoWatch = obj.NoWatch
	main.NoStreamWatch = obj.NoStreamWatch
	main.NoDeployWatch = obj.NoDeployWatch

	main.Noop = obj.Noop
	main.Sema = obj.Sema
	main.Graphviz = obj.Graphviz
	main.GraphvizFilter = obj.GraphvizFilter
	main.ConvergedTimeout = obj.ConvergedTimeout
	main.ConvergedTimeoutNoExit = obj.ConvergedTimeoutNoExit
	main.ConvergedStatusFile = obj.ConvergedStatusFile
	main.MaxRuntime = obj.MaxRuntime

	main.Seeds = obj.Seeds
	main.ClientURLs = obj.ClientURLs
	main.ServerURLs = obj.ServerURLs
	main.AdvertiseClientURLs = obj.AdvertiseClientURLs
	main.AdvertiseServerURLs = obj.AdvertiseServerURLs
	main.IdealClusterSize = obj.IdealClusterSize
	main.NoServer = obj.NoServer
	main.NoNetwork = obj.NoNetwork

	main.NoPgp = obj.Prometheus
	main.PgpKeyPath = obj.PgpKeyPath
	main.PgpIdentity = obj.PgpIdentity

	main.Prometheus = obj.Prometheus
	main.PrometheusListen = obj.PrometheusListen

	if err := main.Validate(); err != nil {
		return false, err
	}

	if err := main.Init(); err != nil {
		return false, err
	}

	// install the exit signal handler
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	exit := make(chan struct{})
	defer close(exit)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// must have buffer for max number of signals
		signals := make(chan os.Signal, 3+1) // 3 * ^C + 1 * SIGTERM
		signal.Notify(signals, os.Interrupt) // catch ^C
		//signal.Notify(signals, os.Kill) // catch signals
		signal.Notify(signals, syscall.SIGTERM)
		var count uint8
		for {
			select {
			case sig := <-signals: // any signal will do
				if sig != os.Interrupt {
					log.Printf("interrupted by signal")
					main.Interrupt(fmt.Errorf("killed by %v", sig))
					return
				}

				switch count {
				case 0:
					log.Printf("interrupted by ^C")
					main.Exit(nil)
				case 1:
					log.Printf("interrupted by ^C (fast pause)")
					main.FastExit(nil)
				case 2:
					log.Printf("interrupted by ^C (hard interrupt)")
					main.Interrupt(nil)
				}
				count++

			case <-exit:
				return
			}
		}
	}()

	reterr := main.Run()
	if reterr != nil {
		// log the error message returned
		if main.Flags.Debug {
			log.Printf("main: %+v", reterr)
		}
	}

	if err := main.Close(); err != nil {
		if main.Flags.Debug {
			log.Printf("main: Close: %+v", err)
		}
		if reterr == nil {
			return false, err
		}
		reterr = errwrap.Append(reterr, err)
	}

	if reterr != nil {
		return false, reterr
	}
	return true, nil
}
