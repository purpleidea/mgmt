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
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	stringExistingInHelp = "--version, -v  print the version"
)

// TestHelp verified the most simple invocation of mgmt does not fail.
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestHelp(t *testing.T) {
	out, err := exec.Command(mgmt).Output()
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	if !strings.Contains(string(out), stringExistingInHelp) {
		t.Logf("Command output: %s", string(out))
		t.Fatal("Expected output not found")
	}
}

// TestSmoketest makes sure the most basic run functionality works.
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestSmoketest(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t.Failed())

	// run mgmt to convergence
	assert.Nil(t, m.Run())

	// verify output contains what is expected from a converging and finished run
	assert.Nil(t, m.Finished(true))
}

// TestSimple applies a simple mcl file and tests the result.
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestSimple(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t.Failed())

	// apply the configration from lang/simple.mcl
	assert.Nil(t, m.RunLangFile("lang/simple.mcl"))

	// verify output contains what is expected from a converging and finished run
	assert.Nil(t, m.Finished(true))

	// verify if a non-empty `pass` file is created in the working directory
	assert.Nil(t, m.Pass())
}

// TestDeploy checks if background running and deployment works.
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestDeploy(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t.Failed())

	// start a mgmt instance running in the background
	assert.Nil(t, m.RunBackground())

	// wait until server is up and running
	assert.Nil(t, m.WaitUntilIdle())

	// expect mgmt to listen on default client and server url
	if _, err := http.Get("http://127.0.0.1:2379"); err != nil {
		t.Fatal("default client url is not reachable over tcp")
	}
	if _, err := http.Get("http://127.0.0.1:2380"); err != nil {
		t.Fatal("default server url is not reachable over tcp")
	}

	// deploy lang file to the just started instance
	assert.Nil(t, m.DeployLangFile("lang/simple.mcl"))

	// wait for deploy to come to a rest
	assert.Nil(t, m.WaitUntilConverged())

	// stop the running instance
	assert.Nil(t, m.StopBackground())

	// verify output contains what is expected from a converged and cleanly finished run
	assert.Nil(t, m.Finished(false))

	// verify if a non-empty `pass` file is created in the working directory
	assert.Nil(t, m.Pass())
}

// TestDeployLang tests deploying lang code directly.
// This also is the most lean/simple example for a deploy integrationtest
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestDeployLang(t *testing.T) {
	m := Instance{}
	defer m.Cleanup(t.Failed())

	assert.Nil(t, m.RunBackground())
	assert.Nil(t, m.WaitUntilIdle())

	// deploy lang file to the just started instance
	assert.Nil(t, m.DeployLang(`
	$root = getenv("MGMT_TEST_ROOT")
	file "${root}/pass" {
		content => "not empty",
	}
	`))
	assert.Nil(t, m.WaitUntilConverged())

	assert.Nil(t, m.Pass())
}
