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
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/urfave/cli"
)

// run is the main run target.
func run(c *cli.Context, name string, gapiObj gapi.GAPI) error {
	cliContext := c.Parent() // these are the flags from `run`
	if cliContext == nil {
		return fmt.Errorf("could not get cli context")
	}

	obj := &Main{}

	obj.Program, obj.Version = safeProgram(c.App.Name), c.App.Version
	if val, exists := c.App.Metadata["flags"]; exists {
		if flags, ok := val.(Flags); ok {
			obj.Flags = flags
		}
	}

	if h := cliContext.String("hostname"); cliContext.IsSet("hostname") && h != "" {
		obj.Hostname = &h
	}

	if s := cliContext.String("prefix"); cliContext.IsSet("prefix") && s != "" {
		obj.Prefix = &s
	}
	obj.TmpPrefix = cliContext.Bool("tmp-prefix")
	obj.AllowTmpPrefix = cliContext.Bool("allow-tmp-prefix")

	// create a memory backed temporary filesystem for storing runtime data
	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
	standaloneFs := &util.Fs{Afero: afs}
	obj.DeployFs = standaloneFs

	cliInfo := &gapi.CliInfo{
		CliContext: c, // don't pass in the parent context

		Fs:    standaloneFs,
		Debug: obj.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("cli: "+format, v...)
		},
	}

	deploy, err := gapiObj.Cli(cliInfo)
	if err != nil {
		return errwrap.Wrapf(err, "cli parse error")
	}
	obj.Deploy = deploy
	if obj.Deploy == nil {
		// nobody activated, but we'll still watch the etcd deploy chan,
		// and if there is deployed code that's ready to run, we'll run!
		log.Printf("main: no frontend selected (no GAPI activated)")
	}

	obj.NoWatch = cliContext.Bool("no-watch")
	obj.NoConfigWatch = cliContext.Bool("no-config-watch")
	obj.NoStreamWatch = cliContext.Bool("no-stream-watch")
	obj.NoDeployWatch = cliContext.Bool("no-deploy-watch")

	obj.Noop = cliContext.Bool("noop")
	obj.Sema = cliContext.Int("sema")
	obj.Graphviz = cliContext.String("graphviz")
	obj.GraphvizFilter = cliContext.String("graphviz-filter")
	obj.ConvergedTimeout = cliContext.Int("converged-timeout")
	obj.ConvergedTimeoutNoExit = cliContext.Bool("converged-timeout-no-exit")
	obj.ConvergedStatusFile = cliContext.String("converged-status-file")
	obj.MaxRuntime = uint(cliContext.Int("max-runtime"))

	obj.Seeds = cliContext.StringSlice("seeds")
	obj.ClientURLs = cliContext.StringSlice("client-urls")
	obj.ServerURLs = cliContext.StringSlice("server-urls")
	obj.AdvertiseClientURLs = cliContext.StringSlice("advertise-client-urls")
	obj.AdvertiseServerURLs = cliContext.StringSlice("advertise-server-urls")
	obj.IdealClusterSize = cliContext.Int("ideal-cluster-size")
	obj.NoServer = cliContext.Bool("no-server")
	obj.NoNetwork = cliContext.Bool("no-network")

	obj.NoPgp = cliContext.Bool("no-pgp")

	if kp := cliContext.String("pgp-key-path"); cliContext.IsSet("pgp-key-path") {
		obj.PgpKeyPath = &kp
	}

	if us := cliContext.String("pgp-identity"); cliContext.IsSet("pgp-identity") {
		obj.PgpIdentity = &us
	}

	obj.Prometheus = cliContext.Bool("prometheus")
	obj.PrometheusListen = cliContext.String("prometheus-listen")

	if err := obj.Validate(); err != nil {
		return err
	}

	if err := obj.Init(); err != nil {
		return err
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
					obj.Interrupt(fmt.Errorf("killed by %v", sig))
					return
				}

				switch count {
				case 0:
					log.Printf("interrupted by ^C")
					obj.Exit(nil)
				case 1:
					log.Printf("interrupted by ^C (fast pause)")
					obj.FastExit(nil)
				case 2:
					log.Printf("interrupted by ^C (hard interrupt)")
					obj.Interrupt(nil)
				}
				count++

			case <-exit:
				return
			}
		}
	}()

	reterr := obj.Run()
	if reterr != nil {
		// log the error message returned
		log.Printf("main: Error: %v", reterr)
	}

	if err := obj.Close(); err != nil {
		log.Printf("main: Close: %v", err)
		if reterr == nil {
			return err
		}
	}

	return reterr
}
