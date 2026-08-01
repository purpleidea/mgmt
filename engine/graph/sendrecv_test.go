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
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/resources"
	"github.com/purpleidea/mgmt/engine/traits"
)

type sendRecvValueSource struct {
	resources.NoopRes
	traits.Sendable
}

type sendRecvValueSends struct {
	Value *string `lang:"value"`
}

func (obj *sendRecvValueSource) Sends() interface{} {
	return &sendRecvValueSends{}
}

func TestSendRecvAnyChanged(t *testing.T) {
	sender := &sendRecvValueSource{}
	sender.SetKind("noop")
	sender.SetName("sender")

	receiver := &resources.ValueRes{}
	receiver.SetKind("value")
	receiver.SetName("receiver")
	receiver.SetRecv(map[string]*engine.Send{
		"any": {
			Res: sender,
			Key: "value",
		},
	})

	send := func(s string) {
		t.Helper()
		if err := sender.Send(&sendRecvValueSends{Value: &s}); err != nil {
			t.Fatalf("func Send: %v", err)
		}
	}
	sendRecv := func() bool { // did it change?
		t.Helper()
		updated, err := SendRecv(receiver, nil)
		if err != nil {
			t.Fatalf("func SendRecv: %v", err)
		}
		return updated[receiver]["any"].Changed
	}
	received := func() interface{} {
		t.Helper()
		if receiver.Any == nil {
			t.Fatalf("receiver never got a value")
		}
		return *receiver.Any
	}

	// The sender field is a *string and the receiver field is an
	// *interface{}, so the two golang container types differ even when they
	// hold the same mcl value.
	send("hello")
	if !sendRecv() {
		t.Errorf("the first transfer did not change the receiver")
	}
	if v := received(); v != "hello" {
		t.Errorf("the receiver has `%v`, expected `hello`", v)
	}

	// The flag is sticky until the engine consumes it, even though a repeat
	// run finds the value that the previous run already stored in the
	// field.
	if !sendRecv() {
		t.Errorf("an unconsumed change was lost by a second transfer")
	}

	ClearRecv(receiver) // the receiver had its CheckApply

	// Once consumed, an identical value must not look like a change.
	if sendRecv() {
		t.Errorf("an identical transfer changed the receiver")
	}
	if v := received(); v != "hello" {
		t.Errorf("the receiver has `%v`, expected `hello`", v)
	}

	// A different value is a change again.
	send("world")
	if !sendRecv() {
		t.Errorf("a new value did not change the receiver")
	}
	if v := received(); v != "world" {
		t.Errorf("the receiver has `%v`, expected `world`", v)
	}
}
