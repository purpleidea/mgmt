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
	"net"
	"net/http"
	"os"
	"path"
	"testing"
	"time"
)

// TestDomainSockets verifies mgmt run and deploy work over socket files.
func TestDomainSockets(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{UnixSockets: true}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)

	// TODO: sleep is bad UX on testing, wait for correct output on mgmt run log
	time.Sleep(5 * time.Second)

	// verify correct socket files exist and no tcp ports are open
	if _, err := os.Stat(path.Join(m.Prefix, "clients.sock:0")); os.IsNotExist(err) {
		t.Fatal("client socket file does not exist")
	}
	if _, err := os.Stat(path.Join(m.Prefix, "servers.sock:0")); os.IsNotExist(err) {
		t.Fatal("server socket file does not exist")
	}
	// TODO: test for connection refused instead of any error
	if _, err := net.Dial("tcp", "127.0.0.1:2379"); err == nil {
		t.Fatal("default client url is reachable over tcp")
	}
	// TODO: test for connection refused instead of any error
	if _, err := http.Get("http://127.0.0.1:2380"); err == nil {
		t.Fatal("default server url is reachable over tcp")
	}

	// deploy lang file to the just started instance
	m.DeployLangFile(t, "lang/simple.mcl")

	// wait for instance to converge and stop it
	m.StopBackground(t)

	// verify output contains what is expected from a converging and finished run
	m.Finished(t, false)

	// verify if a non-empty `pass` file is created in the working directory
	m.Pass(t)
}

// TestDomainSocketsLV verifies that kv store interaction is not broken using unix domain sockets
func TestDomainSocketsKv(t *testing.T) {
	// create an mgmt test environment and ensure cleanup/debug logging on failure/exit
	m := Instance{UnixSockets: true}
	defer m.Cleanup(t)

	// start a mgmt instance running in the background
	m.RunBackground(t)

	// TODO: sleep is bad UX on testing, wait for correct output on mgmt run log
	time.Sleep(5 * time.Second)

	// deploy lang file to the just started instance
	m.DeployLangFile(t, "lang/kv.mcl")

	// wait for instance to converge and stop it
	m.StopBackground(t)

	// verify output contains what is expected from a converging and finished run
	m.Finished(t, false)

	// verify if a non-empty `pass` file is created in the working directory
	m.Pass(t)
}
