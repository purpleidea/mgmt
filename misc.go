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
	"path"
	"strings"
)

// Similar to the GNU dirname command
func Dirname(p string) string {
	d, _ := path.Split(path.Clean(p))
	return d
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

func PathIsDir(p string) bool {
	return p[len(p)-1:] == "/" // a dir has a trailing slash in this context
}
