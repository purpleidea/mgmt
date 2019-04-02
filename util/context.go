// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
	"sync"
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
