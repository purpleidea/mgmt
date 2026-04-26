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

package graph

import (
	"context"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

func init() {
	gob.Register(&reversibleGraphTestRes{})
}

type reversibleGraphTestRes struct {
	traits.Base
	traits.Reversible

	Value string
}

func (obj *reversibleGraphTestRes) Default() engine.Res {
	return &reversibleGraphTestRes{}
}

func (obj *reversibleGraphTestRes) Validate() error { return nil }

func (obj *reversibleGraphTestRes) Init(*engine.Init) error { return nil }

func (obj *reversibleGraphTestRes) Cleanup() error { return nil }

func (obj *reversibleGraphTestRes) Watch(context.Context) error { return nil }

func (obj *reversibleGraphTestRes) CheckApply(context.Context, bool) (bool, error) {
	return true, nil
}

func (obj *reversibleGraphTestRes) Cmp(engine.Res) error { return nil }

func (obj *reversibleGraphTestRes) Copy() engine.CopyableRes {
	return &reversibleGraphTestRes{
		Value: obj.Value,
	}
}

func (obj *reversibleGraphTestRes) Reversed() (engine.ReversibleRes, error) {
	cp, err := engine.ResCopy(obj)
	if err != nil {
		return nil, err
	}
	rev, ok := cp.(engine.ReversibleRes)
	if !ok {
		return nil, nil
	}
	rev.ReversibleMeta().Disabled = true
	return rev, nil
}

func newReversibleGraphTestRes(name string) *reversibleGraphTestRes {
	res := &reversibleGraphTestRes{
		Value: "hello",
	}
	res.SetKind("copytest")
	res.SetName(name)
	return res
}

func newReversibleGraphTestState(t *testing.T, res engine.Res) *State {
	t.Helper()

	st := &State{
		Vertex:   res,
		Hostname: "test-host",
		Prefix:   t.TempDir(),
		Logf:     func(string, ...interface{}) {},
	}
	if err := st.Init(); err != nil {
		t.Fatalf("state init failed: %v", err)
	}
	return st
}

func TestReversalInitDoesNotMutateOriginalReversibleMeta(t *testing.T) {
	res := newReversibleGraphTestRes("reversal-init")
	res.ReversibleMeta().Disabled = false

	st := newReversibleGraphTestState(t, res)
	if err := st.ReversalInit(); err != nil {
		t.Fatalf("reversal init failed: %v", err)
	}

	if res.ReversibleMeta().Disabled {
		t.Fatalf("reversal init mutated original Disabled flag")
	}
	if res.ReversibleMeta().Reversal {
		t.Fatalf("reversal init mutated original Reversal flag")
	}
}

func TestReversalCleanupDoesNotDeletePendingReverseFromOriginalResource(t *testing.T) {
	res := newReversibleGraphTestRes("reversal-cleanup")
	res.ReversibleMeta().Disabled = false

	st := newReversibleGraphTestState(t, res)
	if err := st.ReversalInit(); err != nil {
		t.Fatalf("reversal init failed: %v", err)
	}

	reverseFile := filepath.Join(st.Prefix, ReverseFile)
	if _, err := os.Stat(reverseFile); err != nil {
		t.Fatalf("reverse file missing after init: %v", err)
	}

	st.isStateOK.Store(true)
	if err := st.ReversalCleanup(); err != nil {
		t.Fatalf("reversal cleanup failed: %v", err)
	}

	if _, err := os.Stat(reverseFile); err != nil {
		t.Fatalf("original resource removed pending reverse file: %v", err)
	}
}
