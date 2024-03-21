// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package cli

import (
	"context"
	"fmt"
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
)

// RunArgs is the CLI parsing structure and type of the parsed result. This
// particular one contains all the common flags for the `run` subcommand which
// all frontends can use.
type RunArgs struct {
	lib.Config // embedded config (can't be a pointer) https://github.com/alexflint/go-arg/issues/240

	RunEmpty      *cliUtil.EmptyArgs      `arg:"subcommand:empty" help:"run empty payload"`
	RunLang       *cliUtil.LangArgs       `arg:"subcommand:lang" help:"run lang (mcl) payload"`
	RunYaml       *cliUtil.YamlArgs       `arg:"subcommand:yaml" help:"run yaml graph payload"`
	RunPuppet     *cliUtil.PuppetArgs     `arg:"subcommand:puppet" help:"run puppet graph payload"`
	RunLangPuppet *cliUtil.LangPuppetArgs `arg:"subcommand:langpuppet" help:"run a combined lang/puppet graph payload"`
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
	if cmd := obj.RunPuppet; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "puppet"
		args = cmd
	}
	if cmd := obj.RunLangPuppet; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "langpuppet"
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
	main.Config = &obj.Config // pass in all the parsed data

	main.Program, main.Version = data.Program, data.Version
	main.Debug, main.Logf = data.Flags.Debug, data.Flags.Logf // no prefix
	Logf := func(format string, v ...interface{}) {
		data.Flags.Logf("main: "+format, v...)
	}

	cliUtil.Hello(main.Program, main.Version, data.Flags) // say hello!
	defer Logf("goodbye!")

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
		Debug: data.Flags.Debug,
		Logf: func(format string, v ...interface{}) {
			data.Flags.Logf("cli: "+format, v...)
		},
	}

	deploy, err := gapiObj.Cli(info)
	if err != nil {
		return false, cliUtil.CliParseError(err) // TODO: it seems unlikely that parsing the CLI failed at this stage, and then the error will be misleading
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
		data.Flags.Logf("main: no frontend selected (no GAPI activated)")
	}

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
					data.Flags.Logf("interrupted by signal")
					main.Interrupt(fmt.Errorf("killed by %v", sig))
					return
				}

				switch count {
				case 0:
					data.Flags.Logf("interrupted by ^C")
					main.Exit(nil)
				case 1:
					data.Flags.Logf("interrupted by ^C (fast pause)")
					main.FastExit(nil)
				case 2:
					data.Flags.Logf("interrupted by ^C (hard interrupt)")
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
		if data.Flags.Debug {
			data.Flags.Logf("main: %+v", reterr)
		}
	}

	if err := main.Close(); err != nil {
		if data.Flags.Debug {
			data.Flags.Logf("main: Close: %+v", err)
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
