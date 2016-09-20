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

import (
	"log"
	"math"
	"path"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/global"
	"github.com/purpleidea/mgmt/util"

	"gopkg.in/fsnotify.v1"
	//"github.com/go-fsnotify/fsnotify" // git master of "gopkg.in/fsnotify.v1"
)

// ConfigWatcher returns events on a channel anytime one of its files events.
type ConfigWatcher struct {
	ch        chan string
	wg        sync.WaitGroup
	closechan chan struct{}
}

// NewConfigWatcher creates a new ConfigWatcher struct.
func NewConfigWatcher() *ConfigWatcher {
	return &ConfigWatcher{
		ch:        make(chan string),
		closechan: make(chan struct{}),
	}
}

// The Add method adds a new file path to watch for events on.
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
		ch := ConfigWatch(file[0])
		for {
			select {
			case <-ch:
				obj.ch <- file[0]
				continue
			case <-obj.closechan:
				return
			}
		}
	}()
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
}

// ConfigWatch writes on the channel everytime an event is seen for the path.
// XXX: it would be great if we could reuse code between this and the file resource
// XXX: patch this to submit it as part of go-fsnotify if they're interested...
func ConfigWatch(file string) chan bool {
	ch := make(chan bool)
	go func() {
		var safename = path.Clean(file) // no trailing slash

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		defer watcher.Close()

		patharray := util.PathSplit(safename) // tokenize the path
		var index = len(patharray)            // starting index
		var current string                    // current "watcher" location
		var deltaDepth int                    // depth delta between watcher and event
		var send = false                      // send event?

		for {
			current = strings.Join(patharray[0:index], "/")
			if current == "" { // the empty string top is the root dir ("/")
				current = "/"
			}
			if global.DEBUG {
				log.Printf("Watching: %v", current) // attempting to watch...
			}
			// initialize in the loop so that we can reset on rm-ed handles
			err = watcher.Add(current)
			if err != nil {
				if err == syscall.ENOENT {
					index-- // usually not found, move up one dir
				} else if err == syscall.ENOSPC {
					// XXX: occasionally: no space left on device,
					// XXX: probably due to lack of inotify watches
					log.Printf("Out of inotify watches for config(%v)", file)
					log.Fatal(err)
				} else {
					log.Printf("Unknown config(%v) error:", file)
					log.Fatal(err)
				}
				index = int(math.Max(1, float64(index)))
				continue
			}

			select {
			case event := <-watcher.Events:
				// the deeper you go, the bigger the deltaDepth is...
				// this is the difference between what we're watching,
				// and the event... doesn't mean we can't watch deeper
				if current == event.Name {
					deltaDepth = 0 // i was watching what i was looking for

				} else if util.HasPathPrefix(event.Name, current) {
					deltaDepth = len(util.PathSplit(current)) - len(util.PathSplit(event.Name)) // -1 or less

				} else if util.HasPathPrefix(current, event.Name) {
					deltaDepth = len(util.PathSplit(event.Name)) - len(util.PathSplit(current)) // +1 or more

				} else {
					// TODO different watchers get each others events!
					// https://github.com/go-fsnotify/fsnotify/issues/95
					// this happened with two values such as:
					// event.Name: /tmp/mgmt/f3 and current: /tmp/mgmt/f2
					continue
				}
				//log.Printf("The delta depth is: %v", deltaDepth)

				// if we have what we wanted, awesome, send an event...
				if event.Name == safename {
					//log.Println("Event!")
					// TODO: filter out some of the events, is Write a sufficient minimum?
					if event.Op&fsnotify.Write == fsnotify.Write {
						send = true
					}

					// file removed, move the watch upwards
					if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
						//log.Println("Removal!")
						watcher.Remove(current)
						index--
					}

					// we must be a parent watcher, so descend in
					if deltaDepth < 0 {
						watcher.Remove(current)
						index++
					}

					// if safename starts with event.Name, we're above, and no event should be sent
				} else if util.HasPathPrefix(safename, event.Name) {
					//log.Println("Above!")

					if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
						log.Println("Removal!")
						watcher.Remove(current)
						index--
					}

					if deltaDepth < 0 {
						log.Println("Parent!")
						if util.PathPrefixDelta(safename, event.Name) == 1 { // we're the parent dir
							//send = true
						}
						watcher.Remove(current)
						index++
					}

					// if event.Name startswith safename, send event, we're already deeper
				} else if util.HasPathPrefix(event.Name, safename) {
					//log.Println("Event2!")
					//send = true
				}

			case err := <-watcher.Errors:
				log.Printf("error: %v", err)
				log.Fatal(err)

			}

			// do our event sending all together to avoid duplicate msgs
			if send {
				send = false
				ch <- true
			}
		}
		//close(ch)
	}()
	return ch
}
