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
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/fsnotify/fsnotify"
)

// These are variables so that the tests can pretend that users have changed.
var (
	passwdFile     = util.EtcPasswdFile
	expandHomeFunc = util.ExpandHome
)

// Event represents a watcher event. These can include errors.
type Event struct {
	Error error
	Body  *fsnotify.Event
}

// RecWatcher is the struct for the recursive watcher. To start things up call
// Run() on it.
type RecWatcher struct {
	// Path is the computer path that we're watching. It must be absolute,
	// unless the ExpandHome option is used, in which case it can also start
	// with ~/ or ~user/ instead.
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
	events   chan *Event // one channel for events and err...
	wg       *sync.WaitGroup
	cancel   context.CancelFunc // shuts down the watch goroutine
	ready    func(error)
}

// NewRecWatcher creates and initializes a new recursive watcher. This returns
// only once the watcher has started up or errored. This is ergonomic to use
// because you can cancel this by closing the context, or by using the Cleanup
// method.
func NewRecWatcher(ctx context.Context, path string, recurse bool, opts ...Option) (*RecWatcher, error) {
	obj := &RecWatcher{
		Path:    path,
		Recurse: recurse,
		Opts:    opts,
	}
	return obj, obj.Run(ctx)
}

// validatePath checks that our path is one that we know how to watch.
func (obj *RecWatcher) validatePath() error {
	if strings.HasPrefix(obj.Path, "/") {
		return nil
	}
	if !strings.HasPrefix(obj.Path, "~") || !obj.options.expandHome {
		return fmt.Errorf("recwatch: path must be absolute: %s", obj.Path)
	}
	if !strings.Contains(obj.Path, "/") { // eg: ~james
		return fmt.Errorf("recwatch: path must start with ~/ or ~user/: %s", obj.Path)
	}
	return nil
}

