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

//go:build linux

package atomicfile

import (
	"io/fs"
	"math/rand/v2"
	"os"
	"path"
	"strconv"

	"golang.org/x/sys/unix"
)

func open(fspath string) (*os.File, error) {
	// Note: this only works on certain filesystems on Linux. See the `O_TMPFILE`
	// documentation on `open(2)` for more details.
	//
	// Relevant notes from Linux's open(2) are:
	//
	// > Create an unnamed temporary regular file.  The path
	// > argument specifies a directory; an unnamed inode will be
	// > created in that directory's filesystem.
	//
	// Where a filesystem doesn't support O_TMPFILE, open() will return
	// EOPNOTSUPP.
	dir := path.Dir(fspath)
	f, err := os.OpenFile(dir, unix.O_RDWR|unix.O_TMPFILE, 0600)
	if err == nil {
		return f, nil
	}

	// Check if OpenFile failed because O_TMPFILE isn't supported by the
	// filesystem.
	if err, ok := err.(*fs.PathError); ok && err.Err == unix.EOPNOTSUPP {
		// O_TMPFILE doesn't work on this filesystem, try another method.
		return openTemp(fspath)
	}

	return nil, err
}

func (obj *AtomicFile) onSameFilesystem() (bool, error) {
	conn, err := obj.SyscallConn()
	if err != nil {
		return false, err
	}

	var srcstat, dststat unix.Stat_t
	conn.Control(func(fd uintptr) {
		err = unix.Fstat(int(fd), &srcstat)
	})
	if err != nil {
		return false, err
	}

	if err = unix.Stat(obj.path, &dststat); err != nil {
		if os.IsNotExist(err) {
			// Full path doesn't exist. Check the parent directory.
			if err = unix.Stat(path.Dir(obj.path), &dststat); err != nil {
				return false, err
			}

			return srcstat.Dev == dststat.Dev, nil
		}
		return false, err
	}

	return srcstat.Dev == dststat.Dev, nil
}

func (obj *AtomicFile) commit() error {
	same, err := obj.onSameFilesystem()
	if err != nil {
		return err
	}
	if same {
		err = obj.commitWithLinkat()
		if err, ok := err.(*fs.PathError); ok && err.Err == unix.EACCES {
			return obj.commitByCopy()
		}
		return err
	} else {
		return obj.commitByCopy()
	}
}

func (obj *AtomicFile) commitWithLinkat() error {
	try := 0
	base := path.Join(
		path.Dir(obj.path),
		path.Base(obj.path)+".new-",
	)

	// Select a filename that doesn't exist yet in order to give our
	// currently unnamed file a real name so that we can rename() it.
	//
	// Note: This for loop was modeled after golang's CreateTemp implementation
	var current string
	for {
		current = base + strconv.FormatUint(rand.Uint64(), 16)

		// Previously, our open() func should have created an unnamed file. In
		// order to rename(2) to deliver our committed file, we must give our
		// unnamed file a name!
		// See linkat(2) and open(2)'s O_TMPFILE documentation on Linux for more
		// information.
		err := unix.Linkat(int(obj.Fd()), "", unix.AT_FDCWD, current, unix.AT_EMPTY_PATH)
		if err != nil {
			if os.IsExist(err) {
				if try++; try < 10000 {
					continue
				}

				// Exhausted tries.
				return &os.PathError{Op: "linkat", Path: current, Err: err}
			} else {
				return err
			}
		}

		// Linkat succeeded!
		break
	}

	defer obj.Close()
	return os.Rename(current, obj.path)
}
