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
	"crypto/sha256"
	"encoding/hex"
	"gopkg.in/fsnotify.v1"
	//"github.com/go-fsnotify/fsnotify" // git master of "gopkg.in/fsnotify.v1"
	"io"
	"log"
	"math"
	"os"
	"path"
	"strings"
	"syscall"
)

type FileRes struct {
	BaseRes   `yaml:",inline"`
	Path      string `yaml:"path"` // path variable (should default to name)
	Dirname   string `yaml:"dirname"`
	Basename  string `yaml:"basename"`
	Content   string `yaml:"content"`
	State     string `yaml:"state"` // state: exists/present?, absent, (undefined?)
	sha256sum string
}

func NewFileRes(name, path, dirname, basename, content, state string) *FileRes {
	// FIXME if path = nil, path = name ...
	obj := &FileRes{
		BaseRes: BaseRes{
			Name: name,
		},
		Path:      path,
		Dirname:   dirname,
		Basename:  basename,
		Content:   content,
		State:     state,
		sha256sum: "",
	}
	obj.Init()
	return obj
}

func (obj *FileRes) Init() {
	obj.BaseRes.kind = "File"
	obj.BaseRes.Init() // call base init, b/c we're overriding
}

func (obj *FileRes) GetPath() string {
	d := Dirname(obj.Path)
	b := Basename(obj.Path)
	if !obj.Validate() || (obj.Dirname == "" && obj.Basename == "") {
		return obj.Path
	} else if obj.Dirname == "" {
		return d + obj.Basename
	} else if obj.Basename == "" {
		return obj.Dirname + b
	} else { // if obj.dirname != "" && obj.basename != "" {
		return obj.Dirname + obj.Basename
	}
}

// validate if the params passed in are valid data
func (obj *FileRes) Validate() bool {
	if obj.Dirname != "" {
		// must end with /
		if obj.Dirname[len(obj.Dirname)-1:] != "/" {
			return false
		}
	}
	if obj.Basename != "" {
		// must not start with /
		if obj.Basename[0:1] == "/" {
			return false
		}
	}
	return true
}

// File watcher for files and directories
// Modify with caution, probably important to write some test cases first!
// obj.GetPath(): file or directory
func (obj *FileRes) Watch() {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)

	//var recursive bool = false
	//var isdir = (obj.GetPath()[len(obj.GetPath())-1:] == "/") // dirs have trailing slashes
	//log.Printf("IsDirectory: %v", isdir)
	//vertex := obj.GetVertex()         // stored with SetVertex
	var safename = path.Clean(obj.GetPath()) // no trailing slash

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
	var exit = false
	var dirty = false

	for {
		current = strings.Join(patharray[0:index], "/")
		if current == "" { // the empty string top is the root dir ("/")
			current = "/"
		}
		if DEBUG {
			log.Printf("File[%v]: Watching: %v", obj.GetName(), current) // attempting to watch...
		}
		// initialize in the loop so that we can reset on rm-ed handles
		err = watcher.Add(current)
		if err != nil {
			if DEBUG {
				log.Printf("File[%v]: watcher.Add(%v): Error: %v", obj.GetName(), current, err)
			}
			if err == syscall.ENOENT {
				index-- // usually not found, move up one dir
			} else if err == syscall.ENOSPC {
				// XXX: occasionally: no space left on device,
				// XXX: probably due to lack of inotify watches
				log.Printf("%v[%v]: Out of inotify watches!", obj.Kind(), obj.GetName())
				log.Fatal(err)
			} else {
				log.Printf("Unknown file[%v] error:", obj.Name)
				log.Fatal(err)
			}
			index = int(math.Max(1, float64(index)))
			continue
		}

		obj.SetState(resStateWatching) // reset
		select {
		case event := <-watcher.Events:
			if DEBUG {
				log.Printf("File[%v]: Watch(%v), Event(%v): %v", obj.GetName(), current, event.Name, event.Op)
			}
			obj.SetConvergedState(resConvergedNil) // XXX: technically i can detect if the event is erroneous or not first
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
				// FIXME: should all these below cases trigger?
				send = true
				dirty = true

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
						send = true
						dirty = true
					}
					watcher.Remove(current)
					index++
				}

				// if event.Name startswith safename, send event, we're already deeper
			} else if HasPathPrefix(event.Name, safename) {
				//log.Println("Event2!")
				send = true
				dirty = true
			}

		case err := <-watcher.Errors:
			obj.SetConvergedState(resConvergedNil) // XXX ?
			log.Println("error:", err)
			log.Fatal(err)
			//obj.events <- fmt.Sprintf("file: %v", "error") // XXX: how should we handle errors?

		case event := <-obj.events:
			obj.SetConvergedState(resConvergedNil)
			if exit, send = obj.ReadEvent(&event); exit {
				return // exit
			}
			//dirty = false // these events don't invalidate state

		case _ = <-TimeAfterOrBlock(obj.ctimeout):
			obj.SetConvergedState(resConvergedTimeout)
			obj.converged <- true
			continue
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			// only invalid state on certain types of events
			if dirty {
				dirty = false
				obj.isStateOK = false // something made state dirty
			}
			Process(obj) // XXX: rename this function
		}
	}
}

func (obj *FileRes) HashSHA256fromContent() string {
	if obj.sha256sum != "" { // return if already computed
		return obj.sha256sum
	}

	hash := sha256.New()
	hash.Write([]byte(obj.Content))
	obj.sha256sum = hex.EncodeToString(hash.Sum(nil))
	return obj.sha256sum
}

