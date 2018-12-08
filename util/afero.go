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

package util

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/afero"
)

// FsTree returns a string representation of the file system tree similar to the
// well-known `tree` command.
func FsTree(fs afero.Fs, name string) (string, error) {
	str := ".\n" // top level dir
	s, err := stringify(fs, path.Clean(name), []bool{})
	if err != nil {
		return "", err
	}
	str += s
	return str, nil
}

func stringify(fs afero.Fs, name string, indent []bool) (string, error) {
	str := ""
	dir, err := fs.Open(name)
	if err != nil {
		return "", err
	}

	fileinfo, err := dir.Readdir(-1)
	if err != nil && err != io.EOF {
		return "", err
	}
	for i, fi := range fileinfo {
		for _, last := range indent {
			if last {
				str += "    "
			} else {
				str += "│   "
			}
		}

		header := "├── "
		var last bool
		if i == len(fileinfo)-1 { // if last
			header = "└── "
			last = true
		}

		p := fi.Name()
		if fi.IsDir() {
			p += "/" // identify as a dir
		}
		str += fmt.Sprintf("%s%s\n", header, p)
		if fi.IsDir() {
			indented := append(indent, last)
			s, err := stringify(fs, path.Join(name, p), indented)
			if err != nil {
				return "", err // TODO: return partial tree?
			}
			str += s
		}
	}
	return str, nil
}

// CopyFs copies a dir from the srcFs to a dir on the dstFs. It expects that the
// dst will be either empty, or that the force flag will be set to true. If the
// dst has a different set of contents in the same location, the behaviour is
// currently undefined.
// TODO: this should be made more rsync like and robust!
func CopyFs(srcFs, dstFs afero.Fs, src, dst string, force bool) error {
	if src == "" {
		src = "/"
	}
	if dst == "" {
		dst = "/"
	}
	walkFn := func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		//perm := info.Perm()
		perm := info.Mode() // TODO: is this correct?
		p := path.Join(dst, filepath.Base(name))
		if info.IsDir() {
			err := dstFs.Mkdir(p, perm)
			if os.IsExist(err) && (name == "/" || force) {
				return nil
			}
			return err
		}

		data, err := afero.ReadFile(srcFs, name)
		if err != nil {
			return err
		}
		// create file
		return afero.WriteFile(dstFs, p, data, perm)
	}

	return afero.Walk(srcFs, src, walkFn)
}

// CopyFsToDisk performs exactly as CopyFs, except that the dst fs is our local
// disk os fs.
func CopyFsToDisk(srcFs afero.Fs, src, dst string, force bool) error {
	return CopyFs(srcFs, afero.NewOsFs(), src, dst, force)
}

// CopyDiskToFs performs exactly as CopyFs, except that the src fs is our local
// disk os fs.
func CopyDiskToFs(dstFs afero.Fs, src, dst string, force bool) error {
	return CopyFs(afero.NewOsFs(), dstFs, src, dst, force)
}