// Run starts the recursive file watcher. It returns only once the watcher has
// started up or errored or exited.
func (obj *RecWatcher) Run(ctx context.Context) (reterr error) {
	ctx, obj.cancel = context.WithCancel(ctx)
	defer func() {
		if reterr != nil {
			obj.cancel() // startup failed, nobody will call Cleanup
			obj.wg.Wait()
		}
	}()
	obj.wg = &sync.WaitGroup{}

	obj.options = &recwatchOptions{ // default recwatch options
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

	if err := obj.validatePath(); err != nil {
		return err
	}

	obj.events = make(chan *Event)

	if strings.HasPrefix(obj.Path, "~") { // validated above
		return obj.runTilde(ctx)
	}

	var err error
	obj.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	obj.watches = make(map[string]struct{})
	obj.isDir = strings.HasSuffix(obj.Path, "/") // dirs have trailing slashes
	obj.safename = path.Clean(obj.Path)          // no trailing slash

	if obj.isDir {
		if err := obj.addSubFolders(obj.safename); err != nil {
			// Startup failed before the watch goroutine ran, so we
			// must close this ourselves here for it.
			reterr := errwrap.Append(err, obj.watcher.Close())
			obj.watcher = nil
			return reterr
		}
	}

	ready := make(chan error, 1)
	once := &sync.Once{}
	obj.ready = func(err error) {
		once.Do(func() { ready <- err })
	}

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.events)
		// The watch goroutine owns the watcher for its entire lifetime,
		// so it's usually responsible for closing it.
		defer func() {
			_ = obj.watcher.Close()
		}()
		err := obj.watch(ctx)
		obj.ready(err) // report failure if startup never completed
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			// We're shutting down so this isn't a failure, and the
			// caller already knows from its own ctx.
			return
		}
		select {
		case obj.events <- &Event{Error: err}:
		case <-ctx.Done():
		}
	}()

	select {
	case err := <-ready:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

//func (obj *RecWatcher) Add(path string) error { // XXX: implement me or not?
//
//}
//
//func (obj *RecWatcher) Remove(path string) error { // XXX: implement me or not?
//
//}

// Cleanup shuts the watcher down and waits for it to finish before returning.
func (obj *RecWatcher) Cleanup() {
	obj.cancel()  // signal shutdown...
	obj.wg.Wait() // ...then wait for the watch goroutine to exit
}

// Events returns a channel of events. These include events for errors. An error
// is always a real failure, and it is the last event before the channel closes.
// The channel only closes without such an error when the input ctx was
// cancelled, or when Cleanup was called.
func (obj *RecWatcher) Events() chan *Event { return obj.events }

// watch is the primary listener for this resource and it outputs events.
func (obj *RecWatcher) watch(ctx context.Context) error {

	patharray := util.PathSplit(obj.safename) // tokenize the path
	var index = len(patharray)                // starting index
	var current string                        // current "watcher" location
	var deltaDepth int                        // depth delta between watcher and event
	var send = false                          // send event?
	var body *fsnotify.Event

	for {
		if index < 1 || index > len(patharray) {
			return fmt.Errorf("recwatch: invalid path index %d for %q", index, obj.safename)
		}
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
				if index > 1 {
					index-- // usually not found, move up one dir
				}
				continue
			}

			if err == syscall.ENOSPC {
				// no space left on device, out of inotify watches
				// TODO: consider letting the user fall back to
				// polling if they hit this error very often...
				return fmt.Errorf("out of inotify watches: %v", err)
			}

			if os.IsPermission(err) {
				return fmt.Errorf("permission denied adding a watch: %v", err)
			}

			return fmt.Errorf("unknown error: %v", err)
		}

		obj.ready(nil) // obj.watcher.Add(...) above has now completed...

		select {
		case event, ok := <-obj.watcher.Events:
			if !ok {
				// TODO: return a future ctx.Err() instead?
				return fmt.Errorf("unexpected nil watcher events shutdown")
			}
			//if event == nil { // not possible
			//	return fmt.Errorf("unexpected nil watcher event")
			//}
			body = &event // what body to send?

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
						// best-effort: the watch may already be gone
						_ = obj.watcher.Remove(event.Name)
						delete(obj.watches, event.Name)
					}
					if (event.Op&fsnotify.Create == fsnotify.Create) && isDir(event.Name) {
						// best-effort: re-add on the next event if this fails
						_ = obj.watcher.Add(event.Name)
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
					if index > 1 {
						//obj.options.logf("removal!")
						// best-effort: the watch may already be gone
						_ = obj.watcher.Remove(current)
						index--
					}
				}

				// when the file is moved, remove the watcher and add a new one,
				// so we stop tracking the old inode.
				if deltaDepth >= 0 && (event.Op&fsnotify.Rename == fsnotify.Rename) {
					// best-effort: re-establish the watch on the new inode
					_ = obj.watcher.Remove(current)
					_ = obj.watcher.Add(current)
				}

				// we must be a parent watcher, so descend in
				if deltaDepth < 0 {
					if index < len(patharray) {
						// XXX: we can block here due to: https://github.com/fsnotify/fsnotify/issues/123
						// best-effort: the watch may already be gone
						_ = obj.watcher.Remove(current)
						index++
					}
				}

				goto Send
			}

			// if safename starts with event.Name, we're above, and no event should be sent
			if util.HasPathPrefix(obj.safename, event.Name) {
				//obj.options.logf("above!")

				if deltaDepth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					if index > 1 {
						//obj.options.logf("removal!")
						// best-effort: the watch may already be gone
						_ = obj.watcher.Remove(current)
						index--
					}
				}

				if deltaDepth < 0 {
					//obj.options.logf("parent!")
					if util.PathPrefixDelta(obj.safename, event.Name) == 1 { // we're the parent dir
						send = true
					}
					if index < len(patharray) {
						// XXX: we can block here due to: https://github.com/fsnotify/fsnotify/issues/123
						// best-effort: the watch may already be gone
						_ = obj.watcher.Remove(current)
						index++
					}
				}

				goto Send
			}

			// if event.Name startswith safename, send event, we're already deeper
			if util.HasPathPrefix(event.Name, obj.safename) {
				//obj.options.logf("event2!")
				send = true
				goto Send
			}

		case err, ok := <-obj.watcher.Errors:
			if !ok {
				// TODO: return a future ctx.Err() instead?
				return fmt.Errorf("unexpected nil watcher error shutdown")
			}
			if err == nil {
				// TODO: can this even happen?
				return fmt.Errorf("unexpected nil watcher error")
			}

			return fmt.Errorf("unknown watcher error: %v", err)

		case <-ctx.Done():
			return ctx.Err()
		}

	Send:
		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// only invalid state on certain types of events
			select {
			// exit even when we're blocked on event sending
			case obj.events <- &Event{Body: body}:
			case <-ctx.Done():
				return ctx.Err()
			}
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

