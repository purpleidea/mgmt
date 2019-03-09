// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package traits

import (
	"github.com/purpleidea/mgmt/engine"
)

// Sendable contains a general implementation with some of the properties and
// methods needed to implement sending from resources. You'll need to implement
// the Sends method, and call the Send method in CheckApply via the Init API.
type Sendable struct {
	send interface{}

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
// resource API and consumed that way.
func (obj *Sendable) Send(st interface{}) error {
	// TODO: can we (or should we) run the type checking here instead?
	obj.send = st
	return nil
}

// Sent returns the struct of values that have been sent by this resource.
func (obj *Sendable) Sent() interface{} {
	return obj.send
}

// Recvable contains a general implementation with some of the properties and
// methods needed to implement receiving from resources.
type Recvable struct {
	recv map[string]*engine.Send

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// SetRecv is used to inject incoming values into the resource.
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
