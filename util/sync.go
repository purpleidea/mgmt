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

package util

import (
	"sync"
)

// EasyAck is a wrapper to build ack functionality into a simple interface.
type EasyAck struct {
	done chan struct{}
}

// NewEasyAck builds the object. This must be called before use.
func NewEasyAck() *EasyAck {
	return &EasyAck{
		done: make(chan struct{}),
	}
}

// Ack sends the acknowledgment message. This can only be called once.
func (obj *EasyAck) Ack() {
	close(obj.done)
}

// Wait returns a channel that you can wait on for the ack message.
func (obj *EasyAck) Wait() <-chan struct{} {
	return obj.done
}

// EasyOnce is a wrapper for the sync.Once functionality which lets you define
// and register the associated `run once` function at declaration time. It may
// be copied at any time.
type EasyOnce struct {
	Func func()

	once *sync.Once
}

// Done runs the function which was defined in `Func` a maximum of once. Please
// note that this is not currently thread-safe. Wrap calls to this with a mutex.
func (obj *EasyOnce) Done() {
	if obj.once == nil {
		// we must initialize it!
		obj.once = &sync.Once{}
	}
	if obj.Func != nil {
		obj.once.Do(obj.Func)
	}
}

// EasyExit is a struct that helps you build a close switch and signal which can
// be called multiple times safely, and used as a signal many times in parallel.
type EasyExit struct {
	mutex *sync.Mutex
	exit  chan struct{}
	once  *sync.Once
	err   error
}

// NewEasyExit builds an easy exit struct.
func NewEasyExit() *EasyExit {
	return &EasyExit{
		mutex: &sync.Mutex{},
		exit:  make(chan struct{}),
		once:  &sync.Once{},
	}
}

// Done triggers the exit signal. It associates an error condition with it too.
// This is thread-safe.
func (obj *EasyExit) Done(err error) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if obj.once == nil { // redundant
		// we must initialize it!
		obj.once = &sync.Once{}
	}
	if err != nil {
		// TODO: we could add a mutex, and turn this into a multierr
		obj.err = err
	}
	obj.once.Do(func() { close(obj.exit) })
}

// Signal returns the channel that we watch for the exit signal on. It will
// close to signal us when triggered by Exit().
func (obj *EasyExit) Signal() <-chan struct{} {
	return obj.exit
}

// Error returns the error condition associated with the Done signal. It blocks
// until Done is called at least once. It then returns any of the errors or nil.
// It is only guaranteed to at least return the error from the first Done error.
func (obj *EasyExit) Error() error {
	select {
	case <-obj.exit:
	}
	return obj.err
}
