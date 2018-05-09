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

// +build !root

package etcd

import (
	"testing"

	etcdtypes "github.com/coreos/etcd/pkg/types"
)

func TestNewEmbdEtcd(t *testing.T) {
	// should return a new etcd object

	noServer := false
	var flags Flags

	obj := NewEmbdEtcd("", nil, nil, nil, nil, nil, noServer, 0, flags, "", nil)
	if obj == nil {
		t.Fatal("failed to create server object")
	}
}

func TestNewEmbdEtcdConfigValidation(t *testing.T) {
	// running --no-server with no --seeds specified should fail early

	seeds := make(etcdtypes.URLs, 0)
	noServer := true
	var flags Flags

	obj := NewEmbdEtcd("", seeds, nil, nil, nil, nil, noServer, 0, flags, "", nil)
	if obj != nil {
		t.Fatal("server initialization should fail on invalid configuration")
	}
}
