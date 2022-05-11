// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// This particular file is dual-licensed. It's available under the GNU LGPL-3.0+
// and the GNU GPL-3.0+ so that it can be used by other projects easily. If it's
// popular we can consider spinning it out into its own separate git repository.
// SPDX-License-Identifier: GPL-3.0+ OR LGPL-3.0+

// Package safepath implements some types and methods for dealing with POSIX
// file paths safely. When golang programmers use strings to express paths, it
// can sometimes be confusing whether a particular string represents either a
// file or a directory, and whether it is absolute or relative. This package
// provides a type for each of these, and safe methods to manipulate them all.
// The convention is that directories must end with a slash, and absolute paths
// must start with one. There are no generic "path" types for now, you must be
// more specific when using this library. If you can't discern exactly what you
// are, then use a string. It is your responsibility to build the type correctly
// and to call Validate on them to ensure you've done it correctly. If you
// don't, then you could cause a panic.
//
// As a reminder, this library knows nothing about the special path characters
// like period (as in ./) and it only knows about two periods (..) in so far as
// it uses the stdlib Clean method when pulling in new paths.
//
// The ParseInto* family of functions will sometimes add or remove a trailing
// slash to ensure you get a directory or file. It is recommended that you make
// sure to verify your desired type is what you expect before calling this.
package safepath

// NOTE: I started the design of this library by thinking about what types I
// wanted. Absolute and relative files and directories. I don't think there's a
// need to handle symlinks at the moment. This means I'll need four types. Next
// I need to work out what possible operations are valid. My sketch of that is:
//
// join(absdir, relfile) => absfile
// join(absdir, reldir) => absdir
// join(reldir, relfile) => relfile
// join(reldir, reldir) => reldir
// join(absdir, absdir) => error
// join(absdir, absfile) => error
// join(absfile, absfile) => error
// join(relfile, relfile) => error
//
// This isn't Haskell, so I'll either need four separate functions or one join
// function that takes interfaces. The latter would be a big unsafe mess. As it
// turns out, there is exactly one join operation that produces each of
// the four types. So instead of naming each one like `JoinAbsDirRelFile` and so
// on, I decided to name them based on their return type.
//
// Be consistent! Order: file, dir, abs, rel, absfile, absdir, relfile, reldir.
//
// This could probably get spun off into it's own standalone library.

import (
	"fmt"
	stdlibPath "path"
	"strings"
)

// Path represents any absolute or relative file or directory.
type Path interface {
	fmt.Stringer
	Path() string
	IsDir() bool
	IsAbs() bool

	isPath() // private to prevent others from implementing this (ok?)
}

// File represents either an absolute or relative file. Directories are not
// included.
type File interface {
	fmt.Stringer
	Path() string
	//IsDir() bool // TODO: add this to allow a File to become a Path?
	IsAbs() bool

	isFile() // only the files have this
}

// Dir represents either an absolute or relative directory. Files are not
// included.
type Dir interface {
	fmt.Stringer
	Path() string
	//IsDir() bool // TODO: add this to allow a Dir to become a Path?
	IsAbs() bool

	isDir() // only the dirs have this
}

// Abs represents an absolute file or directory. Relative paths are not
// included.
type Abs interface {
	fmt.Stringer
	Path() string
	IsDir() bool
	//IsAbs() bool // TODO: add this to allow an Abs to become a Path?

	isAbs() // only the abs have this
}

// Rel represents a relative file or directory. Absolute paths are not included.
type Rel interface {
	fmt.Stringer
	Path() string
	IsDir() bool
	//IsAbs() bool // TODO: add this to allow a Rel to become a Path?

	isRel() // only the rel have this
}

// AbsFile represents an absolute file path.
type AbsFile struct {
	path string
}

func (obj AbsFile) isAbs()  {}
func (obj AbsFile) isFile() {}
func (obj AbsFile) isPath() {}

// String returns the canonical "friendly" representation of this path. If it is
// a directory, then it will end with a slash.
func (obj AbsFile) String() string { return obj.path }

// Path returns the cleaned version of this path. It is what you expect after
// running the golang path cleaner on the internal representation.
func (obj AbsFile) Path() string { return stdlibPath.Clean(obj.path) }

// IsDir returns false for this struct.
func (obj AbsFile) IsDir() bool { return false }

// IsAbs returns true for this struct.
func (obj AbsFile) IsAbs() bool { return true }

