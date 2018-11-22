// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// from the ioutil package:
// NopCloser(r io.Reader) io.ReadCloser // not implemented here
// ReadAll(r io.Reader) ([]byte, error)
// ReadDir(dirname string) ([]os.FileInfo, error)
// ReadFile(filename string) ([]byte, error)
// TempDir(dir, prefix string) (name string, err error)
// TempFile(dir, prefix string) (f *os.File, err error) // slightly different here
// WriteFile(filename string, data []byte, perm os.FileMode) error

// Fs is an interface that represents this file system API that we support.
// TODO: this should be in the gapi package or elsewhere.
type Fs interface {
	//fmt.Stringer // TODO: add this method?
	afero.Fs     // TODO: why doesn't this interface exist in the os pkg?
	URI() string // returns the URI for this file system

	//DirExists(path string) (bool, error)
	//Exists(path string) (bool, error)
	//FileContainsAnyBytes(filename string, subslices [][]byte) (bool, error)
	//FileContainsBytes(filename string, subslice []byte) (bool, error)
	//FullBaseFsPath(basePathFs *BasePathFs, relativePath string) string
	//GetTempDir(subPath string) string
	//IsDir(path string) (bool, error)
	//IsEmpty(path string) (bool, error)
	//NeuterAccents(s string) string
	//ReadAll(r io.Reader) ([]byte, error) // not needed, same as ioutil
	ReadDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	//SafeWriteReader(path string, r io.Reader) (err error)
	TempDir(dir, prefix string) (name string, err error)
	TempFile(dir, prefix string) (f afero.File, err error) // slightly different from upstream
	//UnicodeSanitize(s string) string
	//Walk(root string, walkFn filepath.WalkFunc) error
	WriteFile(filename string, data []byte, perm os.FileMode) error
	//WriteReader(path string, r io.Reader) (err error)
}
