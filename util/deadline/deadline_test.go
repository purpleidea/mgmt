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

//go:build !root

// Package deadline_test has examples to test the testing deadline system.
package deadline_test

import (
	"context"
	"testing"
	"time"
)

func TestSlowTest1(t *testing.T) {
	time.Sleep(1 * time.Second)
}

func TestDeadlineTimeout1(t *testing.T) {
	const timeout = 1 * time.Second // how long we'd like to wait for work
	const cleanup = 1 * time.Second // approx time needed to clean up after
	const safety = 1 * time.Second  // extra safety margin, just in case

	// We'd like to wait for our work for `timeout`, but if the test binary
	// is going to give up before that, then we must wake up early enough to
	// still have time to clean up before it panics. Never wait on the test
	// deadline alone, since it might not exist (with -timeout 0) and since
	// it's usually many minutes away, which would stall the whole package.
	now := time.Now()
	d := now.Add(timeout)
	if deadline, ok := t.Deadline(); ok {
		if x := deadline.Add(-(cleanup + safety)); x.Before(d) {
			d = x
		}
	}
	t.Logf("  now: %+v", now)
	t.Logf("    d: %+v", d)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()

	work := make(chan struct{}) // pretend this is some long-running work

	select {
	case <-work:
		t.Errorf("work finished unexpectedly")
		return

	case <-ctx.Done():
		t.Logf("  ctx: %+v", time.Now())
		time.Sleep(cleanup) // pretend we're cleaning up after ourselves
		t.Logf("sleep: %+v", time.Now())
	}

	if deadline, ok := t.Deadline(); ok && !time.Now().Before(deadline) {
		t.Errorf("test ran past its deadline")
	}
}
