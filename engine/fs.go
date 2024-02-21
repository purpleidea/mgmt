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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package engine

import (
	"os"

	"github.com/spf13/afero"
)

// Fs is an interface that represents the file system API that we support.
// TODO: rename this to FS for consistency with the io/fs.FS naming scheme
type Fs interface {
	//fmt.Stringer // TODO: add this method?

	// URI returns a unique string handle to access this filesystem.
	URI() string // returns the URI for this file system

	afero.Fs // TODO: why doesn't this interface exist in the os pkg?

	// FS is the read-only filesystem interface from the io/fs.FS package.
	//fs.FS // io/fs.FS

	// ReadDir reads the named directory and returns a list of directory
	// entries sorted by filename.
	//
	// This mimics the signature from io/fs.ReadDirFS and has the same docs.
	//
	// XXX: Not currently implemented because of legacy Afero.Fs above
	//ReadDir(name string) ([]fs.DirEntry, error) // io/fs.ReadDirFS

	// ReadFile reads the named file and returns its contents. A successful
	// call returns a nil error, not io.EOF. (Because ReadFile reads the
	// whole file, the expected EOF from the final Read is not treated as an
	// error to be reported.)
	//
	// The caller is permitted to modify the returned byte slice. This
	// method should return a copy of the underlying data.
	//
	// This mimics the signature from io/fs.ReadFileFS and has the same
	// docs.
	ReadFile(name string) ([]byte, error) // io/fs.ReadFileFS

	// Stat returns a FileInfo describing the file. If there is an error, it
	// should be of type *fs.PathError.
	//
	// This mimics the signature from io/fs.StatFS and has the same docs.
	//
	// XXX: Not currently implemented because of legacy Afero.Fs above
	//Stat(name string) (FileInfo, error) // io/fs.StatFS

	// afero.Fs versions:

	ReadDir(dirname string) ([]os.FileInfo, error)
}

// WriteableFS is our internal filesystem interface for filesystems we write to.
// It can wrap whatever implementations we want.
type WriteableFS interface {
	Fs

	// WriteFile writes data to the named file, creating it if necessary. If
	// the file does not exist, WriteFile creates it with permissions perm
	// (before umask); otherwise WriteFile truncates it before writing,
	// without changing permissions. Since Writefile requires multiple
	// system calls to complete, a failure mid-operation can leave the file
	// in a partially written state.
	//
	// This mimics the internal os.WriteFile function and has the same docs.
	WriteFile(name string, data []byte, perm os.FileMode) error
}
