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

//go:build !root

package util

import (
	"bytes"
	"os"
	"sort"
	"testing"

	"github.com/spf13/afero"
)

var dirInputs = []struct {
	srcDirFull  string
	srcCopyRoot string
	dstCopyRoot string
	dstExpected string
	force       bool
}{
	{"/tmp/foo/bar/baz/", "/tmp/foo/", "/", "/foo/bar/baz", false},
	{"/tmp/zoo/zar/zaz/", "/tmp/zoo/zar", "/start/dir", "/start/dir/zar/zaz", false},
	{"/foo", "/foo", "/", "/foo", false},
	{"/foo", "/foo", "/", "/foo", true},
}

func TestCopyFs1(t *testing.T) {
	for _, tt := range dirInputs {
		src := afero.NewMemMapFs()
		dst := afero.NewMemMapFs()

		t.Run(tt.srcDirFull, func(t *testing.T) {
			err := src.MkdirAll(tt.srcDirFull, 0700)
			if err != nil {
				t.Errorf("could not MkdirAll %+v", err)
				return
			}
			err = CopyFs(src, dst, tt.srcCopyRoot, tt.dstCopyRoot, tt.force, false)
			if err != nil {
				t.Errorf("error copying source %s to dest %s", tt.srcCopyRoot, tt.dstCopyRoot)
				return
			}

			isDir, err := afero.IsDir(dst, tt.dstExpected)
			if err != nil {
				t.Errorf("could not check IsDir: %+v", err)
				return
			}
			if !isDir {
				t.Errorf("expected directory tree %s to exist in dest", tt.dstExpected)
				return
			}
		})
	}
}

func TestCopyFs2(t *testing.T) {
	tree := "/foo/bar/baz/"
	var files = []struct {
		path    string
		content []byte
	}{
		{"/foo/foo.txt", []byte("foo")},
		{"/foo/bar/bar.txt", []byte("bar")},
		{"/foo/bar/baz/baz.txt", []byte("baz")},
	}

	src := afero.NewMemMapFs()
	dst := afero.NewMemMapFs()

	err := src.MkdirAll(tree, 0700)
	if err != nil {
		t.Errorf("could not MkdirAll: %+v", err)
		return
	}

	for _, f := range files {
		err = afero.WriteFile(src, f.path, f.content, 0600)
		if err != nil {
			t.Errorf("could not WriteFile: %+v", err)
			return
		}
	}

	if err = CopyFs(src, dst, "", "", false, false); err != nil {
		t.Errorf("could not CopyFs: %+v", err)
		return
	}

	for _, f := range files {
		content, err := afero.ReadFile(dst, f.path)
		if err != nil {
			t.Errorf("could not ReadFile: %+v", err)
			return
		}
		if !bytes.Equal(content, f.content) {
			t.Errorf("expected: %s, actual: %s, for file %s", string(f.content), string(content), f.path)
			return
		}
	}
}

func TestCopyDiskToFs1(t *testing.T) {
	dir, err := TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		treeFile := f + ".tree" // expected tree file
		treeFileFull := dir + treeFile
		info, err := os.Stat(treeFileFull)
		if err != nil || info.IsDir() {
			//t.Logf("skipping: %s -> %+v", treeFile, err)
			continue
		}
		content, err := os.ReadFile(treeFileFull)
		if err != nil {
			t.Errorf("could not read tree file: %+v", err)
			return
		}
		str := string(content) // expected tree

		t.Logf("testing: %s", treeFile)

		mmFs := afero.NewMemMapFs()
		afs := &afero.Afero{Fs: mmFs} // wrap to implement the fs API's
		fs := &AferoFs{Afero: afs}

		if err := CopyDiskToFs(fs, dir+f+"/", "/", false); err != nil {
			t.Errorf("copying to fs failed: %+v", err)
			return
		}

		// this shows us what we pulled in from the test dir:
		tree, err := FsTree(fs, "/")
		if err != nil {
			t.Errorf("tree failed: %+v", err)
			return
		}
		t.Logf("tree:\n%s", tree)

		if tree != str {
			t.Errorf("trees differ for: %s", treeFile)
			return
		}
	}
}

func TestCopyDiskToFs2(t *testing.T) {
	dir, err := TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		treeFile := f + ".tree" // expected tree file
		treeFileFull := dir + treeFile
		info, err := os.Stat(treeFileFull)
		if err != nil || info.IsDir() {
			//t.Logf("skipping: %s -> %+v", treeFile, err)
			continue
		}
		content, err := os.ReadFile(treeFileFull)
		if err != nil {
			t.Errorf("could not read tree file: %+v", err)
			return
		}
		str := string(content) // expected tree

		t.Logf("testing: %s", treeFile)

		mmFs := afero.NewMemMapFs()
		afs := &afero.Afero{Fs: mmFs} // wrap to implement the fs API's
		fs := &AferoFs{Afero: afs}

		src := dir + f + "/"
		dst := "/dest/"
		t.Logf("cp `%s` -> `%s`", src, dst)
		if err := CopyDiskToFs(fs, src, dst, false); err != nil {
			t.Errorf("copying to fs failed: %+v", err)
			return
		}

		// this shows us what we pulled in from the test dir:
		tree, err := FsTree(fs, "/")
		if err != nil {
			t.Errorf("tree failed: %+v", err)
			return
		}
		t.Logf("tree:\n%s", tree)

		if tree != str {
			t.Errorf("trees differ for: %s", treeFile)
			return
		}
	}
}

func TestCopyDiskContentsToFs1(t *testing.T) {
	dir, err := TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		treeFile := f + ".tree" // expected tree file
		treeFileFull := dir + treeFile
		info, err := os.Stat(treeFileFull)
		if err != nil || info.IsDir() {
			//t.Logf("skipping: %s -> %+v", treeFile, err)
			continue
		}
		content, err := os.ReadFile(treeFileFull)
		if err != nil {
			t.Errorf("could not read tree file: %+v", err)
			return
		}
		str := string(content) // expected tree

		t.Logf("testing: %s", treeFile)

		mmFs := afero.NewMemMapFs()
		afs := &afero.Afero{Fs: mmFs} // wrap to implement the fs API's
		fs := &AferoFs{Afero: afs}

		if err := CopyDiskContentsToFs(fs, dir+f+"/", "/", false); err != nil {
			t.Errorf("copying to fs failed: %+v", err)
			return
		}

		// this shows us what we pulled in from the test dir:
		tree, err := FsTree(fs, "/")
		if err != nil {
			t.Errorf("tree failed: %+v", err)
			return
		}
		t.Logf("tree:\n%s", tree)

		if tree != str {
			t.Errorf("trees differ for: %s", treeFile)
			return
		}
	}
}
