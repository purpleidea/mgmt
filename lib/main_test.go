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

package lib

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

// test logic for (tmp) prefix creation
func TestPrefix(t *testing.T) {
	workdir, err := ioutil.TempDir("", "mgmt-test-")
	if err != nil {
		t.Errorf("failed to setup test environment: %v", err)
	}
	hostname := "example.com"

	// test normal prefix creation
	obj := &Main{}
	// use 'deep' prefix path to test parent directory creation
	prefix1 := path.Join(workdir, "var", "lib", "prefix1")
	obj.Prefix = &prefix1
	// create prefix
	prefix, err := obj.createPrefix(hostname)
	// should not have failed
	if err != nil {
		t.Errorf("unexpected error during prefix creation: %v", err)
	}
	// expected prefix should be returned
	if prefix != prefix1 {
		t.Errorf("wrong prefix returned: %s, expected: %s", prefix, prefix1)
	}
	// directory should have been created
	if _, err := os.Stat(prefix); os.IsNotExist(err) {
		t.Errorf("prefix directory not created")
	}

	// tmp-prefix fallback
	obj = &Main{}
	obj.AllowTmpPrefix = true
	prefix2 := path.Join(workdir, "prefix2")
	// create a file on the prefix path so directory creation fails
	os.OpenFile(prefix2, os.O_RDONLY|os.O_CREATE, 0666)
	obj.Prefix = &prefix2
	// create prefix
	prefix, err = obj.createPrefix(hostname)
	// should not have failed
	if err != nil {
		t.Errorf("unexpected error during prefix creation: %v", err)
	}
	// check if prefix returned is a tmp-prefix
	if strings.HasPrefix(prefix, path.Join(workdir, fmt.Sprintf("-%s-", hostname))) {
		t.Errorf("wrong prefix returned: %s, expected: %s", prefix, prefix2)
	}
	// check if tmp-prefix is actually created
	if _, err := os.Stat(prefix); os.IsNotExist(err) {
		t.Errorf("prefix directory not created")
	}

	// explicit tmp-prefix creation
	obj = &Main{}
	obj.TmpPrefix = true
	prefix3 := path.Join(workdir, "prefix3")
	// create a file on the prefix path so directory creation fails
	os.OpenFile(prefix3, os.O_RDONLY|os.O_CREATE, 0666)
	obj.Prefix = &prefix3
	// create prefix
	prefix, err = obj.createPrefix(hostname)
	// should not have failed
	if err != nil {
		t.Errorf("unexpected error during prefix creation: %v", err)
	}
	// check if prefix returned is a tmp-prefix
	if strings.HasPrefix(prefix, path.Join(workdir, fmt.Sprintf("-%s-", hostname))) {
		t.Errorf("wrong prefix returned: %s, expected: %s", prefix, prefix3)
	}
	// check if tmp-prefix is actually created
	if _, err := os.Stat(prefix); os.IsNotExist(err) {
		t.Errorf("prefix directory not created")
	}

	// prefix create fail, no fallback
	obj = &Main{}
	prefix4 := path.Join(workdir, "prefix4")
	// create a file on the prefix path so directory creation fails
	os.OpenFile(prefix4, os.O_RDONLY|os.O_CREATE, 0666)
	obj.Prefix = &prefix4
	// create prefix
	prefix, err = obj.createPrefix(hostname)
	// should fail
	if err == nil {
		t.Errorf("unexpected success during prefix creation")
	}
}
