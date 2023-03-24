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

// Package deadline_test has examples to test the testing deadline system.
package deadline_test

import (
	"context"
	"testing"
	"time"
)

func TestSlowTest1(t *testing.T) {
	time.Sleep(3 * time.Second)
}

func TestDeadlineTimeout1(t *testing.T) {
	now := time.Now()
	min := time.Second * 3 // approx min time needed for the test
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		d := deadline.Add(-min)
		t.Logf("  now: %+v", now)
		t.Logf("    d: %+v", d)
		newCtx, cancel := context.WithDeadline(ctx, d)
		ctx = newCtx
		defer cancel()
	}

	select {
	case <-ctx.Done():
		t.Logf("  ctx: %+v", time.Now())
		time.Sleep(min - (1 * time.Second)) // one second less for safety
		t.Logf("sleep: %+v", time.Now())
	}
}
