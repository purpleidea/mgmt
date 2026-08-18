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

package resources

import (
	"context"
	"testing"
)

type remoteTestWorld struct {
	advertisedEndpoints map[string][]string
}

func (obj *remoteTestWorld) LocalEndpoints(context.Context) ([]string, error) {
	return []string{}, nil
}

func (obj *remoteTestWorld) AdvertisedEndpoints(context.Context) (map[string][]string, error) {
	return obj.advertisedEndpoints, nil
}

func (obj *remoteTestWorld) WatchEndpoints(context.Context) (<-chan error, error) {
	return nil, nil
}

func TestRemoteGetSeedsFiltersUnspecifiedAddresses(t *testing.T) {
	obj := &RemoteRes{
		world: &remoteTestWorld{
			advertisedEndpoints: map[string][]string{
				"host-a": {
					"http://localhost:2379",
					"http://127.0.0.1:2379",
					"http://0.0.0.0:2379",
					"http://[::1]:2379",
					"http://[::]:2379",
					"http://192.0.2.1:2379",
				},
				"host-b": {
					"http://etcd.example.com:2379",
				},
			},
		},
	}

	got, err := obj.getSeeds(context.Background())
	if err != nil {
		t.Fatalf("getSeeds failed: %+v", err)
	}
	want := "http://127.0.0.1:2379,http://192.0.2.1:2379,http://[::1]:2379,http://etcd.example.com:2379,http://localhost:2379"
	if got != want {
		t.Errorf("got seeds %q, want %q", got, want)
	}
}
