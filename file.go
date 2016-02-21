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
	return &FileRes{
		BaseRes: BaseRes{
			Name:   name,
			events: make(chan Event),
			vertex: nil,
		},
		Path:      path,
		Dirname:   dirname,
		Basename:  basename,
		Content:   content,
		State:     state,
		sha256sum: "",
	}
}

func (obj *FileRes) GetRes() string {
	return "File"
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
				log.Printf("Lack of watches for file[%v] error: %+v", obj.Name, err.Error) // 0x408da0
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

// FIXME: add the obj.CleanState() calls all over the true returns!
func (obj *FileRes) StateOK() bool {
	if obj.isStateOK { // cache the state
		return true
	}

	if _, err := os.Stat(obj.GetPath()); os.IsNotExist(err) {
		// no such file or directory
		if obj.State == "absent" {
			return obj.CleanState() // missing file should be missing, phew :)
		} else {
			// state invalid, skip expensive checksums
			return false
		}
	}

	// TODO: add file mode check here...

	if PathIsDir(obj.GetPath()) {
		return obj.StateOKDir()
	} else {
		return obj.StateOKFile()
	}
}

func (obj *FileRes) StateOKFile() bool {
	if PathIsDir(obj.GetPath()) {
		log.Fatal("This should only be called on a File resource.")
	}

	// run a diff, and return true if needs changing

	hash := sha256.New()

	f, err := os.Open(obj.GetPath())
	if err != nil {
		//log.Fatal(err)
		return false
	}
	defer f.Close()

	if _, err := io.Copy(hash, f); err != nil {
		//log.Fatal(err)
		return false
	}

	sha256sum := hex.EncodeToString(hash.Sum(nil))
	//log.Printf("sha256sum: %v", sha256sum)

	if obj.HashSHA256fromContent() == sha256sum {
		return true
	}

	return false
}

func (obj *FileRes) StateOKDir() bool {
	if !PathIsDir(obj.GetPath()) {
		log.Fatal("This should only be called on a Dir resource.")
	}

	// XXX: not implemented
	log.Fatal("Not implemented!")
	return false
}

func (obj *FileRes) Apply() bool {
	log.Printf("%v[%v]: Apply", obj.GetRes(), obj.GetName())

	if PathIsDir(obj.GetPath()) {
		return obj.ApplyDir()
	} else {
		return obj.ApplyFile()
	}
}

func (obj *FileRes) ApplyFile() bool {

	if PathIsDir(obj.GetPath()) {
		log.Fatal("This should only be called on a File resource.")
	}

	if obj.State == "absent" {
		log.Printf("About to remove: %v", obj.GetPath())
		err := os.Remove(obj.GetPath())
		if err != nil {
			return false
		}
		return true
	}

	//log.Println("writing: " + filename)
	f, err := os.Create(obj.GetPath())
	if err != nil {
		log.Println("error:", err)
		return false
	}
	defer f.Close()

	_, err = io.WriteString(f, obj.Content)
	if err != nil {
		log.Println("error:", err)
		return false
	}

	return true
}

func (obj *FileRes) ApplyDir() bool {
	if !PathIsDir(obj.GetPath()) {
		log.Fatal("This should only be called on a Dir resource.")
	}

	// XXX: not implemented
	log.Fatal("Not implemented!")
	return true
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
