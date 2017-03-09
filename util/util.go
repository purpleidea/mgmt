// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

// Package util contains a collection of miscellaneous utility functions.
package util

import (
	"path"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus"
)

// FirstToUpper returns the string with the first character capitalized.
func FirstToUpper(str string) string {
	if str == "" {
		return ""
	}
	return strings.ToUpper(str[0:1]) + str[1:]
}

// StrInList returns true if a string exists inside a list, otherwise false.
func StrInList(needle string, haystack []string) bool {
	for _, x := range haystack {
		if needle == x {
			return true
		}
	}
	return false
}

// Uint64KeyFromStrInMap returns true if needle is found in haystack of keys
// that have uint64 type.
func Uint64KeyFromStrInMap(needle string, haystack map[uint64]string) (uint64, bool) {
	for k, v := range haystack {
		if v == needle {
			return k, true
		}
	}
	return 0, false
}

// StrRemoveDuplicatesInList removes any duplicate values in the list.
// This implementation is possibly sub-optimal (O(n^2)?) but preserves ordering.
func StrRemoveDuplicatesInList(list []string) []string {
	unique := []string{}
	for _, x := range list {
		if !StrInList(x, unique) {
			unique = append(unique, x)
		}
	}
	return unique
}

// StrFilterElementsInList removes any of the elements in filter, if they exist
// in the list.
func StrFilterElementsInList(filter []string, list []string) []string {
	result := []string{}
	for _, x := range list {
		if !StrInList(x, filter) {
			result = append(result, x)
		}
	}
	return result
}

// StrListIntersection removes any of the elements in filter, if they don't
// exist in the list. This is an in order intersection of two lists.
func StrListIntersection(list1 []string, list2 []string) []string {
	result := []string{}
	for _, x := range list1 {
		if StrInList(x, list2) {
			result = append(result, x)
		}
	}
	return result
}

// ReverseStringList reverses a list of strings.
func ReverseStringList(in []string) []string {
	var out []string // empty list
	l := len(in)
	for i := range in {
		out = append(out, in[l-i-1])
	}
	return out
}

// StrMapKeys return the sorted list of string keys in a map with string keys.
// NOTE: i thought it would be nice for this to use: map[string]interface{} but
// it turns out that's not allowed. I know we don't have generics, but come on!
func StrMapKeys(m map[string]string) []string {
	result := []string{}
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result) // deterministic order
	return result
}

// StrMapKeysUint64 return the sorted list of string keys in a map with string
// keys but uint64 values.
func StrMapKeysUint64(m map[string]uint64) []string {
	result := []string{}
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result) // deterministic order
	return result
}

// BoolMapValues returns the sorted list of bool values in a map with string
// values.
func BoolMapValues(m map[string]bool) []bool {
	result := []bool{}
	for _, v := range m {
		result = append(result, v)
	}
	//sort.Bools(result) // TODO: deterministic order
	return result
}

// StrMapValues returns the sorted list of string values in a map with string
// values.
func StrMapValues(m map[string]string) []string {
	result := []string{}
	for _, v := range m {
		result = append(result, v)
	}
	sort.Strings(result) // deterministic order
	return result
}

// StrMapValuesUint64 return the sorted list of string values in a map with
// string values.
func StrMapValuesUint64(m map[uint64]string) []string {
	result := []string{}
	for _, v := range m {
		result = append(result, v)
	}
	sort.Strings(result) // deterministic order
	return result
}

// BoolMapTrue returns true if everyone in the list is true.
func BoolMapTrue(l []bool) bool {
	for _, b := range l {
		if !b {
			return false
		}
	}
	return true
}

// Dirname is similar to the GNU dirname command.
func Dirname(p string) string {
	if p == "/" {
		return ""
	}
	d, _ := path.Split(path.Clean(p))
	return d
}

// Basename is the base of a path string.
func Basename(p string) string {
	_, b := path.Split(path.Clean(p))
	if p == "" {
		return ""
	}
	if p[len(p)-1:] == "/" { // don't loose the tail slash
		b += "/"
	}
	return b
}

// PathSplit splits a path into an array of tokens excluding any trailing empty
// tokens.
func PathSplit(p string) []string {
	if p == "/" { // TODO: can't this all be expressed nicely in one line?
		return []string{""}
	}
	return strings.Split(path.Clean(p), "/")
}

// HasPathPrefix tells us if a path string contain the given path prefix in it.
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

