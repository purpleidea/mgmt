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

//go:build race

package traits

import (
	"runtime"
	"sync"
	"testing"
)

type sendableRaceValue struct {
	Value string
}

// TestSendableSendSentRace reproduces issue #926 at the smallest shared state:
// Send writes Sendable.send while Sent reads it.
func TestSendableSendSentRace(t *testing.T) {
	obj := &Sendable{}
	start := make(chan struct{})
	wg := &sync.WaitGroup{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < 100000; i++ {
			if err := obj.Send(&sendableRaceValue{Value: "value"}); err != nil {
				t.Errorf("method Send failed: %+v", err)
				return
			}
			runtime.Gosched()
		}
	}()
	go func() {
		defer wg.Done()
		<-start

		for i := 0; i < 100000; i++ {
			_ = obj.Sent()
			runtime.Gosched()
		}
	}()

	close(start)
	wg.Wait()
}
