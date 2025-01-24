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

// Package pprof is a simple wrapper around the pprof utility code which we use.
package pprof

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
)

// Run looks in a special environment var for the path to log pprof data to and
// if it finds it, it begins profiling. If it doesn't find a special environment
// var, it returns nil. If this is not able to start logging, then it errors. If
// it starts logging, this waits for an exit signal in a goroutine and returns
// nil. The magic env name is MGMT_PPROF_PATH. Example usage:
// MGMT_PPROF_PATH="~/pprof/out.pprof" ./mgmt run lang examples/lang/hello.mcl
// go tool pprof -no_browser -http :10000 ~/pprof/out.pprof
func Run(ctx context.Context) error {
	s := os.Getenv("MGMT_PPROF_PATH")
	logf := func(format string, v ...interface{}) {
		// TODO: is this a sane prefix to use here?
		log.Printf(format, v...) // XXX: use parent logger when available
	}

	if s == "" || !strings.HasPrefix(s, "/") {
		return nil // not activated
	}
	logf("pprof logging to: %s", s)

	f, err := os.Create(s)
	if err != nil {
		return fmt.Errorf("could not create CPU profile: %v", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		return fmt.Errorf("could not start CPU profile: %v", err)
	}

	signals := make(chan os.Signal, 1+1) // 1 * ^C + 1 * SIGTERM
	signal.Notify(signals, os.Interrupt) // catch ^C
	//signal.Notify(signals, os.Kill) // catch signals
	signal.Notify(signals, syscall.SIGTERM)

	go func() {
		defer func() {
			if err := f.Close(); err != nil {
				logf("pprof write error: %v", err)
				return
			}
			logf("pprof wrote file to: %s", s)
		}()
		defer pprof.StopCPUProfile()
		select {
		case sig := <-signals: // any signal will do
			logf("pprof logging shutdown by sig: %v", sig)

		case <-ctx.Done():
			logf("pprof logging shutdown by ctx: %v", ctx.Err())
		}
	}()

	return nil
}