func (obj *FileRes) FileHashSHA256Check() (bool, error) {
	if PathIsDir(obj.GetPath()) { // assert
		log.Fatal("This should only be called on a File resource.")
	}
	// run a diff, and return true if it needs changing
	hash := sha256.New()
	f, err := os.Open(obj.GetPath())
	if err != nil {
		if e, ok := err.(*os.PathError); ok && (e.Err.(syscall.Errno) == syscall.ENOENT) {
			return false, nil // no "error", file is just absent
		}
		return false, err
	}
	defer f.Close()
	if _, err := io.Copy(hash, f); err != nil {
		return false, err
	}
	sha256sum := hex.EncodeToString(hash.Sum(nil))
	//log.Printf("sha256sum: %v", sha256sum)
	if obj.HashSHA256fromContent() == sha256sum {
		return true, nil
	}
	return false, nil
}

func (obj *FileRes) FileApply() error {
	if PathIsDir(obj.GetPath()) {
		log.Fatal("This should only be called on a File resource.")
	}

	if obj.State == "absent" {
		log.Printf("About to remove: %v", obj.GetPath())
		err := os.Remove(obj.GetPath())
		return err // either nil or not, for success or failure
	}

	f, err := os.Create(obj.GetPath())
	if err != nil {
		return nil
	}
	defer f.Close()

	_, err = io.WriteString(f, obj.Content)
	if err != nil {
		return err
	}

	return nil // success
}

func (obj *FileRes) CheckApply(apply bool) (stateok bool, err error) {
	log.Printf("%v[%v]: CheckApply(%t)", obj.Kind(), obj.GetName(), apply)

	if obj.isStateOK { // cache the state
		return true, nil
	}

	if _, err = os.Stat(obj.GetPath()); os.IsNotExist(err) {
		// no such file or directory
		if obj.State == "absent" {
			// missing file should be missing, phew :)
			obj.isStateOK = true
			return true, nil
		}
	}
	err = nil // reset

	// FIXME: add file mode check here...

	if PathIsDir(obj.GetPath()) {
		log.Fatal("Not implemented!") // XXX
	} else {
		ok, err := obj.FileHashSHA256Check()
		if err != nil {
			return false, err
		}
		if ok {
			obj.isStateOK = true
			return true, nil
		}
		// if no err, but !ok, then we continue on...
	}

	// state is not okay, no work done, exit, but without error
	if !apply {
		return false, nil
	}

	// apply portion
	log.Printf("%v[%v]: Apply", obj.Kind(), obj.GetName())
	if PathIsDir(obj.GetPath()) {
		log.Fatal("Not implemented!") // XXX
	} else {
		err = obj.FileApply()
		if err != nil {
			return false, err
		}
	}

	obj.isStateOK = true
	return false, nil // success
}


type FileUUID struct {
	BaseUUID
	path string
}

// if and only if they are equivalent, return true
// if they are not equivalent, return false
func (obj *FileUUID) IFF(uuid ResUUID) bool {
	res, ok := uuid.(*FileUUID)
	if !ok {
		return false
	}
	return obj.path == res.path
}

type FileResAutoEdges struct {
	data    []ResUUID
	pointer int
	found   bool
}

func (obj *FileResAutoEdges) Next() []ResUUID {
	if obj.found {
		log.Fatal("Shouldn't be called anymore!")
	}
	if len(obj.data) == 0 { // check length for rare scenarios
		return nil
	}
	value := obj.data[obj.pointer]
	obj.pointer += 1
	return []ResUUID{value} // we return one, even though api supports N
}

// get results of the earlier Next() call, return if we should continue!
func (obj *FileResAutoEdges) Test(input []bool) bool {
	// if there aren't any more remaining
	if len(obj.data) <= obj.pointer {
		return false
	}
	if obj.found { // already found, done!
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		log.Fatal("Expecting a single value!")
	}
	if input[0] { // if a match is found, we're done!
		obj.found = true // no more to find!
		return false
	}
	return true // keep going
}

// generate a simple linear sequence of each parent directory from bottom up!
func (obj *FileRes) AutoEdges() AutoEdge {
	var data []ResUUID                             // store linear result chain here...
	values := PathSplitFullReversed(obj.GetPath()) // build it
	_, values = values[0], values[1:]              // get rid of first value which is me!
	for _, x := range values {
		var reversed bool = true // cheat by passing a pointer
		data = append(data, &FileUUID{
			BaseUUID: BaseUUID{
				name:     obj.GetName(),
				kind:     obj.Kind(),
				reversed: &reversed,
			},
			path: x, // what matters
		}) // build list
	}
	return &FileResAutoEdges{
		data:    data,
		pointer: 0,
		found:   false,
	}
}

func (obj *FileRes) GetUUIDs() []ResUUID {
	x := &FileUUID{
		BaseUUID: BaseUUID{name: obj.GetName(), kind: obj.Kind()},
		path:     obj.GetPath(),
	}
	return []ResUUID{x}
}

func (obj *FileRes) Compare(res Res) bool {
	switch res.(type) {
	case *FileRes:
		res := res.(*FileRes)
		if obj.Name != res.Name {
			return false
		}
		if obj.GetPath() != res.Path {
			return false
		}
		if obj.Content != res.Content {
			return false
		}
		if obj.State != res.State {
			return false
		}
	default:
		return false
	}
	return true
}
