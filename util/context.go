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
	"sync"
	"time"
)

// ContextWithCloser wraps a context and returns a new one exactly like the
// other context.With* functions, except that it takes a channel as an alternate
// cancel signal.
// TODO: switch to interface{} instead of <-chan struct to allow any chan type.
func ContextWithCloser(ctx context.Context, ch <-chan struct{}) (context.Context, context.CancelFunc) {
	newCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		select {
		case <-ch: // what we're waiting for
		case <-newCtx.Done(): // parent ctx was canceled
		}
	}()

	return newCtx, cancel
}

// ctxKeyWg is the context value key used by CtxWithWg and WgFromCtx.
type ctxKeyWg struct{}

// CtxWithWg takes a context and a wait group, and returns a new context that is
// embedded with the wait group. You must use WgFromCtx to extract it.
func CtxWithWg(ctx context.Context, wg *sync.WaitGroup) context.Context {
	//if wg == nil {
	//	return ctx // return the parent?
	//}
	key := ctxKeyWg(struct{}{})
	return context.WithValue(ctx, key, wg)
}

// WgFromCtx takes a context and returns the stored wait group. You must use
// CtxWithWg to store it.
func WgFromCtx(ctx context.Context) *sync.WaitGroup {
	key := ctxKeyWg(struct{}{})
	if val := ctx.Value(key); val != nil {
		return val.(*sync.WaitGroup) // panic's if assertion fails
	}
	//return &sync.WaitGroup{} // return a dud?
	return nil
}

// WithPostCancelTimeout returns a context that is cancelled after a timeout
// starting when the parent context is cancelled. If you run the cancel function
// then the returned context is cancelled immediately.
func WithPostCancelTimeout(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		select {
		case <-parent.Done():
			// start the timer
		case <-ctx.Done():
			return // exit early
		}
		select {
		case <-time.After(duration): // doesn't leak in golang 1.23+
			cancel(context.DeadlineExceeded)
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		cancel(context.Canceled)
	}
}
