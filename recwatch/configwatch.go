// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

package recwatch

import (
	"log"
	"sync"
)

// ConfigWatcher returns events on a channel anytime one of its files events.
type ConfigWatcher struct {
	Flags Flags

	ch        chan string
	wg        sync.WaitGroup
	closechan chan struct{}
	errorchan chan error
}

// NewConfigWatcher creates a new ConfigWatcher struct.
func NewConfigWatcher() *ConfigWatcher {
	return &ConfigWatcher{
		ch:        make(chan string),
		closechan: make(chan struct{}),
		errorchan: make(chan error),
	}
}

// Add new file paths to watch for events on.
func (obj *ConfigWatcher) Add(file ...string) {
	if len(file) == 0 {
		return
	}
	if len(file) > 1 {
		for _, f := range file { // add all the files...
			obj.Add(f) // recurse
		}
		return
	}
	// otherwise, add the one file passed in...
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		ch := obj.ConfigWatch(file[0])
		for {
			select {
			case e, ok := <-ch:
				if !ok { // channel closed
					return
				}
				if e != nil {
					obj.errorchan <- e
					return
				}
				select {
				case obj.ch <- file[0]: // send on channel
				case <-obj.closechan:
					return // never mind, close early!
				}
				continue
				// not needed, closes via ConfigWatch() chan close
				//case <-obj.closechan:
				//	return
			}
		}
	}()
}

// Error returns a channel of errors that notifies us of permanent issues.
func (obj *ConfigWatcher) Error() <-chan error {
	return obj.errorchan
}

// Events returns a channel to listen on for file events. It closes when it is
// emptied after the Close() method is called. You can test for closure with the
// f, more := <-obj.Events() pattern.
func (obj *ConfigWatcher) Events() chan string {
	return obj.ch
}

// Close shuts down the ConfigWatcher object. It closes the Events channel after
// all the currently pending events have been emptied.
func (obj *ConfigWatcher) Close() {
	if obj.ch == nil {
		return
	}
	close(obj.closechan)
	obj.wg.Wait() // wait until everyone is done sending on obj.ch
	//obj.ch <- "" // send finished message
	close(obj.ch)
	obj.ch = nil
	close(obj.errorchan)
}

// ConfigWatch writes on the channel every time an event is seen for the path.
func (obj *ConfigWatcher) ConfigWatch(file string) chan error {
	ch := make(chan error)
	go func() {
		recWatcher, err := NewRecWatcher(file, false)
		if err != nil {
			ch <- err
			close(ch)
			return
		}
		recWatcher.Flags = obj.Flags
		defer recWatcher.Close()
		for {
			if obj.Flags.Debug {
				log.Printf("Watching: %v", file)
			}
			select {
			case event, ok := <-recWatcher.Events():
				if !ok { // channel is closed
					close(ch)
					return
				}
				if err := event.Error; err != nil {
					ch <- err
					close(ch)
					return
				}
				select {
				case ch <- nil: // send event!
				case <-obj.closechan:
					close(ch)
					return
				}
			}
		}
		//close(ch)
	}()
	return ch
}
