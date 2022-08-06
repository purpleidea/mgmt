// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

//go:build !root

package resources

import (
	"io/ioutil"
	"os"
	"testing"

	fstab "github.com/deniswernert/go-fstab"
)

const fstabMock1 = `UUID=ef5726f2-615c-4350-b0ab-f106e5fc90ad / ext4 defaults 1 1` + "\n"

var fstabWriteTests = []struct {
	in fstab.Mounts
}{
	{
		fstab.Mounts{
			&fstab.Mount{
				Spec:    "UUID=00112233-4455-6677-8899-aabbccddeeff",
				File:    "/boot",
				VfsType: "ext3",
				MntOps:  map[string]string{"defaults": ""},
				Freq:    1,
				PassNo:  2,
			},
			&fstab.Mount{
				Spec:    "/dev/mapper/home",
				File:    "/home",
				VfsType: "ext3",
				MntOps:  map[string]string{"defaults": ""},
				Freq:    1,
				PassNo:  2,
			},
		},
	},
	{
		fstab.Mounts{
			&fstab.Mount{
				Spec:    "/dev/cdrom",
				File:    "/mnt/cdrom",
				VfsType: "iso9660",
				MntOps:  map[string]string{"ro": "", "blocksize": "2048"},
			},
		},
	},
}

func (obj *MountRes) TestFstabWrite(t *testing.T) {
	file, err := ioutil.TempFile("", "fstab")
	if err != nil {
		t.Errorf("error creating temp file: %v", err)
		return
	}
	defer os.Remove(file.Name())

	for _, test := range fstabWriteTests {
		if err := obj.fstabWrite(file.Name(), test.in); err != nil {
			t.Errorf("error writing fstab file: %s: %v", file.Name(), err)
			return
		}
		for _, mount := range test.in {
			exists, err := fstabEntryExists(file.Name(), mount)
			if err != nil {
				t.Errorf("error checking if fstab entry %s exists: %v", mount.String(), err)
				return
			}
			if !exists {
				t.Errorf("failed to write %s to fstab", mount.String())
			}
		}
	}
}

var fstabEntryAddTests = []struct {
	fstabMock []byte
	in        *fstab.Mount
}{
	{
		[]byte(fstabMock1),
		&fstab.Mount{
			Spec:    "/dev/sdb1",
			File:    "/mnt/foo",
			VfsType: "ext2",
			MntOps:  map[string]string{"ro": "", "blocksize": "2048"},
		},
	},
	{
		[]byte(fstabMock1),
		&fstab.Mount{
			Spec:    "UUID=00112233-4455-6677-8899-aabbccddeeff",
			File:    "/",
			VfsType: "ext3",
			MntOps:  map[string]string{"defaults": ""},
			Freq:    1,
			PassNo:  2,
		},
	},
}

func (obj *MountRes) TestFstabEntryAdd(t *testing.T) {
	file, err := ioutil.TempFile("", "fstab")
	if err != nil {
		t.Errorf("error creating temp file: %v", err)
		return
	}
	defer os.Remove(file.Name())

	for _, test := range fstabEntryAddTests {
		if err := ioutil.WriteFile(file.Name(), test.fstabMock, 0644); err != nil {
			t.Errorf("error writing fstab file: %s: %v", file.Name(), err)
			return
		}
		err := obj.fstabEntryAdd(file.Name(), test.in)
		if err != nil {
			t.Errorf("error adding fstab entry: %s to file: %s: %v", test.in.String(), file.Name(), err)
			return
		}
		exists, err := fstabEntryExists(file.Name(), test.in)
		if err != nil {
			t.Errorf("error checking if %s exists: %v", test.in.String(), err)
			return
		}
		if !exists {
			t.Errorf("fstab failed to add entry: %s to fstab", test.in.String())
		}
	}
}

var fstabEntryRemoveTests = []struct {
	fstabMock []byte
	in        *fstab.Mount
}{
	{
		[]byte(fstabMock1),
		&fstab.Mount{
			Spec:    "UUID=ef5726f2-615c-4350-b0ab-f106e5fc90ad",
			File:    "/",
			VfsType: "ext4",
			MntOps:  map[string]string{"defaults": ""},
			Freq:    1,
			PassNo:  1,
		},
	},
}

