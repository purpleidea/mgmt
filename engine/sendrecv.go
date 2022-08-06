// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package engine

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
			panic("res does not support the Sendable trait")
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
			panic("res does not support the Recvable trait")
		}
		return r.Recv()
	}
}
