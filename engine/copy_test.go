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

package engine_test

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

type reversibleCopyTestRes struct {
	traits.Base
	traits.Reversible

	Value string
}

func (obj *reversibleCopyTestRes) Default() engine.Res {
	return &reversibleCopyTestRes{}
}

func (obj *reversibleCopyTestRes) Validate() error { return nil }

func (obj *reversibleCopyTestRes) Init(*engine.Init) error { return nil }

func (obj *reversibleCopyTestRes) Cleanup() error { return nil }

func (obj *reversibleCopyTestRes) Watch(context.Context) error { return nil }

func (obj *reversibleCopyTestRes) CheckApply(context.Context, bool) (bool, error) {
	return true, nil
}

func (obj *reversibleCopyTestRes) Cmp(engine.Res) error { return nil }

func (obj *reversibleCopyTestRes) Copy() engine.CopyableRes {
	return &reversibleCopyTestRes{
		Value: obj.Value,
	}
}

func (obj *reversibleCopyTestRes) Reversed() (engine.ReversibleRes, error) {
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

func newReversibleCopyTestRes(name string) *reversibleCopyTestRes {
	res := &reversibleCopyTestRes{
		Value: "hello",
	}
	res.SetKind("copytest")
	res.SetName(name)
	return res
}

func TestResCopyCopiesReversibleMetaByValue(t *testing.T) {
	original := newReversibleCopyTestRes("meta-by-value")
	original.ReversibleMeta().Disabled = false
	original.ReversibleMeta().Overwrite = true

	copiedRes, err := engine.ResCopy(original)
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}

	copied, ok := copiedRes.(*reversibleCopyTestRes)
	if !ok {
		t.Fatalf("unexpected copy type: %T", copiedRes)
	}

	if copied.ReversibleMeta() == original.ReversibleMeta() {
		t.Fatalf("reversible meta pointer was aliased")
	}

	copied.ReversibleMeta().Disabled = true
	copied.ReversibleMeta().Reversal = true

	if original.ReversibleMeta().Disabled {
		t.Fatalf("mutating copied reversible meta changed original Disabled flag")
	}
	if original.ReversibleMeta().Reversal {
		t.Fatalf("mutating copied reversible meta changed original Reversal flag")
	}
}

func TestReversedDoesNotMutateOriginalReversibleMeta(t *testing.T) {
	original := newReversibleCopyTestRes("reversed-isolated")
	original.ReversibleMeta().Disabled = false

	reversed, err := original.Reversed()
	if err != nil {
		t.Fatalf("reverse failed: %v", err)
	}
	if reversed == nil {
		t.Fatalf("reverse returned nil")
	}

	if !reversed.ReversibleMeta().Disabled {
		t.Fatalf("reversed resource should disable its own reversal")
	}
	if original.ReversibleMeta().Disabled {
		t.Fatalf("building the reversed resource mutated the original Disabled flag")
	}
}
