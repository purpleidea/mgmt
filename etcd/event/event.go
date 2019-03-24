// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

// Package event provides some primitives that are used for message passing.
package event

import (
	"fmt"
)

// Resp is a channel to be used for boolean responses. A nil represents an ACK,
// and a non-nil represents a NACK (false). This also lets us use custom errors.
type Resp chan error

// NewResp is just a helper to return the right type of response channel.
func NewResp() Resp {
	resp := make(chan error)
	return resp
}

// ACK sends a true value to resp.
func (resp Resp) ACK() {
	if resp != nil {
		resp <- nil // TODO: close instead?
	}
}

// NACK sends a false value to resp.
func (resp Resp) NACK() {
	if resp != nil {
		resp <- fmt.Errorf("NACK")
	}
}

// ACKNACK sends a custom ACK or NACK. The ACK value is always nil, the NACK can
// be any non-nil error value.
func (resp Resp) ACKNACK(err error) {
	if resp != nil {
		resp <- err
	}
}

// Wait waits for any response from a Resp channel and returns it.
func (resp Resp) Wait() error {
	return <-resp
}

// ACKWait waits for a +ive Ack from a Resp channel.
func (resp Resp) ACKWait() {
	for {
		// wait until true value
		if resp.Wait() == nil {
			return
		}
	}
}
