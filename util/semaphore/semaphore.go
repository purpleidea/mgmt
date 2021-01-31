// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

// Package semaphore contains an implementation of a counting semaphore.
package semaphore

import (
	"fmt"
)

// Semaphore is a counting semaphore. It must be initialized before use.
type Semaphore struct {
	C      chan struct{}
	closed chan struct{}
}

// NewSemaphore creates a new semaphore.
func NewSemaphore(size int) *Semaphore {
	obj := &Semaphore{}
	obj.Init(size)
	return obj
}

// Init initializes the semaphore.
func (obj *Semaphore) Init(size int) {
	obj.C = make(chan struct{}, size)
	obj.closed = make(chan struct{})
}

// Close shuts down the semaphore and releases all the locks.
func (obj *Semaphore) Close() {
	// TODO: we could return an error if any semaphores were killed, but
	// it's not particularly useful to know that for this application...
	close(obj.closed)
}

// P acquires n resources.
func (obj *Semaphore) P(n int) error {
	for i := 0; i < n; i++ {
		select {
		case obj.C <- struct{}{}: // acquire one
		case <-obj.closed: // exit signal
			return fmt.Errorf("closed")
		}
	}
	return nil
}

// V releases n resources.
func (obj *Semaphore) V(n int) error {
	for i := 0; i < n; i++ {
		select {
		case <-obj.C: // release one
		// TODO: is the closed signal needed if unlocks should always pass?
		case <-obj.closed: // exit signal
			return fmt.Errorf("closed")
		// TODO: is it true you shouldn't call a release before a lock?
		default: // trying to release something that isn't locked
			panic("semaphore: V > P")
		}
	}
	return nil
}
