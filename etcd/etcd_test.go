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

//go:build !root

package etcd

import (
	"net/url"
	"strings"
	"testing"

	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
)

func TestValidation1(t *testing.T) {
	// running with no --seeds should not validate at the moment
	embdEtcd := &EmbdEtcd{
		//Seeds: etcdtypes.URLs{},
	}
	if err := embdEtcd.Validate(); err == nil {
		t.Errorf("expected validation err, got nil")
	}
	if err := embdEtcd.Init(); err == nil {
		t.Errorf("expected init err, got nil")
		defer embdEtcd.Cleanup()
	}
}

func TestValidateOverlappingURLs(t *testing.T) {
	mustURL := func(s string) url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatalf("bad test URL %q: %v", s, err)
		}
		return *u
	}
	logf := func(format string, v ...interface{}) {}

	// Same host:port for both client and server should fail.
	embdEtcd := &EmbdEtcd{
		Hostname:   "test",
		ClientURLs: etcdtypes.URLs{mustURL("http://127.0.0.1:2381")},
		ServerURLs: etcdtypes.URLs{mustURL("http://127.0.0.1:2381")},
		NS:         "test",
		Prefix:     "/tmp/test",
		Logf:       logf,
	}
	err := embdEtcd.Validate()
	if err == nil {
		t.Errorf("expected error for overlapping client/server URLs, got nil")
	} else if !strings.Contains(err.Error(), "share the same host:port") {
		t.Errorf("expected host:port overlap error, got: %v", err)
	}

	// Different ports should pass this check (may fail others).
	embdEtcd2 := &EmbdEtcd{
		Hostname:   "test",
		ClientURLs: etcdtypes.URLs{mustURL("http://127.0.0.1:2379")},
		ServerURLs: etcdtypes.URLs{mustURL("http://127.0.0.1:2380")},
		NS:         "test",
		Prefix:     "/tmp/test",
		Logf:       logf,
	}
	err = embdEtcd2.Validate()
	if err != nil {
		t.Errorf("expected no error for non-overlapping URLs, got: %v", err)
	}
}