// StrInPathPrefixList returns true if the needle is a PathPrefix in the
// haystack.
func StrInPathPrefixList(needle string, haystack []string) bool {
	for _, x := range haystack {
		if HasPathPrefix(x, needle) {
			return true
		}
	}
	return false
}

// RemoveCommonFilePrefixes removes redundant file path prefixes that are under
// the tree of other files.
func RemoveCommonFilePrefixes(paths []string) []string {
	var result = make([]string, len(paths))
	for i := 0; i < len(paths); i++ { // copy, b/c append can modify the args!!
		result[i] = paths[i]
	}
	// is there a string path which is common everywhere?
	// if so, remove it, and iterate until nothing common is left
	// return what's left over, that's the most common superset
loop:
	for {
		if len(result) <= 1 {
			return result
		}
		for i := 0; i < len(result); i++ {
			var copied = make([]string, len(result))
			for j := 0; j < len(result); j++ { // copy, b/c append can modify the args!!
				copied[j] = result[j]
			}
			noi := append(copied[:i], copied[i+1:]...) // rm i
			if StrInPathPrefixList(result[i], noi) {
				// delete the element common to everyone
				result = noi
				continue loop
			}
		}
		break
	}
	return result
}

// PathPrefixDelta returns the delta of the path prefix, which tells you how
// many path tokens different the prefix is.
func PathPrefixDelta(p, prefix string) int {

	if !HasPathPrefix(p, prefix) {
		return -1
	}
	patharray := PathSplit(p)
	prefixarray := PathSplit(prefix)
	return len(patharray) - len(prefixarray)
}

// PathSplitFullReversed returns the full list of "dependency" paths for a given
// path in reverse order.
func PathSplitFullReversed(p string) []string {
	var result []string
	split := PathSplit(p)
	count := len(split)
	var x string
	for i := 0; i < count; i++ {
		x = "/" + path.Join(split[0:i+1]...)
		if i != 0 && !(i+1 == count && !strings.HasSuffix(p, "/")) {
			x += "/" // add trailing slash
		}
		result = append(result, x)
	}
	return ReverseStringList(result)
}

// DirifyFileList adds trailing slashes to any likely dirs in a package manager
// fileList if removeDirs is true, otherwise, don't keep the dirs in our output.
func DirifyFileList(fileList []string, removeDirs bool) []string {
	dirs := []string{}
	for _, file := range fileList {
		dir, _ := path.Split(file) // dir
		dir = path.Clean(dir)      // clean so cmp is easier
		if !StrInList(dir, dirs) {
			dirs = append(dirs, dir)
		}
	}

	result := []string{}
	for _, file := range fileList {
		cleanFile := path.Clean(file)
		if !StrInList(cleanFile, dirs) { // we're not a directory!
			result = append(result, file) // pass through
		} else if !removeDirs {
			result = append(result, cleanFile+"/")
		}
	}

	return result
}

// FlattenListWithSplit flattens a list of input by splitting each element by
// any and all of the strings listed in the split array
func FlattenListWithSplit(input []string, split []string) []string {
	if len(split) == 0 { // nothing to split by
		return input
	}
	out := []string{}
	for _, x := range input {
		s := []string{}
		if len(split) == 1 {
			s = strings.Split(x, split[0]) // split by only string
		} else {
			s = []string{x} // initial
			for i := range split {
				s = FlattenListWithSplit(s, []string{split[i]}) // recurse
			}
		}
		out = append(out, s...)
	}
	return out
}

// TimeAfterOrBlock is aspecial version of time.After that blocks when given a
// negative integer. When used in a case statement, the timer restarts on each
// select call to it.
func TimeAfterOrBlock(t int) <-chan time.Time {
	if t < 0 {
		return make(chan time.Time) // blocks forever
	}
	return time.After(time.Duration(t) * time.Second)
}

// SystemBusPrivateUsable makes using the private bus usable
// TODO: should be upstream: https://github.com/godbus/dbus/issues/15
func SystemBusPrivateUsable() (conn *dbus.Conn, err error) {
	conn, err = dbus.SystemBusPrivate()
	if err != nil {
		return nil, err
	}
	if err = conn.Auth(nil); err != nil {
		conn.Close()
		conn = nil
		return
	}
	if err = conn.Hello(); err != nil {
		conn.Close()
		conn = nil
	}
	return conn, nil // success
}
