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
	"encoding/gob"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"syscall"
)

func init() {
	gob.Register(&FileRes{})
}

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
func (obj *FileRes) Watch(processChan chan Event) {
	if obj.IsWatching() {
		return
	}
	obj.SetWatching(true)
	defer obj.SetWatching(false)
	cuuid := obj.converger.Register()
	defer cuuid.Unregister()

	//var recursive bool = false
	//var isdir = (obj.GetPath()[len(obj.GetPath())-1:] == "/") // dirs have trailing slashes
	//log.Printf("IsDirectory: %v", isdir)
	var cleanObjPath = path.Clean(obj.GetPath()) // no trailing slash

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	patharray := PathSplit(cleanObjPath)[1:] // tokenize the path
	var objDepth = len(patharray)            // total elements in the path, will not change
	var watchDepth = len(patharray)          // elements in the path to the current watch
	var eventDepth int                       // elements in the path to what triggered the most recent event
	var send = false                         // send event?
	var exit = false
	var dirty = false
	var watchPath string
	var watching = false

	for {
		watchPath = "/" + strings.Join(patharray[0:watchDepth], "/")

		if DEBUG {
			log.Printf("File[%v]: Watching: %v", obj.GetName(), watchPath) // attempting to watch...
		}

		if !watching {
			err = watcher.Add(watchPath)
			if err != nil {
				if DEBUG {
					log.Printf("File[%v]: watcher.Add(%v): Error: %v", obj.GetName(), watchPath, err)
				}
				if err == syscall.ENOENT {
					watchDepth-- // usually not found, move up one dir
					if watchDepth < 0 {
						log.Fatal("somehow trying to watch file above the fs root")
					}
					continue
				}
				if err == syscall.ENOSPC {
					// XXX: occasionally: no space left on device,
					// XXX: probably due to lack of inotify watches
					log.Printf("%v[%v]: Out of inotify watches!", obj.Kind(), obj.GetName())
				} else {
					log.Printf("Unknown file[%v] error:", obj.Name)
				}
				log.Fatal(err)
			}
			watching = true
		}

		obj.SetState(resStateWatching) // reset
		select {
		case event := <-watcher.Events:
			if DEBUG {
				log.Printf("File[%v]: Watch(%v), Event(%v): %v", obj.GetName(), watchPath, event.Name, event.Op)
			}

			if !HasPathPrefix(event.Name, cleanObjPath) && !HasPathPrefix(cleanObjPath, event.Name) {
				// TODO different watchers get each others events!
				// https://github.com/go-fsnotify/fsnotify/issues/95
				// this happened with two values such as:
				// event.Name: /tmp/mgmt/f3 and watchPath: /tmp/mgmt/f2
				if DEBUG {
					log.Printf("File[%v]: ignoring event, it's not related", obj.GetName)
				}
				continue
			}

			if DEBUG {
				log.Printf("File[%v]: event depth %v, watch depth %v", obj.GetName(), eventDepth, watchDepth)
			}

			cuuid.SetConverged(false)
			eventDepth = len(PathSplit(event.Name)[1:])
			// reset the watch
			watcher.Remove(watchPath)
			watchDepth = objDepth
			watching = false

			if eventDepth == objDepth {
				// this event was triggered by the managed file: send an event
				send = true
				dirty = true
			}

			// were we watching an ancestor?
			if eventDepth < objDepth {

				if event.Op&fsnotify.Remove == 0 && objDepth-eventDepth == 1 {
					// event from the parent of the managed file
					// that is not its removal
					log.Printf("File[%v]: Parent event!", obj.GetName())
					send = true
					dirty = true
				}
			}

			// event from within a managed directory tree?
			if eventDepth > objDepth {
				//log.Println("Event2!")
				send = true
				dirty = true
			}

		case err := <-watcher.Errors:
			cuuid.SetConverged(false) // XXX ?
			log.Printf("error: %v", err)
			log.Fatal(err)
			//obj.events <- fmt.Sprintf("file: %v", "error") // XXX: how should we handle errors?

		case event := <-obj.events:
			cuuid.SetConverged(false)
			if exit, send = obj.ReadEvent(&event); exit {
				return // exit
			}
			//dirty = false // these events don't invalidate state

		case _ = <-cuuid.ConvergedTimer():
			cuuid.SetConverged(true) // converged!
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
			resp := NewResp()
			processChan <- Event{eventNil, resp, "", true} // trigger process
			resp.ACKWait()                                 // wait for the ACK()
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

func (obj *FileRes) CheckApply(apply bool) (checkok bool, err error) {
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
	obj.pointer++
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
		var reversed = true // cheat by passing a pointer
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

func (obj *FileRes) GroupCmp(r Res) bool {
	_, ok := r.(*FileRes)
	if !ok {
		return false
	}
	// TODO: we might be able to group directory children into a single
	// recursive watcher in the future, thus saving fanotify watches
	return false // not possible atm
}

func (obj *FileRes) Compare(res Res) bool {
	switch res.(type) {
	case *FileRes:
		res := res.(*FileRes)
		if !obj.BaseRes.Compare(res) { // call base Compare
			return false
		}

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

func (obj *FileRes) CollectPattern(pattern string) {
	// XXX: currently the pattern for files can only override the Dirname variable :P
	obj.Dirname = pattern // XXX: simplistic for now
}