// Validate returns an error if the path was not specified correctly.
func (obj AbsFile) Validate() error {
	if !strings.HasPrefix(obj.path, "/") {
		return fmt.Errorf("file is not absolute")
	}

	if strings.HasSuffix(obj.path, "/") {
		return fmt.Errorf("path is not a file")
	}

	return nil
}

// PanicValidate panics if the path was not specified correctly.
func (obj AbsFile) PanicValidate() {
	if err := obj.Validate(); err != nil {
		panic(err.Error())
	}
}

// Cmp compares two AbsFile's and returns nil if they have the same path.
func (obj AbsFile) Cmp(absFile AbsFile) error {
	if obj.path != absFile.path {
		return fmt.Errorf("files differ")
	}
	return nil
}

// Base returns the last component of the AbsFile, in this case, the filename.
func (obj AbsFile) Base() RelFile {
	obj.PanicValidate()
	ix := strings.LastIndex(obj.path, "/")
	return RelFile{
		path: obj.path[ix+1:],
	}
}

// Dir returns the head component of the AbsFile, in this case, the directory.
func (obj AbsFile) Dir() AbsDir {
	obj.PanicValidate()
	ix := strings.LastIndex(obj.path, "/")
	if ix == 0 {
		return AbsDir{
			path: "/",
		}
	}
	return AbsDir{
		path: obj.path[0:ix],
	}
}

// HasDir returns true if the input relative dir is present in the path.
func (obj AbsFile) HasDir(relDir RelDir) bool {
	obj.PanicValidate()
	relDir.PanicValidate()
	//if obj.path == "/" {
	//	return false
	//}
	// TODO: test with ""

	i := strings.Index(obj.path, relDir.path)
	if i == -1 {
		return false // not found
	}
	if i == 0 {
		// not possible unless relDir is /
		//return false // found the root dir
		panic("relDir was root which isn't relative")
	}
	// We want to make sure we land on a split char, or we didn't match it.
	// We don't need to check the last char, because we know it's a /
	return obj.path[i-1] == '/' // check if the char before is a /
}

// HasExt checks if the file ends with the given extension. It checks for an
// exact string match. You might prefer using HasExtInsensitive instead. As a
// special case, if you pass in an empty string as the extension to match, this
// will return false.
// TODO: add tests
func (obj AbsFile) HasExt(ext string) bool {
	obj.PanicValidate()

	if ext == "" { // special case, not consistent with strings.HasSuffix
		return false
	}

	if !strings.HasSuffix(obj.path, ext) {
		return false
	}

	return true
}

// HasExtInsensitive checks if the file ends with the given extension. It checks
// with a fancy case-insensitive match. As a special case, if you pass in an
// empty string as the extension to match, this will return false.
func (obj AbsFile) HasExtInsensitive(ext string) bool {
	obj.PanicValidate()

	return hasExtInsensitive(obj.path, ext)
}

// ParseIntoAbsFile takes an input path and ensures it's an AbsFile. It doesn't
// do anything particularly magical. It then runs Validate to ensure the path
// was valid overall. It also runs the stdlib path Clean function on it. Please
// note, that passing in the root slash / will cause this to fail.
func ParseIntoAbsFile(path string) (AbsFile, error) {
	if path == "" {
		return AbsFile{}, fmt.Errorf("path is empty")
	}

	path = stdlibPath.Clean(path)

	absFile := AbsFile{path: path}
	return absFile, absFile.Validate()
}

// UnsafeParseIntoAbsFile performs exactly as ParseIntoAbsFile does, but it
// panics if the latter would have returned an error.
func UnsafeParseIntoAbsFile(path string) AbsFile {
	absFile, err := ParseIntoAbsFile(path)
	if err != nil {
		panic(err.Error())
	}
	return absFile
}

// AbsDir represents an absolute dir path.
type AbsDir struct {
	path string
}

func (obj AbsDir) isAbs()  {}
func (obj AbsDir) isDir()  {}
func (obj AbsDir) isPath() {}

// String returns the canonical "friendly" representation of this path. If it is
// a directory, then it will end with a slash.
func (obj AbsDir) String() string { return obj.path }

// Path returns the cleaned version of this path. It is what you expect after
// running the golang path cleaner on the internal representation.
func (obj AbsDir) Path() string { return stdlibPath.Clean(obj.path) }

