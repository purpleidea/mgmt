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

package engine

import (
	"fmt"
)

// SendableRes is the interface a resource must implement to support sending
// named parameters. You must specify to the engine what kind of values (and
// with their types) you will be sending. This is used for static type checking.
// Formerly, you had to make sure not to overwrite omitted parameters, otherwise
// it will be as if you've now declared a fixed state for that param. For that
// example, if a parameter `Foo string` had the zero value to mean that it was
// undefined, and you learned that the value is actually `up`, then sending on
// that param would cause that state to be managed, when it was previously not.
// This new interface actually provides a different namespace for sending keys.
type SendableRes interface {
	Res // implement everything in Res but add the additional requirements

	// Sends returns a struct containing the defaults of the type we send.
	Sends() interface{}

	// Send is used in CheckApply to send the desired data. It returns an
	// error if the data is malformed or doesn't type check. You should use
	// the GenerateSendFunc helper function to build this function for use
	// in the resource internal state handle.
	Send(st interface{}) error

	// Sent returns the most recently sent data. This is used by the engine.
	Sent() interface{}

	// SendActive let's the resource know if it must send a value. This is
	// usually called during CheckApply, but it's probably safe to check it
	// during Init as well.
	// XXX: Not doing this for now. If a send/recv edge wasn't initially on,
	// and the sender ran CheckApply, but didn't cache the value to send,
	// and then the edge flipped on, we'd have to error. Better to always
	// generate the cache, and only consider adding this if we have a more
	// important privacy or performance situation that requires it.
	//SendActive() bool

	// SendSetActive is used by the compiler to store the "SendActive" bool
	// so that it will later know if it will need to send or not. Only the
	// engine should call this function.
	// TODO: We could instead pass in the various edges we will be sending,
	// and store a map of those for the resource to know more precisely.
	// XXX: Not doing this for now. If a send/recv edge wasn't initially on,
	// and the sender ran CheckApply, but didn't cache the value to send,
	// and then the edge flipped on, we'd have to error. Better to always
	// generate the cache, and only consider adding this if we have a more
	// important privacy or performance situation that requires it.
	//SendSetActive(bool)
}

// RecvableRes is the interface a resource must implement to support receiving
// on public parameters. The resource only has to include the correct trait for
// this interface to be fulfilled, as no additional methods need to be added. To
// get information about received changes, you can use the Recv method from the
// input API that comes in via Init.
type RecvableRes interface {
	Res

	// SetRecv stores the map of sendable data which should arrive here. It
	// is called by the GAPI when building the resource.
	SetRecv(recv map[string]*Send)

	// Recv is used by the resource to get information on changes. This data
	// can be used to invalidate caches, restart watches, or it can be
	// ignored entirely. You should use the GenerateRecvFunc helper function
	// to build this function for use in the resource internal state handle.
	Recv() map[string]*Send
}

// Send points to a value that a resource will send.
type Send struct {
	Res SendableRes // a handle to the resource which is sending a value
	Key string      // the key in the resource that we're sending

	Changed bool // set to true if this key was updated, read only!
}

// GenerateSendFunc generates the Send function using the resource of our choice
// for use in the resource internal state handle.
func GenerateSendFunc(res Res) func(interface{}) error {
	return func(st interface{}) error {
		//fmt.Printf("from: %+v\n", res)
		//fmt.Printf("send: %+v\n", st)
		r, ok := res.(SendableRes)
		if !ok {
			panic(fmt.Sprintf("res of kind `%s` does not support the Sendable trait", res.Kind()))
		}
		// XXX: type check this
		//expected := r.Sends()
		//if err := XXX_TYPE_CHECK(expected, st); err != nil {
		//	return err
		//}

		return r.Send(st) // send the struct
	}
}

// GenerateRecvFunc generates the Recv function using the resource of our choice
// for use in the resource internal state handle.
func GenerateRecvFunc(res Res) func() map[string]*Send {
	return func() map[string]*Send { // TODO: change this API?
		r, ok := res.(RecvableRes)
		if !ok {
			panic(fmt.Sprintf("res of kind `%s` does not support the Recvable trait", res.Kind()))
		}
		return r.Recv()
	}
}
