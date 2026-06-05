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

//go:build !root && race

package lib

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/gapi/empty"
)

func TestMainRunEtcdStartupErrorRace(t *testing.T) {
	clientListener, clientURL := localTCPListener(t)
	if err := clientListener.Close(); err != nil {
		t.Fatalf("could not release client listener: %v", err)
	}

	peerListener, peerURL := localTCPListener(t)
	defer peerListener.Close()

	prefix := t.TempDir()
	obj := &Main{
		Config: &Config{
			Program:       "mgmt",
			Version:       "test",
			Logf:          func(format string, v ...interface{}) {},
			Prefix:        &prefix,
			NoPgp:         true,
			NoRaiseLimits: true,
			ClientURLs:    []string{clientURL},
			ServerURLs:    []string{peerURL},
		},
		Deploy: &gapi.Deploy{
			Name: empty.Name,
			GAPI: &empty.GAPI{},
		},
	}
	if err := obj.Validate(); err != nil {
		t.Fatalf("could not validate main: %v", err)
	}
	if err := obj.Init(); err != nil {
		t.Fatalf("could not initialize main: %v", err)
	}

	err := obj.Run(context.Background())
	if err == nil {
		t.Fatalf("expected etcd startup error")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("expected bind error, got: %+v", err)
	}
}

func localTCPListener(t *testing.T) (net.Listener, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on localhost: %v", err)
	}
	return listener, "http://" + listener.Addr().String()
}
