// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

//go:build !root

package util

import (
	"testing"
	"time"
)

func TestBlockedTimer1(t *testing.T) {
	bt := &BlockedTimer{Seconds: 1}
	defer bt.Cancel()
	ch := make(chan struct{})
	bt.Run(func() { close(ch) })
	select {
	case <-time.After(time.Duration(3) * time.Second):
		t.Errorf("the timer was too slow")
	case <-ch:
	}
}

func TestBlockedTimer2(t *testing.T) {
	bt := &BlockedTimer{Seconds: 3}
	defer bt.Cancel()
	ch := make(chan struct{})
	bt.Run(func() { close(ch) })
	select {
	case <-time.After(time.Duration(1) * time.Second):
	case <-ch:
		t.Errorf("the timer was too fast")
	}
}

func TestBlockedTimer3(t *testing.T) {
	bt := &BlockedTimer{Seconds: 2}
	defer bt.Cancel()
	ch := make(chan struct{})
	bt.Run(func() { close(ch) })
	select {
	case <-time.After(time.Duration(1) * time.Second):
	}
	bt.Cancel()
	select {
	case <-time.After(time.Duration(2) * time.Second):
	case <-ch:
		t.Errorf("the channel should not have closed")
	}
}

func TestBlockedTimer1b(t *testing.T) {
	bt := BlockedTimer{Seconds: 1}
	defer bt.Cancel()
	ch := make(chan struct{})
	bt.Run(func() { close(ch) })
	select {
	case <-time.After(time.Duration(3) * time.Second):
		t.Errorf("the timer was too slow")
	case <-ch:
	}
}

func TestBlockedTimer2b(t *testing.T) {
	bt := BlockedTimer{Seconds: 3}
	defer bt.Cancel()
	ch := make(chan struct{})
	bt.Run(func() { close(ch) })
	select {
	case <-time.After(time.Duration(1) * time.Second):
	case <-ch:
		t.Errorf("the timer was too fast")
	}
}

func TestBlockedTimer3b(t *testing.T) {
	bt := BlockedTimer{Seconds: 3}
	defer bt.Cancel()
	ch := make(chan struct{})
	bt.Run(func() { close(ch) })
	select {
	case <-time.After(time.Duration(1) * time.Second):
	}
	bt.Cancel()
	select {
	case <-time.After(time.Duration(2) * time.Second):
	case <-ch:
		t.Errorf("the channel should not have closed")
	}
}

// just an example to see how to use AfterFunc instead of BlockedTimer
func TestAfterFunc1(t *testing.T) {
	ch := make(chan struct{})
	af := time.AfterFunc(time.Duration(2)*time.Second, func() { close(ch) })
	defer af.Stop()
	select {
	case <-time.After(time.Duration(1) * time.Second):
	}
	af.Stop()
	af.Stop()
	af.Stop()
	select {
	case <-ch:
		t.Errorf("the timer was too fast")
	default:
	}
}
