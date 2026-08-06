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

// Package signals has utility functions for exiting when we're signalled.
package signals

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Rung is one step of a Ladder. The Func runs at most once, and only if every
// rung above it has already run.
type Rung struct {
	// Name is an optional short description of what this rung does, such as
	// "interrupt". It's added to the message which is logged when we climb
	// onto this rung. The topmost rung usually doesn't need one, since the
	// reason for climbing says enough on its own.
	Name string

	// Func is what we run when we climb onto this rung. It may be nil if
	// this rung only exists to log something.
	Func func()
}

// Ladder runs a sequence of functions, one per signal received. This let's us
// handle the escalation path of a user pressing ^C multiple times.
//
// Each rung runs at most once, and something has to ask for the next one: a
// signal climbs exactly one rung. Once we're on the bottom rung, further
// signals do nothing, since there's nothing left to escalate to.
//
// A service manager like systemd is the reason we have GraceTimeout. Unlike a
// user at a terminal, it signals exactly once and then waits on a stopwatch of
// its own before it sends an unstoppable SIGKILL, so it will never ask for the
// next rung, and we'd be killed mid-operation instead of exiting cleanly. When
// the first signal isn't an interrupt, a timer climbs the remaining rungs on
// its behalf.
type Ladder struct {
	// Signals is the list of signals to catch. If it's empty, then we catch
	// ^C and SIGTERM.
	Signals []os.Signal

	// Rungs are the steps of the ladder, in the order that we climb them. A
	// nil entry is an empty rung: climbing onto it logs the reason for the
	// climb, and does nothing else.
	Rungs []*Rung

	// GraceTimeout is how long each rung gets when we were signalled by
	// something which won't ever signal us again. If it's zero, then we
	// never climb on our own, and only a signal moves us.
	GraceTimeout time.Duration

	// Logf is used to log what we're doing and why. It may be nil.
	Logf func(format string, v ...interface{})
}

// Start installs the signal handler and returns immediately. It returns a stop
// function which uninstalls it and waits for it to finish. That function is
// safe to call more than once, and it's usually deferred by the caller.
//
// The handler has to outlive whatever it's shutting down. In particular, if a
// rung cancels a context, then don't stop the handler when that context is
// done, or the first signal would end it and none of the rungs below could ever
// run. Stop it when the work you're exiting from has actually returned.
func (obj *Ladder) Start() func() {
	sigs := obj.Signals // don't shadow the package name
	if len(sigs) == 0 {
		sigs = []os.Signal{
			os.Interrupt, // catch ^C
			syscall.SIGTERM,
			//os.Kill, // can't be caught
		}
	}
	logf := obj.Logf
	if logf == nil {
		logf = func(format string, v ...interface{}) {} // no-op
	}

	// must have buffer for max number of signals
	ch := make(chan os.Signal, len(obj.Rungs)+1)
	signal.Notify(ch, sigs...)

	wg := &sync.WaitGroup{}
	exit := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer signal.Stop(ch)
		var count int

		// escalate climbs one rung of the ladder, logging the given
		// reason for it, and it reports whether any rungs are left
		// below.
		escalate := func(reason string) bool {
			if count >= len(obj.Rungs) {
				return false // we're already at the bottom
			}
			rung := obj.Rungs[count]
			if rung == nil {
				rung = &Rung{} // no name, and nothing to run
			}
			if rung.Name != "" {
				reason += fmt.Sprintf(" (%s)", rung.Name)
			}
			logf("%s", reason)
			if rung.Func != nil {
				rung.Func()
			}
			count++
			return count < len(obj.Rungs)
		}

		var graceChan <-chan time.Time // nil until a signal arms it
		for {
			select {
			case sig := <-ch: // any signal will do
				// Every signal starts at the top of the ladder,
				// including the ones which a service manager
				// sends. A SIGTERM is the polite "please stop"
				// which comes before it escalates to a SIGKILL
				// of its own, so going straight to the bottom
				// rung would throw away in-flight work that we
				// were given the time to finish.
				first := count == 0
				more := escalate(fmt.Sprintf("interrupted by %s", sigName(sig)))

				// Unlike a user, a service manager signals
				// exactly once and then waits on a stopwatch of
				// its own before it kills us, so it will never
				// ask for the next rung. Start the timer which
				// does that on its behalf.
				if first && more && sig != os.Interrupt && obj.GraceTimeout > 0 {
					graceChan = time.After(obj.GraceTimeout)
				}

			case <-graceChan: // only ever armed by a signal
				graceChan = nil // re-armed below if there's more
				if escalate(fmt.Sprintf("still exiting after %s", obj.GraceTimeout)) {
					graceChan = time.After(obj.GraceTimeout)
				}

			case <-exit: // we were told to stop listening
				return
			}
		}
	}()

	once := &sync.Once{}
	return func() {
		once.Do(func() { close(exit) })
		wg.Wait()
	}
}

// sigName returns a short, readable name for the given signal, for use in a
// message such as "interrupted by X". The String method of a signal gives a
// lowercase description rather than a name, so the two we almost always get are
// spelled out here, eg: "interrupted by SIGTERM".
func sigName(sig os.Signal) string {
	switch sig {
	case os.Interrupt:
		return "^C"
	case syscall.SIGTERM:
		return "SIGTERM"
	}
	return fmt.Sprintf("a %v signal", sig) // eg: "a hangup signal"
}
