// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

//go:build !root

package util

import (
	"context"
	"sync"
	"testing"
)

func TestContextWithCloser1(t *testing.T) {
	ch := make(chan struct{})
	close(ch) // cancel it here!
	ctx, cancel := ContextWithCloser(context.Background(), ch)
	defer cancel()

	// can't use a default case, because it would race...
	select {
	case <-ctx.Done(): // if this deadlocks, then we fail
	}
}

func TestContextWithCloser2(t *testing.T) {
	ch := make(chan struct{})
	ctx, cancel := ContextWithCloser(context.Background(), ch)
	defer cancel()

	select {
	case <-ctx.Done(): // this should NOT be cancelled!
		t.Errorf("should not be closed")
	default:
	}
}

func TestWgCtx1(t *testing.T) {
	wg := &sync.WaitGroup{}
	ctx := CtxWithWg(context.Background(), wg)
	wg.Add(1)
	WgFromCtx(ctx).Done()
	wg.Wait() // if this deadlocks, then we fail
}
