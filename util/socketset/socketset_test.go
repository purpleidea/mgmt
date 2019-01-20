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

package socketset

import (
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// test cases for socketSet.fdSet()
var fdSetTests = []struct {
	in  *SocketSet
	out *unix.FdSet
}{
	{
		&SocketSet{
			fdEvents: 3,
			fdPipe:   4,
		},
		&unix.FdSet{
			Bits: [16]int64{0x18}, // 11000
		},
	},
	{
		&SocketSet{
			fdEvents: 12,
			fdPipe:   8,
		},
		&unix.FdSet{
			Bits: [16]int64{0x1100}, // 1000100000000
		},
	},
	{
		&SocketSet{
			fdEvents: 9,
			fdPipe:   21,
		},
		&unix.FdSet{
			Bits: [16]int64{0x200200}, // 1000000000001000000000
		},
	},
}

// test socketSet.fdSet()
func TestFdSet(t *testing.T) {
	for _, test := range fdSetTests {
		result := test.in.fdSet()
		if *result != *test.out {
			t.Errorf("fdSet test wanted: %b, got: %b", *test.out, *result)
		}
	}
}

// test cases for socketSet.nfd()
var nfdTests = []struct {
	in  *SocketSet
	out int
}{
	{
		&SocketSet{
			fdEvents: 3,
			fdPipe:   4,
		},
		5,
	},
	{
		&SocketSet{
			fdEvents: 8,
			fdPipe:   4,
		},
		9,
	},
	{
		&SocketSet{
			fdEvents: 90,
			fdPipe:   900,
		},
		901,
	},
}

// test socketSet.nfd()
func TestNfd(t *testing.T) {
	for _, test := range nfdTests {
		result := test.in.nfd()
		if result != test.out {
			t.Errorf("nfd test wanted: %d, got: %d", test.out, result)
		}
	}
}

// test SocketSet.Shutdown()
func TestShutdown(t *testing.T) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	// pass 0 so we create a socket that doesn't receive any events
	ss, err := NewSocketSet(0, "pipe.sock", 0)
	if err != nil {
		t.Errorf("could not create SocketSet: %+v", err)
	}
	closeChan := make(chan struct{})
	defer close(closeChan)
	defer ss.Close()
	defer ss.Shutdown()

	// create a listener that never receives any data
	wg.Add(1) // add a waitgroup to ensure this will block if we don't properly unblock Select
	go func() {
		defer wg.Done()
		_, _ = ss.ReceiveBytes() // this should block
		select {
		case <-closeChan:
			return
		}
	}()
}
