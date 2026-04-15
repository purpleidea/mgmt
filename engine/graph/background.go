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

package graph

import (
	"context"
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/util"
)

// StartBackground starts up the background function for this kind, if one isn't
// already running. If you cancel this context, it's because you want to abort
// the startup and exit everything early. That function may error after startup.
// This function isn't thread-safe, because it's currently only called linearly
// from inside the Commit function, which uses it sequentially.
func (obj *Engine) StartBackground(ctx context.Context, kind string) error {
	if _, exists := obj.bgState[kind]; exists {
		return nil // already running
	}

	background, err := engine.LookupBackgroundFunc(kind)
	if err != nil {
		return err
	}

	var reterr error
	wg := &sync.WaitGroup{} // wg.Wait() in StopBackground
	// This bgCtx should not depend on the incoming ctx for this function.
	// This is for the lifecycle of the background worker, and the ctx of
	// this function is to let this start process to exit sooner if asked.
	bgCtx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	//exit := make(chan struct{})
	state := &bgState{
		wg:     wg,
		ctx:    bgCtx,
		cancel: cancel,
		//err: nil, // err is set after background func exits
	}
	obj.bgState[kind] = state
	handle := &engine.BackgroundHandle{
		Local: obj.Local,
		World: obj.World,
		Debug: obj.Debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			obj.Logf(fmt.Sprintf("background(%s): ", kind)+format, v...)
		},
	}
	backgroundFunc := background(handle) // build the real background function

	// TODO: Should we add in a global waitgroup for the engine?
	wg.Add(1)
	go func() {
		defer wg.Done()
		//defer close(exit)
		defer cancel() // make sure to free the memory on early exit
		reterr = backgroundFunc(bgCtx, ready)
		if reterr == context.Canceled { // ignore these, we asked for it
			reterr = nil
		}
		state.err = err // read by StopBackground after the wg.Wait()

		if reterr != nil {
			// Run a shutdown of the main graph engine, we're broken!
			obj.Cancel(reterr) // trigger an exit!
		}
	}()

	// TODO: should we have a startup timeout here too?
	select {
	case <-ready:
		return nil
	//case <-exit: // exited early before ready
	//	return reterr
	case <-ctx.Done(): // exited early before ready, so cleanup early
		cancel()  // main ctx died, something is wrong, kill bgCtx
		wg.Wait() // wait for the background goroutine to finish
		delete(obj.bgState, kind)
		if err := state.err; err != nil {
			return err
		}
		return ctx.Err()
	}
}

// StopBackground stops the background function for this kind if one is running.
// This doesn't take a context, because it's a shutdown procedure, and
// cancelling this kind of scenario is something we'd do on shutdown anyways...
// This function isn't thread-safe, because it's currently only called linearly
// from inside the Commit function, which uses it sequentially.
func (obj *Engine) StopBackground(kind string) error {
	bgState, exists := obj.bgState[kind]
	if !exists {
		return nil // already stopped
	}

	bgState.cancel()

	// add a watchdog to catch slow exiting or blocked background functions
	watchdogFn := func(msg string) {
		obj.Logf("background: %s: %s", kind, msg)
	}
	cancel := util.WatchdogFn(watchdogFn)
	bgState.wg.Wait()
	cancel()

	delete(obj.bgState, kind)
	return bgState.err // read only after waitgroup
}

// bgState holds state information for each running background function. The err
// is what is returned after the background function exits, and may only be read
// after the wait group wait expires.
type bgState struct {
	wg     *sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	err    error
}