// IsDir returns true for this struct.
func (obj AbsDir) IsDir() bool { return true }

// IsAbs returns true for this struct.
func (obj AbsDir) IsAbs() bool { return true }

// Validate returns an error if the path was not specified correctly.
func (obj AbsDir) Validate() error {
	if !strings.HasPrefix(obj.path, "/") {
		return fmt.Errorf("dir is not absolute")
	}

	if !strings.HasSuffix(obj.path, "/") {
		return fmt.Errorf("path is not a dir")
	}

	return nil
}

// PanicValidate panics if the path was not specified correctly.
func (obj AbsDir) PanicValidate() {
	if err := obj.Validate(); err != nil {
		panic(err.Error())
	}
}

// Cmp compares two AbsDir's and returns nil if they have the same path.
func (obj AbsDir) Cmp(absDir AbsDir) error {
	if obj.path != absDir.path {
		return fmt.Errorf("dirs differ")
	}
	return nil
}

// HasDir returns true if the input relative dir is present in the path.
func (obj AbsDir) HasDir(relDir RelDir) bool {
	obj.PanicValidate()
	relDir.PanicValidate()
	if obj.path == "/" {
		return false
	}
	// TODO: test with ""

	i := strings.Index(obj.path, relDir.path)
	if i == -1 {
		return false // not found
	}
	if i == 0 {
		// not possible unless relDir is /
		//return false // found the root dir
		panic("relDir was root which isn't relative")
	}
	// We want to make sure we land on a split char, or we didn't match it.
	// We don't need to check the last char, because we know it's a /
	return obj.path[i-1] == '/' // check if the char before is a /
}

// HasDirOne returns true if the input relative dir is present in the path. It
// only works with a single dir as relDir, so it won't work if relDir is `a/b/`.
func (obj AbsDir) HasDirOne(relDir RelDir) bool {
	obj.PanicValidate()
	relDir.PanicValidate()
	if obj.path == "/" {
		return false
	}
	// TODO: test with ""
	sa := strings.Split(obj.path, "/")
	for i := 1; i < len(sa)-1; i++ {
		p := sa[i] + "/"
		if p == relDir.path {
			return true
		}
	}
	return false
}

// ParseIntoAbsDir takes an input path and ensures it's an AbsDir, by adding a
// trailing slash if it's missing. It then runs Validate to ensure the path was
// valid overall. It also runs the stdlib path Clean function on it.
func ParseIntoAbsDir(path string) (AbsDir, error) {
	if path == "" {
		return AbsDir{}, fmt.Errorf("path is empty")
	}

	path = stdlibPath.Clean(path)

	// NOTE: after clean we won't have a trailing slash I think ;)
	if !strings.HasSuffix(path, "/") { // add trailing slash if missing
		path += "/"
	}

	absDir := AbsDir{path: path}
	return absDir, absDir.Validate()
}

// UnsafeParseIntoAbsDir performs exactly as ParseIntoAbsDir does, but it panics
// if the latter would have returned an error.
func UnsafeParseIntoAbsDir(path string) AbsDir {
	absDir, err := ParseIntoAbsDir(path)
	if err != nil {
		panic(err.Error())
	}
	return absDir
}

// RelFile represents a relative file path.
type RelFile struct {
	path string
}

func (obj RelFile) isRel()  {}
func (obj RelFile) isFile() {}
func (obj RelFile) isPath() {}

// String returns the canonical "friendly" representation of this path. If it is
// a directory, then it will end with a slash.
func (obj RelFile) String() string { return obj.path }

// Path returns the cleaned version of this path. It is what you expect after
// running the golang path cleaner on the internal representation.
func (obj RelFile) Path() string { return stdlibPath.Clean(obj.path) }

// IsDir returns false for this struct.
func (obj RelFile) IsDir() bool { return false }

// IsAbs returns false for this struct.
func (obj RelFile) IsAbs() bool { return false }

// Validate returns an error if the path was not specified correctly.
func (obj RelFile) Validate() error {
	if strings.HasPrefix(obj.path, "/") {
		return fmt.Errorf("file is not relative")
	}

	if strings.HasSuffix(obj.path, "/") {
		return fmt.Errorf("path is not a file")
	}

	if obj.path == "" {
		return fmt.Errorf("path is empty")
	}

	return nil
}

// PanicValidate panics if the path was not specified correctly.
func (obj RelFile) PanicValidate() {
	if err := obj.Validate(); err != nil {
		panic(err.Error())
	}
}

