// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package gapi

import (
	"os"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Umask is the default to use when none has been specified.
// TODO: apparently using 0666 is equivalent to respecting the current umask
const Umask = 0666

// CopyFileToFs copies a file from src path on the local fs to a dst path on fs.
func CopyFileToFs(fs engine.WriteableFS, src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return errwrap.Wrapf(err, "can't read from file `%s`", src)
	}
	if err := fs.WriteFile(dst, data, Umask); err != nil {
		return errwrap.Wrapf(err, "can't write to file `%s`", dst)
	}
	return nil
}

// CopyBytesToFs copies a list of bytes to a dst path on fs.
func CopyBytesToFs(fs engine.WriteableFS, b []byte, dst string) error {
	if err := fs.WriteFile(dst, b, Umask); err != nil {
		return errwrap.Wrapf(err, "can't write to file `%s`", dst)
	}
	return nil
}

// CopyStringToFs copies a string to a dst path on fs.
func CopyStringToFs(fs engine.WriteableFS, str, dst string) error {
	if err := fs.WriteFile(dst, []byte(str), Umask); err != nil {
		return errwrap.Wrapf(err, "can't write to file `%s`", dst)
	}
	return nil
}

// CopyDirToFs copies a dir from src path on the local fs to a dst path on fs.
// FIXME: I'm not sure this does the logical thing when the dst path is a dir.
// FIXME: We've got a workaround for this inside of the lang CLI GAPI.
func CopyDirToFs(fs engine.Fs, src, dst string) error {
	return util.CopyDiskToFs(fs, src, dst, false)
}

// CopyDirToFsForceAll copies a dir from src path on the local fs to a dst path
// on fs, but it doesn't error when making a dir that already exists. It also
// uses `MkdirAll` to prevent some issues.
// FIXME: This is being added because of issues with CopyDirToFs. POSIX is hard.
func CopyDirToFsForceAll(fs engine.Fs, src, dst string) error {
	return util.CopyDiskToFsAll(fs, src, dst, true, true)
}

// CopyDirContentsToFs copies a dir contents from src path on the local fs to a
// dst path on fs.
func CopyDirContentsToFs(fs engine.Fs, src, dst string) error {
	return util.CopyDiskContentsToFs(fs, src, dst, false)
}

// MkdirAllOnFs writes a dir to a dst path on fs. It makes the parent dirs if
// they don't exist.
func MkdirAllOnFs(fs engine.WriteableFS, dst string, perm os.FileMode) error {
	return fs.MkdirAll(dst, perm)
}
