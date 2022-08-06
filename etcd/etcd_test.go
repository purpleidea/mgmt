// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

//go:build !root

package etcd

import (
	"testing"
)

func TestValidation1(t *testing.T) {
	// running --no-server with no --seeds should not validate at the moment
	embdEtcd := &EmbdEtcd{
		//Seeds: etcdtypes.URLs{},
		NoServer: true,
	}
	if err := embdEtcd.Validate(); err == nil {
		t.Errorf("expected validation err, got nil")
	}
	if err := embdEtcd.Init(); err == nil {
		t.Errorf("expected init err, got nil")
		defer embdEtcd.Close()
	}
}