// Cmp compares two RelFile's and returns nil if they have the same path.
func (obj RelFile) Cmp(relfile RelFile) error {
	if obj.path != relfile.path {
		return fmt.Errorf("files differ")
	}
	return nil
}

// HasDir returns true if the input relative dir is present in the path.
func (obj RelFile) HasDir(relDir RelDir) bool {
	obj.PanicValidate()
	relDir.PanicValidate()
	//if obj.path == "/" {
	//	return false
	//}
	// TODO: test with ""

	i := strings.Index(obj.path, relDir.path)
	if i == -1 {
		return false // not found
	}
	if i == 0 {
		return true // found at the beginning
	}
	// We want to make sure we land on a split char, or we didn't match it.
	// We don't need to check the last char, because we know it's a /
	return obj.path[i-1] == '/' // check if the char before is a /
}

// HasExt checks if the file ends with the given extension. It checks for an
// exact string match. You might prefer using HasExtInsensitive instead. As a
// special case, if you pass in an empty string as the extension to match, this
// will return false.
// TODO: add tests
func (obj RelFile) HasExt(ext string) bool {
	obj.PanicValidate()

	if ext == "" { // special case, not consistent with strings.HasSuffix
		return false
	}

	if !strings.HasSuffix(obj.path, ext) {
		return false
	}

	return true
}

// HasExtInsensitive checks if the file ends with the given extension. It checks
// with a fancy case-insensitive match. As a special case, if you pass in an
// empty string as the extension to match, this will return false.
func (obj RelFile) HasExtInsensitive(ext string) bool {
	obj.PanicValidate()

	return hasExtInsensitive(obj.path, ext)
}

// ParseIntoRelFile takes an input path and ensures it's an RelFile. It doesn't
// do anything particularly magical. It then runs Validate to ensure the path
// was valid overall. It also runs the stdlib path Clean function on it.
func ParseIntoRelFile(path string) (RelFile, error) {
	if path == "" {
		return RelFile{}, fmt.Errorf("path is empty")
	}

	path = stdlibPath.Clean(path)

	relFile := RelFile{path: path}
	return relFile, relFile.Validate()
}

// UnsafeParseIntoRelFile performs exactly as ParseIntoRelFile does, but it
// panics if the latter would have returned an error.
func UnsafeParseIntoRelFile(path string) RelFile {
	relFile, err := ParseIntoRelFile(path)
	if err != nil {
		panic(err.Error())
	}
	return relFile
}

// RelDir represents a relative dir path.
type RelDir struct {
	path string
}

func (obj RelDir) isRel()  {}
func (obj RelDir) isDir()  {}
func (obj RelDir) isPath() {}

// String returns the canonical "friendly" representation of this path. If it is
// a directory, then it will end with a slash.
func (obj RelDir) String() string { return obj.path }

// Path returns the cleaned version of this path. It is what you expect after
// running the golang path cleaner on the internal representation.
func (obj RelDir) Path() string { return stdlibPath.Clean(obj.path) }

// IsDir returns true for this struct.
func (obj RelDir) IsDir() bool { return true }

// IsAbs returns false for this struct.
func (obj RelDir) IsAbs() bool { return false }

// Validate returns an error if the path was not specified correctly.
func (obj RelDir) Validate() error {
	if strings.HasPrefix(obj.path, "/") {
		return fmt.Errorf("dir is not relative")
	}

	if !strings.HasSuffix(obj.path, "/") {
		return fmt.Errorf("path is not a dir")
	}

	return nil
}

// PanicValidate panics if the path was not specified correctly.
func (obj RelDir) PanicValidate() {
	if err := obj.Validate(); err != nil {
		panic(err.Error())
	}
}

// Cmp compares two RelDir's and returns nil if they have the same path.
func (obj RelDir) Cmp(relDir RelDir) error {
	if obj.path != relDir.path {
		return fmt.Errorf("dirs differ")
	}
	return nil
}

// HasDir returns true if the input relative dir is present in the path.
func (obj RelDir) HasDir(relDir RelDir) bool {
	obj.PanicValidate()
	relDir.PanicValidate()
	//if obj.path == "/" {
	//	return false
	//}
	// TODO: test with ""

	i := strings.Index(obj.path, relDir.path)
	if i == -1 {
		return false // not found
	}
	if i == 0 {
		return true // found at the beginning
	}
	// We want to make sure we land on a split char, or we didn't match it.
	// We don't need to check the last char, because we know it's a /
	return obj.path[i-1] == '/' // check if the char before is a /
}

