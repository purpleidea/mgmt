// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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
	"code.google.com/p/go-uuid/uuid"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

type FileType struct {
	uuid      string
	Type      string      // always "file"
	Name      string      // name variable
	Events    chan string // FIXME: eventually a struct for the event?
	Path      string      // path variable (should default to name)
	Content   string
	State     string // state: exists/present?, absent, (undefined?)
	sha256sum string
}

func NewFileType(name, path, content, state string) *FileType {
	// FIXME if path = nil, path = name ...
	return &FileType{
		uuid:      uuid.New(),
		Type:      "file",
		Name:      name,
		Events:    make(chan string, 1), // XXX: chan size?
		Path:      path,
		Content:   content,
		State:     state,
		sha256sum: "",
	}
}

// File watcher for files and directories
// Modify with caution, probably important to write some test cases first!
func (obj FileType) Watch(v *Vertex) {
	// obj.Path: file or directory
	//var recursive bool = false
	//var isdir = (obj.Path[len(obj.Path)-1:] == "/") // dirs have trailing slashes
	//fmt.Printf("IsDirectory: %v\n", isdir)

	var safename = path.Clean(obj.Path) // no trailing slash

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	patharray := PathSplit(safename) // tokenize the path
	var index = len(patharray)       // starting index
	var current string               // current "watcher" location
	var delta_depth int              // depth delta between watcher and event
	var send = false                 // send event?
	var extraCheck = false

	for {
		current = strings.Join(patharray[0:index], "/")
		if current == "" { // the empty string top is the root dir ("/")
			current = "/"
		}
		log.Printf("Watching: %v\n", current) // attempting to watch...

		// initialize in the loop so that we can reset on rm-ed handles
		err = watcher.Add(current)
		if err != nil {
			if err == syscall.ENOENT {
				index-- // usually not found, move up one dir
			} else if err == syscall.ENOSPC {
				// XXX: i sometimes see: no space left on device
				// XXX: why causes this to happen ?
				log.Printf("Strange file[%v] error: %+v\n", obj.Name, err.Error) // 0x408da0
				log.Fatal(err)
			} else {
				log.Printf("Unknown file[%v] error:\n", obj.Name)
				log.Fatal(err)
			}
			index = int(math.Max(1, float64(index)))
			continue
		}

		// XXX: check state after inotify started
		// SMALL RACE: after we terminate watch, till when it's started
		// something could have gotten created/changed/etc... right?
		if extraCheck {
			extraCheck = false
			// XXX
			//if exists ... {
			//	send signal
			//	continue
			//	change index? i don't think so. be thorough and check
			//}
		}

		select {
		case event := <-watcher.Events:
			// the deeper you go, the bigger the delta_depth is...
			// this is the difference between what we're watching,
			// and the event... doesn't mean we can't watch deeper
			if current == event.Name {
				delta_depth = 0 // i was watching what i was looking for

			} else if HasPathPrefix(event.Name, current) {
				delta_depth = len(PathSplit(current)) - len(PathSplit(event.Name)) // -1 or less

			} else if HasPathPrefix(current, event.Name) {
				delta_depth = len(PathSplit(event.Name)) - len(PathSplit(current)) // +1 or more

			} else {
				// XXX multiple watchers receive each others events
				// https://github.com/go-fsnotify/fsnotify/issues/95
				// this happened with two values such as:
				// event.Name: /tmp/mgmt/f3 and current: /tmp/mgmt/f2
				// are the different watchers getting each others events??
				//log.Printf("The delta depth is NaN...\n")
				//log.Printf("Value of event.Name is: %v\n", event.Name)
				//log.Printf("........    current is: %v\n", current)
				//log.Fatal("The delta depth is NaN!")
				continue
			}
			//log.Printf("The delta depth is: %v\n", delta_depth)

			// if we have what we wanted, awesome, send an event...
			if event.Name == safename {
				//log.Println("Event!")
				send = true

				// file removed, move the watch upwards
				if delta_depth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					//log.Println("Removal!")
					watcher.Remove(current)
					index--
				}

				// we must be a parent watcher, so descend in
				if delta_depth < 0 {
					watcher.Remove(current)
					index++
				}

				// if safename starts with event.Name, we're above, and no event should be sent
			} else if HasPathPrefix(safename, event.Name) {
				//log.Println("Above!")

				if delta_depth >= 0 && (event.Op&fsnotify.Remove == fsnotify.Remove) {
					log.Println("Removal!")
					watcher.Remove(current)
					index--
				}

				if delta_depth < 0 {
					log.Println("Parent!")
					if PathPrefixDelta(safename, event.Name) == 1 { // we're the parent dir
						send = true
					}
					watcher.Remove(current)
					index++
				}

				// if event.Name startswith safename, send event, we're already deeper
			} else if HasPathPrefix(event.Name, safename) {
				//log.Println("Event2!")
				send = true
			}

		case err := <-watcher.Errors:
			log.Println("error:", err)
			log.Fatal(err)
			v.Events <- fmt.Sprintf("file: %v", "error")

		case exit := <-obj.Events:
			if exit == "exit" {
				return
			} else {
				log.Fatal("Unknown event: %v\n", exit)
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			//log.Println("Sending event!")
			//v.Events <- fmt.Sprintf("file(%v): %v", obj.Path, event.Op)
			v.Events <- fmt.Sprintf("file(%v): %v", obj.Path, "event!") // FIXME: use struct
		}
	}
}

