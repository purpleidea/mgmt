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
	"gopkg.in/fsnotify.v1"
	//"github.com/go-fsnotify/fsnotify" // git master of "gopkg.in/fsnotify.v1"
	"log"
	"math"
	"path"
	"strings"
	"syscall"
)

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

		patharray := PathSplit(safename) // tokenize the path
		var index = len(patharray)       // starting index
		var current string               // current "watcher" location
		var deltaDepth int               // depth delta between watcher and event
		var send = false                 // send event?

		for {
			current = strings.Join(patharray[0:index], "/")
			if current == "" { // the empty string top is the root dir ("/")
				current = "/"
			}
			log.Printf("Watching: %v", current) // attempting to watch...

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

				} else if HasPathPrefix(event.Name, current) {
					deltaDepth = len(PathSplit(current)) - len(PathSplit(event.Name)) // -1 or less

				} else if HasPathPrefix(current, event.Name) {
					deltaDepth = len(PathSplit(event.Name)) - len(PathSplit(current)) // +1 or more

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
					send = true

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
				} else if HasPathPrefix(safename, event.Name) {
					//log.Println("Above!")

					if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
						log.Println("Removal!")
						watcher.Remove(current)
						index--
					}

					if deltaDepth < 0 {
						log.Println("Parent!")
						if PathPrefixDelta(safename, event.Name) == 1 { // we're the parent dir
							//send = true
						}
						watcher.Remove(current)
						index++
					}

					// if event.Name startswith safename, send event, we're already deeper
				} else if HasPathPrefix(event.Name, safename) {
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
