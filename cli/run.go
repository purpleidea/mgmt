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
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/lib"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/spf13/afero"
	"github.com/urfave/cli/v2"
)

// run is the main run target.
func run(c *cli.Context, name string, gapiObj gapi.GAPI) error {
	cliContext := c.Lineage()[1] // these are the flags from `run`
	if cliContext == nil {
		return fmt.Errorf("could not get cli context")
	}

	main := &lib.Main{}

	main.Program, main.Version = cliUtil.SafeProgram(c.App.Name), c.App.Version
	var flags cliUtil.Flags
	if val, exists := c.App.Metadata["flags"]; exists {
		if f, ok := val.(cliUtil.Flags); ok {
			flags = f
			main.Flags = lib.Flags{
				Debug:   f.Debug,
				Verbose: f.Verbose,
			}
		}
	}
	Logf := func(format string, v ...interface{}) {
		log.Printf("main: "+format, v...)
	}

	cliUtil.Hello(main.Program, main.Version, flags) // say hello!
	defer Logf("goodbye!")

	if h := cliContext.String("hostname"); cliContext.IsSet("hostname") && h != "" {
		main.Hostname = &h
	}

	if s := cliContext.String("prefix"); cliContext.IsSet("prefix") && s != "" {
		main.Prefix = &s
	}
	main.TmpPrefix = cliContext.Bool("tmp-prefix")
	main.AllowTmpPrefix = cliContext.Bool("allow-tmp-prefix")

	// create a memory backed temporary filesystem for storing runtime data
	mmFs := afero.NewMemMapFs()
	afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
	standaloneFs := &util.AferoFs{Afero: afs}
	main.DeployFs = standaloneFs

	cliInfo := &gapi.CliInfo{
		CliContext: c, // don't pass in the parent context

		Fs:    standaloneFs,
		Debug: main.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			log.Printf("cli: "+format, v...)
		},
	}

	deploy, err := gapiObj.Cli(cliInfo)
	if err != nil {
		return errwrap.Wrapf(err, "cli parse error")
	}
	if c.Bool("only-unify") && deploy == nil {
		return nil // we end early
	}
	main.Deploy = deploy
	if main.Deploy == nil {
		// nobody activated, but we'll still watch the etcd deploy chan,
		// and if there is deployed code that's ready to run, we'll run!
		log.Printf("main: no frontend selected (no GAPI activated)")
	}

	main.NoWatch = cliContext.Bool("no-watch")
	main.NoStreamWatch = cliContext.Bool("no-stream-watch")
	main.NoDeployWatch = cliContext.Bool("no-deploy-watch")

	main.Noop = cliContext.Bool("noop")
	main.Sema = cliContext.Int("sema")
	main.Graphviz = cliContext.String("graphviz")
	main.GraphvizFilter = cliContext.String("graphviz-filter")
	main.ConvergedTimeout = cliContext.Int64("converged-timeout")
	main.ConvergedTimeoutNoExit = cliContext.Bool("converged-timeout-no-exit")
	main.ConvergedStatusFile = cliContext.String("converged-status-file")
	main.MaxRuntime = uint(cliContext.Int("max-runtime"))

	main.Seeds = cliContext.StringSlice("seeds")
	main.ClientURLs = cliContext.StringSlice("client-urls")
	main.ServerURLs = cliContext.StringSlice("server-urls")
	main.AdvertiseClientURLs = cliContext.StringSlice("advertise-client-urls")
	main.AdvertiseServerURLs = cliContext.StringSlice("advertise-server-urls")
	main.IdealClusterSize = cliContext.Int("ideal-cluster-size")
	main.NoServer = cliContext.Bool("no-server")
	main.NoNetwork = cliContext.Bool("no-network")

	main.NoPgp = cliContext.Bool("no-pgp")

	if kp := cliContext.String("pgp-key-path"); cliContext.IsSet("pgp-key-path") {
		main.PgpKeyPath = &kp
	}

	if us := cliContext.String("pgp-identity"); cliContext.IsSet("pgp-identity") {
		main.PgpIdentity = &us
	}

	main.Prometheus = cliContext.Bool("prometheus")
	main.PrometheusListen = cliContext.String("prometheus-listen")

	if err := main.Validate(); err != nil {
		return err
	}

	if err := main.Init(); err != nil {
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
			return err
		}
		reterr = errwrap.Append(reterr, err)
	}

	return reterr
}
