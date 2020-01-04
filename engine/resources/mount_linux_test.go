// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

// +build !root !darwin

package resources

import (
	"io/ioutil"
	"os"
	"testing"

	fstab "github.com/deniswernert/go-fstab"
)

func TestMountExists(t *testing.T) {
	const procMock1 = `/tmp/mount0 /mnt/proctest ext4 rw,seclabel,relatime,data=ordered 0 0` + "\n"

	var mountExistsTests = []struct {
		procMock []byte
		in       *fstab.Mount
		out      bool
	}{
		{
			[]byte(procMock1),
			&fstab.Mount{
				Spec:    "/tmp/mount0",
				File:    "/mnt/proctest",
				VfsType: "ext4",
				MntOps:  map[string]string{"defaults": ""},
				Freq:    1,
				PassNo:  1,
			},
			true,
		},
	}

	file, err := ioutil.TempFile("", "proc")
	if err != nil {
		t.Errorf("error creating temp file: %v", err)
		return
	}
	defer os.Remove(file.Name())
	for _, test := range mountExistsTests {
		if err := ioutil.WriteFile(file.Name(), test.procMock, 0664); err != nil {
			t.Errorf("error writing proc file: %s: %v", file.Name(), err)
			return
		}
		if err := ioutil.WriteFile(test.in.Spec, []byte{}, 0664); err != nil {
			t.Errorf("error writing fstab file: %s: %v", file.Name(), err)
			return
		}
		result, err := mountExists(file.Name(), test.in)
		if err != nil {
			t.Errorf("error checking if fstab entry %s exists: %v", test.in.String(), err)
			return
		}
		if result != test.out {
			t.Errorf("mountExistsTests test wanted: %t, got: %t", test.out, result)
		}
	}
}
