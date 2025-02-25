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

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TestDir gets the absolute path to the test directory if it exists. If the dir
// does not exist, then this will error, but the path will still be returned.
// This is a utility function that is used in some tests.
func TestDir(suffix string) (string, error) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("could not determine filename")
	}
	dir := filepath.Dir(filename) + "/" // location of this test
	testDir := dir + suffix             // test directory
	if info, err := os.Stat(testDir); err != nil || !info.IsDir() {
		return testDir, fmt.Errorf("error getting test dir, err was: %+v", err)
	}

	return testDir, nil
}

// TestDirFull gets the full absolute path to a unique test directory if it
// exists. If the dir does not exist, then this will error, but the path will
// still be returned. This is a utility function that is used in some tests.
func TestDirFull() (string, error) {
	function, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("could not determine filename")
	}
	dir := filepath.Dir(filename) + "/" // location of this test
	name := filepath.Base(filename)     // something like: foo_test.go
	ext := filepath.Ext(name)
	if ext != ".go" {
		return "", fmt.Errorf("unexpected extension of: %s", ext)
	}
	name = strings.TrimSuffix(name, ext) + "/"  // remove extension, add slash
	fname := runtime.FuncForPC(function).Name() // full fqdn func name
	ix := strings.LastIndex(fname, ".")         // ends with package.<function name>
	if fname == "" || ix == -1 {
		return "", fmt.Errorf("function name not found")
	}
	fname = fname[ix+len("."):] + "/" // just the function name
	testDir := dir + name + fname     // full test directory
	if info, err := os.Stat(testDir); err != nil || !info.IsDir() {
		return testDir, fmt.Errorf("error getting test dir, err was: %+v", err)
	}

	return testDir, nil
}
