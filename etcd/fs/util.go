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

package fs

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// ReadAll reads from r until an error or EOF and returns the data it read. A
// successful call returns err == nil, not err == EOF. Because ReadAll is
// defined to read from src until EOF, it does not treat an EOF from Read as an
// error to be reported.
//func (obj *Fs) ReadAll(r io.Reader) ([]byte, error) {
//	// NOTE: doesn't need Fs, same as ioutil.ReadAll package
//	return afero.ReadAll(r)
//}

// ReadDir reads the directory named by dirname and returns a list of sorted
// directory entries.
func (obj *Fs) ReadDir(dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(obj, dirname)
}

// ReadFile reads the file named by filename and returns the contents. A
// successful call returns err == nil, not err == EOF. Because ReadFile reads
// the whole file, it does not treat an EOF from Read as an error to be
// reported.
func (obj *Fs) ReadFile(filename string) ([]byte, error) {
	return afero.ReadFile(obj, filename)
}

// TempDir creates a new temporary directory in the directory dir with a name
// beginning with prefix and returns the path of the new directory. If dir is
// the empty string, TempDir uses the default directory for temporary files (see
// os.TempDir). Multiple programs calling TempDir simultaneously will not choose
// the same directory. It is the caller's responsibility to remove the directory
// when no longer needed.
func (obj *Fs) TempDir(dir, prefix string) (name string, err error) {
	return afero.TempDir(obj, dir, prefix)
}

// TempFile creates a new temporary file in the directory dir with a name
// beginning with prefix, opens the file for reading and writing, and returns
// the resulting *File. If dir is the empty string, TempFile uses the default
// directory for temporary files (see os.TempDir). Multiple programs calling
// TempFile simultaneously will not choose the same file. The caller can use
// f.Name() to find the pathname of the file. It is the caller's responsibility
// to remove the file when no longer needed.
func (obj *Fs) TempFile(dir, prefix string) (f afero.File, err error) {
	return afero.TempFile(obj, dir, prefix)
}

// WriteFile writes data to a file named by filename. If the file does not
// exist, WriteFile creates it with permissions perm; otherwise WriteFile
// truncates it before writing.
func (obj *Fs) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(obj, filename, data, perm)
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The files are walked in lexical
// order, which makes the output deterministic but means that for very large
// directories Walk can be inefficient. Walk does not follow symbolic links.
func (obj *Fs) Walk(root string, walkFn filepath.WalkFunc) error {
	return afero.Walk(obj, root, walkFn)
}
