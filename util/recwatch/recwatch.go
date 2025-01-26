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

// Package recwatch provides recursive file watching events via fsnotify.
package recwatch

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/util"

	"github.com/fsnotify/fsnotify"
)

// Event represents a watcher event. These can include errors.
type Event struct {
	Error error
	Body  *fsnotify.Event
}

// RecWatcher is the struct for the recursive watcher. Run Init() on it.
type RecWatcher struct {
	// Path is the computer path that we're watching.
	Path string

	// Recurse specifies if we should watch recursively.
	// TODO: Should this get converted into an Option?
	Recurse bool

	// Opts are the list of options that we are using this with.
	Opts []Option

	options  *recwatchOptions // computed options
	isDir    bool             // computed isDir
	safename string           // safe path
	watcher  *fsnotify.Watcher
	watches  map[string]struct{}
	events   chan Event // one channel for events and err...
	closed   bool       // is the events channel closed?
	mutex    sync.Mutex // lock guarding the channel closing
	wg       sync.WaitGroup
	exit     chan struct{}
}

// NewRecWatcher creates an initializes a new recursive watcher.
func NewRecWatcher(path string, recurse bool, opts ...Option) (*RecWatcher, error) {
	obj := &RecWatcher{
		Path:    path,
		Recurse: recurse,
		Opts:    opts,
	}
	return obj, obj.Init()
}

// Init starts the recursive file watcher.
func (obj *RecWatcher) Init() error {
	obj.watcher = nil
	obj.watches = make(map[string]struct{})
	obj.events = make(chan Event)
	obj.exit = make(chan struct{})
	obj.isDir = strings.HasSuffix(obj.Path, "/") // dirs have trailing slashes
	obj.safename = path.Clean(obj.Path)          // no trailing slash
	obj.options = &recwatchOptions{              // default recwatch options
		debug: false,
		logf: func(format string, v ...interface{}) {
			// noop
		},
	}
	for _, optionFunc := range obj.Opts { // apply the recwatch options
		optionFunc(obj.options)
	}

	if obj.options.logf == nil {
		return fmt.Errorf("recwatch: logf must not be nil")
	}

	var err error
	obj.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if obj.isDir {
		if err := obj.addSubFolders(obj.safename); err != nil {
			return err
		}
	}

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		if err := obj.Watch(); err != nil {
			// we need this mutex, because if we Init and then Close
			// immediately, this can send after closed which panics!
			obj.mutex.Lock()
			if !obj.closed {
				select {
				// TODO: &Event instead?
				case obj.events <- Event{Error: err}:
				case <-obj.exit:
					// pass
				}
			}
			obj.mutex.Unlock()
		}
	}()
	return nil
}

//func (obj *RecWatcher) Add(path string) error { // XXX: implement me or not?
//
//}
//
//func (obj *RecWatcher) Remove(path string) error { // XXX: implement me or not?
//
//}

// Close shuts down the watcher.
func (obj *RecWatcher) Close() error {
	var err error
	close(obj.exit) // send exit signal
	obj.wg.Wait()
	if obj.watcher != nil {
		err = obj.watcher.Close()
		obj.watcher = nil
	}
	obj.mutex.Lock() // FIXME: I don't think this mutex is needed anymore...
	obj.closed = true
	close(obj.events)
	obj.mutex.Unlock()
	return err
}

// Events returns a channel of events. These include events for errors.
func (obj *RecWatcher) Events() chan Event { return obj.events }

