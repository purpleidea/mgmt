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

	errwrap "github.com/pkg/errors"
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
	defer m.Cleanup(t)

	// run mgmt to convergence
	m.Run(t)

	// verify output contains what is expected from a converging and finished run
	m.Finished(t, true)
}

// TestSimple applies a simple mcl file and tests the result.
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestSimple(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// apply the configration from lang/simple.mcl
	m.RunLangFile(t, "lang/simple.mcl")

	// verify output contains what is expected from a converging and finished run
	m.Finished(t, true)

	// verify if a non-empty `pass` file is created in the working directory
	m.Pass(t)
}

// TestDeploy checks if background running and deployment works.
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestDeploy(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)

	// wait until server is up and running
	m.WaitUntilIdle(t)

	// expect mgmt to listen on default client and server url
	if _, err := http.Get("http://127.0.0.1:2379"); err != nil {
		t.Fatal("default client url is not reachable over tcp")
	}
	if _, err := http.Get("http://127.0.0.1:2380"); err != nil {
		t.Fatal("default server url is not reachable over tcp")
	}

	// deploy lang file to the just started instance
	out, err := m.DeployLangFile(nil, "lang/simple.mcl")
	if err != nil {
		t.Fatal(errwrap.Wrapf(err, "deploy command failed, output: %s", out))
	}

	// wait for deploy to come to a rest
	m.WaitUntilConverged(t)

	// stop the running instance
	m.StopBackground(t)

	// verify output contains what is expected from a converged and cleanly finished run
	m.Finished(t, false)

	// verify if a non-empty `pass` file is created in the working directory
	m.Pass(t)
}

// TestDeployLang tests deploying lang code directly.
// This also is the most lean/simple example for a deploy integrationtest
// If this test fails assumptions made by the rest of the testsuite are invalid.
func TestDeployLang(t *testing.T) {
	m := Instance{}
	defer m.Cleanup(t)

	m.RunBackground(t)
	m.WaitUntilIdle(t)

	// deploy lang file to the just started instance
	m.DeployLang(t, `
	$root = getenv("MGMT_TEST_ROOT")
	file "${root}/pass" {
		content => "not empty",
	}
	`)
	m.WaitUntilConverged(t)

	m.Pass(t)
}