// HasDirOne returns true if the input relative dir is present in the path. It
// only works with a single dir as relDir, so it won't work if relDir is `a/b/`.
func (obj RelDir) HasDirOne(relDir RelDir) bool {
	obj.PanicValidate()
	relDir.PanicValidate()
	// TODO: test with "" and "/"
	sa := strings.Split(obj.path, "/")
	for i := 1; i < len(sa)-1; i++ {
		p := sa[i] + "/"
		if p == relDir.path {
			return true
		}
	}
	return false
}

// ParseIntoRelDir takes an input path and ensures it's an RelDir, by adding a
// trailing slash if it's missing. It then runs Validate to ensure the path was
// valid overall. It also runs the stdlib path Clean function on it.
func ParseIntoRelDir(path string) (RelDir, error) {
	if path == "" {
		return RelDir{}, fmt.Errorf("path is empty")
	}

	path = stdlibPath.Clean(path)

	// NOTE: after clean we won't have a trailing slash I think ;)
	if !strings.HasSuffix(path, "/") { // add trailing slash if missing
		path += "/"
	}

	relDir := RelDir{path: path}
	return relDir, relDir.Validate()
}

// UnsafeParseIntoRelDir performs exactly as ParseIntoRelDir does, but it panics
// if the latter would have returned an error.
func UnsafeParseIntoRelDir(path string) RelDir {
	relDir, err := ParseIntoRelDir(path)
	if err != nil {
		panic(err.Error())
	}
	return relDir
}

// ParseIntoPath takes an input path and a boolean that specifies if it is a dir
// and returns a type that fulfills the Path interface. The isDir boolean
// usually comes from the io/fs.FileMode IsDir() method. The returned underlying
// type will be one of AbsFile, AbsDir, RelFile, RelDir. It then runs Validate
// to ensure the path was valid overall.
func ParseIntoPath(path string, isDir bool) (Path, error) {
	//var safePath Path
	if isDir {
		dir, err := ParseIntoDir(path)
		if err != nil {
			return nil, err
		}
		if absDir, ok := dir.(AbsDir); ok {
			return absDir, absDir.Validate()
		}
		if relDir, ok := dir.(RelDir); ok {
			return relDir, relDir.Validate()
		}
		return nil, fmt.Errorf("unknown dir") // bug
	}
	file, err := ParseIntoFile(path)
	if err != nil {
		return nil, err
	}
	if absFile, ok := file.(AbsFile); ok {
		return absFile, absFile.Validate()
	}
	if relFile, ok := file.(RelFile); ok {
		return relFile, relFile.Validate()
	}
	return nil, fmt.Errorf("unknown file") // bug
}

// UnsafeParseIntoPath performs exactly as ParseIntoPath does, but it panics if
// the latter would have returned an error.
func UnsafeParseIntoPath(path string, isDir bool) Path {
	p, err := ParseIntoPath(path, isDir)
	if err != nil {
		panic(err.Error())
	}
	return p
}

// SmartParseIntoPath performs exactly as ParseIntoPath does, except it
// determines if something is a dir based on whetherthe string path has a
// trailing slash or not.
func SmartParseIntoPath(path string) (Path, error) {
	return ParseIntoPath(path, IsDir(path))
}

// UnsafeSmartParseIntoPath performs exactly as SmartParseIntoPath does, but it
// panics if the latter would have returned an error.
func UnsafeSmartParseIntoPath(path string) Path {
	p, err := SmartParseIntoPath(path)
	if err != nil {
		panic(err.Error())
	}
	return p
}

// ParseIntoFile takes an input path and returns a type that fulfills the File
// interface. The returned underlying type will be one of AbsFile or RelFile.
func ParseIntoFile(path string) (File, error) {
	if strings.HasPrefix(path, "/") { // also matches "/", but that would error
		return ParseIntoAbsFile(path)
	}
	return ParseIntoRelFile(path)
}

// UnsafeParseIntoFile performs exactly as ParseIntoFile does, but it panics if
// the latter would have returned an error.
func UnsafeParseIntoFile(path string) File {
	p, err := ParseIntoFile(path)
	if err != nil {
		panic(err.Error())
	}
	return p
}

