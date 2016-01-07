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
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"path"
	"strings"
	"time"
)

// Similar to the GNU dirname command
func Dirname(p string) string {
	if p == "/" {
		return ""
	}
	d, _ := path.Split(path.Clean(p))
	return d
}

func Basename(p string) string {
	_, b := path.Split(path.Clean(p))
	if p[len(p)-1:] == "/" { // don't loose the tail slash
		b += "/"
	}
	return b
}

// Split a path into an array of tokens excluding any trailing empty tokens
func PathSplit(p string) []string {
	return strings.Split(path.Clean(p), "/")
}

// Does path string contain the given path prefix in it?
func HasPathPrefix(p, prefix string) bool {

	patharray := PathSplit(p)
	prefixarray := PathSplit(prefix)

	if len(prefixarray) > len(patharray) {
		return false
	}

	for i := 0; i < len(prefixarray); i++ {
		if prefixarray[i] != patharray[i] {
			return false
		}
	}

	return true
}

// Delta of path prefix, tells you how many path tokens different the prefix is
func PathPrefixDelta(p, prefix string) int {

	if !HasPathPrefix(p, prefix) {
		return -1
	}
	patharray := PathSplit(p)
	prefixarray := PathSplit(prefix)
	return len(patharray) - len(prefixarray)
}

func PathIsDir(p string) bool {
	return p[len(p)-1:] == "/" // a dir has a trailing slash in this context
}

// encode an object as base 64, serialize and then base64 encode
func ObjToB64(obj interface{}) (string, bool) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(obj)
	if err != nil {
		//log.Println("Gob failed to Encode: ", err)
		return "", false
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), true
}

// TODO: is it possible to somehow generically just return the obj?
// decode an object into the waiting obj which you pass a reference to
func B64ToObj(str string, obj interface{}) bool {
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		//log.Println("Base64 failed to Decode: ", err)
		return false
	}
	b := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b)
	err = d.Decode(obj)
	if err != nil {
		//log.Println("Gob failed to Decode: ", err)
		return false
	}
	return true
}

// special version of time.After that blocks when given a negative integer
// when used in a case statement, the timer restarts on each select call to it
func TimeAfterOrBlock(t int) <-chan time.Time {
	if t < 0 {
		return make(chan time.Time) // blocks forever
	}
	return time.After(time.Duration(t) * time.Second)
}
