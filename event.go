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

//go:generate stringer -type=eventName -output=eventname_stringer.go
type eventName int

const (
	eventExit eventName = iota
	eventStart
	eventPause
	eventPoke
	eventChanged
)

type Event struct {
	Name eventName
	Resp chan bool // channel to send an ack response on, nil to skip
	//Wg   *sync.WaitGroup // receiver barrier to Wait() for everyone else on
	Msg string // some words for fun
}

// send a single acknowledgement on the channel if one was requested
func (event *Event) ACK() {
	if event.Resp != nil { // if they've requested an ACK
		event.Resp <- true // send ACK
	}
}

func (event *Event) NACK() {
	if event.Resp != nil { // if they've requested an ACK
		event.Resp <- false // send NACK
	}
}
