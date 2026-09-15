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

// Package atomicfile contains a mechanism to write ore replace a given filename
// with a single, atomic operation.
package atomicfile

import (
	"io"
	"io/fs"
	"os"
	"path"

	"golang.org/x/sys/unix"
)

// AtomicFile is an os.File-like struct which allows you to write data and
// modify metadata before committing it to a specific filename.
//
// The intended use is to ensure complete files are only ever available at the
// given path.
//
// If, for any reason, a Commit() never occurs, this module attempts to ensure
// that no temporary files are left around.
//
// Thus:
// * If the program crashes before a Commit(), the temporary file shall be
//   lost (on purpose).
// * If the program calls Close() without a Commit(), the temporary file and
//   its writes shall be lost (on purpose).
// * If the computer fails before a Commit(), the temporary file shall be
//   lost (on purpose).
//
// For any of the above scenarios, no disk space should be occupied by these
// lost files.
//
// This module focuses on delivering "whole files" and is not concerned with
// durability or serializability of operations.
type AtomicFile struct {
	os.File
	path string
}

// New opens an unnamed temporary file.
//
// If you wish to commit writes to the given fspath, you must call Commit().
func New(fspath string) (*AtomicFile, error) {
	f, err := open(fspath)

	if err != nil {
		return nil, err
	}

	return &AtomicFile{File: *f, path: fspath}, nil
}

// Commit acts upon the file to atomically replace it on disk.
// This closes the file and further changes should not be attempted.
func (obj *AtomicFile) Commit() error {
	err := obj.commit()

	// Close after attempting to commit, even if the commit fails.
	// Nothing can be done so we should still close the file
	_ = obj.Close()

	return err
}

// A default "create a nearby tempfile" strategy
func openTemp(fspath string) (*os.File, error) {
	f, err := os.CreateTemp(path.Dir(fspath), path.Base(fspath)+".new*")
	if err, ok := err.(*fs.PathError); ok {
		if err.Err == unix.ENOENT || err.Err == unix.EACCES {
			var err2 error
			f, err2 = os.CreateTemp("", path.Base(fspath)+".new*")
			if err2 != nil {
				// XXX: Return a composite error of err+err2?
				return nil, err2
			}
		} else {
			return nil, err
		}
	}

	// Delete the filename while leaving the file open.
	// The file will get a real name on disk during commit()
	err = os.Remove(f.Name())
	if err != nil {
		return nil, err
	}

	return f, nil
}

// Open the existing path for writing and copy our data into it.
// This is a last resort to write the data in cases where atomic operations
// like rename(2) are not available.
func (obj *AtomicFile) commitByCopy() error {

	// Truncate the file and open it for writing.
	// This is not "atomic" but this function is a fallback for cases
	// where atomic methods do not work.
	file, err := os.OpenFile(obj.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// Move the reader to the beginning of our temporary file.
	obj.Seek(0, io.SeekStart)

	if _, err = io.Copy(file, obj); err != nil {
		return err
	}

	if err = file.Sync(); err != nil {
		if err, ok := err.(*fs.PathError); ok && err.Err == unix.EINVAL {
			// Filesystem likely doesn't support fsync. Safe to ignore.
		} else {
			return err
		}
	}

	return nil
}
