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

// This filesystem implementation is based on the afero.BasePathFs code.

package util

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	// RelPathFsScheme returns a unique name for this type of filesystem.
	RelPathFsScheme = "RelPathFs"
)

var (
	_ afero.Lstater  = (*RelPathFs)(nil)
	_ fs.ReadDirFile = (*RelPathFile)(nil)
)

// RelPathFs removes a prefix from all operations to a given path within an Fs.
// The given file name to the operations on this Fs will have a prefix removed
// before calling the base Fs.
//
// When initializing it with "/", a call to `/foo` turns into `foo`.
//
// Note that it does not clean the error messages on return, so you may reveal
// the real path on errors.
type RelPathFs struct {
	source afero.Fs

	prefix string
}

// RelPathFile represents a file node.
type RelPathFile struct {
	afero.File

	prefix string
}

// Name returns the path of the file.
func (obj *RelPathFile) Name() string {
	sourcename := obj.File.Name()
	//return strings.TrimPrefix(sourcename, filepath.Clean(obj.prefix))
	return filepath.Clean(obj.prefix) + sourcename // add prefix back on
}

// ReadDir lists the contents of the directory and returns a list of file info
// objects for each entry.
func (obj *RelPathFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if rdf, ok := obj.File.(fs.ReadDirFile); ok {
		return rdf.ReadDir(n)
	}
	return readDirFile{obj.File}.ReadDir(n)
}

// NewRelPathFs creates a new RelPathFs.
func NewRelPathFs(source afero.Fs, prefix string) afero.Fs {
	return &RelPathFs{source: source, prefix: prefix}
}

// RealPath returns the correct path with the prefix removed.
func (obj *RelPathFs) RealPath(name string) (string, error) {
	if name == "/" {
		return ".", nil // special trim
	}
	if name == "" {
		return filepath.Clean(name), nil // returns a single period
	}
	path := filepath.Clean(name)         // actual path
	prefix := filepath.Clean(obj.prefix) // is often a / and we trim it off

	//if strings.HasPrefix(path, prefix) { // redundant
	path = strings.TrimPrefix(path, prefix)
	//}

	return path, nil
}

// Chtimes changes the access and modification times of the named file, similar
// to the Unix utime() or utimes() functions. The underlying filesystem may
// truncate or round the values to a less precise time unit. If there is an
// error, it will be of type *PathError.
func (obj *RelPathFs) Chtimes(name string, atime, mtime time.Time) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "chtimes", Path: name, Err: err}
	}
	return obj.source.Chtimes(name, atime, mtime)
}

// Chmod changes the mode of a file.
func (obj *RelPathFs) Chmod(name string, mode os.FileMode) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "chmod", Path: name, Err: err}
	}
	return obj.source.Chmod(name, mode)
}

// Chown changes the ownership of a file. It is the equivalent of os.Chown.
func (obj *RelPathFs) Chown(name string, uid, gid int) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "chown", Path: name, Err: err}
	}
	return obj.source.Chown(name, uid, gid)
}

// Name returns the name of this filesystem.
func (obj *RelPathFs) Name() string {
	return RelPathFsScheme
}

// Stat returns some information about the particular path.
func (obj *RelPathFs) Stat(name string) (fi os.FileInfo, err error) {
	if name, err = obj.RealPath(name); err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}
	return obj.source.Stat(name)
}

// Rename moves or renames a file or directory.
func (obj *RelPathFs) Rename(oldname, newname string) (err error) {
	if oldname, err = obj.RealPath(oldname); err != nil {
		return &os.PathError{Op: "rename", Path: oldname, Err: err}
	}
	if newname, err = obj.RealPath(newname); err != nil {
		return &os.PathError{Op: "rename", Path: newname, Err: err}
	}
	return obj.source.Rename(oldname, newname)
}

// RemoveAll removes path and any children it contains. It removes everything it
// can but returns the first error it encounters. If the path does not exist,
// RemoveAll returns nil (no error).
func (obj *RelPathFs) RemoveAll(name string) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "remove_all", Path: name, Err: err}
	}
	return obj.source.RemoveAll(name)
}

// Remove removes a path.
func (obj *RelPathFs) Remove(name string) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "remove", Path: name, Err: err}
	}
	return obj.source.Remove(name)
}