// Watch is the primary listener for this resource and it outputs events.
func (obj *RecWatcher) Watch() error {
	if obj.watcher == nil {
		return fmt.Errorf("the watcher is not initialized")
	}

	patharray := util.PathSplit(obj.safename) // tokenize the path
	var index = len(patharray)                // starting index
	var current string                        // current "watcher" location
	var deltaDepth int                        // depth delta between watcher and event
	var send = false                          // send event?

	for {
		current = strings.Join(patharray[0:index], "/")
		if current == "" { // the empty string top is the root dir ("/")
			current = "/"
		}
		if obj.options.debug {
			obj.options.logf("watching: %s", current) // attempting to watch...
		}
		// initialize in the loop so that we can reset on rm-ed handles
		if err := obj.watcher.Add(current); err != nil {
			if obj.options.debug {
				obj.options.logf("watcher.Add(%s): Error: %v", current, err)
			}
			// ENOENT for linux, etc and IsNotExist for macOS
			if err == syscall.ENOENT || os.IsNotExist(err) {
				index-- // usually not found, move up one dir
				index = int(math.Max(1, float64(index)))
				continue
			}

			if err == syscall.ENOSPC {
				// no space left on device, out of inotify watches
				// TODO: consider letting the user fall back to
				// polling if they hit this error very often...
				return fmt.Errorf("out of inotify watches: %v", err)
			} else if os.IsPermission(err) {
				return fmt.Errorf("permission denied adding a watch: %v", err)
			}
			return fmt.Errorf("unknown error: %v", err)
		}

		select {
		case event := <-obj.watcher.Events:
			if obj.options.debug {
				obj.options.logf("watch(%s), event(%s): %v", current, event.Name, event.Op)
			}
			// the deeper you go, the bigger the deltaDepth is...
			// this is the difference between what we're watching,
			// and the event... doesn't mean we can't watch deeper
			if current == event.Name {
				deltaDepth = 0 // i was watching what i was looking for

			} else if util.HasPathPrefix(event.Name, current) {
				deltaDepth = len(util.PathSplit(current)) - len(util.PathSplit(event.Name)) // -1 or less

			} else if util.HasPathPrefix(current, event.Name) {
				deltaDepth = len(util.PathSplit(event.Name)) - len(util.PathSplit(current)) // +1 or more
				// if below me...
				if _, exists := obj.watches[event.Name]; exists {
					send = true
					if event.Op&fsnotify.Remove == fsnotify.Remove {
						obj.watcher.Remove(event.Name)
						delete(obj.watches, event.Name)
					}
					if (event.Op&fsnotify.Create == fsnotify.Create) && isDir(event.Name) {
						obj.watcher.Add(event.Name)
						obj.watches[event.Name] = struct{}{}
						if err := obj.addSubFolders(event.Name); err != nil {
							return err
						}
					}
				}

			} else {
				// TODO: different watchers get each others events!
				// https://github.com/fsnotify/fsnotify/issues/95
				// this happened with two values such as:
				// event.Name: /tmp/mgmt/f3 and current: /tmp/mgmt/f2
				continue
			}
			//obj.options.logf("the delta depth is: %v", deltaDepth)

			// if we have what we wanted, awesome, send an event...
			if event.Name == obj.safename {
				//obj.options.logf("event!")
				// FIXME: should all these below cases trigger?
				send = true

				if obj.isDir {
					if err := obj.addSubFolders(obj.safename); err != nil {
						return err
					}
				}

				// file removed, move the watch upwards
				if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					//obj.options.logf("removal!")
					obj.watcher.Remove(current)
					index--
				}

				// when the file is moved, remove the watcher and add a new one,
				// so we stop tracking the old inode.
				if deltaDepth >= 0 && (event.Op&fsnotify.Rename == fsnotify.Rename) {
					obj.watcher.Remove(current)
					obj.watcher.Add(current)
				}

				// we must be a parent watcher, so descend in
				if deltaDepth < 0 {
					// XXX: we can block here due to: https://github.com/fsnotify/fsnotify/issues/123
					obj.watcher.Remove(current)
					index++
				}

				// if safename starts with event.Name, we're above, and no event should be sent
			} else if util.HasPathPrefix(obj.safename, event.Name) {
				//obj.options.logf("above!")

				if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					//obj.options.logf("removal!")
					obj.watcher.Remove(current)
					index--
				}

				if deltaDepth < 0 {
					//obj.options.logf("parent!")
					if util.PathPrefixDelta(obj.safename, event.Name) == 1 { // we're the parent dir
						send = true
					}
					obj.watcher.Remove(current)
					index++
				}

				// if event.Name startswith safename, send event, we're already deeper
			} else if util.HasPathPrefix(event.Name, obj.safename) {
				//obj.options.logf("event2!")
				send = true
			}

			// do all our event sending all together to avoid duplicate msgs
			if send {
				send = false
				// only invalid state on certain types of events
				select {
				// exit even when we're blocked on event sending
				// TODO: &Event instead?
				case obj.events <- Event{Error: nil, Body: &event}:
				case <-obj.exit:
					return fmt.Errorf("pending event not sent")
				}
			}

		case err := <-obj.watcher.Errors:
			return fmt.Errorf("unknown watcher error: %v", err)

		case <-obj.exit:
			return nil
		}
	}
}

// addSubFolders is a helper that is used to add recursive dirs to the watches.
func (obj *RecWatcher) addSubFolders(p string) error {
	if !obj.Recurse {
		return nil // if we're not watching recursively, just exit early
	}
	// look at all subfolders...
	walkFn := func(path string, info os.FileInfo, err error) error {
		if obj.options.debug {
			obj.options.logf("walk: %s (%v): %v", path, info, err)
		}
		if err != nil {
			return nil
		}
		if info.IsDir() {
			obj.watches[path] = struct{}{} // add key
			err := obj.watcher.Add(path)
			if err != nil {
				return err // TODO: will this bubble up?
			}
		}
		return nil
	}
	err := filepath.Walk(p, walkFn)
	return err
}

// Option is a type that can be used to configure the recwatcher.
type Option func(*recwatchOptions)

type recwatchOptions struct {
	debug bool
	logf  func(format string, v ...interface{})
	// TODO: add more options
}

// Debug specifies whether we should run in debug mode or not.
func Debug(debug bool) Option {
	return func(rwo *recwatchOptions) {
		rwo.debug = debug
	}
}

// Logf passes a logger function that we can use if so desired.
func Logf(logf func(format string, v ...interface{})) Option {
	return func(rwo *recwatchOptions) {
		rwo.logf = logf
	}
}

func isDir(path string) bool {
	finfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return finfo.IsDir()
}

// MergeChannels is a helper function to combine different recwatch events. It
// would be preferable to actually fix the horrible recwatch code to allow it to
// monitor more than one path from the start.
func MergeChannels(chanList ...<-chan Event) <-chan Event {
	out := make(chan Event)
	wg := &sync.WaitGroup{}
	wg.Add(len(chanList)) // do them all together
	for _, ch := range chanList {
		//wg.Add(1)
		go func(ch <-chan Event) {
			defer wg.Done()
			for v := range ch {
				out <- v
			}
		}(ch)
	}
	go func() {
		// Last one closes!
		wg.Wait()
		close(out)
	}()
	return out
}
