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

package integration

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
)

var mgmt string

// TestMain ensures a temporary environment is created (and cleaned afterward) to perform tests in
func TestMain(m *testing.M) {
	// get absolute path for mgmt binary from testenvironment
	mgmt = os.Getenv("MGMT")
	// fallback to assumption based on current directory if path is not provided
	if mgmt == "" {
		path, err := filepath.Abs("../mgmt")
		if err != nil {
			log.Printf("failed to get absolute mgmt path")
			os.Exit(1)
		}
		mgmt = path
	}
	if _, err := os.Stat(mgmt); os.IsNotExist(err) {
		log.Printf("mgmt executable %s does not exist", mgmt)
		os.Exit(1)
	}

	// move to clean/stateless directory before running tests
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("failed to get current directory")
		os.Exit(1)
	}
	tmpdir, err := ioutil.TempDir("", "mgmt-integrationtest")
	if err != nil {
		log.Printf("failed to create test working directory")
		os.Exit(1)
	}
	if err := os.Chdir(tmpdir); err != nil {
		log.Printf("failed to enter test working directory")
		os.Exit(1)
	}

	// run all the tests
	os.Exit(m.Run())

	// and back to where we started
	os.Chdir(cwd)

	if err := os.RemoveAll(tmpdir); err != nil {
		log.Printf("failed to remove working directory")
		os.Exit(1)
	}
}