// runTilde watches a ~/ or ~user/ path. Until it expands, we treat it as a path
// that doesn't exist yet. We watch the passwd file to find out when that has
// changed, and then move our watch to wherever the path now expands to. That
// move sends an event, since the thing we're watching is now different.
func (obj *RecWatcher) runTilde(ctx context.Context) error {
	opts := []Option{
		Debug(obj.options.debug),
		Logf(obj.options.logf),
	}

	passwd, err := NewRecWatcher(ctx, passwdFile, false, opts...)
	if err != nil {
		close(obj.events) // no goroutine will do it for us
		return err
	}

	current := ""         // the expanded path, or empty if it doesn't expand
	var inner *RecWatcher // watches current, if it's not empty

	// retarget moves our watch to wherever the path now expands to. It
	// returns true if that changed.
	retarget := func() (bool, error) {
		p, err := expandHomeFunc(obj.Path)
		if err != nil { // probably the user doesn't exist yet
			if obj.options.debug {
				obj.options.logf("can't expand %s: %v", obj.Path, err)
			}
			p = ""
		}
		if p == current {
			return false, nil
		}
		if obj.options.debug {
			obj.options.logf("expanded %s to: %s", obj.Path, p)
		}

		if inner != nil {
			inner.Cleanup()
			inner, current = nil, ""
		}
		if p == "" {
			return true, nil
		}
		rw, err := NewRecWatcher(ctx, p, obj.Recurse, opts...)
		if err != nil {
			return false, err
		}
		inner, current = rw, p
		return true, nil
	}

	if _, err := retarget(); err != nil {
		passwd.Cleanup()
		close(obj.events) // no goroutine will do it for us
		return err
	}

	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(obj.events)
		defer passwd.Cleanup()
		defer func() {
			if inner != nil {
				inner.Cleanup()
			}
		}()

		for {
			var innerEvents chan *Event // nil (blocks forever) if unused
			if inner != nil {
				innerEvents = inner.Events()
			}

			var event *Event
			select {
			case e, ok := <-passwd.Events():
				if !ok { // we're shutting down
					return
				}
				event = e
				if e.Error != nil {
					break
				}
				changed, err := retarget()
				if err != nil && ctx.Err() != nil {
					// We're shutting down so this isn't a
					// failure and the caller already knows.
					return
				}
				if err != nil {
					event = &Event{Error: err}
				} else if !changed {
					continue // nothing happened to our path
				}

			case e, ok := <-innerEvents:
				if !ok { // we're shutting down
					return
				}
				event = e

			case <-ctx.Done():
				return
			}

			select {
			case obj.events <- event:
			case <-ctx.Done():
				return
			}
			if event.Error != nil {
				return // that was the last one
			}
		}
	}()

	return nil
}

// Option is a type that can be used to configure the recwatcher.
type Option func(*recwatchOptions)

// recwatchOptions is the collections of options that users can set on recwatch.
type recwatchOptions struct {
	expandHome bool

	debug bool
	logf  func(format string, v ...interface{})
	// TODO: add more options
}

// ExpandHome lets the path start with ~/ or ~user/. This is watched even if
// that user doesn't exist yet, and the watch follows their home directory if it
// changes.
func ExpandHome(expandHome bool) Option {
	return func(rwo *recwatchOptions) {
		rwo.expandHome = expandHome
	}
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

// isDir is a simple helper to determine if a path is a dir on disk or not.
func isDir(path string) bool {
	finfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return finfo.IsDir()
}

// MergeChannels is a helper function to combine different recwatch events. It
// would be preferable to actually fix the horrible recwatch code to allow it to
// monitor more than one path from the start. An error is the last event before
// the channel closes, and it only closes without one when ctx was cancelled, or
// when all of the inputs closed without one. Cancel ctx when you stop reading
// so that nothing stays blocked.
func MergeChannels(ctx context.Context, chanList ...<-chan *Event) <-chan *Event {
	ctx, cancel := context.WithCancel(ctx)

	in := make(chan *Event)
	wg := &sync.WaitGroup{}
	wg.Add(len(chanList)) // do them all together
	for _, ch := range chanList {
		//wg.Add(1)
		go func(ch <-chan *Event) {
			defer wg.Done()
			for v := range ch {
				select {
				case in <- v:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}
	go func() {
		wg.Wait()
		close(in) // nothing can send on this anymore
	}()

	// A single sender lets us guarantee that nothing follows an error.
	out := make(chan *Event)
	go func() {
		defer close(out)
		defer cancel() // if one of them fails, stop the others too
		for v := range in {
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
			if v != nil && v.Error != nil {
				return // that was the last one
			}
		}
	}()
	return out
}
