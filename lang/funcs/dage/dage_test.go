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
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func TestRunReturnsContextCanceledFromCallShutdown(t *testing.T) {
	fn := &blockingContextFunc{
		called: make(chan struct{}),
	}
	fn.Locate(0, 0, 0, 1)

	engine := &Engine{
		Name: "test",
		Logf: t.Logf,
	}
	if err := engine.Setup(); err != nil {
		t.Fatalf("Setup failed: %+v", err)
	}

	txn := engine.Txn()
	defer txn.Free()
	txn.AddVertex(fn)
	if err := txn.Commit(); err != nil {
		t.Fatalf("Commit failed: %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- engine.Run(ctx)
	}()

	select {
	case <-fn.called:
	case <-time.After(5 * time.Second):
		t.Fatalf("function was not called")
	}

	cancel()

	select {
	case err := <-runErr:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled from Run, got: %+v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Run did not stop")
	}

	if err := errwrap.WithoutContext(engine.Err()); err != context.Canceled {
		t.Fatalf("expected context.Canceled from Err, got: %+v", err)
	}
}

type blockingContextFunc struct {
	interfaces.Textarea

	called chan struct{}
	once   sync.Once
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
	return nil, ctx.Err()
}