// OpenFile opens a path with a particular flag and permission.
func (obj *RelPathFs) OpenFile(name string, flag int, mode os.FileMode) (f afero.File, err error) {
	if name, err = obj.RealPath(name); err != nil {
		return nil, &os.PathError{Op: "openfile", Path: name, Err: err}
	}
	sourcef, err := obj.source.OpenFile(name, flag, mode)
	if err != nil {
		return nil, err
	}
	return &RelPathFile{File: sourcef, prefix: obj.prefix}, nil
}

// Open opens a path. It will be opened read-only.
func (obj *RelPathFs) Open(name string) (f afero.File, err error) {
	if name, err = obj.RealPath(name); err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	sourcef, err := obj.source.Open(name)
	if err != nil {
		return nil, err
	}
	return &RelPathFile{File: sourcef, prefix: obj.prefix}, nil
}

// Mkdir makes a new directory.
func (obj *RelPathFs) Mkdir(name string, mode os.FileMode) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	return obj.source.Mkdir(name, mode)
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAll creates. If path is already a directory,
// MkdirAll does nothing and returns nil.
func (obj *RelPathFs) MkdirAll(name string, mode os.FileMode) (err error) {
	if name, err = obj.RealPath(name); err != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	return obj.source.MkdirAll(name, mode)
}

// Create creates a new file.
func (obj *RelPathFs) Create(name string) (f afero.File, err error) {
	if name, err = obj.RealPath(name); err != nil {
		return nil, &os.PathError{Op: "create", Path: name, Err: err}
	}
	sourcef, err := obj.source.Create(name)
	if err != nil {
		return nil, err
	}
	return &RelPathFile{File: sourcef, prefix: obj.prefix}, nil
}

// LstatIfPossible is for the lstater interface.
func (obj *RelPathFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	name, err := obj.RealPath(name)
	if err != nil {
		return nil, false, &os.PathError{Op: "lstat", Path: name, Err: err}
	}
	if lstater, ok := obj.source.(afero.Lstater); ok {
		return lstater.LstatIfPossible(name)
	}
	fi, err := obj.source.Stat(name)
	return fi, false, err
}

// SymlinkIfPossible is for the weird Afero symlink API.
func (obj *RelPathFs) SymlinkIfPossible(oldname, newname string) error {
	oldname, err := obj.RealPath(oldname)
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: err}
	}
	newname, err = obj.RealPath(newname)
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: err}
	}
	if linker, ok := obj.source.(afero.Linker); ok {
		return linker.SymlinkIfPossible(oldname, newname)
	}
	return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
}

// ReadlinkIfPossible is for the weird Afero readlink API.
func (obj *RelPathFs) ReadlinkIfPossible(name string) (string, error) {
	name, err := obj.RealPath(name)
	if err != nil {
		return "", &os.PathError{Op: "readlink", Path: name, Err: err}
	}
	if reader, ok := obj.source.(afero.LinkReader); ok {
		return reader.ReadlinkIfPossible(name)
	}
	return "", &os.PathError{Op: "readlink", Path: name, Err: afero.ErrNoReadlink}
}

// readDirFile provides adapter from afero.File to fs.ReadDirFile needed for
// correct Open
type readDirFile struct {
	afero.File
}

var _ fs.ReadDirFile = readDirFile{}

func (r readDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	items, err := r.File.Readdir(n)
	if err != nil {
		return nil, err
	}

	ret := make([]fs.DirEntry, len(items))
	for i := range items {
		//ret[i] = common.FileInfoDirEntry{FileInfo: items[i]}
		ret[i] = FileInfoDirEntry{FileInfo: items[i]}
	}

	return ret, nil
}

var _ fs.DirEntry = FileInfoDirEntry{}

// FileInfoDirEntry provides an adapter from os.FileInfo to fs.DirEntry
type FileInfoDirEntry struct {
	fs.FileInfo
}

// Type returns the FileMode for this DirEntry.
func (obj FileInfoDirEntry) Type() fs.FileMode { return obj.FileInfo.Mode().Type() }

// Info returns the FileInfo for this DirEntry.
func (obj FileInfoDirEntry) Info() (fs.FileInfo, error) { return obj.FileInfo, nil }
