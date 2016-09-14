// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"fmt"
)

//go:generate stringer -type=eventName -output=eventname_stringer.go
type eventName int

const (
	eventNil eventName = iota
	eventExit
	eventStart
	eventPause
	eventPoke
	eventBackPoke
)

// Resp is a channel to be used for boolean responses. A nil represents an ACK,
// and a non-nil represents a NACK (false). This also lets us use custom errors.
type Resp chan error

// Event is the main struct that stores event information and responses.
type Event struct {
	Name eventName
	Resp Resp // channel to send an ack response on, nil to skip
	//Wg   *sync.WaitGroup // receiver barrier to Wait() for everyone else on
	Msg      string // some words for fun
	Activity bool   // did something interesting happen?
}

// ACK sends a single acknowledgement on the channel if one was requested.
func (event *Event) ACK() {
	if event.Resp != nil { // if they've requested an ACK
		event.Resp.ACK()
	}
}

// NACK sends a negative acknowledgement message on the channel if one was requested.
func (event *Event) NACK() {
	if event.Resp != nil { // if they've requested a NACK
		event.Resp.NACK()
	}
}

// ACKNACK sends a custom ACK or NACK message on the channel if one was requested.
func (event *Event) ACKNACK(err error) {
	if event.Resp != nil { // if they've requested a NACK
		event.Resp.ACKNACK(err)
	}
}

// NewResp is just a helper to return the right type of response channel.
func NewResp() Resp {
	resp := make(chan error)
	return resp
}

// ACK sends a true value to resp.
func (resp Resp) ACK() {
	if resp != nil {
		resp <- nil
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

// GetActivity returns the activity value.
func (event *Event) GetActivity() bool {
	return event.Activity
}
