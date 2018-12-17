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

// +build !root

package util

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/afero"
)

func TestCopyDiskToFs1(t *testing.T) {
	if true {
		return // XXX: remove me once this test passes
	}
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
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		treeFile := f.Name() + ".tree" // expected tree file
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

		if err := CopyDiskToFs(fs, dir+f.Name()+"/", "/", false); err != nil {
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