// ParseIntoDir takes an input path and returns a type that fulfills the Dir
// interface. The returned underlying type will be one of AbsDir or RelDir.
func ParseIntoDir(path string) (Dir, error) {
	if strings.HasPrefix(path, "/") { // also matches "/"
		return ParseIntoAbsDir(path)
	}
	return ParseIntoRelDir(path)
}

// UnsafeParseIntoDir performs exactly as ParseIntoDir does, but it panics if
// the latter would have returned an error.
func UnsafeParseIntoDir(path string) Dir {
	p, err := ParseIntoDir(path)
	if err != nil {
		panic(err.Error())
	}
	return p
}

// JoinToAbsFile joins an absolute dir with a relative file to produce an
// absolute file.
func JoinToAbsFile(absDir AbsDir, relFile RelFile) AbsFile {
	absDir.PanicValidate()
	relFile.PanicValidate()
	return AbsFile{
		path: absDir.path + relFile.path,
	}
}

// JoinToAbsDir joins an absolute dir with a relative dir to produce an absolute
// dir.
func JoinToAbsDir(absDir AbsDir, relDir RelDir) AbsDir {
	absDir.PanicValidate()
	relDir.PanicValidate()
	return AbsDir{
		path: absDir.path + relDir.path,
	}
}

// JoinToRelFile joins a relative dir with a relative file to produce a relative
// file.
func JoinToRelFile(relDir RelDir, relFile RelFile) RelFile {
	relDir.PanicValidate()
	relFile.PanicValidate()
	return RelFile{
		path: relDir.path + relFile.path,
	}
}

// JoinToRelDir joins any number of relative dir's to produce a relative dir.
func JoinToRelDir(relDir ...RelDir) RelDir {
	p := ""
	for _, x := range relDir {
		x.PanicValidate()
		p += x.path
	}
	return RelDir{
		path: p,
	}
}

// HasPrefix determines if the given path has the specified dir prefix. The
// prefix and path can be absolute or relative. Keep in mind, that if the path
// is absolute, then only an absolute dir can successfully match. Similarly, if
// the path is relative, then only a relative dir can successfully match.
func HasPrefix(path Path, prefix Dir) bool {
	return strings.HasPrefix(path.String(), prefix.String())
}

// StripPrefix removes a dir prefix from a path if it is possible to do so. The
// prefix and path can be both absolute or both relative. The returned result is
// either a relative dir or a relative file. Keep in mind, that if the path is
// absolute, then only an absolute dir can successfully match. Similarly, if the
// path is relative, then only a relative dir can successfully match. The input
// path will be returned unchanged if it is not possible to match (although it
// will return an error in parallel) and if there is a match, then either a
// relative dir or a relative file will be returned with the path interface.
// This is logical, because after removing some prefix, only relative paths can
// possibly remain. This relative returned path will be a file if the input path
// was a file, and a dir if the input path was a dir.
// XXX: add tests!
func StripPrefix(path Path, prefix Dir) (Path, error) {
	if !HasPrefix(path, prefix) {
		return path, fmt.Errorf("no prefix")
	}
	p := strings.TrimPrefix(path.String(), prefix.String())
	// XXX: what happens if we strip the entire dir path from itself? Empty path?
	//if p == "" {
	//	return ???, ???
	//}

	if path.IsDir() {
		return ParseIntoRelDir(p)
	}
	return ParseIntoRelFile(p)
}

// IsDir is a helper that returns true if a string path is considered as such by
// the presence of a trailing slash.
func IsDir(path string) bool {
	return strings.HasSuffix(path, "/")
}

// IsAbs is a helper that returns true if a string path is considered as such by
// the presence of a leading slash.
func IsAbs(path string) bool {
	return strings.HasPrefix(path, "/")
}

// hasExtInsensitive is the helper function for checking if the file ends with
// the given extension. It checks with a fancy case-insensitive match. As a
// special case, if you pass in an empty string as the extension to match, this
// will return false.
// TODO: add tests!
func hasExtInsensitive(path, ext string) bool {
	if ext == "" { // special case, not consistent with strings.HasSuffix
		return false
	}

	if len(ext) > len(path) { // file not long enough to have extension
		return false
	}

	s := path[len(path)-len(ext):] // extract ext length of chars

	if !strings.EqualFold(s, ext) { // fancy case-insensitive compare
		return false
	}

	return true
}
