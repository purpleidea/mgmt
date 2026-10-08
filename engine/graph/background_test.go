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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
)

const (
	backgroundErrorKind = "graph-test-background-error"
	backgroundNilKind   = "graph-test-background-nil"
)

var errBackgroundTest = errors.New("background test error")

func init() {
	engine.RegisterResource(backgroundErrorKind, func() engine.Res { return &backgroundErrorRes{} })
	engine.RegisterResource(backgroundNilKind, func() engine.Res { return &backgroundNilRes{} })
}

// backgroundErrorRes has a background function which errors before it's ready.
type backgroundErrorRes struct {
	resources.NoopRes
}

// Background errors without ever sending the ready signal.
func (obj *backgroundErrorRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		return errBackgroundTest
	}
}

// backgroundNilRes has a background function which exits before it's ready.
type backgroundNilRes struct {
	resources.NoopRes
}

// Background exits without an error, and without sending the ready signal.
func (obj *backgroundNilRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		return nil
	}
}

// TestStartBackgroundEarlyExit checks that a background function which exits
// before it's ready makes StartBackground return right away, with its error.
func TestStartBackgroundEarlyExit(t *testing.T) {
	tests := []struct {
		kind   string
		cancel error // what the engine Cancel func should receive
	}{
		{backgroundErrorKind, errBackgroundTest},
		{backgroundNilKind, nil},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			mutex := &sync.Mutex{}
			var cause error
			obj := &Engine{
				Cancel: func(err error) {
					mutex.Lock()
					defer mutex.Unlock()
					cause = err
				},
				Logf:    t.Logf,
				bgState: make(map[string]*bgState),
			}

			// This ctx is never cancelled by the test, so a hang
			// here would mean that we only noticed the exit on a
			// timeout.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := obj.StartBackground(ctx, tc.kind)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if ctx.Err() != nil {
				t.Fatalf("we timed out instead: %v", err)
			}
			if tc.cancel != nil && !errors.Is(err, tc.cancel) {
				t.Errorf("unexpected error: %v", err)
			}
			if n := len(obj.bgState); n != 0 {
				t.Errorf("expected no background state, got %d", n)
			}

			mutex.Lock()
			defer mutex.Unlock()
			if cause != tc.cancel {
				t.Errorf("unexpected engine cancel: %v", cause)
			}
		})
	}
}
