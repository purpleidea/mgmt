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
	"os"
	"path/filepath"
	"runtime"
)

// TestDir gets the absolute path to the test directory if it exists. This is a
// utility function that is used in some tests.
func TestDir(suffix string) (string, error) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("could not determine filename")
	}
	dir := filepath.Dir(filename) + "/" // location of this test
	testDir := dir + suffix             // test directory
	if info, err := os.Stat(testDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("error getting test dir, err was: %+v", err)
	}

	return testDir, nil
}
