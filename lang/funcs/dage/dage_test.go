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

package dage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// shutdownCases are what a func might return once we cancel it. Only the
// cancellation itself, even if it's wrapped, is a clean shutdown. Anything else
// is a real error, which we must still see, even though we're shutting down.
var shutdownCases = map[string]struct {
	err  func(context.Context) error
	fail bool
}{
	"bare": {
		err: func(ctx context.Context) error {
			return ctx.Err()
		},
	},
	"wrapped": {
		err: func(ctx context.Context) error {
			return errwrap.Wrapf(ctx.Err(), "stopped")
		},
	},
	"other": {
		err: func(ctx context.Context) error {
			return fmt.Errorf("unexpected close")
		},
		fail: true,
	},
}

func TestRunCallShutdown(t *testing.T) {
	for name, tc := range shutdownCases {
		t.Run(name, func(t *testing.T) {
			fn := &blockingContextFunc{
				called: make(chan struct{}),
				err:    tc.err,
			}
			fn.Locate(0, 0, 0, 1)
			testRunShutdown(t, fn, fn.called, tc.fail)
		})
	}
}

func TestRunStreamShutdown(t *testing.T) {
	for name, tc := range shutdownCases {
		t.Run(name, func(t *testing.T) {
			fn := &shutdownStreamFunc{
				started: make(chan struct{}),
				err:     tc.err,
			}
			fn.Locate(0, 0, 0, 1)
			testRunShutdown(t, fn, fn.started, tc.fail)
		})
	}
}

// testRunShutdown runs an engine with this one function, and shuts it down once
// the ready channel closes. A clean shutdown must look like context.Canceled,
// even if the function wrapped that error. If fail is true, the function
// returns a real error, and we must see it.
func testRunShutdown(t *testing.T, fn interfaces.Func, ready <-chan struct{}, fail bool) {
	t.Helper()

	engine := &Engine{
		Name: "test",
		Logf: t.Logf,
	}
	if err := engine.Setup(); err != nil {
		t.Fatalf("setup failed: %+v", err)
	}

	txn := engine.Txn()
	defer txn.Free()
	txn.AddVertex(fn)
	if err := txn.Commit(); err != nil {
		t.Fatalf("commit failed: %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- engine.Run(ctx)
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatalf("function was not ready")
	}

	cancel()

	select {
	case err := <-runErr:
		// A Stream error doesn't go through Run, so it's in Err.
		if !fail && err != context.Canceled {
			t.Fatalf("expected context.Canceled from Run, got: %+v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("run did not stop")
	}

	// This is what lang.Run reports, so it must show us any real error.
	err := errwrap.WithoutContext(engine.Err())
	if !fail && err != context.Canceled {
		t.Fatalf("expected context.Canceled from Err, got: %+v", err)
	}
	if fail && (err == nil || !strings.Contains(err.Error(), "unexpected close")) {
		t.Fatalf("expected the real error from Err, got: %+v", err)
	}
}

type blockingContextFunc struct {
	interfaces.Textarea

	called chan struct{}
	once   sync.Once
	err    func(context.Context) error // what Call returns once cancelled
}

func (obj *blockingContextFunc) String() string {
	return "blockingContext"
}

func (obj *blockingContextFunc) Validate() error {
	return nil
}

func (obj *blockingContextFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() str"),
	}
}

func (obj *blockingContextFunc) Init(init *interfaces.Init) error {
	return nil
}

func (obj *blockingContextFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	obj.once.Do(func() {
		close(obj.called)
	})

	<-ctx.Done()
	return nil, obj.err(ctx)
}

// shutdownStreamFunc blocks in Stream until it's cancelled, and then returns
// whatever error it was given.
type shutdownStreamFunc struct {
	interfaces.Textarea

	started chan struct{}
	err     func(context.Context) error // what Stream returns once cancelled
}

func (obj *shutdownStreamFunc) String() string {
	return "shutdownStream"
}

func (obj *shutdownStreamFunc) Validate() error {
	return nil
}

func (obj *shutdownStreamFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() str"),
	}
}

func (obj *shutdownStreamFunc) Init(init *interfaces.Init) error {
	return nil
}

func (obj *shutdownStreamFunc) Stream(ctx context.Context) error {
	close(obj.started)
	<-ctx.Done()
	return obj.err(ctx)
}

func (obj *shutdownStreamFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{V: "hello"}, nil
}