func (obj FileType) Exit() bool {
	obj.Events <- "exit"
	return true
}

func (obj FileType) HashSHA256fromContent() string {
	if obj.sha256sum != "" { // return if already computed
		return obj.sha256sum
	}

	hash := sha256.New()
	hash.Write([]byte(obj.Content))
	obj.sha256sum = hex.EncodeToString(hash.Sum(nil))
	return obj.sha256sum
}

func (obj FileType) StateOK() bool {
	if _, err := os.Stat(obj.Path); os.IsNotExist(err) {
		// no such file or directory
		if obj.State == "absent" {
			return true // missing file should be missing, phew :)
		} else {
			// state invalid, skip expensive checksums
			return false
		}
	}

	// TODO: add file mode check here...

	if PathIsDir(obj.Path) {
		return obj.StateOKDir()
	} else {
		return obj.StateOKFile()
	}
}

func (obj FileType) StateOKFile() bool {
	if PathIsDir(obj.Path) {
		log.Fatal("This should only be called on a File type.")
	}

	// run a diff, and return true if needs changing

	hash := sha256.New()

	f, err := os.Open(obj.Path)
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
	//fmt.Printf("sha256sum: %v\n", sha256sum)

	if obj.HashSHA256fromContent() == sha256sum {
		return true
	}

	return false
}

func (obj FileType) StateOKDir() bool {
	if !PathIsDir(obj.Path) {
		log.Fatal("This should only be called on a Dir type.")
	}

	// XXX: not implemented
	log.Fatal("Not implemented!")
	return false
}

func (obj FileType) Apply() bool {
	fmt.Printf("Apply->%v[%v]\n", obj.Type, obj.Name)

	if PathIsDir(obj.Path) {
		return obj.ApplyDir()
	} else {
		return obj.ApplyFile()
	}
}

func (obj FileType) ApplyFile() bool {

	if PathIsDir(obj.Path) {
		log.Fatal("This should only be called on a File type.")
	}

	if obj.State == "absent" {
		log.Printf("About to remove: %v\n", obj.Path)
		err := os.Remove(obj.Path)
		if err != nil {
			return false
		}
		return true
	}

	//fmt.Println("writing: " + filename)
	f, err := os.Create(obj.Path)
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

func (obj FileType) ApplyDir() bool {
	if !PathIsDir(obj.Path) {
		log.Fatal("This should only be called on a Dir type.")
	}

	// XXX: not implemented
	log.Fatal("Not implemented!")
	return true
}
