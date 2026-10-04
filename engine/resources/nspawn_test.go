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

package resources

import (
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

func TestNspawnCmp(t *testing.T) {
	// nspawn returns an nspawn resource which hasn't been through Init,
	// like the engine has when it compares a new graph against the old one.
	nspawn := func(t *testing.T) *NspawnRes {
		res, err := engine.NewNamedResource("nspawn", "nspawn1")
		if err != nil {
			t.Fatalf("func NewNamedResource: %v", err)
		}
		r := res.(*NspawnRes).Default().(*NspawnRes) // must not panic
		r.SetKind("nspawn")
		r.SetName("nspawn1")
		return r
	}

	t.Run("same", func(t *testing.T) {
		if err := engine.ResCmp(nspawn(t), nspawn(t)); err != nil {
			t.Errorf("expected the same, got: %v", err)
		}
	})

	// This is what the engine does when it swaps graphs: it compares the
	// running resource, which has been through Init, with the new one.
	t.Run("same after init", func(t *testing.T) {
		r1, r2 := nspawn(t), nspawn(t)
		// Init makes the nested svc, but needs a real engine to run.
		svc, err := r1.makeComposite()
		if err != nil {
			t.Fatalf("func makeComposite: %v", err)
		}
		r1.svc = svc
		if err := engine.ResCmp(r1, r2); err != nil {
			t.Errorf("expected the same, got: %v", err)
		}
	})

	t.Run("state", func(t *testing.T) {
		r1, r2 := nspawn(t), nspawn(t)
		r2.State = stopped
		if err := engine.ResCmp(r1, r2); err == nil {
			t.Errorf("expected a different state to differ")
		}
	})
}

func TestNspawnUIDs(t *testing.T) {
	res, err := engine.NewNamedResource("nspawn", "nspawn1")
	if err != nil {
		t.Fatalf("func NewNamedResource: %v", err)
	}
	r := res.(*NspawnRes) // must not panic

	// This hasn't been through Init, which is when autoedges would call it.
	uids := r.UIDs()
	if len(uids) != 2 {
		t.Fatalf("expected the nspawn and the svc uid, got: %d", len(uids))
	}
	if _, ok := uids[0].(*NspawnUID); !ok {
		t.Errorf("expected an nspawn uid first, got: %T", uids[0])
	}
	if _, ok := uids[1].(*SvcUID); !ok {
		t.Errorf("expected a svc uid second, got: %T", uids[1])
	}
}
