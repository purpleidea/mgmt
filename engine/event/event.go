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

// Package event provides some primitives that are used for message passing.
package event

//go:generate stringer -type=Kind -output=kind_stringer.go

// Kind represents the type of event being passed.
type Kind int

// The different event kinds are used in different contexts.
const (
	KindNil Kind = iota
	KindStart
	KindPause
	KindPoke
	KindExit
)

// Pre-built messages so they can be used directly without having to use NewMsg.
// These are useful when we don't want a response via ACK().
var (
	Start = &Msg{Kind: KindStart}
	Pause = &Msg{Kind: KindPause} // probably unused b/c we want a resp
	Poke  = &Msg{Kind: KindPoke}
	Exit  = &Msg{Kind: KindExit}
)

// Msg is an event primitive that represents a kind of event, and optionally a
// request for an ACK.
type Msg struct {
	Kind Kind

	resp chan struct{}
}

// NewMsg builds a new message struct. It will want an ACK. If you don't want an
// ACK then use the pre-built messages in the package variable globals.
func NewMsg(kind Kind) *Msg {
	return &Msg{
		Kind: kind,
		resp: make(chan struct{}),
	}
}

// CanACK determines if an ACK is possible for this message. It does not say
// whether one has already been sent or not.
func (obj *Msg) CanACK() bool {
	return obj.resp != nil
}

// ACK acknowledges the event. It must not be called more than once for the same
// event. It unblocks the past and future calls of Wait for this event.
func (obj *Msg) ACK() {
	close(obj.resp)
}

// Wait on ACK for this event. It doesn't matter if this runs before or after
// the ACK. It will unblock either way.
// TODO: consider adding a context if it's ever useful.
func (obj *Msg) Wait() error {
	select {
	//case <-ctx.Done():
	//	return ctx.Err()
	case <-obj.resp:
		return nil
	}
}
