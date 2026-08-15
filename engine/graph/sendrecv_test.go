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
	"github.com/purpleidea/mgmt/pgraph"
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
			Kind: "noop",
			Name: "sender",
			Key:  "value",
		},
	})
	obj := newSendRecvEngine(sender)

	send := func(s string) {
		t.Helper()
		if err := sender.Send(&sendRecvValueSends{Value: &s}); err != nil {
			t.Fatalf("func Send: %v", err)
		}
	}
	sendRecv := func() bool { // did it change?
		t.Helper()
		updated, err := obj.sendRecv(receiver, nil)
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

func TestSendRecvGroupedIntoNonRecvableParent(t *testing.T) {
	sender := &sendRecvValueSource{}
	sender.SetKind("noop")
	sender.SetName("sender")

	// The http:server res is groupable but not recvable, while the
	// http:server:file res grouped inside of it is recvable. We must still
	// descend into the parent, or the child would never get its value.
	child := &resources.HTTPServerFileRes{}
	child.SetKind("http:server:file")
	child.SetName("/index.html")
	child.SetRecv(map[string]*engine.Send{
		"data": {
			Kind: "noop",
			Name: "sender",
			Key:  "value",
		},
	})

	parent := &resources.HTTPServerRes{}
	parent.SetKind("http:server")
	parent.SetName(":8080")
	parent.SetGroup([]engine.GroupableRes{child})

	if _, ok := engine.Res(parent).(engine.RecvableRes); ok {
		t.Fatalf("the http:server res is recvable, pick another parent")
	}

	value := "hello"
	if err := sender.Send(&sendRecvValueSends{Value: &value}); err != nil {
		t.Fatalf("func Send: %v", err)
	}

	updated, err := newSendRecvEngine(sender).sendRecv(parent, nil)
	if err != nil {
		t.Fatalf("func SendRecv: %v", err)
	}
	if send, exists := updated[child]["data"]; !exists {
		t.Errorf("the grouped child was never visited")
	} else if !send.Changed {
		t.Errorf("the grouped child did not change")
	}
	if child.Data != "hello" {
		t.Errorf("the grouped child has `%v`, expected `hello`", child.Data)
	}

	// ClearRecv must descend the same way that SendRecv does.
	ClearRecv(parent)
	if child.Recv()["data"].Changed {
		t.Errorf("func ClearRecv did not reach the grouped child")
	}
}

// newSendRecvFlag builds an http:server:flag, which is the autogrouped sender
// that motivated all of this.
func newSendRecvFlag(name string) *resources.HTTPServerFlagRes {
	flag := &resources.HTTPServerFlagRes{}
	flag.SetKind("http:server:flag")
	flag.SetName(name)
	return flag
}

// newSendRecvServer builds an http:server holding these grouped resources.
func newSendRecvServer(name string, grouped ...engine.GroupableRes) *resources.HTTPServerRes {
	server := &resources.HTTPServerRes{}
	server.SetKind("http:server")
	server.SetName(name)
	server.SetGroup(grouped)
	return server
}

// newSendRecvEngine builds the sender index the way a graph sync does, by
// handing the engine each resource which entered the graph.
func newSendRecvEngine(added ...engine.Res) *Engine {
	obj := &Engine{
		Logf:    func(format string, v ...interface{}) {},
		senders: make(map[string]engine.SendableRes),
	}
	for _, res := range added {
		obj.addSenders(res)
	}
	return obj
}

// TestSendRecvGroupedSenderAcrossSwap is the regression guard for a receiver
// which outlives the sender object it was compiled against. The graph sync
// decides for each resource separately whether to keep the one it has or take
// the incoming one, so a receiver can easily survive while its sender is
// replaced underneath it, or the other way around.
//
// An autogrouped http:server:flag makes this trivial to hit, because it isn't a
// vertex of its own: it lives or dies with the http:server it got grouped into,
// and that parent is far more stable than the resources receiving from it.
// Naming the sender instead of pointing at it is what makes the receiver follow
// the swap.
func TestSendRecvGroupedSenderAcrossSwap(t *testing.T) {
	receiver := &resources.ValueRes{}
	receiver.SetKind("value")
	receiver.SetName("receiver")
	receiver.SetRecv(map[string]*engine.Send{
		"any": {
			Kind: "http:server:flag",
			Name: "/flag",
			Key:  "value",
		},
	})

	send := func(flag *resources.HTTPServerFlagRes, s string) {
		t.Helper()
		if err := flag.Send(&resources.HTTPServerFlagSends{Value: &s}); err != nil {
			t.Fatalf("func Send: %v", err)
		}
	}
	received := func(obj *Engine) interface{} {
		t.Helper()
		if _, err := obj.sendRecv(receiver, nil); err != nil {
			t.Fatalf("func SendRecv: %v", err)
		}
		if receiver.Any == nil {
			t.Fatalf("the receiver never got a value")
		}
		return *receiver.Any
	}

	// The flag is grouped, so the engine only ever sees the http:server. It
	// still has to find the flag inside of it.
	before := newSendRecvFlag("/flag")
	server := newSendRecvServer(":8080", before)
	obj := newSendRecvEngine(server)

	send(before, "hello")
	if v := received(obj); v != "hello" {
		t.Errorf("the receiver has `%v`, expected `hello`", v)
	}
	ClearRecv(receiver) // the receiver had its CheckApply

	// Now swap the server out for a new one, the way a graph sync does: it
	// runs every remove before any add. The receiver is untouched, and the
	// flag it named is a different object now.
	after := newSendRecvFlag("/flag")
	obj.deleteSenders(server)
	obj.addSenders(newSendRecvServer(":8080", after))

	send(after, "world")
	if v := received(obj); v != "world" {
		t.Errorf("the receiver has `%v`, expected `world`", v)
	}

	// With the sender gone entirely there is nothing to read from, and we
	// want to hear about that rather than silently keep a stale value.
	obj.deleteSenders(newSendRecvServer(":8080", after))
	if _, err := obj.sendRecv(receiver, nil); err == nil {
		t.Errorf("a missing sender did not error")
	}
}

// TestSendRecvHiddenSender checks that a Hidden resource is not indexed as a
// sender. It never runs a CheckApply so it could never send, and leaving it out
// is what keeps the index unambiguous, since it is the only thing allowed to
// share a kind and name with a regular resource.
func TestSendRecvHiddenSender(t *testing.T) {
	hidden := newSendRecvFlag("/flag")
	hidden.MetaParams().Hidden = true

	obj := newSendRecvEngine(hidden)

	if _, err := obj.sender("http:server:flag", "/flag"); err == nil {
		t.Errorf("a hidden resource was indexed as a sender")
	}
}

// TestEngineSendRecvNilSender checks that the graph swap pass tolerates a
// sender which hasn't produced a value yet. That pass runs over the whole graph
// at once instead of in dependency order, so it can reach a sender before its
// first CheckApply, and erroring there takes the entire deploy down with it.
func TestEngineSendRecvNilSender(t *testing.T) {
	sender := &sendRecvValueSource{}
	sender.SetKind("noop")
	sender.SetName("sender")

	newReceiver := func() *resources.ValueRes {
		res := &resources.ValueRes{}
		res.SetKind("value")
		res.SetName("receiver")
		res.SetRecv(map[string]*engine.Send{
			"any": {
				Kind: "noop",
				Name: "sender",
				Key:  "value",
			},
		})
		return res
	}

	oldReceiver := newReceiver()
	old, err := pgraph.NewGraph("old")
	if err != nil {
		t.Fatalf("func NewGraph: %v", err)
	}
	old.AddVertex(oldReceiver)

	nextReceiver := newReceiver()
	next, err := pgraph.NewGraph("next")
	if err != nil {
		t.Fatalf("func NewGraph: %v", err)
	}
	next.AddVertex(nextReceiver)

	obj := newSendRecvEngine(sender)
	obj.graph = old
	obj.nextGraph = next

	// The sender hasn't sent, so there is simply nothing to pre-fill with.
	if err := obj.SendRecv(); err != nil {
		t.Fatalf("func SendRecv: %v", err)
	}
	if nextReceiver.Any != nil {
		t.Errorf("the receiver has `%v`, expected nothing", *nextReceiver.Any)
	}

	// Once it has sent, the new resource gets pre-filled as usual, which is
	// what stops it from comparing differently and being re-made.
	value := "hello"
	if err := sender.Send(&sendRecvValueSends{Value: &value}); err != nil {
		t.Fatalf("func Send: %v", err)
	}
	if err := obj.SendRecv(); err != nil {
		t.Fatalf("func SendRecv: %v", err)
	}
	if nextReceiver.Any == nil {
		t.Fatalf("the receiver never got a value")
	}
	if v := *nextReceiver.Any; v != "hello" {
		t.Errorf("the receiver has `%v`, expected `hello`", v)
	}
}
