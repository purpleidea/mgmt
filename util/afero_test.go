// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

// +build !root

package util

import (
	"bytes"
	"io/ioutil"
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
			err = CopyFs(src, dst, tt.srcCopyRoot, tt.dstCopyRoot, tt.force)
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

	if err = CopyFs(src, dst, "", "", false); err != nil {
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
	files, err := ioutil.ReadDir(dir)
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
		content, err := ioutil.ReadFile(treeFileFull)
		if err != nil {
			t.Errorf("could not read tree file: %+v", err)
			return
		}
		str := string(content) // expected tree

		t.Logf("testing: %s", treeFile)

		mmFs := afero.NewMemMapFs()
		afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
		fs := &Fs{afs}

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

func TestCopyDiskContentsToFs1(t *testing.T) {
	dir, err := TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)
	files, err := ioutil.ReadDir(dir)
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
		content, err := ioutil.ReadFile(treeFileFull)
		if err != nil {
			t.Errorf("could not read tree file: %+v", err)
			return
		}
		str := string(content) // expected tree

		t.Logf("testing: %s", treeFile)

		mmFs := afero.NewMemMapFs()
		afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
		fs := &Fs{afs}

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