func (obj *MountRes) TestFstabEntryRemove(t *testing.T) {
	file, err := ioutil.TempFile("", "fstab")
	if err != nil {
		t.Errorf("error creating temp file: %v", err)
		return
	}
	defer os.Remove(file.Name())

	for _, test := range fstabEntryRemoveTests {
		if err := ioutil.WriteFile(file.Name(), test.fstabMock, 0644); err != nil {
			t.Errorf("error writing fstab file: %s: %v", file.Name(), err)
			return
		}
		err := obj.fstabEntryRemove(file.Name(), test.in)
		if err != nil {
			t.Errorf("error removing fstab entry: %s from file: %s: %v", test.in.String(), file.Name(), err)
			return
		}
		exists, err := fstabEntryExists(file.Name(), test.in)
		if err != nil {
			t.Errorf("error checking if %s exists: %v", test.in.String(), err)
			return
		}
		if exists {
			t.Errorf("fstab failed to remove entry: %s from fstab", test.in.String())
		}
	}
}

var mountCompareTests = []struct {
	dIn *fstab.Mount
	pIn *fstab.Mount
	out bool
}{
	{
		&fstab.Mount{
			Spec:    "/dev/foo",
			File:    "/mnt/foo",
			VfsType: "ext3",
			MntOps:  map[string]string{"defaults": ""},
		},
		&fstab.Mount{
			Spec:    "/dev/foo",
			File:    "/mnt/foo",
			VfsType: "ext3",
			MntOps:  map[string]string{"foo": "bar", "baz": ""},
		},
		true,
	},
	{
		&fstab.Mount{
			Spec:    "UUID=00112233-4455-6677-8899-aabbccddeeff",
			File:    "/mnt/foo",
			VfsType: "ext3",
		},
		&fstab.Mount{
			Spec:    "UUID=00112233-4455-6677-8899-aabbccddeeff",
			File:    "/mnt/bar",
			VfsType: "ext3",
		},
		false,
	},
}

var fstabEntryExistsTests = []struct {
	fstabMock []byte
	in        *fstab.Mount
	out       bool
}{
	{
		[]byte(fstabMock1),
		&fstab.Mount{
			Spec:    "UUID=ef5726f2-615c-4350-b0ab-f106e5fc90ad",
			File:    "/",
			VfsType: "ext4",
			MntOps:  map[string]string{"defaults": ""},
			Freq:    1,
			PassNo:  1,
		},
		true,
	},
	{
		[]byte(fstabMock1),
		&fstab.Mount{
			Spec:    "/dev/mapper/root",
			File:    "/home",
			VfsType: "ext4",
			MntOps:  map[string]string{"defaults": ""},
			Freq:    1,
			PassNo:  1,
		},
		false,
	},
}

func TestFstabEntryExists(t *testing.T) {
	file, err := ioutil.TempFile("", "fstab")
	if err != nil {
		t.Errorf("error creating temp file: %v", err)
		return
	}
	defer os.Remove(file.Name())

	for _, test := range fstabEntryExistsTests {
		if err := ioutil.WriteFile(file.Name(), test.fstabMock, 0644); err != nil {
			t.Errorf("error writing fstab file: %s: %v", file.Name(), err)
			return
		}
		result, err := fstabEntryExists(file.Name(), test.in)
		if err != nil {
			t.Errorf("error checking if fstab entry %s exists: %v", test.in.String(), err)
			return
		}
		if result != test.out {
			t.Errorf("fstabEntryExists test wanted: %t, got: %t", test.out, result)
		}
	}
}

func TestMountCompare(t *testing.T) {
	for _, test := range mountCompareTests {
		result, err := mountCompare(test.dIn, test.pIn)
		if err != nil {
			t.Errorf("error comparing mounts: %s and %s: %v", test.dIn.String(), test.pIn.String(), err)
			return
		}
		if result != test.out {
			t.Errorf("mountCompare test wanted: %t, got: %t", test.out, result)
		}
	}
}
