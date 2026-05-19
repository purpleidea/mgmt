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

package traits

import (
	"sync/atomic"
	"unsafe"

	"github.com/purpleidea/mgmt/engine"
)

// sendableValue is an immutable wrapper around the published payload. The slot
// always holds a non-nil *sendableValue of this single concrete type, which is
// what lets us use an atomic.Value without ever hitting its panic-on-nil or
// panic-on-type-change constraints, while still representing "nothing sent yet"
// (a nil Load) and a nil payload distinctly. Unlike atomic.Pointer,
// atomic.Value carries no noCopy field, so resources embedding the Sendable
// trait stay copyable and don't fail golang vet checks.
type sendableValue struct {
	value interface{}
}

// Sendable contains a general implementation with some of the properties and
// methods needed to implement sending from resources. You'll need to implement
// the Sends method, and call the Send method in CheckApply via the Init API.
type Sendable struct {
	// addr restores the copy protection that atomic.Pointer's noCopy would
	// have given us for free, in the spirit of strings.Builder's copyCheck.
	// atomic.Value must not be copied once Store has been called. The only
	// by-value resource copy in the tree is UnmarshalYAML's rawRes(*res),
	// which runs before any Send, so addr is still nil and nothing panics.
	// Any later copy-then-use panics loudly instead of corrupting silently.
	// It is a plain unsafe.Pointer (no noCopy) so resources stay golang vet
	// copyable for that pre-Send dance, and it is touched only via
	// sync/atomic because Send and Sent run on different resource workers.
	addr unsafe.Pointer // *Sendable; set once on first Send/Sent

	// send is published by CheckApply and read by other resource workers.
	// It always holds a *sendableValue.
	send atomic.Value

	//sendIsActive bool // TODO: public?

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// Sends returns a struct containing the defaults of the type we send. This
// needs to be implemented (overridden) by the struct with the Sendable trait to
// be able to send any values. The field struct tag names are the keys used.
func (obj *Sendable) Sends() interface{} {
	return nil
}

// Send is used to send a struct in CheckApply. This is typically wrapped in the
// resource API and consumed that way. The atomic store gives the cross-worker
// handoff a synchronization boundary. See the SendableRes interface for the
// snapshot/no-mutate contract that callers must honour.
func (obj *Sendable) Send(st interface{}) error {
	obj.copyCheck()
	// TODO: can we (or should we) run the type checking here instead?
	obj.send.Store(&sendableValue{value: st})
	return nil
}

// Sent returns the struct of values that have been sent by this resource, or
// nil if nothing has been sent yet. It should not be called before a value was
// sent, the nil return is a courtesy. It may run concurrently with Send. See
// the SendableRes interface for the read-only contract on the returned value.
func (obj *Sendable) Sent() interface{} {
	obj.copyCheck()
	value := obj.send.Load()
	if value == nil {
		return nil
	}
	return value.(*sendableValue).value
}

// copyCheck panics if this Sendable has been copied by value after its first
// use. It is safe to call concurrently: the first user wins the CAS, everyone
// else (including readers on other workers) only loads and compares.
func (obj *Sendable) copyCheck() {
	self := unsafe.Pointer(obj)
	if atomic.CompareAndSwapPointer(&obj.addr, nil, self) {
		return // first use of this Sendable
	}
	if atomic.LoadPointer(&obj.addr) != self {
		panic("traits.Sendable: illegal copy of a resource after Send/Sent")
	}
}

// SendActive let's the resource know if it must send a value. This is usually
// called during CheckApply, but it's probably safe to check it during Init as
// well. This is the implementation of this function.
// XXX: Not doing this for now, see the interface for more information.
//func (obj *Sendable) SendActive() bool {
//	return obj.sendIsActive
//}

// SendSetActive is used by the compiler to store the "SendActive" bool so that
// it will later know if it will need to send or not. Only the engine should
// call this function. This is the implementation of this function.
// TODO: We could instead pass in the various edges we will be sending, and
// store a map of those for the resource to know more precisely.
// XXX: Not doing this for now, see the interface for more information.
//func (obj *Sendable) SendSetActive(b bool) {
//	obj.sendIsActive = b
//}

// Recvable contains a general implementation with some of the properties and
// methods needed to implement receiving from resources.
type Recvable struct {
	recv map[string]*engine.Send

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// SetRecv is used to inject incoming values into the resource. More
// specifically, it stores the mapping of what gets received from what, so that
// later on, we know which resources should ask which other resources for the
// values that they want to receive. Since this happens when we're building the
// graph, and before the autogrouping step, we'll have pointers to the original,
// ungrouped resources here, so that it will work even after they're grouped in!
func (obj *Recvable) SetRecv(recv map[string]*engine.Send) {
	//if obj.recv == nil {
	//	obj.recv = make(map[string]*engine.Send)
	//}
	obj.recv = recv
}

// Recv is used to get information that was passed in. This data can then be
// used to run the Send/Recv data transfer.
func (obj *Recvable) Recv() map[string]*engine.Send {
	return obj.recv
}
