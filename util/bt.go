// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

package util

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BlockedTimer is a helper facility for printing log messages when you have a
// possible deadlock. Alternatively, it can run an arbitrary function instead.
// It does this by starting a timer, and if that timer isn't cancelled soon
// enough, it executes the task. This is usually most useful before a select
// statement which should ordinarily unblock rather quickly. It's helpful to
// avoid unnecessary "waiting for select" log messages being constantly printed,
// but should those block for longer, you'd definitely like to know where to
// look first. This is safe for concurrent use. Multiple invocations of Printf
// or Run are permitted. They each have their own separate countdown timers, but
// are all cancelled when Cancel is run. It is safe to call Cancel multiple
// times. If Cancel is called before Printf or Run, then those will never run. A
// BlockedTimer must not be copied after first use.
type BlockedTimer struct {
	// Duration specifies how long we should wait before we run (or print)
	// the function or message. The counter starts when that respective
	// function is run. For an easier method, specify the Seconds parameter
	// instead.
	Duration time.Duration

	// Seconds works exactly as Duration does, except it can be used as a
	// shorter method to accomplish the same thing. If this value is zero,
	// then Duration is used instead.
	Seconds int

	ctx       context.Context
	cancel    func()
	cancelled bool
	mutex     sync.Mutex
}

// Printf will print as expected when the timer expires if Cancel isn't run
// first. This can be used multiple times.
func (obj *BlockedTimer) Printf(format string, v ...interface{}) {
	f := func() {
		// safe Logf in case f.String contains %? chars...
		s := fmt.Sprintf(format, v...)
		fmt.Printf("%s", s)
	}
	obj.Run(f)
}

// Run will run the passed function as expected when the timer expires if Cancel
// isn't run first. This can be used multiple times.
func (obj *BlockedTimer) Run(f func()) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.cancelled { // we already cancelled
		return
	}

	if obj.cancel == nil { // only do it once
		obj.ctx, obj.cancel = context.WithCancel(context.Background())
	}

	d := time.Duration(obj.Seconds) * time.Second
	if obj.Seconds == 0 {
		d = obj.Duration
	}

	go func() {
		select {
		case <-time.After(d):
			// print!
		case <-obj.ctx.Done():
			// cancel the print
			return
		}
		f() // run it
	}()
}

// Cancel cancels the execution of any Run or Printf functions. It is safe to
// call it multiple times. It is important to call this at least once (on defer
// for example) if you've used either Printf or Run, because otherwise you will
// leak goroutines.
func (obj *BlockedTimer) Cancel() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	obj.cancelled = true
	if obj.cancel == nil { // race! never let it run
		return
	}
	obj.cancel()
}
