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

package util

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// WatchdogMin is the lower limit that the watchdog timer will alarm at.
	WatchdogMin = 1 * time.Second

	// WatchdogMax is the max limit, beyond which it will not warn any more.
	WatchdogMax = 4 * time.Second
)

// WatchdogLogf runs a logf logging function when various watchdog events occur.
// The start timer and maximum timer are defined by constants. You must cancel
// the watchdog in time with the returned function to avoid firing the log func.
func WatchdogLogf(logf func(format string, v ...interface{})) func() {
	ctx, cancel := context.WithCancel(context.Background())
	d := WatchdogMin // start
	timer := time.NewTimer(d)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if d == WatchdogMin { // it never ran a logf
				return
			}
			logf("watchdog: exited")
		}()
		defer cancel()
		defer timer.Stop() // ensures cleanup when goroutine exits
		for {
			select {
			case <-timer.C:
				logf("watchdog: blocked for %d seconds", int(d.Seconds()))
				if d < WatchdogMax {
					d *= 2
					timer.Reset(d)
				}
				continue

			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		defer wg.Wait() // wait for exited message to be printed
		cancel()
	}
}

// WatchdogFn runs a fn with a status string when various watchdog events occur.
// The start timer and maximum timer are defined by constants. You must cancel
// the watchdog in time with the returned function to avoid firing the function.
func WatchdogFn(fn func(string)) func() {
	ctx, cancel := context.WithCancel(context.Background())
	d := WatchdogMin // start
	timer := time.NewTimer(d)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if d == WatchdogMin { // it never ran a logf
				return
			}
			fn("watchdog: exited")
		}()
		defer cancel()
		defer timer.Stop() // ensures cleanup when goroutine exits
		for {
			select {
			case <-timer.C:
				fn(fmt.Sprintf("watchdog: blocked for %d seconds", int(d.Seconds())))
				if d < WatchdogMax {
					d *= 2
					timer.Reset(d)
				}
				continue

			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		defer wg.Wait() // wait for exited message to be printed
		cancel()
	}
}
