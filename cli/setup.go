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
	"os"
	"os/signal"
	"sync"
	"syscall"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/setup"
)

// SetupArgs is the CLI parsing structure and type of the parsed result. This
// particular one contains all the common flags for the `setup` subcommand.
type SetupArgs struct {
	setup.Config // embedded config (can't be a pointer) https://github.com/alexflint/go-arg/issues/240

	SetupPkg       *cliUtil.SetupPkgArgs       `arg:"subcommand:pkg" help:"setup packages"`
	SetupSvc       *cliUtil.SetupSvcArgs       `arg:"subcommand:svc" help:"setup services"`
	SetupFirstboot *cliUtil.SetupFirstbootArgs `arg:"subcommand:firstboot" help:"setup firstboot"`
}

// Run executes the correct subcommand. It errors if there's ever an error. It
// returns true if we did activate one of the subcommands. It returns false if
// we did not. This information is used so that the top-level parser can return
// usage or help information if no subcommand activates. This particular Run is
// the run for the main `setup` subcommand. The setup command does some
// bootstrap work to help get things going.
func (obj *SetupArgs) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var name string
	var args interface{}
	if cmd := obj.SetupPkg; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "pkg"
		args = cmd
	}
	if cmd := obj.SetupSvc; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "svc"
		args = cmd
	}
	if cmd := obj.SetupFirstboot; cmd != nil {
		name = cliUtil.LookupSubcommand(obj, cmd) // "firstboot"
		args = cmd
	}
	_ = name

	Logf := func(format string, v ...interface{}) {
		// Don't block this globally...
		//if !data.Flags.Debug {
		//	return
		//}
		data.Flags.Logf("main: "+format, v...)
	}

	var api setup.API

	if cmd := obj.SetupPkg; cmd != nil {
		api = &setup.Pkg{
			SetupPkgArgs: args.(*cliUtil.SetupPkgArgs),
			Config:       obj.Config,
			Program:      data.Program,
			Version:      data.Version,
			Debug:        data.Flags.Debug,
			Logf:         Logf,
		}
	}
	if cmd := obj.SetupSvc; cmd != nil {
		api = &setup.Svc{
			SetupSvcArgs: args.(*cliUtil.SetupSvcArgs),
			Config:       obj.Config,
			Program:      data.Program,
			Version:      data.Version,
			Debug:        data.Flags.Debug,
			Logf:         Logf,
		}
	}
	if cmd := obj.SetupFirstboot; cmd != nil {
		api = &setup.Firstboot{
			SetupFirstbootArgs: args.(*cliUtil.SetupFirstbootArgs),
			Config:             obj.Config,
			Program:            data.Program,
			Version:            data.Version,
			Debug:              data.Flags.Debug,
			Logf:               Logf,
		}
	}

	if api == nil {
		return false, nil // nothing found (display help!)
	}

	// We don't use these for the setup command in normal operation.
	if data.Flags.Debug {
		cliUtil.Hello(data.Program, data.Version, data.Flags) // say hello!
		defer Logf("goodbye!")
	}

	// install the exit signal handler
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	exit := make(chan struct{})
	defer close(exit)
	wg.Add(1)
	go func() {
		defer cancel()
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
					return
				}

				switch count {
				case 0:
					data.Flags.Logf("interrupted by ^C")
					cancel()
				case 1:
					data.Flags.Logf("interrupted by ^C (fast pause)")
					cancel()
				case 2:
					data.Flags.Logf("interrupted by ^C (hard interrupt)")
					cancel()
				}
				count++

			case <-exit:
				return
			}
		}
	}()

	if err := api.Main(ctx); err != nil {
		if data.Flags.Debug {
			data.Flags.Logf("main: %+v", err)
		}
		return false, err
	}

	return true, nil
}
