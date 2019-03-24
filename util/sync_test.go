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

// +build !root

package util

import (
	"testing"
	"time"
)

func TestEasyAck1(t *testing.T) {
	ea := NewEasyAck()
	ea.Ack() // send the ack
	select {
	case <-ea.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the Ack did not arrive in time")
	}
}

func TestEasyAck2(t *testing.T) {
	ea := NewEasyAck()
	// never send an ack
	select {
	case <-ea.Wait(): // we got it!
		t.Errorf("the Ack arrived unexpectedly")
	default:
	}
}

func TestEasyAck3(t *testing.T) {
	ea := NewEasyAck()
	ea.Ack() // send the ack
	select {
	case <-ea.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the Ack did not arrive in time")
	}

	ea = NewEasyAck() // build a new one
	ea.Ack()          // send the ack
	select {
	case <-ea.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the second Ack did not arrive in time")
	}
}
