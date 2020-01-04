// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
	"context"
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

// EasyAckOnce is a wrapper to build ack functionality into a simple interface.
// It is safe because the Ack function can be called multiple times safely.
type EasyAckOnce struct {
	done chan struct{}
	once *sync.Once
}

// NewEasyAckOnce builds the object. This must be called before use.
func NewEasyAckOnce() *EasyAckOnce {
	return &EasyAckOnce{
		done: make(chan struct{}),
		once: &sync.Once{},
	}
}

// Ack sends the acknowledgment message. This can be called as many times as you
// like. Only the first Ack is meaningful. Subsequent Ack's are redundant. It is
// thread-safe.
func (obj *EasyAckOnce) Ack() {
	fn := func() { close(obj.done) }
	obj.once.Do(fn)
}

// Wait returns a channel that you can wait on for the ack message. The return
// channel closes on the first Ack it receives. Subsequent Ack's have no effect.
func (obj *EasyAckOnce) Wait() <-chan struct{} {
	return obj.done
}

// EasyExit is a struct that helps you build a close switch and signal which can
// be called multiple times safely, and used as a signal many times in parallel.
// It can also provide a context, if you prefer to use that as a signal instead.
type EasyExit struct {
	mutex *sync.Mutex
	exit  chan struct{}
	once  *sync.Once
	err   error
	wg    *sync.WaitGroup
}

// NewEasyExit builds an easy exit struct.
func NewEasyExit() *EasyExit {
	return &EasyExit{
		mutex: &sync.Mutex{},
		exit:  make(chan struct{}),
		once:  &sync.Once{},
		wg:    &sync.WaitGroup{},
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

// Context returns a context that is canceled when the Done signal is triggered.
// This can be used in addition to or instead of the Signal method.
func (obj *EasyExit) Context() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	obj.wg.Add(1) // prevent leaks
	go func() {
		defer obj.wg.Done()
		defer cancel()
		select {
		case <-obj.Signal():
		}
	}()

	return ctx
}

// Error returns the error condition associated with the Done signal. It blocks
// until Done is called at least once. It then returns any of the errors or nil.
// It is only guaranteed to at least return the error from the first Done error.
func (obj *EasyExit) Error() error {
	select {
	case <-obj.exit:
	}
	obj.wg.Wait() // wait for cleanup
	return obj.err
}

// SubscribedSignal represents a synchronized read signal. It doesn't need to be
// instantiated before it can be used. It must not be copied after first use. It
// is equivalent to receiving a multicast signal from a closing channel, except
// that it must be acknowledged by every reader of the signal, and once this is
// done, it is reset and can be re-used. Readers must obtain a handle to the
// signal with the Subscribe method, and the signal is sent out with the Done
// method.
type SubscribedSignal struct {
	wg    sync.WaitGroup
	exit  chan struct{}
	mutex sync.RWMutex
}

// Subscribe is used by any reader of the signal. Once this function returns, it
// means that you're now ready to watch the signal. The signal can be watched as
// is done normally with any other ready channel. Once you have received the
// signal or when you are no longer interested in the signal you *must* call the
// cancel/ack function which is returned by this function on subscribe. If you
// do not, you will block the Send portion of this subscribed signal
// indefinitely. This is thread safe and can be called multiple times in
// parallel because this call is protected by a mutex. The mutex also prevents
// simultaneous calls with the Send method. the returned cancel/ack method must
// return before it's safe to call this method a subsequent time for a new
// signal. One important note: there is a possible race that *you* can cause if
// you race this Subscribe call, with the Send call. Make sure you run Subscribe
// and it returns *before* you run Send if you want to be sure to receive the
// next signal. This should be common sense but it is mentioned here to be
// helpful. They are protected by a lock, so they can't both run simultaneously.
func (obj *SubscribedSignal) Subscribe() (<-chan struct{}, func()) {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.exit == nil { // initialize on first use (safe b/c we use a lock)
		obj.exit = make(chan struct{}) // initialize
	}

	obj.wg.Add(1)
	return obj.exit, func() { // cancel/ack function
		obj.wg.Done()

		// wait for the reset signal before proceeding
		obj.mutex.RLock()
		defer obj.mutex.RUnlock()
	}
}

// Send is called if you want to multicast the signal to all subscribed parties.
// It will require all parties to acknowledge the receipt of the signal before
// it will unblock. Just before returning, it will reset the signal so that it
// can be called a subsequent time. This is thread safe and can be called
// multiple times in parallel because this call is protected by a mutex. The
// mutex also prevents simultaneous calls with the Subscribe method.
func (obj *SubscribedSignal) Send() {
	obj.mutex.Lock()
	defer obj.mutex.Unlock()

	if obj.exit != nil { // in case we Send before anyone runs Subscribe
		close(obj.exit) // send the close signal
	}
	obj.wg.Wait() // wait for everyone to ack

	obj.exit = make(chan struct{}) // reset

	// release (re-use the above mutex)
}
